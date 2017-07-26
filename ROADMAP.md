# IPFS Cluster - Roadmap

## Q3 2017

Since Q1 ipfs-cluster has progressed a lot with a strong effort in reaching a minimal feature set that allows to use it in production settings along with lots of focus on testing. In the meantime different endeavours have taken lots of time and cluster did not get a roadmap for Q2.

For Q3, ipfs-cluster will focus on:

* Making all the ipfs-kubernetes tests fit
* Working on low-hanging fruit: easy features with significant impact
* Adopting ipfs-cluster as a replacement for the ipfs pin-bot (or deploying a production-maintained ipfs-cluster)
* Outlining more a Q4 roadmap where development efforts come back in-full to ipfs-cluster

For a more detailed explanation on what has been achieved during the last months, check the [Captain's Log](CAPTAIN.log.md).

## Q1 2017

This quarter is going to be focused on bringing ipfs-cluster to life as a usable product in the IPFS ecosystem. That means:

* It should be hard to crash
* It shouldn't lose track of content
* It should play well with go-ipfs
* It should support a replication-factor
* It should be very well tested
* It should be very easy to setup and manage
* It should have stable APIs

On these lines, there are several endeavours which stand out for themselves and are officially part of the general IPFS Roadmaps:

* Dynamically add and remove cluster peers in an easy fashion (https://github.com/ipfs/pm/issues/353)

This involves easily adding a peer (or removing) from a running cluster. `ipfs-cluster-service peer add <maddress>` should work and should update the peer set of all components of all peers, along with their configurations.

* Replication-factor-based pinning strategy (https://github.com/ipfs/pm/issues/353)

This involves being able to pin an item in, say, 2 nodes only. Cluster should re-pin whenever an item is found to be underpinned, which means that monitoring of pinsets must exist and be automated.

* Tests (https://github.com/ipfs/pm/issues/360)

In the context of the Interplanetary Test Lab, there should be tests end to end tests in which cluster is tested, benchmarked along with IPFS.
