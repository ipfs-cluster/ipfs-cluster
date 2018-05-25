package stateless

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/optracker"
	"github.com/ipfs/ipfs-cluster/pintracker/util"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	cid "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	peer "github.com/libp2p/go-libp2p-peer"
)

var logger = logging.Logger("pintracker")

// Tracker uses the optracker.OperationTracker to manage
// transitioning shared ipfs-cluster state (Pins) to the local IPFS node.
type Tracker struct {
	config *Config

	optracker *optracker.OperationTracker

	opErrsMu sync.RWMutex
	opErrs   map[string]api.PinInfo

	remotesMu sync.RWMutex
	remotes   map[string]api.PinInfo

	peerID peer.ID

	ctx    context.Context
	cancel func()

	rpcClient *rpc.Client
	rpcReady  chan struct{}

	pinCh   chan api.Pin
	unpinCh chan api.Pin

	shutdownMu sync.Mutex
	shutdown   bool
	wg         sync.WaitGroup
}

// New creates a new StatelessPinTracker.
func New(cfg *Config, pid peer.ID) *Tracker {
	ctx, cancel := context.WithCancel(context.Background())

	spt := &Tracker{
		config:    cfg,
		peerID:    pid,
		ctx:       ctx,
		cancel:    cancel,
		optracker: optracker.NewOperationTracker(ctx),
		opErrs:    make(map[string]api.PinInfo),
		remotes:   make(map[string]api.PinInfo),
		rpcReady:  make(chan struct{}, 1),
		pinCh:     make(chan api.Pin, cfg.MaxPinQueueSize),
		unpinCh:   make(chan api.Pin, cfg.MaxPinQueueSize),
	}

	for i := 0; i < spt.config.ConcurrentPins; i++ {
		go spt.pinWorker()
	}
	go spt.unpinWorker()

	return spt
}

// reads the queue and makes pins to the IPFS daemon one by one
func (spt *Tracker) pinWorker() {
	for {
		select {
		case p := <-spt.pinCh:
			if opc, ok := spt.optracker.Get(p.Cid); ok && opc.Op == optracker.OperationPin {
				spt.optracker.UpdateOperationPhase(p.Cid, optracker.PhaseInProgress)
				spt.pin(p)
			}
		case <-spt.ctx.Done():
			return
		}
	}
}

// reads the queue and makes unpin requests to the IPFS daemon
func (spt *Tracker) unpinWorker() {
	for {
		select {
		case p := <-spt.unpinCh:
			if opc, ok := spt.optracker.Get(p.Cid); ok && opc.Op == optracker.OperationUnpin {
				spt.optracker.UpdateOperationPhase(p.Cid, optracker.PhaseInProgress)
				spt.unpin(p)
			}
		case <-spt.ctx.Done():
			return
		}
	}
}

// SetClient makes the StatelessPinTracker ready to perform RPC requests to
// other components.
func (spt *Tracker) SetClient(c *rpc.Client) {
	spt.rpcClient = c
	spt.rpcReady <- struct{}{}
}

// Shutdown finishes the services provided by the StatelessPinTracker
// and cancels any active context.
func (spt *Tracker) Shutdown() error {
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

// Track tells the StatelessPinTracker to start managing a Cid,
// possibly triggering Pin operations on the IPFS daemon.
func (spt *Tracker) Track(c api.Pin) error {
	logger.Debugf("tracking %s", c.Cid)
	if opc, ok := spt.optracker.Get(c.Cid); ok {
		if opc.Op == optracker.OperationUnpin {
			switch opc.Phase {
			case optracker.PhaseQueued:
				spt.optracker.Finish(c.Cid)
				return nil
			case optracker.PhaseInProgress:
				spt.optracker.Finish(c.Cid)
				// NOTE: this may leave the api.PinInfo in an error state
				// so a pin operation needs to be run on it (same as Recover)
			}
		}
	}

	if util.IsRemotePin(c, spt.peerID) {
		if spt.Status(c.Cid).Status == api.TrackerStatusPinned {
			spt.optracker.TrackNewOperation(
				spt.ctx,
				c.Cid,
				optracker.OperationUnpin,
			)
			spt.unpin(c)
		}
		spt.setRemote(c.Cid)
		return nil
	}
	spt.removeRemote(c.Cid)

	spt.optracker.TrackNewOperation(spt.ctx, c.Cid, optracker.OperationPin)

	select {
	case spt.pinCh <- c:
	default:
		err := errors.New("pin queue is full")
		spt.setError(c.Cid, err)
		logger.Error(err.Error())
		return err
	}
	return nil
}

// Untrack tells the StatelessPinTracker to stop managing a Cid.
// If the Cid is pinned locally, it will be unpinned.
func (spt *Tracker) Untrack(c *cid.Cid) error {
	logger.Debugf("untracking %s", c)
	if opc, ok := spt.optracker.Get(c); ok {
		if opc.Op == optracker.OperationPin {
			spt.optracker.Finish(c) // cancel it

			switch opc.Phase {
			case optracker.PhaseQueued:
				return nil
			case optracker.PhaseInProgress:
				// continues below to run a full unpin
			}
		}
	}

	spt.optracker.TrackNewOperation(spt.ctx, c, optracker.OperationUnpin)

	select {
	case spt.unpinCh <- api.PinCid(c):
	default:
		err := errors.New("unpin queue is full")
		spt.setError(c, err)
		logger.Error(err.Error())
		return err
	}
	return nil
}

// StatusAll returns information for all Cids pinned to the local IPFS node.
func (spt *Tracker) StatusAll() []api.PinInfo {
	// get statuses from ipfs node first
	localpis, _ := spt.ipfsStatusAll()
	// get all inflight operations from optracker
	infops := spt.optracker.GetAll()
	// put them into the map, deduplicating any already 'pinned' items with
	// their inflight operation
	for _, infop := range infops {
		localpis[infop.Cid.String()] = infop.ToPinInfo(spt.peerID)
	}
	// get all remote pins
	for _, remote := range spt.getRemotesAll() {
		localpis[remote.Cid.String()] = remote
	}
	// get all err pins
	for _, err := range spt.getErrorsAll() {
		localpis[err.Cid.String()] = err
	}
	// convert to array
	var pis []api.PinInfo
	for _, pi := range localpis {
		logger.Debugf("statusall: pins: %v", pi)
		pis = append(pis, pi)
	}
	return pis
}

// Status returns information for a Cid pinned to the local IPFS node.
func (spt *Tracker) Status(c *cid.Cid) api.PinInfo {
	// check errors map
	if pi, ok := spt.getError(c); ok {
		return pi
	}
	// check remotes map
	if pi, ok := spt.getRemote(c); ok {
		return pi
	}
	// check if c has an inflight operation in optracker
	if op, ok := spt.optracker.Get(c); ok {
		// if it does return the status of the operation
		return op.ToPinInfo(spt.peerID)
	}
	// else attempt to get status from ipfs node
	//TODO(ajl): log error
	pi, _ := spt.ipfsStatus(c)
	return pi
}

// SyncAll verifies that the statuses of all tracked Cids (from the shared state)
// match the one reported by the IPFS daemon. If not, they will be transitioned
// to PinError or UnpinError.
//
// SyncAll returns the list of local status for all tracked Cids which
// were updated or have errors. Cids in error states can be recovered
// with Recover().
// An error is returned if we are unable to contact the IPFS daemon.
func (spt *Tracker) SyncAll() ([]api.PinInfo, error) {
	pis := spt.getErrorsAll()
	return pis, nil
}

// Sync verifies that the status of a Cid in shared state, matches that of
// the IPFS daemon. If not, it will be transitioned
// to PinError or UnpinError.
//
// Sync returns the updated local status for the given Cid.
// Pins in error states can be recovered with Recover().
// An error is returned if we are unable to contact
// the IPFS daemon.
func (spt *Tracker) Sync(c *cid.Cid) (api.PinInfo, error) {
	// no-op in stateless tracker implementation
	return spt.Status(c), nil
}

// RecoverAll attempts to recover all items tracked by this peer.
func (spt *Tracker) RecoverAll() ([]api.PinInfo, error) {
	statuses := spt.StatusAll()
	resp := make([]api.PinInfo, 0)
	for _, st := range statuses {
		r, err := spt.Recover(st.Cid)
		if err != nil {
			return resp, err
		}
		resp = append(resp, r)
	}
	return resp, nil
}

// Recover will re-track or re-untrack a Cid in error state,
// possibly retriggering an IPFS pinning operation and returning
// only when it is done. The pinning/unpinning operation happens
// synchronously, jumping the queues.
func (spt *Tracker) Recover(c *cid.Cid) (api.PinInfo, error) {
	logger.Debugf("attempting to recover cid: %s", c)
	pi, ok := spt.getError(c)
	if !ok {
		return spt.Status(c), nil
	}
	var err error
	switch pi.Status {
	case api.TrackerStatusPinError:
		spt.optracker.RemoveErroredOperations(c)
		err = spt.pin(api.PinCid(c))
	case api.TrackerStatusUnpinError:
		spt.optracker.RemoveErroredOperations(c)
		err = spt.unpin(api.PinCid(c))
	default:
		logger.Warningf("%s does not need recovery. Try syncing first", c)
		return pi, nil
	}
	if err != nil {
		logger.Errorf("error recovering %s: %s", c, err)
		return pi, err
	}
	spt.removeError(c)
	return spt.Status(c), nil
}

func (spt *Tracker) pin(c api.Pin) error {
	logger.Debugf("issuing pin call for %s", c.Cid)

	var ctx context.Context
	opc, ok := spt.optracker.Get(c.Cid)
	if !ok {
		logger.Debug("pin operation wasn't being tracked")
		ctx = spt.ctx
	} else {
		ctx = opc.Ctx
	}

	err := spt.rpcClient.CallContext(
		ctx,
		"",
		"Cluster",
		"IPFSPin",
		c.ToSerial(),
		&struct{}{},
	)
	if err != nil {
		logger.Debugf("failed to pin: %s", c.Cid)
		spt.setError(c.Cid, err)
		return err
	}

	spt.optracker.Finish(c.Cid)
	return nil
}

func (spt *Tracker) unpin(c api.Pin) error {
	logger.Debugf("issuing unpin call for %s", c.Cid)

	var ctx context.Context
	opc, ok := spt.optracker.Get(c.Cid)
	if !ok {
		logger.Debug("pin operation wasn't being tracked")
		ctx = spt.ctx
	} else {
		ctx = opc.Ctx
	}

	err := spt.rpcClient.CallContext(
		ctx,
		"",
		"Cluster",
		"IPFSUnpin",
		c.ToSerial(),
		&struct{}{},
	)
	if err != nil {
		spt.setError(c.Cid, err)
		return err
	}

	spt.optracker.Finish(c.Cid)
	return nil
}

func (spt *Tracker) ipfsStatus(c *cid.Cid) (api.PinInfo, error) {
	var ips api.IPFSPinStatus
	err := spt.rpcClient.Call(
		"",
		"Cluster",
		"IPFSPinLsCid",
		api.PinCid(c).ToSerial(),
		&ips,
	)
	if err != nil {
		return api.PinInfo{}, err
	}
	pi := api.PinInfo{
		Cid:    c,
		Peer:   spt.peerID,
		Status: ips.ToTrackerStatus(),
		//TODO(ajl): figure out a way to preserve time value, represents when the cid was pinned
	}
	return pi, nil
}

func (spt *Tracker) ipfsStatusAll() (map[string]api.PinInfo, error) {
	var ipsMap map[string]api.IPFSPinStatus
	err := spt.rpcClient.Call(
		"",
		"Cluster",
		"IPFSPinLs",
		"recursive",
		&ipsMap,
	)
	if err != nil {
		logger.Error(err)
		return nil, err
	}
	pins := make(map[string]api.PinInfo, 0)
	for cidstr, ips := range ipsMap {
		c, err := cid.Decode(cidstr)
		if err != nil {
			logger.Error(err)
			continue
		}
		p := api.PinInfo{
			Cid:    c,
			Peer:   spt.peerID,
			Status: ips.ToTrackerStatus(),
		}
		pins[cidstr] = p
	}
	return pins, nil
}

func (spt *Tracker) getError(c *cid.Cid) (api.PinInfo, bool) {
	spt.opErrsMu.RLock()
	defer spt.opErrsMu.RUnlock()
	pi, ok := spt.opErrs[c.String()]
	return pi, ok
}

func (spt *Tracker) getErrorsAll() []api.PinInfo {
	spt.opErrsMu.RLock()
	defer spt.opErrsMu.RUnlock()
	var pis []api.PinInfo
	for _, pi := range spt.opErrs {
		pis = append(pis, pi)
	}
	return pis
}

// setError sets a Cid in error state
func (spt *Tracker) setError(c *cid.Cid, err error) {
	spt.opErrsMu.Lock()
	spt.unsafeSetError(c, err)
	spt.opErrsMu.Unlock()
}

func (spt *Tracker) unsafeSetError(c *cid.Cid, err error) {
	op := spt.optracker.SetError(c)
	spt.opErrs[c.String()] = api.PinInfo{
		Cid:    c,
		Peer:   spt.peerID,
		Status: op.ToTrackerStatus(),
		TS:     time.Now(),
		Error:  err.Error(),
	}
}

func (spt *Tracker) removeError(c *cid.Cid) {
	spt.opErrsMu.Lock()
	delete(spt.opErrs, c.String())
	spt.opErrsMu.Unlock()
}

func (spt *Tracker) getRemote(c *cid.Cid) (api.PinInfo, bool) {
	spt.remotesMu.RLock()
	defer spt.remotesMu.RUnlock()
	pi, ok := spt.remotes[c.String()]
	return pi, ok
}

func (spt *Tracker) getRemotesAll() []api.PinInfo {
	spt.remotesMu.RLock()
	defer spt.remotesMu.RUnlock()
	var pis []api.PinInfo
	for _, pi := range spt.remotes {
		pis = append(pis, pi)
	}
	return pis
}

// setRemote sets a Cid in error state
func (spt *Tracker) setRemote(c *cid.Cid) {
	spt.remotesMu.Lock()
	defer spt.remotesMu.Unlock()

	spt.remotes[c.String()] = api.PinInfo{
		Cid:    c,
		Peer:   spt.peerID,
		Status: api.TrackerStatusRemote,
		TS:     time.Now(),
	}
}

func (spt *Tracker) removeRemote(c *cid.Cid) {
	spt.remotesMu.Lock()
	delete(spt.remotes, c.String())
	spt.remotesMu.Unlock()
}
