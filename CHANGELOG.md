# IPFS Cluster Changelog

### v0.11.0 - 2019-09-13

#### Summary

IPFS Cluster v0.11.0 is the biggest release in the project's history. Its main
feature is the introduction of the new CRDT "consensus" component. Leveraging
Pubsub, Bitswap and the DHT and using CRDTs, cluster peers can track the
global pinset without needing to be online or worrying about the rest of the
peers as it happens with the original Raft approach.

The CRDT component brings a lots of features around it, like RPC
authorization, which effectively lets cluster peers run in clusters where only
a trusted subset of nodes can access peer endpoints and made modifications to
the pinsets.

We have additionally taken lots of steps to improve configuration management
of peers, separating the peer identity from the rest of the configuration and
allowing to use remote configurations fetched from an HTTP url (which may well
be the local IPFS gateway). This allows cluster administrators to provide
the configurations needed for any peers to join a cluster as followers.

The CRDT arrival incorporates a large number of improvements in peerset
management, bootstrapping, connection management and auto-recovery of peers
after network disconnections. We have improved the peer monitoring system,
added support for efficient Pin-Update-based pinning, reworked timeout control
for pinning and fixed a number of annoying bugs.

This release is mostly backwards compatible with the previous one and
clusters should keep working with the same configurations, but users should
have a look to the sections below and read the updated documentation, as a
number of changes have been introduced to support both consensus components.

Consensus selection happens during initialization of the configuration (see
configuration changes below). Migration of the pinset is necessary by doing
`state export` (with Raft configured), followed by `state import` (with CRDT
configured). Note that all peers should be configured with the same consensus
type.


#### List of changes

##### Features


* crdt: introduce crdt-based consensus component | [ipfs/ipfs-cluster#685](https://github.com/ipfs/ipfs-cluster/issues/685) | [ipfs/ipfs-cluster#804](https://github.com/ipfs/ipfs-cluster/issues/804) | [ipfs/ipfs-cluster#787](https://github.com/ipfs/ipfs-cluster/issues/787) | [ipfs/ipfs-cluster#798](https://github.com/ipfs/ipfs-cluster/issues/798) | [ipfs/ipfs-cluster#805](https://github.com/ipfs/ipfs-cluster/issues/805) | [ipfs/ipfs-cluster#811](https://github.com/ipfs/ipfs-cluster/issues/811) | [ipfs/ipfs-cluster#816](https://github.com/ipfs/ipfs-cluster/issues/816) | [ipfs/ipfs-cluster#820](https://github.com/ipfs/ipfs-cluster/issues/820) | [ipfs/ipfs-cluster#856](https://github.com/ipfs/ipfs-cluster/issues/856) | [ipfs/ipfs-cluster#857](https://github.com/ipfs/ipfs-cluster/issues/857) | [ipfs/ipfs-cluster#834](https://github.com/ipfs/ipfs-cluster/issues/834) | [ipfs/ipfs-cluster#856](https://github.com/ipfs/ipfs-cluster/issues/856) | [ipfs/ipfs-cluster#867](https://github.com/ipfs/ipfs-cluster/issues/867) | [ipfs/ipfs-cluster#874](https://github.com/ipfs/ipfs-cluster/issues/874) | [ipfs/ipfs-cluster#885](https://github.com/ipfs/ipfs-cluster/issues/885) | [ipfs/ipfs-cluster#899](https://github.com/ipfs/ipfs-cluster/issues/899) | [ipfs/ipfs-cluster#906](https://github.com/ipfs/ipfs-cluster/issues/906) | [ipfs/ipfs-cluster#918](https://github.com/ipfs/ipfs-cluster/issues/918)
* configs: separate identity and configuration | [ipfs/ipfs-cluster#760](https://github.com/ipfs/ipfs-cluster/issues/760) | [ipfs/ipfs-cluster#766](https://github.com/ipfs/ipfs-cluster/issues/766) | [ipfs/ipfs-cluster#780](https://github.com/ipfs/ipfs-cluster/issues/780)
* configs: support running with a remote `service.json` (http) | [ipfs/ipfs-cluster#868](https://github.com/ipfs/ipfs-cluster/issues/868)
* configs: support a `follower_mode` option | [ipfs/ipfs-cluster#803](https://github.com/ipfs/ipfs-cluster/issues/803) | [ipfs/ipfs-cluster#864](https://github.com/ipfs/ipfs-cluster/issues/864)
* service/configs: do not load API components if no config present | [ipfs/ipfs-cluster#452](https://github.com/ipfs/ipfs-cluster/issues/452) | [ipfs/ipfs-cluster#836](https://github.com/ipfs/ipfs-cluster/issues/836)
* service: add `ipfs-cluster-service init --peers` flag to initialize with given peers | [ipfs/ipfs-cluster#835](https://github.com/ipfs/ipfs-cluster/issues/835) | [ipfs/ipfs-cluster#839](https://github.com/ipfs/ipfs-cluster/issues/839) | [ipfs/ipfs-cluster#870](https://github.com/ipfs/ipfs-cluster/issues/870)
* cluster: RPC auth: block rpc endpoints for non trusted peers | [ipfs/ipfs-cluster#775](https://github.com/ipfs/ipfs-cluster/issues/775) | [ipfs/ipfs-cluster#710](https://github.com/ipfs/ipfs-cluster/issues/710) | [ipfs/ipfs-cluster#666](https://github.com/ipfs/ipfs-cluster/issues/666) | [ipfs/ipfs-cluster#773](https://github.com/ipfs/ipfs-cluster/issues/773) | [ipfs/ipfs-cluster#905](https://github.com/ipfs/ipfs-cluster/issues/905)
* cluster: introduce connection manager | [ipfs/ipfs-cluster#791](https://github.com/ipfs/ipfs-cluster/issues/791)
* cluster: support new `PinUpdate` option for new pins | [ipfs/ipfs-cluster#869](https://github.com/ipfs/ipfs-cluster/issues/869) | [ipfs/ipfs-cluster#732](https://github.com/ipfs/ipfs-cluster/issues/732)
* cluster: trigger `Recover` automatically on a configurable interval | [ipfs/ipfs-cluster#831](https://github.com/ipfs/ipfs-cluster/issues/831) | [ipfs/ipfs-cluster#887](https://github.com/ipfs/ipfs-cluster/issues/887)
* cluster: enable mDNS discovery for peers | [ipfs/ipfs-cluster#882](https://github.com/ipfs/ipfs-cluster/issues/882) | [ipfs/ipfs-cluster#900](https://github.com/ipfs/ipfs-cluster/issues/900)
* IPFS Proxy: Support `pin/update` | [ipfs/ipfs-cluster#732](https://github.com/ipfs/ipfs-cluster/issues/732) | [ipfs/ipfs-cluster#768](https://github.com/ipfs/ipfs-cluster/issues/768) | [ipfs/ipfs-cluster#887](https://github.com/ipfs/ipfs-cluster/issues/887)
* monitor: Accrual failure detection. Leaderless re-pinning | [ipfs/ipfs-cluster#413](https://github.com/ipfs/ipfs-cluster/issues/413) | [ipfs/ipfs-cluster#713](https://github.com/ipfs/ipfs-cluster/issues/713) | [ipfs/ipfs-cluster#714](https://github.com/ipfs/ipfs-cluster/issues/714) | [ipfs/ipfs-cluster#812](https://github.com/ipfs/ipfs-cluster/issues/812) | [ipfs/ipfs-cluster#813](https://github.com/ipfs/ipfs-cluster/issues/813) | [ipfs/ipfs-cluster#814](https://github.com/ipfs/ipfs-cluster/issues/814) | [ipfs/ipfs-cluster#815](https://github.com/ipfs/ipfs-cluster/issues/815)
* datastore: Expose badger configuration | [ipfs/ipfs-cluster#771](https://github.com/ipfs/ipfs-cluster/issues/771) | [ipfs/ipfs-cluster#776](https://github.com/ipfs/ipfs-cluster/issues/776)
* IPFSConnector: pin timeout start counting from last received block | [ipfs/ipfs-cluster#497](https://github.com/ipfs/ipfs-cluster/issues/497) | [ipfs/ipfs-cluster#738](https://github.com/ipfs/ipfs-cluster/issues/738)
* IPFSConnector: remove pin method options | [ipfs/ipfs-cluster#875](https://github.com/ipfs/ipfs-cluster/issues/875)
* IPFSConnector: `unpin_disable` removes the ability to unpin anything from ipfs (experimental) | [ipfs/ipfs-cluster#793](https://github.com/ipfs/ipfs-cluster/issues/793) | [ipfs/ipfs-cluster#832](https://github.com/ipfs/ipfs-cluster/issues/832)
* REST API Client: Load-balancing Go client | [ipfs/ipfs-cluster#448](https://github.com/ipfs/ipfs-cluster/issues/448) | [ipfs/ipfs-cluster#737](https://github.com/ipfs/ipfs-cluster/issues/737)
* REST API: Return allocation objects on pin/unpin | [ipfs/ipfs-cluster#843](https://github.com/ipfs/ipfs-cluster/issues/843)
* REST API: Support request logging | [ipfs/ipfs-cluster#574](https://github.com/ipfs/ipfs-cluster/issues/574) | [ipfs/ipfs-cluster#894](https://github.com/ipfs/ipfs-cluster/issues/894)
* Adder: improve error handling. Keep adding while at least one allocation works | [ipfs/ipfs-cluster#852](https://github.com/ipfs/ipfs-cluster/issues/852) | [ipfs/ipfs-cluster#871](https://github.com/ipfs/ipfs-cluster/issues/871)
* Adder: support user-given allocations for the `Add` operation | [ipfs/ipfs-cluster#761](https://github.com/ipfs/ipfs-cluster/issues/761) | [ipfs/ipfs-cluster#890](https://github.com/ipfs/ipfs-cluster/issues/890)
* ctl: support adding pin metadata | [ipfs/ipfs-cluster#670](https://github.com/ipfs/ipfs-cluster/issues/670) | [ipfs/ipfs-cluster#891](https://github.com/ipfs/ipfs-cluster/issues/891)


##### Bug fixes

* REST API: Fix `/allocations` when filter unset | [ipfs/ipfs-cluster#762](https://github.com/ipfs/ipfs-cluster/issues/762)
* REST API: Fix DELETE returning 500 when pin does not exist | [ipfs/ipfs-cluster#742](https://github.com/ipfs/ipfs-cluster/issues/742) | [ipfs/ipfs-cluster#854](https://github.com/ipfs/ipfs-cluster/issues/854)
* REST API: Return JSON body on 404s | [ipfs/ipfs-cluster#657](https://github.com/ipfs/ipfs-cluster/issues/657) | [ipfs/ipfs-cluster#879](https://github.com/ipfs/ipfs-cluster/issues/879)
* service: connectivity fixes | [ipfs/ipfs-cluster#787](https://github.com/ipfs/ipfs-cluster/issues/787) | [ipfs/ipfs-cluster#792](https://github.com/ipfs/ipfs-cluster/issues/792)
* service: fix using `/dnsaddr` peers | [ipfs/ipfs-cluster#818](https://github.com/ipfs/ipfs-cluster/issues/818)
* service: reading empty lines on peerstore panics | [ipfs/ipfs-cluster#886](https://github.com/ipfs/ipfs-cluster/issues/886)
* service/ctl: fix parsing string lists | [ipfs/ipfs-cluster#876](https://github.com/ipfs/ipfs-cluster/issues/876) | [ipfs/ipfs-cluster#841](https://github.com/ipfs/ipfs-cluster/issues/841)
* IPFSConnector: `pin/ls` does handle base32 and base58 cids properly | [ipfs/ipfs-cluster#808](https://github.com/ipfs/ipfs-cluster/issues/808) [ipfs/ipfs-cluster#809](https://github.com/ipfs/ipfs-cluster/issues/809)
* configs: some config keys not matching ENV vars names | [ipfs/ipfs-cluster#837](https://github.com/ipfs/ipfs-cluster/issues/837) | [ipfs/ipfs-cluster#778](https://github.com/ipfs/ipfs-cluster/issues/778)
* raft: delete removed raft peers from peerstore | [ipfs/ipfs-cluster#840](https://github.com/ipfs/ipfs-cluster/issues/840) | [ipfs/ipfs-cluster#846](https://github.com/ipfs/ipfs-cluster/issues/846)
* cluster: peers forgotten after being down | [ipfs/ipfs-cluster#648](https://github.com/ipfs/ipfs-cluster/issues/648) | [ipfs/ipfs-cluster#860](https://github.com/ipfs/ipfs-cluster/issues/860)
* cluster: State sync should not keep tracking when queue is full | [ipfs/ipfs-cluster#377](https://github.com/ipfs/ipfs-cluster/issues/377) | [ipfs/ipfs-cluster#901](https://github.com/ipfs/ipfs-cluster/issues/901)
* cluster: avoid random order on peer lists and listen multiaddresses | [ipfs/ipfs-cluster#327](https://github.com/ipfs/ipfs-cluster/issues/327) | [ipfs/ipfs-cluster#878](https://github.com/ipfs/ipfs-cluster/issues/878)
* cluster: fix recover and allocation re-assignment to existing pins | [ipfs/ipfs-cluster#912](https://github.com/ipfs/ipfs-cluster/issues/912) | [ipfs/ipfs-cluster#888](https://github.com/ipfs/ipfs-cluster/issues/888)

##### Other changes

* cluster: Dependency updates | [ipfs/ipfs-cluster#769](https://github.com/ipfs/ipfs-cluster/issues/769) | [ipfs/ipfs-cluster#789](https://github.com/ipfs/ipfs-cluster/issues/789) | [ipfs/ipfs-cluster#795](https://github.com/ipfs/ipfs-cluster/issues/795) | [ipfs/ipfs-cluster#822](https://github.com/ipfs/ipfs-cluster/issues/822) | [ipfs/ipfs-cluster#823](https://github.com/ipfs/ipfs-cluster/issues/823) | [ipfs/ipfs-cluster#828](https://github.com/ipfs/ipfs-cluster/issues/828) | [ipfs/ipfs-cluster#830](https://github.com/ipfs/ipfs-cluster/issues/830) | [ipfs/ipfs-cluster#853](https://github.com/ipfs/ipfs-cluster/issues/853) | [ipfs/ipfs-cluster#839](https://github.com/ipfs/ipfs-cluster/issues/839)
* cluster: Set `[]peer.ID` as type for user allocations | [ipfs/ipfs-cluster#767](https://github.com/ipfs/ipfs-cluster/issues/767)
* cluster: RPC: Split services among components | [ipfs/ipfs-cluster#773](https://github.com/ipfs/ipfs-cluster/issues/773)
* cluster: Multiple improvements to tests | [ipfs/ipfs-cluster#360](https://github.com/ipfs/ipfs-cluster/issues/360) | [ipfs/ipfs-cluster#502](https://github.com/ipfs/ipfs-cluster/issues/502) | [ipfs/ipfs-cluster#779](https://github.com/ipfs/ipfs-cluster/issues/779) | [ipfs/ipfs-cluster#833](https://github.com/ipfs/ipfs-cluster/issues/833) | [ipfs/ipfs-cluster#863](https://github.com/ipfs/ipfs-cluster/issues/863) | [ipfs/ipfs-cluster#883](https://github.com/ipfs/ipfs-cluster/issues/883) | [ipfs/ipfs-cluster#884](https://github.com/ipfs/ipfs-cluster/issues/884) | [ipfs/ipfs-cluster#797](https://github.com/ipfs/ipfs-cluster/issues/797) | [ipfs/ipfs-cluster#892](https://github.com/ipfs/ipfs-cluster/issues/892)
* cluster: Remove Gx | [ipfs/ipfs-cluster#765](https://github.com/ipfs/ipfs-cluster/issues/765) | [ipfs/ipfs-cluster#781](https://github.com/ipfs/ipfs-cluster/issues/781)
* cluster: Use `/p2p/` instead of `/ipfs/` in multiaddresses | [ipfs/ipfs-cluster#431](https://github.com/ipfs/ipfs-cluster/issues/431) | [ipfs/ipfs-cluster#877](https://github.com/ipfs/ipfs-cluster/issues/877)
* cluster: consolidate parsing of pin options | [ipfs/ipfs-cluster#913](https://github.com/ipfs/ipfs-cluster/issues/913)
* REST API: Replace regexps with `strings.HasPrefix` | [ipfs/ipfs-cluster#806](https://github.com/ipfs/ipfs-cluster/issues/806) | [ipfs/ipfs-cluster#807](https://github.com/ipfs/ipfs-cluster/issues/807)
* docker: use GOPROXY to build containers | [ipfs/ipfs-cluster#872](https://github.com/ipfs/ipfs-cluster/issues/872)
* docker: support `IPFS_CLUSTER_CONSENSUS` flag and other improvements | [ipfs/ipfs-cluster#882](https://github.com/ipfs/ipfs-cluster/issues/882)
* ctl: increase space for peernames | [ipfs/ipfs-cluster#887](https://github.com/ipfs/ipfs-cluster/issues/887)
* ctl: improve replication factor 0 explanation | [ipfs/ipfs-cluster#755](https://github.com/ipfs/ipfs-cluster/issues/755) | [ipfs/ipfs-cluster#909](https://github.com/ipfs/ipfs-cluster/issues/909)

#### Upgrading notices


##### Configuration changes

This release introduces a number of backwards-compatible configuration changes:

* The `service.json` file no longer includes `ID` and `PrivateKey`, which are
  now part of an `identity.json` file. However things should work as before if
  they do. Running `ipfs-cluster-service daemon` on a older configuration will
  automatically write an `indentity.json` file with the old credentials so that
  things do not break when the compatibility hack is removed.

* The `service.json` can use a new single top-level `source` field which can
  be set to an HTTP url pointing to a full `service.json`. When present,
  this will be read and used when starting the daemon. `ipfs-cluster-service
  init http://url` produces this type of "remote configuration" file.

* `cluster` section:
  * A new, hidden `follower_mode` option has been introduced in the main
    `cluster` configuration section. When set, the cluster peer will provide
    clear errors when pinning or unpinning. This is a UI feature. The capacity
    of a cluster peer to pin/unpin depends on whether it is trusted by other
    peers, not on settin this hidden option.
  * A new `pin_recover_interval` option to controls how often pins in error
    states are retried.
  * A new `mdns_interval` controls the time between mDNS broadcasts to
    discover other peers in the network. Setting it to 0 disables mDNS
    altogether (default is 10 seconds).
  * A new `connection_manager` object can be used to limit the number of
    connections kept by the libp2p host:

```js
"connection_manager": {
    "high_water": 400,
    "low_water": 100,
    "grace_period": "2m0s"
},
```


* `consensus` section:
  * Only one configuration object is allowed inside the `consensus` section,
    and it must be either the `crdt` or the `raft` one. The presence of one or
    another is used to autoselect the consensus component to be used when
    running the daemon or performing `ipfs-cluster-service state`
    operations. `ipfs-cluster-service init` receives an optional `--consensus`
    flag to select which one to produce. By default it is the `crdt`.

* `ipfs_connector/ipfshttp` section:
  * The `pin_timeout` in the `ipfshttp` section is now starting from the last
    block received. Thus it allows more flexibility for things which are
    pinning very slowly, but still pinning.
  * The `pin_method` option has been removed, as go-ipfs does not do a
    pin-global-lock anymore. Therefore `pin add` will be called directly, can
    be called multiple times in parallel and should be faster than the
    deprecated `refs -r` way.
  * The `ipfshttp` section has a new (hidden) `unpin_disable` option
    (boolean). The component will refuse to unpin anything from IPFS when
    enabled. It can be used as a failsafe option to make sure cluster peers
    never unpin content.

* `datastore` section:
  * The configuration has a new `datastore/badger` section, which is relevant
    when using the `crdt` consensus component. It allows full control of the
    [Badger configuration](https://godoc.org/github.com/dgraph-io/badger#Options),
    which is particuarly important when running on systems with low memory:
  

```
  "datastore": {
    "badger": {
      "badger_options": {
        "dir": "",
        "value_dir": "",
        "sync_writes": true,
        "table_loading_mode": 2,
        "value_log_loading_mode": 2,
        "num_versions_to_keep": 1,
        "max_table_size": 67108864,
        "level_size_multiplier": 10,
        "max_levels": 7,
        "value_threshold": 32,
        "num_memtables": 5,
        "num_level_zero_tables": 5,
        "num_level_zero_tables_stall": 10,
        "level_one_size": 268435456,
        "value_log_file_size": 1073741823,
        "value_log_max_entries": 1000000,
        "num_compactors": 2,
        "compact_l_0_on_close": true,
        "read_only": false,
        "truncate": false
      }
    }
    }
```

* `pin_tracker/maptracker` section:
  * The `max_pin_queue_size` parameter has been hidden for default
    configurations and the default has been set to 1000000. 

* `api/restapi` section:
  * A new `http_log_file` options allows to redirect the REST API logging to a
    file. Otherwise, it is logged as part of the regular log. Lines follow the
    Apache Common Log Format (CLF).

##### REST API

The `POST /pins/{cid}` and `DELETE /pins/{cid}` now returns a pin object with
`200 Success` rather than an empty `204 Accepted` response.

Using an unexistent route will now correctly return a JSON object along with
the 404 HTTP code, rather than text.

##### Go APIs

There have been some changes to Go APIs. Applications integrating Cluster
directly will be affected by the new signatures of Pin/Unpin:

* The `Pin` and `Unpin` methods now return an object of `api.Pin` type, along with an error.
* The `Pin` method takes a CID and `PinOptions` rather than an `api.Pin` object wrapping
those.
* A new `PinUpdate` method has been introduced.

Additionally:

* The Consensus Component interface has changed to accommodate peer-trust operations.
* The IPFSConnector Component interface `Pin` method has changed to take an `api.Pin` type.


##### Other

* The IPFS Proxy now hijacks the `/api/v0/pin/update` and makes a Cluster PinUpdate.
* `ipfs-cluster-service init` now takes a `--consensus` flag to select between
  `crdt` (default) and `raft`. Depending on the values, the generated
  configuration will have the relevant sections for each.
* The Dockerfiles have been updated to:
  * Support the `IPFS_CLUSTER_CONSENSUS` flag to determine which consensus to
  use for the automatic `init`.
  * No longer use `IPFS_API` environment variable to do a `sed` replacement on
    the config, as `CLUSTER_IPFSHTTP_NODEMULTIADDRESS` is the canonical one to
    use.
  * No longer use `sed` replacement to set the APIs listen IPs to `0.0.0.0`
    automatically, as this can be achieved with environment variables
    (`CLUSTER_RESTAPI_HTTPLISTENMULTIADDRESS` and
    `CLUSTER_IPFSPROXY_LISTENMULTIADDRESS`) and can be dangerous for containers
    running in `net=host` mode.
  * The `docker-compose.yml` has been updated and simplified to launch a CRDT
    3-peer TEST cluster
* Cluster now uses `/p2p/` instead of `/ipfs/` for libp2p multiaddresses by
  default, but both protocol IDs are equivalent and interchangeable.
* Pinning an already existing pin will re-submit it to the consensus layer in
  all cases, meaning that pins in error states will start pinning again
  (before, sometimes this was only possible using recover). Recover stays as a
  broadcast/sync operation to trigger pinning on errored items. As a reminder,
  pin is consensus/async operation.
    
---


### v0.10.1 - 2019-04-10

#### Summary

This release is a maintenance release with a number of bug fixes and a couple of small features.


#### List of changes

##### Features

* Switch to go.mod | [ipfs/ipfs-cluster#706](https://github.com/ipfs/ipfs-cluster/issues/706) | [ipfs/ipfs-cluster#707](https://github.com/ipfs/ipfs-cluster/issues/707) | [ipfs/ipfs-cluster#708](https://github.com/ipfs/ipfs-cluster/issues/708)
* Remove basic monitor | [ipfs/ipfs-cluster#689](https://github.com/ipfs/ipfs-cluster/issues/689) | [ipfs/ipfs-cluster#726](https://github.com/ipfs/ipfs-cluster/issues/726)
* Support `nocopy` when adding URLs | [ipfs/ipfs-cluster#735](https://github.com/ipfs/ipfs-cluster/issues/735)

##### Bug fixes

* Mitigate long header attack | [ipfs/ipfs-cluster#636](https://github.com/ipfs/ipfs-cluster/issues/636) | [ipfs/ipfs-cluster#712](https://github.com/ipfs/ipfs-cluster/issues/712)
* Fix download link in README | [ipfs/ipfs-cluster#723](https://github.com/ipfs/ipfs-cluster/issues/723)
* Fix `peers ls` error when peers down | [ipfs/ipfs-cluster#715](https://github.com/ipfs/ipfs-cluster/issues/715) | [ipfs/ipfs-cluster#719](https://github.com/ipfs/ipfs-cluster/issues/719)
* Nil pointer panic on `ipfs-cluster-ctl add` | [ipfs/ipfs-cluster#727](https://github.com/ipfs/ipfs-cluster/issues/727) | [ipfs/ipfs-cluster#728](https://github.com/ipfs/ipfs-cluster/issues/728)
* Fix `enc=json` output on `ipfs-cluster-ctl add` | [ipfs/ipfs-cluster#729](https://github.com/ipfs/ipfs-cluster/issues/729)
* Add SSL CAs to Docker container | [ipfs/ipfs-cluster#730](https://github.com/ipfs/ipfs-cluster/issues/730) | [ipfs/ipfs-cluster#731](https://github.com/ipfs/ipfs-cluster/issues/731)
* Remove duplicate import | [ipfs/ipfs-cluster#734](https://github.com/ipfs/ipfs-cluster/issues/734)
* Fix version json object | [ipfs/ipfs-cluster#743](https://github.com/ipfs/ipfs-cluster/issues/743) | [ipfs/ipfs-cluster#752](https://github.com/ipfs/ipfs-cluster/issues/752)

#### Upgrading notices



##### Configuration changes

There are no configuration changes on this release.

##### REST API

The `/version` endpoint now returns a version object with *lowercase* `version` key.

##### Go APIs

There are no changes to the Go APIs.

##### Other

Since we have switched to Go modules for dependency management, `gx` is no
longer used and the maintenance of Gx dependencies has been dropped. The
`Makefile` has been updated accordinly, but now a simple `go install
./cmd/...` works.

---

### v0.10.0 - 2019-03-07

#### Summary

As we get ready to introduce a new CRDT-based "consensus" component to replace
Raft, IPFS Cluster 0.10.0 prepares the ground with substancial under-the-hood
changes. many performance improvements and a few very useful features.

First of all, this release **requires** users to run `state upgrade` (or start
their daemons with `ipfs-cluster-service daemon --upgrade`). This is the last
upgrade in this fashion as we turn to go-datastore-based storage. The next
release of IPFS Cluster will not understand or be able to upgrade anything
below 0.10.0.

Secondly, we have made some changes to internal types that should greatly
improve performance a lot, particularly calls involving large collections of
items (`pin ls` or `status`). There are also changes on how the state is
serialized, avoiding unnecessary in-memory copies. We have also upgraded the
dependency stack, incorporating many fixes from libp2p.

Thirdly, our new great features:

* `ipfs-cluster-ctl pin add/rm` now supports IPFS paths (`/ipfs/Qmxx.../...`,
  `/ipns/Qmxx.../...`, `/ipld/Qm.../...`) which are resolved automatically
  before pinning.
* All our configuration values can now be set via environment variables, and
these will be reflected when initializing a new configuration file.
* Pins can now specify a list of "priority allocations". This allows to pin
items to specific Cluster peers, overriding the default allocation policy.
* Finally, the REST API supports adding custom metadata entries as `key=value`
  (we will soon add support in `ipfs-cluster-ctl`). Metadata can be added as
  query arguments to the Pin or PinPath endpoints: `POST
  /pins/<cid-or-path>?meta-key1=value1&meta-key2=value2...`

Note that on this release we have also removed a lot of backwards-compatiblity
code for things older than version 0.8.0, which kept things working but
printed respective warnings. If you're upgrading from an old release, consider
comparing your configuration with the new default one.


#### List of changes

##### Features

  * Add full support for environment variables in configurations and initialization | [ipfs/ipfs-cluster#656](https://github.com/ipfs/ipfs-cluster/issues/656) | [ipfs/ipfs-cluster#663](https://github.com/ipfs/ipfs-cluster/issues/663) | [ipfs/ipfs-cluster#667](https://github.com/ipfs/ipfs-cluster/issues/667)
  * Switch to codecov | [ipfs/ipfs-cluster#683](https://github.com/ipfs/ipfs-cluster/issues/683)
  * Add auto-resolving IPFS paths | [ipfs/ipfs-cluster#450](https://github.com/ipfs/ipfs-cluster/issues/450) | [ipfs/ipfs-cluster#634](https://github.com/ipfs/ipfs-cluster/issues/634)
  * Support user-defined allocations | [ipfs/ipfs-cluster#646](https://github.com/ipfs/ipfs-cluster/issues/646) | [ipfs/ipfs-cluster#647](https://github.com/ipfs/ipfs-cluster/issues/647)
  * Support user-defined metadata in pin objects | [ipfs/ipfs-cluster#681](https://github.com/ipfs/ipfs-cluster/issues/681)
  * Make normal types serializable and remove `*Serial` types | [ipfs/ipfs-cluster#654](https://github.com/ipfs/ipfs-cluster/issues/654) | [ipfs/ipfs-cluster#688](https://github.com/ipfs/ipfs-cluster/issues/688) | [ipfs/ipfs-cluster#700](https://github.com/ipfs/ipfs-cluster/issues/700)
  * Support IPFS paths in the IPFS proxy | [ipfs/ipfs-cluster#480](https://github.com/ipfs/ipfs-cluster/issues/480) | [ipfs/ipfs-cluster#690](https://github.com/ipfs/ipfs-cluster/issues/690)
  * Use go-datastore as backend for the cluster state | [ipfs/ipfs-cluster#655](https://github.com/ipfs/ipfs-cluster/issues/655)
  * Upgrade dependencies | [ipfs/ipfs-cluster#675](https://github.com/ipfs/ipfs-cluster/issues/675) | [ipfs/ipfs-cluster#679](https://github.com/ipfs/ipfs-cluster/issues/679) | [ipfs/ipfs-cluster#686](https://github.com/ipfs/ipfs-cluster/issues/686) | [ipfs/ipfs-cluster#687](https://github.com/ipfs/ipfs-cluster/issues/687)
  * Adopt MIT+Apache 2 License (no more sign-off required) | [ipfs/ipfs-cluster#692](https://github.com/ipfs/ipfs-cluster/issues/692)
  * Add codecov configurtion file | [ipfs/ipfs-cluster#693](https://github.com/ipfs/ipfs-cluster/issues/693)
  * Additional tests for basic auth | [ipfs/ipfs-cluster#645](https://github.com/ipfs/ipfs-cluster/issues/645) | [ipfs/ipfs-cluster#694](https://github.com/ipfs/ipfs-cluster/issues/694)

##### Bug fixes

  * Fix docker compose tests | [ipfs/ipfs-cluster#696](https://github.com/ipfs/ipfs-cluster/issues/696)
  * Hide `ipfsproxy.extract_headers_ttl` and `ipfsproxy.extract_headers_path` options by default | [ipfs/ipfs-cluster#699](https://github.com/ipfs/ipfs-cluster/issues/699)

#### Upgrading notices

This release needs an state upgrade before starting the Cluster daemon. Run `ipfs-cluster-service state upgrade` or run it as `ipfs-cluster-service daemon --upgrade`. We recommend backing up the `~/.ipfs-cluster` folder or exporting the pinset with `ipfs-cluster-service state export`.

##### Configuration changes

Configurations now respects environment variables for all sections. They are
in the form:

`CLUSTER_COMPONENTNAME_KEYNAMEWITHOUTSPACES=value`

Environment variables will override `service.json` configuration options when
defined and the Cluster peer is started. `ipfs-cluster-service init` will
reflect the value of any existing environment variables in the new
`service.json` file.

##### REST API

The main breaking change to the REST API corresponds to the JSON
representation of CIDs in response objects:

* Before: `"cid": "Qm...."`
* Now: `"cid": { "/": "Qm...."}`

The new CID encoding is the default as defined by the `cid`
library. Unfortunately, there is no good solution to keep the previous
representation without copying all the objects (an innefficient technique we
just removed). The new CID encoding is otherwise aligned with the rest of the
stack.

The API also gets two new "Path" endpoints:

* `POST /pins/<ipfs|ipns|ipld>/<path>/...` and
* `DELETE /pins/<ipfs|ipns|ipld>/<path>/...`

Thus, it is equivalent to pin a CID with `POST /pins/<cid>` (as before) or
with `POST /pins/ipfs/<cid>`.

The calls will however fail when a non-compliant IPFS path is provided: `POST
/pins/<cid>/my/path` will fail because all paths must start with the `/ipfs`,
`/ipns` or `/ipld` components.

##### Go APIs

This release introduces lots of changes to the Go APIs, including the Go REST
API client, as we have started returning pointers to objects rather than the
objects directly. The `Pin` will now take `api.PinOptions` instead of
different arguments corresponding to the options. It is aligned with the new
`PinPath` and `UnpinPath`.

##### Other

As pointed above, 0.10.0's state migration is a required step to be able to
use future version of IPFS Cluster.

---

### v0.9.0 - 2019-02-18

#### Summary

IPFS Cluster version 0.9.0 comes with one big new feature, [OpenCensus](https://opencensus.io) support! This allows for the collection of distributed traces and metrics from the IPFS Cluster application as well as supporting libraries. Currently, we support the use of [Jaeger](https://jaegertracing.io) as the tracing backend and [Prometheus](https://prometheus.io) as the metrics backend. Support for other [OpenCensus backends](https://opencensus.io/exporters/) will be added as requested by the community.

#### List of changes

##### Features

  * Integrate [OpenCensus](https://opencensus.io) tracing and metrics into IPFS Cluster codebase | [ipfs/ipfs-cluster#486](https://github.com/ipfs/ipfs-cluster/issues/486) | [ipfs/ipfs-cluster#658](https://github.com/ipfs/ipfs-cluster/issues/658) | [ipfs/ipfs-cluster#659](https://github.com/ipfs/ipfs-cluster/issues/659) | [ipfs/ipfs-cluster#676](https://github.com/ipfs/ipfs-cluster/issues/676) | [ipfs/ipfs-cluster#671](https://github.com/ipfs/ipfs-cluster/issues/671) | [ipfs/ipfs-cluster#674](https://github.com/ipfs/ipfs-cluster/issues/674)

##### Bug Fixes

No bugs were fixed from the previous release.

##### Deprecated

  * The snap distribution of IPFS Cluster has been removed | [ipfs/ipfs-cluster#593](https://github.com/ipfs/ipfs-cluster/issues/593) | [ipfs/ipfs-cluster#649](https://github.com/ipfs/ipfs-cluster/issues/649).

#### Upgrading notices

##### Configuration changes

No changes to the existing configuration.

There are two new configuration sections with this release:

###### `tracing` section

The `tracing` section configures the use of Jaeger as a tracing backend.

```js
    "tracing": {
      "enable_tracing": false,
      "jaeger_agent_endpoint": "/ip4/0.0.0.0/udp/6831",
      "sampling_prob": 0.3,
      "service_name": "cluster-daemon"
    }
```

###### `metrics` section

The `metrics` section configures the use of Prometheus as a metrics collector.

```js
    "metrics": {
      "enable_stats": false,
      "prometheus_endpoint": "/ip4/0.0.0.0/tcp/8888",
      "reporting_interval": "2s"
    }
```

##### REST API

No changes to the REST API.

##### Go APIs

The Go APIs had the minor change of having a `context.Context` parameter added as the first argument 
to those that didn't already have it. This was to enable the proporgation of tracing and metric
values.

The following is a list of interfaces and their methods that were affected by this change:
 - Component
    - Shutdown
 - Consensus
    - Ready
    - LogPin
    - LogUnpin
    - AddPeer
    - RmPeer
    - State
    - Leader
    - WaitForSync
    - Clean
    - Peers
 - IpfsConnector
    - ID
    - ConnectSwarm
    - SwarmPeers
    - RepoStat
    - BlockPut
    - BlockGet
 - Peered
    - AddPeer
    - RmPeer
 - PinTracker
    - Track
    - Untrack
    - StatusAll
    - Status
    - SyncAll
    - Sync
    - RecoverAll
    - Recover
 - Informer
    - GetMetric
 - PinAllocator
    - Allocate
 - PeerMonitor
    - LogMetric
    - PublishMetric
    - LatestMetrics
 - state.State
    - Add
    - Rm
    - List
    - Has
    - Get
    - Migrate
 - rest.Client
    - ID
    - Peers
    - PeerAdd
    - PeerRm
    - Add
    - AddMultiFile
    - Pin
    - Unpin
    - Allocations
    - Allocation
    - Status
    - StatusAll
    - Sync
    - SyncAll
    - Recover
    - RecoverAll
    - Version
    - IPFS
    - GetConnectGraph
    - Metrics

These interface changes were also made in the respective implementations.
All export methods of the Cluster type also had these changes made.


##### Other

No other things.

---

### v0.8.0 - 2019-01-16

#### Summary

IPFS Cluster version 0.8.0 comes with a few useful features and some bugfixes.
A significant amount of work has been put to correctly handle CORS in both the
REST API and the IPFS Proxy endpoint, fixing some long-standing issues (we
hope once are for all).

There has also been heavy work under the hood to separate the IPFS HTTP
Connector (the HTTP client to the IPFS daemon) from the IPFS proxy, which is
essentially an additional Cluster API. Check the configuration changes section
below for more information about how this affects the configuration file.

Finally we have some useful small features:

* The `ipfs-cluster-ctl status --filter` option allows to just list those
items which are still `pinning` or `queued` or `error` etc. You can combine
multiple filters. This translates to a new `filter` query parameter in the
`/pins` API endpoint.
* The `stream-channels=false` query parameter for the `/add` endpoint will let
the API buffer the output when adding and return a valid JSON array once done,
making this API endpoint behave like a regular, non-streaming one.
`ipfs-cluster-ctl add --no-stream` acts similarly, but buffering on the client
side. Note that this will cause in-memory buffering of potentially very large
responses when the number of added files is very large, but should be
perfectly fine for regular usage.
* The `ipfs-cluster-ctl add --quieter` flag now applies to the JSON output
too, allowing the user to just get the last added entry JSON object when
adding a file, which is always the root hash.

#### List of changes

##### Features

  * IPFS Proxy extraction to its own `API` component: `ipfsproxy` | [ipfs/ipfs-cluster#453](https://github.com/ipfs/ipfs-cluster/issues/453) | [ipfs/ipfs-cluster#576](https://github.com/ipfs/ipfs-cluster/issues/576) | [ipfs/ipfs-cluster#616](https://github.com/ipfs/ipfs-cluster/issues/616) | [ipfs/ipfs-cluster#617](https://github.com/ipfs/ipfs-cluster/issues/617)
  * Add full CORS handling to `restapi` | [ipfs/ipfs-cluster#639](https://github.com/ipfs/ipfs-cluster/issues/639) | [ipfs/ipfs-cluster#640](https://github.com/ipfs/ipfs-cluster/issues/640)
  * `restapi` configuration section entries can be overriden from environment variables | [ipfs/ipfs-cluster#609](https://github.com/ipfs/ipfs-cluster/issues/609)
  * Update to `go-ipfs-files` 2.0 | [ipfs/ipfs-cluster#613](https://github.com/ipfs/ipfs-cluster/issues/613)
  * Tests for the `/monitor/metrics` endpoint | [ipfs/ipfs-cluster#587](https://github.com/ipfs/ipfs-cluster/issues/587) | [ipfs/ipfs-cluster#622](https://github.com/ipfs/ipfs-cluster/issues/622)
  * Support `stream-channels=fase` query parameter in `/add` | [ipfs/ipfs-cluster#632](https://github.com/ipfs/ipfs-cluster/issues/632) | [ipfs/ipfs-cluster#633](https://github.com/ipfs/ipfs-cluster/issues/633)
  * Support server side `/pins` filtering  | [ipfs/ipfs-cluster#445](https://github.com/ipfs/ipfs-cluster/issues/445) | [ipfs/ipfs-cluster#478](https://github.com/ipfs/ipfs-cluster/issues/478) | [ipfs/ipfs-cluster#627](https://github.com/ipfs/ipfs-cluster/issues/627)
  * `ipfs-cluster-ctl add --no-stream` option | [ipfs/ipfs-cluster#632](https://github.com/ipfs/ipfs-cluster/issues/632) | [ipfs/ipfs-cluster#637](https://github.com/ipfs/ipfs-cluster/issues/637)
  * Upgrade dependencies and libp2p to version 6.0.29 | [ipfs/ipfs-cluster#624](https://github.com/ipfs/ipfs-cluster/issues/624)

##### Bug fixes

 * Respect IPFS daemon response headers on non-proxied calls | [ipfs/ipfs-cluster#382](https://github.com/ipfs/ipfs-cluster/issues/382) | [ipfs/ipfs-cluster#623](https://github.com/ipfs/ipfs-cluster/issues/623) | [ipfs/ipfs-cluster#638](https://github.com/ipfs/ipfs-cluster/issues/638)
 * Fix `ipfs-cluster-ctl` usage with HTTPs and `/dns*` hostnames | [ipfs/ipfs-cluster#626](https://github.com/ipfs/ipfs-cluster/issues/626)
 * Minor fixes in sharness | [ipfs/ipfs-cluster#641](https://github.com/ipfs/ipfs-cluster/issues/641) | [ipfs/ipfs-cluster#643](https://github.com/ipfs/ipfs-cluster/issues/643)
 * Fix error handling when parsing the configuration | [ipfs/ipfs-cluster#642](https://github.com/ipfs/ipfs-cluster/issues/642)



#### Upgrading notices

This release comes with some configuration changes that are important to notice,
even though the peers will start with the same configurations as before.

##### Configuration changes

##### `ipfsproxy` section

This version introduces a separate `ipfsproxy` API component. This is
reflected in the `service.json` configuration, which now includes a new
`ipfsproxy` subsection under the `api` section. By default it looks like:

```js
    "ipfsproxy": {
      "node_multiaddress": "/ip4/127.0.0.1/tcp/5001",
      "listen_multiaddress": "/ip4/127.0.0.1/tcp/9095",
      "read_timeout": "0s",
      "read_header_timeout": "5s",
      "write_timeout": "0s",
      "idle_timeout": "1m0s"
   }
```

We have however added the necessary safeguards to keep backwards compatibility
for this release. If the `ipfsproxy` section is empty, it will be picked up from
the `ipfshttp` section as before. An ugly warning will be printed in this case.

Based on the above, the `ipfshttp` configuration section loses the
proxy-related options. Note that `node_multiaddress` stays in both component
configurations and should likely be the same in most cases, but you can now
potentially proxy requests to a different daemon than the one used by the
cluster peer.

Additional hidden configuration options to manage custom header extraction
from the IPFS daemon (for power users) have been added to the `ipfsproxy`
section but are not shown by default when initializing empty
configurations. See the documentation for more details.

###### `restapi` section

The introduction of proper CORS handling in the `restapi` component introduces
a number of new keys:

```js
      "cors_allowed_origins": [
        "*"
      ],
      "cors_allowed_methods": [
        "GET"
      ],
      "cors_allowed_headers": [],
      "cors_exposed_headers": [
        "Content-Type",
        "X-Stream-Output",
        "X-Chunked-Output",
        "X-Content-Length"
      ],
      "cors_allow_credentials": true,
      "cors_max_age": "0s"
```

Note that CORS will be essentially unconfigured when these keys are not
defined.

The `headers` key, which was used before to add some CORS related headers
manually, takes a new empty default. **We recommend emptying `headers` from
any CORS-related value.**


##### REST API

The REST API is fully backwards compatible:

* The `GET /pins` endpoint takes a new `?filter=<filter>` option. See
  `ipfs-cluster-ctl status --help` for acceptable values.
* The `POST /add` endpoint accepts a new `?stream-channels=<true|false>`
  option. By default it is set to `true`.

##### Go APIs

The signature for the `StatusAll` method in the REST `client` module has
changed to include a `filter` parameter.

There may have been other minimal changes to internal exported Go APIs, but
should not affect users.

##### Other

Proxy requests which are handled by the Cluster peer (`/pin/ls`, `/pin/add`,
`/pin/rm`, `/repo/stat` and `/add`) will now attempt to fully mimic ipfs
responses to the header level. This is done by triggering CORS pre-flight for
every hijacked request along with an occasional regular request to `/version`
to extract other headers (and possibly custom ones).

The practical result is that the proxy now behaves correctly when dropped
instead of IPFS into CORS-aware contexts (like the browser).

---

### v0.7.0 - 2018-11-01

#### Summary

IPFS Cluster version 0.7.0 is a maintenance release that includes a few bugfixes and some small features.

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
