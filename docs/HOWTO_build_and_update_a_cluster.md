# Building and updating an IPFS Cluster

### Step 0: Run your first cluster node

This step creates a single-node IPFS Cluster.

First, create a secret that we will use for all cluster peers:

```
node0 $ export CLUSTER_SECRET=$(od  -vN 32 -An -tx1 /dev/urandom | tr -d ' \n')
node0 $ echo $CLUSTER_SECRET
9a420ec947512b8836d8eb46e1c56fdb746ab8a78015b9821e6b46b38344038f
```

Second initialize the configuration (see [the **Initialization** section of the
`ipfs-cluster-service` README](../ipfs-cluster-service/dist/README.md#initialization) for more details).

```
node0 $ ipfs-cluster-service init
INFO     config: Saving configuration config.go:311
ipfs-cluster-service configuration written to /home/hector/.ipfs-cluster/service.json
```

Then run cluster:

```
node0> ipfs-cluster-service
INFO    cluster: IPFS Cluster v0.3.0 listening on: cluster.go:91
INFO    cluster:         /ip4/127.0.0.1/tcp/9096/ipfs/QmZjSoXUQgJ9tutP1rXjjNYwTrRM9QPhmD9GHVjbtgWxEn cluster.go:93
INFO    cluster:         /ip4/192.168.1.2/tcp/9096/ipfs/QmZjSoXUQgJ9tutP1rXjjNYwTrRM9QPhmD9GHVjbtgWxEn cluster.go:93
INFO  consensus: starting Consensus and waiting for a leader... consensus.go:61
INFO  consensus: bootstrapping raft cluster raft.go:108
INFO    restapi: REST API: /ip4/127.0.0.1/tcp/9094 restapi.go:266
INFO   ipfshttp: IPFS Proxy: /ip4/127.0.0.1/tcp/9095 -> /ip4/127.0.0.1/tcp/5001 ipfshttp.go:159
INFO  consensus: Raft Leader elected: QmZjSoXUQgJ9tutP1rXjjNYwTrRM9QPhmD9GHVjbtgWxEn raft.go:261
INFO  consensus: Raft state is catching up raft.go:273
INFO  consensus: Consensus state is up to date consensus.go:116
INFO    cluster: Cluster Peers (without including ourselves): cluster.go:450
INFO    cluster:     - No other peers cluster.go:452
INFO    cluster: IPFS Cluster is ready cluster.go:461
```

### Step 1: Add new members to the cluster

Initialize and run cluster in a different node(s), bootstrapping them to the first node:

```
node1> export CLUSTER_SECRET=<copy from node0>
node1> ipfs-cluster-service init
INFO     config: Saving configuration config.go:311
ipfs-cluster-service configuration written to /home/hector/.ipfs-cluster/service.json
node1> ipfs-cluster-service --bootstrap /ip4/192.168.1.2/tcp/9096/ipfs/QmZjSoXUQgJ9tutP1rXjjNYwTrRM9QPhmD9GHVjbtgWxEn
INFO    cluster: IPFS Cluster v0.3.0 listening on: cluster.go:91
INFO    cluster:         /ip4/127.0.0.1/tcp/10096/ipfs/QmYFYwnFUkjFhJcSJJGN72wwedZnpQQ4aNpAtPZt8g5fCd cluster.go:93
INFO    cluster:         /ip4/192.168.1.3/tcp/10096/ipfs/QmYFYwnFUkjFhJcSJJGN72wwedZnpQQ4aNpAtPZt8g5fCd cluster.go:93
INFO  consensus: starting Consensus and waiting for a leader... consensus.go:61
INFO  consensus: bootstrapping raft cluster raft.go:108
INFO    cluster: Bootstrapping to /ip4/127.0.0.1/tcp/9096/ipfs/QmZjSoXUQgJ9tutP1rXjjNYwTrRM9QPhmD9GHVjbtgWxEn cluster.go:471
INFO    restapi: REST API: /ip4/127.0.0.1/tcp/10094 restapi.go:266
INFO   ipfshttp: IPFS Proxy: /ip4/127.0.0.1/tcp/10095 -> /ip4/127.0.0.1/tcp/5001 ipfshttp.go:159
INFO  consensus: Raft Leader elected: QmZjSoXUQgJ9tutP1rXjjNYwTrRM9QPhmD9GHVjbtgWxEn raft.go:261
INFO  consensus: Raft state is catching up raft.go:273
INFO  consensus: Consensus state is up to date consensus.go:116
INFO  consensus: Raft Leader elected: QmZjSoXUQgJ9tutP1rXjjNYwTrRM9QPhmD9GHVjbtgWxEn raft.go:261
INFO  consensus: Raft state is catching up raft.go:273
INFO    cluster: QmYFYwnFUkjFhJcSJJGN72wwedZnpQQ4aNpAtPZt8g5fCd: joined QmZjSoXUQgJ9tutP1rXjjNYwTrRM9QPhmD9GHVjbtgWxEn's cluster cluster.go:777
INFO    cluster: Cluster Peers (without including ourselves): cluster.go:450
INFO    cluster:     - QmZjSoXUQgJ9tutP1rXjjNYwTrRM9QPhmD9GHVjbtgWxEn cluster.go:456
INFO    cluster: IPFS Cluster is ready cluster.go:461
INFO     config: Saving configuration config.go:311
```

You can repeat the process with any other nodes.

You can check the current list of cluster peers and see it shows 2 peers:

```
node1 > ipfs-cluster-ctl peers ls
QmYFYwnFUkjFhJcSJJGN72wwedZnpQQ4aNpAtPZt8g5fCd | Sees 1 other peers
  > Addresses:
    - /ip4/127.0.0.1/tcp/10096/ipfs/QmYFYwnFUkjFhJcSJJGN72wwedZnpQQ4aNpAtPZt8g5fCd
    - /ip4/192.168.1.3/tcp/10096/ipfs/QmYFYwnFUkjFhJcSJJGN72wwedZnpQQ4aNpAtPZt8g5fCd
  > IPFS: Qmaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
    - /ip4/127.0.0.1/tcp/4001/ipfs/Qmaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
    - /ip4/192.168.1.3/tcp/4001/ipfs/Qmaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
QmZjSoXUQgJ9tutP1rXjjNYwTrRM9QPhmD9GHVjbtgWxEn | Sees 1 other peers
  > Addresses:
    - /ip4/127.0.0.1/tcp/9096/ipfs/QmZjSoXUQgJ9tutP1rXjjNYwTrRM9QPhmD9GHVjbtgWxEn
    - /ip4/192.168.1.2/tcp/9096/ipfs/QmZjSoXUQgJ9tutP1rXjjNYwTrRM9QPhmD9GHVjbtgWxEn
  > IPFS: Qmbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
    - /ip4/127.0.0.1/tcp/4001/ipfs/Qmbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
    - /ip4/192.168.1.2/tcp/4001/ipfs/Qmbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
```

### Step 2: Remove no longer needed nodes

You can use `ipfs-cluster-ctl peers rm <peer_id>` to remove and disconnect any nodes from your cluster. The nodes will be automatically
shutdown. They can be restarted manually and re-added to the Cluster any time:

```
node0> ipfs-cluster-ctl peers rm QmYFYwnFUkjFhJcSJJGN72wwedZnpQQ4aNpAtPZt8g5fCd
```

The `node1` is then disconnected and shuts down, as its logs show:

```
INFO     config: Saving configuration config.go:311
INFO    cluster: this peer has been removed and will shutdown cluster.go:386
INFO    cluster: shutting down Cluster cluster.go:498
INFO  consensus: stopping Consensus component consensus.go:159
INFO  consensus: consensus data cleaned consensus.go:400
INFO    monitor: stopping Monitor peer_monitor.go:161
INFO    restapi: stopping Cluster API restapi.go:284
INFO   ipfshttp: stopping IPFS Proxy ipfshttp.go:534
INFO pintracker: stopping MapPinTracker maptracker.go:116
INFO     config: Saving configuration config.go:311
```
