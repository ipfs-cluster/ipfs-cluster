# IPFS Cluster - Roadmap

## Q4 2017

The Q4 roadmap has been crystalized in the [ipfs-cluster OKRs](https://docs.google.com/spreadsheets/d/1rLxvRfdYohv-dhzVAo1xfeJOUa7uGfq6N_yyKw0iRw0/edit?usp=sharing). They can be summarized as:

* Making cluster production grade
  * Run a live deployment of ipfs-cluster
  * Integrate it with pin-bot
  * Run kubernetes tests in a real kubernetes cluster

* Start looking into big datasets
  * Design a strategy to handle "big files"
  * Look into ipld/ipfs-pack

* Support users
  * Give prompt feedback
  * Fix all bugs
  * Improve documentation
  * Make releases

## Q3 2017

Since Q1 ipfs-cluster has made some progress with a strong effort in reaching a minimal feature set that allows to use it in production settings along with lots of focus on testing. The summary of what has been achieved can be seen in the [Captain's Log](CAPTAIN.log.md).

For Q3, this is the tentative roadmap of things on which ipfs-cluster will focus on:

* Making all the ipfs-kubernetes tests fit
* Working on low-hanging fruit: easy features with significant impact
* Adopting ipfs-cluster as a replacement for the ipfs pin-bot (or deploying a production-maintained ipfs-cluster)
* Outlining more a Q4 roadmap where development efforts come back in-full to ipfs-cluster


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
