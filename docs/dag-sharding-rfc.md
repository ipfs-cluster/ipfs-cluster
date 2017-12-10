# Introduction

This document is a Request For Comments outlining a proposal for supporting sharded DAGs in ipfs-cluster, it outlines motivations for sharding DAGs across the nodes of an ipfs-cluster, and proposes a development path to achieve the motivated goals.

# Motivation

There are two primary motivations for adding data sharding to ipfs-cluster.  See the WIP use cases documents in PR #215, issue #212, and posts on the discuss.ipfs forum (ex: https://discuss.ipfs.io/t/ubuntu-archive-on-top-of-ipfs/1579 and https://discuss.ipfs.io/t/segmentation-file-in-ipfs-cluster/1465/2 and https://discuss.ipfs.io/t/when-file-upload-to-node-that-can-ipfs-segmentation-file-to-many-nodes/1451) for some more context.

## Motivation 1: store files too big for a single node

This is one of the most requested features for ipfs-cluster.  ipfs-pack (https://github.com/ipfs/ipfs-pack) exists to fill a similar need by allowing data stored in a POSIX filesystem to be referenced from an ipfs-node's datastore without acutally copying all of the data into the node's repo.  However certain use cases require functionality beyond ipfs-pack. For example, what if a cluster of nodes needs to download a large file bigger than any of its nodes' individual machines' disk size?  In this case using ipfs-pack is not enough, such massive files would not fit on a single node's storage device and coordination between nodes to store the file becomes essential.

Storing large files is a feature that would serve many use cases and the more general ability to store large ipld DAGs of arbitrary structure has the potential to support others.  As an example consider splitting a massive cryptocurrency ledger or git tree among the nodes of an ipfs cluster.

## Motivation 2: store files with different replication strategies for fault tolerance

This feature is also frequently requested, including in the original thread leading to the creation of ipfs-cluster (See issue #1), and in @raptortech-js 's thorough discussion in issue #9.  The idea is to use sharding to incorporate space-efficient replication techniques to provide better fault tolerance of storage.  This would open up ipfs-cluster to new usage patterns, especially storing archives with infrequent updates and long storage periods.

Again, while files storage will benefit from efficient, fault tolerant encodings, these properties are potentially also quite useful for storing arbitrary merkle DAGs. 

# Background

To add a file ipfs must "import" it.  ipfs chunks the file's raw bytes and builds a merkle DAG out of these chunks to organize the data for storage in the ipfs repo. The format of the merkle DAG nodes, the way the chunks are produced and the layout of the DAG depend on configurable strategies. Usually ipfs represents the data added as a tree which mimics a unix-style hierarchy so that the data can be directly translated into a unix-like filesystem representation.

Regardless of the chunking or layout strategy, importing a file can be viewed abstractly as a process that takes in a stream of data and outputs a stream of DAG nodes, or more precisely "blocks", which is the term for the representation of DAG nodes stored on disk.  As blocks represent DAG nodes they contain links to other blocks.  Together these blocks and their links determine the structure of the DAG built by the importing process.  Furthermore these blocks are content-addressed by ipfs, which resolves blocks by these addresses upon user request.  Although the current imorting process adds all blocks corresponding to a file to a single ipfs node, this is not important for preserving the DAG structure.  ipfs nodes advertise the address of every block added to their storage repo.  If a DAG's blocks exist across multiple ipfs peers the individual blocks can readily be discovered by any peer and the information the blocks carry can be put back together.  This location flexibility makes partitioning a file's data at the block level an attractive mechanism for implementing sharding in ipfs-cluster.

Therefore we propose that sharding in ipfs-cluster amounts to allocating a stream of blocks among different ipfs nodes.  Cluster should aim to do so:
1. Efficiently: best utilizing the resources available in a cluster as provided by the underlying ipfs nodes
2. Lightly: the cluster state should not carry more information than relevant to cluster, e.g. no more than is relevant for coordinating allocation of collections of blocks
3. Cleverly: the allocation of blocks might benefit from the information provided by the DAG layout or ipld-format of the DAG nodes.  For example cluster should eventually support grouping together logical subgraphs of the DAG.

On a first approach we aim for a simple layout and format-agnostic strategy which simply groups blocks output from the importing process into a size-based unit, from here on a "shard", that fits within a single ipfs node's storage repo.

## Implementation
### Overview
We propose that cluster organizes sharding by pinning a management "Cluster DAG" defines links to groups of sharded data.  We also propose a sharding pipeline for ingesting data, constructing a DAG, and sharding across cluster nodes.  To allow for ingestion of datasets too large for any one ipfs node's repo we propose a pipeline that acts on a stream of data, forwarding blocks across the cluster while reading from the input stream to avoid overloading local resources.  First the data passes through the HTTP api of the adding cluster peer.  Here it is dispatched through an importing process to transform raw data into the blocks of a dag.  Finally these blocks are used to create the Cluster DAG and the data of both graphs is distributed among the nodes of the cluster.  Note that our goal of supporting large files implies that we cannot simply hand off data to the local ipfs daemon for importing, in general the data will not fit in the repo and AFAIK streaming blocks out of an ipfs daemon endpoint during importing is not something ipfs readily supports. The Cluster DAG's structure and the ingestiong pipeline steps are explained in further detail in the following sections.

### Cluster DAG
We propose to pin two dags in ipfs-cluster for every sharded file.  The first is the ipfs unixfs dag encoded in the blocks output by the importing process.  Additionally ipfs-cluster builds the Cluster DAG specifically built to track and pin the blocks of the file dag in a way that groups data into shards.  Each shard is pinned by (at least) one cluster peer.  The graph of shards is organized in a 3 level tree.  The first level is the cluster-dag root, the second represents the shards into which data is divided with at least one shard per cluster peer.  The third level lists all the cids of blocks of the file dag included in a shard:
```
The root would be like:
{
  "shards" : [
     {"/": <shard1>},
     {"/": <shard2>},
     ...
   ]
}

where each shard looks like:

{
  "blocks" : [
     {"/": <block1>},
     {"/": <block2>},
     ...
   ]
}

```
Each cluster node recursively pins a shard-node of the cluster-dag, ensuring that the blocks of file data referenced underneath the shard are pinned by that node.  The index of shards, i.e. the root, can be (non-recursively) pinned on any one/all cluster nodes.

With this implementation the cluster can provide the entire original ipfs file dag on request.  For example if an ipfs peer queries for the entire file they first resolve the root node of the dag which must be pinned on some shard.  If the root has child links then ipfs peers in the cluster are guaranteed to resolve them with content, as all blocks are pinned somewhere in the cluster as children of a shard-node in the cluster-dag.  Note that this implementation conforms well to the goal of coordinating "lightly" above because the cluster state need only keep track of 1 cid per shard.  This prevents the state size from growing too large or more complex than necessary for the purpose of allocation.

The state format would change slightly to account for linking together the root cid of an ipfs file dag and the cluster-dag pinning its leaves

```
"<cid>": {
  "name": <pinname>,
  "cid": <cid>,
  "clusterdag": <cidcluster>,
  "allocations" : []
}
"<shard1-cid>": {
  "name": "<pinname>-shard1",
  "cid": <shard1-cid>,
  "clusterdag": nil,
  "allocations": [cluster peers]
}
....
```

Under this implementation replication of shards proceeds like replication of an ordinary pinned cid.  As a consequence shards can be pinned with a replication factor.

### User APIs

#### Importing files for sharding
Data sharding must begin with a user facing endpoint.  Ideally cluster could stream files, or other data to be imported, across its HTTP API.  This way `ipfs-cluster-ctl` could launch the command for adding and sharding data in the usual way, and users also have the option of hitting the HTTP endpoint over the network.  Multiple files are typically packaged in an HTTP request as the parts of a `Content-Type: multi-part` request and HTTP achieves streaming with `Transfer-Type: chunked`.  Because golang http and mime/multipart [support chunked (a.k.a. streamed) transfer of multipart messages](https://gist.github.com/ZenGround0/49e4a1aa126736f966a1dfdcb84abdae), building the user endpoint into the HTTP API should be relatively straightforward.

The golang multipart-reader exposes a reader to each part of the body in succession.  When the part's body is read completely, a reader to the next part's body can be generated by the multipart-reader.  The part meta-data (e.g. this part is a directory, that part is a file) and the data read from the part, must then be passed to the importer stage of the pipeline.  Our proposal is to call an external library, with a stream of data and metadata as input, from within the HTTP API to move to this next stage.

As this endpoint will be used for streaming large amounts of data it has the potential to be somewhat difficult to use as system limits could be pushed and errors could occur after annoyingly long time investments.  Some questions that may come up as usability becomes a bigger concern:

* Is it possible that remote users pushing data to the endpoint could overload cluster's resources by pushing at a rate faster than processing can occur in cluster? If this is an issue how can the endpoint maintain connections in a resource-aware non-degrading way?  One potential solution we are considering is to provide the HTTP API over libp2p, which is built for situations like this.
* What features can cluster provide to make the process easier?  For example if cluster could report an estimate aggregate of all of the space in its nodes' repos then users could check that a huge file will fit on the cluster before importing it.
* How should we handle the case where the sharding process fails part way through leaving behind pinned blocks in nodes across the cluster?  Could we have a built in mechanism for attempting cleanup after a failure is detected?  Could we provide a command for launching such a cleanup?
* After implementing an HTTP API endpoint should we build out other methods of ingesting data such as pulling from a filesystem device or location address over the network?

#### Sharding existing DAGs
An endpoint should also exist for sharding an existing DAG among an ipfs-cluster.  A user will run something like `ipfs-cluster-ctl pin add --shard <cid>` and pin shards of the DAG rooted at `cid` across different nodes.  The importing step of the pipeline can be skipped in this case.  The naive implementation is to interact with the local daemon's DAG API to retrieve a stream of blocks as input to the sharding component.  While this implementation would still be quite useful for distributing DAGs shards from a node with a large amount of repo storage across a cluster, we currently suspect that this is not going to work well for large DAGs that cannot fit in the node's repo.  The "Future Work" subsection: "sharding large existing DAGs" discusses some possibilities for scaling this to large DAGs on nodes of arbitrary repo size.

### Importer module
As they are currently implemented in ipfs, importers run in three main phases.  The importer first begins by traversing a collection of data and metadata that can be interpreted as files and directories.  For example the tar importer reads data from a tar archive and the file importer used by `ipfs add` iterates over a collection of file objects.  Files are then broken into chunks with a chunker.  These chunks are then used as the data to make the dag-nodes constructed by the dag layout building process.  The dag-nodes are serialized as blocks and saved to the repo.

Importing is currently a stream friendly process.  The objects traversed in the first stage are typically all wrappers around a reader of some kind and so the data and metadata (AFAIK) can be streamed in.  Existing chunkers read from their reader as needed when `NextBytes()` is called to get the next chunk.  The layout process creates in-memory DAG nodes derived from the leaf nodes, themselves thin wrappers around chunk data.  Nodes are flushed to the dagservice's `Batch()` process as soon as their parent links are established, and care is taken to avoid leaving references in memory so the golang garbage collector can reclaim the memory.  The balanced layout builder makes a balanced tree and so the memory high water mark is logN for a DAG of N nodes (I will have to investigate trickle's potential for streaming but my guess right now is it's good too).  Furthermore the dagservice batching collects a group of nodes only up to a threshold before forwarding them to their destination in the dagservice's blockstore.  Our least intrusive option is to create a "streaming blockstore" for the dagservice used in the importing process's dagbuilder helper for forwarding the imported blocks to the final stage of the pipeline.

As it stands it is not clear that the existing unix files version of the first stage (go-ipfs/core/coreunix/add.go) is particularly streaming friendly as at a glance its traversal structure implies that all of the files of a directory exist while processing the directory.  Additionally there are a fair amount of non-importer specific ipfs dependencies used in the current implementation (most strikingly mfs) and we should evaluate how much of this we can live without.  We are currently expecting to move this stage of the pipeline to an importers github repo, allowing the modified importing process to be called as a library function from the HTTP API.  Perhaps this will be a step forward for the "dex" project to boot.  The tar format is a good example moving forward of how we might rewrite the files importer, as it's input is a logical stream of readers bearing many similarities to the multipart reader interface that will serve as input to the cluster importer.

The format we use to encode file metadata in HTTP multipart requests can be mimicked from ipfs and should be standardized as much as possible for intuitive use of the API endpoint.

Designing a streaming blockstore for outputting block nodes begs the question of how the blocks will be transferred to the cluster sharding component.  Multiple methods exist for streaming blocks out and we will need to investigate our desired assumptions and performance goals further to make a decision here.  Note that in addition to transferring data over rpc or directly over a libp2p stream muxer we have another option: pass data to the sharding cluster component by instantiating the sharding functionality as a component of the blockstore used in the importer.

### Sharding cluster component

At a high level the sharding component takes in a stream of dag-node blocks, maintains the necessary data to build a shard node of the Cluster DAG, and forwards the stream of blocks to a peer for pinning.  When blocks arrive the sharding process records the cids which are needed for constructing links between the shard node and the blocks of the DAG being sharded.  This is all the sharding component must do before forwarding blocks to other peers.  The peer receiving the shard is calculated once by querying the allocator.  The sharding component must be aware of how much space peers have for storing shards and will maintain state for determining when a shard is complete and the receiving peer needs to be recalculated.  The actual forwarding can be accomplished by periodically calling an RPC that runs DAG API `put` commands with processed blocks on the target peer.  After all the blocks of a shard are transmitted the relevant sub-graph of the Cluster DAG is constructed and sent as a final transmission to the allocated peer.  Upon receiving this final transmission the allocated peer recursively pins the node of the Cluster DAG that it receives, ensuring that all blocks of the shard stay in its ipfs repo.  When all shards are constructed and allocated the final root node pointing to all shard node is pinned (non-recursively) throughout the cluster.

One remaining challenge to work out is preventing garbage collection in the receiving node from removing the dag blocks streamed and added between the first and final transmission.  We will either need to find a way to disable gc over the ipfs API (AFAIK this is not currently supported), or work with the potentially expensive workaround of pinning every block as it arrives, recursively pinning the shard node at the end of transmission, and then unpinning the individual blocks.

# Future work

## Fault tolerance
Disclaimer: this needs more work and that work will go into its own RFC. This section provides a basis upon which we can build.  It is included to demonstrate that the current sharding model works well for implementing this important extension.  We will bring this effort back into focus once the prerequisite basic sharding discussed above is implemented.

### Background reading
This [report on RS coding for fault tolerance in RAID-like Systems by Plank](https://web.eecs.utk.edu/~plank/plank/papers/CS-96-332.pdf) is a very straightforward and practical guide.  It has been helpful in planning out how to approach fault tolerance in cluster and will be very helpful when we actually implement.  It has an excellent description of how to implement the algorithm that calculates the code from data, and how to implement recovery.  Furthermore one of his example use case studies include algorithms for initialization of the checksum values that will be useful when replicating very large files.

### Proposed implementation
#### Overview
The general idea of fault tolerant storage is to store your data in such a way that if several machines go down all of your data can be reconstructed.  Of course you could simply store all of your data on every machine in your cluster, but many clever approaches use data sharding and techniques like erasure codes to achieve the same result with fewer bits stored.  A standard approach is to store shards of data across multiple data devices and then store some kind of checksum information in separate, checksum devices.  It is a simple matter to extend the basic sharding implementation above to work well in this paradigm.  When storing a file in a fault tolerant configuration ipfs-cluster, as in basic sharding, will store the ipfs files DAG without its leaves and an cluster-DAG.  However now the cluster-DAG has additional shards not referencing the leaves of the ipfs files DAG, but rather to checksum data taken over all the file's data.  For an m out of n encoding:

```
The root would be like:
{
  "shards" : [
     {"/": <shard1>},
     {"/": <shard2>},
     ...
     {"/": <shardN>},
   ]
   "checksums" : [
     {"/": <chksum1>},
     {"/": <chksum2>},
     ...
     {"/": <chksumM>},
}
```

While there are several RAID modes using different configurations of erasure codes and data to checksum device ratios, my opinion is that we probably can ignore most of these as using m,n RS coding is superior in terms of space efficiency for fault tolerance gained.  However different RAID modes have different time efficiency properties in their original setting anyway.  It is unclear if implementing something (maybe) more time efficient but less space efficient and fault tolerant than RS has much value in ipfs-cluster.  I lean towards no but I should investigate further. (TODO -- answer these questions, any counter example use cases?  any big gains for using other RAID modes that help these use cases?) On another note in Feb 2019 the tornado code patent is set to expire (\o/) and we could check back in then and look into the feasibility of using (perhaps implementing if no OSS exists yet?!?) tornado codes (which are faster).  There are others we'll want to check the legal/implementation situation for (biff codes) so pluggability is important.

Overall this is pretty cool for users because the original DAG (recall how basic sharding works) and the original data exist within the cluster.  This way users can query any cid from the original DAG and the cluster ipfs nodes will seamlessly provide it, all while silently and with very efficient overhead they are protecting the data from a potentially large number of peer faults.

We have some options for allowing users to specify this mode.  It could be a cluster specific flag to the "add" endpoint or a config option setting a cluster wide default.

#### Importing with checksums
If memory/repo size constraints are not a limiting factor it should be straightforward for the cluster-DAG importer running on the adding node to keep a running tally of the checksum values and then allocate them to nodes after getting every data shard pinned.  Note this claim is specific to RS as the coding calculations are simple linear combinations of operations and everything commutes, while I wouldn't be surprised if potential future codes also had this property it is something we'll need to check up on once we get serious about pluggability.

If we are in a situation where shards approach the size of ipfs repo or working memory then we can gather inspiration from the report by Plank, specifically the section "Implementation and Performance Details: Checkpointing Systems".  In this section Plank outlines two algorithms for setting checksum values after the data shards are already stored by sending messages over the network.  From my first read-through the broadcast algorithm looks the most promising.  This algorithm would allow cluster to send shards one at a time to the peer holding a zeroed out checksum shard and then perform successive updates to calculate the checksums, rather than requiring that the cluster-DAG importer hold the one shard being filled up for pinning alongside the m checksum shards being calculated.

## Clever sharding
Future work on sharding should aim to keep parts of the DAG that are typically fetched together within the same shard.  This feature should be able to take advantage of particular chunking and layout algorithms, for example grouping together subDAGs representing packages when importing a file as a linux package archive.  It would also be nice to have some techniques, possibly pluggable, available for intelligently grouping blocks of an arbitrary DAG based on the DAG structure.

## Sharding large existing DAGs
AFAIK this would require new features in go-ipfs.  Two approaches are apparent.

If ipfs-cluster is to stream DAG nodes from its local daemon to create a Cluster DAG, the ipfs api would need to include an endpoint that provides a stream of a DAG's blocks without over-committing the daemon's resources (i.e. not downloading the whole DAG first if the repo is too small).  One relevant effort is the [CAR (Certified ARchives) project](https://github.com/ipld/specs/pull/51) which aims to define a format for storing DAGs.  One of its goals as stated in a recent proposal is to explicitly allow users to "Traverse a large DAG streamed over HTTP without downloading the entire thing."  Cluster could shard DAG nodes extracted from a streamed CAR file, either kept on disk or provided at a location on the network.  CAR integration with go-ipfs could potentially allow resource aware streaming of DAG nodes over the ipfs daemon's api so that the sharding cluster peer need only know the DAG's root hash.

Another relevant effort is that of ipld selectors.  ipld selectors are interesting because they are an orthogonal approach to the Cluster DAG for succinctly organizing large pinsets.  We did not end up including them in the main proposal because adding large datasets necessitates importing data to a DAG anyway.  In this case, from our perspective, building a management DAG for pinning and directly transferring blocks is simpler than adding a portion of the DAG to the local ipfs node, executing a remote selector pin, blocking on ipfs to do the transfer, and then clearing resources from the local node.  However in the case that the dag is already on ipfs, pinning selectors for shards is a potential implementation strategy.  The selectors would essentially act as the shard-nodes of the Cluster DAG, selecting shards of the DAG that could fit on individual nodes and getting pinned by the assigned nodes.  At the very least this seems to require that ipld selectors can select based on sub-tree size.  It is currently unclear how well matched ipld selectors would be for this use case.

# Work plan

There is a fair amount that needs to be built, updated and serviced to make all of this work.  Here I am focusing on basic sharding, allowing support for files that wouldn't fit in an ipfs repo.  Support for fault tolerance, more clever sharding strategies and sharding large existing DAGs will come after this is nailed down.

* Build up a version of the `importers` (aka `dagger` aka `dex`) project that suits cluster's needs (if possible keep it general enough for general use).  In short we want a library that imports data to ipfs DAGs from arbitrary formats (tar, deb, unix files, etc) with arbitrary chunking and layout algorithms.  For our use case it is important that the interfaces support taking in a stream of data and returning a stream of blocks or DAG nodes.
* Implement a refactored importers library (e.g. proto `dex`) to be imported and called by the HTTP API.  It will need to provide an entry call that supports streaming and an output mechanism that can connect to the sharding component.
* Create a cluster add endpoint on the REST API that reads in file data on HTTP multipart requests and calls the cluster importer library entrypoint function.  Listen on an output stream and put the output blocks into the local ipfs daemon and pin the root with cluster.  This will require nailing down the APIs of these components and development of a garbage collection management strategy.
* Build out the sharding component, implementing creation of the Cluster DAG and pin blocks by way of pinning shards.  This is still all confined to one cluster peer.  This stage may require defining a cluster ipld-format (if we go that route).
* Update the state format so that pinned cids reference Cluster DAGs and so that the state differentiates between cids and shards.
* Build support for allocation of different Cluster DAG shard-nodes to different peers.  This includes implementing RPC transmission of all chunks of a given shard to the allocated peer by the sharding component.
* Work to make the state format scale to large numbers of cids
* Add in usability features: don't make sharding default on add endpoint but trigger with --shard, maybe make configs that can set sharding as default or importer defaults, allow add to specify different chunking and importing algorithms like ipfs, ipfs add proxy endpoint can also take in --shard and then calls same functionality as add endpoint, add in tools useful for managing big downloads like reporting the total storage capacity of cluster or the expected download time.
* Testing should happen throughout and we should have a plan with regards to which tests we run in place early (maybe before we start).  Eventually we will want a huge cluster in the cloud with a few TB of storage for ingesting huge files
* Move on beyond basic sharding and start design process again to support clever techniques for certain DAGs/usecases, sharding large existing DAGs, and fault tolerance
