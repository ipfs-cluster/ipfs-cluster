RFC: Follower cluster peers
===========================


1. Per https://github.com/ipfs/ipfs-cluster/pull/710/files we will introduce
   RPC authorization policies:

  * `raft` policy is still pretty much fully open (all endpoints that can
  potentially be used by someone else are open to everyone who has the `cluster secret`
  * `crdtsoft` allows trusted peers to perform sync/recover/block put etc on any other peer
  * `crdtstrict` allows only read-only methods to trusted peers and almost nothing to the rest (connect swarms (?))

Running with `crdtsoft` policy where all peers trust all the other peers
should be equivalent to running with `raft` policy (right?).

Regarding the configuration:

* `rpc_auth_policy` key will be introduced in the main `cluster` section
* `trusted_peers` array for trusted peer IDs will be introduced in the main `cluster` section.

This brings the question:

##  Does it make sense to make `raft` policy selectable (with a different name) for "trusted" environments running with CRDTS (so that the user does not need to add/rm trusted peers when they join/leave the cluster?

I'd say yes. Options:

* We can settle in 3 `rpc_auth_policy`: `open`, `trusted` and `strict`. Unless
  defined, `ipfs-cluster-service` will run with `open` for raft and `trusted`
  for crdts.
* We remove the `open` option and allow a `trusted_peers: all` setting
  (default?). Maybe even letting `trusted_peers` be a subkey of the
  `rpc_auth_policy`.
* More? Overall I'm not too happy here. Maybe we can hide `rpc_auth_policy` completely:
  * When trusted_peers == all -> we run with `trusted`
  * When trusted_peers has some peer but peer started with
    `ipfs-cluster-service daemon` we use `trusted` (raft users should never
    set `trusted_peers`.
  * When we start with `ipfs-cluster-service follow` we use `strict`.

More options?

---

2. We are separating `ID+PrivateKey` into a separate `identity.json`
   file. https://github.com/ipfs/ipfs-cluster/pull/766

---

3. `ipfs-cluster-service --config xxx` (or without the flag when using a
   default config location) will autogenerate an identity when it does not
   exist in `~/ipfs-cluster/` nor in the existing configuration.

---

4. Support for remote configs is added via `ipfs-cluster-service --config
   <ipfspath>` so that `ipfs-cluster-service --config /ipfs/xxx daemon` will:
  * Create a default ipfs connector config and apply env vars (to let the user
    modify the node multiaddress)
  * Fetch the config file from IPFS
  * Use that to initqialize the IPFS Connector
  * Use the identity and the fetched configuration to launch the cluster peer
  * Auto-generate and store an identity if no existing one can be found
  * Note that the peer does not store the configuration on disk. It just runs
    with it.
  * It would be good to support `dnslink` here too, somehow.

---

5. On the same line, `ipfs-cluster-service init --from /ipfs/xxx` downloads a
   config to `~/.ipfs-cluster`

---

6. The cluster section in the configuration re-introduces a `bootstrappers`
   key. These bootstrappers are used when:
  * No `--bootstrap` flag is provided
  * Starting `raft` for the first time (but not when a raft state exists).
  * Always when using CRDTs.

---

7. Default IPFS bootstrappers are used when running with CRDTs and no
   `network_key` is set (or `network_key` forcefully not unset)

---

8. `ipfs-cluster-service follow` will be a shorthand to launch
   `ipfs-cluster-service --consensus crdt` with a `strict` RPC auth policy. We
   aim to be able to do `ipfs-cluster-service --config /ipns/something follow`
   where the used config is a template correctly pre-filled with
   `trusted_peers` and `bootstrappers` (and/or secret)

---

9. We remove the `secret` from the main `cluster` configuration section and
   move it to the `raft` (visible, same usage as now, with warning when not
   set).

---

10. We add a hidden `network_key` to the `crdt` configuration section. When
    unset, it is automatically set by cluster by hashing the
    `ClusterName`. When set to `""` then no private network key is used at
    all, this allows to run CRDT clusters in the public network. This means
    that by default CRDT runs in a private network.
    
