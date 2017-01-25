# IPFS Cluster architecture

## Definitions

These definitions are still evolving and may change:

  * Member: an ipfs cluster server node which is member of the consensus. Alternatively: node, peer
  * ipfs-cluster: the IPFS cluster software
  * ipfs-cluster-ctl: the IPFS cluster client command line application
  * ipfs-cluster-service: the IPFS cluster node application
  * API: the REST-ish API implemented by the RESTAPI component and used by the clients.
  * RPC API: the internal API that cluster members and components use.
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
  * The **Consensus** component. This component separate but internal to Cluster in the sense that it cannot be provided arbitrarily during initialization. The consensus component uses `go-libp2p-raft` via `go-libp2p-consensus`. While it is attempted to be agnostic from the underlying consensus implementation, it is not possible in all places. These places are however well marked.

Components perform a number of functions and need to be able to communicate with eachothers: i.e.:

  * the API needs to use functionality provided by the main component
  * the PinTracker needs to use functionality provided by the IPFSConnector
  * the main component needs to use functionality provided by the main component of a different member (the leader)

### RPC API

Communication between components happens through the RPC API: a set of functions which stablishes which functions are available to components (`rpc_api.go`).

The RPC API uses `go-libp2p-gorpc`. The main Cluster component runs an RPC server. RPC Clients are provided to all components for their use. The main feature of this setup is that **Components can use `go-libp2p-gorpc` to perform operations in the local cluster and in any remote cluster node using the same API**.

This makes broadcasting operations, contacting the Cluster leader really easy. It also allows to think of a future where components may be completely arbitrary and run from different applications. Local RPC calls, on their side, do not suffer any penalty as the execution is short cut directly to the server component of the Cluster, without network intervention.

### Code layout

Currently, all components live in the same `ipfscluster` Go module, but they shall be moved to their own submodules without trouble in the future.

## Applications

### `ipfs-cluster-service`

This is the service application of IPFS Cluster. It brings up a cluster, connects to other members, gets the latest consensus state and participates in cluster.

### `ipfs-cluster-ctl`

This is the client/control application of IPFS Cluster. It is a command line interface which uses the REST API to communicate with Cluster.

## Legacy illustrations

See: https://ipfs.io/ipfs/QmWhbV7KX9toZbi4ycj6J9GVbTVvzGx5ERffc6ymYLT5HS

They need to be updated but they are mostly accurate.

