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
  - [Pre-compiled binaries](#pre-compiled-binaries)
  - [Docker](#docker)
  - [Install from sources](#install-from-sources)
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

### Pre-compiled binaries

You can download pre-compiled binaries for your platform from the [dist.ipfs.io](https://dist.ipfs.io) website:

* [Builds for `ipfs-cluster-service`](https://dist.ipfs.io/#ipfs-cluster-service)
* [Builds for `ipfs-cluster-ctl`](https://dist.ipfs.io/#ipfs-cluster-ctl)

Note that since IPFS Cluster is evolving fast, the these builds may not contain the latest features/bugfixes as they are updated only bi-weekly.

### Docker

You can build or download an automated build of the ipfs-cluster docker container. This container runs both the IPFS daemon and `ipfs-cluster-service` and includes `ipfs-cluster-ctl`. To launch the latest published version on Docker run:

`$ docker run ipfs/ipfs-cluster`

To build the container manually you can:

`$ docker build . -t ipfs-cluster`

You can mount your local ipfs-cluster configuration and data folder by passing `-v /data/ipfs-cluster your-local-ipfs-cluster-folder` to Docker.

### Install from sources

Installing from `master` is the best way to have the latest features and bugfixes. In order to install the `ipfs-cluster-service` the `ipfs-cluster-ctl` tools you will need `Go` installed in your system and the run the following commands:

```
$ go get -u -d github.com/ipfs/ipfs-cluster
$ cd $GOPATH/src/github.com/ipfs/ipfs-cluster
$ make install
```

This will install `ipfs-cluster-service` and `ipfs-cluster-ctl` in your `$GOPATH/bin` folder. See the usage below.

## Usage

### `ipfs-cluster-service`

For information on how to configure and launch an IPFS Cluster peer see the [`ipfs-cluster-service` README](ipfs-cluster-service/dist/README.md).

### `ipfs-cluster-ctl`

For information on how to manage and perform operations on an IPFS Cluster peer see the [`ipfs-cluster-ctl` README](ipfs-cluster-ctl/dist/README.md).

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
