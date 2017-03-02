# ipfs-cluster


[![](https://img.shields.io/badge/made%20by-Protocol%20Labs-blue.svg?style=flat-square)](http://ipn.io)
[![](https://img.shields.io/badge/project-ipfs-blue.svg?style=flat-square)](http://github.com/ipfs/ipfs)
[![](https://img.shields.io/badge/freenode-%23ipfs-blue.svg?style=flat-square)](http://webchat.freenode.net/?channels=%23ipfs)
[![standard-readme compliant](https://img.shields.io/badge/standard--readme-OK-green.svg?style=flat-square)](https://github.com/RichardLitt/standard-readme)
[![GoDoc](https://godoc.org/github.com/ipfs/ipfs-cluster?status.svg)](https://godoc.org/github.com/ipfs/ipfs-cluster)
[![Go Report Card](https://goreportcard.com/badge/github.com/ipfs/ipfs-cluster)](https://goreportcard.com/report/github.com/ipfs/ipfs-cluster)
[![Build Status](https://travis-ci.org/ipfs/ipfs-cluster.svg?branch=master)](https://travis-ci.org/ipfs/ipfs-cluster)
[![Coverage Status](https://coveralls.io/repos/github/ipfs/ipfs-cluster/badge.svg?branch=master)](https://coveralls.io/github/ipfs/ipfs-cluster?branch=master)


> Collective pinning and composition for IPFS.

**THIS SOFTWARE IS ALPHA**

`ipfs-cluster` allows to replicate content (by pinning) in multiple IPFS nodes:

* Works on top of the IPFS daemon by running one cluster peer per IPFS node (`ipfs-cluster-service`)
* A `replication_factor` controls how many times a CID is pinned in the cluster
* Re-pins stuff in a different place when a peer goes down
* Provides an HTTP API and a command-line wrapper (`ipfs-cluster-ctl`)
* Provides an IPFS daemon API Proxy which intercepts any "pin"/"unpin" requests and does cluster pinning instead
* The IPFS Proxy allows to build cluster composition, with a cluster peer acting as an IPFS daemon for another higher-level cluster.
* Peers share the state using Raft-based consensus. Uses the LibP2P stack (`go-libp2p-raft`, `go-libp2p-rpc`...)


## Table of Contents

- [Maintainers and Roadmap](#maintainers-and-roadmap)
- [Install](#install)
- [Usage](#usage)
  - [`ipfs-cluster-service`](#ipfs-cluster-service)
  - [`ipfs-cluster-ctl`](#ipfs-cluster-ctl)
  - [`Go`](#go)
  - [`Additional docs`](#additional-docs)
- [API](#api)
- [Architecture](#api)
- [Contribute](#contribute)
- [License](#license)


## Maintainers and Roadmap

This project is captained by [@hsanjuan](https://github.com/hsanjuan). See the [captain's log](CAPTAIN.LOG.md) for a written summary of current status and upcoming features. You can also check out the project's [Roadmap](ROADMAP.md) for a high level overview of what's coming and the project's [Waffle Board](https://waffle.io/ipfs/ipfs-cluster) to see what issues are being worked on at the moment.

## Install

`ipfs-cluster` is written in Go. In order to install the `ipfs-cluster-service` the `ipfs-cluster-ctl` tool  simply download this repository and run `make` as follows:

```
$ go get -u -d github.com/ipfs/ipfs-cluster
$ cd $GOPATH/src/github.com/ipfs/ipfs-cluster
$ make install
```

This will install `ipfs-cluster-service` and `ipfs-cluster-ctl` in your `$GOPATH/bin` folder. See the usage below.

## Usage

### `ipfs-cluster-service`

`ipfs-cluster-service` runs a cluster peer. Usage information can be obtained running:

```
$ ipfs-cluster-service -h
```

Before running `ipfs-cluster-service` for the first time, initialize a configuration file with:

```
$ ipfs-cluster-service -init
```

The configuration will be placed in `~/.ipfs-cluster/service.json` by default.

You can add the multiaddresses for the other cluster peers the `bootstrap` variable. For example, here is a valid configuration for a single-peer cluster:

```json
{
    "id": "QmXMhZ53zAoes8TYbKGn3rnm5nfWs5Wdu41Fhhfw9XmM5A",
    "private_key": "<redacted>",
    "cluster_peers": [],
    "bootstrap": [],
    "leave_on_shutdown": false,
    "cluster_multiaddress": "/ip4/0.0.0.0/tcp/9096",
    "api_listen_multiaddress": "/ip4/127.0.0.1/tcp/9094",
    "ipfs_proxy_listen_multiaddress": "/ip4/127.0.0.1/tcp/9095",
    "ipfs_node_multiaddress": "/ip4/127.0.0.1/tcp/5001",
    "consensus_data_folder": "/home/user/.ipfs-cluster/data",
    "state_sync_seconds": 60,
    "replication_factor": -1
}
```

The configuration file should probably be identical among all cluster peers, except for the `id` and `private_key` fields. Once every cluster peer has the configuration in place, you can run `ipfs-cluster-service` to start the cluster. See the [additional docs](#additional-docs) section for detailed documentation on how to build a cluster.

#### Clusters using `cluster_peers`

The `cluster_peers` configuration variable holds a list of current cluster members. If you know the members of the cluster in advance, or you want to start a cluster fully in parallel, set `cluster_peers` in all configurations so that every peer knows the rest upon boot. Leave `bootstrap` empty. A cluster peer address looks like: `/ip4/1.2.3.4/tcp/9096/<id>`.

#### Clusters using `bootstrap`

When the `cluster_peers` variable is empty, the multiaddresses `bootstrap` can be used to have a peer join an existing cluster. The peer will contact those addresses (in order) until one of them succeeds in joining it to the cluster. When the peer is shut down, it will save the current cluster peers in the `cluster_peers` configuration variable for future use.

Bootstrap is a convenient method, but more prone to errors than `cluster_peers`. It can be used as well with `ipfs-cluster-service --bootstrap <multiaddress>`. Note that bootstrapping nodes with an old state (or diverging state) from the one running in the cluster may lead to problems with
the consensus, so usually you would want to bootstrap blank nodes.

#### Debugging

`ipfs-cluster-service` offers two debugging options:

* `--debug` enables debug logging from the `ipfs-cluster`, `go-libp2p-raft` and `go-libp2p-rpc` layers. This will be a very verbose log output, but at the same time it is the most informative.
* `--loglevel` sets the log level (`[error, warning, info, debug]`) for the `ipfs-cluster` only, allowing to get an overview of the what cluster is doing. The default log-level is `info`.

### `ipfs-cluster-ctl`

`ipfs-cluster-ctl` is the client application to manage the cluster nodes and perform actions. `ipfs-cluster-ctl` uses the HTTP API provided by the nodes and it is completely separate from the cluster service. It can talk to any cluster peer (`--host`) and uses `localhost` by default.

After installing, you can run `ipfs-cluster-ctl --help` to display general description and options, or alternatively `ipfs-cluster-ctl help [cmd]` to display
information about supported commands.

In summary, it works as follows:

```
$ ipfs-cluster-ctl id                                                       # show cluster peer and ipfs daemon information
$ ipfs-cluster-ctl peers ls                                                 # list cluster peers
$ ipfs-cluster-ctl peers add /ip4/1.2.3.4/tcp/1234/<peerid>                 # add a new cluster peer
$ ipfs-cluster-ctl peers rm <peerid>                                        # remove a cluster peer
$ ipfs-cluster-ctl pin add Qma4Lid2T1F68E3Xa3CpE6vVJDLwxXLD8RfiB9g1Tmqp58   # pins a CID in the cluster
$ ipfs-cluster-ctl pin rm Qma4Lid2T1F68E3Xa3CpE6vVJDLwxXLD8RfiB9g1Tmqp58    # unpins a CID from the cluster
$ ipfs-cluster-ctl status                                                   # display tracked CIDs information
$ ipfs-cluster-ctl sync Qma4Lid2T1F68E3Xa3CpE6vVJDLwxXLD8RfiB9g1Tmqp58      # sync information from the IPFS daemon
$ ipfs-cluster-ctl recover Qma4Lid2T1F68E3Xa3CpE6vVJDLwxXLD8RfiB9g1Tmqp58   # attempt to re-pin/unpin CIDs in error state
```

#### Debugging

`ipfs-cluster-ctl` provides a `--debug` flag which allows to inspect request paths and raw response bodies.

### Go

IPFS Cluster nodes can be launched directly from Go. The `Cluster` object provides methods to interact with the cluster and perform actions.

Documentation and examples on how to use IPFS Cluster from Go can be found in [godoc.org/github.com/ipfs/ipfs-cluster](https://godoc.org/github.com/ipfs/ipfs-cluster).

### Additional docs

You can find more information and detailed guides:

* [Building and updating an IPFS Cluster](docs/HOWTO_build_and_update_a_cluster.md)

Note: please contribute to improve and add more documentation!

## API

TODO: Swagger

This is a quick summary of API endpoints offered by the Rest API component (these may change before 1.0):

|Method|Endpoint            |Comment|
|------|--------------------|-------|
|GET   |/id                 |Cluster peer information|
|GET   |/version            |Cluster version|
|GET   |/peers              |Cluster peers|
|POST  |/peers              |Add new peer|
|DELETE|/peers/{peerID}     |Remove a peer|
|GET   |/pinlist            |List of pins in the consensus state|
|GET   |/pins               |Status of all tracked CIDs|
|POST  |/pins/sync          |Sync all|
|GET   |/pins/{cid}         |Status of single CID|
|POST  |/pins/{cid}         |Pin CID|
|DELETE|/pins/{cid}         |Unpin CID|
|POST  |/pins/{cid}/sync    |Sync CID|
|POST  |/pins/{cid}/recover |Recover CID|


## Architecture

The best place to get an overview of how cluster works, what components exist etc. is the [architecture.md](architecture.md) doc.

## Contribute

PRs accepted.

Small note: If editing the README, please conform to the [standard-readme](https://github.com/RichardLitt/standard-readme) specification.

## License

MIT © Protocol Labs, Inc.
