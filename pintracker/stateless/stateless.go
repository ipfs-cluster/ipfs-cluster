// Package stateless implements a PinTracker component for IPFS Cluster, which
// aims to reduce the memory footprint when handling really large cluster
// states.
package stateless

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/pintracker/optracker"
	"github.com/ipfs/ipfs-cluster/state"

	cid "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	peer "github.com/libp2p/go-libp2p-core/peer"
	rpc "github.com/libp2p/go-libp2p-gorpc"

	"go.opencensus.io/trace"
)

var logger = logging.Logger("pintracker")

var (
	// ErrFullQueue is the error used when pin or unpin operation channel is full.
	ErrFullQueue = errors.New("pin/unpin operation queue is full. Try increasing max_pin_queue_size")

	// items with this error should be recovered
	errUnexpectedlyUnpinned = errors.New("the item should be pinned but it is not")
)

// Tracker uses the optracker.OperationTracker to manage
// transitioning shared ipfs-cluster state (Pins) to the local IPFS node.
type Tracker struct {
	config *Config

	optracker *optracker.OperationTracker

	peerID   peer.ID
	peerName string

	ctx    context.Context
	cancel func()

	getState func(ctx context.Context) (state.ReadOnly, error)

	rpcClient *rpc.Client
	rpcReady  chan struct{}

	priorityPinCh chan *optracker.Operation
	pinCh         chan *optracker.Operation
	unpinCh       chan *optracker.Operation

	shutdownMu sync.Mutex
	shutdown   bool
	wg         sync.WaitGroup
}

// New creates a new StatelessPinTracker.
func New(cfg *Config, pid peer.ID, peerName string, getState func(ctx context.Context) (state.ReadOnly, error)) *Tracker {
	ctx, cancel := context.WithCancel(context.Background())

	spt := &Tracker{
		config:        cfg,
		peerID:        pid,
		peerName:      peerName,
		ctx:           ctx,
		cancel:        cancel,
		getState:      getState,
		optracker:     optracker.NewOperationTracker(ctx, pid, peerName),
		rpcReady:      make(chan struct{}, 1),
		priorityPinCh: make(chan *optracker.Operation, cfg.MaxPinQueueSize),
		pinCh:         make(chan *optracker.Operation, cfg.MaxPinQueueSize),
		unpinCh:       make(chan *optracker.Operation, cfg.MaxPinQueueSize),
	}

	for i := 0; i < spt.config.ConcurrentPins; i++ {
		go spt.opWorker(spt.pin, spt.priorityPinCh, spt.pinCh)
	}
	go spt.opWorker(spt.unpin, spt.unpinCh, nil)

	return spt
}

// we can get our IPFS id from our own monitor ping metrics which
// are refreshed regularly.
func (spt *Tracker) getIPFSID(ctx context.Context) peer.ID {
	// Wait until RPC is ready
	<-spt.rpcReady

	var pid peer.ID
	spt.rpcClient.CallContext(
		ctx,
		"",
		"Cluster",
		"IPFSID",
		struct{}{},
		&pid,
	)
	return pid
}

// receives a pin Function (pin or unpin) and channels.  Used for both pinning
// and unpinning.
func (spt *Tracker) opWorker(pinF func(*optracker.Operation) error, prioCh, normalCh chan *optracker.Operation) {

	var op *optracker.Operation

	for {
		// Process the priority channel first.
		select {
		case op = <-prioCh:
			goto APPLY_OP
		case <-spt.ctx.Done():
			return
		default:
		}

		// Then process things on the other channels.
		// Block if there are no things to process.
		select {
		case op = <-prioCh:
			goto APPLY_OP
		case op = <-normalCh:
			goto APPLY_OP
		case <-spt.ctx.Done():
			return
		}

		// apply operations that came from some channel
	APPLY_OP:
		if clean := applyPinF(pinF, op); clean {
			spt.optracker.Clean(op.Context(), op)
		}
	}
}

// applyPinF returns true if the operation can be considered "DONE".
func applyPinF(pinF func(*optracker.Operation) error, op *optracker.Operation) bool {
	if op.Cancelled() {
		// operation was cancelled. Move on.
		// This saves some time, but not 100% needed.
		return false
	}
	op.SetPhase(optracker.PhaseInProgress)
	op.IncAttempt()
	err := pinF(op) // call pin/unpin
	if err != nil {
		if op.Cancelled() {
			// there was an error because
			// we were cancelled. Move on.
			return false
		}
		op.SetError(err)
		op.Cancel()
		return false
	}
	op.SetPhase(optracker.PhaseDone)
	op.Cancel()
	return true // this tells the opWorker to clean the operation from the tracker.
}

func (spt *Tracker) pin(op *optracker.Operation) error {
	ctx, span := trace.StartSpan(op.Context(), "tracker/stateless/pin")
	defer span.End()

	logger.Debugf("issuing pin call for %s", op.Cid())
	err := spt.rpcClient.CallContext(
		ctx,
		"",
		"IPFSConnector",
		"Pin",
		op.Pin(),
		&struct{}{},
	)
	if err != nil {
		return err
	}
	return nil
}

func (spt *Tracker) unpin(op *optracker.Operation) error {
	ctx, span := trace.StartSpan(op.Context(), "tracker/stateless/unpin")
	defer span.End()

	logger.Debugf("issuing unpin call for %s", op.Cid())
	err := spt.rpcClient.CallContext(
		ctx,
		"",
		"IPFSConnector",
		"Unpin",
		op.Pin(),
		&struct{}{},
	)
	if err != nil {
		return err
	}
	return nil
}

// Enqueue puts a new operation on the queue, unless ongoing exists.
func (spt *Tracker) enqueue(ctx context.Context, c *api.Pin, typ optracker.OperationType) error {
	ctx, span := trace.StartSpan(ctx, "tracker/stateless/enqueue")
	defer span.End()

	logger.Debugf("entering enqueue: pin: %+v", c)
	op := spt.optracker.TrackNewOperation(ctx, c, typ, optracker.PhaseQueued)
	if op == nil {
		return nil // the operation exists and must be queued already.
	}

	var ch chan *optracker.Operation

	switch typ {
	case optracker.OperationPin:
		isPriorityPin := time.Now().Before(c.Timestamp.Add(spt.config.PriorityPinMaxAge)) &&
			op.AttemptCount() <= spt.config.PriorityPinMaxRetries
		op.SetPriorityPin(isPriorityPin)

		if isPriorityPin {
			ch = spt.priorityPinCh
		} else {
			ch = spt.pinCh
		}
	case optracker.OperationUnpin:
		ch = spt.unpinCh
	}

	select {
	case ch <- op:
	default:
		err := ErrFullQueue
		op.SetError(err)
		op.Cancel()
		logger.Error(err.Error())
		return err
	}
	return nil
}

// SetClient makes the StatelessPinTracker ready to perform RPC requests to
// other components.
func (spt *Tracker) SetClient(c *rpc.Client) {
	spt.rpcClient = c
	close(spt.rpcReady)
}

// Shutdown finishes the services provided by the StatelessPinTracker
// and cancels any active context.
func (spt *Tracker) Shutdown(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "tracker/stateless/Shutdown")
	_ = ctx
	defer span.End()

	spt.shutdownMu.Lock()
	defer spt.shutdownMu.Unlock()

	if spt.shutdown {
		logger.Debug("already shutdown")
		return nil
	}

	logger.Info("stopping StatelessPinTracker")
	spt.cancel()
	spt.wg.Wait()
	spt.shutdown = true
	return nil
}

// Track tells the StatelessPinTracker to start managing a Cid,
// possibly triggering Pin operations on the IPFS daemon.
func (spt *Tracker) Track(ctx context.Context, c *api.Pin) error {
	ctx, span := trace.StartSpan(ctx, "tracker/stateless/Track")
	defer span.End()

	logger.Debugf("tracking %s", c.Cid)

	// Sharded pins are never pinned. A sharded pin cannot turn into
	// something else or viceversa like it happens with Remote pins so
	// we just ignore them.
	if c.Type == api.MetaType {
		return nil
	}

	// Trigger unpin whenever something remote is tracked
	// Note, IPFSConn checks with pin/ls before triggering
	// pin/rm.
	if c.IsRemotePin(spt.peerID) {
		op := spt.optracker.TrackNewOperation(ctx, c, optracker.OperationRemote, optracker.PhaseInProgress)
		if op == nil {
			return nil // ongoing unpin
		}
		err := spt.unpin(op)
		op.Cancel()
		if err != nil {
			op.SetError(err)
			return nil
		}

		op.SetPhase(optracker.PhaseDone)
		spt.optracker.Clean(ctx, op)
		return nil
	}

	return spt.enqueue(ctx, c, optracker.OperationPin)
}

// Untrack tells the StatelessPinTracker to stop managing a Cid.
// If the Cid is pinned locally, it will be unpinned.
func (spt *Tracker) Untrack(ctx context.Context, c cid.Cid) error {
	ctx, span := trace.StartSpan(ctx, "tracker/stateless/Untrack")
	defer span.End()

	logger.Debugf("untracking %s", c)
	return spt.enqueue(ctx, api.PinCid(c), optracker.OperationUnpin)
}

// StatusAll returns information for all Cids pinned to the local IPFS node.
func (spt *Tracker) StatusAll(ctx context.Context, filter api.TrackerStatus) []*api.PinInfo {
	ctx, span := trace.StartSpan(ctx, "tracker/stateless/StatusAll")
	defer span.End()

	pininfos, err := spt.localStatus(ctx, true, filter)
	if err != nil {
		return nil
	}

	// get all inflight operations from optracker and put them into the
	// map, deduplicating any existing items with their inflight operation.
	//
	// we cannot filter in GetAll, because we are meant to replace items in
	// pininfos and set the correct status, as otherwise they will remain in
	// PinError.
	ipfsid := spt.getIPFSID(ctx)
	for _, infop := range spt.optracker.GetAll(ctx) {
		infop.IPFS = ipfsid
		pininfos[infop.Cid] = infop
	}

	var pis []*api.PinInfo
	for _, pi := range pininfos {
		// Last filter.
		if pi.Status.Match(filter) {
			pis = append(pis, pi)
		}
	}
	return pis
}

// Status returns information for a Cid pinned to the local IPFS node.
func (spt *Tracker) Status(ctx context.Context, c cid.Cid) *api.PinInfo {
	ctx, span := trace.StartSpan(ctx, "tracker/stateless/Status")
	defer span.End()

	// check if c has an inflight operation or errorred operation in optracker
	if oppi, ok := spt.optracker.GetExists(ctx, c); ok {
		// if it does return the status of the operation
		oppi.IPFS = spt.getIPFSID(ctx)
		return oppi
	}

	pinInfo := &api.PinInfo{
		Cid:  c,
		Peer: spt.peerID,
		Name: "", // etc to be filled later
		PinInfoShort: api.PinInfoShort{
			PeerName:     spt.peerName,
			IPFS:         spt.getIPFSID(ctx),
			TS:           time.Now(),
			AttemptCount: 0,
			PriorityPin:  false,
		},
	}

	// check global state to see if cluster should even be caring about
	// the provided cid
	var gpin *api.Pin
	st, err := spt.getState(ctx)
	if err != nil {
		logger.Error(err)
		addError(pinInfo, err)
		return pinInfo
	}

	gpin, err = st.Get(ctx, c)
	if err == state.ErrNotFound {
		pinInfo.Status = api.TrackerStatusUnpinned
		return pinInfo
	}
	if err != nil {
		logger.Error(err)
		addError(pinInfo, err)
		return pinInfo
	}
	// The pin IS in the state.
	pinInfo.Name = gpin.Name
	pinInfo.TS = gpin.Timestamp
	pinInfo.Allocations = gpin.Allocations
	pinInfo.Origins = gpin.Origins
	pinInfo.Metadata = gpin.Metadata

	// check if pin is a meta pin
	if gpin.Type == api.MetaType {
		pinInfo.Status = api.TrackerStatusSharded
		return pinInfo
	}

	// check if pin is a remote pin
	if gpin.IsRemotePin(spt.peerID) {
		pinInfo.Status = api.TrackerStatusRemote
		return pinInfo
	}

	// else attempt to get status from ipfs node
	var ips api.IPFSPinStatus
	err = spt.rpcClient.CallContext(
		ctx,
		"",
		"IPFSConnector",
		"PinLsCid",
		gpin,
		&ips,
	)
	if err != nil {
		logger.Error(err)
		addError(pinInfo, err)
		return pinInfo
	}

	ipfsStatus := ips.ToTrackerStatus()
	switch ipfsStatus {
	case api.TrackerStatusUnpinned:
		// The item is in the state but not in IPFS:
		// PinError. Should be pinned.
		pinInfo.Status = api.TrackerStatusUnexpectedlyUnpinned
		pinInfo.Error = errUnexpectedlyUnpinned.Error()
	default:
		pinInfo.Status = ipfsStatus
	}
	return pinInfo
}

// RecoverAll attempts to recover all items tracked by this peer. It returns
// items that have been re-queued.
func (spt *Tracker) RecoverAll(ctx context.Context) ([]*api.PinInfo, error) {
	ctx, span := trace.StartSpan(ctx, "tracker/stateless/RecoverAll")
	defer span.End()

	statuses := spt.StatusAll(ctx, api.TrackerStatusUndefined)
	resp := make([]*api.PinInfo, 0)
	for _, st := range statuses {
		// Break out if we shutdown. We might be going through
		// a very long list of statuses.
		select {
		case <-spt.ctx.Done():
			return nil, spt.ctx.Err()
		default:
			r, err := spt.recoverWithPinInfo(ctx, st)
			if err != nil {
				return resp, err
			}
			if r != nil {
				resp = append(resp, r)
			}
		}
	}
	return resp, nil
}

// Recover will trigger pinning or unpinning for items in
// PinError or UnpinError states.
func (spt *Tracker) Recover(ctx context.Context, c cid.Cid) (*api.PinInfo, error) {
	ctx, span := trace.StartSpan(ctx, "tracker/stateless/Recover")
	defer span.End()

	// Check if we have a status in the operation tracker and use that
	// pininfo. Otherwise, get a status by checking against IPFS and use
	// that.
	pi, ok := spt.optracker.GetExists(ctx, c)
	if !ok {
		pi = spt.Status(ctx, c)
	}

	recPi, err := spt.recoverWithPinInfo(ctx, pi)
	// if it was not enqueued, no updated pin-info is returned.
	// Use the one we had.
	if recPi == nil {
		recPi = pi
	}
	return recPi, err
}

func (spt *Tracker) recoverWithPinInfo(ctx context.Context, pi *api.PinInfo) (*api.PinInfo, error) {
	var err error
	switch pi.Status {
	case api.TrackerStatusPinError, api.TrackerStatusUnexpectedlyUnpinned:
		logger.Infof("Restarting pin operation for %s", pi.Cid)
		err = spt.enqueue(ctx, api.PinCid(pi.Cid), optracker.OperationPin)
	case api.TrackerStatusUnpinError:
		logger.Infof("Restarting unpin operation for %s", pi.Cid)
		err = spt.enqueue(ctx, api.PinCid(pi.Cid), optracker.OperationUnpin)
	default:
		// We do not return any information when recover was a no-op
		return nil, nil
	}
	if err != nil {
		return spt.Status(ctx, pi.Cid), err
	}

	// This status call should be cheap as it would normally come from the
	// optracker and does not need to hit ipfs.
	return spt.Status(ctx, pi.Cid), nil
}

func (spt *Tracker) ipfsStatusAll(ctx context.Context) (map[cid.Cid]*api.PinInfo, error) {
	ctx, span := trace.StartSpan(ctx, "tracker/stateless/ipfsStatusAll")
	defer span.End()

	var ipsMap map[string]api.IPFSPinStatus
	err := spt.rpcClient.CallContext(
		ctx,
		"",
		"IPFSConnector",
		"PinLs",
		"recursive",
		&ipsMap,
	)
	if err != nil {
		logger.Error(err)
		return nil, err
	}
	ipfsid := spt.getIPFSID(ctx)
	pins := make(map[cid.Cid]*api.PinInfo, len(ipsMap))
	for cidstr, ips := range ipsMap {
		c, err := cid.Decode(cidstr)
		if err != nil {
			logger.Error(err)
			continue
		}
		p := &api.PinInfo{
			Cid:         c,
			Name:        "",  // to be filled later
			Allocations: nil, // to be filled later
			Origins:     nil, // to be filled later
			Metadata:    nil, // to be filled later
			Peer:        spt.peerID,
			PinInfoShort: api.PinInfoShort{
				PeerName:     spt.peerName,
				IPFS:         ipfsid,
				Status:       ips.ToTrackerStatus(),
				TS:           time.Now(), // to be set later
				AttemptCount: 0,
				PriorityPin:  false,
			},
		}
		pins[c] = p
	}
	return pins, nil
}

// localStatus returns a joint set of consensusState and ipfsStatus marking
// pins which should be meta or remote and leaving any ipfs pins that aren't
// in the consensusState out. If incExtra is true, Remote and Sharded pins
// will be added to the status slice. If a filter is provided, only statuses
// matching the filter will be returned.
func (spt *Tracker) localStatus(ctx context.Context, incExtra bool, filter api.TrackerStatus) (map[cid.Cid]*api.PinInfo, error) {
	ctx, span := trace.StartSpan(ctx, "tracker/stateless/localStatus")
	defer span.End()

	var statePins []*api.Pin

	// get shared state
	st, err := spt.getState(ctx)
	if err != nil {
		logger.Error(err)
		return nil, err
	}

	// Only list the full pinset if we are interested in pin types that
	// require it. Otherwise said, this whole method is mostly a no-op
	// when filtering for queued/error items which are all in the operation
	// tracker.
	if filter.Match(
		api.TrackerStatusPinned | api.TrackerStatusUnexpectedlyUnpinned |
			api.TrackerStatusSharded | api.TrackerStatusRemote) {
		statePins, err = st.List(ctx)
		if err != nil {
			logger.Error(err)
			return nil, err
		}
	}

	var localpis map[cid.Cid]*api.PinInfo
	// Only query IPFS if we want to status for pinned items
	if filter.Match(api.TrackerStatusPinned | api.TrackerStatusUnexpectedlyUnpinned) {
		localpis, err = spt.ipfsStatusAll(ctx)
		if err != nil {
			logger.Error(err)
			return nil, err
		}
	}

	pininfos := make(map[cid.Cid]*api.PinInfo, len(statePins))
	ipfsid := spt.getIPFSID(ctx)
	for _, p := range statePins {
		ipfsInfo, pinnedInIpfs := localpis[p.Cid]
		// base pinInfo object - status to be filled.
		pinInfo := api.PinInfo{
			Cid:         p.Cid,
			Name:        p.Name,
			Peer:        spt.peerID,
			Allocations: p.Allocations,
			Origins:     p.Origins,
			Metadata:    p.Metadata,
			PinInfoShort: api.PinInfoShort{
				PeerName:     spt.peerName,
				IPFS:         ipfsid,
				TS:           p.Timestamp,
				AttemptCount: 0,
				PriorityPin:  false,
			},
		}

		switch {
		case p.Type == api.MetaType:
			if !incExtra || !filter.Match(api.TrackerStatusSharded) {
				continue
			}
			pinInfo.Status = api.TrackerStatusSharded
			pininfos[p.Cid] = &pinInfo
		case p.IsRemotePin(spt.peerID):
			if !incExtra || !filter.Match(api.TrackerStatusRemote) {
				continue
			}
			pinInfo.Status = api.TrackerStatusRemote
			pininfos[p.Cid] = &pinInfo
		case pinnedInIpfs: // always false unless filter matches TrackerStatusPinnned
			ipfsInfo.Name = p.Name
			ipfsInfo.TS = p.Timestamp
			ipfsInfo.Allocations = p.Allocations
			ipfsInfo.Origins = p.Origins
			ipfsInfo.Metadata = p.Metadata
			pininfos[p.Cid] = ipfsInfo
		default:
			// report as UNEXPECTEDLY_UNPINNED for this peer.
			// this will be overwritten if the operation tracker
			// has more info for this (an ongoing pinning
			// operation). Otherwise, it means something should be
			// pinned and it is not known by IPFS. Should be
			// handled to "recover".

			pinInfo.Status = api.TrackerStatusUnexpectedlyUnpinned
			pinInfo.Error = errUnexpectedlyUnpinned.Error()
			pininfos[p.Cid] = &pinInfo
		}
	}
	return pininfos, nil
}

// func (spt *Tracker) getErrorsAll(ctx context.Context) []*api.PinInfo {
// 	return spt.optracker.Filter(ctx, optracker.PhaseError)
// }

// OpContext exports the internal optracker's OpContext method.
// For testing purposes only.
func (spt *Tracker) OpContext(ctx context.Context, c cid.Cid) context.Context {
	return spt.optracker.OpContext(ctx, c)
}

func addError(pinInfo *api.PinInfo, err error) {
	pinInfo.Error = err.Error()
	pinInfo.Status = api.TrackerStatusClusterError
}
