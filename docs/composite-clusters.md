# Usecase: composite clusters

## Introduction

The idea of composite ipfs-clusters is based on the capacity of a cluster node to provide an ipfs-proxy endpoint in which HTTP requests are partially hijacked and used to trigger cluster operations or retrieve cluster information. This means that ipfs-clusters offers a partial ipfs-compatible API.

If this API were to be completely handled by cluster (at least for all the applicable endpoints in which cluster may play a part), we would find that an IPFS API endpoint could be instead an ipfs-cluster peer, and it could be used to control an ipfs-cluster.

Because each ipfs-cluster peer is attached to an IPFS daemon, using the IPFS HTTP API, composition can be achieved by associating cluster peers to cluster-ipfs-proxy-endpoints. Thus, when a cluster peers signals the ipfs daemon to pin an item, it would be instead pinning an item in an underlying cluster. When a cluster peer obtains avaiable repository space, it would be instead obtaining the aggregation of all available space in an underlying cluster.

## Definitions

* Overcluster: the cluster at the top of a cluster composition topology
* Undercluster(s): the clusters at the bottom of a cluster composition topology
* "Peer attached to a peer": A cluster peer that uses the ipfs-proxy-endpoint provided by another peer as its IPFS daemon address.

## Topologies

### Each cluster peer attached to a peer from different underlying clusters (multi-ring clusters)

The most straightforward cluster composition topology consists of a regular ipfs-cluster where the IPFS-endpoints for each cluster peer point to underlying, separate ipfs-clusters.

Todo: add drawing

#### Uses

##### Aggregating resources from several parties

In this scenario, each of the underlying clusters belong to a different party. They make one (or more, for redundancy), proxy-endpoints available to build a larger cluster. This allows to contribute resources in a rather simple fashion to a larger federation. The overcluster is simple to maintain and administer, while the underclusters are adaptable to the specific necessities of each party, including running different cluster versions and security requirements.

##### Creating handling regional ipfs-clusters

In this scenario, each of the undercluster represents an ipfs-cluster deployment in a different datacenter, or a different region. The obercluster can thus distribute content to all different regions, and each region functions independently regarding the replication factor and resources allocated to it. This may be a useful usecase for geo-allocation and CDNs. The overcluster migth be configured with high-latency settings, but the undercluster might be tuned for intra-datacenter operation.

It would be possible to implement geo-allocation as a new ipfs-cluster informer/allocator pair, but this is not supported yet. This usecase allows to implement this scenario in an alternative way.

##### Failure resistance

In this scenario, a large part of the resources allocated to a cluster is perhaps unreliable or not always available. In a regular cluster setup, losing 51% of the peers would leave the cluster in a headless/blocked state. Using composition allows to group unreliable sets of peers into a single cluster peer in the overcluster, and do the same with reliable cluster peers, allowing to give more weight to stable peers. In such case, perhaps the majority of the total account of cluster peers are unavailable at some point, but the overcluster can keep functioning because they are grouped under a single peer.

##### Scaling

In this scenario, the ipfs-cluster setup has reached a number of peers which degrades the performance of the cluster. By spliting a single cluster into several composite subclusters, it allows to reduce the number of peers participating in the consensus and thus, scaling the cluster.



### Each cluster peer attached to a peer from other but unique cluster (cylinder clusters)

In this topology, each cluster peers uses as ipfs daemon another cluster peer from a different cluster, but all underlying peers belong to the same cluster.

Todo: drawing

#### Uses

##### Multimetric allocation

In this case, each cluster uses a different allocator/informer metric. The overcluster would allocate content to the undercluster based on the first metric. The undercluster may use the information from the first allocation as input to another allocator with different metric. In order for this use-case to be useful, such special allocator would need to exist.

##### Double-cluster

In this case (and given an allocate-everywhere replication factor), the obercluster and the undercluster would maintain the same state. The overcluster could be take offline, be upgraded, point to the real ipfs daemons and replace the undercluster, which could then be taken offline. Of course, pinning and unpinning operations would not need to be allowed when there is no overcluster, or manual sync of the state should be needed when it comes back up. It is not very clear if this is a useful usecase.

##### Pyramid pinning

In a cylinder cluster, pin requests to the overcluster are tracked by all the underclusters below. Pin requests to the underclusters, are not tracked by the overcluster, but everything is eventually pinned in the ipfs daemons at the bottom. This allows to build pinning hierarchies, where the bottom clusters have visibility to larger pinsets than the top clusters.

This is a specific case of **inverse multi-ring topology** described below, with a single overcluster.

### Multiple clusters have their cluster peers attached to peers from a single cluster (inverse multi-ring)

In this topology, we have several overclusters which all share the same undercluster.

#### Uses

##### Data partitioning

A group of parties may which to share a common cluster. With an inverse multi-ring setup, each party will mantain their own pinset in their own cluster, but it will use a common infrastructure (the undercluster), which is shared with others. Each overcluster may administer how cluster APIs are exposed and what authentication to use.

In this case, the undercluster needs to be able to scale to be able to handle the pinsets from all the overclusters. The overclusters are probably small, with several nodes just for redundancy, connected to several of the undercluster peers.


### Each cluster peer attached to peers from its own cluster (inception clusters)

In this case, each cluster peers uses as ipfs daemon another cluster peer from the same cluster. overcluster == undercluster.

#### Uses

##### Destroy the world

If the overcluster asks one of the peers to pin, it would in turn ask the overcluster to pin (through a different peer) and so on, ad-infinitum or until the requests falls in a peer which is not attached to the cluster. This would likely make things explode.


## Challenges and open questions

### The IPFS-like cluster API

* Does it need to be and behave exactly like an IPFS node would ?
* Is the IPFS proxy setup the right way to do composition?
* Can the overcluster have any awareness that it is talking to an undercluster?
* Can the proxy requests be forwarded via RPC if we are aware that we are attached not to an IPFS deaemon but to a cluster peer?
* What are the latencies introduced by multi-level cluster, do they need to be better accounted in the IPFS connector?

### Security

* What authentication methods are needed in a composite setup? What authentication should the ipfs proxy support?
* What is the trust relationship between the overclusters and the underclusters?
* What authorization strategies make sense?
  * Read only use?
  * Only some enabled endpoints for some users?
* Should channels be encrypted ?

### Administration overhead

* How hard is it to manage a composite cluster vs. managing a single one? Is it worth it to the use-cases?
* What happens when there are errors, how are they handled in the different levels? Is the handling appropiate?
* Are there alternative ways of handling usecases, like pubsub? Are they better?

### Dynamic clusters

* What advantages do composite clusters offer in managing sets of dynamic/unreliable peers/ipfs-daemons?
* What advantages do composite clusters offer in managing sets of untrusted peeers/daemons?

### Replication

* What replication/allocation strategies make sense in each topology and which not?
* How do current replication/allocation strategies behave in composite clusters?
* Would it be worth adding composition-specific allocators?
* Does compositing facilitate implementation of things like RAID stripping?

