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
  - [`ipfs-cluster-service`](#ipfs-cluster-service)
  - [`ipfs-cluster-ctl`](#ipfs-cluster-ctl)
  - [Quick start: Building and updating an IPFS Cluster](#quick-start-building-and-updating-an-ipfs-cluster)
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

You can add the multiaddresses for the other cluster peers the `bootstrap_multiaddresses` variable. For example, here is a valid configuration for a single-peer cluster:

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
    "consensus_data_folder": "/home/hector/go/src/github.com/ipfs/ipfs-cluster/ipfs-cluster-service/d1/data",
    "state_sync_seconds": 60
}
```

The configuration file should probably be identical among all cluster peers, except for the `id` and `private_key` fields. Once every cluster peer has the configuration in place, you can run `ipfs-cluster-service` to start the cluster.

#### Clusters using `cluster_peers`

The `cluster_peers` configuration variable holds a list of current cluster members. If you know the members of the cluster in advance, or you want to start a cluster fully in parallel, set `cluster_peers` in all configurations so that every peer knows the rest upon boot. Leave `bootstrap` empty (although it will be ignored anyway)

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

### Quick start: Building and updating an IPFS Cluster

#### Step 0: Run your first cluster node

This step creates a single-node IPFS Cluster.

First initialize the configuration:

```
node0 $ ipfs-cluster-service init
ipfs-cluster-service configuration written to /home/user/.ipfs-cluster/service.json
```

Then run cluster:

```
node0> ipfs-cluster-service
13:33:34.044  INFO    cluster: IPFS Cluster v0.0.1 listening on: cluster.go:59
13:33:34.044  INFO    cluster:         /ip4/127.0.0.1/tcp/9096/ipfs/QmQHKLBXfS7hf8o2acj7FGADoJDLat3UazucbHrgxqisim cluster.go:61
13:33:34.044  INFO    cluster:         /ip4/192.168.1.103/tcp/9096/ipfs/QmQHKLBXfS7hf8o2acj7FGADoJDLat3UazucbHrgxqisim cluster.go:61
13:33:34.044  INFO    cluster: starting Consensus and waiting for a leader... consensus.go:163
13:33:34.047  INFO    cluster: PinTracker ready map_pin_tracker.go:71
13:33:34.047  INFO    cluster: waiting for leader raft.go:118
13:33:34.047  INFO    cluster: REST API: /ip4/127.0.0.1/tcp/9094 rest_api.go:309
13:33:34.047  INFO    cluster: IPFS Proxy: /ip4/127.0.0.1/tcp/9095 -> /ip4/127.0.0.1/tcp/5001 ipfs_http_connector.go:168
13:33:35.420  INFO    cluster: Raft Leader elected: QmQHKLBXfS7hf8o2acj7FGADoJDLat3UazucbHrgxqisim raft.go:145
13:33:35.921  INFO    cluster: Consensus state is up to date consensus.go:214
13:33:35.921  INFO    cluster: IPFS Cluster is ready cluster.go:191
13:33:35.921  INFO    cluster: Cluster Peers (not including ourselves): cluster.go:192
13:33:35.921  INFO    cluster:     - No other peers cluster.go:195
```

#### Step 1: Add new members to the cluster

Initialize and run cluster in a different node(s):

```
node1> ipfs-cluster-service init
ipfs-cluster-service configuration written to /home/user/.ipfs-cluster/service.json
node1> ipfs-cluster-service
13:36:19.313  INFO    cluster: IPFS Cluster v0.0.1 listening on: cluster.go:59
13:36:19.313  INFO    cluster:         /ip4/127.0.0.1/tcp/7096/ipfs/QmU7JJftGsP1zM8H37XwvfA7dwdepsB7xVnJA6sKpozc85 cluster.go:61
13:36:19.313  INFO    cluster:         /ip4/192.168.1.103/tcp/7096/ipfs/QmU7JJftGsP1zM8H37XwvfA7dwdepsB7xVnJA6sKpozc85 cluster.go:61
13:36:19.313  INFO    cluster: starting Consensus and waiting for a leader... consensus.go:163
13:36:19.316  INFO    cluster: REST API: /ip4/127.0.0.1/tcp/7094 rest_api.go:309
13:36:19.316  INFO    cluster: IPFS Proxy: /ip4/127.0.0.1/tcp/7095 -> /ip4/127.0.0.1/tcp/5001 ipfs_http_connector.go:168
13:36:19.316  INFO    cluster: waiting for leader raft.go:118
13:36:19.316  INFO    cluster: PinTracker ready map_pin_tracker.go:71
13:36:20.834  INFO    cluster: Raft Leader elected: QmU7JJftGsP1zM8H37XwvfA7dwdepsB7xVnJA6sKpozc85 raft.go:145
13:36:21.334  INFO    cluster: Consensus state is up to date consensus.go:214
13:36:21.334  INFO    cluster: IPFS Cluster is ready cluster.go:191
13:36:21.334  INFO    cluster: Cluster Peers (not including ourselves): cluster.go:192
13:36:21.334  INFO    cluster:     - No other peers cluster.go:195
```

Add them to the original cluster with `ipfs-cluster-ctl peers add <multiaddr>`. The command will return the ID information of the newly added member:

```
node0> ipfs-cluster-ctl peers add /ip4/192.168.1.103/tcp/7096/ipfs/QmU7JJftGsP1zM8H37XwvfA7dwdepsB7xVnJA6sKpozc85
{
  "id": "QmU7JJftGsP1zM8H37XwvfA7dwdepsB7xVnJA6sKpozc85",
  "public_key": "CAASpgIwggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIBAQDtjpvI+XKVGT5toXTimtWceONYsf/1bbRMxLt/fCSYJoSeJqj0HUtttCD3dcBv1M2rElIMXDhyLUpkET+AN6otr9lQnbgi0ZaKrtzphR0w6g/0EQZZaxI2scxF4NcwkwUfe5ceEmPFwax1+C00nd2BF+YEEp+VHNyWgXhCxncOGO74p0YdXBrvXkyfTiy/567L3PPX9F9x+HiutBL39CWhx9INmtvdPB2HwshodF6QbfeljdAYCekgIrCQC8mXOVeePmlWgTwoge9yQbuViZwPiKwwo1AplANXFmSv8gagfjKL7Kc0YOqcHwxBsoUskbJjfheDZJzl19iDs9EvUGk5AgMBAAE=",
  "addresses": [
    "/ip4/127.0.0.1/tcp/7096/ipfs/QmU7JJftGsP1zM8H37XwvfA7dwdepsB7xVnJA6sKpozc85",
    "/ip4/192.168.1.103/tcp/7096/ipfs/QmU7JJftGsP1zM8H37XwvfA7dwdepsB7xVnJA6sKpozc85"
  ],
  "cluster_peers": [
    "/ip4/192.168.123.103/tcp/7096/ipfs/QmU7JJftGsP1zM8H37XwvfA7dwdepsB7xVnJA6sKpozc85"
  ],
  "version": "0.0.1",
  "commit": "83baa5c859b9b17b2deec4f782d1210590025c80",
  "rpc_protocol_version": "/ipfscluster/0.0.1/rpc",
  "ipfs": {
    "id": "QmXZrtE5jQwXNqCJMfHUTQkvhQ4ZAnqMnmzFMJfLewur2n",
    "addresses": [
      "/ip4/127.0.0.1/tcp/4001/ipfs/QmXZrtE5jQwXNqCJMfHUTQkvhQ4ZAnqMnmzFMJfLewur2n",
      "/ip4/192.168.1.103/tcp/4001/ipfs/QmXZrtE5jQwXNqCJMfHUTQkvhQ4ZAnqMnmzFMJfLewur2n"
    ]
  }
}
```

You can repeat the process with any other nodes.

#### Step 3: Remove no longer needed nodes

You can use `ipfs-cluster-ctl peers rm <multiaddr>` to remove and disconnect any nodes from your cluster. The nodes will be automatically
shutdown. They can be restarted manually and re-added to the Cluster any time:

```
node0> ipfs-cluster-ctl peers rm QmbGFbZVTF3UAEPK9pBVdwHGdDAYkHYufQwSh4k1i8bbbb
Request succeeded
```

The `node1` is then disconnected and shuts down:

```
13:42:50.828 WARNI    cluster: this peer has been removed from the Cluster and will shutdown itself in 5 seconds peer_manager.go:48
13:42:51.828  INFO    cluster: stopping Consensus component consensus.go:257
13:42:55.836  INFO    cluster: shutting down IPFS Cluster cluster.go:235
13:42:55.836  INFO    cluster: Saving configuration config.go:283
13:42:55.837  INFO    cluster: stopping Cluster API rest_api.go:327
13:42:55.837  INFO    cluster: stopping IPFS Proxy ipfs_http_connector.go:332
13:42:55.837  INFO    cluster: stopping MapPinTracker map_pin_tracker.go:87
```

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


## Contribute

PRs accepted.

Small note: If editing the README, please conform to the [standard-readme](https://github.com/RichardLitt/standard-readme) specification.

## License

MIT © Protocol Labs, Inc.
