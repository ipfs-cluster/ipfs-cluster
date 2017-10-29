# IPFS cluster context, motivations, use cases

This document is an attempt to help explain why ipfs-cluster exists, why people use it and why it looks the way it does.  A lot of this summarizes old discussion scattered around other repos, issues and docs, so please read the linked documents to get the full context.

## ipfs-cluster's Purpose: ipfs Coordination
ipfs-cluster is a tool to coordinate ipfs nodes together.  It was clearly described here during its early planning stages: https://github.com/ipfs/notes/issues/58.
From this description ipfs-cluster can be used to coordinate any aspect of an ipfs node’s state among a group of ipfs nodes.  This includes pin sets, the current focus of the project, but could also include state related to bitswap strategies or inter-node trust models, etc.  To achieve coordination of complex state among ipfs nodes without centralization, the ipfs-cluster project relies on a consensus protocol.  Today this is raft but in the future we want to make this be pluggable.  Today the project is solely focused on coordinating pinset state: https://github.com/ipfs/ipfs-cluster/blob/master/README.md.  This is a sensible place to focus because ipfs is at its core a protocol for storing and moving data and the pinset of an ipfs node determines what data it stores and distributes.


## A key concept: the ipfs vNode abstraction:
Early discussion, again in https://github.com/ipfs/notes/issues/58, outlines a particular approach that remains relevant to discussion today: ipfs-cluster as a virtual ipfs node (vNode for short).  The idea is that ipfs-cluster nodes could implement the ipfs api, only exposing state agreed upon by all nodes through consensus.  A simple example: all nodes in the cluster agree on a single peer id, and after reaching agreement all nodes respond with this id to requests on their ipfs vNode id endpoint.  A more useful example: an ipfs-cluster node gets an `add <file>` request through the ipfs vNode add endpoint, the cluster nodes coordinate adding this file, perhaps doing something clever like replicating file data across multiple nodes, and reach agreement that the data was added and pinned successfully via consensus.  After all this occurs the api endpoint returns the normal message indicating success.

Designing ipfs-cluster to work this way has some benefits, including a familiar api for user interaction, the ability to use a cluster anywhere an ipfs node is used and the ability to make ipfs-clusters depend on other ipfs-clusters as the ipfs nodes that they coordinate.  This last property has the potential to make scaling ipfs-cluster easier; if large groups of participants can be abstracted away consensus peer group size can remain bounded as cluster participants grow arbitrarily.  It is not always the case that an ipfs api is the best user interface for adding files to ipfs-cluster.  If ipfs-cluster were to support behavior like per-pin replication configuration information, for example different pins specifying different replication factors as it does today, then the ipfs api has no endpoint to encode this information and some kind of cluster specific interface is needed  (See discussion https://botbot.me/freenode/ipfs/2017-02-09/ here for a somewhat related conversation that includes discussion of more use cases).

Though an ipfs vNode api IS partially implemented in ipfs-cluster to the point that ipfs-clusters can be composed, only the subset of the vNode api that ipfs-cluster needs to call in order to function has received attention.  There has been some discussion, in “Other open questions” https://github.com/hsanjuan/ipfsclusterspec/blob/master/README.md, about the difficulties involved in implementing a full ipfs vNode interface, and framing the vNode interface as a separate concern from the ipfs-cluster project’s primary goal of coordinating ipfs nodes.  Today emphasis on implementing the vNode interface exists to the extent that it enables composition of ipfs-clusters, but not beyond this.


## Use cases
These are some very WIP ipfs-cluster use cases.  Note these are not formal use cases and are more accurately groups of related use cases, they could be further decomposed into the narrower scope operations found in formal use cases.

### Collaborative archiving

### ipfs node mirror

### Structured data storage and distribution network

### Application specific ipfs CDN

### Long-distance replication and offsite backup (first reported here https://github.com/ipfs/ipfs-cluster/issues/157)


### Pinning Rings (first reported here https://github.com/ipfs/ipfs-cluster/issues/7)


### Home directories with ipfs-cluster (first reported here https://github.com/ipfs/ipfs-cluster/issues/3)

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