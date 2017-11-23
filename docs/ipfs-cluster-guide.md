# A guide to running ipfs-cluster

Revised for version `0.3.0`.

## Index

0. Introduction, definitions and other useful documentation
1. Installation and deployment
2. The configuration file
3. Starting your cluster peers
4. The consensus algorithm
5. The shared state, the local state and the ipfs state
6. Static cluster membership considerations
7. Dynamic cluster membership considerations
8. Pinning an item
9. Unpinning an item
10. Cluster monitoring and pin failover
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
* [The "How to Build and Update a Cluster" guide](../HOWTO_build_and_update_a_cluster.md)
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

* 1. Download the repository and run `make install`.
* 2. Run the [docker ipfs/ipfs-cluster container](https://hub.docker.com/r/ipfs/ipfs-cluster/). The container runs `ipfs-cluster-service`.
* 3. Download pre-built binaries for your platform at [dist.ipfs.io](https://dist.ipfs.io). Note that we test on Linux and ARM. We're happy to hear if other platforms are working or not.
* 4. Install from the [snapcraft.io](https://snapcraft.io) store: `sudo snap install ipfs-cluster --edge`. Note that there is no stable snap yet.

You can deploy cluster in the way which fits you best, as both ipfs and ipfs-cluster have no dependencies. There are some [Ansible roles](https://github.com/hsanjuan/ansible-ipfs-cluster) available to help you.

We try to keep `master` stable. It will always have the latest bugfixes but it may also incorporate behaviour changes without warning. The pre-built binaries are only provided for tagged versions. They are stable builds to the best of our ability, but they may still contain serious bugs which have only been fixed in `master`. It is recommended to check the [`CHANGELOG`](../CHANGELOG.md) to get an overview of past changes, and any open [release-tagged issues](https://github.com/ipfs/ipfs-cluster/issues?utf8=%E2%9C%93&q=is%3Aissue%20label%3Arelease) to get an overview of what's coming in the next release.


## The configuration file

The ipfs-cluster configuration file is usually found at `~/.ipfs-cluster/service.json`. It holds all the configurable options for cluster and its different components. The configuration file is divided in sections. Each section represents a component. Each item inside the section represents an implementation of that component and contains specific options. For more information on components, check the [ipfs-cluster architecture overview](../architecture.md).

A default configuration file can be generated with `ipfs-cluster-service init`. It is recommended that you re-create the configuration file after an upgrade, to make sure that you are up to date with any new options. The `-c` option can be used to specify a different configuration folder path, and allows to create a default configuration in a temporary folder, which you can then compare with the existing one.

The `cluster` section of the configuration stores a `secret`: a 32 byte (hex-encoded) key which **must be shared by all cluster peers**. Using an empty key has security implications (see the Security section below). Using different keys will prevent different peers from talking to each other.

Each section of the configuration file and the options in it depend on their associated component. We offer here a quick reference of the configuration format:

```
{
  "cluster": {                                              // main cluster component configuration
    "id": "QmZyXksFG3vmLdAnmkXreMVZvxc4sNi1u21VxbRdNa2S1b", // peer ID
    "private_key": "<base64 representation of the key>",
    "secret": "<32-bit hex encoded secret>",
    "peers": [],                                            // List of peers' multiaddresses
    "bootstrap": [],                                        // List of bootstrap peers' multiaddresses
    "leave_on_shutdown": false,                             // Abandon cluster on shutdown
    "listen_multiaddress": "/ip4/0.0.0.0/tcp/9096",         // Cluster RPC listen
    "state_sync_interval": "1m0s",                          // Time between state syncs
    "ipfs_sync_interval": "2m10s",                          // Time between ipfs-state syncs
    "replication_factor": -1,                               // Replication factor. -1 == all
    "monitor_ping_interval": "15s"                          // Time between alive-pings. See cluster monitoring section
  },
  "consensus": {
    "raft": {
      "wait_for_leader_timeout": "15s",                     // How long to wait for a leader when there is none
      "network_timeout": "10s",                             // How long to wait before timing out a network operation
      "commit_retries": 1,                                  // How many retries should we make before giving up on a commit failure
      "commit_retry_delay": "200ms",                        // How long to wait between commit retries
      "heartbeat_timeout": "1s",                            // Here and below: Raft options.
      "election_timeout": "1s",                             // See https://godoc.org/github.com/hashicorp/raft#Config
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
      "listen_multiaddress": "/ip4/127.0.0.1/tcp/9094",     // API listen
      "ssl_cert_file": "path_to_certificate",               // Path to SSL public certificate. Unless absolute, relative to config folder
      "ssl_key_file": "path_to_key",                        // Path to SSL private key. Unless absolute, relative to config folder
      "read_timeout": "30s",                                // Here and below, timeoutes for network operations
      "read_header_timeout": "5s",
      "write_timeout": "1m0s",
      "idle_timeout": "2m0s",
      "basic_auth_credentials": [                           // Leave null for no-basic-auth
        "user": "pass"
      ]
    }
  },
  "ipfs_connector": {
    "ipfshttp": {
      "proxy_listen_multiaddress": "/ip4/127.0.0.1/tcp/9095", // ipfs-proxy listen address
      "node_multiaddress": "/ip4/127.0.0.1/tcp/5001",         // ipfs-node api location
      "connect_swarms_delay": "7s",                           // after boot, how long to wait before trying to connect ipfs peers
      "proxy_read_timeout": "10m0s",                          // Here and below, timeouts for network operations
      "proxy_read_header_timeout": "5s",
      "proxy_write_timeout": "10m0s",
      "proxy_idle_timeout": "1m0s"
    }
  },
  "monitor": {
    "monbasic": {
      "check_interval": "15s"                                 // How often to check for expired alerts. See cluster monitoring section
    }
  },
  "informer": {
    "disk": {                                                 // Used when using the disk informer (default)
      "metric_ttl": "30s",                                    // Amount of time this metric is valid. Will be polled at TTL/2.
      "metric_type": "freespace"                              // or "reposize": type of metric
    },
    "numpin": {                                               // Used when using the numpin informer
      "metric_ttl": "10s"                                     // Amount of time this metric is valid. Will be polled at TTL/2.
    }
  }
}
```

## Starting your cluster peers

`ipfs-cluster-service` will launch your cluster peer. If you have not configured any `cluster.peers` in the configuration, nor any `cluster.bootstrap` addresses, a single-peer cluster will be launched.

When filling in `peers` with some other peers' listening multiaddresses (i.e. `/ip4/192.168.1.103/tcp/9096/ipfs/QmQHKLBXfS7hf8o2acj7FGADoJDLat3UazucbHrgxqisim`), the peer will expect to be part of a cluster with the given `peers`. On boot, it will wait to learn who is the leader (see `raft.wait_for_leader_timeout` option) and sync it's internal state up to the last known state before becoming `ready`.

If you are using the `peers` configuration value, then **it is very important that the `peers` configuration value in all cluster members is the same for all peers**: it should contain the multiaddresses for the other peers in the cluster. It may contain a peer's own multiaddress too (but it will be removed automatically). If `peers` is not correct for all peer members, your node might not start or misbehave in not obvious ways.

You are expected to start the majority of the nodes at the same time when using this method. If half of them are not started, they will fail to elect a cluster leader before `raft.wait_for_leader_timeout`. Then they will shut themselves down. If there are peers missing, the cluster will not be in a healthy state (error messages will be displayed). The cluster will operate, as long as a majority of peers is up.

Alternatively, you can use the `bootstrap` variable to provide one or several bootstrap peers. In short, bootstrapping will use the given peer to request the list of cluster peers and fill-in the `peers` variable automatically. The bootstrapped peer will be, in turn, added to the cluster and made known to every other existing (and connected peer). You can also launch several peers at once, as long as they are bootstrapping from the same already-running-peer. The `--bootstrap` flag allows to provide a bootsrapping peer directly when calling `ipfs-cluster-service`.

Use the `bootstrap` method only when the rest of the cluster is healthy and all current participating peers are running. If you need to, remove any unhealthy peers with `ipfs-cluster-ctl peers rm <pid>`. Bootstrapping peers should be in a `clean` state, that is, with no previous raft-data loaded. If they are not, remove or rename the `~/.ipfs-cluster/ipfs-cluster-data` folder.

Once a cluster is up, peers are expected to run continuously. You may need to stop a peer, or it may die due to external reasons. The restart-behaviour will depend on whether the peer has left the consensus:

* The *default* case - peer has not been removed and `cluster.leave_on_shutdown` is `false`: in this case the peer has not left the consensus peerset, and you may start the peer again normally. Do not manually update `cluster.peers`, even if other peers have been removed from the cluster.
* The *left the cluster* case - peer has been manually removed or `cluster.leave_on_shutdown` is `true`: in this case, unless the peer died, it has probably been removed from the consensus (you can check if it's missing from `ipfs-cluster-ctl peers ls` on a running peer). This will mean that the state of the peer has been cleaned up (see the Dynamic Cluster Membership considerations below), and the last known `cluster.peers` have been moved to `cluster.bootstrap`. When the peer is restarted, it will attempt to rejoin the cluster from which it was removed by using the addresses in `cluster.bootstrap`.

Remember that a clean peer bootstrapped to an existing cluster will always fetch the latest state. A shutdown-peer which did not leave the cluster will also catch up with the rest of peers after re-starting. See the next section for more information about the consensus algorithm used by ipfs-cluster.

If the startup initialization fails, `ipfs-cluster-service` will exit automatically after a few seconds. Pay attention to the INFO and ERROR messages during startup. When ipfs-cluster is ready, a message will indicate it along with a list of peers.


## The consensus algoritm

ipfs-cluster peers coordinate their state (the list of CIDs which are pinned, their peer allocations and replication factor) using a consensus algorithm called Raft.

Raft is used to commit log entries to a "distributed log" which every peer follows. Every "Pin" and "Unpin" requests are log entries in that log. When a peer receives a log "Pin" operation, it updates its local copy of the shared state to indicate that the CID is now pinned.

In order to work, Raft elects a cluster "Leader", which is the only peer allowed to commit entries to the log. Thus, a Leader election can only succeed if at least half of the nodes are online. Log entries, and other parts of ipfs-cluster functionality (initialization, monitoring), can only happen when a Leader exists.

For example, a commit operation to the log is triggered with  `ipfs-cluster-ctl pin add <cid>`. This will use the peer's API to send a Pin request. The peer will in turn forward the request to the cluster's Leader, which will perform the commit of the operation. This is explained in more detail in the "Pinning an item" section.

The "peer add" and "peer remove" operations also trigger log entries (internal to Raft) and depend too on a healthy consensus status. Modifying the cluster peers is a tricky operation because it requires informing every peer of the new peer's multiaddresses. If a peer is down during this operation, the operation will fail, as otherwise that peer will not know how to contact the new member. Thus, it is recommended remove and bootstrap any peer that is offline before making changes to the peerset.

By default, the consensus log data is backed in the `ipfs-cluster-data` subfolder, next to the main configuration file. This folder stores two types of information: the **boltDB** database storing the Raft log, and the state snapshots. Snapshots from the log are performed regularly when the log grows too big (see the `raft` configuration section for options). When a peer is far behind in catching up with the log, Raft may opt to send a snapshot directly, rather than to send every log entry that makes up the state individually. This data is initialized on the first start of a cluster peer and maintained throughout its life. Removing or renaming the `ipfs-cluster-data` folder effectively resets the peer to a clean state. Only peers with a clean state should bootstrap to already running clusters.

When running a cluster peer, **it is very important that the consensus data folder does not contain any data from a different cluster setup**, or data from diverging logs. What this essentially means is that different Raft logs should not be mixed. Removing or renaming the `ipfs-cluster-data` folder, will clean all consensus data from the peer, but, as long as the rest of the cluster is running, it will recover last state upon start by fetching it from a different cluster peer.

On clean shutdowns, ipfs-cluster peers will save a human-readable state snapshot in `~/.ipfs-cluster/backups`, which can be used to inspect the last known state for that peer. We are working in making those snapshots restorable.


## The shared state, the local state and the ipfs state

It is important to understand that ipfs-cluster deals with three types of states:

* The **shared state** is maintained by the consensus algorithm and a copy is kept in every cluster peer. The shared state stores the list of CIDs which are tracked by ipfs-cluster, their allocations (peers which are pinning them) and their replication factor.
* The **local state** is maintained separately by every peer and represents the state of CIDs tracked by cluster for that specific peer: status in ipfs (pinned or not), modification time etc.
* The **ipfs state** is the actual state in ipfs (`ipfs pin ls`) which is maintained by the ipfs daemon.

In normal operation, all three states are in sync, as updates to the *shared state* cascade to the local and the ipfs states. Additionally, syncing operations are regularly triggered by ipfs-cluster. Unpinning cluster-pinned items directly from ipfs will, for example, cause a mismatch between the local and the ipfs state. Luckily, there are ways to inspect every state:


* `ipfs-cluster-ctl pin ls` shows information about the *shared state*. The result of this command is produced locally, directly from the state copy stored the peer.

* `ipfs-cluster-ctl status` shows information about the *local state* in every cluster peer. It does so by aggregating local state information received from every cluster member.

`ipfs-cluster-ctl sync` makes sure that the *local state* matches the *ipfs state*. In other words, it makes sure that what cluster expects to be pinned is actually pinned in ipfs. As mentioned, this also happens automatically. Every sync operations triggers an `ipfs pin ls --type=recursive` call to the local node.

Depending on the size of your pinset, you may adjust the interval between the different sync operations using the `cluster.state_sync_interval` and `cluster.ipfs_sync_interval` configuration options.

As a final note, the *local state* may show items in *error*. This happens when an item took too long to pin/unpin, or the ipfs daemon became unavailable. `ipfs-cluster-ctl recover <cid>` can be used to rescue these items. See the "Pinning an item" section below for more information.


## Static cluster membership considerations

We call a static cluster, that in which the set of `cluster.peers` is fixed, where "peer add"/"peer rm"/bootstrapping operations don't happen (at least normally) and where every cluster member is expected to be running all the time.

Static clusters are a way to run ipfs-cluster in a stable fashion, since the membership of the consensus remains unchanged, they don't suffer the dangers of dynamic peer sets, where it is important that operations modifying the peer set succeed for every cluster member.

Static clusters expect every member peer to be up and responding. Otherwise, the Leader will detect missing heartbeats and start logging errors. When a peer is not responding, ipfs-cluster will detect that a peer is down and re-allocate any content pinned by that peer to other peers. ipfs-cluster will still work as long as there is a Leader (half of the peers are still running). In the case of a network split, or if a majority of nodes is down, cluster will be unable to commit any operations the the log and thus, it's functionality will be limited to read operations.

## Dynamic cluster membership considerations

We call a dynamic cluster, that in which the set of `cluster.peers` changes. Nodes are bootstrapped to existing cluster peers (`cluster.bootstrap` option), the "peer rm" operation is used and/or the `cluster.leave_on_shutdown` configuration option is enabled. This option allows a node to abandon the consensus membership when shutting down. Thus reducing the cluster size by one.

Dynamic clusters allow greater flexibility at the cost of stablity. Leave and, specially, join operations are tricky as they change the consensus membership. They are likely to fail in unhealthy clusters. All operations modifying the peerset require an elected and working leader. Also, bear in mind that removing a peer from the cluster will trigger a re-allocation of the pins that were associated to it. If the replication factor was 1, it is recommended to keep the ipfs daemon running so the content can actually be copied out to a daemon managed by a different peer.

Peers joining an existing cluster should not have any consensus state (contents in `./ipfs-cluster/ipfs-cluster-data`). Peers leaving a cluster are not expected to re-join it with stale consensus data. For this reason, **the consensus data folder is renamed** when a peer leaves the current cluster. For example, `ipfs-cluster-data` becomes `ipfs-cluster-data.old.0` and so on. Currently, up to 5 copies of the cluster data will be left around, with `old.0` being the most recent, and `old.4` the oldest.

When a peer leaves or is removed, any existing peers will be saved as `bootstrap` peers, so that it is easier to re-join the cluster by simply re-launching it. Since the state has been cleaned, the peer will be able to re-join and fetch the latest state cleanly. See "The consensus algorithm" and the "Starting your cluster peers" sections above for more information.

This does not mean that there are not possibilities of somehow getting a broken cluster membership. The best way to diagnose it and fix it is to:

* Select a healthy node
* Run `ipfs-cluster-ctl peers ls`
* Examine carefully the results and any errors
* Run `ipfs-cluster-ctl peers rm <peer in error>` for every peer not responding
* If the peer count is different depending on the peers responding, remove those peers too. Once stopped, remove the consensus data folder and bootstrap them to a healthy cluster peer. Always make sure to keep 1/2+1 peers alive and healthy.
* `ipfs-cluster-ctl --enc=json peers ls` provides additional useful information, like the list of peers for every responding peer.
* In cases were leadership has been lost beyond solution (meaning faulty peers cannot be removed), it is best to stop all peers and restore the state from the backup (currently, a manual operation).

Remember: if you have a problematic cluster peer trying to join an otherwise working cluster, the safest way is to rename the `ipfs-cluster-data` folder (keeping it as backup) and to set the correct `bootstrap`. The consensus algorithm will then resend the state from scratch.

ipfs-cluster will fail to start if `cluster.peers` do not match the current Raft peerset. If the current Raft peerset is correct, you can manually update `cluster.peers`. Otherwise, it is easier to clean and bootstrap.

Finally, note that when bootstrapping a peer to an existing cluster, **the new peer must be configured with the same `cluster.secret` as the rest of the cluster**.



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


## Cluster monitoring and pin failover

ipfs-cluster includes a basic monitoring component which gathers metrics and triggers alerts when a metric is no longer renewed. There are currently two types of metrics:

* `informer` metrics are used to decide on allocations when a pin request arrives. Different "informers" can be configured. The default is the disk informer using the `freespace` metric.
* a `ping` metric is used to signal that a peer is alive.

Every metric carries a Time-To-Live associated with it. This TTL can be configued in the `informers` configuration section. The `ping` metric TTL is determined by the `cluster.monitoring_ping_interval`, and is equal to 2x its value.

Every ipfs-cluster peers push metrics to the cluster Leader regularly. This happens TTL/2 intervals for the `informer` metrics and in `cluster.monitoring_ping_interval` for the `ping` metrics.

When a metric for an existing cluster peer stops arriving and previous metrics have outlived their Time-To-Live, the monitoring component triggers an alert for that metric. `monbasic.check_interval` determines how often the monitoring component checks for expired TTLs and sends these alerts. If you wish to detect expired metrics more quickly, decrease this interval. Otherwise, increase it.

ipfs-cluster will react to `ping` metrics alerts by searching for pins allocated to the alerting peer and triggering re-pinning requests for them.

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

Note that **this feature has not been extensively tested**, but we aim to introduce improvements and fully support it in the mid-term.


## Security

ipfs-cluster peers communicate with each other using libp2p-encrypted streams (`secio`), with the ipfs daemon using plain http, provide an HTTP API themselves (used by `ipfs-cluster-ctl`) and an IPFS Proxy. This means that there are four endpoints to be wary about when thinking of security:

* `cluster.listen_multiaddress`, defaults to `/ip4/0.0.0.0/tcp/9096` and is the listening address to communicate with other peers (via Remote RPC calls mostly). These endpoints are protected by the `cluster.secret` value specified in the configuration. Only peers holding the same secret can communicate between each other. If the secret is empty, then **nothing prevents anyone from sending RPC commands to the cluster RPC endpoint** and thus, controlling the cluster and the ipfs daemon (at least when it comes to pin/unpin/pin ls and swarm connect operations. ipfs-cluster administrators should therefore be careful keep this endpoint unaccessible to third-parties when no `cluster.secret` is set.
* `restapi.listen_multiaddress`, defaults to `/ip4/127.0.0.1/tcp/9094` and is the listening address for the HTTP API that is used by `ipfs-cluster-ctl`. The considerations for `restapi.listen_multiaddress` are the same as for `cluster.listen_multiaddress`, as access to this endpoint allows to control ipfs-cluster and the ipfs daemon to a extent. By default, this endpoint listens on locahost which means it can only be used by `ipfs-cluster-ctl` running in the same host. The REST API component provides HTTPS support for this endpoint, along with Basic Authentication. These can be used to protect an exposed API endpoint.
* `ipfshttp.proxy_listen_multiaddress` defaults to `/ip4/127.0.0.1/tcp/9095`. As explained before, this endpoint offers control of ipfs-cluster pin/unpin operations and access to the underlying ipfs daemon. This endpoint should be treated with at least the same precautions as the ipfs HTTP API.
* `ipfshttp.node_multiaddress` defaults to `/ip4/127.0.0.1/tcp/5001` and contains the address of the ipfs daemon HTTP API. The recommendation is running IPFS on the same host as ipfs-cluster. This way it is not necessary to make ipfs API listen on other than localhost.


## Upgrading

The safest way to upgrade ipfs-cluster is to stop all cluster peers, update and restart them.

As long as the *shared state* format has not changed, there is nothing preventing from stopping cluster peers separately, updating and launching them.

When the shared state format has changed, a state migration will be required. ipfs-cluster will refuse to start and inform the user in this case.  Currently state migrations are supported in one direction, from old state formats to the format used by the updated ipfs-cluster-service.  This is accomplished by stopping the ipfs-cluster-service daemon and running `ipfs-cluster-service state upgrade`.  Note that due to changes in state serialization introduced while implementing state migrations ipfs-cluster shared state saved before December 2017 can not be migrated with this method.

The upgrading procedures is something which is actively worked on and will improve over time.


## Troubleshooting and getting help

### Have a question, idea or suggestion?

Open an issue on [ipfs-cluster](https://github.com/ipfs/ipfs-cluster) or ask on [discuss.ipfs.io](https://discuss.ipfs.io).

If you want to collaborate in ipfs-cluster, look at the list of open issues. Many are conveniently marked with [HELP WANTED](https://github.com/ipfs/ipfs-cluster/issues?q=is%3Aissue+is%3Aopen+label%3A%22help+wanted%22) and organized by difficulty. If in doubt, just ask!

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
* Is the `cluster.secret` the same for all peers?
* Double-check that the addresses in `cluster.peers` and `cluster.bootstrap` are correct.
* Double-check that the rest of the cluster is in a healthy state.
* In some cases, it may help to delete everything in the consensus data folder (specially if the reason for not starting is a mismatch between the raft state and the cluster peers). Assuming that the cluster is healthy, this will allow the non-starting peer to pull a clean state from the cluster Leader when bootstrapping.


### Peer stopped unexpectedly

When a peer stops unexpectedly:

* Make sure you simply haven't removed the peer from the cluster or triggered a shutdown
* Check the logs for any clues that the process died because of an internal fault
* Check your system logs to find if anything external killed the process
* Report any application panics, as they should not happen, along with the logs


### `ipfs-cluster-ctl status <cid>` does not report CID information for all peers

This is usually the result of a desync between the *shared state* and the *local state*, or between the *local state* and the ipfs state. If the problem does not autocorrect itself after a couple of minutes (thanks to auto-syncing), try running `ipfs-cluster-ctl sync [cid]` for the problematic item. You can also restart your node.

### libp2p errors

Since cluster is built on top of libp2p, many errors that new users face come from libp2p and have confusing messages which are not obvious at first sight. This list compiles some of them:

* `dial attempt failed: misdial to <peer.ID XXXXXX> through ....`: this means that the multiaddress you are contacting has a different peer in it than expected.
* `dial attempt failed: context deadline exceeded`: this means that the address is not reachable.
* `dial attempt failed: incoming message was too large`: this probably means that your cluster peers are not sharing the same secret.
* `version not supported`: this means that your nodes are running different versions of raft/cluster.
