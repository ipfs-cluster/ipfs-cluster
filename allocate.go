package ipfscluster

import (
	"context"
	"errors"
	"fmt"

	cid "github.com/ipfs/go-cid"
	peer "github.com/libp2p/go-libp2p-core/peer"

	"go.opencensus.io/trace"

	"github.com/ipfs/ipfs-cluster/api"
)

// This file gathers allocation logic used when pinning or re-pinning
// to find which peers should be allocated to a Cid. Allocation is constrained
// by ReplicationFactorMin and ReplicationFactorMax parameters obtained
// from the Pin object.

// The allocation process has several steps:
//
// * Find which peers are pinning a CID
// * Obtain the last values for the configured informer metrics from the
//   monitor component
// * Divide the metrics between "current" (peers already pinning the CID)
//   and "candidates" (peers that could pin the CID), as long as their metrics
//   are valid.
// * Given the candidates:
//   * Check if we are overpinning an item
//   * Check if there are not enough candidates for the "needed" replication
//     factor.
//   * If there are enough candidates:
//     * Call the configured allocator, which sorts the candidates (and
//       may veto some depending on the allocation strategy.
//     * The allocator returns a list of final candidate peers sorted by
//       order of preference.
//     * Take as many final candidates from the list as we can, until
//       ReplicationFactorMax is reached. Error if there are less than
//       ReplicationFactorMin.

// allocate finds peers to allocate a hash using the informer and the monitor
// it should only be used with valid replicationFactors (if rplMin and rplMax
// are > 0, then rplMin <= rplMax).
// It always returns allocations, but if no new allocations are needed,
// it will return the current ones. Note that allocate() does not take
// into account if the given CID was previously in a "pin everywhere" mode,
// and will consider such Pins as currently unallocated ones, providing
// new allocations as available.
func (c *Cluster) allocate(ctx context.Context, hash cid.Cid, currentPin *api.Pin, rplMin, rplMax int, blacklist []peer.ID, priorityList []peer.ID) ([]peer.ID, error) {
	ctx, span := trace.StartSpan(ctx, "cluster/allocate")
	defer span.End()

	if (rplMin + rplMax) == 0 {
		return nil, fmt.Errorf("bad replication factors: %d/%d", rplMin, rplMax)
	}

	if rplMin < 0 && rplMax < 0 { // allocate everywhere
		return []peer.ID{}, nil
	}

	// Figure out who is holding the CID
	var currentAllocs []peer.ID
	if currentPin != nil {
		currentAllocs = currentPin.Allocations
	}

	mSet := make(api.MetricsSet)
	for _, informer := range c.informers {
		metricType := informer.Name()
		mSet[metricType] = c.monitor.LatestMetrics(ctx, metricType)
	}

	curSet, curPeers, candSet, candPeers, prioSet, prioPeers := filterMetrics(
		mSet,
		len(c.informers),
		currentAllocs,
		priorityList,
		blacklist,
	)

	newAllocs, err := c.obtainAllocations(
		ctx,
		hash,
		rplMin,
		rplMax,
		curSet,
		candSet,
		prioSet,
		curPeers,
		candPeers,
		prioPeers,
	)
	if err != nil {
		return newAllocs, err
	}
	if newAllocs == nil {
		newAllocs = currentAllocs
	}
	return newAllocs, nil
}

// Given metrics from all informers, split them into 3 MetricsSet:
// - Those corresponding to currently allocated peers
// - Those corresponding to priority allocations
// - Those corresponding to "candidate" allocations
// And return also an slice of the peers in those groups.
//
// For a metric/peer to be included in a group, it is necessary that it has
// metrics for all informers.
func filterMetrics(mSet api.MetricsSet, numInformers int, currentAllocs, priorityList, blacklist []peer.ID) (
	curSet api.MetricsSet,
	curPeers []peer.ID,
	candSet api.MetricsSet,
	candPeers []peer.ID,
	prioSet api.MetricsSet,
	prioPeers []peer.ID,
) {
	curPeersMap := make(map[peer.ID][]*api.Metric)
	candPeersMap := make(map[peer.ID][]*api.Metric)
	prioPeersMap := make(map[peer.ID][]*api.Metric)

	// Divide the metric by current/candidate/prio and by peer
	for _, metrics := range mSet {
		for _, m := range metrics {
			switch {
			case containsPeer(blacklist, m.Peer):
				// discard blacklisted peers
				continue
			case containsPeer(currentAllocs, m.Peer):
				curPeersMap[m.Peer] = append(curPeersMap[m.Peer], m)
			case containsPeer(priorityList, m.Peer):
				prioPeersMap[m.Peer] = append(prioPeersMap[m.Peer], m)
			default:
				candPeersMap[m.Peer] = append(candPeersMap[m.Peer], m)
			}
		}
	}

	fillMetricsSet := func(peersMap map[peer.ID][]*api.Metric) (api.MetricsSet, []peer.ID) {
		mSet := make(api.MetricsSet)
		peers := make([]peer.ID, 0, len(peersMap))

		// Put the metrics in their sets if peers have metrics for all informers
		// Record peers
		for p, metrics := range peersMap {
			if len(metrics) == numInformers {
				for _, m := range metrics {
					mSet[m.Name] = append(mSet[m.Name], m)
				}
				peers = append(peers, p)
			} // otherwise this peer will be ignored.
		}
		return mSet, peers
	}

	curSet, curPeers = fillMetricsSet(curPeersMap)
	candSet, candPeers = fillMetricsSet(candPeersMap)
	prioSet, prioPeers = fillMetricsSet(prioPeersMap)

	return
}

// allocationError logs an allocation error
func allocationError(hash cid.Cid, needed, wanted int, candidatesValid []peer.ID) error {
	logger.Errorf("Not enough candidates to allocate %s:", hash)
	logger.Errorf("  Needed: %d", needed)
	logger.Errorf("  Wanted: %d", wanted)
	logger.Errorf("  Valid candidates: %d:", len(candidatesValid))
	for _, c := range candidatesValid {
		logger.Errorf("    - %s", c.Pretty())
	}
	errorMsg := "not enough peers to allocate CID. "
	errorMsg += fmt.Sprintf("Needed at least: %d. ", needed)
	errorMsg += fmt.Sprintf("Wanted at most: %d. ", wanted)
	errorMsg += fmt.Sprintf("Valid candidates: %d. ", len(candidatesValid))
	errorMsg += "See logs for more info."
	return errors.New(errorMsg)
}

func (c *Cluster) obtainAllocations(
	ctx context.Context,
	hash cid.Cid,
	rplMin, rplMax int,
	currentValidMetrics, candidatesMetrics, priorityMetrics api.MetricsSet,
	currentPeers, candidatePeers, priorityPeers []peer.ID,
) ([]peer.ID, error) {
	ctx, span := trace.StartSpan(ctx, "cluster/obtainAllocations")
	defer span.End()

	nCurrentValid := len(currentPeers)
	nCandidatesValid := len(candidatePeers) + len(priorityPeers)
	needed := rplMin - nCurrentValid // The minimum we need
	wanted := rplMax - nCurrentValid // The maximum we want

	logger.Debugf("obtainAllocations: current: %d", nCurrentValid)
	logger.Debugf("obtainAllocations: candidates: %d", nCandidatesValid)
	logger.Debugf("obtainAllocations: priority: %d", len(priorityPeers))
	logger.Debugf("obtainAllocations: Needed: %d", needed)
	logger.Debugf("obtainAllocations: Wanted: %d", wanted)

	// Reminder: rplMin <= rplMax AND >0

	if wanted < 0 { // allocations above maximum threshold: drop some
		// This could be done more intelligently by dropping them
		// according to the allocator order (i.e. free-ing peers
		// with most used space first).
		return currentPeers[0 : len(currentPeers)+wanted], nil
	}

	if needed <= 0 { // allocations are above minimal threshold
		// We don't provide any new allocations
		return nil, nil
	}

	if nCandidatesValid < needed { // not enough candidates
		return nil, allocationError(hash, needed, wanted, append(priorityPeers, candidatePeers...))
	}

	// We can allocate from this point. Use the allocator to decide
	// on the priority of candidates grab as many as "wanted"

	// the allocator returns a list of peers ordered by priority
	finalAllocs, err := c.allocator.Allocate(
		ctx,
		hash,
		currentValidMetrics,
		candidatesMetrics,
		priorityMetrics,
	)
	if err != nil {
		return nil, logError(err.Error())
	}

	logger.Debugf("obtainAllocations: allocate(): %s", finalAllocs)

	// check that we have enough as the allocator may have returned
	// less candidates than provided.
	if got := len(finalAllocs); got < needed {
		return nil, allocationError(hash, needed, wanted, finalAllocs)
	}

	allocationsToUse := minInt(wanted, len(finalAllocs))

	// the final result is the currently valid allocations
	// along with the ones provided by the allocator
	return append(currentPeers, finalAllocs[0:allocationsToUse]...), nil
}
