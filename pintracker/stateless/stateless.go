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
	logging "github.com/ipfs/go-log"
	peer "github.com/libp2p/go-libp2p-core/peer"
	rpc "github.com/libp2p/go-libp2p-gorpc"

	"go.opencensus.io/trace"
)

var logger = logging.Logger("pintracker")

var (
	// ErrNoChannel indicates that there doesn't exist a channel for given operation.
	ErrNoChannel = errors.New("operation doesn't have an associated channel")
	// ErrFullQueue is the error used when pin or unpin operation channel is full.
	ErrFullQueue = errors.New("pin/unpin operation queue is full (too many operations), increasing max_pin_queue_size would help")
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

	state state.ReadOnly

	rpcClient *rpc.Client
	rpcReady  chan struct{}

	pinCh   chan *optracker.Operation
	unpinCh chan *optracker.Operation

	shutdownMu sync.Mutex
	shutdown   bool
	wg         sync.WaitGroup
}

// New creates a new StatelessPinTracker.
func New(cfg *Config, pid peer.ID, peerName string) *Tracker {
	ctx, cancel := context.WithCancel(context.Background())

	spt := &Tracker{
		config:    cfg,
		peerID:    pid,
		peerName:  peerName,
		ctx:       ctx,
		cancel:    cancel,
		state:     nil,
		optracker: optracker.NewOperationTracker(ctx, pid, peerName),
		rpcReady:  make(chan struct{}, 1),
		pinCh:     make(chan *optracker.Operation, cfg.MaxPinQueueSize),
		unpinCh:   make(chan *optracker.Operation, cfg.MaxPinQueueSize),
	}

	for i := 0; i < spt.config.ConcurrentPins; i++ {
		go spt.opWorker(spt.pin, spt.pinCh)
	}
	go spt.opWorker(spt.unpin, spt.unpinCh)
	return spt
}

// receives a pin Function (pin or unpin) and a channel.
// Used for both pinning and unpinning
func (spt *Tracker) opWorker(pinF func(*optracker.Operation) error, opChan chan *optracker.Operation) {
	logger.Debug("entering opworker")
	// stop tracking operation shard and remove this ticker
	ticker := time.NewTicker(10 * time.Second) //TODO(ajl): make config var
	for {
		select {
		case <-ticker.C:
			// every tick, clear out all Done operations
			spt.optracker.CleanAllDone(spt.ctx)
		case op := <-opChan:
			if cont := applyPinF(pinF, op); cont {
				continue
			}

			spt.optracker.Clean(op.Context(), op)
		case <-spt.ctx.Done():
			return
		}
	}
}

// applyPinF returns true if caller should call `continue` inside calling loop.
func applyPinF(pinF func(*optracker.Operation) error, op *optracker.Operation) bool {
	if op.Cancelled() {
		// operation was cancelled. Move on.
		// This saves some time, but not 100% needed.
		return true
	}
	op.SetPhase(optracker.PhaseInProgress)
	err := pinF(op) // call pin/unpin
	if err != nil {
		if op.Cancelled() {
			// there was an error because
			// we were cancelled. Move on.
			return true
		}
		op.SetError(err)
		op.Cancel()
		return true
	}
	op.SetPhase(optracker.PhaseDone)
	op.Cancel()
	return false
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
		return nil // ongoing operation.
	}

	var ch chan *optracker.Operation

	switch typ {
	case optracker.OperationPin:
		ch = spt.pinCh
	case optracker.OperationUnpin:
		ch = spt.unpinCh
	default:
		return ErrNoChannel
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
	spt.rpcReady <- struct{}{}
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
	close(spt.rpcReady)
	spt.wg.Wait()
	spt.shutdown = true
	return nil
}

// SetState sets the pin state and returns true if state was not empty.
func (spt *Tracker) SetState(st state.ReadOnly) bool {
	spt.state = st

	return !state.IsEmpty(st)
}

// Track tells the StatelessPinTracker to start managing a Cid,
// possibly triggering Pin operations on the IPFS daemon.
func (spt *Tracker) Track(ctx context.Context, c *api.Pin) error {
	ctx, span := trace.StartSpan(ctx, "tracker/stateless/Track")
	defer span.End()

	logger.Debugf("tracking %s", c.Cid)

	// Sharded pins are never pinned, so do nothing.
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
		if err != nil {
			op.SetError(err)
			op.Cancel()
			return nil
		}

		op.SetPhase(optracker.PhaseDone)
		op.Cancel()
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
func (spt *Tracker) StatusAll(ctx context.Context) []*api.PinInfo {
	ctx, span := trace.StartSpan(ctx, "tracker/stateless/StatusAll")
	defer span.End()

	pininfos, err := spt.localStatus(ctx, true)
	if err != nil {
		logger.Error(err)
		return nil
	}

	// get all inflight operations from optracker and
	// put them into the map, deduplicating any already 'pinned' items with
	// their inflight operation
	for _, infop := range spt.optracker.GetAll(ctx) {
		pininfos[infop.Cid.String()] = infop
	}

	var pis []*api.PinInfo
	for _, pi := range pininfos {
		pis = append(pis, pi)
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
		return oppi
	}

	pinInfo := &api.PinInfo{
		Cid:      c,
		Peer:     spt.peerID,
		PeerName: spt.peerName,
		TS:       time.Now(),
	}

	// check global state to see if cluster should even be caring about
	// the provided cid
	var gpin *api.Pin
	var err error
	if state.IsEmpty(spt.state) {
		err = spt.rpcClient.Call(
			"",
			"Cluster",
			"PinGet",
			c,
			gpin,
		)
		if err != nil {
			if rpc.IsRPCError(err) {
				logger.Error(err)
				pinInfo.Status = api.TrackerStatusClusterError
				pinInfo.Error = err.Error()
				return pinInfo
			}
			// not part of global state. we should not care about
			pinInfo.Status = api.TrackerStatusUnpinned
			return pinInfo
		}
	} else {
		gpin, err = spt.state.Get(ctx, c)
		if err == state.ErrNotFound {
			pinInfo.Status = api.TrackerStatusUnpinned
			return pinInfo
		}
		if err != nil {
			logger.Error(err)
			return nil
		}
	}

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
	err = spt.rpcClient.Call(
		"",
		"IPFSConnector",
		"PinLsCid",
		c,
		&ips,
	)
	if err != nil {
		logger.Error(err)
		return nil
	}

	pinInfo.Status = ips.ToTrackerStatus()
	pinInfo.TS = time.Now()
	return pinInfo
}

// RecoverAll attempts to recover all items tracked by this peer.
func (spt *Tracker) RecoverAll(ctx context.Context) ([]*api.PinInfo, error) {
	ctx, span := trace.StartSpan(ctx, "tracker/stateless/RecoverAll")
	defer span.End()

	statuses := spt.StatusAll(ctx)
	resp := make([]*api.PinInfo, 0)
	for _, st := range statuses {
		r, err := spt.Recover(ctx, st.Cid)
		if err != nil {
			return resp, err
		}
		resp = append(resp, r)
	}
	return resp, nil
}

// Recover will re-track or re-untrack a Cid in error state,
// possibly retriggering an IPFS pinning operation and returning
// only when it is done.
func (spt *Tracker) Recover(ctx context.Context, c cid.Cid) (*api.PinInfo, error) {
	ctx, span := trace.StartSpan(ctx, "tracker/stateless/Recover")
	defer span.End()

	pInfo, ok := spt.optracker.GetExists(ctx, c)
	if !ok {
		return spt.Status(ctx, c), nil
	}

	var err error
	switch pInfo.Status {
	case api.TrackerStatusPinError:
		logger.Infof("Restarting pin operation for %s", c)
		err = spt.enqueue(ctx, api.PinCid(c), optracker.OperationPin)
	case api.TrackerStatusUnpinError:
		logger.Infof("Restarting unpin operation for %s", c)
		err = spt.enqueue(ctx, api.PinCid(c), optracker.OperationUnpin)
	}
	if err != nil {
		return spt.Status(ctx, c), err
	}

	return spt.Status(ctx, c), nil
}

func (spt *Tracker) ipfsStatusAll(ctx context.Context) (map[string]*api.PinInfo, error) {
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
	pins := make(map[string]*api.PinInfo, 0)
	for cidstr, ips := range ipsMap {
		c, err := cid.Decode(cidstr)
		if err != nil {
			logger.Error(err)
			continue
		}
		p := &api.PinInfo{
			Cid:      c,
			Peer:     spt.peerID,
			PeerName: spt.peerName,
			Status:   ips.ToTrackerStatus(),
			TS:       time.Now(),
		}
		pins[cidstr] = p
	}
	return pins, nil
}

// localStatus returns a joint set of consensusState and ipfsStatus
// marking pins which should be meta or remote and leaving any ipfs pins that
// aren't in the consensusState out. If incExtra is true, Remote and Sharded
// pins will be added to the status slice.
func (spt *Tracker) localStatus(ctx context.Context, incExtra bool) (map[string]*api.PinInfo, error) {
	ctx, span := trace.StartSpan(ctx, "tracker/stateless/localStatus")
	defer span.End()

	// get shared state
	var statePins []*api.Pin
	var err error
	if spt.state == nil || state.IsEmpty(spt.state) {
		err = spt.rpcClient.CallContext(
			ctx,
			"",
			"Cluster",
			"Pins",
			struct{}{},
			&statePins,
		)
	} else {
		statePins, err = spt.state.List(ctx)
	}
	if err != nil {
		logger.Error(err)
		return nil, err
	}

	// get statuses from ipfs node first
	localpis, err := spt.ipfsStatusAll(ctx)
	if err != nil {
		logger.Error(err)
		return nil, err
	}

	pininfos := make(map[string]*api.PinInfo)
	for _, p := range statePins {
		pCid := p.Cid.String()
		pinInfo := &api.PinInfo{
			Cid:      p.Cid,
			Peer:     spt.peerID,
			PeerName: spt.peerName,
			TS:       time.Now(),
		}

		if p.Type == api.MetaType && incExtra {
			// add pin to pininfos with sharded status
			pinInfo.Status = api.TrackerStatusSharded
			pininfos[pCid] = pinInfo
			continue
		}

		if p.IsRemotePin(spt.peerID) && incExtra {
			// add pin to pininfos with a status of remote
			pinInfo.Status = api.TrackerStatusRemote
			pininfos[pCid] = pinInfo
			continue
		}
		// lookup p in localpis
		if lp, ok := localpis[pCid]; ok {
			pininfos[pCid] = lp
		}
	}
	return pininfos, nil
}

func (spt *Tracker) getErrorsAll(ctx context.Context) []*api.PinInfo {
	return spt.optracker.Filter(ctx, optracker.PhaseError)
}

// OpContext exports the internal optracker's OpContext method.
// For testing purposes only.
func (spt *Tracker) OpContext(ctx context.Context, c cid.Cid) context.Context {
	return spt.optracker.OpContext(ctx, c)
}
