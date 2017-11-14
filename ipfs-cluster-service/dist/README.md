# `ipfs-cluster-service`

> IPFS cluster peer daemon

`ipfs-cluster-service` runs a full IPFS Cluster peer.

![ipfs-cluster-service example](https://ipfs.io/ipfs/QmWf2asBu54nEaCzfJtdyP1KQjf4pWXmqeHYHZJm86eHAT)

### Usage

Usage information can be obtained with:

```
$ ipfs-cluster-service -h
```

### Initialization

Before running `ipfs-cluster-service` for the first time, initialize a configuration file with:

```
$ ipfs-cluster-service init
```

`init` will randomly generate a `cluster_secret` (unless specified by the `CLUSTER_SECRET` environment variable or running with `--custom-secret`, which will prompt it interactively).

**All peers in a cluster must share the same cluster secret**. Using an empty secret may compromise the security of your cluster (see the documentation for more information).


### Configuration

After initialization, the configuration will be placed in `~/.ipfs-cluster/service.json` by default.

You can add the multiaddresses for the other cluster peers to the `cluster.peers` or `cluster.bootstrap` variables (see below). A configuration example with explanations is provided in [A guide to running IPFS Cluster](https://github.com/ipfs/ipfs-cluster/blob/master/docs/ipfs-cluster-guide.md).

The configuration file should probably be identical among all cluster peers, except for the `id` and `private_key` fields. Once every cluster peer has the configuration in place, you can run `ipfs-cluster-service` to start the cluster.

#### Clusters using `cluster.peers`

The `peers` configuration variable holds a list of current cluster members. If you know the members of the cluster in advance, or you want to start a cluster fully in parallel, set `peers` in all configurations so that every peer knows the rest upon boot. Leave `bootstrap` empty. A cluster peer address looks like: `/ip4/1.2.3.4/tcp/9096/<id>`.

The list of `cluster.peers` is maintained automatically and saved by `ipfs-cluster-service` when it changes.

#### Clusters using `cluster.bootstrap`

When the `peers` variable is empty, the multiaddresses in `bootstrap` (or the `--bootstrap` parameter to `ipfs-cluster-service`) can be used to have a peer join an existing cluster. The peer will contact those addresses (in order) until one of them succeeds in joining it to the cluster. When the peer is shut down, it will save the current cluster peers in the `peers` configuration variable for future use (unless `leave_on_shutdown` is true, in which case it will save them in `bootstrap`).

Bootstrap is a convenient method to sequentially start the peers of a cluster. **Only bootstrap clean nodes** which have not been part of a cluster before (or clean the `ipfs-cluster-data` folder). Bootstrapping nodes with an old state (or diverging state) from the one running in the cluster will fail or lead to problems with the consensus layer.

When setting the `leave_on_shutdown` option, or calling `ipfs-cluster-service` with the `--leave` flag, the node will attempt to leave the cluster in an orderly fashion when shutdown. The node will be cleaned up when this happens and can be bootstrapped safely again.

### Debugging

`ipfs-cluster-service` offers two debugging options:

* `--debug` enables debug logging from the `ipfs-cluster`, `go-libp2p-raft` and `go-libp2p-rpc` layers. This will be a very verbose log output, but at the same time it is the most informative.
* `--loglevel` sets the log level (`[error, warning, info, debug]`) for the `ipfs-cluster` only, allowing to get an overview of the what cluster is doing. The default log-level is `info`.
