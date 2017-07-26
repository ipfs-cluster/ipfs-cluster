# IPFS Cluster - Captain's log

## 20170726 | @hsanjuan

Unfortunately, I have not thought of updating the Captain's log for some months. The Coinlist effort has had me very busy, which means that my time and mind were not 100% focused on cluster. That said, there has been significant progress during this period. Much of that progress has happened thanks to @Zenground0 and @dgrisham, who have been working on cluster for most of Q2 making valuable contributions (many of them on the testing front).

As a summary, since my last update, we have:

* Added a [A guide to running IPFS Cluster](docs/ipfs-cluster-guide.md), with detailed information on how cluster works, what behaviours to expect and why. It should answer many questions which are not covered by the getting-started-quickly guides.
* Added sharness tests, which make sure that `ìpfs-cluster-ctl` and `ipfs-cluster-service` are tested and not broken in obvious ways at least.
* Pushed the [kubernetes-ipfs](https://github.com/ipfs/kubernetes-ipfs) project great lengths, adding a lot of DLS-language features and a bunch of highly advanced ipfs-cluster tests. The goal is to be able to test deployments layouts which are closer to reality, including escalability tests.
* The extra tests uncovered and allowed us to fix a number of nasty bugs, usually around the ipfs-cluster behaviour when peers go down or stop responding.
* Added CID re-allocation on peer removal.
* Added "Private Networks" support to ipfs-cluster. Private Networks is a libp2p feature which allows to secure a libp2p connection with a key. This means that inter-peer communication is now protected and isolated with a `cluster_secret`. This brings a significant reduction on the security pitfalls of running ipfs-cluster: default setup does not allow anymore remote control of a cluster peer. More information on security can be read on the [guide](docs/ipfs-cluster-guide.md).
* Added HTTPs support for the REST API endpoint. This facilitates exposing the API endpoints directly and is a necessary preamble to supporting basic authentication (in the works).

All the above changes are about to crystallize in the `v0.1.0` release, which we'll publish in the next days.


## 20170328 | @hsanjuan

The last weeks were spent on improving go-ipfs/libp2p/multiformats documentation as part of the [documentation sprint](https://github.com/ipfs/pm/issues/357) mentioned earlier.

That said, a few changes have made it to ipfs-cluster:

* All components have now been converted into submodules. This clarifies
the project layout and actually makes the component borders explicit.
* Increase pin performance. By using `type=recursive` in IPFS API queries
they return way faster.
* Connect different ipfs nodes in the cluster: we now trigger `swarm connect` operations for each ipfs node associated to a cluster peer, both at start up and
upon operations like `peer add`. This should ensure that ipfs nodes in the
cluster know each others.
* Add `disk` informer. The default allocation strategy now is based on how
big the IPFS repository is. Pins will be allocated to peers with lower
repository sizes.

I will be releasing new builds/release for ipfs-cluster in the following days.

## 20170310 | @hsanjuan

This week has been mostly spent on making IPFS Cluster easy to install, writing end-to-end tests as part of the Test Lab Sprint and bugfixing:

* There is now an ipfs-cluster docker container, which should be part of the ipfs docker hub very soon
* IPFS Cluster builds are about to appear in dist.ipfs.io
* I shall be publishing some Ansible roles to deploy ipfs+ipfs-cluster
* There are now some tests using [kubernetes-ipfs](https://github.com/ipfs/kubernetes-ipfs)with new docker container. These tests are the first automated tests that are truly end-to-end, using a real IPFS-daemon under the hood.
* I have added replication-factor-per-pin support. Which means that for every pinned item, it can be specified what it's replication factor should be, and this factor can be updated. This allows to override the global configuration option for replication factor.
* Bugfixes: one affecting re-pinning+replication and some others in ipfs-cluster-ctl output.

Next week will probably focus on the [Delightful documentation sprint](https://github.com/ipfs/pm/issues/357). I'll try to throw in some more tests for `ipfs-cluster-ctl` and will send the call for early testers that I was talking about in the last update, now that we have new multiple install options.

## 20170302 | @hsanjuan

IPFS cluster now has basic peer monitoring and re-pinning support when a cluster peer goes down.

This is done by broadcasting a "ping" from each peer to the monitor component. When it detects no pings are arriving from a current cluster member, it triggers an alert, which makes cluster trigger re-pins for all the CIDs associated to that peer.

The next days will be spent fixing small things and figuring out how to get better tests as part of the [Test Lab Sprint](https://github.com/ipfs/pm/issues/354). I also plan to make a call for early testers, to see if we can get some people on board to try IPFS Cluster out.

## 20170215 | @hsanjuan

A global replication factor is now supported! A new configuration file option `replication_factor` allows to specify how many peers should be allocated to pin a CID. `-1` means "Pin everywhere", and maintains compatibility with the previous behaviour. A replication factor >= 1 pin request is subjec to a number of requirements:

* It needs to not be allocated already. If it is the pin will return with an error saying so.
* It needs to find enough peers to pin.

How the peers are allocated content has been most of the work in this feature. We have two three componenets for doing so:

* An `Informer` component. Informer is used to fetch some metric (agnostic to Cluster). The metric has a Time-to-Live and it is pushed in TTL/2 intervals to the Cluster leader.
* An `Allocator` component. The allocator is used to provide an `Allocate()` method which, given current allocations, candidate peers and the last valid metrics pushed from the `Informers`, can decide which peers should perform the pinning. For example, a metric could be the used disk space in a cluster peer, and the allocation algorithm would be to sort candidate peers according to that metrics. The first in the list are the ones with less disk used, and will then be chosen to perform the pin. An `Allocator` could also work by receiving a location metric and making sure that the most preferential location is different from the already existing ones etc.
* A `PeerMonitor` component, which is in charge of logging metrics and providing the last valid ones. It will be extended in the future to detect peer failures and trigger alerts.

The current allocation strategy is a simple one called `numpin`, which just distributes the pins according to the number of CIDs peers are already pinning. More useful strategies should come in the future (help wanted!).

The next steps in Cluster will be wrapping up this milestone with failure detection and re-balancing.

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
