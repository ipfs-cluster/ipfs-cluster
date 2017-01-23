# IPFS Cluster - Roadmap

## Q1 2017

This quarter is going to be focused on bringing ipfs-cluster to life as a usable product in the IPFS ecosystem. That means:

* It shouldn't hard to crash
* It shouldn't lose track of content
* It should play well with go-ipfs
* It should support a replication-factor
* It should be very well tested
* It should be very easy to setup and manage
* It should have stable APIs

On these lines, there are several endeavours which stand out for themselves and are officially part of the general IPFS Roadmaps:

* Dynamically add and remove cluster members in an easy fashion (https://github.com/ipfs/pm/issues/353)

This involves easily adding a member (or removing) from a running cluster. `ipfs-cluster-service member add <maddress>` should work and should update the peer set of all components of all members, along with their configurations.

* Replication-factor-based pinning strategy (https://github.com/ipfs/pm/issues/353)

This involves being able to pin an item in, say, 2 nodes only. Cluster should re-pin whenever an item is found to be underpinned, which means that monitoring of pinsets must exist and be automated.

* Tests (https://github.com/ipfs/pm/issues/360)

In the context of the Interplanetary Test Lab, there should be tests end to end tests in which cluster is tested, benchmarked along with IPFS.
