# IPFS Cluster


[![Made by](https://img.shields.io/badge/made%20by-Protocol%20Labs-blue.svg?style=flat-square)](https://protocol.ai)
[![Main project](https://img.shields.io/badge/project-ipfs-blue.svg?style=flat-square)](http://github.com/ipfs/ipfs)
[![Matrix channel](https://img.shields.io/badge/matrix-%23ipfs--cluster-blue.svg?style=flat-square)](https://app.element.io/#/room/#ipfs-cluster:ipfs.io)
[![GoDoc](https://godoc.org/github.com/ipfs/ipfs-cluster?status.svg)](https://godoc.org/github.com/ipfs/ipfs-cluster)
[![Go Report Card](https://goreportcard.com/badge/github.com/ipfs/ipfs-cluster)](https://goreportcard.com/report/github.com/ipfs/ipfs-cluster)
[![Build Status](https://travis-ci.com/ipfs/ipfs-cluster.svg?branch=master)](https://travis-ci.com/ipfs/ipfs-cluster)
[![codecov](https://codecov.io/gh/ipfs/ipfs-cluster/branch/master/graph/badge.svg)](https://codecov.io/gh/ipfs/ipfs-cluster)

> Automated data availability and redundancy on IPFS

<p align="center">
<img src="https://cluster.ipfs.io/cluster/png/IPFS_Cluster_color_no_text.png" alt="logo" width="300" height="300" />
</p>

IPFS Cluster provides data orchestration across a swarm of IPFS daemons by allocating, replicating and tracking a global pinset distributed among multiple peers.

It provides:

* A cluster peer application: `ipfs-cluster-service`, to be run along with `go-ipfs`.
* A client CLI application: `ipfs-cluster-ctl`, which allows easily interacting with the peer's HTTP API.
* An additional "follower" peer application: `ipfs-cluster-follow`, focused on simplifying the process of configuring and running follower peers.

---

### Are you using IPFS Cluster?

Please participate in the [IPFS Cluster user registry](https://docs.google.com/forms/d/e/1FAIpQLSdWF5aXNXrAK_sCyu1eVv2obTaKVO3Ac5dfgl2r5_IWcizGRg/viewform).

---

## Table of Contents

- [Documentation](#documentation)
- [News & Roadmap](#news--roadmap)
- [Install](#install)
- [Usage](#usage)
- [Contribute](#contribute)
- [License](#license)


## Documentation

Please visit https://cluster.ipfs.io/documentation/ to access user documentation, guides and any other resources, including detailed **download** and **usage** instructions.

## News & Roadmap

We regularly post project updates to https://cluster.ipfs.io/news/ .

The most up-to-date *Roadmap* is available at https://cluster.ipfs.io/roadmap/ .

## Install

Instructions for different installation methods (including from source) are available at https://cluster.ipfs.io/download .

## Usage

Extensive usage information is provided at https://cluster.ipfs.io/documentation/ , including:

* [Docs for `ipfs-cluster-service`](https://cluster.ipfs.io/documentation/reference/service/)
* [Docs for `ipfs-cluster-ctl`](https://cluster.ipfs.io/documentation/reference/ctl/)
* [Docs for `ipfs-cluster-follow`](https://cluster.ipfs.io/documentation/reference/follow/)

## Contribute

PRs accepted. As part of the IPFS project, we have some [contribution guidelines](https://cluster.ipfs.io/support/#contribution-guidelines).

## License

This library is dual-licensed under Apache 2.0 and MIT terms.

© 2020. Protocol Labs, Inc.
