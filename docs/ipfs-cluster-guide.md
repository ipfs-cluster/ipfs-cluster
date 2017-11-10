# A guide to running ipfs-cluster

Revised for version `0.0.12`.

## Index

0. Introduction, definitions and other useful documentation
1. Installation and deployment
2. The configuration file
3. Starting your cluster
4. The consensus algorithm
5. The shared state, the local state and the ipfs state
6. Static cluster membership considerations
7. Dynamic cluster membership considerations
8. Pinning an item
9. Unpinning an item
10. Cluster monitoring and failover
11. Using the IPFS-proxy
12. Composite clusters
13. Security
14. Upgrading
15. Troubleshooting and getting help


## Introduction, definitions and other useful documentation

This guide aims to collect useful considerations and information for running an ipfs-cluster deployment. It provides extended information and expands on that provided by other docs, which are worth checking too:

* [The main README](../README.md)
* [The `ipfs-cluster-service` README](../ipfs-cluster-service/dist/README.md)
* [The `ipfs-cluster-ctl` README](../ipfs-cluster-ctl/dist/README.md)
* [The How to Build and Update Guide](../HOWTO_build_and_update_a_cluster.md)
* [ipfs-cluster architecture overview](../architecture.md)
* [Godoc documentation for ipfs-cluster](https://godoc.org/github.com/ipfs/ipfs-cluster)

### Definitions

* ipfs-cluster: the software as a whole, usually referring to the peer which is run by `ipfs-cluster-service`.
* ipfs: the ipfs daemon, usually `go-ipfs`.
* peer: a member of the cluster.
* CID: Content IDentifier. The hash that identifies an ipfs object and which is pinned in cluster.
* Pin: A CID which is tracked by cluster and "pinned" in the underlying ipfs daemons.
* healty state: An ipfs-cluster is in healthy state when all peers are up.


## Installation and deployment

There are several ways to install ipfs-cluster. They are described in the main README and summarized here:

* Download the repository and run `make install`.
* Run the [docker ipfs/ipfs-cluster container](https://hub.docker.com/r/ipfs/ipfs-cluster/). The container includes and runs ipfs.
* Download pre-built binaries for your platform at [dist.ipfs.io](https://dist.ipfs.io). Note that we test on Linux and ARM. We're happy to hear if other platforms are working or not. These builds may be slightly outdated compared to Docker.
* Install from the [snapcraft.io](https://snapcraft.io) store: `sudo snap install ipfs-cluster --edge`. Note that there is no stable snap yet.

You can deploy cluster in the way which fits you best, as both ipfs and ipfs-cluster have no dependencies. There are some [Ansible roles](https://github.com/hsanjuan/ansible-ipfs-cluster) available to help you.


## The configuration file

The ipfs-cluster configuration file is usually found at `~/.ipfs-cluster/service.json`. It holds all the configurable options for cluster and its different components.

A default configuration file can be generated with `ipfs-cluster-service init`. It is recommended that you re-create the configuration file after an upgrade, to make sure that you are up to date with any new options. The `-c` option to specify a different configuration folder path allows to create a default configuration in a temporary folder, which you can then compare with the existing one.

The `cluster` section of the configuration stores a `secret`: a 32 byte (hex-encoded) key which **must be shared by all cluster peers**. Using an empty key has security implications (see the Security section). Using different keys will prevent different peers from talking to each other.

Each section of the configuration file and the options in it depend on their associated component. We offer here a quick reference of the configuration format:

```json
{
  "cluster": { // main cluster component configuration
    "id": "QmZyXksFG3vmLdAnmkXreMVZvxc4sNi1u21VxbRdNa2S1b", // peer ID
    "private_key": "<base64 representation of the key>",
    "secret": "<32-bit hex encoded secret>",
    "peers": [], // List of peers' multiaddresses
    "bootstrap": [], // List of bootstrap peers' multiaddresses
    "leave_on_shutdown": false, // Abandon consensus on exit
    "listen_multiaddress": "/ip4/0.0.0.0/tcp/9096", // Cluster RPC listen
    "state_sync_interval": "1m0s", // Time between state syncs
    "ipfs_sync_interval": "2m10s", // Time between ipfs-state syncs
    "replication_factor": -1, // Replication factor. -1 == all
    "monitor_ping_interval": "15s" // Time between alive-pings
  },
  "consensus": {
    "raft": { // Raft options, see hashicorp/raft.Configuration
      "data_folder": "<custom consensus data folder>"
      "heartbeat_timeout": "1s",
      "election_timeout": "1s",
      "commit_timeout": "50ms",
      "max_append_entries": 64,
      "trailing_logs": 10240,
      "snapshot_interval": "2m0s",
      "snapshot_threshold": 8192,
      "leader_lease_timeout": "500ms"
    }
  },
  "api": {
    "restapi": {
      "listen_multiaddress": "/ip4/127.0.0.1/tcp/9094", // API listen
      "read_timeout": "30s",
      "read_header_timeout": "5s",
      "write_timeout": "1m0s",
      "idle_timeout": "2m0s",
      "basic_auth_credentials": [ // leave null for no-basic-auth
        "user": "pass"
      ]
    }
  },
  "ipfs_connector": {
    "ipfshttp": {
      "proxy_listen_multiaddress": "/ip4/127.0.0.1/tcp/9095", // ipfs-proxy
      "node_multiaddress": "/ip4/127.0.0.1/tcp/5001", // ipfs-node location
      "connect_swarms_delay": "7s",
      "proxy_read_timeout": "10m0s",
      "proxy_read_header_timeout": "5s",
      "proxy_write_timeout": "10m0s",
      "proxy_idle_timeout": "1m0s"
    }
  },
  "monitor": {
    "monbasic": {
      "check_interval": "15s" // how often to check for expired metrics/alert
    }
  },
  "informer": {
    "disk": {
      "metric_ttl": "30s", // determines how often metric is updated
      "metric_type": "freespace" // or "reposize"
    },
    "numpin": {
      "metric_ttl": "10s"
    }
  }
}
```

## Starting your cluster

`ipfs-cluster-service` will launch your cluster peer. If you have not configured any `cluster.peers` in the configuration, nor any `cluster.bootstrap` addresses, a single-peer cluster will be launched.

When filling in `peers` with some other peers' listening multiaddresses (i.e. `/ip4/192.168.1.103/tcp/9096/ipfs/QmQHKLBXfS7hf8o2acj7FGADoJDLat3UazucbHrgxqisim`), the initialization process will consist in joining the current cluster's consensus, waiting for a leader (either to learn or to elect one), and syncing up to the last known state.

If you are using the `peers` configuration value, then **it is very important that the `peers` configuration value in all cluster members has the same value.** If not, your node will misbehave in not obvious ways. You can start all nodes at once when using this method. If they are not started at once, a node will be missing from the cluster, and since other peers expect it to be online, the cluster will not be in a healthy state (although it will operate if at least half of the peers are running).

Alternatively, you can use the `bootstrap` variable to provide one or several bootstrap peers. Bootstrapping will use the given peer to request the list of cluster peers and fill-in the `peers` variable automatically. The bootstrapped peer will be, in turn, added to the cluster and made known to every other existing (and connected peer).

Use the `bootstrap` method only when the rest of the cluster is healthy and all peers are running. You can also launch several peers at once, as long as they are bootstrapping from the same already-running-peer. The `--bootstrap` flag allows to provide a bootsrapping peer directly when calling `ipfs-cluster-service`.

If the startup initialization fails, `ipfs-cluster-service` will exit automatically (after 30 seconds). Pay attention to the INFO messages during startup. When ipfs-cluster is ready, a message will indicate it along with a list of peers.


## The consensus algoritm

ipfs-cluster peers coordinate their state (the list of CIDs which are pinned, their peer allocations and replication factor) using a consensus algorithm called Raft.

Raft is used to commit log entries to a "distributed log" which every peer follows. Every "Pin" and "Unpin" requests are log entries in that log. When a peer receives a log "Pin" operation, it updates its local copy of the shared state to indicate that the CID is now pinned.

In order to work, Raft elects a cluster "Leader", which is the only peer allowed to commit entries to the log. Thus, a Leader election can only succeed if at least half of the nodes are online. Log entries, and other parts of ipfs-cluster functionality (initialization, monitoring), can only happen when a Leader exists.

For example, a commit operation to the log is triggered with  `ipfs-cluster-ctl pin add <cid>`. This will use the peer's API to send a Pin request. The peer will in turn forward the request to the cluster's Leader, which will perform the commit of the operation. This is explained in more detail in the "Pinning an item" section.

The "peer add" and "peer remove" operations also trigger log entries and fully depend on a healthy consensus status. Modifying the cluster peers is a tricky operation because it requires informing every peer of the new peer set. If a peer is down during this operation, it is likely that it will not learn about it when it comes up again. Thus, it is recommended to bootstrap the peer again when the cluster has changed in the meantime.

By default the consensus log data is backed in the `ipfs-cluster-data` subfolder, next to the main configuration file. This folder stores two types of information: the [boltDB] database storing the Raft log, and the state snapshots. Snapshots from the log are performed regularly when the log grows too big (or on shutdown). When a peer is far behind in catching up with the log, Raft may opt to send a snapshot directly, rather than to send every log entry that make up the state individually.

When running a cluster peer, **it is very important that the consensus data folder does not contain any data from a different cluster setup**, or data from diverging logs. What this essentially means is that different Raft logs should not be mixed. Removing the `ipfs-cluster-data` folder, will destroy all consensus data from the peer, but, as long as the rest of the cluster is running, it will recover last state upon start by fetching it from a different cluster peer.

On clean shutdowns, ipfs-cluster peers will save a human-readable state snapshot in the `~/.ipfs-cluster/backups` folder, which can be used to inspect the last known state for that peer.


## The shared state, the local state and the ipfs state

It is important to understand that ipfs-cluster deals with three types of states:

* The **shared state** is maintained by the consensus algorithm and a copy is kept in every cluster peer. The shared state stores the list of CIDs which are tracked by ipfs-cluster, their allocations (peers which are pinning them) and their replication factor.
* The **local state** is maintained separately by every peer and represents the state of CIDs tracked by cluster for that specific peer: status in ipfs (pinned or not), modification time etc.
* The **ipfs state** is the actual state in ipfs (`ipfs pin ls`) which is maintained by the ipfs daemon.

In normal operation, all three states are in sync, as updates to the *shared state* cascade to the local and the ipfs states. Additionally, syncing operations are regularly triggered by ipfs-cluster. Unpinning cluster-pinned items directly from ipfs will, for example, cause a mismatch between the local and the ipfs state. Luckily, there are ways to inspect every state:


* `ipfs-cluster-ctl pin ls` shows information about the *shared state*. The result of this command is produced locally, directly from the state copy stored the peer.

* `ipfs-cluster-ctl status` shows information about the *local state* in every cluster peer. It does so by aggregating local state information received from every cluster member.

`ipfs-cluster-ctl sync` makes sure that the *local state* matches the *ipfs state*. In other words, it makes sure that what cluster expects to be pinned is actually pinned in ipfs. As mentioned, this also happens automatically. Every sync operations triggers an `ipfs pin ls --type=recursive` call to the local node. Depending on the size of your pinset, you may adjust the interval between sync operations using the `ipfs_sync_seconds` configuration variable.


## Static cluster membership considerations

We call a static cluster, that in which the set of `cluster.peers` is fixed, where "peer add"/"peer rm"/bootstrapping operations don't happen (at least normally) and where every cluster member is expected to be running all the time.

Static clusters are a way to run ipfs-cluster in a stable fashion, since the membership of the consensus remains unchanged, they don't suffer the dangers of dynamic peer sets, where it is important that operations modifying the peer set suceed for every cluster member.

Static clusters expect every member peer to be up and responding. Otherwise, the Leader will detect missing heartbeats start logging errors. When a peer is not responding, ipfs-cluster will detect that a peer is down and re-allocate any content pinned by that peer to other peers. ipfs-cluster will still work as long as there is a Leader (half of the peers are still running). In the case of a network split, or if a majority of nodes is down, cluster will be unable to commit any operations the the log and thus, it's functionality will be limited to read operations.

## Dynamic cluster membership considerations

We call a dynamic cluster, that in which the set of `cluster.peers` changes. Nodes are bootstrapped to existing cluster peers, the "peer add" and "peer rm" operations are used and/or the `cluster.leave_on_shutdown` configuration option is enabled. This option allows a node to abandon the consensus membership when shutting down. Thus reducing the cluster size by one.

Dynamic clusters allow greater flexibility at the cost of stablity. Join and leave operations are tricky as they change the consensus membership and they are likely to create bad situations in unhealthy clusters. Also, bear in mind than removing a peer from the cluster will trigger a re-allocation of the pins that were associated to it. If the replication factor was 1, it is recommended to keep the ipfs daemon running so the content can actually be copied out to a daemon managed by a different peer.

Peers joining an existing cluster should have a non-divergent state. That means: their consensus `ipfs-cluster-data` folder should be empty or, if not, it should contain data belonging to the existing cluster. A joining peer should have not been running on its own separately from the running cluster. When a peer leaves or is removed, any existing peers will be saved as `bootstrap` peers, so that it is not easy to start the departing peer in "single-peer-mode" by mistake: that would essentially create a diverging state, preventing it to re-join its previous cluster cleanly.

The best way to diagnose and fix a broken cluster membership issue is to:

* Select a healthy node
* Run `ipfs-cluster-ctl peers ls`
* Examine carefully the results and any errors
* Run `ipfs-cluster-ctl peers rm <peer in error>` for every peer not responding
* If the peer count is different in the peers responding, identify peers with
wrong peer count, stop them, fix `cluster_peers` manually and restart them.
* `ipfs-cluster-ctl --enc=json peers ls` provides additional useful information, like the list of peers for every responding peer.
* In cases were no Leader can be elected, then manual stop and editing of `cluster.peers` is necessary.

If you have a problematic cluster peer trying to join an otherwise working cluster, the safest way is to remove the `ipfs-cluster-data` folder and to set the correct `bootstrap`. The consensus algorithm will then resend the state from scratch.

Note that when adding a peer to an existing cluster, **the new peer must be configured with the same `cluster.secret` as the rest of the cluster**.



## Pinning an item

`ipfs-cluster-ctl pin add <cid>` will tell ipfs-cluster to pin a CID.

This involves:

* Deciding which peers will be allocated the CID (that is, which cluster peers will ask ipfs to pin the CID). This depends on the replication factor and the allocation strategy.
* Forwarding the pin request to the Raft Leader.
* Commiting the pin entry to the log.
* *At this point, a success/failure is returned to the user, but ipfs-cluster has more things to do.*
* Receiving the log update and modifying the *shared state* accordingly.
* Updating the local state.
* If the peer has been allocated the content, then:
  * Queueing the pin request and setting the pin status to `PINNING`.
  * Triggering a pin operation
  * Waiting until it completes and setting the pin status to `PINNED`.

Errors in the first part of the process (before the entry is commited) will be returned to the user and the whole operation is aborted. Errors in the second part of the process will result in pins with an status of `PIN_ERROR`.

In order to check the status of a pin, use `ipfs-cluster-ctl status <cid>`. Retries for pins in error state can be triggered with `ipfs-cluster-ctl recover <cid>`.

The reason pins (and unpin) requests are queued is because ipfs only performs one pin at a time, while any other requests are hanging in the meantime. All in all, pinning items which are unavailable in the network may create significants bottlenecks (this is a problem that comes from ipfs), as the pin request takes very long to time out. Facing this problem involves restarting the ipfs node.


## Unpinning an item

`ipfs-cluster-ctl pin rm <cid>` will tell ipfs-cluster to unpin a CID.

The process is very similar to the "Pinning an item" described above. Removed pins are wiped from the shared and local states. When requesting the local `status` for a given CID, it will show as `UNPINNED`. Errors will be reflected as `UNPIN_ERROR` in the pin local status.


## Cluster monitoring and failover

ipfs-cluster includes a basic monitoring component which gathers metrics and triggers alerts when a metric is no longer renewed. There are currently two types of metrics:

* `informer` metrics are used to decide on allocations when a pin request arrives. Different "informers" can be configured. The default is the disk informer using the `freespace` metric.
* a `ping` metric is used to monitor peers.

Every metric carries a Time-To-Live associated with it. ipfs-cluster peers pushes metrics from every peer to the cluster Leader in TTL/2 intervals. When a metric for an existing cluster peer stops arriving and previous metrics have outlived their Time-To-Live, the monitoring component triggers an alert for that metric.

ipfs-cluster will react to `ping` metrics alerts by searching for pins allocated to the alerting peer and triggering re-pinning requests for them. If the node is down, informer metrics are likely to be also invalid, causing the content to be given allocations from different peers.

The monitoring and failover system in cluster is very basic and requires improvements. Failover is likely to not work properly when several nodes go offline at once (specially if the current Leader is affected). Manual re-pinning can be triggered with `ipfs-cluster-ctl pin <cid>`. `ipfs-cluster-ctl pin ls <CID>` can be used to find out the current list of peers allocated to a CID.


## Using the IPFS-proxy

ipfs-cluster provides an proxy to ipfs (which by default listens on `/ip4/127.0.0.1/tcp/9095`). This allows ipfs-cluster to behave as if it was an ipfs node. It achieves this by intercepting the following requests:

* `/add`: the proxy adds the content to the local ipfs daemon and pins the resulting hash[es] in ipfs-cluster.
* `/pin/add`: the proxy pins the given CID in ipfs-cluster.
* `/pin/rm`: the proxy unpins the given CID from ipfs-cluster.
* `/pin/ls`: the proxy lists the pinned items in ipfs-cluster.

Responses from the proxy mimic ipfs daemon responses. This allows to use ipfs-cluster with the `ipfs` CLI as the following examples show:

* `ipfs --api /ip4/127.0.0.1/tcp/9095 pin add <cid>`
* `ipfs --api /ip4/127.0.0.1/tcp/9095 add myfile.txt`
* `ipfs --api /ip4/127.0.0.1/tcp/9095 pin rm <cid>`
* `ipfs --api /ip4/127.0.0.1/tcp/9095 pin ls`

Any other requests are directly forwarded to the ipfs daemon and responses and sent back from it.

Intercepted endpoints aim to mimic the format and response code from ipfs, but they may lack headers. If you encounter a problem where something works with ipfs but not with cluster, open an issue.


## Composite clusters

Since ipfs-cluster provides an IPFS Proxy (an endpoint that act likes an IPFS daemon), it is also possible to use an ipfs-cluster proxy endpoint as the `ipfs_node_multiaddress` for a different cluster.

This means that the top cluster will think that it is performing requests to an IPFS daemon, but it is instead using an ipfs-cluster peer which belongs to a sub-cluster.

This allows to scale ipfs-cluster deployments and provides a method for building ipfs-cluster topologies that may be better adapted to certain needs.

Note that **this feature has not been extensively tested**.


## Security

ipfs-cluster peers communicate which eachother using libp2p-encrypted streams (`secio`), with the ipfs daemon using plain http, provide an HTTP API themselves (used by `ipfs-cluster-ctl`) and an IPFS Proxy. This means that there are four endpoints to be wary about when thinking of security:

* `cluster_multiaddress`, defaults to `/ip4/0.0.0.0/tcp/9096` and is the listening address to communicate with other peers (via Remote RPC calls mostly). These endpoints are protected by the `cluster.secret` value specified in the configuration. Only peers holding the same secret can communicate between
each other. If the secret is empty, then **nothing prevents anyone from sending RPC commands to the cluster RPC endpoint** and thus, controlling the cluster and the ipfs daemon (at least when it comes to pin/unpin/pin ls and swarm connect operations. ipfs-cluster administrators should therefore be careful keep this endpoint unaccessible to third-parties when no `cluster.secret` is set.
* `api_listen_multiaddress`, defaults to `/ip4/127.0.0.1/tcp/9094` and is the listening address for the HTTP API that is used by `ipfs-cluster-ctl`. The considerations for `api_listen_multiaddress` are the same as for `cluster_multiaddress`, as access to this endpoint allows to control ipfs-cluster and the ipfs daemon to a extent. By default, this endpoint listens on locahost which means it can only be used by `ipfs-cluster-ctl` running in the same host.
* `ipfs_proxy_listen_multiaddress` defaults to `/ip4/127.0.0.1/tcp/9095`. As explained before, this endpoint offers control of ipfs-cluster pin/unpin operations and a full access to the underlying ipfs daemon. This endpoint should be treated with the same precautions as the ipfs HTTP API.
* `ipfs_node_multiaddress` defaults to `/ip4/127.0.0.1/tcp/5001` and contains the address of the ipfs daemon HTTP API. The recommendation is running IPFS on the same host as ipfs-cluster. This way it is not necessary to make ipfs API listen on other than localhost.


## Upgrading

The safest way to upgrade ipfs-cluster is to stop all cluster peers, update and restart them.

As long as the *shared state* format has not changed, there is nothing preventing from stopping cluster peers separately, updating and launching them.

When the shared state format has changed, a state migration will be required. ipfs-cluster will refuse to start and inform the user in this case, although this feature is yet to be implemented next time the state format changes. It will also mean a minor version bump (where normally only the patch version increases).


## Troubleshooting and getting help

### Have a question, idea or suggestion?

Open an issue on [ipfs-cluster](https://github.com/ipfs/ipfs-cluster) or ask on [discuss.ipfs.io](https://discuss.ipfs.io).

If you want to collaborate in ipfs-cluster, open an issue and ask about what you can do.

### Debugging

By default, `ipfs-cluster-service` prints only `INFO`, `WARNING` and `ERROR` messages. Sometimes, it is useful to increase verbosity with the `--loglevel debug` flag. This will make ipfs-cluster and its components much more verbose. The `--debug` flag will make ipfs-cluster, its components and its most prominent dependencies (raft, libp2p-raft, libp2p-gorpc) verbose.

`ipfs-cluster-ctl` offers a `--debug` flag which will print information about the API endpoints used by the tool. `--enc json` allows to print raw `json` responses from the API.

Interpreting debug information can be tricky. For example:

```
18:21:50.343 ERROR   ipfshttp: error getting:Get http://127.0.0.1:5001/api/v0/repo/stat: dial tcp 127.0.0.1:5001: getsockopt: connection refused ipfshttp.go:695
```

The above line shows a message of `ERROR` severity, coming from the `ipfshttp` facility. This facility corresponds to the `ipfshttp` module which implements the IPFS Connector component. This information helps narrowing the context from which the error comes from. The error message indicates that the component failed to perform a GET request to the ipfs HTTP API. The log entry contains the file and line-number in which the error was logged.

When discovering a problem, it will probably be useful if you can provide some logs when asking for help.


### Peer is not starting

When your peer is not starting:

* Check the logs and look for errors
* Are all the listen addresses free or are they used by a different process?
* Are other peers of the cluster reachable?
* Is the `cluster_secret` the same for all peers?
* Double-check that the addresses in `cluster_peers` are correct.
* Double-check that the rest of the cluster is in a healthy state.
* In some cases, it may help to delete everything in the `consensus_data_folder`. Assuming that the cluster is healthy, this will allow the non-starting peer to pull a clean state from the cluster Leader. Make a backup first, just in case.


### Peer stopped unexpectedly

When a peer stops unexpectedly:

* Check the logs for any clues that the process died because of an internal fault
* Check your system logs to find if anything external killed the process
* Report any application panics, as they should not happen, along with the logs.


### `ipfs-cluster-ctl status <cid>` does not report CID information for all peers

This is usually the result of a desync between the *shared state* and the *local state*, or between the *local state* and the ipfs state. If the problem does not autocorrect itself after a couple of minutes (thanks to auto-syncing), try running `ipfs-cluster-ctl sync [cid]` for the problematic item. You can also restart your node.
