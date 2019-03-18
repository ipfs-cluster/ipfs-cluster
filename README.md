# IPFS Cluster


[![Made by](https://img.shields.io/badge/made%20by-Protocol%20Labs-blue.svg?style=flat-square)](https://protocol.ai)
[![Main project](https://img.shields.io/badge/project-ipfs-blue.svg?style=flat-square)](http://github.com/ipfs/ipfs)
[![IRC channel](https://img.shields.io/badge/freenode-%23ipfs--cluster-blue.svg?style=flat-square)](http://webchat.freenode.net/?channels=%23ipfs-cluster)
[![standard-readme compliant](https://img.shields.io/badge/standard--readme-OK-green.svg?style=flat-square)](https://github.com/RichardLitt/standard-readme)
[![GoDoc](https://godoc.org/github.com/ipfs/ipfs-cluster?status.svg)](https://godoc.org/github.com/ipfs/ipfs-cluster)
[![Go Report Card](https://goreportcard.com/badge/github.com/ipfs/ipfs-cluster)](https://goreportcard.com/report/github.com/ipfs/ipfs-cluster)
[![Build Status](https://travis-ci.com/ipfs/ipfs-cluster.svg?branch=master)](https://travis-ci.com/ipfs/ipfs-cluster)
[![codecov](https://codecov.io/gh/ipfs/ipfs-cluster/branch/master/graph/badge.svg)](https://codecov.io/gh/ipfs/ipfs-cluster)

> Pinset orchestration for IPFS.

<p align="center">
<img src="https://cluster.ipfs.io/cluster/png/IPFS_Cluster_color_no_text.png" alt="logo" width="300" height="300" />
</p>

IPFS Cluster is a stand-alone application and a CLI client that allocates, replicates, and tracks pins across a cluster of IPFS daemons.

It provides:

* A cluster peer application: `ipfs-cluster-service`, to be run along with `go-ipfs`.
* A client CLI application: `ipfs-cluster-ctl`, which allows easily interacting with the peer's HTTP API.

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

* [Docs for `ipfs-cluster-service`](https://cluster.ipfs.io/documentation/ipfs-cluster-service/)
* [Docs for `ipfs-cluster-ctl`](https://cluster.ipfs.io/documentation/ipfs-cluster-ctl/)

## Contribute

PRs accepted. As part of the IPFS project, we have some [contribution guidelines](https://cluster.ipfs.io/developer/contribute).

Small note: If editing the README, please conform to the [standard-readme](https://github.com/RichardLitt/standard-readme) specification.

## License

This library is dual-licensed under Apache 2.0 and MIT terms.

© 2019. Protocol Labs, Inc.
