# IPFS Cluster - Captain's log

## 20180125 | @hsanjuan

We are about to tag the `0.3.2` release and it comes with two nice features.

On one side, @zenground0 has been focused in implementing state offline export and import capabilities, a complement to the state upgrades added in the last release. They allow taking the shared from an offline cluster (and in a human readable format), and place it somewhere else, or in the same place. This feature might save the day in situations when the quorum of a cluster is completely lost and peers cannot be started anymore due to the lack of master.

Additionally, I have been putting some time into a new approach to replication factors. Instead of forcing cluster to store a pin an specific number of times, we now support a lower and upper bounds to the the replication factor (in the form of `replication_factor_min` and `replication_factor_max`). This feature (and great idea) was originally proposed by @segator.

Having this margin means that cluster will attempt its best when pinning an item (reach the max factor), but it won't error if it cannot find enough available peers, as long as it finds more than the minimum replication factor.

In the same way, a peer going offline, will not trigger a re-allocation of the CID as it did before, if the replication factor is still within the margin. This allows, for example, taking a peer offline for maintenance, without having cluster vacate all the pins associated to it (and then coming up empty).

Of course, the previous behaviour can still be obtained by setting both the max and the min to the same values.

Finally, it is very important to remark that we recently finished the [Sharding RFC draft](https://github.com/ipfs/ipfs-cluster/blob/master/docs/dag-sharding-rfc.md). This document outlines how we are going to approach the implementation of one of the most difficult but important features upcoming in cluster: the ability to distribute a single CID (tree) among several nodes. This will allow to use cluster to store files or archives too big for a single ipfs node. Input from the community on this draft can be provided at https://github.com/ipfs/notes/issues/278.

## 20171211 | @hsanjuan

During the last weeks we've been working hard on making the first "live" deployment of ipfs-cluster. I am happy to announce that a 10-peer cluster runs on ipfs-gateway nodes, maintaining a >2000-length pinset.

The nodes are distributed, run a vanilla ipfs-cluster docker container mounting a volume with a customized [cluster configuration](https://github.com/ipfs/infrastructure/blob/master/ipfs-cluster/service.json.tpl) which uses higher-than-default timeouts and intervals. The injection of the pin-set took a while, but enventually every pin in every node became PINNED. In one occassion, a single IPFS node hanged while pinning. After re-starting the IPFS node in question, all pins in the queue became PIN_ERRORs, but they could easily be fixed with a `recover` operation.

Additionally, the [IPFS IRC Pinbot](https://github.com/ipfs/pinbot-irc) now supports cluster-pinning, by using the ipfs-cluster proxy to ipfs, which intercepts pin requests and performs them in cluster. This allowed us to re-use the `go-ipfs-api` library to interact with cluster.

The first live setup has shown nevertheless that some things were missing. For example, we added `--local` flags to Sync, Status and Recover operations (and allowed a local RecoverAll). They are handy when a single node is at fault and you want to fix the pins on that specific node. We will also work on a `go-ipfs-cluster-api` library which provides a REST API client which allows to programatically interact with cluster more easily.

Parallel to all this, @zenground0 has been working on state migrations. The cluster's consensus state is stored on disk via snapshots in certain format. This format might evolve in the future and we need a way to migrate between versions without losing all the state data. In the new approach, we are able to extract the state from Raft snapshots, migrate it, and create a new snapshot with the new format so that the next time cluster starts everything works. This has been a complex feature but a very important step to providing a production grade release of ipfs-cluster.

Last but not least, the next release will include useful things like pin-names (a string associated to every pin) and peer names. This will allow to easily identify pins and peers by other than their multihash. They have been contributed by @te0d, who is working on https://github.com/te0d/js-ipfs-cluster-api, a JS Rest API client for our REST API, and https://github.com/te0d/bunker, a web interface to manage ipfs-cluster.

## 20171115 | @hsanjuan

This update comes as our `0.3.0` release is about to be published. This release includes quite a few bug fixes, but the main change is the upgrade of the underlying Raft libraries to a recently published version.

Raft 1.0.0 hardens the management of peersets and makes it more difficult to arrive to situations in which cluster peers have different, inconsistent states. These issues are usually very confusing for new users, as they manifest themselves with lots of error messages with apparently cryptic meanings, coming from Raft and LibP2P. We have embraced the new safeguards and made documentation and code changes to stress the workflows that should be followed when altering the cluster peerset. These can be summarized with:

* `--bootstrap` is the method to add a peer to a running cluster as it ensures that no diverging state exists during first boot.
* The `ipfs-cluster-data` folder is renamed whenever a peer leaves the cluster, resulting on a clean state for the next start. Peers with a dirty state will not be able to join a cluster.
* Whenever `ipfs-cluster-data` has been initialized, `cluster.peers` should match the internal peerset from the data, or the node will not start.

In the documentation, we have stressed the importance of the consensus data and described the workflows for starting peers and leaving the cluster in more detail.

I'm also happy to announce that we now build and publish "snaps". [Snaps](https://snapcraft.io/) are "universal Linux packages designed to be secure, sandboxed, containerised applications isolated from the underlying system and from other applications". We are still testing them. For the moment we publish a new snap on every master build.

You are welcome to check the [changelog](CHANGELOG.md) for a detailed list of other new features and bugfixes.

Our upcoming work will be focused on setting up a live ipfs-cluster and run it in a "production" fashion, as well as adding more capabilities to manage the internal cluster state while offline (migrate, export, import) etc.

## 20171023 | @hsanjuan

We have now started the final quarter of 2017 with renewed energy and plans for ipfs-cluster. The team has grown and come up with a set of priorities for the next weeks and months. The gist of these is:

* To make cluster stable and run it on production ourselves
* To start looking into the handling of "big datasets", including IPLD integration
* To provide users with a delightful experience with a focus in documentation and support

The `v0.2.0` marks the start of this cycle and includes. Check the [changelog](CHANGELOG.md) for a list of features and bugfixes. Among them, the new configuration options in the consensus component options will allow our users to experiment in environments with larger latencies than usual.

Finally, coming up in the pipeline we have:

* the upgrade of Raft library to v1.0.0, which is likely to provide a much better experience with dynamic-membership clusters.
* Swagger documentation for the Rest API.
* Work on connectivity graphs, allowing to easily spot any connectivity problem among cluster peers.

## 20170726 | @hsanjuan

Unfortunately, I have not thought of updating the Captain's log for some months. The Coinlist effort has had me very busy, which means that my time and mind were not fully focused on cluster as before. That said, there has been significant progress during this period. Much of that progress has happened thanks to @Zenground0 and @dgrisham, who have been working on cluster for most of Q2 making valuable contributions (many of them on the testing front).

As a summary, since my last update, we have:

* [A guide to running IPFS Cluster](docs/ipfs-cluster-guide.md), with detailed information on how cluster works, what behaviours to expect and why. It should answer many questions which are not covered by the getting-started-quickly guides.
* Added sharness tests, which make sure that `ìpfs-cluster-ctl` and `ipfs-cluster-service` are tested and not broken in obvious ways at least and complement our testing pipeline.
* Pushed the [kubernetes-ipfs](https://github.com/ipfs/kubernetes-ipfs) project great lengths, adding a lot of features to its DSL and a bunch of highly advanced ipfs-cluster tests. The goal is to be able to test deployments layouts which are closer to reality, including escalability tests.
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
