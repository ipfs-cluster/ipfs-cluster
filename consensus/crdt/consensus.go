package crdt

import (
	"context"
	"errors"
	"sync"
	"time"

	ipfslite "github.com/hsanjuan/ipfs-lite"
	dshelp "github.com/ipfs/go-ipfs-ds-help"
	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/pstoremgr"
	"github.com/ipfs/ipfs-cluster/state"
	"github.com/ipfs/ipfs-cluster/state/dsstate"
	multihash "github.com/multiformats/go-multihash"
	"go.opencensus.io/trace"

	ds "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	query "github.com/ipfs/go-datastore/query"
	crdt "github.com/ipfs/go-ds-crdt"
	logging "github.com/ipfs/go-log"
	rpc "github.com/libp2p/go-libp2p-gorpc"
	host "github.com/libp2p/go-libp2p-host"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	peer "github.com/libp2p/go-libp2p-peer"
	peerstore "github.com/libp2p/go-libp2p-peerstore"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

var logger = logging.Logger("crdt")

var (
	blocksNs   = "b" // blockstore namespace
	connMgrTag = "crdt"
)

// Common variables for the module.
var (
	ErrNoLeader = errors.New("crdt consensus component does not provide a leader")
	ErrRmPeer   = errors.New("crdt consensus component cannot remove peers")
)

// Consensus implement ipfscluster.Consensus and provides the facility to add
// and remove pins from the Cluster shared state. It uses a CRDT-backed
// implementation of go-datastore (go-ds-crdt).
type Consensus struct {
	ctx    context.Context
	cancel context.CancelFunc

	config *Config

	trustedPeers sync.Map

	host        host.Host
	peerManager *pstoremgr.Manager

	store     ds.Datastore
	namespace ds.Key

	state state.State
	crdt  *crdt.Datastore

	dht    *dht.IpfsDHT
	pubsub *pubsub.PubSub

	rpcClient *rpc.Client
	rpcReady  chan struct{}
	readyCh   chan struct{}

	shutdownLock sync.RWMutex
	shutdown     bool
}

// New creates a new crdt Consensus component. The given PubSub will be used to
// broadcast new heads. The given thread-safe datastore will be used to persist
// data and all will be prefixed with cfg.DatastoreNamespace.
func New(
	host host.Host,
	dht *dht.IpfsDHT,
	pubsub *pubsub.PubSub,
	cfg *Config,
	store ds.Datastore,
) (*Consensus, error) {
	err := cfg.Validate()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	css := &Consensus{
		ctx:         ctx,
		cancel:      cancel,
		config:      cfg,
		host:        host,
		peerManager: pstoremgr.New(ctx, host, ""),
		dht:         dht,
		store:       store,
		namespace:   ds.NewKey(cfg.DatastoreNamespace),
		pubsub:      pubsub,
		rpcReady:    make(chan struct{}, 1),
		readyCh:     make(chan struct{}, 1),
	}

	go css.setup()
	return css, nil
}

func (css *Consensus) setup() {
	select {
	case <-css.ctx.Done():
		return
	case <-css.rpcReady:
	}

	// Set up a fast-lookup trusted peers cache.
	// Protect these peers in the ConnMgr
	for _, p := range css.config.TrustedPeers {
		css.Trust(css.ctx, p)
	}

	// Hash the cluster name and produce the topic name from there
	// as a way to avoid pubsub topic collisions with other
	// pubsub applications potentially when both potentially use
	// simple names like "test".
	topicName := css.config.ClusterName
	topicHash, err := multihash.Sum([]byte(css.config.ClusterName), multihash.MD5, -1)
	if err != nil {
		logger.Errorf("error hashing topic: %s", err)
	} else {
		topicName = topicHash.B58String()
	}

	// Validate pubsub messages for our topic (only accept
	// from trusted sources)
	err = css.pubsub.RegisterTopicValidator(
		topicName,
		func(ctx context.Context, p peer.ID, msg *pubsub.Message) bool {
			return css.IsTrustedPeer(ctx, p)
		},
	)
	if err != nil {
		logger.Errorf("error registering topic validator: %s", err)
	}

	var blocksDatastore ds.Batching
	blocksDatastore = namespace.Wrap(css.store, css.namespace.ChildString(blocksNs))

	ipfs, err := ipfslite.New(
		css.ctx,
		blocksDatastore,
		css.host,
		css.dht,
		&ipfslite.Config{
			Offline: false,
		},
	)
	if err != nil {
		logger.Errorf("error creating ipfs-lite: %s", err)
		return
	}

	dagSyncer := newLiteDAGSyncer(css.ctx, ipfs)

	broadcaster, err := crdt.NewPubSubBroadcaster(
		css.ctx,
		css.pubsub,
		topicName, // subscription name
	)
	if err != nil {
		logger.Errorf("error creating broadcaster: %s", err)
		return
	}

	opts := crdt.DefaultOptions()
	opts.RebroadcastInterval = css.config.RebroadcastInterval
	opts.DAGSyncerTimeout = time.Minute
	opts.Logger = logger
	opts.PutHook = func(k ds.Key, v []byte) {
		ctx, span := trace.StartSpan(css.ctx, "crdt/PutHook")
		defer span.End()

		pin := &api.Pin{}
		err := pin.ProtoUnmarshal(v)
		if err != nil {
			logger.Error(err)
			return
		}

		// TODO: tracing for this context
		err = css.rpcClient.CallContext(
			ctx,
			"",
			"PinTracker",
			"Track",
			pin,
			&struct{}{},
		)
		if err != nil {
			logger.Error(err)
		}
		logger.Infof("new pin added: %s", pin.Cid)
	}
	opts.DeleteHook = func(k ds.Key) {
		ctx, span := trace.StartSpan(css.ctx, "crdt/DeleteHook")
		defer span.End()

		c, err := dshelp.DsKeyToCid(k)
		if err != nil {
			logger.Error(err, k)
			return
		}
		pin := api.PinCid(c)

		err = css.rpcClient.CallContext(
			ctx,
			"",
			"PinTracker",
			"Untrack",
			pin,
			&struct{}{},
		)
		if err != nil {
			logger.Error(err)
		}
		logger.Infof("pin removed: %s", c)
	}

	crdt, err := crdt.New(
		css.store,
		css.namespace,
		dagSyncer,
		broadcaster,
		opts,
	)
	if err != nil {
		logger.Error(err)
		return
	}

	css.crdt = crdt

	clusterState, err := dsstate.New(
		css.crdt,
		// unsure if we should set something else but crdt is already
		// namespaced and this would only namespace the keys, which only
		// complicates things.
		"",
		dsstate.DefaultHandle(),
	)
	if err != nil {
		logger.Errorf("error creating cluster state datastore: %s", err)
		return
	}
	css.state = clusterState
	css.readyCh <- struct{}{}
}

// Shutdown closes this component, cancelling the pubsub subscription.
func (css *Consensus) Shutdown(ctx context.Context) error {
	css.shutdownLock.Lock()
	defer css.shutdownLock.Unlock()

	if css.shutdown {
		logger.Debug("already shutdown")
		return nil
	}

	logger.Info("stopping Consensus component")

	css.cancel()

	// Only close crdt after cancelling the context, otherwise
	// the pubsub broadcaster stays on and locks it.
	if crdt := css.crdt; crdt != nil {
		crdt.Close()
	}

	if css.config.hostShutdown {
		css.host.Close()
	}

	css.shutdown = true
	close(css.rpcReady)
	return nil
}

// SetClient gives the component the ability to communicate and
// leaves it ready to use.
func (css *Consensus) SetClient(c *rpc.Client) {
	css.rpcClient = c
	css.rpcReady <- struct{}{}
}

// Ready returns a channel which is signalled when the component
// is ready to use.
func (css *Consensus) Ready(ctx context.Context) <-chan struct{} {
	return css.readyCh
}

// IsTrustedPeer returns whether the given peer is taken into account
// when submitting updates to the consensus state.
func (css *Consensus) IsTrustedPeer(ctx context.Context, pid peer.ID) bool {
	if pid == css.host.ID() {
		return true
	}
	_, ok := css.trustedPeers.Load(pid)
	return ok
}

// Trust marks a peer as "trusted". It makes sure it is trusted as issuer
// for pubsub updates, it is protected in the connection manager, it
// has the highest priority when the peerstore is saved, and it's addresses
// are always remembered.
func (css *Consensus) Trust(ctx context.Context, pid peer.ID) error {
	css.trustedPeers.Store(pid, struct{}{})
	if conman := css.host.ConnManager(); conman != nil {
		conman.Protect(pid, connMgrTag)
	}
	css.peerManager.SetPriority(pid, 0)
	addrs := css.host.Peerstore().Addrs(pid)
	css.host.Peerstore().SetAddrs(pid, addrs, peerstore.PermanentAddrTTL)
	return nil
}

// Distrust removes a peer from the "trusted" set.
func (css *Consensus) Distrust(ctx context.Context, pid peer.ID) error {
	css.trustedPeers.Delete(pid)
	return nil
}

// LogPin adds a new pin to the shared state.
func (css *Consensus) LogPin(ctx context.Context, pin *api.Pin) error {
	ctx, span := trace.StartSpan(ctx, "consensus/LogPin")
	defer span.End()

	return css.state.Add(ctx, pin)
}

// LogUnpin removes a pin from the shared state.
func (css *Consensus) LogUnpin(ctx context.Context, pin *api.Pin) error {
	return css.state.Rm(ctx, pin.Cid)
}

// Peers returns the current known peerset. It uses
// the monitor component and considers every peer with
// valid known metrics a member.
func (css *Consensus) Peers(ctx context.Context) ([]peer.ID, error) {
	var metrics []*api.Metric

	err := css.rpcClient.CallContext(
		ctx,
		"",
		"PeerMonitor",
		"LatestMetrics",
		css.config.PeersetMetric,
		&metrics,
	)
	if err != nil {
		return nil, err
	}

	var peers []peer.ID

	selfIncluded := false
	for _, m := range metrics {
		peers = append(peers, m.Peer)
		if m.Peer == css.host.ID() {
			selfIncluded = true
		}
	}

	// Always include self
	if !selfIncluded {
		peers = append(peers, css.host.ID())
	}

	return peers, nil
}

// WaitForSync is a no-op as it is not necessary to be fully synced for the
// component to be usable.
func (css *Consensus) WaitForSync(ctx context.Context) error { return nil }

// AddPeer is a no-op as we do not need to do peerset management with
// Merkle-CRDTs. Therefore adding a peer to the peerset means doing nothing.
func (css *Consensus) AddPeer(ctx context.Context, pid peer.ID) error {
	return nil
}

// RmPeer is a no-op which always errors, as, since we do not do peerset
// management, we also have no ability to remove a peer from it.
func (css *Consensus) RmPeer(ctx context.Context, pid peer.ID) error {
	return ErrRmPeer
}

// State returns the cluster shared state.
func (css *Consensus) State(ctx context.Context) (state.ReadOnly, error) { return css.state, nil }

// Clean deletes all crdt-consensus datas from the datastore.
func (css *Consensus) Clean(ctx context.Context) error {
	return Clean(ctx, css.config, css.store)
}

// Clean deletes all crdt-consensus datas from the given datastore.
func Clean(ctx context.Context, cfg *Config, store ds.Datastore) error {
	logger.Info("cleaning all CRDT data from datastore")
	q := query.Query{
		Prefix:   cfg.DatastoreNamespace,
		KeysOnly: true,
	}

	results, err := store.Query(q)
	if err != nil {
		return err
	}
	defer results.Close()

	for r := range results.Next() {
		if r.Error != nil {
			return err
		}
		k := ds.NewKey(r.Key)
		err := store.Delete(k)
		if err != nil {
			// do not die, continue cleaning
			logger.Error(err)
		}
	}
	return nil
}

// Leader returns ErrNoLeader.
func (css *Consensus) Leader(ctx context.Context) (peer.ID, error) {
	return "", ErrNoLeader
}

// OfflineState returns an offline, read-only batching state using the given
// datastore. Any writes to this state are processed through the given
// ipfs connector (the state is offline as it does not require a
// running cluster peer).
func OfflineState(cfg *Config, store ds.Datastore) (state.BatchingState, error) {
	batching, ok := store.(ds.Batching)
	if !ok {
		return nil, errors.New("must provide a Bathing datastore")
	}
	opts := crdt.DefaultOptions()
	opts.Logger = logger

	var blocksDatastore ds.Batching
	blocksDatastore = namespace.Wrap(
		batching,
		ds.NewKey(cfg.DatastoreNamespace).ChildString(blocksNs),
	)

	ipfs, err := ipfslite.New(
		context.Background(),
		blocksDatastore,
		nil,
		nil,
		&ipfslite.Config{
			Offline: true,
		},
	)

	if err != nil {
		return nil, err
	}

	dags := newLiteDAGSyncer(context.Background(), ipfs)

	crdt, err := crdt.New(
		batching,
		ds.NewKey(cfg.DatastoreNamespace),
		dags,
		nil,
		opts,
	)
	if err != nil {
		return nil, err
	}
	return dsstate.NewBatching(crdt, "", dsstate.DefaultHandle())
}
