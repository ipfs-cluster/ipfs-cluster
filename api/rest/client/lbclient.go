package client

import (
	"context"
	"math/rand"
	"time"

	cid "github.com/ipfs/go-cid"
	shell "github.com/ipfs/go-ipfs-api"
	files "github.com/ipfs/go-ipfs-files"
	"github.com/ipfs/ipfs-cluster/api"
	peer "github.com/libp2p/go-libp2p-peer"
)

// Strategy values
const (
	Random Strategy = iota
	RoundRobin
)

// Strategy represents load balancing strategy
type Strategy int

// loadBalancingClient a client to interact with IPFS Cluster APIs.
// It balances the load by distributing it among peers.
type loadBalancingClient struct {
	clients  []*Client
	strategy Strategy
	retries  int
	length   int
}

var clientIndex int

func (lc *loadBalancingClient) whichClient() Client {
	switch lc.strategy {
	case Random:
		return *lc.clients[rand.Intn(len(lc.clients))]
	case RoundRobin:
		clientIndex++
		if clientIndex == lc.length {
			clientIndex = 0
		}
		return *lc.clients[clientIndex]
	default:
		return nil
	}
}

// NewLBClient returens a new client that would load balance among
// clients
func NewLBClient(cfgs []*Config, strategy Strategy, retries int) (Client, error) {
	var clients []*Client
	for _, cfg := range cfgs {
		defaultClient, err := NewDefaultClient(cfg)
		if err != nil {
			return nil, err
		}
		clients = append(clients, &defaultClient)
	}

	rand.Seed(time.Now().UnixNano())

	return &loadBalancingClient{clients: clients, strategy: strategy, retries: retries, length: len(clients)}, nil
}

// ID returns information about the cluster Peer.
func (lc *loadBalancingClient) ID(ctx context.Context) (*api.ID, error) {
	var err error
	var id *api.ID
	for {
		id, err = lc.whichClient().ID(ctx)
		if err != nil {
			break
		}
	}
	return id, err
}

// Peers requests ID information for all cluster peers.
func (lc *loadBalancingClient) Peers(ctx context.Context) ([]*api.ID, error) {
	dc := lc.whichClient()
	return dc.Peers(ctx)
}

// PeerAdd adds a new peer to the cluster.
func (lc *loadBalancingClient) PeerAdd(ctx context.Context, pid peer.ID) (*api.ID, error) {
	dc := lc.whichClient()

	return dc.PeerAdd(ctx, pid)
}

// PeerRm removes a current peer from the cluster
func (lc *loadBalancingClient) PeerRm(ctx context.Context, id peer.ID) error {
	dc := lc.whichClient()

	return dc.PeerRm(ctx, id)
}

// Pin tracks a Cid with the given replication factor and a name for
// human-friendliness.
func (lc *loadBalancingClient) Pin(ctx context.Context, ci cid.Cid, opts api.PinOptions) error {
	dc := lc.whichClient()

	return dc.Pin(ctx, ci, opts)
}

// Unpin untracks a Cid from cluster.
func (lc *loadBalancingClient) Unpin(ctx context.Context, ci cid.Cid) error {
	dc := lc.whichClient()

	return dc.Unpin(ctx, ci)
}

// PinPath allows to pin an element by the given IPFS path.
func (lc *loadBalancingClient) PinPath(ctx context.Context, path string, opts api.PinOptions) (*api.Pin, error) {
	dc := lc.whichClient()

	return dc.PinPath(ctx, path, opts)
}

// UnpinPath allows to unpin an item by providing its IPFS path.
// It returns the unpinned api.Pin information of the resolved Cid.
func (lc *loadBalancingClient) UnpinPath(ctx context.Context, p string) (*api.Pin, error) {
	dc := lc.whichClient()

	return dc.UnpinPath(ctx, p)
}

// Allocations returns the consensus state listing all tracked items and
// the peers that should be pinning them.
func (lc *loadBalancingClient) Allocations(ctx context.Context, filter api.PinType) ([]*api.Pin, error) {
	dc := lc.whichClient()

	return dc.Allocations(ctx, filter)
}

// Allocation returns the current allocations for a given Cid.
func (lc *loadBalancingClient) Allocation(ctx context.Context, ci cid.Cid) (*api.Pin, error) {
	dc := lc.whichClient()

	return dc.Allocation(ctx, ci)
}

// Status returns the current ipfs state for a given Cid. If local is true,
// the information affects only the current peer, otherwise the information
// is fetched from all cluster peers.
func (lc *loadBalancingClient) Status(ctx context.Context, ci cid.Cid, local bool) (*api.GlobalPinInfo, error) {
	dc := lc.whichClient()

	return dc.Status(ctx, ci, local)
}

// StatusAll gathers Status() for all tracked items. If a filter is
// provided, only entries matching the given filter statuses
// will be returned. A filter can be built by merging TrackerStatuses with
// a bitwise OR operation (st1 | st2 | ...). A "0" filter value (or
// api.TrackerStatusUndefined), means all.
func (lc *loadBalancingClient) StatusAll(ctx context.Context, filter api.TrackerStatus, local bool) ([]*api.GlobalPinInfo, error) {
	dc := lc.whichClient()

	return dc.StatusAll(ctx, filter, local)
}

// Sync makes sure the state of a Cid corresponds to the state reported by
// the ipfs daemon, and returns it. If local is true, this operation only
// happens on the current peer, otherwise it happens on every cluster peer.
func (lc *loadBalancingClient) Sync(ctx context.Context, ci cid.Cid, local bool) (*api.GlobalPinInfo, error) {
	dc := lc.whichClient()

	return dc.Sync(ctx, ci, local)
}

// SyncAll triggers Sync() operations for all tracked items. It only returns
// informations for items that were de-synced or have an error state. If
// local is true, the operation is limited to the current peer. Otherwise
// it happens on every cluster peer.
func (lc *loadBalancingClient) SyncAll(ctx context.Context, local bool) ([]*api.GlobalPinInfo, error) {
	dc := lc.whichClient()

	return dc.SyncAll(ctx, local)
}

// Recover retriggers pin or unpin ipfs operations for a Cid in error state.
// If local is true, the operation is limited to the current peer, otherwise
// it happens on every cluster peer.
func (lc *loadBalancingClient) Recover(ctx context.Context, ci cid.Cid, local bool) (*api.GlobalPinInfo, error) {
	dc := lc.whichClient()

	return dc.Recover(ctx, ci, local)
}

// RecoverAll triggers Recover() operations on all tracked items. If local is
// true, the operation is limited to the current peer. Otherwise, it happens
// everywhere.
func (lc *loadBalancingClient) RecoverAll(ctx context.Context, local bool) ([]*api.GlobalPinInfo, error) {
	dc := lc.whichClient()

	return dc.RecoverAll(ctx, local)
}

// Version returns the ipfs-cluster peer's version.
func (lc *loadBalancingClient) Version(ctx context.Context) (*api.Version, error) {
	dc := lc.whichClient()

	return dc.Version(ctx)
}

// GetConnectGraph returns an ipfs-cluster connection graph.
// The serialized version, strings instead of pids, is returned
func (lc *loadBalancingClient) GetConnectGraph(ctx context.Context) (*api.ConnectGraph, error) {
	dc := lc.whichClient()

	return dc.GetConnectGraph(ctx)
}

// Metrics returns a map with the latest valid metrics of the given name
// for the current cluster peers.
func (lc *loadBalancingClient) Metrics(ctx context.Context, name string) ([]*api.Metric, error) {
	dc := lc.whichClient()

	return dc.Metrics(ctx, name)
}

// Add imports files to the cluster from the given paths. A path can
// either be a local filesystem location or an web url (http:// or https://).
// In the latter case, the destination will be downloaded with a GET request.
// The AddParams allow to control different options, like enabling the
// sharding the resulting DAG across the IPFS daemons of multiple cluster
// peers. The output channel will receive regular updates as the adding
// process progresses.
func (lc *loadBalancingClient) Add(
	ctx context.Context,
	paths []string,
	params *api.AddParams,
	out chan<- *api.AddedOutput,
) error {
	dc := lc.whichClient()

	return dc.Add(ctx, paths, params, out)
}

// AddMultiFile imports new files from a MultiFileReader. See Add().
func (lc *loadBalancingClient) AddMultiFile(
	ctx context.Context,
	multiFileR *files.MultiFileReader,
	params *api.AddParams,
	out chan<- *api.AddedOutput,
) error {
	dc := lc.whichClient()

	return dc.AddMultiFile(ctx, multiFileR, params, out)
}

// IPFS returns an instance of go-ipfs-api's Shell, pointing to the
// configured ProxyAddr (or to the default Cluster's IPFS proxy port).
// It re-uses this Client's HTTP client, thus will be constrained by
// the same configurations affecting it (timeouts...).
func (lc *loadBalancingClient) IPFS(ctx context.Context) *shell.Shell {
	dc := lc.whichClient()
	return dc.IPFS(ctx)
}
