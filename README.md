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

In order to do so `ipfs-cluster` nodes use a libp2p-based consensus algorithm (currently Raft) to agree on a log of operations and build a consistent state across the cluster. The state represents which objects should be pinned by which nodes.

Additionally, `ipfs-cluster` nodes act as a proxy/wrapper to the IPFS API, so an IPFS cluster can be used as a regular node.

`ipfs-cluster` provides a Go API, an equivalent HTTP API (in a RESTful fashion) and a command-line interface tool (`ipfs-cluster-ctl`) which plugs directly into it.

## Table of Contents

- [Background](#background)
- [Install](#install)
- [Usage](#usage)
- [API](#api)
- [Contribute](#contribute)
- [License](#license)

## Background

Since the start of IPFS it was clear that a tool to coordinate a number of different nodes (and the content they are supposed to store) would add a great value to the IPFS ecosystem. Naïve approaches are possible, but they present some weaknesses, specially at dealing with error handling and incorporating multiple pinning strategies.

`ipfs-cluster` aims to address this issues by providing a IPFS node wrapper which coordinates multiple cluster members via a consensus algorithm. This ensures that the desired state of the system is always agreed upon and can be easily maintained by the members of the cluster. Thus, every cluster member has an overview of where each hash is pinned, and the cluster can react to any contingencies, like IPFS nodes dying, by redistributing the storage load to others.

## Install

TODO

## Usage

The documentation and examples for `ipfs-cluster` (useful if you are integrating it in Go) can be found in [godoc.org/github.com/ipfs/ipfs-cluster](https://godoc.org/github.com/ipfs/ipfs-cluster).

TODO

## API

TODO

## Contribute

PRs accepted.

Small note: If editing the README, please conform to the [standard-readme](https://github.com/RichardLitt/standard-readme) specification.

## License

MIT © Protocol Labs, Inc.
