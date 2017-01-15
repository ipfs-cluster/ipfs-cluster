# ipfs-cluster


[![](https://img.shields.io/badge/made%20by-Protocol%20Labs-blue.svg?style=flat-square)](http://ipn.io)
[![](https://img.shields.io/badge/project-ipfs-blue.svg?style=flat-square)](http://github.com/ipfs/ipfs)
[![](https://img.shields.io/badge/freenode-%23ipfs-blue.svg?style=flat-square)](http://webchat.freenode.net/?channels=%23ipfs)
[![standard-readme compliant](https://img.shields.io/badge/standard--readme-OK-green.svg?style=flat-square)](https://github.com/RichardLitt/standard-readme)
[![GoDoc](https://godoc.org/github.com/ipfs/ipfs-cluster?status.svg)](https://godoc.org/github.com/ipfs/ipfs-cluster)
[![Build Status](https://travis-ci.org/ipfs/ipfs-cluster.svg?branch=master)](https://travis-ci.org/ipfs/ipfs-cluster)
[![Coverage Status](https://coveralls.io/repos/github/ipfs/ipfs-cluster/badge.svg?branch=master)](https://coveralls.io/github/ipfs/ipfs-cluster?branch=master)


> Clustering for IPFS nodes.

**WORK IN PROGRESS**

**DO NOT USE IN PRODUCTION**

`ipfs-cluster` is a tool which groups a number of IPFS nodes together, allowing to collectively perform operations such as pinning.

In order to do so IPFS Cluster server nodes use a libp2p-based consensus algorithm (currently Raft) to agree on a log of operations and build a consistent state across the cluster. The state represents which objects should be pinned by which nodes.

Additionally, server nodes act as a proxy/wrapper to the IPFS API, so they can be used as a regular node, with the difference that pin requests are handled by the Cluster.

IPFS Cluster provides a server application (`ipfscluster-server`), a Go API, a HTTP API and a command-line tool (`ipfscluster`).

Current functionality only allows pinning in all cluster members, but more strategies (like setting a replication factor for each pin) will be developed.

## Table of Contents

- [Background](#background)
- [Install](#install)
- [Usage](#usage)
- [API](#api)
- [Contribute](#contribute)
- [License](#license)

## Background

Since the start of IPFS it was clear that a tool to coordinate a number of different nodes (and the content they are supposed to store) would add a great value to the IPFS ecosystem. Naïve approaches are possible, but they present some weaknesses, specially at dealing with error handling, recovery and implementation of advanced pinning strategies.

`ipfs-cluster` aims to address this issues by providing a IPFS node wrapper which coordinates multiple cluster members via a consensus algorithm. This ensures that the desired state of the system is always agreed upon and can be easily maintained by the members of the cluster. Thus, every cluster member knows which content is tracked, can decide whether asking IPFS to pin it and can react to any contingencies like server reboots.

## Install

In order to install the `ipfscluster-server` the `ipfscluster` tool  simply download this repository and run `make` as follows:

```
$ go get -u -d github.com/ipfs/ipfs-cluster
$ cd $GOPATH/src/github.com/ipfs/ipfs-cluster
$ make install
```

This will install `ipfscluster-server` and `ipfscluster` in your `$GOPATH/bin` folder.

## Usage

### `ipfscluster-server`

`ipfscluster-server` runs a member node for the cluster. Usage information can be obtained running:

```
$ ipfscluster-server -h

```

Before running `ipfscluster-server` for the first time, initialize a configuration file with:

```
$ ipfscluster-server -init
```

The configuration will be placed in `~/.ipfs-cluster/server.json` by default.

You can add the multiaddresses for the other members of the cluster in the `cluster_peers` variable. For example, here is a valid configuration for a cluster of 4 members:

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
    "cluster_addr": "0.0.0.0",
    "cluster_port": 9096,
    "consensus_data_folder": "/home/user/.ipfs-cluster/data",
    "api_addr": "0.0.0.0",
    "api_port": 9094,
    "ipfs_api_addr": "0.0.0.0",
    "ipfs_api_port": 9095,
    "ipfs_addr": "127.0.0.1",
    "ipfs_port": 5001
}
```

The configuration file should probably be identical among all cluster members, except for the `id` and `private_key` fields. To facilitate configuration, `cluster_peers` includes its own address, but it does not have to.

Once every cluster member has the configuration in place, you can run `ipfscluster-server` to start the cluster.


### `ipfscluster`

`ipfscluster` is the client application to manage the cluster servers and perform actions. `ipfscluster` uses the HTTP API provided by the server nodes.

After installing, you can run `ipfscluster --help` to display general description and options, or alternatively `ipfscluster help [cmd]` to display
information about supported commands.

In summary, it works as follows:

```
$ ipfscluster member ls                                                # list cluster members
$ ipfscluster pin add Qma4Lid2T1F68E3Xa3CpE6vVJDLwxXLD8RfiB9g1Tmqp58   # pins a Cid in the cluster
$ ipfscluster pin add Qma4Lid2T1F68E3Xa3CpE6vVJDLwxXLD8RfiB9g1Tmqp58   # unpins a Cid from the cluster
$ ipfscluster status                                                   # display tracked Cids information
$ ipfscluster sync Qma4Lid2T1F68E3Xa3CpE6vVJDLwxXLD8RfiB9g1Tmqp58      # recover Cids in error status
```

### Go

IPFS Cluster nodes can be launched directly from Go. The `Cluster` object provides methods to interact with the cluster and perform actions.

Documentation and examples on how to use IPFS Cluster from Go can be found in [godoc.org/github.com/ipfs/ipfs-cluster](https://godoc.org/github.com/ipfs/ipfs-cluster).

## API

TODO: Swagger

## Contribute

PRs accepted.

Small note: If editing the README, please conform to the [standard-readme](https://github.com/RichardLitt/standard-readme) specification.

## License

MIT © Protocol Labs, Inc.
