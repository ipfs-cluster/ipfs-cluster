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
    * **API**: a component which offers a public facing API. Default: `RESTAPI`
    * **IPFSConnector**: a component which talks to the IPFS daemon and provides a proxy to it. Default: `IPFSHTTPConnector`
    * **State**: a component which keeps a list of Pins (maintained by the Consensus component)
    * **PinTracker**: a component which tracks the pin set, makes sure that it is persisted by IPFS daemon as intended. Default: `MapPinTracker`
    * **PeerMonitor**: a component to log metrics and detect peer failures. Default: `StdPeerMonitor`
    * **PinAllocator**: a component to decide which peers should pin a CID given some metrics. Default: `NumPinAllocator`
    * **Informer**: a component to collect metrics which are used by the `PinAllocator` and the `PinMonitor`. Default: `NumPin`
  * The **Consensus** component. This component is separate but internal to Cluster in the sense that it cannot be provided arbitrarily during initialization. The consensus component uses `go-libp2p-raft` via `go-libp2p-consensus`. While it is attempted to be agnostic from the underlying consensus implementation, it is not possible in all places. These places are however well marked.

Components perform a number of functions and need to be able to communicate with eachothers: i.e.:

  * the API needs to use functionality provided by the main component
  * the PinTracker needs to use functionality provided by the IPFSConnector
  * the main component needs to use functionality provided by the main component of different peers

### RPC API

Communication between components happens through the RPC API: a set of functions which stablishes which functions are available to components (`rpc_api.go`).

The RPC API uses `go-libp2p-gorpc`. The main Cluster component runs an RPC server. RPC Clients are provided to all components for their use. The main feature of this setup is that **Components can use `go-libp2p-gorpc` to perform operations in the local cluster and in any remote cluster node using the same API**.

This makes broadcasting operations and contacting the Cluster leader really easy. It also allows to think of a future where components may be completely arbitrary and run from different applications. Local RPC calls, on their side, do not suffer any penalty as the execution is short-cut directly to the server component of the Cluster, without network intervention.

On the down-side, the RPC API involves "reflect" magic and it is not easy to verify that a call happens to a method registered on the RPC server. Every RPC-based functionality should be tested. Bad operations will result in errors so they are easy to catch on tests.

### Code layout

Eventually, as the project grow, components will be organized in different submodules. The groundwork for this is already there (i.e. a there is a submodule providing API related types), but most components still live in the base project.

## Applications

### `ipfs-cluster-service`

This is the service application of IPFS Cluster. It brings up a cluster, connects to other peers, gets the latest consensus state and participates in cluster. Handles clean shutdowns when receiving Interrupts.

### `ipfs-cluster-ctl`

This is the client/control application of IPFS Cluster. It is a command line interface which uses the REST API to communicate with Cluster.

## How does it work?

This sections gives an overview of how some things work in Cluster. Doubts? Something missing? Open an issue and ask!

### Startup

* Initialize the P2P host.
* Initialize the PeerManager: needs to keep track of cluster peers.
* Initialize Consensus: start looking for a leader asap.
* Setup RPC in all componenets: allow them to communicate with different parts of the cluster.
* Bootstrap: if we are bootstrapping from another node, do the dance (contact, receive cluster peers, join consensus)
* Cluster is up, but it is only `Ready()` when consensus is ready (which in this case means it has found a leader).
* All components are doing its thing.

Consensus startup deserves a note:

* Raft is setup and wrapped with `go-libp2p-consensus`.
* Waits until there is a leader
* Waits until the state is fully synced (compare raft last log index with current)
* Triggers a `SyncState` to the `PinTracker`, which means that we have recovered the full state of the system and the pin tracker should
keep tabs on the Pins in that state (we don't wait for completion of this operation).
* Consensus is then ready.

If the above procedures don't work, Cluster might just itself down (i.e. if a leader is not found after a while, we automatically shutdown).


### Pinning

* If it's an API request, it involves an RPC request to the cluster main component.
* `Cluster.Pin()`
* We find some peers to pin (`Allocate`) using the metrics from the PeerMonitor and the `PinAllocator`.
* We have a CID and some allocations: time to make a log entry in the consensus log.
* The consensus component will forward this to the Raft leader, as only the leader can make log entries.
* The Raft leader makes a log entry with a `LogOp` that says "pin this CID with this allocations".
* Every peer receives the operation. Per `ConsensusOpLog`, the `Apply(state)` method is triggered. Every peer modifies the `State` accordingly to this operation. The state keeps the full map of CID/Allocations and is only updated via log operations.
* The Apply method triggers a `Track` RPC request to the `PinTracker` in each peer.
* The pin tracker now knows that it needs to track a certain CID.
* If the CID is allocated to the peer, the `PinTracker` will trigger an RPC request to the `IPFSConnector` asking it to pin the CID.
* If the CID is allocated to different peers, the `PinTracker` will mark is as `remote`.
* Now the content is officially pinned.

Notes:
* the log operation should never fail. It has no real places to fail. The calls to the `PinTracker` are asynchronous and a side effect of the state modification.
* the `PinTracker` also should not fail to track anything, BUT
* the `IPFSConnector` might fail to pin something. In this case pins are marked as `pin_error` in the pin tracker.
* the `MapPinTracker` (current implementation of `PinTracker`) queues pin requests so they happen one by one to IPFS. In the meantime things are in `pinning` state.


### Adding a peer

* If it's an API requests, it involves an RPC request to the cluster main component.
* `Cluster.PeerAdd()`
* Figure out the real multiaddress for that peer (the one we see).
* Let the `PeerManager` component know about the new peer. This adds it to the Libp2p host and notifies any component which needs to know
about peers. This means also ability to perform RPC requests to that peer.
* Figure out our multiaddress in regard to the new peer (the one it sees to connect to us). This is done with an RPC request and it also
ensure that the peer is up and reachable.
* Trigger a consensus `LopOp` indicating that there is a new peer.
* Send the new peer the list of cluster peers. This is an RPC request to the new peers `PeerManager` which allows to keep a tab on the current cluster peers.

The consensus part has its own complexity:

* As usual, the "add peer" operation is forwarded to the Raft leader.
* The consensus component logs such operation and also uses Raft `AddPeer` method or equivalent.
* This results in two log entries, one internal to Raft which updates the internal Raft peerstore in all peers, and one from Cluster which is
received by the `Apply` method. This `Apply` operation does not modify the shared `State` (like when pinning), but rather only notifies the `PeerManager` about a new peer so the nodes can be set up to talk to it.

As such we are efectively using the consensus log to broadcast a PeerAdd operation. That is because this is critical and should either succeed everywhere or fail. If we performed a regular "for loop broadcast" and some peers fail, we end up with a mess that needs to be cleaned up and uncertain state. By using the consensus log, those peers which did not receive the operation have the oportunity to receive it later (on restart if they were down). There are still pitfalls to this (restoring from snapshot might result in peers with missing cluster peers) but the errors are more obvious and isolated than in other ways.

Notes:

* The `Join()` (used for bootstrapping) is just a `PeerAdd()` operation triggered remotely by an RPC request from the joining peer.


## Legacy illustrations

See: https://ipfs.io/ipfs/QmWhbV7KX9toZbi4ycj6J9GVbTVvzGx5ERffc6ymYLT5HS

They need to be updated but they are mostly accurate.

