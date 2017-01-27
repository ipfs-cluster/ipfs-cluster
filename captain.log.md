# IPFS Cluster - Captain's log

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
