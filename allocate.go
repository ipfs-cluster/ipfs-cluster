package ipfscluster

import (
	"errors"
	"fmt"

	cid "github.com/ipfs/go-cid"
	peer "github.com/libp2p/go-libp2p-peer"

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
// it should only be used with valid replicationFactors (rplMin and rplMax
// which are positive and rplMin <= rplMax).
// It always returns allocations, but if no new allocations are needed,
// it will return the current ones. Note that allocate() does not take
// into account if the given CID was previously in a "pin everywhere" mode,
// and will consider such Pins as currently unallocated ones, providing
// new allocations as available.
func (c *Cluster) allocate(hash *cid.Cid, rplMin, rplMax int, blacklist []peer.ID, prioritylist []peer.ID) ([]peer.ID, error) {
	// Figure out who is holding the CID
	currentPin, _ := c.getCurrentPin(hash)
	currentAllocs := currentPin.Allocations
	metrics, err := c.getInformerMetrics()
	if err != nil {
		return nil, err
	}

	currentMetrics := make(map[peer.ID]api.Metric)
	candidatesMetrics := make(map[peer.ID]api.Metric)
	priorityMetrics := make(map[peer.ID]api.Metric)

	// Divide metrics between current and candidates.
	// All metrics in metrics are valid (at least the
	// moment they were compiled by the monitor)
	for _, m := range metrics {
		switch {
		case containsPeer(blacklist, m.Peer):
			// discard blacklisted peers
			continue
		case containsPeer(currentAllocs, m.Peer):
			currentMetrics[m.Peer] = m
		case containsPeer(prioritylist, m.Peer):
			priorityMetrics[m.Peer] = m
		default:
			candidatesMetrics[m.Peer] = m
		}
	}

	newAllocs, err := c.obtainAllocations(
		hash,
		rplMin,
		rplMax,
		currentMetrics,
		candidatesMetrics,
		priorityMetrics,
	)
	if err != nil {
		return newAllocs, err
	}
	if newAllocs == nil {
		newAllocs = currentAllocs
	}
	return newAllocs, nil
}

// getCurrentPin returns the Pin object for h, if we can find one
// or builds an empty one.
func (c *Cluster) getCurrentPin(h *cid.Cid) (api.Pin, bool) {
	st, err := c.consensus.State()
	if err != nil {
		return api.PinCid(h), false
	}
	ok := st.Has(h)
	return st.Get(h), ok
}

// getInformerMetrics returns the MonitorLastMetrics() for the
// configured informer.
func (c *Cluster) getInformerMetrics() ([]api.Metric, error) {
	var metrics []api.Metric
	metricName := c.informer.Name()
	l, err := c.consensus.Leader()
	if err != nil {
		return nil, errors.New("cannot determine leading Monitor")
	}

	err = c.rpcClient.Call(l,
		"Cluster", "PeerMonitorLastMetrics",
		metricName,
		&metrics)
	if err != nil {
		return nil, err
	}
	return metrics, nil
}

// allocationError logs an allocation error
func allocationError(hash *cid.Cid, needed, wanted int, candidatesValid []peer.ID) error {
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
	hash *cid.Cid,
	rplMin, rplMax int,
	currentValidMetrics, candidatesMetrics map[peer.ID]api.Metric,
	priorityMetrics map[peer.ID]api.Metric) ([]peer.ID, error) {

	// The list of peers in current
	validAllocations := make([]peer.ID, 0, len(currentValidMetrics))
	for k := range currentValidMetrics {
		validAllocations = append(validAllocations, k)
	}

	nCurrentValid := len(validAllocations)
	nCandidatesValid := len(candidatesMetrics) + len(priorityMetrics)
	needed := rplMin - nCurrentValid // The minimum we need
	wanted := rplMax - nCurrentValid // The maximum we want

	logger.Debugf("obtainAllocations: current valid: %d", nCurrentValid)
	logger.Debugf("obtainAllocations: candidates valid: %d", nCandidatesValid)
	logger.Debugf("obtainAllocations: Needed: %d", needed)
	logger.Debugf("obtainAllocations: Wanted: %d", wanted)

	// Reminder: rplMin <= rplMax AND >0

	if wanted < 0 { // allocations above maximum threshold: drop some
		// This could be done more intelligently by dropping them
		// according to the allocator order (i.e. free-ing peers
		// with most used space first).
		return validAllocations[0 : len(validAllocations)+wanted], nil
	}

	if needed <= 0 { // allocations are above minimal threshold
		// We don't provide any new allocations
		return nil, nil
	}

	if nCandidatesValid < needed { // not enough candidates
		candidatesValid := []peer.ID{}
		for k := range priorityMetrics {
			candidatesValid = append(candidatesValid, k)
		}
		for k := range candidatesMetrics {
			candidatesValid = append(candidatesValid, k)
		}
		return nil, allocationError(hash, needed, wanted, candidatesValid)
	}

	// We can allocate from this point. Use the allocator to decide
	// on the priority of candidates grab as many as "wanted"

	// the allocator returns a list of peers ordered by priority
	finalAllocs, err := c.allocator.Allocate(
		hash, currentValidMetrics, candidatesMetrics, priorityMetrics)
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
	return append(validAllocations, finalAllocs[0:allocationsToUse]...), nil
}
