# ipfs-cluster client


[![Made by](https://img.shields.io/badge/made%20by-Protocol%20Labs-blue.svg?style=flat-square)](https://protocol.ai)
[![Main project](https://img.shields.io/badge/project-ipfs-blue.svg?style=flat-square)](http://github.com/ipfs/ipfs)
[![IRC channel](https://img.shields.io/badge/freenode-%23ipfs--cluster-blue.svg?style=flat-square)](http://webchat.freenode.net/?channels=%23ipfs-cluster)
[![standard-readme compliant](https://img.shields.io/badge/standard--readme-OK-green.svg?style=flat-square)](https://github.com/RichardLitt/standard-readme)
[![GoDoc](https://godoc.org/github.com/ipfs/ipfs-cluster?status.svg)](https://godoc.org/github.com/ipfs/ipfs-cluster)
[![Go Report Card](https://goreportcard.com/badge/github.com/ipfs/ipfs-cluster)](https://goreportcard.com/report/github.com/ipfs/ipfs-cluster)
[![Build Status](https://travis-ci.org/ipfs/ipfs-cluster.svg?branch=master)](https://travis-ci.org/ipfs/ipfs-cluster)
[![Coverage Status](https://coveralls.io/repos/github/ipfs/ipfs-cluster/badge.svg?branch=master)](https://coveralls.io/github/ipfs/ipfs-cluster?branch=master)


> Go client for ipfs-cluster HTTP API.

This is a Go client library to use the ipfs-cluster REST HTTP API.


## Table of Contents

- [Install](#install)
- [Usage](#usage)
- [Contribute](#contribute)
- [License](#license)

## Install

You can import `github.com/ipfs/ipfs-cluster/api/rest/client` in your code. If you wish to use [`gx`](https://github.com/whyrusleeping/gx-go) for dependency management, it can be imported with:

```
$ gx import github.com/ipfs/ipfs-cluster/
```

The code can be downloaded and tested with:

```
$ go get -u -d github.com/ipfs/ipfs-cluster
$ cd $GOPATH/src/github.com/ipfs/ipfs-cluster/rest/api/client
$ go test -v
```

## Usage

Documentation can be read at [Godoc](https://godoc.org/github.com/ipfs/ipfs-cluster/api/rest/client).

## Contribute

PRs accepted.

## License

MIT © Protocol Labs
