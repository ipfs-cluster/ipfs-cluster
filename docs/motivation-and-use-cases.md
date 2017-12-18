# IPFS cluster use cases 

These are some WIP ipfs-cluster use case sketches.  These are not formal use cases and are more accurately groups of related use cases; they could be further decomposed into the more narrowly scoped operations found in formal use cases.

## ipfs node mirror

## Blockchain storage

## ipfs CDN for dApp data

## Backend for ipfs gateways

## Long-distance bidirectional offsite backups (first reported here https://github.com/ipfs/ipfs-cluster/issues/157)

Actors:
	admin of a local network

Description: An admin wants automatic backups of a local network's data to an offsite location.  The offsite storage is also located in its own local network in need of its own backup storage.  The admin therefore needs some mechanism for bidirectional backups.  NAS units in the two networks serve as local backup devices.  The admin runs ipfs on the NAS units in each network, exposing their fuse interfaces to allow for conventional means of local backups within the local network, for example using rsync between the NAS and machines in need of backup.  Additionally end user machines are granted access to the local network's ipfs endpoint, again via the fuse interface, to allow backup mechanisms like Time Machine to the NAS.  The admin sets up the system so that the NAS units in each network host both an ipfs and cluster daemon.  When data is backed up in a NAS's ipfs daemon, it is automatically pinned by cluster and replicated as appropriate to the offsite backups.  The admin can add new NAS units in the existing or in new networks to the ipfs-cluster without difficulty.

Implied ipfs-cluster requirements:
- Some way to automatically cluster pin (with a standard replication policy) anything that is added to the ipfs peer
- Any cluster peer can add and replicate content (true today)
- Adding new peers to an ipfs-cluster is a smooth process (mostly true today)
- ipfs-cluster can handle the latencies of long distance backups (I think mostly true especially with new raft config options, but this should be measured)

Thoughts: Perhaps cluster should support a way to allow for automatically pinning anything added to the ipfs daemon.  If things are added by the api endpoint this could be achievable by substituting the ipfs api endpoint with the cluster endpoint and performing a cluster pin before proxying the add to the ipfs daemon.  However if things are added to the fuse interface this won't work, so maybe the general solution is outside of cluster's scope. 


## Pinning Rings (first reported here https://github.com/ipfs/ipfs-cluster/issues/7)


## Big ipfs node for ubuntu archive seeding (reported here https://discuss.ipfs.io/t/ubuntu-archive-on-top-of-ipfs/1579)

Actors:
	admin of package-seeding cluster
	mirror site providing package data
	users fetching packages over ipfs

Description:  An admin wishes to set up an ipfs node storing a mirror of ubuntu deb packages to support an apt transport that downloads deb packages from ipfs.  The admin wishes to download the packages and directory structure from one of the existing mirrors over http and store in ipfs.  The admin does not have a server with the 2TB of storage necessary to host the entire mirror and so cannot fit all packages on a single ipfs node.  However the admin does have access to a set of smaller servers (say 4 servers of 50GB) that together fulfill the total storage capacity of the mirror.  The admin installs ipfs-cluster on each server and then commands the cluster to download the mirror data.  During download the cluster allocates different pieces of the mirror directory to different machines, spreading load evenly.  If there is extra space on the servers then replication of packages is a bonus, but this is not a primary concern for this usecase.  The cluster hosting the mirror can be assumed stable, with a fixed number of servers all ran by a single admin or administrative body.  After packages are added to the cluster users will fetch packages by path name from the root hash (QmAAA.../mirrorDir1/mirrorDir2/package.deb).  

Implied ipfs-cluster requirements
- ipfs-cluster can handle importing, sharding and distributing across the cluster a file too big for one node, see PR #268.
- ipfs importers can import ubuntu archives as directories

Thoughts:
This use case's requirement on sharding large files is blocked on implementing basic sharding in cluster.  It is a clear goal of cluster to eventually support this and other use cases ingesting large amounts of data into ipfs.  Although this is possible without ipfs support for an importer specific to the ubuntu archive data format, this use case would work best with such support.  This implies a need for a refactored and user extensible library (dagger/dex) for handling the data format --> ipld dag transformation across many data formats.


## Home directories with ipfs-cluster (first reported here https://github.com/ipfs/ipfs-cluster/issues/3)

Actors:
	file system users
	system admin

Description:  An admin wishing to host a distributed filesystem on ipfs-cluster sets up a few ipfs-cluster machines on dedicated nodes on a WAN.  Nodes in the same LAN or same region are organized in a sub cluster.  The admin begins by adding existing home directories to ipfs-cluster.  This includes adding unix style authentication information to different directories and files so that only those with the right permissions can access files.  After set up is complete, users begin accessing their files, reading, writing and sharing amongst themselves.  When users get started with the new file system they add a new ipfs-cluster peer located on their local machine or workstation to the cluster, this way they can contribute to storing and distributing user data while accessing files.  Throughout the life-time of the distributed file-system’s use the admin administers the underlying ipfs-cluster with a web-based management console.  Clients can provide quotas across a couple of metrics (ex: bandwidth and storage) so that ipfs-cluster doesn’t hog all their resources.

Implied ipfs-cluster requirements:

- ipfs-cluster provides key management, allow for per-user keys and implements built in encryption for robust, out-of-the-box security. 
- ipfs-cluster’s replication strategies are sophisticated enough to achieve excellent availability, this may include live, feedback-based replication strategies that take into consideration the popularity and age of files
- ipfs-cluster exposes a management console that allows the setting of authentication and encryption metadata, and setting policies for ipfs-cluster as a unit and the sub clusters of which it consists.  Policy parameters include replication schemes and allocation schemes based on storage space, location, bandwidth and subcluster membership.  
- ipfs-cluster implements complex allocation strategies and supports the policies that outline these strategies, as well as live updates of these policies
- ipfs-cluster provides configurable load balancing across ipfs-cluster nodes (WAN cluster and LAN subclusters have different balancers), that can handle frequent changes to peersets
- ipfs-clusters are easy to join and leave without having to think too much about set-up or generating errors
- ipfs-cluster allows for retrieval of data across sub clusters that are not necessarily connected, except as two subtrees of the same larger cluster.
ipfs-cluster allows individual peers to specify resource constraints 

Thoughts:
This is a mature use case that requires a lot of not-yet-there features.  It's something good to shoot for but more long term.  From a quick glance it seems likely that some of the sought-after features might belong to other projects (maybe the permissioning of the file-system and built in security primitives belong in a layer over ipfs for example, maybe some of the balancer and replication strategy modules could be pluggable and project specific).  Accessing data pinned in one subcluster from another subcluster is an interesting problem given the current composite cluster module that we should think about further.



# IPFS cluster historical context and motivation

This section is an attempt to help explain why ipfs-cluster exists, why people use it and why it looks the way it does.  A lot of this summarizes old discussion scattered around other repos, issues and docs, so please read the linked documents to get the full context.  This is kept with use cases for now because it helps frame ipfs-cluster in terms of the general set of problems it can solve and the general interface that it provides users.  This in turn facilitates imagining broader use cases for cluster that are non-obvious from today's implementation.  

## ipfs-cluster's Purpose: ipfs Coordination
ipfs-cluster is a tool to coordinate ipfs nodes together.  It was clearly described here during its early planning stages: https://github.com/ipfs/notes/issues/58.
From this description ipfs-cluster can be used to coordinate any aspect of an ipfs node’s state among a group of ipfs nodes.  This includes pin sets, the current focus of the project, but could also include state related to bitswap strategies or inter-node trust models, etc.  To achieve coordination of complex state among ipfs nodes without centralization, the ipfs-cluster project relies on a consensus protocol.  Today this is raft but in the future we want to make this pluggable.  Today the project is solely focused on coordinating pinset state: https://github.com/ipfs/ipfs-cluster/blob/master/README.md.  This is a sensible place to focus because ipfs is at its core a protocol for storing and moving data and the pinset of an ipfs node determines what data it stores and distributes.


## A key concept: the ipfs vNode abstraction:
Early discussion, again in https://github.com/ipfs/notes/issues/58, outlines a particular approach that remains relevant to discussion today: ipfs-cluster as a virtual ipfs node (vNode for short).  The idea is that ipfs-cluster nodes could implement the ipfs api, only exposing state agreed upon by all nodes through consensus.  A simple example: all nodes in the cluster agree on a single peer id, and after reaching agreement all nodes respond with this id to requests on their ipfs vNode id endpoint.  A more useful example: an ipfs-cluster node gets an `add <file>` request through the ipfs vNode add endpoint, the cluster nodes coordinate adding this file, perhaps doing something clever like replicating file data across multiple nodes, and reach agreement that the data was added and pinned successfully via consensus.  After all this occurs the api endpoint returns the normal message indicating success.

Designing ipfs-cluster to work this way has some benefits, including a familiar api for user interaction, the ability to use a cluster anywhere an ipfs node is used and the ability to make ipfs-clusters depend on other ipfs-clusters as the ipfs nodes that they coordinate.  This last property has the potential to make scaling ipfs-cluster easier; if large groups of participants can be abstracted away consensus peer group size can remain bounded as cluster participants grow arbitrarily.  It is not always the case that an ipfs api is the best user interface for adding files to ipfs-cluster.  Ipfs-cluster's support for behavior like per-pin replication configuration information, for example the current feature that pins can specify different replication factors, has no direct analogue to characteristics of an ipfs node exposed over the ipfs api.  As the ipfs api has no endpoint to encode this information, some kind of cluster specific interface is often useful, for example the current cluster `pin` command that allows setting replication factors.

Though an ipfs vNode api IS partially implemented in ipfs-cluster to the point that ipfs-clusters can be composed, only the subset of the vNode api that ipfs-cluster needs to call in order to function has received attention.  There has been some discussion, in “Other open questions” https://github.com/hsanjuan/ipfsclusterspec/blob/master/README.md, about the difficulties involved in implementing a full ipfs vNode interface, and framing the vNode interface as a separate concern from the ipfs-cluster project’s primary goal of coordinating ipfs nodes.  Today emphasis on implementing the vNode interface exists to the extent that it enables composition of ipfs-clusters, and further work may be revisited.
