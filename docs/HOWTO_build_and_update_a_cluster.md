# Building and updating an IPFS Cluster

### Step 0: Run your first cluster node

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

### Step 1: Add new members to the cluster

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

### Step 2: Remove no longer needed nodes

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
