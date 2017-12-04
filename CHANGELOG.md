# ipfs-cluster changelog

### v0.3.1 - 2017-12-11

This release includes changes around the consensus state management, so that upgrades can be performed when the internal format changes. It also comes with several features and changes to support a live deployment and integration with IPFS pin-bot, including a REST API client for Go.

* Features
 * `ipfs-cluster-service state upgrade` | [ipfs/ipfs-cluster#194](https://github.com/ipfs/ipfs-cluster/issues/194)
 * `ipfs-cluster-test` Docker image runs with `ipfs:master` | [ipfs/ipfs-cluster#155](https://github.com/ipfs/ipfs-cluster/issues/155) | [ipfs/ipfs-cluster#259](https://github.com/ipfs/ipfs-cluster/issues/259)
 * `ipfs-cluster` Docker image only runs `ipfs-cluster-service` (and not the ipfs daemon anymore) | [ipfs/ipfs-cluster#197](https://github.com/ipfs/ipfs-cluster/issues/197) | [ipfs/ipfs-cluster#155](https://github.com/ipfs/ipfs-cluster/issues/155) | [ipfs/ipfs-cluster#259](https://github.com/ipfs/ipfs-cluster/issues/259)
 * Support for DNS multiaddresses for cluster peers | [ipfs/ipfs-cluster#155](https://github.com/ipfs/ipfs-cluster/issues/155) | [ipfs/ipfs-cluster#259](https://github.com/ipfs/ipfs-cluster/issues/259)
 * Add configuration section and options for `pin_tracker` | [ipfs/ipfs-cluster#155](https://github.com/ipfs/ipfs-cluster/issues/155) | [ipfs/ipfs-cluster#259](https://github.com/ipfs/ipfs-cluster/issues/259)
 * Add `local` flag to Status, Sync, Recover endpoints which allows to run this operations only in the peer receiving the request | [ipfs/ipfs-cluster#155](https://github.com/ipfs/ipfs-cluster/issues/155) | [ipfs/ipfs-cluster#259](https://github.com/ipfs/ipfs-cluster/issues/259)
 * Add Pin names | [ipfs/ipfs-cluster#249](https://github.com/ipfs/ipfs-cluster/issues/249)
 * Add Peer names | [ipfs/ipfs-cluster#250](https://github.com/ipfs/ipfs-cluster/issues/250)
 * New REST API Client module `github.com/ipfs/ipfs-cluster/api/rest/client` allows to integrate against cluster | [ipfs/ipfs-cluster#260](https://github.com/ipfs/ipfs-cluster/issues/260) | [ipfs/ipfs-cluster#263](https://github.com/ipfs/ipfs-cluster/issues/263) | [ipfs/ipfs-cluster#266](https://github.com/ipfs/ipfs-cluster/issues/266)
 * A few rounds addressing code quality issues | [ipfs/ipfs-cluster#264](https://github.com/ipfs/ipfs-cluster/issues/264)

This release should stay backwards compatible with the previous one. Nevertheless, some REST API endpoints take the `local` flag, and matching new Go public functions have been added (`RecoverAllLocal`, `SyncAllLocal`...).

### v0.3.0 - 2017-11-15

This release introduces Raft 1.0.0 and incorporates deep changes to the management of the cluster peerset.

* Features
  * Upgrade Raft to 1.0.0 | [ipfs/ipfs-cluster#194](https://github.com/ipfs/ipfs-cluster/issues/194) | [ipfs/ipfs-cluster#196](https://github.com/ipfs/ipfs-cluster/issues/196)
  * Support Snaps | [ipfs/ipfs-cluster#234](https://github.com/ipfs/ipfs-cluster/issues/234) | [ipfs/ipfs-cluster#228](https://github.com/ipfs/ipfs-cluster/issues/228) | [ipfs/ipfs-cluster#232](https://github.com/ipfs/ipfs-cluster/issues/232)
  * Rotating backups for ipfs-cluster-data | [ipfs/ipfs-cluster#233](https://github.com/ipfs/ipfs-cluster/issues/233)
  * Bring documentation up to date with the code [ipfs/ipfs-cluster#223](https://github.com/ipfs/ipfs-cluster/issues/223)

Bugfixes:
  * Fix docker startup | [ipfs/ipfs-cluster#216](https://github.com/ipfs/ipfs-cluster/issues/216) | [ipfs/ipfs-cluster#217](https://github.com/ipfs/ipfs-cluster/issues/217)
  * Fix configuration save | [ipfs/ipfs-cluster#213](https://github.com/ipfs/ipfs-cluster/issues/213) | [ipfs/ipfs-cluster#214](https://github.com/ipfs/ipfs-cluster/issues/214)
  * Forward progress updates with IPFS-Proxy | [ipfs/ipfs-cluster#224](https://github.com/ipfs/ipfs-cluster/issues/224) | [ipfs/ipfs-cluster#231](https://github.com/ipfs/ipfs-cluster/issues/231)
  * Delay ipfs connect swarms on boot and safeguard against panic condition | [ipfs/ipfs-cluster#238](https://github.com/ipfs/ipfs-cluster/issues/238)
  * Multiple minor fixes | [ipfs/ipfs-cluster#236](https://github.com/ipfs/ipfs-cluster/issues/236)
    * Avoid shutting down consensus in the middle of a commit
    * Return an ID containing current peers in PeerAdd
    * Do not shut down libp2p host in the middle of peer removal
    * Send cluster addresses to the new peer before adding it
    * Wait for configuration save on init
    * Fix error message when not enough allocations exist for a pin

This releases introduces some changes affecting the configuration file and some breaking changes affecting `go` and the REST APIs:

* The `consensus.raft` section of the configuration has new options but should be backwards compatible.
* The `Consensus` component interface has changed, `LogAddPeer` and `LogRmPeer` have been replaced by `AddPeer` and `RmPeer`. It additionally provides `Clean` and `Peers` methods. The `consensus/raft` implementation has been updated accordingly.
* The `api.ID` (used in REST API among others) object key `ClusterPeers` key is now a list of peer IDs, and not a list of multiaddresses as before. The object includes a new key `ClusterPeersAddresses` which includes the multiaddresses.
* Note that `--bootstrap` and `--leave` flags when calling `ipfs-cluster-service` will be stored permanently in the configuration (see [ipfs/ipfs-cluster#235](https://github.com/ipfs/ipfs-cluster/issues/235)).

---

### v0.2.1 - 2017-10-26

This is a maintenance release with some important bugfixes.

* Fixes:
  * Dockerfile runs `ipfs-cluster-service` instead of `ctl` | [ipfs/ipfs-cluster#194](https://github.com/ipfs/ipfs-cluster/issues/194) | [ipfs/ipfs-cluster#196](https://github.com/ipfs/ipfs-cluster/issues/196)
  * Peers and bootstrap entries in the configuration are ignored | [ipfs/ipfs-cluster#203](https://github.com/ipfs/ipfs-cluster/issues/203) | [ipfs/ipfs-cluster#204](https://github.com/ipfs/ipfs-cluster/issues/204)
  * Informers do not work on 32-bit architectures | [ipfs/ipfs-cluster#202](https://github.com/ipfs/ipfs-cluster/issues/202) | [ipfs/ipfs-cluster#205](https://github.com/ipfs/ipfs-cluster/issues/205)
  * Replication factor entry in the configuration is ignored | [ipfs/ipfs-cluster#208](https://github.com/ipfs/ipfs-cluster/issues/208) | [ipfs/ipfs-cluster#209](https://github.com/ipfs/ipfs-cluster/issues/209)

The fix for 32-bit architectures has required a change in the `IPFSConnector` interface (`FreeSpace()` and `Reposize()` return `uint64` now). The current implementation by the `ipfshttp` module has changed accordingly.


---

### v0.2.0 - 2017-10-23

* Features:
  * Basic authentication support added to API component | [ipfs/ipfs-cluster#121](https://github.com/ipfs/ipfs-cluster/issues/121) | [ipfs/ipfs-cluster#147](https://github.com/ipfs/ipfs-cluster/issues/147) | [ipfs/ipfs-cluster#179](https://github.com/ipfs/ipfs-cluster/issues/179)
  * Copy peers to bootstrap when leaving a cluster | [ipfs/ipfs-cluster#170](https://github.com/ipfs/ipfs-cluster/issues/170) | [ipfs/ipfs-cluster#112](https://github.com/ipfs/ipfs-cluster/issues/112)
  * New configuration format | [ipfs/ipfs-cluster#162](https://github.com/ipfs/ipfs-cluster/issues/162) | [ipfs/ipfs-cluster#177](https://github.com/ipfs/ipfs-cluster/issues/177)
  * Freespace disk metric implementation. It's now the default. | [ipfs/ipfs-cluster#142](https://github.com/ipfs/ipfs-cluster/issues/142) | [ipfs/ipfs-cluster#99](https://github.com/ipfs/ipfs-cluster/issues/99)

* Fixes:
  * IPFS Connector should use only POST | [ipfs/ipfs-cluster#176](https://github.com/ipfs/ipfs-cluster/issues/176) | [ipfs/ipfs-cluster#161](https://github.com/ipfs/ipfs-cluster/issues/161)
  * `ipfs-cluster-ctl` exit status with error responses | [ipfs/ipfs-cluster#174](https://github.com/ipfs/ipfs-cluster/issues/174)
  * Sharness tests and update testing container | [ipfs/ipfs-cluster#171](https://github.com/ipfs/ipfs-cluster/issues/171)
  * Update Dockerfiles | [ipfs/ipfs-cluster#154](https://github.com/ipfs/ipfs-cluster/issues/154) | [ipfs/ipfs-cluster#185](https://github.com/ipfs/ipfs-cluster/issues/185)
  * `ipfs-cluster-service`: Do not run service with unknown subcommands | [ipfs/ipfs-cluster#186](https://github.com/ipfs/ipfs-cluster/issues/186)

This release introduces some breaking changes affecting configuration files and `go` integrations:

* Config: The old configuration format is no longer valid and cluster will fail to start from it. Configuration file needs to be re-initialized with `ipfs-cluster-service init`.
* Go: The `restapi` component has been renamed to `rest` and some of its public methods have been renamed.
* Go: Initializers (`New<Component>(...)`) for most components have changed to accept a `Config` object. Some initializers have been removed.

---

Note, when adding changelog entries, write links to issues as `@<issuenumber>` and then replace them with links with the following command:

```
sed -i -r 's/@([0-9]+)/[ipfs\/ipfs-cluster#\1](https:\/\/github.com\/ipfs\/ipfs-cluster\/issues\/\1)/g' CHANGELOG.md
```
