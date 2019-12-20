# `ipfs-cluster-follow`

> A tool to run IPFS Cluster follower peers

`ipfs-cluster-follow` allows to setup and run IPFS Cluster follower peers.

Follower peers can join collaborative clusters to track content in the
cluster. Follower peers do not have permissions to modify the cluster pinset
or access endpoints from other follower peers.

`ipfs-cluster-follow` allows to run several peers at the same time (each
joining a different cluster) and it is intended to be a very easy to use
application with a minimal feature set. In order to run a fully-featured peer
(follower or not), use `ipfs-cluster-service`.

### Usage

The `ipfs-cluster-follow` command is always followed by the cluster name
that we wish to work with. Full usage information can be obtained by running:

```
$ ipfs-cluster-follow --help
$ ipfs-cluster-follow --help
$ ipfs-cluster-follow <clusterName> --help
$ ipfs-cluster-follow <clusterName> info --help
$ ipfs-cluster-follow <clusterName> init --help
$ ipfs-cluster-follow <clusterName> run --help
$ ipfs-cluster-follow <clusterName> list --help
```

For more information, please check the [Documentation](https://cluster.ipfs.io/documentation), in particular the [`ipfs-cluster-follow` section](https://cluster.ipfs.io/documentation/ipfs-cluster-follow).


