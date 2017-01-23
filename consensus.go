package ipfscluster

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	rpc "github.com/hsanjuan/go-libp2p-rpc"
	cid "github.com/ipfs/go-cid"
	consensus "github.com/libp2p/go-libp2p-consensus"
	host "github.com/libp2p/go-libp2p-host"
	peer "github.com/libp2p/go-libp2p-peer"
	libp2praft "github.com/libp2p/go-libp2p-raft"
)

const (
	maxSnapshots   = 5
	raftSingleMode = true
)

// Type of pin operation
const (
	LogOpPin = iota + 1
	LogOpUnpin
)

// LeaderTimeout specifies how long to wait during initialization
// before failing for not having a leader.
var LeaderTimeout = 120 * time.Second

type clusterLogOpType int

// clusterLogOp represents an operation for the OpLogConsensus system.
// It implements the consensus.Op interface.
type clusterLogOp struct {
	Cid       string
	Type      clusterLogOpType
	ctx       context.Context
	rpcClient *rpc.Client
}

// ApplyTo applies the operation to the State
func (op *clusterLogOp) ApplyTo(cstate consensus.State) (consensus.State, error) {
	state, ok := cstate.(State)
	var err error
	if !ok {
		// Should never be here
		panic("received unexpected state type")
	}

	c, err := cid.Decode(op.Cid)
	if err != nil {
		// Should never be here
		panic("could not decode a CID we ourselves encoded")
	}

	switch op.Type {
	case LogOpPin:
		err := state.AddPin(c)
		if err != nil {
			goto ROLLBACK
		}
		// Async, we let the PinTracker take care of any problems
		op.rpcClient.Go("",
			"Cluster",
			"Track",
			NewCidArg(c),
			&struct{}{},
			nil)
	case LogOpUnpin:
		err := state.RmPin(c)
		if err != nil {
			goto ROLLBACK
		}
		// Async, we let the PinTracker take care of any problems
		op.rpcClient.Go("",
			"Cluster",
			"Untrack",
			NewCidArg(c),
			&struct{}{},
			nil)
	default:
		logger.Error("unknown clusterLogOp type. Ignoring")
	}
	return state, nil

ROLLBACK:
	// We failed to apply the operation to the state
	// and therefore we need to request a rollback to the
	// cluster to the previous state. This operation can only be performed
	// by the cluster leader.
	logger.Error("Rollbacks are not implemented")
	return nil, errors.New("a rollback may be necessary. Reason: " + err.Error())
}

// Consensus handles the work of keeping a shared-state between
// the members of an IPFS Cluster, as well as modifying that state and
// applying any updates in a thread-safe manner.
type Consensus struct {
	ctx context.Context

	cfg  *Config
	host host.Host

	consensus consensus.OpLogConsensus
	actor     consensus.Actor
	baseOp    *clusterLogOp
	p2pRaft   *libp2pRaftWrap

	rpcClient *rpc.Client
	rpcReady  chan struct{}

	shutdownLock sync.Mutex
	shutdown     bool
	shutdownCh   chan struct{}
	wg           sync.WaitGroup
}

// NewConsensus builds a new ClusterConsensus component. The state
// is used to initialize the Consensus system, so any information in it
// is discarded.
func NewConsensus(cfg *Config, host host.Host, state State) (*Consensus, error) {
	ctx := context.Background()
	op := &clusterLogOp{
		ctx: context.Background(),
	}

	cc := &Consensus{
		ctx:        ctx,
		cfg:        cfg,
		host:       host,
		baseOp:     op,
		shutdownCh: make(chan struct{}),
		rpcReady:   make(chan struct{}, 1),
	}

	logger.Info("starting Consensus component")
	logger.Infof("waiting %d seconds for leader", LeaderTimeout/time.Second)
	con, actor, wrapper, err := makeLibp2pRaft(cc.cfg,
		cc.host, state, cc.baseOp)
	if err != nil {
		panic(err)
	}
	con.SetActor(actor)
	cc.actor = actor
	cc.consensus = con
	cc.p2pRaft = wrapper

	// Wait for a leader
	start := time.Now()
	leader := peer.ID("")
	for time.Since(start) < LeaderTimeout {
		time.Sleep(500 * time.Millisecond)
		leader, err = cc.Leader()
		if err == nil {
			break
		}
	}
	if leader == "" {
		return nil, errors.New("no leader was found after timeout")
	}

	logger.Debugf("raft leader is %s", leader)
	cc.run(state)
	return cc, nil
}

func (cc *Consensus) run(state State) {
	cc.wg.Add(1)
	go func() {
		defer cc.wg.Done()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		cc.ctx = ctx
		cc.baseOp.ctx = ctx

		upToDate := make(chan struct{})
		go func() {
			logger.Info("consensus state is catching up")
			time.Sleep(time.Second)
			for {
				lai := cc.p2pRaft.raft.AppliedIndex()
				li := cc.p2pRaft.raft.LastIndex()
				logger.Debugf("current Raft index: %d/%d",
					lai, li)
				if lai == li {
					upToDate <- struct{}{}
					break
				}
				time.Sleep(500 * time.Millisecond)
			}
		}()

		<-upToDate
		logger.Info("consensus state is up to date")

		// While rpc is not ready we cannot perform a sync
		<-cc.rpcReady

		var pInfo []PinInfo
		cc.rpcClient.Go(
			"",
			"Cluster",
			"StateSync",
			struct{}{},
			&pInfo,
			nil)

		<-cc.shutdownCh
	}()
}

// Shutdown stops the component so it will not process any
// more updates. The underlying consensus is permanently
// shutdown, along with the libp2p transport.
func (cc *Consensus) Shutdown() error {
	cc.shutdownLock.Lock()
	defer cc.shutdownLock.Unlock()

	if cc.shutdown {
		logger.Debug("already shutdown")
		return nil
	}

	logger.Info("stopping Consensus component")

	close(cc.rpcReady)
	cc.shutdownCh <- struct{}{}

	// Raft shutdown
	errMsgs := ""

	f := cc.p2pRaft.raft.Snapshot()
	err := f.Error()
	if err != nil && !strings.Contains(err.Error(), "Nothing new to snapshot") {
		errMsgs += "could not take snapshot: " + err.Error() + ".\n"
	}
	f = cc.p2pRaft.raft.Shutdown()
	err = f.Error()
	if err != nil {
		errMsgs += "could not shutdown raft: " + err.Error() + ".\n"
	}
	// BUG(hector): See go-libp2p-raft#16
	// err = cc.p2pRaft.transport.Close()
	// if err != nil {
	// 	errMsgs += "could not close libp2p transport: " + err.Error() + ".\n"
	// }
	err = cc.p2pRaft.boltdb.Close() // important!
	if err != nil {
		errMsgs += "could not close boltdb: " + err.Error() + ".\n"
	}

	if errMsgs != "" {
		errMsgs += "Consensus shutdown unsucessful"
		logger.Error(errMsgs)
		return errors.New(errMsgs)
	}
	cc.wg.Wait()
	cc.shutdown = true
	return nil
}

// SetClient makes the component ready to perform RPC requets
func (cc *Consensus) SetClient(c *rpc.Client) {
	cc.rpcClient = c
	cc.baseOp.rpcClient = c
	cc.rpcReady <- struct{}{}
}

func (cc *Consensus) op(c *cid.Cid, t clusterLogOpType) *clusterLogOp {
	return &clusterLogOp{
		Cid:  c.String(),
		Type: t,
	}
}

// LogPin submits a Cid to the shared state of the cluster.
func (cc *Consensus) LogPin(c *cid.Cid) error {
	// Create pin operation for the log
	op := cc.op(c, LogOpPin)
	_, err := cc.consensus.CommitOp(op)
	if err != nil {
		// This means the op did not make it to the log
		return err
	}
	logger.Infof("pin commited to global state: %s", c)
	return nil
}

// LogUnpin removes a Cid from the shared state of the cluster.
func (cc *Consensus) LogUnpin(c *cid.Cid) error {
	// Create  unpin operation for the log
	op := cc.op(c, LogOpUnpin)
	_, err := cc.consensus.CommitOp(op)
	if err != nil {
		return err
	}
	logger.Infof("unpin commited to global state: %s", c)
	return nil
}

// State retrieves the current consensus State. It may error
// if no State has been agreed upon or the state is not
// consistent. The returned State is the last agreed-upon
// State known by this node.
func (cc *Consensus) State() (State, error) {
	st, err := cc.consensus.GetLogHead()
	if err != nil {
		return nil, err
	}
	state, ok := st.(State)
	if !ok {
		return nil, errors.New("wrong state type")
	}
	return state, nil
}

// Leader returns the peerID of the Leader of the
// cluster. It returns an error when there is no leader.
func (cc *Consensus) Leader() (peer.ID, error) {
	// FIXME: Hashicorp Raft specific
	raftactor := cc.actor.(*libp2praft.Actor)
	return raftactor.Leader()
}

// Rollback replaces the current agreed-upon
// state with the state provided. Only the consensus leader
// can perform this operation.
func (cc *Consensus) Rollback(state State) error {
	return cc.consensus.Rollback(state)
}
