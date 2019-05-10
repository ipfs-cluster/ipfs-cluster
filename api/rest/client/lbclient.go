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

// loadBalancingClient a client to interact with IPFS Cluster APIs.
// It balances the load by distributing it among peers.
type loadBalancingClient struct {
	strategy LBStrategy
}

// LBStrategy is a strategy to load balance request among clients
type LBStrategy interface {
	Next(count int, call func(Client) error) error
}

// RoundRobin is a load balancing strategy that would use clients in a sequence.
type RoundRobin struct {
	clients []Client
	length  int
	retries int
	counter chan int
}

// NewRoundRobin would return an LBStrategy that load balances requests by using
// clients in sequence.
func NewRoundRobin(cfgs []*Config, retries int) (LBStrategy, error) {
	var clients []Client
	for _, cfg := range cfgs {
		defaultClient, err := NewDefaultClient(cfg)
		if err != nil {
			return nil, err
		}
		clients = append(clients, defaultClient)
	}

	rand.Seed(time.Now().UnixNano())

	counter := make(chan int)
	counter <- 0
	return &RoundRobin{clients: clients, length: len(clients), retries: retries, counter: counter}, nil
}

// Next return the next client to be used
func (r *RoundRobin) Next(count int, call func(Client) error) error {
	i := <-r.counter
	r.counter <- (i + 1) % r.length
	err := call(r.clients[i])
	apiErr, ok := err.(*api.Error)
	if !ok {
		logger.Error("could not cast error into api.Error")
		return err
	}

	count++
	if count == r.retries || err == nil || apiErr.Code != 0 {
		return err
	}
	return r.Next(count, call)
}

// Failover is a load balancing strategy that would try the local cluster peer first.
// If the local call fail it would try other client in a round robin fashion.
type Failover struct {
	clients []Client
	length  int
	retries int
	counter chan int
}

// NewFailover would return an LBStrategy that uses the local client first and
// if that fails it would try other clients in a round robin like fashion.
func NewFailover(cfgs []*Config, retries int) (LBStrategy, error) {
	var clients []Client
	for _, cfg := range cfgs {
		defaultClient, err := NewDefaultClient(cfg)
		if err != nil {
			return nil, err
		}
		clients = append(clients, defaultClient)
	}

	rand.Seed(time.Now().UnixNano())

	counter := make(chan int)
	counter <- 0
	return &RoundRobin{clients: clients, length: len(clients), retries: retries, counter: counter}, nil
}

// Next returns the next client to be used.
func (f *Failover) Next(count int, call func(Client) error) error {
	var err error
	if count == 0 {
		err = call(f.clients[0])
	} else {
		i := <-f.counter
		f.counter <- (i + 1) % f.length
		err = call(f.clients[i])
	}
	apiErr, ok := err.(*api.Error)
	if !ok {
		logger.Error("could not cast error into api.Error")
		return err
	}

	count++
	if count == f.retries || err == nil || apiErr.Code != 0 {
		return err
	}
	return f.Next(count, call)
}

// NewLBClient returns a new client that will load balance amongst
// clients
func NewLBClient(strategy LBStrategy) Client {
	return &loadBalancingClient{strategy: strategy}
}

// ID returns information about the cluster Peer.
func (lc *loadBalancingClient) ID(ctx context.Context) (*api.ID, error) {
	var id *api.ID
	call := func(c Client) error {
		var err error
		id, err = c.ID(ctx)
		return err
	}

	err := lc.strategy.Next(0, call)
	return id, err
}

// Peers requests ID information for all cluster peers.
func (lc *loadBalancingClient) Peers(ctx context.Context) ([]*api.ID, error) {
	var peers []*api.ID
	call := func(c Client) error {
		var err error
		peers, err = c.Peers(ctx)
		return err
	}

	err := lc.strategy.Next(0, call)
	return peers, err
}

// PeerAdd adds a new peer to the cluster.
func (lc *loadBalancingClient) PeerAdd(ctx context.Context, pid peer.ID) (*api.ID, error) {
	var id *api.ID
	call := func(c Client) error {
		var err error
		id, err = c.PeerAdd(ctx, pid)
		return err
	}

	err := lc.strategy.Next(0, call)
	return id, err
}

// PeerRm removes a current peer from the cluster
func (lc *loadBalancingClient) PeerRm(ctx context.Context, id peer.ID) error {
	call := func(c Client) error {
		return c.PeerRm(ctx, id)
	}

	return lc.strategy.Next(0, call)
}

// Pin tracks a Cid with the given replication factor and a name for
// human-friendliness.
func (lc *loadBalancingClient) Pin(ctx context.Context, ci cid.Cid, opts api.PinOptions) error {
	call := func(c Client) error {
		return c.Pin(ctx, ci, opts)
	}

	return lc.strategy.Next(0, call)
}

// Unpin untracks a Cid from cluster.
func (lc *loadBalancingClient) Unpin(ctx context.Context, ci cid.Cid) error {
	call := func(c Client) error {
		return c.Unpin(ctx, ci)
	}

	return lc.strategy.Next(0, call)
}

// PinPath allows to pin an element by the given IPFS path.
func (lc *loadBalancingClient) PinPath(ctx context.Context, path string, opts api.PinOptions) (*api.Pin, error) {
	var pin *api.Pin
	call := func(c Client) error {
		var err error
		pin, err = c.PinPath(ctx, path, opts)
		return err
	}

	err := lc.strategy.Next(0, call)
	return pin, err
}

// UnpinPath allows to unpin an item by providing its IPFS path.
// It returns the unpinned api.Pin information of the resolved Cid.
func (lc *loadBalancingClient) UnpinPath(ctx context.Context, p string) (*api.Pin, error) {
	var pin *api.Pin
	call := func(c Client) error {
		var err error
		pin, err = c.UnpinPath(ctx, p)
		return err
	}

	err := lc.strategy.Next(0, call)
	return pin, err
}

// Allocations returns the consensus state listing all tracked items and
// the peers that should be pinning them.
func (lc *loadBalancingClient) Allocations(ctx context.Context, filter api.PinType) ([]*api.Pin, error) {
	var pins []*api.Pin
	call := func(c Client) error {
		var err error
		pins, err = c.Allocations(ctx, filter)
		return err
	}

	err := lc.strategy.Next(0, call)
	return pins, err
}

// Allocation returns the current allocations for a given Cid.
func (lc *loadBalancingClient) Allocation(ctx context.Context, ci cid.Cid) (*api.Pin, error) {
	var pin *api.Pin
	call := func(c Client) error {
		var err error
		pin, err = c.Allocation(ctx, ci)
		return err
	}

	err := lc.strategy.Next(0, call)
	return pin, err
}

// Status returns the current ipfs state for a given Cid. If local is true,
// the information affects only the current peer, otherwise the information
// is fetched from all cluster peers.
func (lc *loadBalancingClient) Status(ctx context.Context, ci cid.Cid, local bool) (*api.GlobalPinInfo, error) {
	var pinInfo *api.GlobalPinInfo
	call := func(c Client) error {
		var err error
		pinInfo, err = c.Status(ctx, ci, local)
		return err
	}

	err := lc.strategy.Next(0, call)
	return pinInfo, err
}

// StatusAll gathers Status() for all tracked items. If a filter is
// provided, only entries matching the given filter statuses
// will be returned. A filter can be built by merging TrackerStatuses with
// a bitwise OR operation (st1 | st2 | ...). A "0" filter value (or
// api.TrackerStatusUndefined), means all.
func (lc *loadBalancingClient) StatusAll(ctx context.Context, filter api.TrackerStatus, local bool) ([]*api.GlobalPinInfo, error) {
	var pinInfos []*api.GlobalPinInfo
	call := func(c Client) error {
		var err error
		pinInfos, err = c.StatusAll(ctx, filter, local)
		return err
	}

	err := lc.strategy.Next(0, call)
	return pinInfos, err
}

// Sync makes sure the state of a Cid corresponds to the state reported by
// the ipfs daemon, and returns it. If local is true, this operation only
// happens on the current peer, otherwise it happens on every cluster peer.
func (lc *loadBalancingClient) Sync(ctx context.Context, ci cid.Cid, local bool) (*api.GlobalPinInfo, error) {
	var pinInfo *api.GlobalPinInfo
	call := func(c Client) error {
		var err error
		pinInfo, err = c.Sync(ctx, ci, local)
		return err
	}

	err := lc.strategy.Next(0, call)
	return pinInfo, err
}

// SyncAll triggers Sync() operations for all tracked items. It only returns
// informations for items that were de-synced or have an error state. If
// local is true, the operation is limited to the current peer. Otherwise
// it happens on every cluster peer.
func (lc *loadBalancingClient) SyncAll(ctx context.Context, local bool) ([]*api.GlobalPinInfo, error) {
	var pinInfos []*api.GlobalPinInfo
	call := func(c Client) error {
		var err error
		pinInfos, err = c.SyncAll(ctx, local)
		return err
	}

	err := lc.strategy.Next(0, call)
	return pinInfos, err
}

// Recover retriggers pin or unpin ipfs operations for a Cid in error state.
// If local is true, the operation is limited to the current peer, otherwise
// it happens on every cluster peer.
func (lc *loadBalancingClient) Recover(ctx context.Context, ci cid.Cid, local bool) (*api.GlobalPinInfo, error) {
	var pinInfo *api.GlobalPinInfo
	call := func(c Client) error {
		var err error
		pinInfo, err = c.Recover(ctx, ci, local)
		return err
	}

	err := lc.strategy.Next(0, call)
	return pinInfo, err
}

// RecoverAll triggers Recover() operations on all tracked items. If local is
// true, the operation is limited to the current peer. Otherwise, it happens
// everywhere.
func (lc *loadBalancingClient) RecoverAll(ctx context.Context, local bool) ([]*api.GlobalPinInfo, error) {
	var pinInfos []*api.GlobalPinInfo
	call := func(c Client) error {
		var err error
		pinInfos, err = c.RecoverAll(ctx, local)
		return err
	}

	err := lc.strategy.Next(0, call)
	return pinInfos, err
}

// Version returns the ipfs-cluster peer's version.
func (lc *loadBalancingClient) Version(ctx context.Context) (*api.Version, error) {
	var v *api.Version
	call := func(c Client) error {
		var err error
		v, err = c.Version(ctx)
		return err
	}

	err := lc.strategy.Next(0, call)
	return v, err
}

// GetConnectGraph returns an ipfs-cluster connection graph.
// The serialized version, strings instead of pids, is returned
func (lc *loadBalancingClient) GetConnectGraph(ctx context.Context) (*api.ConnectGraph, error) {
	var graph *api.ConnectGraph
	call := func(c Client) error {
		var err error
		graph, err = c.GetConnectGraph(ctx)
		return err
	}

	err := lc.strategy.Next(0, call)
	return graph, err
}

// Metrics returns a map with the latest valid metrics of the given name
// for the current cluster peers.
func (lc *loadBalancingClient) Metrics(ctx context.Context, name string) ([]*api.Metric, error) {
	var metrics []*api.Metric
	call := func(c Client) error {
		var err error
		metrics, err = c.Metrics(ctx, name)
		return err
	}

	err := lc.strategy.Next(0, call)
	return metrics, err
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
	call := func(c Client) error {
		return c.Add(ctx, paths, params, out)
	}

	return lc.strategy.Next(0, call)
}

// AddMultiFile imports new files from a MultiFileReader. See Add().
func (lc *loadBalancingClient) AddMultiFile(
	ctx context.Context,
	multiFileR *files.MultiFileReader,
	params *api.AddParams,
	out chan<- *api.AddedOutput,
) error {
	call := func(c Client) error {
		return c.AddMultiFile(ctx, multiFileR, params, out)
	}

	return lc.strategy.Next(0, call)
}

// IPFS returns an instance of go-ipfs-api's Shell, pointing to the
// configured ProxyAddr (or to the default Cluster's IPFS proxy port).
// It re-uses this Client's HTTP client, thus will be constrained by
// the same configurations affecting it (timeouts...).
func (lc *loadBalancingClient) IPFS(ctx context.Context) *shell.Shell {
	var s *shell.Shell
	call := func(c Client) error {
		s = c.IPFS(ctx)
		return nil
	}

	lc.strategy.Next(0, call)

	return s
}
