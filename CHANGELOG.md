# IPFS Cluster Changelog

### v0.7.0 - 2018-10-25

#### Summary

IPFS version 0.7.0 is a maintenance release that includes a few bugfixes and some small features.

Note that the REST API response format for the `/add` endpoint has changed. Thus all clients need to be upgraded to deal with the new format. The `rest/api/client` has been accordingly updated.

#### List of changes

##### Features

  * Clean (rotate) the state when running `init` | [ipfs/ipfs-cluster#532](https://github.com/ipfs/ipfs-cluster/issues/532) | [ipfs/ipfs-cluster#553](https://github.com/ipfs/ipfs-cluster/issues/553)
  * Configurable REST API headers and CORS defaults | [ipfs/ipfs-cluster#578](https://github.com/ipfs/ipfs-cluster/issues/578)
  * Upgrade libp2p and other deps | [ipfs/ipfs-cluster#580](https://github.com/ipfs/ipfs-cluster/issues/580) | [ipfs/ipfs-cluster#590](https://github.com/ipfs/ipfs-cluster/issues/590) | [ipfs/ipfs-cluster#592](https://github.com/ipfs/ipfs-cluster/issues/592) | [ipfs/ipfs-cluster#598](https://github.com/ipfs/ipfs-cluster/issues/598) | [ipfs/ipfs-cluster#599](https://github.com/ipfs/ipfs-cluster/issues/599)
  * Use `gossipsub` to broadcast metrics | [ipfs/ipfs-cluster#573](https://github.com/ipfs/ipfs-cluster/issues/573)
  * Download gx and gx-go from IPFS preferentially | [ipfs/ipfs-cluster#577](https://github.com/ipfs/ipfs-cluster/issues/577) | [ipfs/ipfs-cluster#581](https://github.com/ipfs/ipfs-cluster/issues/581)
  * Expose peer metrics in the API + ctl commands | [ipfs/ipfs-cluster#449](https://github.com/ipfs/ipfs-cluster/issues/449) | [ipfs/ipfs-cluster#572](https://github.com/ipfs/ipfs-cluster/issues/572) | [ipfs/ipfs-cluster#589](https://github.com/ipfs/ipfs-cluster/issues/589) | [ipfs/ipfs-cluster#587](https://github.com/ipfs/ipfs-cluster/issues/587)
  * Add a `docker-compose.yml` template, which creates a two peer cluster | [ipfs/ipfs-cluster#585](https://github.com/ipfs/ipfs-cluster/issues/585) | [ipfs/ipfs-cluster#588](https://github.com/ipfs/ipfs-cluster/issues/588)
  * Support overwriting configuration values in the `cluster` section with environmental values | [ipfs/ipfs-cluster#575](https://github.com/ipfs/ipfs-cluster/issues/575) | [ipfs/ipfs-cluster#596](https://github.com/ipfs/ipfs-cluster/issues/596)
  * Set snaps to `classic` confinement mode and revert it since approval never arrived | [ipfs/ipfs-cluster#579](https://github.com/ipfs/ipfs-cluster/issues/579) | [ipfs/ipfs-cluster#594](https://github.com/ipfs/ipfs-cluster/issues/594)
* Use Go's reverse proxy library in the proxy endpoint | [ipfs/ipfs-cluster#570](https://github.com/ipfs/ipfs-cluster/issues/570) | [ipfs/ipfs-cluster#605](https://github.com/ipfs/ipfs-cluster/issues/605)
  

##### Bug fixes

  * `/add` endpoints improvements and IPFS Companion compatiblity | [ipfs/ipfs-cluster#582](https://github.com/ipfs/ipfs-cluster/issues/582) | [ipfs/ipfs-cluster#569](https://github.com/ipfs/ipfs-cluster/issues/569)
  * Fix adding with spaces in the name parameter | [ipfs/ipfs-cluster#583](https://github.com/ipfs/ipfs-cluster/issues/583)
  * Escape filter query parameter | [ipfs/ipfs-cluster#586](https://github.com/ipfs/ipfs-cluster/issues/586)
  * Fix some race conditions | [ipfs/ipfs-cluster#597](https://github.com/ipfs/ipfs-cluster/issues/597)
  * Improve pin deserialization efficiency | [ipfs/ipfs-cluster#601](https://github.com/ipfs/ipfs-cluster/issues/601)
  * Do not error remote pins | [ipfs/ipfs-cluster#600](https://github.com/ipfs/ipfs-cluster/issues/600) | [ipfs/ipfs-cluster#603](https://github.com/ipfs/ipfs-cluster/issues/603)
  * Clean up testing folders in `rest` and `rest/client` after tests | [ipfs/ipfs-cluster#607](https://github.com/ipfs/ipfs-cluster/issues/607)

#### Upgrading notices

##### Configuration changes

The configurations from previous versions are compatible, but a new `headers` key has been added to the `restapi` section. By default it gets CORS headers which will allow read-only interaction from any origin.

Additionally, all fields from the main `cluster` configuration section can now be overwrriten with environment variables. i.e. `CLUSTER_SECRET`, or  `CLUSTER_DISABLEREPINNING`.

##### REST API

The `/add` endpoint stream now returns different objects, in line with the rest of the API types.

Before:

```
type AddedOutput struct {
	Error
	Name  string
	Hash  string `json:",omitempty"`
	Bytes int64  `json:",omitempty"`
	Size  string `json:",omitempty"`
}
```

Now:

```
type AddedOutput struct {
	Name  string `json:"name"`
	Cid   string `json:"cid,omitempty"`
	Bytes uint64 `json:"bytes,omitempty"`
	Size  uint64 `json:"size,omitempty"`
}
```

The `/add` endpoint no longer reports errors as part of an AddedOutput object, but instead it uses trailer headers (same as `go-ipfs`). They are handled in the `client`.

##### Go APIs

The `AddedOutput` object has changed, thus the `api/rest/client` from older versions will not work with this one.

##### Other

No other things.

---

### v0.6.0 - 2018-10-03

#### Summary

IPFS version 0.6.0 is a new minor release of IPFS Cluster.

We have increased the minor release number to signal changes to the Go APIs after upgrading to the new `cid` package, but, other than that, this release does not include any major changes.

It brings a number of small fixes and features of which we can highlight two useful ones:

* the first is the support for multiple cluster daemon versions in the same cluster, as long as they share the same major/minor release. That means, all releases in the `0.6` series (`0.6.0`, `0.6.1` and so on...) will be able to speak among each others, allowing partial cluster upgrades.
* the second is the inclusion of a `PeerName` key in the status (`PinInfo`) objects. `ipfs-cluster-status` will now show peer names instead of peer IDs, making it easy to identify the status for each peer.

Many thanks to all the contributors to this release: @lanzafame, @meiqimichelle, @kishansagathiya, @cannium, @jglukasik and @mike-ngu.

#### List of changes

##### Features

  * Move commands to the `cmd/` folder | [ipfs/ipfs-cluster#485](https://github.com/ipfs/ipfs-cluster/issues/485) | [ipfs/ipfs-cluster#521](https://github.com/ipfs/ipfs-cluster/issues/521) | [ipfs/ipfs-cluster#556](https://github.com/ipfs/ipfs-cluster/issues/556)
  * Dependency upgrades: `go-dot`, `go-libp2p`, `cid` | [ipfs/ipfs-cluster#533](https://github.com/ipfs/ipfs-cluster/issues/533) | [ipfs/ipfs-cluster#537](https://github.com/ipfs/ipfs-cluster/issues/537) | [ipfs/ipfs-cluster#535](https://github.com/ipfs/ipfs-cluster/issues/535) | [ipfs/ipfs-cluster#544](https://github.com/ipfs/ipfs-cluster/issues/544) | [ipfs/ipfs-cluster#561](https://github.com/ipfs/ipfs-cluster/issues/561)
  * Build with go-1.11 | [ipfs/ipfs-cluster#558](https://github.com/ipfs/ipfs-cluster/issues/558)
  * Peer names in `PinInfo` | [ipfs/ipfs-cluster#446](https://github.com/ipfs/ipfs-cluster/issues/446) | [ipfs/ipfs-cluster#531](https://github.com/ipfs/ipfs-cluster/issues/531)
  * Wrap API client in an interface | [ipfs/ipfs-cluster#447](https://github.com/ipfs/ipfs-cluster/issues/447) | [ipfs/ipfs-cluster#523](https://github.com/ipfs/ipfs-cluster/issues/523) | [ipfs/ipfs-cluster#564](https://github.com/ipfs/ipfs-cluster/issues/564)
  * `Makefile`: add `prcheck` target and fix `make all` | [ipfs/ipfs-cluster#536](https://github.com/ipfs/ipfs-cluster/issues/536) | [ipfs/ipfs-cluster#542](https://github.com/ipfs/ipfs-cluster/issues/542) | [ipfs/ipfs-cluster#539](https://github.com/ipfs/ipfs-cluster/issues/539)
  * Docker: speed up [re]builds | [ipfs/ipfs-cluster#529](https://github.com/ipfs/ipfs-cluster/issues/529)
  * Re-enable keep-alives on servers | [ipfs/ipfs-cluster#548](https://github.com/ipfs/ipfs-cluster/issues/548) | [ipfs/ipfs-cluster#560](https://github.com/ipfs/ipfs-cluster/issues/560)

##### Bugfixes

  * Fix adding to cluster with unhealthy peers | [ipfs/ipfs-cluster#543](https://github.com/ipfs/ipfs-cluster/issues/543) | [ipfs/ipfs-cluster#549](https://github.com/ipfs/ipfs-cluster/issues/549)
  * Fix Snap builds and pushes: multiple architectures re-enabled | [ipfs/ipfs-cluster#520](https://github.com/ipfs/ipfs-cluster/issues/520) | [ipfs/ipfs-cluster#554](https://github.com/ipfs/ipfs-cluster/issues/554) | [ipfs/ipfs-cluster#557](https://github.com/ipfs/ipfs-cluster/issues/557) | [ipfs/ipfs-cluster#562](https://github.com/ipfs/ipfs-cluster/issues/562) | [ipfs/ipfs-cluster#565](https://github.com/ipfs/ipfs-cluster/issues/565)
  * Docs: Typos in Readme and some improvements | [ipfs/ipfs-cluster#547](https://github.com/ipfs/ipfs-cluster/issues/547) | [ipfs/ipfs-cluster#567](https://github.com/ipfs/ipfs-cluster/issues/567)
  * Fix tests in `stateless` PinTracker | [ipfs/ipfs-cluster#552](https://github.com/ipfs/ipfs-cluster/issues/552) | [ipfs/ipfs-cluster#563](https://github.com/ipfs/ipfs-cluster/issues/563)

#### Upgrading notices

##### Configuration changes

There are no changes to the configuration file on this release.

##### REST API

There are no changes to the REST API.

##### Go APIs

We have upgraded to the new version of the `cid` package. This means all `*cid.Cid` arguments are now `cid.Cid`.

##### Other

We are now using `go-1.11` to build and test cluster. We recommend using this version as well when building from source.

---


### v0.5.0 - 2018-08-23

#### Summary

IPFS Cluster version 0.5.0 is a minor release which includes a major feature: **adding content to IPFS directly through Cluster**.

This functionality is provided by `ipfs-cluster-ctl add` and by the API endpoint `/add`. The upload format (multipart) is similar to the IPFS `/add` endpoint, as well as the options (chunker, layout...). Cluster `add` generates the same DAG as `ipfs add` would, but it sends the added blocks directly to their allocations, pinning them on completion. The pin happens very quickly, as content is already locally available in the allocated peers.

The release also includes most of the needed code for the [Sharding feature](https://cluster.ipfs.io/developer/rfcs/dag-sharding-rfc/), but it is not yet usable/enabled, pending features from go-ipfs.

The 0.5.0 release additionally includes a new experimental PinTracker implementation: the `stateless` pin tracker. The stateless pin tracker relies on the IPFS pinset and the cluster state to keep track of pins, rather than keeping an in-memory copy of the cluster pinset, thus reducing the memory usage when having huge pinsets. It can be enabled with `ipfs-cluster-service daemon --pintracker stateless`.

The last major feature is the use of a DHT as routing layer for cluster peers. This means that peers should be able to discover each others as long as they are connected to one cluster peer. This simplifies the setup requirements for starting a cluster and helps avoiding situations which make the cluster unhealthy.

This release requires a state upgrade migration. It can be performed with `ipfs-cluster-service state upgrade` or simply launching the daemon with `ipfs-cluster-service daemon --upgrade`.

#### List of changes

##### Features

  * Libp2p upgrades (up to v6) | [ipfs/ipfs-cluster#456](https://github.com/ipfs/ipfs-cluster/issues/456) | [ipfs/ipfs-cluster#482](https://github.com/ipfs/ipfs-cluster/issues/482)
  * Support `/dns` multiaddresses for `node_multiaddress` | [ipfs/ipfs-cluster#462](https://github.com/ipfs/ipfs-cluster/issues/462) | [ipfs/ipfs-cluster#463](https://github.com/ipfs/ipfs-cluster/issues/463)
  * Increase `state_sync_interval` to 10 minutes | [ipfs/ipfs-cluster#468](https://github.com/ipfs/ipfs-cluster/issues/468) | [ipfs/ipfs-cluster#469](https://github.com/ipfs/ipfs-cluster/issues/469)
  * Auto-interpret libp2p addresses in `rest/client`'s `APIAddr` configuration option | [ipfs/ipfs-cluster#498](https://github.com/ipfs/ipfs-cluster/issues/498)
  * Resolve `APIAddr` (for `/dnsaddr` usage) in `rest/client` | [ipfs/ipfs-cluster#498](https://github.com/ipfs/ipfs-cluster/issues/498)
  * Support for adding content to Cluster and sharding (sharding is disabled) | [ipfs/ipfs-cluster#484](https://github.com/ipfs/ipfs-cluster/issues/484) | [ipfs/ipfs-cluster#503](https://github.com/ipfs/ipfs-cluster/issues/503) | [ipfs/ipfs-cluster#495](https://github.com/ipfs/ipfs-cluster/issues/495) | [ipfs/ipfs-cluster#504](https://github.com/ipfs/ipfs-cluster/issues/504) | [ipfs/ipfs-cluster#509](https://github.com/ipfs/ipfs-cluster/issues/509) | [ipfs/ipfs-cluster#511](https://github.com/ipfs/ipfs-cluster/issues/511) | [ipfs/ipfs-cluster#518](https://github.com/ipfs/ipfs-cluster/issues/518)
  * `stateless` PinTracker [ipfs/ipfs-cluster#308](https://github.com/ipfs/ipfs-cluster/issues/308) | [ipfs/ipfs-cluster#460](https://github.com/ipfs/ipfs-cluster/issues/460)
  * Add `size-only=true` to `repo/stat` calls | [ipfs/ipfs-cluster#507](https://github.com/ipfs/ipfs-cluster/issues/507)
  * Enable DHT-based peer discovery and routing for cluster peers | [ipfs/ipfs-cluster#489](https://github.com/ipfs/ipfs-cluster/issues/489) | [ipfs/ipfs-cluster#508](https://github.com/ipfs/ipfs-cluster/issues/508)
  * Gx-go upgrade | [ipfs/ipfs-cluster#517](https://github.com/ipfs/ipfs-cluster/issues/517)

##### Bugfixes

  * Fix type for constants | [ipfs/ipfs-cluster#455](https://github.com/ipfs/ipfs-cluster/issues/455)
  * Gofmt fix | [ipfs/ipfs-cluster#464](https://github.com/ipfs/ipfs-cluster/issues/464)
  * Fix tests for forked repositories | [ipfs/ipfs-cluster#465](https://github.com/ipfs/ipfs-cluster/issues/465) | [ipfs/ipfs-cluster#472](https://github.com/ipfs/ipfs-cluster/issues/472)
  * Fix resolve panic on `rest/client` | [ipfs/ipfs-cluster#498](https://github.com/ipfs/ipfs-cluster/issues/498)
  * Fix remote pins stuck in error state | [ipfs/ipfs-cluster#500](https://github.com/ipfs/ipfs-cluster/issues/500) | [ipfs/ipfs-cluster#460](https://github.com/ipfs/ipfs-cluster/issues/460)
  * Fix running some tests with `-race` | [ipfs/ipfs-cluster#340](https://github.com/ipfs/ipfs-cluster/issues/340) | [ipfs/ipfs-cluster#458](https://github.com/ipfs/ipfs-cluster/issues/458)
  * Fix ipfs proxy `/add` endpoint | [ipfs/ipfs-cluster#495](https://github.com/ipfs/ipfs-cluster/issues/495) | [ipfs/ipfs-cluster#81](https://github.com/ipfs/ipfs-cluster/issues/81) | [ipfs/ipfs-cluster#505](https://github.com/ipfs/ipfs-cluster/issues/505)
  * Fix ipfs proxy not hijacking `repo/stat` | [ipfs/ipfs-cluster#466](https://github.com/ipfs/ipfs-cluster/issues/466) | [ipfs/ipfs-cluster#514](https://github.com/ipfs/ipfs-cluster/issues/514)
  * Fix some godoc comments | [ipfs/ipfs-cluster#519](https://github.com/ipfs/ipfs-cluster/issues/519)

#### Upgrading notices

##### Configuration files

**IMPORTANT**: `0s` is the new default for the `read_timeout` and `write_timeout` values in the `restapi` configuration section, as well as `proxy_read_timeout` and `proxy_write_timeout` options in the `ipfshttp` section. Adding files to cluster (via the REST api or the proxy) is likely to timeout otherwise.

The `peerstore` file (in the configuration folder), no longer requires listing the multiaddresses for all cluster peers when initializing the cluster with a fixed peerset. It only requires the multiaddresses for one other cluster peer. The rest will be inferred using the DHT. The peerstore file is updated only on clean shutdown, and will store all known multiaddresses, even if not pertaining to cluster peers.

The new `stateless` PinTracker implementation uses a new configuration subsection in the `pin_tracker` key. This is only generated with `ipfs-cluster-service init`. When not present, a default configuration will be used (and a warning printed).

The `state_sync_interval` default has been increased to 10 minutes, as frequent syncing is not needed with the improvements in the PinTracker. Users are welcome to update this setting.


##### REST API

The `/add` endpoint has been added. The `replication_factor_min` and `replication_factor_max` options (in `POST allocations/<cid>`) have been deprecated and subsititued for `replication-min` and `replication-max`, although backwards comaptibility is kept.

Keep Alive has been disabled for the HTTP servers, as a bug in Go's HTTP client implementation may result adding corrupted content (and getting corrupted DAGs). However, while the libp2p API endpoint also suffers this, it will only close libp2p streams. Thus the performance impact on the libp2p-http endpoint should be minimal.

##### Go APIs

The `Config.PeerAddr` key in the `rest/client` module is deprecated. `APIAddr` should be used for both HTTP and LibP2P API endpoints. The type of address is automatically detected.

The IPFSConnector `Pin` call now receives an integer instead of a `Recursive` flag. It indicates the maximum depth to which something should be pinned. The only supported value is `-1` (meaning recursive). `BlockGet` and `BlockPut` calls have been added to the IPFSConnector component.

##### Other

As noted above, upgrade to `state` format version 5 is needed before starting the cluster service.

---

### v0.4.0 - 2018-05-30

#### Summary

The IPFS Cluster version 0.4.0 includes breaking changes and a considerable number of new features causing them. The documentation (particularly that affecting the configuration and startup of peers) has been updated accordingly in https://cluster.ipfs.io . Be sure to also read it if you are upgrading.

There are four main developments in this release:

* Refactorings around the `consensus` component, removing dependencies to the main component and allowing separate initialization: this has prompted to re-approach how we handle the peerset, the peer addresses and the peer's startup when using bootstrap. We have gained finer control of Raft, which has allowed us to provide a clearer configuration and a better start up procedure, specially when bootstrapping. The configuration file no longer mutates while cluster is running.
* Improvements to the `pintracker`: our pin tracker is now able to cancel ongoing pins when receiving an unpin request for the same CID, and vice-versa. It will also optimize multiple pin requests (by only queuing and triggering them once) and can now report
whether an item is pinning (a request to ipfs is ongoing) vs. pin-queued (waiting for a worker to perform the request to ipfs).
* Broadcasting of monitoring metrics using PubSub: we have added a new `monitor` implementation that uses PubSub (rather than RPC broadcasting). With the upcoming improvements to PubSub this means that we can do efficient broadcasting of metrics while at the same time not requiring peers to have RPC permissions, which is preparing the ground for collaborative clusters.
* We have launched the IPFS Cluster website: https://cluster.ipfs.io . We moved most of the documentation over there, expanded it and updated it.

#### List of changes 

##### Features

  * Consensus refactorings | [ipfs/ipfs-cluster#398](https://github.com/ipfs/ipfs-cluster/issues/398) | [ipfs/ipfs-cluster#371](https://github.com/ipfs/ipfs-cluster/issues/371)
  * Pintracker revamp | [ipfs/ipfs-cluster#308](https://github.com/ipfs/ipfs-cluster/issues/308) | [ipfs/ipfs-cluster#383](https://github.com/ipfs/ipfs-cluster/issues/383) | [ipfs/ipfs-cluster#408](https://github.com/ipfs/ipfs-cluster/issues/408) | [ipfs/ipfs-cluster#415](https://github.com/ipfs/ipfs-cluster/issues/415) | [ipfs/ipfs-cluster#421](https://github.com/ipfs/ipfs-cluster/issues/421) | [ipfs/ipfs-cluster#427](https://github.com/ipfs/ipfs-cluster/issues/427) | [ipfs/ipfs-cluster#432](https://github.com/ipfs/ipfs-cluster/issues/432)
  * Pubsub monitoring | [ipfs/ipfs-cluster#400](https://github.com/ipfs/ipfs-cluster/issues/400)
  * Force killing cluster with double CTRL-C | [ipfs/ipfs-cluster#258](https://github.com/ipfs/ipfs-cluster/issues/258) | [ipfs/ipfs-cluster#358](https://github.com/ipfs/ipfs-cluster/issues/358)
  * 3x faster testsuite | [ipfs/ipfs-cluster#339](https://github.com/ipfs/ipfs-cluster/issues/339) | [ipfs/ipfs-cluster#350](https://github.com/ipfs/ipfs-cluster/issues/350)
  * Introduce `disable_repinning` option | [ipfs/ipfs-cluster#369](https://github.com/ipfs/ipfs-cluster/issues/369) | [ipfs/ipfs-cluster#387](https://github.com/ipfs/ipfs-cluster/issues/387)
  * Documentation moved to website and fixes | [ipfs/ipfs-cluster#390](https://github.com/ipfs/ipfs-cluster/issues/390) | [ipfs/ipfs-cluster#391](https://github.com/ipfs/ipfs-cluster/issues/391) | [ipfs/ipfs-cluster#393](https://github.com/ipfs/ipfs-cluster/issues/393) | [ipfs/ipfs-cluster#347](https://github.com/ipfs/ipfs-cluster/issues/347)
  * Run Docker container with `daemon --upgrade` by default | [ipfs/ipfs-cluster#394](https://github.com/ipfs/ipfs-cluster/issues/394)
  * Remove the `ipfs-cluster-ctl peers add` command (bootstrap should be used to add peers) | [ipfs/ipfs-cluster#397](https://github.com/ipfs/ipfs-cluster/issues/397)
  * Add tests using HTTPs endpoints | [ipfs/ipfs-cluster#191](https://github.com/ipfs/ipfs-cluster/issues/191) | [ipfs/ipfs-cluster#403](https://github.com/ipfs/ipfs-cluster/issues/403)
  * Set `refs` as default `pinning_method` and `10` as default `concurrent_pins` | [ipfs/ipfs-cluster#420](https://github.com/ipfs/ipfs-cluster/issues/420)
  * Use latest `gx` and `gx-go`. Be more verbose when installing | [ipfs/ipfs-cluster#418](https://github.com/ipfs/ipfs-cluster/issues/418)
  * Makefile: Properly retrigger builds on source change | [ipfs/ipfs-cluster#426](https://github.com/ipfs/ipfs-cluster/issues/426)
  * Improvements to StateSync() | [ipfs/ipfs-cluster#429](https://github.com/ipfs/ipfs-cluster/issues/429)
  * Rename `ipfs-cluster-data` folder to `raft` | [ipfs/ipfs-cluster#430](https://github.com/ipfs/ipfs-cluster/issues/430)
  * Officially support go 1.10 | [ipfs/ipfs-cluster#439](https://github.com/ipfs/ipfs-cluster/issues/439)
  * Update to libp2p 5.0.17 | [ipfs/ipfs-cluster#440](https://github.com/ipfs/ipfs-cluster/issues/440)

##### Bugsfixes:

  * Don't keep peers /ip*/ addresses if we know DNS addresses for them | [ipfs/ipfs-cluster#381](https://github.com/ipfs/ipfs-cluster/issues/381)
  * Running cluster with wrong configuration path gives misleading error | [ipfs/ipfs-cluster#343](https://github.com/ipfs/ipfs-cluster/issues/343) | [ipfs/ipfs-cluster#370](https://github.com/ipfs/ipfs-cluster/issues/370) | [ipfs/ipfs-cluster#373](https://github.com/ipfs/ipfs-cluster/issues/373)
  * Do not fail when running with `daemon --upgrade` and no state is present | [ipfs/ipfs-cluster#395](https://github.com/ipfs/ipfs-cluster/issues/395)
  * IPFS Proxy: handle arguments passed as part of the url | [ipfs/ipfs-cluster#380](https://github.com/ipfs/ipfs-cluster/issues/380) | [ipfs/ipfs-cluster#392](https://github.com/ipfs/ipfs-cluster/issues/392)
  * WaitForUpdates() may return before state is fully synced | [ipfs/ipfs-cluster#378](https://github.com/ipfs/ipfs-cluster/issues/378)
  * Configuration mutates no more and shadowing is no longer necessary | [ipfs/ipfs-cluster#235](https://github.com/ipfs/ipfs-cluster/issues/235)
  * Govet fixes | [ipfs/ipfs-cluster#417](https://github.com/ipfs/ipfs-cluster/issues/417)
  * Fix release changelog when having RC tags
  * Fix lock file not being removed on cluster force-kill | [ipfs/ipfs-cluster#423](https://github.com/ipfs/ipfs-cluster/issues/423) | [ipfs/ipfs-cluster#437](https://github.com/ipfs/ipfs-cluster/issues/437)
  * Fix indirect pins not being correctly parsed | [ipfs/ipfs-cluster#428](https://github.com/ipfs/ipfs-cluster/issues/428) | [ipfs/ipfs-cluster#436](https://github.com/ipfs/ipfs-cluster/issues/436)
  * Enable NAT support in libp2p host | [ipfs/ipfs-cluster#346](https://github.com/ipfs/ipfs-cluster/issues/346) | [ipfs/ipfs-cluster#441](https://github.com/ipfs/ipfs-cluster/issues/441)
  * Fix pubsub monitor not working on ARM | [ipfs/ipfs-cluster#433](https://github.com/ipfs/ipfs-cluster/issues/433) | [ipfs/ipfs-cluster#443](https://github.com/ipfs/ipfs-cluster/issues/443)

#### Upgrading notices

##### Configuration file

This release introduces **breaking changes to the configuration file**. An error will be displayed if `ipfs-cluster-service` is started with an old configuration file. We recommend re-initing the configuration file altogether.

* The `peers` and `bootstrap` keys have been removed from the main section of the configuration
* You might need to provide Peer multiaddresses in a text file named `peerstore`, in your `~/.ipfs-cluster` folder (one per line). This allows your peers how to contact other peers.
* A `disable_repinning` option has been added to the main configuration section. Defaults to `false`.
* A `init_peerset` has been added to the `raft` configuration section. It should be used to define the starting set of peers when a cluster starts for the first time and is not bootstrapping to an existing running peer (otherwise it is ignored). The value is an array of peer IDs.
* A `backups_rotate` option has been added to the `raft` section and specifies how many copies of the Raft state to keep as backups when the state is cleaned up.
* An `ipfs_request_timeout` option has been introduced to the `ipfshttp` configuration section, and controls the timeout of general requests to the ipfs daemon. Defaults to 5 minutes.
* A `pin_timeout` option has been introduced to the `ipfshttp` section, it controls the timeout for Pin requests to ipfs. Defaults to 24 hours.
* An `unpin_timeout` option has been introduced to the `ipfshttp` section. it controls the timeout for Unpin requests to ipfs. Defaults to 3h.
* Both `pinning_timeout` and `unpinning_timeout` options have been removed from the `maptracker` section.
* A `monitor/pubsubmon` section configures the new PubSub monitoring component. The section is identical to the existing `monbasic`, its only option being `check_interval` (defaults to 15 seconds).

The `ipfs-cluster-data` folder has been renamed to `raft`. Upon `ipfs-cluster-service daemon` start, the renaming will happen automatically if it exists. Otherwise it will be created with the new name.

##### REST API

There are no changes to REST APIs in this release.

##### Go APIs

Several component APIs have changed: `Consensus`, `PeerMonitor` and `IPFSConnector` have added new methods or changed methods signatures.

##### Other

Calling `ipfs-cluster-service` without subcommands no longer runs the peer. It is necessary to call `ipfs-cluster-service daemon`. Several daemon-specific flags have been made subcommand flags: `--bootstrap` and `--alloc`.

The `--bootstrap` flag can now take a list of comma-separated multiaddresses. Using `--bootstrap` will automatically run `state clean`.

The `ipfs-cluster-ctl` no longer has a `peers add` subcommand. Peers should not be added this way, but rather bootstrapped to an existing running peer.

---

### v0.3.5 - 2018-03-29

This release comes full with new features. The biggest ones are the support for parallel pinning (using `refs -r` rather than `pin add` to pin things in IPFS), and the exposing of the http endpoints through libp2p. This allows users to securely interact with the HTTP API without having to setup SSL certificates.

* Features
  * `--no-status` for `ipfs-cluster-ctl pin add/rm` allows to speed up adding and removing by not fetching the status one second afterwards. Useful for ingesting pinsets to cluster | [ipfs/ipfs-cluster#286](https://github.com/ipfs/ipfs-cluster/issues/286) | [ipfs/ipfs-cluster#329](https://github.com/ipfs/ipfs-cluster/issues/329)
  * `--wait` flag for `ipfs-cluster-ctl pin add/rm` allows to wait until a CID is fully pinned or unpinned [ipfs/ipfs-cluster#338](https://github.com/ipfs/ipfs-cluster/issues/338) | [ipfs/ipfs-cluster#348](https://github.com/ipfs/ipfs-cluster/issues/348) | [ipfs/ipfs-cluster#363](https://github.com/ipfs/ipfs-cluster/issues/363)
  * Support `refs` pinning method. Parallel pinning | [ipfs/ipfs-cluster#326](https://github.com/ipfs/ipfs-cluster/issues/326) | [ipfs/ipfs-cluster#331](https://github.com/ipfs/ipfs-cluster/issues/331)
  * Double default timeouts for `ipfs-cluster-ctl` | [ipfs/ipfs-cluster#323](https://github.com/ipfs/ipfs-cluster/issues/323) | [ipfs/ipfs-cluster#334](https://github.com/ipfs/ipfs-cluster/issues/334)
  * Better error messages during startup | [ipfs/ipfs-cluster#167](https://github.com/ipfs/ipfs-cluster/issues/167) | [ipfs/ipfs-cluster#344](https://github.com/ipfs/ipfs-cluster/issues/344) | [ipfs/ipfs-cluster#353](https://github.com/ipfs/ipfs-cluster/issues/353)
  * REST API client now provides an `IPFS()` method which returns a `go-ipfs-api` shell instance pointing to the proxy endpoint | [ipfs/ipfs-cluster#269](https://github.com/ipfs/ipfs-cluster/issues/269) | [ipfs/ipfs-cluster#356](https://github.com/ipfs/ipfs-cluster/issues/356)
  * REST http-api-over-libp2p. Server, client, `ipfs-cluster-ctl` support added | [ipfs/ipfs-cluster#305](https://github.com/ipfs/ipfs-cluster/issues/305) | [ipfs/ipfs-cluster#349](https://github.com/ipfs/ipfs-cluster/issues/349)
  * Added support for priority pins and non-recursive pins (sharding-related) | [ipfs/ipfs-cluster#341](https://github.com/ipfs/ipfs-cluster/issues/341) | [ipfs/ipfs-cluster#342](https://github.com/ipfs/ipfs-cluster/issues/342)
  * Documentation fixes | [ipfs/ipfs-cluster#328](https://github.com/ipfs/ipfs-cluster/issues/328) | [ipfs/ipfs-cluster#357](https://github.com/ipfs/ipfs-cluster/issues/357)

* Bugfixes
  * Print lock path in logs | [ipfs/ipfs-cluster#332](https://github.com/ipfs/ipfs-cluster/issues/332) | [ipfs/ipfs-cluster#333](https://github.com/ipfs/ipfs-cluster/issues/333)

There are no breaking API changes and all configurations should be backwards compatible. The `api/rest/client` provides a new `IPFS()` method.

We recommend updating the `service.json` configurations to include all the new configuration options:

* The `pin_method` option has been added to the `ipfshttp` section. It supports `refs` and `pin` (default) values. Use `refs` for parallel pinning, but only if you don't run automatic GC on your ipfs nodes.
* The `concurrent_pins` option has been added to the `maptracker` section. Only useful with `refs` option in `pin_method`.
* The `listen_multiaddress` option in the `restapi` section should be renamed to `http_listen_multiaddress`.

This release will require a **state upgrade**. Run `ipfs-cluster-service state upgrade` in all your peers, or start cluster with `ipfs-cluster-service daemon --upgrade`.

---

### v0.3.4 - 2018-02-20

This release fixes the pre-built binaries.

* Bugfixes
  * Pre-built binaries panic on start | [ipfs/ipfs-cluster#320](https://github.com/ipfs/ipfs-cluster/issues/320)

---

### v0.3.3 - 2018-02-12

This release includes additional `ipfs-cluster-service state` subcommands and the connectivity graph feature.

* Features
  * `ipfs-cluster-service daemon --upgrade` allows to automatically run migrations before starting | [ipfs/ipfs-cluster#300](https://github.com/ipfs/ipfs-cluster/issues/300) | [ipfs/ipfs-cluster#307](https://github.com/ipfs/ipfs-cluster/issues/307)
  * `ipfs-cluster-service state version` reports the shared state format version | [ipfs/ipfs-cluster#298](https://github.com/ipfs/ipfs-cluster/issues/298) | [ipfs/ipfs-cluster#307](https://github.com/ipfs/ipfs-cluster/issues/307)
  * `ipfs-cluster-service health graph` generates a .dot graph file of cluster connectivity | [ipfs/ipfs-cluster#17](https://github.com/ipfs/ipfs-cluster/issues/17) | [ipfs/ipfs-cluster#291](https://github.com/ipfs/ipfs-cluster/issues/291) | [ipfs/ipfs-cluster#311](https://github.com/ipfs/ipfs-cluster/issues/311)

* Bugfixes
  * Do not upgrade state if already up to date | [ipfs/ipfs-cluster#296](https://github.com/ipfs/ipfs-cluster/issues/296) | [ipfs/ipfs-cluster#307](https://github.com/ipfs/ipfs-cluster/issues/307)
  * Fix `ipfs-cluster-service daemon` failing with `unknown allocation strategy` error | [ipfs/ipfs-cluster#314](https://github.com/ipfs/ipfs-cluster/issues/314) | [ipfs/ipfs-cluster#315](https://github.com/ipfs/ipfs-cluster/issues/315)

APIs have not changed in this release. The `/health/graph` endpoint has been added.

---

### v0.3.2 - 2018-01-25

This release includes a number of bufixes regarding the upgrade and import of state, along with two important features:
  * Commands to export and import the internal cluster state: these allow to perform easy and human-readable dumps of the shared cluster state while offline, and eventually restore it in a different peer or cluster.
  * The introduction of `replication_factor_min` and `replication_factor_max` parameters for every Pin (along with the deprecation of `replication_factor`). The defaults are specified in the configuration. For more information on the usage and behavour of these new options, check the IPFS cluster guide.

* Features
  * New `ipfs-cluster-service state export/import/cleanup` commands | [ipfs/ipfs-cluster#240](https://github.com/ipfs/ipfs-cluster/issues/240) | [ipfs/ipfs-cluster#290](https://github.com/ipfs/ipfs-cluster/issues/290)
  * New min/max replication factor control | [ipfs/ipfs-cluster#277](https://github.com/ipfs/ipfs-cluster/issues/277) | [ipfs/ipfs-cluster#292](https://github.com/ipfs/ipfs-cluster/issues/292)
  * Improved migration code | [ipfs/ipfs-cluster#283](https://github.com/ipfs/ipfs-cluster/issues/283)
  * `ipfs-cluster-service version` output simplified (see below) | [ipfs/ipfs-cluster#274](https://github.com/ipfs/ipfs-cluster/issues/274)
  * Testing improvements:
    * Added tests for Dockerfiles | [ipfs/ipfs-cluster#200](https://github.com/ipfs/ipfs-cluster/issues/200) | [ipfs/ipfs-cluster#282](https://github.com/ipfs/ipfs-cluster/issues/282)
    * Enabled Jenkins testing and made it work | [ipfs/ipfs-cluster#256](https://github.com/ipfs/ipfs-cluster/issues/256) | [ipfs/ipfs-cluster#294](https://github.com/ipfs/ipfs-cluster/issues/294)
  * Documentation improvements:
    * Guide contains more details on state upgrade procedures | [ipfs/ipfs-cluster#270](https://github.com/ipfs/ipfs-cluster/issues/270)
    * ipfs-cluster-ctl exit status are documented on the README | [ipfs/ipfs-cluster#178](https://github.com/ipfs/ipfs-cluster/issues/178)

* Bugfixes
  * Force cleanup after sharness tests | [ipfs/ipfs-cluster#181](https://github.com/ipfs/ipfs-cluster/issues/181) | [ipfs/ipfs-cluster#288](https://github.com/ipfs/ipfs-cluster/issues/288)
  * Fix state version validation on start | [ipfs/ipfs-cluster#293](https://github.com/ipfs/ipfs-cluster/issues/293)
  * Wait until last index is applied before attempting snapshot on shutdown | [ipfs/ipfs-cluster#275](https://github.com/ipfs/ipfs-cluster/issues/275)
  * Snaps from master not pushed due to bad credentials
  * Fix overpinning or underpinning of CIDs after re-join | [ipfs/ipfs-cluster#222](https://github.com/ipfs/ipfs-cluster/issues/222)
  * Fix unmarshaling state on top of an existing one | [ipfs/ipfs-cluster#297](https://github.com/ipfs/ipfs-cluster/issues/297)
  * Fix catching up on imported state | [ipfs/ipfs-cluster#297](https://github.com/ipfs/ipfs-cluster/issues/297)

These release is compatible with previous versions of ipfs-cluster on the API level, with the exception of the `ipfs-cluster-service version` command, which returns `x.x.x-shortcommit` rather than `ipfs-cluster-service version 0.3.1`. The former output is still available as `ipfs-cluster-service --version`.

The `replication_factor` option is deprecated, but still supported and will serve as a shortcut to set both `replication_factor_min` and `replication_factor_max` to the same value. This affects the configuration file, the REST API and the `ipfs-cluster-ctl pin add` command.

---

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

---

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
