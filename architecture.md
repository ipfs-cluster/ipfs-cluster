# IPFS Cluster architecture

## Definitions

These definitions are still evolving and may change:

  * Peer: an ipfs cluster service node which is member of the consensus. Alternatively: node, member.
  * ipfs-cluster: the IPFS cluster software
  * ipfs-cluster-ctl: the IPFS cluster client command line application
  * ipfs-cluster-service: the IPFS cluster node application
  * API: the REST-ish API implemented by the RESTAPI component and used by the clients.
  * RPC API: the internal API that cluster peers and components use.
  * Go API: the public interface offered by the Cluster object in Go.
  * Component: an ipfs-cluster module which performs specific functions and uses the RPC API to communicate with other parts of the system. Implements the Component interface.

## General overview

### Modularity

`ipfs-cluster` architecture attempts to be as modular as possible, with interfaces between its modules (components) clearly defined, in a way that they can:

  * Be swapped for alternative implementations, improved implementations, separately without affecting the rest of the system
  * Be easily tested in isolation

### Components

`ipfs-cluster` consists of:

  * The definitions of components and their interfaces and related types (`ipfscluster.go`)
  * The **Cluster** main-component which binds together the whole system and offers the Go API (`cluster.go`). This component takes an arbitrary:
    * **PinTracker**: a component which tracks the pin set, makes sure that it is persisted by IPFS daemon as intended. Default: `MapPinTracker`
    * **IPFSConnector**: a component which talks to the IPFS daemon and provides a proxy to it. Default: `IPFSHTTPConnector`
    * **API**: a component which offers a public facing API. Default: `RESTAPI`
    * **State**: a component which keeps a list of Pins (maintained by the Consensus component)
  * The **Consensus** component. This component is separate but internal to Cluster in the sense that it cannot be provided arbitrarily during initialization. The consensus component uses `go-libp2p-raft` via `go-libp2p-consensus`. While it is attempted to be agnostic from the underlying consensus implementation, it is not possible in all places. These places are however well marked.

Components perform a number of functions and need to be able to communicate with eachothers: i.e.:

  * the API needs to use functionality provided by the main component
  * the PinTracker needs to use functionality provided by the IPFSConnector
  * the main component needs to use functionality provided by the main component of a different peer (the leader)

### RPC API

Communication between components happens through the RPC API: a set of functions which stablishes which functions are available to components (`rpc_api.go`).

The RPC API uses `go-libp2p-gorpc`. The main Cluster component runs an RPC server. RPC Clients are provided to all components for their use. The main feature of this setup is that **Components can use `go-libp2p-gorpc` to perform operations in the local cluster and in any remote cluster node using the same API**.

This makes broadcasting operations, contacting the Cluster leader really easy. It also allows to think of a future where components may be completely arbitrary and run from different applications. Local RPC calls, on their side, do not suffer any penalty as the execution is short cut directly to the server component of the Cluster, without network intervention.

### Code layout

Currently, all components live in the same `ipfscluster` Go module, but they shall be moved to their own submodules without trouble in the future.

## Applications

### `ipfs-cluster-service`

This is the service application of IPFS Cluster. It brings up a cluster, connects to other peers, gets the latest consensus state and participates in cluster.

### `ipfs-cluster-ctl`

This is the client/control application of IPFS Cluster. It is a command line interface which uses the REST API to communicate with Cluster.

## Code paths

This sections explains how some things work in Cluster.

### Startup

* `NewCluster` triggers the Cluster bootstrap. `IPFSConnector`, `State`, `PinTracker` component are provided. These components are up but possibly waiting for a `SetClient(RPCClient)` call. Without having an RPCClient, they are unable to communicate with the other components.
* The first step bootstrapping is to create the libp2p `Host`. It is using configuration keys to set up public, private keys, the swarm network etc.
* The `peerManager` is initialized. It allows to list known cluster peers and keep components in the loop about changes.
* The `RPCServer` (`go-libp2p-gorpc`) is setup, along with an `RPCClient`. The client can communicate to any RPC server using libp2p, or with the local one (shortcutting). The `RPCAPI` object is registered: it implements all the RPC operations supported by cluster and used for inter-component/inter-peer communication.
* The `Consensus` component is bootstrapped. It:
  * Sets up a new Raft node (with the `cluster_peers` from the configuration) from scratch, including snapshots, stable datastore (boltDB), log store etc...
  * Initializes `go-libp2p-consensus` components (`Actor` and `LogOpConsensus`) using `go-libp2p-raft`
  * Returns a new `Consensus` object while asynchronously waiting for a Raft leader and then also for the current Raft peer to catch up with the latest index of the log when there is anything to catch up with.
  * Waits for an RPCClient to be set and when it happens it triggers an asynchronous RPC request (`StateSync`) to the local `Cluster` component and reports `Ready()`
  * The `StateSync` operation (from the main `Cluster` component) makes a diff between the local `MapPinTracker` state and the consensus-maintained state. It triggers asynchronous local RPC requests (`Track` and `Untrack`) to the `MapPinTracker`.
  * The `MapPinTracker` receives the `Track` requests and checks, pins or unpins items, as well as updating the local status of a pin (see the Pinning section below)
* While the consensus is being bootstrapped, `SetClient(RPCClient` is called on all components (tracker, ipfs connector, api and consensus)
* The new `Cluster` object is returned.
* Asynchronously, a thread triggers the bootsrap process and then waits for the `Consensus` component to report `Ready()`.
* The bootstrap process uses the configured multiaddresses from the Bootstrap section of the configuration perform a remote RPC `PeerAdd` request (it tries until it works in one of them).
* The remote node then performs a consensus log operation logging the multiaddress and peer ID of the new cluster member. If the log operation is successful, the new member is added to `Raft` (which internally logs it as well). The new node is contacted by Raft, its internal peerset updated. It receives the state from the cluster, including pins (which are synced to the `PinTracker` and other peers, which are handled by the `peerManager`.
* If the bootstrap was successful or not necessary, Cluster reports itself `Ready()`. At this moment, all components are up, consensus is working and cluster is ready to perform any operations. The consensus state may still be syncing, or mostly the `MapPinTracker` may still be verifying that pins are there against the daemon, but this does not causes any problems to use cluster.


### Adding a peer

* The `RESTAPI` component receives a `PeerAdd` request on the respective endpoint. It makes a `PeerAdd` RPC request.
* The local RPC server receives it and calls `PeerAdd` method in the main `Cluster` component.
* If there is a connection from the given PID, we use it to determine the true multiaddress (with the right remote IP), in case it was not
provided correctly (usually when using `Join` since it cannot be determined-see below). This allows to correctly let peers join from accross NATs.
* The peer is added to the `peerManager` so we can perform RPC requests to it.
* A remote RPC `RemoteMultiaddressForPeer` is performed to the new peer, so it reports how our multiaddress looks from their side.
* The address of the new peer is then commited to the consensus state. Since this is a new peer, Raft peerstore is also updated. Raft starts
sending updates to the new peer, among them prompting it to update its own peerstore with the list of peers in this cluster.
* The list of cluster peers (plus ourselves with our real multiaddress) is sent to the new cluster in a remote `PeerManagerAddMultiaddrs` RPC request.
* A remote `ID` RPC request is performed and the information about the new node is returned.

### Joining a cluster

* Joining means calling `AddPeer` on the cluster we want to join.
* This is used also for bootstrapping.
* Join is disallowed if we are not a single-peer cluster.

### Pinning

* The `RESTAPI` component receives a `Pin` request on the respective endpoint. This triggers a `Pin` RPC request.
* The local RPC server receives it and calls `Pin` method in the main `Cluster` component.
* The main cluster component calls `LogPin` in the `Consensus` Component.
* The Consensus component checks that it is the Raft leader, or alternatively makes an `ConsensusLogPin` RPC request to whoever is the leader.
* The Consensus component of the current Raft's leader builds a pin `clusterLogOp` operation performs a `consensus.Commit(op)`. This is handled by `go-libp2p-raft` and, when the operation makes it to the Raft's log, it triggers `clusterLogOp.ApplyTo(state)` in every peer.
* `ApplyTo` modifies the local cluster state by adding the pin and triggers an asynchronous `Track` local RPC request. It returns without waiting for the result. While `ApplyTo` is running no other state updates may be applied so it is a critical code path.
* The RPC server handles the request to the `MapPinTracker` component and its `Track` method. It marks the local state of the pin as `pinning` and makes a local RPC request for the `IPFSHTTPConnector` component (`IPFSPin`) to pin it.
* The `IPFSHTTPConnector` component receives the request and uses the configured IPFS daemon API to send a pin request with the given hash. It waits until the call comes back and returns success or failure.
* The `MapPinTracker` component receives the result from the RPC request to the `IPFSHTTPConnector` and updates the local status of the pin to `pinned` or `pin_error`.

## Legacy illustrations

See: https://ipfs.io/ipfs/QmWhbV7KX9toZbi4ycj6J9GVbTVvzGx5ERffc6ymYLT5HS

They need to be updated but they are mostly accurate.

