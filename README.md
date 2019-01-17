Elastos Hive Cluster
==========================

## Introduction

Elastos Hive Cluster is a decentralized File Storage Service that based on IPFS cluster. Hive Cluster use IPFS and IPFS-cluster as the base infrastructure to save Elastos data.  Hive Cluster is also a stand-alone application as same as IPFS Cluster.

The typical IPFS peer is a resource hunger program. If you install IPFS daemon to your mobile device, it will take up resources and slow down your device. We are creating Hive project, which uses IPFS cluster as the Elastos App storage backend, and can be used in a low resources consumption scenario.

The project distils from the IPFS Cluster, but it will have many differences with the IPFS Cluster.

Elastos Hive Cluster maintains a big IPFS pinset for sharing. It can serve numerous virtual IPFS peers with only one running a real IPFS peer.

Hive Cluster is not only a pinset manager but also a backend for multiple IPFS clients.

## Table of Contents

- [Introduction](#introduction)
- [Install](#install)
- [Usage](#usage)
- [Contribution](#contribution)
- [Acknowledgments](#acknowledgments)
- [License](#license)

## Install

The following requirements apply to the installation from source:

- Go 1.11+
- Git
- IPFS or internet connectivity (to download depedencies).

In order to build and install IPFS Cluster in Unix systems follow the steps:

```shell
$ git clone https://github.com/elastos/Elastos.NET.Hive.Cluster.git $GOPATH/src/github.com/elastos/Elastos.NET.Hive.Cluster
$ cd $GOPATH/src/github.com/elastos/Elastos.NET.Hive.Cluster
$ make install
```
After the dependencies have been downloaded, ipfs-cluster-service and ipfs-cluster-ctl will be installed to your $GOPATH/bin (it uses go install).

If you would rather have them built locally, use make build instead. You can run make clean to remove any generated artifacts and rewrite the import paths to their original form.

Note that when the ipfs daemon is running locally on its default ports, the build process will use it to fetch gx, gx-go and all the needed dependencies directly from IPFS.

## Usage

ipfs-cluster-service is a command line program, please refer to the docs from https://cluster.ipfs.io.
* [Docs for `ipfs-cluster-service`](https://cluster.ipfs.io/documentation/ipfs-cluster-service/)

Hive Cluster uses HTTP interface to serve numerous clients. About HTTP API, please refer to 
* [HTTP API] TODO

## Contribution

We welcome contributions to the Elastos Hive Project.

## Acknowledgments

A sincere thank you to all teams and projects that we rely on directly or indirectly.

## License
This project is licensed under the terms of the [MIT license](https://github.com/elastos/Elastos.Hive.Cluster/blob/master/LICENSE).
