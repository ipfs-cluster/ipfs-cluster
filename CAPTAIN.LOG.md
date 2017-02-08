# IPFS Cluster - Captain's log

## 20170208 | @hsanjuan

So much for commitments... I missed last friday's log entry. The reason is that I was busy with the implementation of [dynamic membership for IPFS Cluster](https://github.com/ipfs/ipfs-cluster/milestone/2).

What seemed a rather simple task turned into a not so simple endeavour because modifying the peer set of Raft has a lot of pitfalls. This is specially if it is during boot (in order to bootstrap). A `peer add` operation implies making everyone aware of a new peer. In Raft this is achieved by commiting a special log entry. However there is no way to notify of such event on a receiver, and such entry only has the peer ID, not the full multiaddress of the new peer (needed so that other nodes can talk to it).

Therefore whoever adds the node must additionally broadcast the new node and also send back the full list of cluster peers to it. After three implementation attempts (all working but all improving on top of the previous), we perform this broadcasting by logging our own `PeerAdd` operation in Raft, with the multiaddress. This proved nicer and simpler than broadcasting to all the nodes (mostly on dealing with failures and errors - what do when a node has missed out). If the operation makes it to the log then everyone should get it, and if not, failure does not involve un-doing the operation in every node with another broadcast. The whole thing is still tricky when joining peers which have disjoint Raft states, so it is best to use it with clean, just started peers.

Same as `peer add`, there is a `join` operation which facilitates bootstrapping a node and have it directly join a cluster. On shut down, each node will save the current cluster peers in the configuration for future use. A `join` operation can be triggered with the `--bootstrap` flag in `ipfs-cluster-service` or with the `bootstrap` option in the configuration and works best with clean nodes.

The next days will be spent on implementing [replication factors](https://github.com/ipfs/ipfs-cluster/milestone/3), which implies the addition of new components to the mix.

## 20170127 | @hsanjuan

Friday is from now on the Captain Log entry day.

Last week, was the first week out of three the current ipfs-cluster *sprintino* (https://github.com/ipfs/pm/issues/353). The work has focused on addressing ["rough edges"](https://github.com/ipfs/ipfs-cluster/milestone/1), most of which came from @jbenet's feedback (#14). The result has been significant changes and improvements to ipfs-cluster:

* I finally nailed down the use of multicodecs in `go-libp2p-raft` and `go-libp2p-gorpc` and the whole dependency tree is now Gx'ed.
* It seems for the moment we have settled for `ipfs-cluster-service` and `ipfs-cluster-ctl` as names for the cluster tools.
* Configuration file has been overhauled. It now has explicit configuration key names and a stronger parser which will be more specific
on the causes of error.
* CLI tools have been rewritten to use `urfave/cli`, which means better help, clearer commands and more consistency.
* The `Sync()` operations, which update the Cluster pin states from the IPFS state have been rewritten. `Recover()` has been promoted
to its own endpoint.
* I have added `ID()` endpoint which provides information about the Cluster peer (ID, Addresses) and about the IPFS daemon it's connected to.
The `Peers()` endpoint retrieves this information from all Peers so it is easy to have a general overview of the Cluster.
* The IPFS proxy is now intercepting `pin add`, `pin rm` and `pin ls` and doing Cluster pinning operations instead. This not only allows
replacing an IPFS daemon by a Cluster peer, but also enables compositing cluster peers with other clusters (pointing `ipfs_node_multiaddress` to a different Cluster proxy endpoint).

The changes above include a large number of API renamings, re-writings and re-organization of the code, but ipfs-cluster has grown more solid as a result.

Next week, the work will focus on making it easy to [add and remove peers from a running cluster](https://github.com/ipfs/ipfs-cluster/milestone/2).

## 20170123 | @hsanjuan

I have just merged the initial cluster version into master. There are many rough edges to address, and significant changes to namings/APIs will happen during the next few days and weeks.

The rest of the quarter will be focused on 4 main issues:

* Simplify the process of adding and removing cluster peers
* Implement a replication-factor-based pinning strategy
* Generate real end to end tests
* Make ipfs-cluster stable

These endaevours will be reflected on the [ROADMAP](ROADMAP.md) file.
