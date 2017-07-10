# `ipfs-cluster-service`

> IPFS cluster peer launcher

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

### Cluster secret

As long as the `CLUSTER_SECRET` environment variable is not set, you will be
prompted to enter a `cluster_secret` value for your configuration. Two peers
will be in the same cluster if and only if they share the same `cluster_secret`
value. When initializing a cluster service, the following conditions are checked
(in the order shown) and determine how `cluster_secret` is set:

1.  If the `--gen-secret` flag is passed to `ipfs-cluster-service init`, then a
    `cluster_secret` value will be automatically generated.
2.  If the `CLUSTER_SECRET` environment variable is set in your current shell
    instance, its value will be read and used as the `cluster_service` value.
3.  If neither of the above conditions were ture, then you will be prompted to
    enter a `cluster_secret` during the initialization.

The `cluster_secret` must be a 64-character string with only hexadecimal
characters (`[0-9a-f]`).

TODO: Explain adding peers/sharing cluster secret once that functionality is
implemented.

### Configuration

After initialization, the configuration will be placed in `~/.ipfs-cluster/service.json` by default.

You can add the multiaddresses for the other cluster peers the `bootstrap` variable. For example, here is a valid configuration for a single-peer cluster:

```json
{
    "id": "QmXMhZ53zAoes8TYbKGn3rnm5nfWs5Wdu41Fhhfw9XmM5A",
    "private_key": "<redacted>",
    "cluster_secret": "<redacted>",
    "cluster_peers": [],
    "bootstrap": [],
    "leave_on_shutdown": false,
    "cluster_multiaddress": "/ip4/0.0.0.0/tcp/9096",
    "api_listen_multiaddress": "/ip4/127.0.0.1/tcp/9094",
    "ipfs_proxy_listen_multiaddress": "/ip4/127.0.0.1/tcp/9095",
    "ipfs_node_multiaddress": "/ip4/127.0.0.1/tcp/5001",
    "consensus_data_folder": "/home/user/.ipfs-cluster/data",
    "state_sync_seconds": 60,
    "replication_factor": -1,
    "monitoring_interval_seconds": 15
}
```

The configuration file should probably be identical among all cluster peers, except for the `id` and `private_key` fields. Once every cluster peer has the configuration in place, you can run `ipfs-cluster-service` to start the cluster. See the [additional docs](#additional-docs) section for detailed documentation on how to build a cluster.

#### Clusters using `cluster_peers`

The `cluster_peers` configuration variable holds a list of current cluster members. If you know the members of the cluster in advance, or you want to start a cluster fully in parallel, set `cluster_peers` in all configurations so that every peer knows the rest upon boot. Leave `bootstrap` empty. A cluster peer address looks like: `/ip4/1.2.3.4/tcp/9096/<id>`.

#### Clusters using `bootstrap`

When the `cluster_peers` variable is empty, the multiaddresses `bootstrap` can be used to have a peer join an existing cluster. The peer will contact those addresses (in order) until one of them succeeds in joining it to the cluster. When the peer is shut down, it will save the current cluster peers in the `cluster_peers` configuration variable for future use.

Bootstrap is a convenient method, but more prone to errors than `cluster_peers`. It can be used as well with `ipfs-cluster-service --bootstrap <multiaddress>`. Note that bootstrapping nodes with an old state (or diverging state) from the one running in the cluster may lead to problems with
the consensus, so usually you would want to bootstrap blank nodes.

### Debugging

`ipfs-cluster-service` offers two debugging options:

* `--debug` enables debug logging from the `ipfs-cluster`, `go-libp2p-raft` and `go-libp2p-rpc` layers. This will be a very verbose log output, but at the same time it is the most informative.
* `--loglevel` sets the log level (`[error, warning, info, debug]`) for the `ipfs-cluster` only, allowing to get an overview of the what cluster is doing. The default log-level is `info`.
