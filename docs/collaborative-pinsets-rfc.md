# Collaborative pinsets

This document outlines a possible implementation plan for "collaborative pinsets" as described below, within the scope of IPFS Cluster.

## Definition

A **collaborative pinset** is a collection of CIDs which is pinned by a number of IPFS Cluster peers which:

* Trust one or several peers to publish and update such pinset, but not others
* May freely participate or stop participating in the cluster, without this affecting
other peers or the pinset

## State of the art in IPFS Cluster

IPFS Cluster currently supports pinsets in a trusted environment where every node, once participating in the cluster has full control of the other peers (via unauthenticated RPC). The cluster secret (pnet key) ensures that only peers with a pre-shared key can request joining the cluster.

Maintenance of the peer-set is performed by the **consensus component**, which provides an interface to 1) The maintenance of the peer-set 2) The maintenance of the pinset (*shared state*).

Other components of cluster are independent from these two tasks, and provide functionality that will be useful in scenarios where the peer-set and pin-set maintenance works in a different manner:

* A state component provides pinset representation and serialization
* An ipfs connector component provides facilities for controlling the ipfs daemon (and the proxy)
* A pintracker component provides functionality to ensure the shared state is followed by ipfs
* A monitoring component provides metric collection.

Thus, just replacing the consensus layer (with some caveats ellaborated below) is a relatively simple approach to support collaborative pinsets.

## A new consensus layer for collaborative pinsets

### State of the art of the consensus component

The consensus component is defined by the following interface:

```go
type Consensus interface {
	Component
	Ready() <-chan struct{}
	LogPin(c api.Pin) error
	LogUnpin(c api.Pin) error
	AddPeer(p peer.ID) error
	RmPeer(p peer.ID) error
	State() (state.State, error)
	Leader() (peer.ID, error)
	WaitForSync() error
	Clean() error
	Peers() ([]peer.ID, error)
}
```

This is essentially a wrapper of the go-libp2p-consensus which adds functionality and utility methods which until now applied to raft (provided by go-libp2p-raft).

The purpose of Raft is to maintain a shared state by providing a distributed append-only log. In a Raft cluster, the log is maintained by an elected Leader, compacted and snapshotted convieniently. IPFS Cluster has spent significant efforts in detaching the state representation from raft, and allowing transformations (upgrades) to run indepedently, based solely on the "state component".

### A new consensus component using a blockchain

In order to be consistent with how components are meant to interact with each-other, it makes sense to implement the shared state maintenance in a new `go-libp2p-consensus` implementation. The main pain points in a collaborative cluster will be:

* Security: all peers should not be able to control or influence the behaviour of all other peers (including updating the state). For this, we will introduce the notion of a *trusted peerset*.
* Scalability: this consensus layer should support very large number of peers, efficient broadcast of state updates. For this we will introduce a blockchain-based consensus layer and improvements on how we interact with other peers over RPC.

### Security: Authentication and authorization for collaborative pinning

In a collaborative pinset scenario, we probably want to have a limited set of peers which are able to modify the shared state and freely connect to API endpoints from any other peers. We call this **trusted peerset**. This implies that we need to find ways to:

* Limit modifications of the state to the  **trusted peerset**, being the rest of cluster peers just "followers".
* Limit the internal RPC API to the **trusted peerset**

We can address the first problem by signing state upgrades (along with a variable number, to avoid replay attacks), allowing any peer to authenticate them (as needed). In libp2p, peers can obtain the public key and verify the signatures for the peers they trust by simply connecting to them.

For the second point, we have to consider the internal RPC API surface. Until now, it is assumed that all cluster peers can contact and use the RPC endpoints of all others. This is however very problematic as it would allow any peer for example to trigger ipfs pin/unpins anywhere. For this reason, we propose **authenticated RPC endpoints**, which will only allow a set of trusted peers to execute any of the RPC calls. This can be added as a feature of libp2p-gorpc, taking advantage of the authentication information provided by libp2p. Note, we will have to do UI adjustments so that non-trusted peers receive appropiate errors when they don't have rights to perform certain operations.


### Scalability

First, we need to address the **scalability of the shared state** and updates to it. Using any of the many blockchains that support custom data payloads (like ethereum) to maintain the shared state addresses the scalability problem for the maintance of the shared state. Essentially, blockchains are scalable a consensus mechanism to maintain a shared state which grows stronger with the number of peers participating in it.

Secondly, we need to address the **scalability problem for inter-peer communications**: for example, sending metrics so that pins can be allocated, or retrieving the status of an item over a very large number of peers will be a problem. In a `pin everywhere` scenario though, where allocations (thus metrics) are not needed, this becomes much smaller. All in all, we should avoid that all peers connect to a single trusted peer (or connect to all other peers). Ideally, peers would be connected only to a subset or would be able to join the cluster by just `bootstrapping` to any other peer, without necessarily keeping permanent connections active to the trusted peers.

There are a few places in cluster where all the peers in the peerset are contacted in order to fetch information (PeerAdd, ConnectGraph etc), but the critical one will be the broadcasts corresponding to Status, Sync and  Recover operations. All these can be optimized by:

a) Limiting requests to the peers that have content allocated to them when dealing with a single CID
b) Supporting iterative (rather than parallel) checking for global state calls
c) Use libp2p connection manager to drop connections when they become grow too numerous (same as ipfs)

All these, however, will need to be battletested. On the plus side, the three operations are not critical to the central problem, which is having many peers follow a pinset.

Third, we need to address the **scalability problem for maintaining the peerset**. For this we can take advantage of pubsub-based monitoring. Listing all peers for which we have received a metric over pubsub will provide an effective peerset. Thus, the consensus layer can rely on the monitoring component for `Peers()`. Using pubsub ensures that metrics are distributed among the swarm in a scalable fashion, without the need for all peers to keep connections open to the *trusted peerset*.


### Dealing with malicious actors

There are several ways that a malicious peer might try to interfere with the activity of a collaborative cluster. In general, we should aim to have a working cluster when a majority of the peers in it are not malicious.

* Having a **trusted peerset** makes it an easy target for DDoS attacks on the swarm.
* A bad peer can pretend to be pinning something but not do it
* A bad peer can pretend to have infinite space and be always allocated things

Many of these problems (and others) have been (or will be) addressed by Filecoin. It is not IPFS Cluster goal to overlap with Filecoin in this feature, but rather to offer a simple way to "follow" a pinset which has less overhead and requirements than the current Raft consensus (in the future it will be good to study how Filecoin can be used as the consensus layer for cluster). For the moment:

* We will recommend high replication factors which make it less likely that all allocations for a Cid belong to malicious actors
* We will have to change the allocator strategy to something that randomly select a subset of peers first, and then applies the strategy. This should as well make it less likely for malicious peers to always appear in front of the allocation candidates.
* We will have to adapt the actual replication factor dynamically: as new peers join the cluster, we'd want to allocate content to them. It helps thinking in percentages: i.e. a CID should be allocated to 10% of the peers. As peers grow, new peers should start tracking that content. Open questions around allocation strategies are discussed in the next section.

### Allocations: open problems

Currently the allocations for a Cid are part of the shared state and consist of a list of peer IDs, decided when the item was added.

In a scenario where peers come and go and come back, this strategy feels suboptimal (although it would work on principle). We should probably work on an allocator component which can efficiently track and handle allocations as peers come and go. For example, if the minimum allocation factor cannot be reached, cluster should still pin the items and track them as underpinned and, as new peers join, it should allocate underpinned items to them. As peers go away, cluster should efficiently identify which pins need re-allocation.

Perhaps the whole allocation format should be re-thought, allowing each of the peers to detect items which have not reached a high-enough replication factor and tracking them. Then these peers would inform the allocator that they are going to worry about the items. This is, again, a difficult problem which Filecoin has solved properly by providing a market where peers can offer to store content.


## Prototype: ipfs blockchain and pubsub

The easiest way to approach the proposal above is with a prototype that uses IPFS to store a blockchain and pubsub to announce the current blockchain head. We can inform of new chain heads by publishing a new message using pubsub that points to the chain head's CID. These messages will be signed by one of the **trusted peers**. We can also use all peers to automatically backup the chain. In the case of conflicts, the longest chain will win.

Each chain block contains a sequential set of LogOp very much in the fashion of the current Raft log. The consensus layer, upon receiving a pubsub message with a new chain head:

* Verifies it's signed by a trusted peer
* If height > current -> processes the chain and so on


## UX in collaborative pinsets

Ideally it will have a super-easy setup:

1. Run the go-ipfs daemon
2. `ipfs-cluster-service init --template /ipns/Qmxxx`
3. `ipfs-cluster-service daemon`

This will fetch configuration template with pre-filled "trusted" peers and the optimal configuration for the collaborative pinset cluster will be provided.
