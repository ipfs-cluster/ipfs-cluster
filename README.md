# Elastos Hive Cluster

[![img](https://camo.githubusercontent.com/9ff0f4b787066b705774659143d8b88f485119ff/68747470733a2f2f696d672e736869656c64732e696f2f62616467652f6d61646525323062792d456c6173746f732532306f72672d626c75652e7376673f7374796c653d666c61742d737175617265)](http://elastos.org)
[![img](https://camo.githubusercontent.com/85d19725dcd92c6f77a1d72a2e9b2b49c36489ab/68747470733a2f2f696d672e736869656c64732e696f2f62616467652f70726f6a6563742d486976652d626c75652e7376673f7374796c653d666c61742d737175617265)](http://elastos.org/)
[![standard-readme compliant](https://camo.githubusercontent.com/a7e665f337914171fa0b60a110690af78fc5d943/68747470733a2f2f696d672e736869656c64732e696f2f62616467652f7374616e646172642d2d726561646d652d4f4b2d677265656e2e7376673f7374796c653d666c61742d737175617265)](https://github.com/RichardLitt/standard-readme)
[![Build Status](https://camo.githubusercontent.com/d95d2cf5f0f2c8ebf5697026daaa4cbfaab6521e/68747470733a2f2f7472617669732d63692e6f72672f656c6173746f732f456c6173746f732e4e45542e486976652e495046532e7376673f6272616e63683d6d6173746572)](https://travis-ci.org/elastos/Elastos.NET.Hive.Cluster)

## Introduction

Elastos Hive Cluster is a decentralized File Storage Service that based on IPFS cluster. Hive Cluster use IPFS and IPFS-cluster as the base infrastructure to save Elastos data.  Hive Cluster is also a stand-alone application as same as IPFS Cluster.

The typical IPFS peer is a resource hunger program. If you install IPFS daemon to your mobile device, it will take up resources and slow down your device. We are creating Hive project, which uses IPFS cluster as the Elastos App storage backend, and can be used in a low resources consumption scenario.

The project distils from the IPFS Cluster, but it will have many differences with the IPFS Cluster.

Elastos Hive Cluster maintains a big IPFS pinset for sharing. It can serve numerous virtual IPFS peers with only one running a real IPFS peer.

Hive Cluster is not only a pinset manager but also a backend for multiple IPFS clients.

## Table of Contents

- [Introduction](#introduction)
- [Install](#install)
  - [Install Go](#Install-Go)
  - [Download Source](#Download-Source)
  - [Build Cluster](#Build-Cluster)
- [Usage](#usage)
- [Get Started](#Get-Started)
  - [Initialize config files](#Initialize-config-files)
  - [Run as daemon](#Run-as-daemon)
- [Contribution](#contribution)
- [Acknowledgments](#acknowledgments)
- [License](#license)

## Install

The following requirements apply to the installation from source:

- Git
- Go 1.11+
- IPFS or internet connectivity (to download depedencies).

#### Install Go

The build process for cluster requires Go 1.10 or higher. Download propriate binary archive from [golang.org](https://golang.org/dl),  and install it onto specific directory:

```
$ curl go1.11.4.linux-amd64.tar.gz -o golang.tar.gz
$ tar -xzvf golang.tar.gz -C YOUR-INSTALL-PATH
```

Then add path  **YOUR-INSTALL-PATH/go/bin**  to user environment variable **PATH**.

```
$ export PATH="YOUR-INSTALL-PATH/go/bin:$PATH"
```

Besides that, build environment for `golang` projects must be required to setup:

```
$ export GOPATH="YOUR-GO-PATH"
$ export PATH="$GOPATH/bin:$PATH"
```

In convinience,  just add these lines to your profile `$HOME/.profile`, then validate it with the command:

```
$ . $HOME/.profile
$ go version
```

Then use **export** command to check if it validated or not.

**Notice** : If you run into trouble, see the [Go install instructions](https://golang.org/doc/install).

#### Download Source

There are two ways to setup your source code. One is directly to download source code under `$GOPATH` environment as below:

```
$ cd $GOPATH/src/github.com/elastos
$ git clone https://github.com/elastos/Elastos.NET.Hive.Cluster.git
```

The other way is to download source to specific location, and then create linkage to that directory underp propriate `$GOPATH` environment:

```
$ cd YOUR-PATH
$ git clone https://github.com/elastos/Elastos.NET.Hive.Cluster.git 
$ cd $GOPATH/src/github.com/elastos
$ link -s YOUR-PATH/Elastos.NET.Hive.Cluster Elastos.NET.Hive.Cluster
```

#### Build Cluster

To build Cluster with the following commands:

```
$ cd $GOPATH/src/github.com/elastos/Elastos.NET.Hive.Cluster
$ make
$ make install
```

After installation, **ipfs-cluster-service** and **ipfs-cluster-ctl**  would be generated under the directory `$GOPATH/bin`

If you would rather have them built locally, use make build instead. You can run make clean to remove any generated artifacts and rewrite the import paths to their original form.

Note that when the ipfs daemon is running locally on its default ports, the build process will use it to fetch gx, gx-go and all the needed dependencies directly from IPFS.

## Usage

**ipfs-cluster-service** is a command line program to start cluster daemon, while **ipfs-cluster-ctl**  is the client application to manage the cluster nodes and perform actions. 

Run the following commands with `hep` options to see more usages:

```
$ ipfs-cluster-service help
...
$ ipfs-cluster-ctl help
...
```

Details about ipfs cluster please refer to the docs from https://cluster.ipfs.io.

Hive cluster uses HTTP interface to serve numerous clients. About HTTP API, please refer to 
* [HTTP API](TODO)

## Get Started

Hive cluster have to be running before starting Hive cluster. About how to  run Hive IPFS daemon, please refer to [Elastos Hive IPFS](https://github.com/elastos/Elastos.NET.Hive.IPFS.git#README.md).

#### Initialize config files

To start using Cluster, you must first initialize Cluster's config files on your system, which would be done with the command below:

```
$ ipfs-cluster-service init
$ ls $HOME/.ipfs-cluster
service.json
```

See `ipfs-cluster-service help` for more information on the optional arguments it takes.

#### Run as Daemon

After initialization and configure, try to run **ipfs-cluster-service** daemon:

```shell
$ ipfs-cluster-service daemon &
```

then you can use **ipfs-cluster-ctl** program to check if cluster is running

```shell
$ ipfs-cluster-ctl id
QmUGXgnUcqgZf9Js7GrvFP1uz7G6soq3urWk9Gz237DsQm | guest | Sees 1 other peers
  > Addresses:
    - /ip4/127.0.0.1/tcp/9096/ipfs/QmUGXgnUcqgZf9Js7GrvFP1uz7G6soq3urWk9Gz237DsQm
    - /ip4/222.222.222.222/tcp/9096/ipfs/QmUGXgnUcqgZf9Js7GrvFP1uz7G6soq3urWk9Gz237DsQm
    - /p2p-circuit/ipfs/QmUGXgnUcqgZf9Js7GrvFP1uz7G6soq3urWk9Gz237DsQm
  > IPFS: QmSPDCbxBSq7PYAnemABL7VpGRkpqBX9NT1AovEmzaXkvM
    - /ip4/127.0.0.1/tcp/4001/ipfs/QmSPDCbxBSq7PYAnemABL7VpGRkpqBX9NT1AovEmzaXkvM
    - /ip4/222.222.222.222/tcp/4001/ipfs/QmSPDCbxBSq7PYAnemABL7VpGRkpqBX9NT1AovEmzaXkvM
    - /ip6/::1/tcp/4001/ipfs/QmSPDCbxBSq7PYAnemABL7VpGRkpqBX9NT1AovEmzaXkvM
```

You also can run **ipfs-cluster-service** as a slave cluster node with following command:

```shell
$ ipfs-cluster-service daemon --bootstrap /ip4/222.222.222.222/tcp/9096/ipfs/QmNTD6Zbhdao
DmjQqGJrp8dKEPvtBQGzRxwWHNcmvNYsbK &
```

## Contribution

We welcome contributions to the Elastos Hive Project.

## Acknowledgments

A sincere thank you to all teams and projects that we rely on directly or indirectly.

## License
This project is licensed under the terms of the [MIT license](https://github.com/elastos/Elastos.Hive.Cluster/blob/master/LICENSE).
