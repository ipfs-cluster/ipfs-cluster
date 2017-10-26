# ipfs-cluster changelog

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
