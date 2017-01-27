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

**WORK IN PROGRESS**

**DO NOT USE IN PRODUCTION**

`ipfs-cluster` is a tool which groups a number of IPFS nodes together, allowing to collectively perform operations such as pinning.

In order to do so IPFS Cluster nodes use a libp2p-based consensus algorithm (currently Raft) to agree on a log of operations and build a consistent state across the cluster. The state represents which objects should be pinned by which nodes.

Additionally, cluster nodes act as a proxy/wrapper to the IPFS API, so they can be used as a regular node, with the difference that `pin add`, `pin rm` and `pin ls` requests are handled by the Cluster.

IPFS Cluster provides a cluster-node application (`ipfs-cluster-service`), a Go API, a HTTP API and a command-line tool (`ipfs-cluster-ctl`).

Current functionality only allows pinning in all cluster peers, but more strategies (like setting a replication factor for each pin) will be developed.

## Table of Contents

- [Background](#background)
- [Maintainers and roadmap](#maintainers-and-roadmap)
- [Install](#install)
- [Usage](#usage)
- [API](#api)
- [Contribute](#contribute)
- [License](#license)

## Background

Since the start of IPFS it was clear that a tool to coordinate a number of different nodes (and the content they are supposed to store) would add a great value to the IPFS ecosystem. Naïve approaches are possible, but they present some weaknesses, specially at dealing with error handling, recovery and implementation of advanced pinning strategies.

`ipfs-cluster` aims to address this issues by providing a IPFS node wrapper which coordinates multiple cluster peers via a consensus algorithm. This ensures that the desired state of the system is always agreed upon and can be easily maintained by the cluster peers. Thus, every cluster node knows which content is tracked, can decide whether asking IPFS to pin it and can react to any contingencies like node reboots.

## Maintainers and roadmap

This project is captained by [@hsanjuan](https://github.com/hsanjuan). See the [captain's log](CAPTAIN.LOG.md) for a written summary of current status and upcoming features. You can also check out the project's [Roadmap](ROADMAP.md) for a high level overview of what's coming and the project's [Waffle Board](https://waffle.io/ipfs/ipfs-cluster) to see what issues are being worked on at the moment.

## Install

In order to install the `ipfs-cluster-service` the `ipfs-cluster-ctl` tool  simply download this repository and run `make` as follows:

```
$ go get -u -d github.com/ipfs/ipfs-cluster
$ cd $GOPATH/src/github.com/ipfs/ipfs-cluster
$ make install
```

This will install `ipfs-cluster-service` and `ipfs-cluster-ctl` in your `$GOPATH/bin` folder.

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

You can add the multiaddresses for the other cluster peers the `cluster_peers` variable. For example, here is a valid configuration for a cluster of 4 peers:

```json
{
    "id": "QmSGCzHkz8gC9fNndMtaCZdf9RFtwtbTEEsGo4zkVfcykD",
    "private_key" : "<redacted>",
    "cluster_peers" : [
          "/ip4/192.168.1.2/tcp/9096/ipfs/QmcQ5XvrSQ4DouNkQyQtEoLczbMr6D9bSenGy6WQUCQUBt",
          "/ip4/192.168.1.3/tcp/9096/ipfs/QmdFBMf9HMDH3eCWrc1U11YCPenC3Uvy9mZQ2BedTyKTDf",
          "/ip4/192.168.1.4/tcp/9096/ipfs/QmYY1ggjoew5eFrvkenTR3F4uWqtkBkmgfJk8g9Qqcwy51",
          "/ip4/192.168.1.5/tcp/9096/ipfs/QmSGCzHkz8gC9fNndMtaCZdf9RFtwtbTEEsGo4zkVfcykD"
          ],
    "cluster_multiaddress": "/ip4/0.0.0.0/tcp/9096",
    "api_listen_multiaddress": "/ip4/127.0.0.1/tcp/9094",
    "ipfs_proxy_listen_multiaddress": "/ip4/127.0.0.1/tcp/9095",
    "ipfs_node_multiaddress": "/ip4/127.0.0.1/tcp/5001",
    "consensus_data_folder": "/home/user/.ipfs-cluster/data",
    "raft_config": {
        "snapshot_interval_seconds": 120,
        "enable_single_node": true
    }
```

The configuration file should probably be identical among all cluster peers, except for the `id` and `private_key` fields. To facilitate configuration, `cluster_peers` may include its own address, but it does not have to. For additional information about the configuration format, see the [JSONConfig documentation](https://godoc.org/github.com/ipfs/ipfs-cluster#JSONConfig).

Once every cluster peer has the configuration in place, you can run `ipfs-cluster-service` to start the cluster.

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

## API

TODO: Swagger

This is a quick summary of API endpoints offered by the Rest API component (these may change before 1.0):

|Method|Endpoint            |Comment|
|------|--------------------|-------|
|GET   |/id                 |Cluster peer information|
|GET   |/version            |Cluster version|
|GET   |/peers              |Cluster peers|
|GET   |/pinlist            |List of pins in the consensus state|
|GET   |/pins               |Status of all tracked CIDs|
|POST  |/pins/sync          |Sync all|
|GET   |/pins/{cid}         |Status of single CID|
|POST  |/pins/{cid}         |Pin CID|
|DELETE|/pins/{cid}         |Unpin CID|
|POST  |/pins/{cid}/sync    |Sync CID|
|POST  |/pins/{cid}/recover |Recover CID|


## Contribute

PRs accepted.

Small note: If editing the README, please conform to the [standard-readme](https://github.com/RichardLitt/standard-readme) specification.

## License

MIT © Protocol Labs, Inc.
