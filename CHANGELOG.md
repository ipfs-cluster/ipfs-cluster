# IPFS Cluster Changelog

### v1.1.2 - 2024-11-28

IPFS Cluster v1.1.2 is a maintenance release which tunes internal pubsub
configuration to be less demanding and resilient, as well as exposing some
of said configuration options.

It additionally contains a bugfix in go-ds-crdt for an issue that can cause
divergence between peers (https://github.com/ipfs/go-ds-crdt/pull/241). The
issue manifests itself when a value has been removed (i.e. when doing `pin rm`
on cluster) and re-added on a different replica before the removal operation
has been applied there. It may manifest itself in incosistencies in pin
information depending on the peer. The fix involves running an automatic
migration that ensures all the pin informations are aligned and that requires
a write operation for every pin that has ever been deleted pins to ensure that
cluster is returning the right values. This happens automatically on the first
boot after upgrade.

#### List of changes

##### Breaking changes

There are no breaking changes on this release.

##### Features

* Gossipsub: optimize for diverse clusters with many peers | [ipfs/ipfs-cluster#2071](https://github.com/ipfs/ipfs-cluster/issues/2071)
* ipfshttp: improve logic to update informer metrics | [ipfs/ipfs-cluster#2073](https://github.com/ipfs/ipfs-cluster/issues/2073)

##### Bug fixes

* crdt: Bubble bugfix for diverging states | [ipfs/ipfs-cluster#2115](https://github.com/ipfs/ipfs-cluster/issues/2115)

##### Other changes

* Dependency upgrades | [ipfs/ipfs-cluster#2074](https://github.com/ipfs/ipfs-cluster/issues/2074) | [ipfs/ipfs-cluster#2075](https://github.com/ipfs/ipfs-cluster/issues/2075) | [ipfs/ipfs-cluster#2115](https://github.com/ipfs/ipfs-cluster/issues/2115) | [ipfs/ipfs-cluster#2117](https://github.com/ipfs/ipfs-cluster/issues/2117)

#### Upgrading notices

##### Configuration changes

The main `cluster` configuration section now contains a `pubsub` sub-section
which, when not present, takes the following defaults:

```js
    "pubsub": {
      "seen_messages_ttl": "30m0s",
      "heartbeat_interval": "10s",
      "d_factor": 4,
      "history_gossip": 2,
      "history_length": 6,
      "flood_publish": false
    },
```

Details on the meaning of the options can be obtained in the
[pubsub documentation](https://pkg.go.dev/github.com/libp2p/go-libp2p-pubsub#GossipSubParams)
or in the [ipfs-cluster documentation for the Config object](https://pkg.go.dev/github.com/ipfs/ipfs-cluster?utm_source=godoc#Config).

##### REST API

No changes.

##### Pinning Service API

No changes.

##### IPFS Proxy API

No changes.

##### Go APIs

No relevant changes.

##### Other

As mentioned, the crdt datastore will run a migration on first start. A message will be printed when it finishes.

---

### v1.1.1 - 2024-06-23

IPFS Cluster v1.1.1 is a maintenance release mostly due to a libp2p-pubsub bug
that may impair the correct distribution of broadcasted metrics in large
clusters.

Along with other dependency upgrades and small fixes, we have also added a new
endpoint to retrieve peer-bandwidth statistics by libp2p protocol.

#### List of changes

##### Breaking changes

There are no breaking changes on this release.

##### Features

* Libp2p metrics and bandwidth stats for libp2p protocols | [ipfs/ipfs-cluster#2056](https://github.com/ipfs/ipfs-cluster/issues/2056)
* ctl: dedicated return value for timeouts | [ipfs/ipfs-cluster#1675](https://github.com/ipfs/ipfs-cluster/issues/1675) | [ipfs/ipfs-cluster#2057](https://github.com/ipfs/ipfs-cluster/issues/2057)

##### Bug fixes

* Fix: freebsd and windows builds | [ipfs/ipfs-cluster#2055](https://github.com/ipfs/ipfs-cluster/issues/2055)
* Fix: defer close() file in ipfs-cluster-follow | [ipfs/ipfs-cluster#2058](https://github.com/ipfs/ipfs-cluster/issues/2058)
* Fix: pubsub propagation issues | [ipfs/ipfs-cluster#2061](https://github.com/ipfs/ipfs-cluster/issues/2061) | [ipfs/ipfs-cluster#2062](https://github.com/ipfs/ipfs-cluster/issues/2062)

##### Other changes

* Dependency upgrades | [ipfs/ipfs-cluster#2062](https://github.com/ipfs/ipfs-cluster/issues/2062)

#### Upgrading notices

##### Configuration changes

No changes.

##### REST API

A new `/health/bandwidth` endpoint has been added. This endpoint returns an
object with keys corresponding to libp2p protocols. Each value is a "bandwidth
stats" object with current rate and totals for inbound and outbound streams,
aggregated by that protocol.

##### Pinning Service API

No changes.

##### IPFS Proxy API

No changes.

##### Go APIs

No relevant changes.

##### Other

Nothing.

---


### v1.1.0 - 2024-05-06

IPFS Cluster v1.1.0 is a maintenance release that comes with a number of
improvements in libp2p connection and resource management. We are bumping the
minor release version to bring the attention to the slight behavioral changes
included in this release.

In order to improve how clusters with a very large number of peers behave, we
have changed the previous logic which made every peer reconnect constantly to
3 specific peers (usually the "trusted peers" in collaborative clusters). For
obvious reasons, this caused bottlenecks when the clusters grew into the
thousands of peers. Swarm connections should now grow more organically and we
only re-bootstrap when they fall below expectable levels.

Anothe relevant change is the exposure of the libp2p Resource Manager
settings, and the new defaults, which limit libp2p usages to 25% of the
system's total memory and 50% of the process' available file descriptors. The
limits can be adjusted as explained below. The new defaults, along with other
internal details controlling the initialization of the resource manager are
likely more restrictive than the defaults used in previous versions. That
means that memory-constrained systems may start seeing resource-manager errors
where there were none before. The solution is to increase the limits. The
limits are conservative as Kubo is the major resource user at the end of the
day.

We have also updated to the latest Pebble release. This should not cause any
problems for users that already bumped `major_format_version` when upgrading
to v1.0.8, otherwise we recommend setting it to `16` per the warning printed
on daemon's start.

#### List of changes

##### Breaking changes

There are no breaking changes on this release.

##### Features

* cluster: expose and customize libp2p's Resource Manager | [ipfs/ipfs-cluster#2039](https://github.com/ipfs/ipfs-cluster/issues/2039) | [ipfs/ipfs-cluster#2049](https://github.com/ipfs/ipfs-cluster/issues/2049)
* ipfsproxy: support talking to Kubo over unix sockets | [ipfs/ipfs-cluster#2027](https://github.com/ipfs/ipfs-cluster/issues/2027)
* pebble: enable in all archs as default datastore | [ipfs/ipfs-cluster#2005](https://github.com/ipfs/ipfs-cluster/issues/2005) | [ipfs/ipfs-cluster#2007](https://github.com/ipfs/ipfs-cluster/issues/2007)
* pebble: set default MajorVersionFormat to newest | [ipfs/ipfs-cluster#2019](https://github.com/ipfs/ipfs-cluster/issues/2019)
* cluster: Announce and NoAnnounce options | [ipfs/ipfs-cluster#952](https://github.com/ipfs/ipfs-cluster/issues/952) | [ipfs/ipfs-cluster#2010](https://github.com/ipfs/ipfs-cluster/issues/2010)

##### Bug fixes

* go-dot: missing dependency, cannot compile | [ipfs/ipfs-cluster#2052](https://github.com/ipfs/ipfs-cluster/issues/2052) | [ipfs/ipfs-cluster#2053](https://github.com/ipfs/ipfs-cluster/issues/2053)
* pebble: fix debug logging not happening. Harden settings against footguns | [ipfs/ipfs-cluster#2047](https://github.com/ipfs/ipfs-cluster/issues/2047)
* config: default empty multiaddresses should be `[]` instead of `null` | [ipfs/ipfs-cluster#2051](https://github.com/ipfs/ipfs-cluster/issues/2051)

##### Other changes

* syntax improvement | [ipfs/ipfs-cluster#2042](https://github.com/ipfs/ipfs-cluster/issues/2042)
* Dependency upgrades (including Pebble and Raft) | [ipfs/ipfs-cluster#2044](https://github.com/ipfs/ipfs-cluster/issues/2044) | [ipfs/ipfs-cluster#2048](https://github.com/ipfs/ipfs-cluster/issues/2048) | [ipfs/ipfs-cluster#2050](https://github.com/ipfs/ipfs-cluster/issues/2050)

#### Upgrading notices

##### Configuration changes

A `resource_manager` setting has been added to the main `cluster` configuration section:

```
cluster: {
  ...
  "resource_manager": {
    "enabled": true,
    "memory_limit_bytes": 0,
    "file_descriptors_limit": 0
  },
```

when not present, the defaults will be as shown above. Using negative values will error.

The new setting controls the working limits for the libp2p Resource
Manager. `0` means "based on system's resources":

* `memory_limit_bytes` defaults to 25% of the system's total memory when set
  to `0`, with a minimum of 1GiB.
* `file_descriptors_limit` defaults to 50% of the process' file descriptor limit when set to `0`.

These limits can be set manually, or the resource manager can be fully
disabled by toggling the `enabled` setting.

When the limits are reached, libp2p will print warnings and errors as
connections and libp2p streams are dropped. Note that the limits only affect
libp2p resources and not the total memory usage of the IPFS Cluster daemon.


##### REST API

No changes.

##### Pinning Service API

No changes.

##### IPFS Proxy API

No changes.

##### Go APIs

No relevant changes.

##### Other

Nothing.

---

### v1.0.8 - 2024-01-30

IPFS Cluster v1.0.8 is a maintenance release.

This release updates dependencies (latest boxo and libp2p) and should bring a couple of Pebble-related improvements:
  * We have upgraded Pebble's version. Some users have reported deadlocks in writes to Pebble ([ipfs/ipfs-cluster#2009](https://github.com/ipfs/ipfs-cluster/issues/2009)) and this seems to have helped.
  * Pebble now supports 32-bit so it can be the default for all archs.
  * We added a warning when Pebble's newest `MajorFormatVersion` is higher than
  what is used in the configuration. **Users should increase their `major_format_version`
  to maintain forward-compatibility with future versions of Pebble.**

Additionally, some bugs have been fixed and a couple of useful features added, as mentioned below.

#### List of changes

##### Breaking changes

There are no breaking changes on this release.

##### Features

* ipfshttp: support talking to Kubo over unix sockets | [ipfs/ipfs-cluster#1999](https://github.com/ipfs/ipfs-cluster/issues/1999)
* ipfsproxy: support talking to Kubo over unix sockets | [ipfs/ipfs-cluster#2027](https://github.com/ipfs/ipfs-cluster/issues/2027)
* pebble: enable in all archs as default datastore | [ipfs/ipfs-cluster#2005](https://github.com/ipfs/ipfs-cluster/issues/2005) | [ipfs/ipfs-cluster#2007](https://github.com/ipfs/ipfs-cluster/issues/2007)
* pebble: set default MajorVersionFormat to newest | [ipfs/ipfs-cluster#2019](https://github.com/ipfs/ipfs-cluster/issues/2019)
* cluster: Announce and NoAnnounce options | [ipfs/ipfs-cluster#952](https://github.com/ipfs/ipfs-cluster/issues/952) | [ipfs/ipfs-cluster#2010](https://github.com/ipfs/ipfs-cluster/issues/2010)


##### Bug fixes

* ipfs-cluster-follow: issue numpin and pinqueue metrics to other peers | [ipfs/ipfs-cluster#2011](https://github.com/ipfs/ipfs-cluster/issues/2011) | [ipfs/ipfs-cluster#2016](https://github.com/ipfs/ipfs-cluster/issues/2016)
* ipfshttp: do no pre-resolve node_multiaddresses | [ipfs/ipfs-cluster#2004](https://github.com/ipfs/ipfs-cluster/issues/2004) | [ipfs/ipfs-cluster#2017](https://github.com/ipfs/ipfs-cluster/issues/2017)
* ipfsproxy: do no pre-resolve node_multiaddresses | [ipfs/ipfs-cluster#2027](https://github.com/ipfs/ipfs-cluster/issues/2027)
* pebble: deadlock | [ipfs/ipfs-cluster#2009](https://github.com/ipfs/ipfs-cluster/issues/2009)

##### Other changes

* The `Dockerfile-bundle` file has been removed (unmaintained) | [ipfs/ipfs-cluster#1986](https://github.com/ipfs/ipfs-cluster/issues/1986)
* Dependency upgrades | [ipfs/ipfs-cluster#2007](https://github.com/ipfs/ipfs-cluster/issues/2007) | [ipfs/ipfs-cluster#2018](https://github.com/ipfs/ipfs-cluster/issues/2018) | [ipfs/ipfs-cluster#2026](https://github.com/ipfs/ipfs-cluster/issues/2026)

#### Upgrading notices

##### Configuration changes

Two new options have been added to forcefully control the cluster peer libp2p host address announcements: `cluster.announce_multiaddress` and `cluster.no_announce_multiaddress`. Both take a slice of multiaddresses.

##### REST API

No changes.

##### Pinning Service API

No changes.

##### IPFS Proxy API

No changes.

##### Go APIs

No relevant changes.

##### Other

Nothing.

---


### v1.0.7 - 2023-10-12

IPFS Cluster v1.0.7 is a maintenance release.

This release updates dependencies and switches to the Boxo library suite with
the latest libp2p release.

See the notes below for a list of changes and bug fixes.

#### List of changes

##### Breaking changes

There are no breaking changes on this release.

##### Features

* API: Add a /health endpoint that bypasses authorization | [ipfs/ipfs-cluster#1884](https://github.com/ipfs/ipfs-cluster/issues/1884) | [ipfs/ipfs-cluster#1919](https://github.com/ipfs/ipfs-cluster/issues/1919)
* Systemd notify support | [ipfs/ipfs-cluster#1144](https://github.com/ipfs/ipfs-cluster/issues/1144) | [ipfs/ipfs-cluster#1894](https://github.com/ipfs/ipfs-cluster/issues/1894)
* Add support for pinning only on "untrusted" peers | [ipfs/ipfs-cluster#1976](https://github.com/ipfs/ipfs-cluster/issues/1976) | [ipfs/ipfs-cluster#1977](https://github.com/ipfs/ipfs-cluster/issues/1977)
* Docker image with multiarch support | [ipfs/ipfs-cluster#1085](https://github.com/ipfs/ipfs-cluster/issues/1085) | [ipfs/ipfs-cluster#1984](https://github.com/ipfs/ipfs-cluster/issues/1984) | [ipfs/ipfs-cluster#1368](https://github.com/ipfs/ipfs-cluster/issues/1368)

##### Bug fixes

* MaxConcurrentCompactions missing from Pebble configuration | [ipfs/ipfs-cluster#1895](https://github.com/ipfs/ipfs-cluster/issues/1895) | [ipfs/ipfs-cluster#1900](https://github.com/ipfs/ipfs-cluster/issues/1900)
* Missing newline in JSON stream from proxy API when doing pin/ls | [ipfs/ipfs-cluster#1885](https://github.com/ipfs/ipfs-cluster/issues/1885) | [ipfs/ipfs-cluster#1893](https://github.com/ipfs/ipfs-cluster/issues/1893)

##### Other changes

* Dependency updates and upgrade to Boxo | [ipfs/ipfs-cluster#1901](https://github.com/ipfs/ipfs-cluster/issues/1901) | [ipfs/ipfs-cluster#1980](https://github.com/ipfs/ipfs-cluster/issues/1980)

#### Upgrading notices

##### Configuration changes

A new option `cluster.pin_only_on_untrusted_peers` has been added, opposite to the `pin_only_on_trusted_peers` that already existed. Defaults to `false`. Both options cannot be `true`. When enabled, only "untrusted" peers are considered for pin allocations.

##### REST API

A new `/health` endpoint has been added, returns 204 (No Content) and no
body. It can be used to monitor that the service is running.

##### Pinning Service API

A new `/health` endpoint has been added, returns 204 (No Content) and no
body. It can be used to monitor that the service is running.

##### IPFS Proxy API

Calling `/api/v0/pin/ls` on the proxy api now adds a final new line at the end
of the response. This should align with what Kubo does.

##### Go APIs

No relevant changes.

##### Other

`ipfs-cluster-service` now sends a notification to systemd when it becomes
"ready" (that is, after all initialization is completed). This means systemd
service files for `ipfs-cluster-service` can use `Type=notify`.

The official docker images are now built with support for linux/amd64,
linux/arm/v7 and linux/arm64/v8 architectures. We have also switched to Alpine
Linux as base image (instead of Busybox). Binaries are now built with
`CGO_ENABLED=0`.

---

### v1.0.6 - 2023-03-06

IPFS Cluster v1.0.6 is a maintenance release with some small fixes. The main
change in this release is that `pebble` becomes the default datastore backend,
as we mentioned in the last release.

Pebble is the datastore backend used by CockroachDB and is inspired in
RocksDB.  Upon testing, Pebble has demonstrated good performance and optimal
disk usage. Pebble incorporates modern datastore-backend features such as
compression, caching and bloom filters. Pebble is actively maintained by the
CockroachDB team and therefore seems like the best default choice for IPFS
Cluster.

Badger3, a very good alternative choice, becomes the new default for platforms
not supported by Pebble (mainly 32bit architectures). Badger and LevelDB are
still supported, but we heavily disencourage their usage for new Cluster peers.

#### List of changes

##### Breaking changes

There are no breaking changes on this release.

##### Features

* Pebble: make the new default | [ipfs/ipfs-cluster#1881](https://github.com/ipfs/ipfs-cluster/issues/1881) | [ipfs/ipfs-cluster#1847](https://github.com/ipfs/ipfs-cluster/issues/1847)
* ctl: support `ipfs-cluster-ctl add --no-pin` flag | [ipfs/ipfs-cluster#1852](https://github.com/ipfs/ipfs-cluster/issues/1852)

##### Bug fixes

* PinStatus information not up-to-date for re-pinned items with ongoing operations | [ipfs/ipfs-cluster#1785](https://github.com/ipfs/ipfs-cluster/issues/1785) | [ipfs/ipfs-cluster#1876](https://github.com/ipfs/ipfs-cluster/issues/1876)
* Broken builds for 32-bit architectures | [ipfs/ipfs-cluster#1844](https://github.com/ipfs/ipfs-cluster/issues/1844) | [ipfs/ipfs-cluster#1854](https://github.com/ipfs/ipfs-cluster/issues/1854) | [ipfs/ipfs-cluster#1851](https://github.com/ipfs/ipfs-cluster/issues/1851)

##### Other changes

* Switch to ipfs/Kubo docker image | [ipfs/ipfs-cluster#1846](https://github.com/ipfs/ipfs-cluster/issues/1846)
* Dependency updates | [ipfs/ipfs-cluster#1855](https://github.com/ipfs/ipfs-cluster/issues/1855) | [ipfs/ipfs-cluster#1880](https://github.com/ipfs/ipfs-cluster/issues/1880)
* Fix typo | [ipfs/ipfs-cluster#1882](https://github.com/ipfs/ipfs-cluster/issues/1882)

#### Upgrading notices

##### Configuration changes

The `pebble` section of the configuration has some additional options and new, adjusted defaults:


* `pebble`:

```js
    "pebble": {
      "pebble_options": {
        "cache_size_bytes": 1073741824,
        "bytes_per_sync": 1048576,
        "disable_wal": false,
        "flush_delay_delete_range": 0,
        "flush_delay_range_key": 0,
        "flush_split_bytes": 4194304,
        "format_major_version": 1,
        "l0_compaction_file_threshold": 750,
        "l0_compaction_threshold": 4,
        "l0_stop_writes_threshold": 12,
        "l_base_max_bytes": 134217728,
        "levels": [
          {
            "block_restart_interval": 16,
            "block_size": 4096,
            "block_size_threshold": 90,
            "compression": 2,
            "filter_type": 0,
            "filter_policy": 10,
            "index_block_size": 4096,
            "target_file_size": 4194304
          },
          {
            "block_restart_interval": 16,
            "block_size": 4096,
            "block_size_threshold": 90,
            "compression": 2,
            "filter_type": 0,
            "filter_policy": 10,
            "index_block_size": 4096,
            "target_file_size": 8388608
          },
          {
            "block_restart_interval": 16,
            "block_size": 4096,
            "block_size_threshold": 90,
            "compression": 2,
            "filter_type": 0,
            "filter_policy": 10,
            "index_block_size": 4096,
            "target_file_size": 16777216
          },
          {
            "block_restart_interval": 16,
            "block_size": 4096,
            "block_size_threshold": 90,
            "compression": 2,
            "filter_type": 0,
            "filter_policy": 10,
            "index_block_size": 4096,
            "target_file_size": 33554432
          },
          {
            "block_restart_interval": 16,
            "block_size": 4096,
            "block_size_threshold": 90,
            "compression": 2,
            "filter_type": 0,
            "filter_policy": 10,
            "index_block_size": 4096,
            "target_file_size": 67108864
          },
          {
            "block_restart_interval": 16,
            "block_size": 4096,
            "block_size_threshold": 90,
            "compression": 2,
            "filter_type": 0,
            "filter_policy": 10,
            "index_block_size": 4096,
            "target_file_size": 134217728
          },
          {
            "block_restart_interval": 16,
            "block_size": 4096,
            "block_size_threshold": 90,
            "compression": 2,
            "filter_type": 0,
            "filter_policy": 10,
            "index_block_size": 4096,
            "target_file_size": 268435456
          }
        ],
        "max_open_files": 1000,
        "mem_table_size": 67108864,
        "mem_table_stop_writes_threshold": 20,
        "read_only": false,
        "wal_bytes_per_sync": 0
      }
    }
```

##### REST API

No changes.

##### Pinning Service API

No changes.

##### IPFS Proxy API

No changes.

##### Go APIs

No relevant changes.

##### Other

The `--datastore` flag to `ipfs-cluster-service init` now defaults to `pebble`
in most platforms, and to `badger3` in those where Pebble is not supported
(arm, 386).


---

### v1.0.5 - 2023-01-27

IPFS Cluster v1.0.5 is a maintenance release with one main feature: support
for `badger3` and `pebble` datastores.

Additionally, this release fixes compatibility with Kubo v0.18.0 and addresses
the crashes related to libp2p autorelay that affected the previous version.

`pebble` and `badger3` are much newer backends that the already available
Badger and LevelDB. They are faster, use significantly less disk-space and
support additional options like compression. We have set `pebble` as the
default datastore used by the official Docker container, and we will be likely
making it the final default choice for new installations. In the meantime, we
encourage the community to try them out and provide feedback.

#### List of changes

##### Breaking changes

There are no breaking changes on this release.

##### Features

* Support for `pebble` and `badger3` datastores | [ipfs/ipfs-cluster#1809](https://github.com/ipfs/ipfs-cluster/issues/1809)

##### Bug fixes

* Fix panic on boot (related to libp2p AutoRelay) | [ipfs/ipfs-cluster#1796](https://github.com/ipfs/ipfs-cluster/issues/1796) | [ipfs/ipfs-cluster#1797](https://github.com/ipfs/ipfs-cluster/issues/1797)
* Fix error parsing quic-v1 multiaddresses from Kubo 0.18.0 | [ipfs/ipfs-cluster#1835](https://github.com/ipfs/ipfs-cluster/issues/1835) | [ipfs/ipfs-cluster#1841](https://github.com/ipfs/ipfs-cluster/issues/1841)
* Fix error building docker container with newer git | [ipfs/ipfs-cluster#1836](https://github.com/ipfs/ipfs-cluster/issues/1836)
* Fix building master due to pinned go-mfs dependency | [ipfs/ipfs-cluster#1817](https://github.com/ipfs/ipfs-cluster/issues/1817) | [ipfs/ipfs-cluster#1818](https://github.com/ipfs/ipfs-cluster/issues/1818)

##### Other changes

* Dependency updates (including libp2p v0.24.2) | [ipfs/ipfs-cluster#1813](https://github.com/ipfs/ipfs-cluster/issues/1813) | [ipfs/ipfs-cluster#1840](https://github.com/ipfs/ipfs-cluster/issues/1840)
* Switch Docker containers to `pebble` by default | [ipfs/ipfs-cluster#1842](https://github.com/ipfs/ipfs-cluster/issues/1842)

#### Upgrading notices

##### Configuration changes

The `datastore` section of the configuration now supports the two new datastore backends:

  * `badger3`:

```js
    "badger3": {
      "gc_discard_ratio": 0.2,
      "gc_interval": "15m0s",
      "gc_sleep": "10s",
      "badger_options": {
        "dir": "",
        "value_dir": "",
        "sync_writes": false,
        "num_versions_to_keep": 1,
        "read_only": false,
        "compression": 0,
        "in_memory": false,
        "metrics_enabled": true,
        "num_goroutines": 8,
        "mem_table_size": 67108864,
        "base_table_size": 2097152,
        "base_level_size": 10485760,
        "level_size_multiplier": 10,
        "table_size_multiplier": 2,
        "max_levels": 7,
        "v_log_percentile": 0,
        "value_threshold": 100,
        "num_memtables": 5,
        "block_size": 4096,
        "bloom_false_positive": 0.01,
        "block_cache_size": 0,
        "index_cache_size": 0,
        "num_level_zero_tables": 5,
        "num_level_zero_tables_stall": 15,
        "value_log_file_size": 1073741823,
        "value_log_max_entries": 1000000,
        "num_compactors": 4,
        "compact_l_0_on_close": false,
        "lmax_compaction": false,
        "zstd_compression_level": 1,
        "verify_value_checksum": false,
        "checksum_verification_mode": 0,
        "detect_conflicts": false,
        "namespace_offset": -1
      }
    }
```

* `pebble`:

```js
    "pebble": {
      "pebble_options": {
        "bytes_per_sync": 524288,
        "disable_wal": false,
        "flush_delay_delete_range": 0,
        "flush_delay_range_key": 0,
        "flush_split_bytes": 4194304,
        "format_major_version": 1,
        "l0_compaction_file_threshold": 500,
        "l0_compaction_threshold": 4,
        "l0_stop_writes_threshold": 12,
        "l_base_max_bytes": 67108864,
        "levels": [
          {
            "block_restart_interval": 16,
            "block_size": 4096,
            "block_size_threshold": 90,
            "compression": 1,
            "filter_type": 0,
            "index_block_size": 4096,
            "target_file_size": 2097152
          }
        ],
        "max_open_files": 1000,
        "mem_table_size": 4194304,
        "mem_table_stop_writes_threshold": 2,
        "read_only": false,
        "wal_bytes_per_sync": 0
      }
    }
```

In order to choose the backend during initialization, use the `--datastore` flag in `ipfs-cluster-service init --datastore <backend>`.

##### REST API

No changes.

##### Pinning Service API

No changes.

##### IPFS Proxy API

No changes.

##### Go APIs

No relevant changes.

##### Other

Docker containers now use `pebble` as the default datastore backend.
Nothing.

---


### v1.0.4 - 2022-09-26

IPFS Cluster v1.0.4 is a maintenance release addressing a couple of bugs and
adding more "state crdt" commands.

One of the bugs has potential to cause a panic, while a second one can
potentially dead-lock pinning operations and hang new pinning requests. We
recommend all users to upgrade as soon as possible.


#### List of changes

##### Breaking changes

There are no breaking changes on this release.

##### Features

* service: add "state crdt info/mark-dirty/mark-clean" commands | [ipfs/ipfs-cluster#1771](https://github.com/ipfs/ipfs-cluster/issues/1771)

##### Bug fixes

* Fix panic returned when request is canceled while rate-limited | [ipfs/ipfs-cluster#1770](https://github.com/ipfs/ipfs-cluster/issues/1770)
* Fix operationtracker not respecting context cancellations and never returning locks | [ipfs/ipfs-cluster#1768](https://github.com/ipfs/ipfs-cluster/issues/1768)
* Fix repinning of a CID not re-allocating errored peers as expected | [ipfs/ipfs-cluster#1774](https://github.com/ipfs/ipfs-cluster/issues/1774)


##### Other changes

No other changes.

#### Upgrading notices

##### Configuration changes

There are no configuration changes for this release.

##### REST API

No changes.

##### Pinning Service API

No changes.

##### IPFS Proxy API

No changes.

##### Go APIs

No relevant changes.

##### Other

Nothing.

---


### v1.0.3 - 2022-09-16

IPFS Cluster v1.0.3 is a maintenance release addressing some bugs and bringing
some improvements to error handling behavior, as well as a couple of small
features.

This release upgrades to the latest libp2p release (v0.22.0).

#### List of changes

##### Breaking changes

There are no breaking changes on this release.

##### Features

* ipfs proxy api: intercept and trigger cluster-pins on `/block/put` and `/dag/put` requests | [ipfs/ipfs-cluster#1738](https://github.com/ipfs/ipfs-cluster/issues/1738) | [ipfs/ipfs-cluster#1756](https://github.com/ipfs/ipfs-cluster/issues/1756)
* "ipfs-cluster-service state crdt dot" command generates a DOT file from the CRDT DAG | [ipfs/ipfs-cluster#1516](https://github.com/ipfs/ipfs-cluster/issues/1516)

##### Bug fixes

* Fix leaking goroutines on aborted /add requests | [ipfs/ipfs-cluster#1732](https://github.com/ipfs/ipfs-cluster/issues/1732)
* Fix ARM build panics because of unaligned structs | [ipfs/ipfs-cluster#1735](https://github.com/ipfs/ipfs-cluster/issues/1735) | [ipfs/ipfs-cluster#1736](https://github.com/ipfs/ipfs-cluster/issues/1736) | [ipfs/ipfs-cluster#1754](https://github.com/ipfs/ipfs-cluster/issues/1754)
* Fix "blockPut response CID XXX does not match the multihash of any blocks sent" warning wrongly displayed | [ipfs/ipfs-cluster#1706](https://github.com/ipfs/ipfs-cluster/issues/1706) | [ipfs/ipfs-cluster#1755](https://github.com/ipfs/ipfs-cluster/issues/1755)
* Fix behavior and error handling when IPFS is unavailable | [ipfs/ipfs-cluster#1733](https://github.com/ipfs/ipfs-cluster/issues/1733) | [ipfs/ipfs-cluster#1762](https://github.com/ipfs/ipfs-cluster/issues/1762)

##### Other changes

* Dependency upgrades | [ipfs/ipfs-cluster#1755](https://github.com/ipfs/ipfs-cluster/issues/1755)
* Update hashicorp raft libraries | [ipfs/ipfs-cluster#1757](https://github.com/ipfs/ipfs-cluster/issues/1757)

#### Upgrading notices

##### Configuration changes

There are no configuration changes for this release.

##### REST API

No changes.

##### Pinning Service API

No changes.

##### IPFS Proxy API

The IPFS Proxy now intercepts `/block/put` and `/dag/put` requests. This happens as follows:

* The request is first forwarded "as is" to the underlying IPFS daemon, with
  the `?pin` query parameter always set to `false`.
* If `?pin=true` was set, a cluster pin is triggered for every block and dag
  object uploaded (reminder that these endpoints accept multipart uploads).
* Regular IPFS response to the uploads is streamed back to the user.


##### Go APIs

No relevant changes.

##### Other

Note that more than 10 failed requests to IPFS will now result in a rate-limit
of 1req/s for any request to IPFS. This may cause things to queue up instead
hammering the ipfs daemon with requets that fail. The rate limit is removed as
soon as one request succeeds.

Also note that now Cluster peers that are started will not become fully
operable until IPFS has been detected to be available: no metrics will be
sent, no recover operations will be run etc. essentially the Cluster peer will
wait for IPFS to be available before starting to do things that need IPFS to
be available, rather than doing them right away and have failures.

---

### v1.0.2 - 2022-07-06

IPFS Cluster v1.0.2 is a maintenance release with bug fixes and another
iteration of the experimental support for the Pinning Services API that was
introduced on v1.0.0, including Bearer token authorization support for both
the REST and the Pinning Service APIs.

**This release includes a
  [security fix in the go-car library](yhttps://github.com/ipld/go-car/security/advisories/GHSA-9x4h-8wgm-8xfg)**. The
  security issue allows an attacker to crash a cluster peer or cause excessive
  memory usage when uploading CAR files via the REST API (`POST
  /add?format=car` endpoint).

This also the first release after moving the project from the "ipfs" to the
the "ipfs-cluster" Github organization, which means the project Go modules
have new paths (everything is redirected though). The Docker builds remain
inside the "ipfs" namespace (i.e. `docker pull ipfs/ipfs-cluster`).

IPFS Cluster is also ready to work with go-ipfs v0.13.0+. We recommend to upgrade.

#### List of changes

##### Breaking changes

##### Features

* REST/PinSVC API: support JWT bearer token authorization | [ipfs/ipfs-cluster#1703](https://github.com/ipfs/ipfs-cluster/issues/1703)
* crdt: commit pending batched pins on shutdown | [ipfs/ipfs-cluster#1697](https://github.com/ipfs/ipfs-cluster/issues/1697) | 1719
* Export a prometheus metric with the current disk informer value | [ipfs/ipfs-cluster#1725](https://github.com/ipfs/ipfs-cluster/issues/1725)

##### Bug fixes

* Fix adding large directories | [ipfs/ipfs-cluster#1691](https://github.com/ipfs/ipfs-cluster/issues/1691) | [ipfs/ipfs-cluster#1700](https://github.com/ipfs/ipfs-cluster/issues/1700)
* PinSVC API: fix compliance errors and bugs | [ipfs/ipfs-cluster#1704](https://github.com/ipfs/ipfs-cluster/issues/1704)
* Pintracker: fix missing and wrong values in PinStatus object fields for
  recovered operations | [ipfs/ipfs-cluster#1705](https://github.com/ipfs/ipfs-cluster/issues/1705)
* ctl: fix "Exp" label showing the pin timestamp instead of the experiation date | [ipfs/ipfs-cluster#1666](https://github.com/ipfs/ipfs-cluster/issues/1666) | [ipfs/ipfs-cluster#1716](https://github.com/ipfs/ipfs-cluster/issues/1716)
* Pintracker: fix races causing wrong counts in metrics | [ipfs/ipfs-cluster#1717](https://github.com/ipfs/ipfs-cluster/issues/1717) | [ipfs/ipfs-cluster#1729](https://github.com/ipfs/ipfs-cluster/issues/1729)
* Update go-car to v0.4.0 (security fixes) | [ipfs/ipfs-cluster#1730](https://github.com/ipfs/ipfs-cluster/issues/1730)

##### Other changes

* Improve language, fix typos to changelog | [ipfs/ipfs-cluster#1667](https://github.com/ipfs/ipfs-cluster/issues/1667)
* Update comment in docker-compose | [ipfs/ipfs-cluster#1689](https://github.com/ipfs/ipfs-cluster/issues/1689)
* Migrate from ipfs/ipfs-cluster to ipfs-cluster/ipfs-cluster | [ipfs/ipfs-cluster#1694](https://github.com/ipfs/ipfs-cluster/issues/1694)
* Enable spell-checking and fix spelling errors (US locale) | [ipfs/ipfs-cluster#1695](https://github.com/ipfs/ipfs-cluster/issues/1695)
* Enable CodeQL analysis and fix security warnings | [ipfs/ipfs-cluster#1696](https://github.com/ipfs/ipfs-cluster/issues/1696)
* Dependency upgrades: libp2p-0.20.1 etc. | [ipfs/ipfs-cluster#1711](https://github.com/ipfs/ipfs-cluster/issues/1711) | [ipfs/ipfs-cluster#1712](https://github.com/ipfs/ipfs-cluster/issues/1712) | [ipfs/ipfs-cluster#1724](https://github.com/ipfs/ipfs-cluster/issues/1724)
* API: improve debug logging during tls setup | [ipfs/ipfs-cluster#1715](https://github.com/ipfs/ipfs-cluster/issues/1715)

#### Upgrading notices

##### Configuration changes

There are no configuration changes for this release.

##### REST API

The REST API has a new `POST /token` endpoint, which returns a JSON object
with a JWT token (when correctly authenticated).

This token can be used to authenticate using `Authorization: Bearer <token>`
header on subsequent requests.

The token is tied and verified against a basic authentication user and
password, as configured in the `basic_auth_credentials` field.

At the moment we do not support revocation, expiration and other token
options.

##### Pinning Service API

The Pinning Service API has a new `POST /token` endpoint, which returns a JSON object
with a JWT token (when correctly authenticated). See the REST API section above.

##### IPFS Proxy API

No changes to IPFS Proxy API.

##### Go APIs

All cluster modules have new paths: every instance of "ipfs/ipfs-cluster" should now be "ipfs-cluster/ipfs-cluster".

##### Other

go-ipfs v0.13.0 introduced some changes to the Block/Put API. IPFS Cluster now
uses the `cid-format` option when performing Block-Puts. We believe the change
does not affect adding blocks and that it should still work with previous
go-ipfs versions, yet we recommend upgrading to go-ipfs v0.13.1 or later.


---

### v1.0.1 - 2022-05-06

IPFS Cluster v1.0.1 is a maintenance release ironing out some issues and
bringing a couple of improvements around observability of cluster performance:

* We have fixed the `ipfscluster_pins` metric and added a few new ones that
  help determine how fast the cluster can pin and add blocks.
* We have added a new Informer that broadcasts current pinning-queue size,
  which means we can take this information into account when making
  allocations, essentially allowing peers with big pinning queues to be
  relieved by peers with smaller pinning queues.

Please read below for a list of changes and things to watch out for.

#### List of changes

##### Breaking changes

Peers running IPFS Cluster v1.0.0 will not be able to read the pin's user-set
metadata fields for pins submitted by peers in later versions, since metadata
is now stored on a different protobuf field. If this is an issue, all peers in
the cluster should upgrade.

##### Features

* Pinqueue Informer: let pinning queue size inform allocation selection | [ipfs-cluster/ipfs-cluster#1649](https://github.com/ipfs-cluster/ipfs-cluster/issues/1649) | [ipfs-cluster/ipfs-cluster#1657](https://github.com/ipfs-cluster/ipfs-cluster/issues/1657)
* Metrics: add additional Prometheus metrics | [ipfs-cluster/ipfs-cluster#1650](https://github.com/ipfs-cluster/ipfs-cluster/issues/1650) | [ipfs-cluster/ipfs-cluster#1659](https://github.com/ipfs-cluster/ipfs-cluster/issues/1659)

##### Bug fixes

* Fix: state import can result in different CRDT-heads | [ipfs-cluster/ipfs-cluster#1547](https://github.com/ipfs-cluster/ipfs-cluster/issues/1547) | [ipfs-cluster/ipfs-cluster#1664](https://github.com/ipfs-cluster/ipfs-cluster/issues/1664)
* Fix: `ipfs-cluster-ctl pin ls` hangs | [ipfs-cluster/ipfs-cluster#1663](https://github.com/ipfs-cluster/ipfs-cluster/issues/1663)
* Fix: restapi client panics on retry | [ipfs-cluster/ipfs-cluster#1655](https://github.com/ipfs-cluster/ipfs-cluster/issues/1655) | [ipfs-cluster/ipfs-cluster#1662](https://github.com/ipfs-cluster/ipfs-cluster/issues/1662)
* Fix: bad behavior while adding and ipfs is down | [ipfs-cluster/ipfs-cluster#1646](https://github.com/ipfs-cluster/ipfs-cluster/issues/1646)
* Fix: `ipfscluster_pins` metric issues bad values | [ipfs-cluster/ipfs-cluster#1645](https://github.com/ipfs-cluster/ipfs-cluster/issues/1645)

##### Other changes

* Dependency upgrades (includes go-libp2p v0.19.1) | [ipfs-cluster/ipfs-cluster#1660](https://github.com/ipfs-cluster/ipfs-cluster/issues/1660)
* Build with go1.18 | [ipfs-cluster/ipfs-cluster#1661](https://github.com/ipfs-cluster/ipfs-cluster/issues/1661)
* Do not issue freespace metrics when freespace is 0 | [ipfs-cluster/ipfs-cluster#1656](https://github.com/ipfs-cluster/ipfs-cluster/issues/1656)
* Convert pinning/queued/error metrics go gauges | [ipfs-cluster/ipfs-cluster#1647](https://github.com/ipfs-cluster/ipfs-cluster/issues/1647) | [ipfs-cluster/ipfs-cluster#1651](https://github.com/ipfs-cluster/ipfs-cluster/issues/1651)

#### Upgrading notices

##### Configuration changes

There is a new `pinqueue` configuration object inside the `informer` section on newly initialized configurations:

```
  "informer": {
    ...
    "pinqueue": {
      "metric_ttl": "30s",
      "weight_bucket_size": 100000
    },
	...
```

This enables the Pinqueue Informer, which broadcasts metrics containing the size of the pinqueue with the metric weight divided by `weight_bucket_size`. The new metric is not used for allocations by default, and it needs to be manually added to the `allocate_by` option in the allocator, usually like:

```
"allocator": {
   "balanced": {
     "allocate_by": [
       "tag:group",
       "pinqueue",
       "freespace"
     ]
   }
```


##### REST API

No changes to REST API.

##### IPFS Proxy API

No changes to IPFS Proxy API.

##### Go APIs

No relevant changes to Go APIs, other than the PinTracker interface now requiring a `PinQueueSize` method.

##### Other

The following metrics are now available in the Prometheus endpoint when enabled:

```
ipfscluster_pins_ipfs_pins gauge
ipfscluster_pins_pin_add counter
ipfscluster_pins_pin_add_errors counter
ipfscluster_blocks_put counter
ipfscluster_blocks_added_size counter
ipfscluster_blocks_added counter
ipfscluster_blocks_put_error counter
```

The following metrics were converted from `counter` to `gauge`:

```
ipfscluster_pins_pin_queued
ipfscluster_pins_pinning
ipfscluster_pins_pin_error
```

Peers that are reporting `freespace` as 0 and which use this metric to
allocate pins, will no longer be available for allocations (they stop
broadcasting this metric). This means setting `StorageMax` on IPFS to 0
effectively prevents any pins from being explicitly allocated to a peer
(that is, when replication_factor != *everywhere*).

---

### v1.0.0 - 2022-04-22

IPFS Cluster v1.0.0 is a major release that represents that this project has
reached maturity and is able to perform and scale on production environment
(50+ million pins and 20 nodes).

This is a breaking release, v1.0.0 cluster peers are not compatible with
previous cluster peers as we have bumped the RPC protocol version (which had
remained unchanged since 0.12.0).

This release's major change is the switch to using streaming RPC endpoints for
several RPC methods (listing pins, listing statuses, listing peers, adding
blocks), which we added support for in go-libp2p-gorpc.

This causes major impact on two areas:

- Memory consumption with very large pinsets: before, listing all the pins on
  the HTTP API required loading all the pins in the pinset into memory, then
  responding with a json-array containing the full pinset. When working at
  large scale with multimillion pinsets, this caused large memory usage spikes
  (whenever the full pinset was needed anywhere). Streaming RPC means
  components no longer need to send requests or responses in a single large
  collection (a json array), but can individually stream items end-to-end,
  without having to load-all and store in memory while the request is being
  handled.

- Adding via cluster peers: before, when adding content to IPFS though a
  Cluster peer, it would chunk and send every individual chunk the cluster
  peers supposed to store the content, and then they would send it to IPFS
  individually, which resulted in a separate `block/put` request against the
  IPFS HTTP API. Files with a dozen chunks already showed that performance was
  not great. With streaming RPC, we can setup a single libp2p stream from the
  adding node to the destinations, and they can stream the blocks with a
  single `block/put` multipart-request directly into IPFS. We recommend using
  go-ipfs >= 0.12.0 for this.

These changes affect how cluster peers talk to each other and also how API
endpoints that responded with array collections behave (they now stream json
objects).

This release additionally includes the first version of the experimental
[IPFS Pinning Service API](https://ipfs.github.io/pinning-services-api-spec/)
for IPFS Cluster. This API runs along the existing HTTP REST API and IPFS
Proxy API and allows sending and querying pins from Cluster using standard
Pinning-service clients (works well with go-ipfs's `ipfs pin remote`). Note
that it does not support authentication nor tracking different requests for
the same CID (request ID is the CID).

The full list of additional features and bug fixes can be found below.

#### List of changes

##### Features

* restapi/adder: Add `?no-pin=true/false` option to `/add` endpoint | [ipfs-cluster/ipfs-cluster#1590](https://github.com/ipfs-cluster/ipfs-cluster/issues/1590)
* cluster: add `pin_only_on_trusted_peers` config option | [ipfs-cluster/ipfs-cluster#1585](https://github.com/ipfs-cluster/ipfs-cluster/issues/1585) | [ipfs-cluster/ipfs-cluster#1591](https://github.com/ipfs-cluster/ipfs-cluster/issues/1591)
* restapi/client: support querying status for multiple CIDs | [ipfs-cluster/ipfs-cluster#1564](https://github.com/ipfs-cluster/ipfs-cluster/issues/1564) | [ipfs-cluster/ipfs-cluster#1592](https://github.com/ipfs-cluster/ipfs-cluster/issues/1592)
* Pinning Services API | [ipfs-cluster/ipfs-cluster#1213](https://github.com/ipfs-cluster/ipfs-cluster/issues/1213) | [ipfs-cluster/ipfs-cluster#1483](https://github.com/ipfs-cluster/ipfs-cluster/issues/1483)
* restapi/adder: Return pin allocations on add output | [ipfs-cluster/ipfs-cluster#1598](https://github.com/ipfs-cluster/ipfs-cluster/issues/1598) | [ipfs-cluster/ipfs-cluster#1599](https://github.com/ipfs-cluster/ipfs-cluster/issues/1599)
* RPC Streaming | [ipfs-cluster/ipfs-cluster#1602](https://github.com/ipfs-cluster/ipfs-cluster/issues/1602) | [ipfs-cluster/ipfs-cluster#1607](https://github.com/ipfs-cluster/ipfs-cluster/issues/1607) | [ipfs-cluster/ipfs-cluster#1611](https://github.com/ipfs-cluster/ipfs-cluster/issues/1611) | [ipfs-cluster/ipfs-cluster#810](https://github.com/ipfs-cluster/ipfs-cluster/issues/810) | [ipfs-cluster/ipfs-cluster#1437](https://github.com/ipfs-cluster/ipfs-cluster/issues/1437) | [ipfs-cluster/ipfs-cluster#1616](https://github.com/ipfs-cluster/ipfs-cluster/issues/1616) | [ipfs-cluster/ipfs-cluster#1621](https://github.com/ipfs-cluster/ipfs-cluster/issues/1621) | [ipfs-cluster/ipfs-cluster#1631](https://github.com/ipfs-cluster/ipfs-cluster/issues/1631) | [ipfs-cluster/ipfs-cluster#1632](https://github.com/ipfs-cluster/ipfs-cluster/issues/1632)

##### Bug fixes

##### Other changes

* pubsubmon: Remove accrual failure detection | [ipfs-cluster/ipfs-cluster#939](https://github.com/ipfs-cluster/ipfs-cluster/issues/939) | [ipfs-cluster/ipfs-cluster#1586](https://github.com/ipfs-cluster/ipfs-cluster/issues/1586) | [ipfs-cluster/ipfs-cluster#1589](https://github.com/ipfs-cluster/ipfs-cluster/issues/1589)
* crdt: log with INFO when batches are committed | [ipfs-cluster/ipfs-cluster#1596](https://github.com/ipfs-cluster/ipfs-cluster/issues/1596)
* Dependency upgrades | [ipfs-cluster/ipfs-cluster#1613](https://github.com/ipfs-cluster/ipfs-cluster/issues/1613) | [ipfs-cluster/ipfs-cluster#1617](https://github.com/ipfs-cluster/ipfs-cluster/issues/1617) | [ipfs-cluster/ipfs-cluster#1627](https://github.com/ipfs-cluster/ipfs-cluster/issues/1627)
* Bump RPC protocol version | [ipfs-cluster/ipfs-cluster#1615](https://github.com/ipfs-cluster/ipfs-cluster/issues/1615)
* Replace cid.Cid with api.Cid wrapper type | [ipfs-cluster/ipfs-cluster#1626](https://github.com/ipfs-cluster/ipfs-cluster/issues/1626)
* Provide string JSON marshaling for PinType | [ipfs-cluster/ipfs-cluster#1628](https://github.com/ipfs-cluster/ipfs-cluster/issues/1628)
* ipfs-cluster-ctl should exit with status 1 when an argument error happens | [ipfs-cluster/ipfs-cluster#1633](https://github.com/ipfs-cluster/ipfs-cluster/issues/1633) | [ipfs-cluster/ipfs-cluster#1634](https://github.com/ipfs-cluster/ipfs-cluster/issues/1634)
* Revamp and fix basic exported metrics: pins, queued, pinning, pin errors | [ipfs-cluster/ipfs-cluster#1187](https://github.com/ipfs-cluster/ipfs-cluster/issues/1187) | [ipfs-cluster/ipfs-cluster#1470](https://github.com/ipfs-cluster/ipfs-cluster/issues/1470) | [ipfs-cluster/ipfs-cluster#1637](https://github.com/ipfs-cluster/ipfs-cluster/issues/1637)

#### Upgrading notices

As mentioned, all peers in the cluster should upgrade and things will heavily break otherwise.

##### Configuration changes

There are no breaking configuration changes. Other than that:

* A `pin_only_on_trusted_peers` boolean option that defaults to `false` has
  been added to the `cluster` configuration section. When enabled, only
  trusted peers will be considered when allocating pins.
* A new `pinsvcapi` section is now added to the `api` configuration section
  for newly-initialized configurations. When this section is present, the
  experimental Pinning Services API is launched. See the docs for the
  different options. Most of the code/options are similar to the `restapi`
  section as both share most of the code.

##### REST API

###### Streaming responses

The following endpoint responses have changed:

* `/allocations` returned a json array of api.Pin object and now it will stream them.
* `/pins` returned a json array of api.PinInfo objects and now it will stream them.
* `/recover` returned a json array of api.PinInfo objects and now it will stream them.

Failures on streaming endpoints are captured in request Trailer headers (same
as `/add`), in particular with a `X-Stream-Error` trailer. Note that the
`X-Stream-Error` trailer may appear even no error happened (empty value in
this case).

###### JSON-encoding of CIDs

As of v1.0.0, every "cid" as returned inside any REST API object will no
longer encode as:

```
{ "/" : "<cid>" }
```

but instead just as `"cid"`.

###### Add endpoint changes

There are two small backwards compatible changes to the `/add` endpoint:

* A `?no-pin` query option has been added. In this case, cluster will not pin
the content after having added it.
* The output objects returned when adding (i.e. the ones containing the CIDs
  of the files) now include an `Allocations` field, with an array of peer IDs
  corresponding to the peers on which the blocks were added.

###### Pin object changes

`Pin` objects (returned from `/allocations`, `POST /pins` etc). will not
encode the Type as a human-readable string and not as a number, as previously
happened.

###### PinInfo object changes

`PinInfo`/`GlobalPinInfo` objects (returned from `/pins` and `/recover` endpoitns), now
include additional fields (which before were only accessible via `/allocations`):

- `allocations`: an array of peer IDs indicating the pin allocations.
- `origins`: the list of origins associated to this pin.
- `metadata`: an object with pin metadata.
- `created`: date when the pin was added to the cluster.
- `ipfs_peer_id`: IPFS peer ID to which the object is pinned (when known).
- `ipfs_peer_addresses`: IPFS addresses of the IPFS daemon to which the object is pinned (when known).

##### Pinning Services API

This API now exists. It does not support Authentication and is experimental.

##### IPFS Proxy API

The `/add?pin=false` call will no longer trigger a cluster pin followed by an unpin.

The `/pin/ls?stream=true` query option is now supported.

##### Go APIs

There have been many changes to different interfaces (i.e. to stream out
collections over channels rather than return slices).

We have also taken the opportunity to get rid of pointers to objects in many
places. This was a bad step, which makes cluster perform many more allocations
that it should, and as a result causes more GC pressure. In any case, it was
not a good Go development practice to use referenced types all around for
objects that are not supposed to be mutated.

##### Other

The following metrics are now available in the Prometheus endpoint when enabled:

```
ipfscluster_pins
ipfscluster_pins_pin_queued
ipfscluster_pins_pin_error
ipfscluster_pins_pinning
```


---

### v0.14.5 - 2022-02-16

This is a minor IPFS Cluster release. The main feature is the upgrade of the
go-ds-crdt library which now supports resuming the processing of CRDT-DAGs
that were not fully synced.

On first start on an updated node, the CRDT library will have to re-walk the
full CRDT-DAG. This happens in the background.

For the full list of feature and bugfixes, see list below.

#### List of changes

##### Features

* CRDT: update with RepairInterval option and more workers | [ipfs-cluster/ipfs-cluster#1561](https://github.com/ipfs-cluster/ipfs-cluster/issues/1561) | [ipfs-cluster/ipfs-cluster#1576](https://github.com/ipfs-cluster/ipfs-cluster/issues/1576)
* Add `?cids` query parameter to /pins: limit status request to several CIDs | [ipfs-cluster/ipfs-cluster#1562](https://github.com/ipfs-cluster/ipfs-cluster/issues/1562)
* Pintracker improvements | [ipfs-cluster/ipfs-cluster#1556](https://github.com/ipfs-cluster/ipfs-cluster/issues/1556) | [ipfs-cluster/ipfs-cluster#1554](https://github.com/ipfs-cluster/ipfs-cluster/issues/1554) | [ipfs-cluster/ipfs-cluster#1212](https://github.com/ipfs-cluster/ipfs-cluster/issues/1212)
  * Status information shows peer ID of IPFS peer pinning the content
  * Peernames correctly set for remote peers on status objects
  * Pin names not set for in-flight pin status objects

##### Bug fixes

* Fix: logging was too noisy | [ipfs-cluster/ipfs-cluster#1581](https://github.com/ipfs-cluster/ipfs-cluster/issues/1581) | [ipfs-cluster/ipfs-cluster#1579](https://github.com/ipfs-cluster/ipfs-cluster/issues/1579)
* Remove warning message about informer metrics | [ipfs-cluster/ipfs-cluster#1543](https://github.com/ipfs-cluster/ipfs-cluster/issues/1543)
* Fix: IPFS repo/stat gets hammered on busy peers | [ipfs-cluster/ipfs-cluster#1559](https://github.com/ipfs-cluster/ipfs-cluster/issues/1559)
* Fix: faster shutdown by aborting state list on context cancellation | [ipfs-cluster/ipfs-cluster#1555](https://github.com/ipfs-cluster/ipfs-cluster/issues/1555)

##### Other changes

* Leave peername empty when unknown on status response | [ipfs-cluster/ipfs-cluster#1569](https://github.com/ipfs-cluster/ipfs-cluster/issues/1569) | [ipfs-cluster/ipfs-cluster#1575](https://github.com/ipfs-cluster/ipfs-cluster/issues/1575)
* Fix comment in graphs.go | [ipfs-cluster/ipfs-cluster#1570](https://github.com/ipfs-cluster/ipfs-cluster/issues/1570) | [ipfs-cluster/ipfs-cluster#1574](https://github.com/ipfs-cluster/ipfs-cluster/issues/1574)
* Make `/add?local=true` requests forcefully allocate to local peer | [ipfs-cluster/ipfs-cluster#1560](https://github.com/ipfs-cluster/ipfs-cluster/issues/1560)
* Dependency upgrades | [ipfs-cluster/ipfs-cluster#1580](https://github.com/ipfs-cluster/ipfs-cluster/issues/1580)

#### Upgrading notices

##### Configuration changes

Configuration is backwards compatible with previous versions.

The `consensus/crdt` section has a new option `repair_interval` which is set
by default to `1h` and controls how often we check if the crdt DAG needs to be
reprocessed (i.e. when it becomes marked dirty due to an error). Setting it to
`0` disables repairs.

The `ipfs_connector/ipfshttp` section has a new option
`informer_trigger_interval` which defaults to `0` (disabled). This controls
whether clusters issue a metrics update every certain number of pins (i.e. for
fine-grain control of freespace after a pin happens).

The `monitor/pubsubmon/failure_threshold` option no longer has any effect.

##### REST API

The `/pins` (StatusAll) endpoint now takes a `?cid=cid1,cid2` option which
allows to filter the resulting list to specific CIDs.

##### Go APIs

We added a `LatestForPeer()` method to the PeerMonitor interface which returns
the latest metric of a certain type received by a peer.

##### Other

Before, adding content using the `local=true` option would add the blocks to
the peer receiving the request and then allocate the pin normally (i.e. to the
peers with most free space available, which may or not be the local peer). Now,
"local add" requests will always allocate the pin to the local peer since it
already has the content.

Before, we would send a freespace metric update every 10 pins. After: we don't
do it anymore and relay on the normal metric interval, unless
`informer_trigger_interval` is configured.

The CRDT library will create a database of processed DAG blocks during the
first start on an upgraded node. This happens on the background and should
only happen once. Peers with very large CRDT-DAGs, may experience increased
disk usage during this time.

---


### v0.14.4 - 2022-01-11

This is a minor IPFS Cluster release with additional performance improvements.

On one side, we have improved branch pruning when syncing CRDT dags. This
should improve the time it takes for a peer to sync the pinset when joining a
high-activity cluster, where branching happens often.

On the other side, we have improved how Cluster finds and re-triggers pinning
operations for items that failed to pin previously, heavily reducing the
pressure on the IPFS daemon and speeding up the operation.


#### List of changes

##### Features

No new features.

##### Bug fixes

* Improved pruning on crdt-sync | [ipfs-cluster/ipfs-cluster#1541](https://github.com/ipfs-cluster/ipfs-cluster/issues/1541)
* Pintracker: avoid pin/ls for every item | [ipfs-cluster/ipfs-cluster#1538](https://github.com/ipfs-cluster/ipfs-cluster/issues/1538)
* Pintracker: set unexpectedly_unpinned status correctly | [ipfs-cluster/ipfs-cluster#1537](https://github.com/ipfs-cluster/ipfs-cluster/issues/1537)
* Tags informer: TTL should be default when not provided | [ipfs-cluster/ipfs-cluster#1519](https://github.com/ipfs-cluster/ipfs-cluster/issues/1519)

##### Other changes

* ipfs-cluster-service: buffered i/o on state import/export | [ipfs-cluster/ipfs-cluster#1517](https://github.com/ipfs-cluster/ipfs-cluster/issues/1517)
* Dependency upgrades, go-libp2p v0.17.0 | 1540

#### Upgrading notices

##### Configuration changes

No changes.

##### REST API

The `/pins/recover` (RecoverAll) endpoint now only returns items that have
been re-queued for pinning (because they were in error). Before, it returned
all items in the state (similar to the `/pins` endpoint, but at a huge perf
impact with large pinsets).

##### Go APIs

No changes.

##### Other

`ipfs-cluster-ctl recover` only returns items that have been re-queued (see
REST APIs above).

---

### v0.14.3 - 2022-01-03

This is a minor IPFS Cluster release with some performance improvements and
bug fixes.

First, we have improved the speed at which the pinset can be listed (around
3x). This is important for very large clusters with millions of items on the
pinset. Cluster peers regularly check on all items in the pinset (i.e. to
re-pin failed items or remove expired pins), so this means these operations
will consume less resources and complete faster.

Second, we have added additional options to the `state import` command to
provide more flexibility when migrating content to a new cluster. For example,
allocations and replication factors for all pins can be replaced on
import. One usecase is to convert a cluster with "replicate-everywhere" pins
into one cluster with pins allocated to a particular set of peers (as a prior
step to scaling up the cluster by adding more peers).

Among the bugs fixed, the worst was one causing errors when deserializing some
pins from their JSON representation. This happened when pins had the `Origins`
property set.


#### List of changes

##### Features

* State import: allow replication factor and allocations overwrite | [ipfs-cluster/ipfs-cluster#1508](https://github.com/ipfs-cluster/ipfs-cluster/issues/1508)

##### Bug fixes

* Fix state deserialization | [ipfs-cluster/ipfs-cluster#1507](https://github.com/ipfs-cluster/ipfs-cluster/issues/1507)
* Fix pintracker shutdown errors | [ipfs-cluster/ipfs-cluster#1510](https://github.com/ipfs-cluster/ipfs-cluster/issues/1510)
* API: CORS pre-flight (OPTIONS) requests should bypass authentication | [ipfs-cluster/ipfs-cluster#1512](https://github.com/ipfs-cluster/ipfs-cluster/issues/1512) | [ipfs-cluster/ipfs-cluster#1513](https://github.com/ipfs-cluster/ipfs-cluster/issues/1513) | [ipfs-cluster/ipfs-cluster#1514](https://github.com/ipfs-cluster/ipfs-cluster/issues/1514)
* Monitor: avoid sending invalid metrics | [ipfs-cluster/ipfs-cluster#1511](https://github.com/ipfs-cluster/ipfs-cluster/issues/1511)

##### Other changes

* Performance improvements to state list and logging for large states | [ipfs-cluster/ipfs-cluster#1510](https://github.com/ipfs-cluster/ipfs-cluster/issues/1510)

#### Upgrading notices

##### Configuration changes

No changes.

##### REST API

No changes.

##### Go APIs

No changes.

##### Other

`ipfs-cluster-service state import` has new `rmin`, `rmax` and `allocations`
flags. See `ipfs-cluster-service state import --help` for more information.

---

### v0.14.2 - 2021-12-09

This is a minor IPFS Cluster release focused on providing features for
production Cluster deployments with very high pin ingestion rates.

It addresses two important questions from our users:

  * How to ensure that my pins are automatically pinned on my cluster peers
  around the world in a balanced fashion.
  * How to ensure that items that cannot be pinned do not delay the pinning
  of items that are available.

We address the first of the questions by introducing an improved allocator and
user-defined "tag" metrics. Each cluster peer can now be tagged, and the
allocator can be configured to pin items in a way that they are distributed
among tags. For example, a cluster peer can tagged with `region: us,
availability-zone: us-west` and so on. Assuming a cluster made of 6 peers, 2
per region, and one per availability zone, the allocator would ensure that a
pin with replication factor = 3 lands in the 3 different regions and in the
availability zones with most available space of the two.

The second question is addressed by enriching pin metadata. Pins will now
store the time that they were added to the cluster. The pin tracker will
additionally keep track of how many times an operation has been retried. Using
these two items, we can prioritize pinning of items that are new and have not
repeatedly failed to pin. The max age and max number of retries used to
prioritize a pin can be controlled in the configuration.

Please see the information below for more details about how to make use and
configure these new features.

#### List of changes

##### Features

  * Tags informer and partition-based allocations | [ipfs-cluster/ipfs-cluster#159](https://github.com/ipfs-cluster/ipfs-cluster/issues/159) | [ipfs-cluster/ipfs-cluster#1468](https://github.com/ipfs-cluster/ipfs-cluster/issues/1468) | [ipfs-cluster/ipfs-cluster#1485](https://github.com/ipfs-cluster/ipfs-cluster/issues/1485)
  * Add timestamps to pin objects | [ipfs-cluster/ipfs-cluster#1484](https://github.com/ipfs-cluster/ipfs-cluster/issues/1484) | [ipfs-cluster/ipfs-cluster#989](https://github.com/ipfs-cluster/ipfs-cluster/issues/989)
  * Support priority pinning for recent pins with small number of retries | [ipfs-cluster/ipfs-cluster#1469](https://github.com/ipfs-cluster/ipfs-cluster/issues/1469) | [ipfs-cluster/ipfs-cluster#1490](https://github.com/ipfs-cluster/ipfs-cluster/issues/1490)

##### Bug fixes

  * Fix flaky adder test | [ipfs-cluster/ipfs-cluster#1461](https://github.com/ipfs-cluster/ipfs-cluster/issues/1461) | [ipfs-cluster/ipfs-cluster#1462](https://github.com/ipfs-cluster/ipfs-cluster/issues/1462)

##### Other changes

  * Refactor API to facilitate re-use of functionality | [ipfs-cluster/ipfs-cluster#1471](https://github.com/ipfs-cluster/ipfs-cluster/issues/1471)
  * Move testing to Github Actions | [ipfs-cluster/ipfs-cluster#1486](https://github.com/ipfs-cluster/ipfs-cluster/issues/1486)
  * Dependency upgrades (go-libp2p v0.16.0 etc.) | [ipfs-cluster/ipfs-cluster#1491](https://github.com/ipfs-cluster/ipfs-cluster/issues/1491) | [ipfs-cluster/ipfs-cluster#1501](https://github.com/ipfs-cluster/ipfs-cluster/issues/1501) | [ipfs-cluster/ipfs-cluster#1504](https://github.com/ipfs-cluster/ipfs-cluster/issues/1504)
  * Improve `health metrics <metric>` output in ipfs-cluster-ctl | [ipfs-cluster/ipfs-cluster#1506](https://github.com/ipfs-cluster/ipfs-cluster/issues/1506)

#### Upgrading notices

Despite of the new features, cluster peers should behave exactly as before
when using the previous configuration and should interact well with peers in
the previous version. However, for the new features to take full effect, all
peers should be upgraded to this release.

##### Configuration changes

The `pintracker/stateless` configuration sector gets 2 new options, which will take defaults when unset:

  * `priority_pin_max_age`, with a default of `24h`, and
  * `priority_pin_max_retries`, with a default of `5`.

A new informer type called "tags" now exists. By default, in has a subsection
in the `informer` configuration section with the following defaults:

```json
   "informer": {
     "disk": {...}
     },
     "tags": {
       "metric_ttl": "30s",
       "tags": {
         "group": "default"
       }
     }
   },
```

This enables the use of the "tags" informer. The `tags` configuration key in
it allows to add user-defined tags to this peer. For every tag, a new metric
will be broadcasted to other peers in the cluster carrying the tag
information. By default, peers would broadcast a metric of type "tag:group"
and value "default" (`ipfs-cluster-ctl health metrics` can be used to see what
metrics a cluster peer knows about). These tags metrics can be used to setup
advanced allocation strategies using the new "balanced" allocator described
below.

A new `allocator` top level section with a `balanced` configuration
sub-section can now be used to setup the new allocator. It has the following
default on new configurations:

```json
  "allocator": {
    "balanced": {
      "allocate_by": [
        "tag:group",
        "freespace"
      ]
    }
  },
```

When the allocator is NOT defined (legacy configurations), the `allocate_by`
option is only set to `["freespace"]`, to keep backwards compatibility (the
tags allocator with a "group:default" tag will not be present).

This asks the allocator to allocate pins first by the value of the "group"
tag-metric, as produced by the tag informer, and then by the value of the
"freespace" metric. Allocating solely by the "freespace" is the equivalent of
the cluster behavior on previous versions. This default assumes the default
`informer/tags` configuration section mentioned above is present.

##### REST API

The objects returned by the `/pins` endpoints ("GlobalPinInfo" types) now
include an additional `attempt_count` property, that counts how many times the
pin or unpin operation was retried, and a `priority_pin` boolean property,
that indicates whether the ongoing pin operation was last queued in the
priority queue or not.

The objects returned by the `/allocations` enpdpoints ("Pin" types) now
include an additional `timestamp` property.

The objects returned by the `/monitor/metrics/<metric>` endpoint now include a
`weight` property, which is used to sort metrics (before they were sorted by
parsing the value as decimal number).

The REST API client will now support QUIC for libp2p requests whenever not
using private networks.

##### Go APIs

There are no relevant changes other than the additional fields in the objects
as mentioned by the section right above.

##### Other

Nothing.

---


### v0.14.1 - 2021-08-16

This is an IPFS Cluster maintenance release addressing some issues and
bringing a couple of tweaks. The main fix is an issue that would prevent
cluster peers with very large pinsets (in the millions of objects) from fully
starting quickly.

This release is fully compatible with the previous release.

#### List of changes

##### Features

* Improve support for pre-0.14.0 peers | [ipfs-cluster/ipfs-cluster#1409](https://github.com/ipfs-cluster/ipfs-cluster/issues/1409) | [ipfs-cluster/ipfs-cluster#1446](https://github.com/ipfs-cluster/ipfs-cluster/issues/1446)
* Improve log-level handling | [ipfs-cluster/ipfs-cluster#1439](https://github.com/ipfs-cluster/ipfs-cluster/issues/1439)
* ctl: --wait returns as soon as replication-factor-min is reached | [ipfs-cluster/ipfs-cluster#1427](https://github.com/ipfs-cluster/ipfs-cluster/issues/1427) | [ipfs-cluster/ipfs-cluster#1444](https://github.com/ipfs-cluster/ipfs-cluster/issues/1444)

##### Bug fixes

* Fix some data races in tests | [ipfs-cluster/ipfs-cluster#1428](https://github.com/ipfs-cluster/ipfs-cluster/issues/1428)
* Do not block peer startup while waiting for RecoverAll | [ipfs-cluster/ipfs-cluster#1436](https://github.com/ipfs-cluster/ipfs-cluster/issues/1436) | [ipfs-cluster/ipfs-cluster#1438](https://github.com/ipfs-cluster/ipfs-cluster/issues/1438)
* Use HTTP 307-redirects on restapi paths ending with "/" | [ipfs-cluster/ipfs-cluster#1415](https://github.com/ipfs-cluster/ipfs-cluster/issues/1415) | [ipfs-cluster/ipfs-cluster#1445](https://github.com/ipfs-cluster/ipfs-cluster/issues/1445)

##### Other changes

* Dependency upgrades | [ipfs-cluster/ipfs-cluster#1451](https://github.com/ipfs-cluster/ipfs-cluster/issues/1451)

#### Upgrading notices

##### Configuration changes

No changes. Configurations are fully backwards compatible.

##### REST API

Paths ending with a `/` (slash) were being automatically redirected to the
path without the slash using a 301 code (permanent redirect). However, most
clients do not respect the method name when following 301-redirects, thus a
POST request to `/allocations/` would become a GET request to `/allocations`.

We have now set these redirects to use 307 instead (temporary
redirect). Clients do keep the HTTP method when following 307 redirects.

##### Go APIs

The parameters object to the RestAPI client `WaitFor` function now has a
`Limit` field. This allows to return as soon as a number of peers have reached
the target status. When unset, previous behavior should be maintained.

##### Other

Per the `WaitFor` modification above, `ipfs-cluster-ctl` now sets the limit to
the replication-factor-min value on pin/add commands when using the `--wait`
flag. These will potentially return earlier.

---

### v0.14.0 - 2021-07-09

This IPFS Cluster release brings a few features to improve cluster operations
at scale (pinsets over 100k items), along with some bug fixes.

This release is not fully compatible with previous ones. Nodes on different
versions will be unable to parse metrics from each other (thus `peers ls`
will not report peers on different versions) and the StatusAll RPC method
(a.k.a `ipfs-cluster-ctl status` or `/pins` API endpoint) will not work. Hence
the minor version bump. **Please upgrade all of your cluster peers**.

This release brings a few key improvements to the cluster state storage:
badger will automatically perform garbage collection on regular intervals,
resolving a long standing issue of badger using up to 100x the actual needed
space. Badger GC will automatically be enabled with defaults, which will
result in increased disk I/O if there is a lot to GC 15 minutes after starting
the peer. **Make sure to disable GC manually if increased disk I/O during GC
may affect your service upon upgrade**. In our tests the impact was soft
enough to consider this a safe default, though in environments with very
constrained disk I/O it will be surely noticed, at least in the first GC
cycle, since the datastore was never GC'ed before.

Badger is the datastore we are more familiar with and the most scalable choice
(chosen by both IPFS and Filecoin). However, it may be that badger behavior
and GC-needs are not best suited or not preferred, or more downsides are
discovered in the future. For those cases, we have added the option to run
with a leveldb backend as an alternative. Level DB does not need GC and it
will auto-compact. It should also scale pretty well for most cases, though we
have not tested or compared against badger with very large pinsets. The
backend can be configured during the daemon `init`, along with the consensus
component using a new `--datastore` flag. Like the default Badger backend, the
new LevelDB backend exposes all LevelDB internal configuration options.

Additionally, operators handling very large clusters may have noticed that
checking status of pinning,queued items (`ipfs-cluster-ctl status --filter
pinning,queued`) took very long as it listed and iterated on the full ipfs
pinset. We have added some fixes so that we save the time when filtering for
items that do not require listing the full state.

Finally, cluster pins now have an `origins` option, which allows submitters to
provide hints for providers of the content. Cluster will instruct IPFS to
connect to the `origins` of a pin before pinning. Note that for the moment
[ipfs will keep connected to those peers permanently](https://github.com/ipfs-cluster/ipfs-cluster/issues/1376).

Please read carefully through the notes below, as the release includes subtle
changes in configuration, defaults and behaviors which may in some cases
affect you (although probably will not).

#### List of changes

##### Features

* Set disable_repinning to true by default, for new configurations | [ipfs-cluster/ipfs-cluster#1398](https://github.com/ipfs-cluster/ipfs-cluster/issues/1398)
* Efficient status queries with filters | [ipfs-cluster/ipfs-cluster#1360](https://github.com/ipfs-cluster/ipfs-cluster/issues/1360) | [ipfs-cluster/ipfs-cluster#1377](https://github.com/ipfs-cluster/ipfs-cluster/issues/1377) | [ipfs-cluster/ipfs-cluster#1399](https://github.com/ipfs-cluster/ipfs-cluster/issues/1399)
* User-provided pin "origins" | [ipfs-cluster/ipfs-cluster#1374](https://github.com/ipfs-cluster/ipfs-cluster/issues/1374) | [ipfs-cluster/ipfs-cluster#1375](https://github.com/ipfs-cluster/ipfs-cluster/issues/1375)
* Provide darwin/arm64 binaries (Apple M1). Needs testing! | [ipfs-cluster/ipfs-cluster#1369](https://github.com/ipfs-cluster/ipfs-cluster/issues/1369)
* Set the "size" field in the response when adding CARs when the archive contains a single unixfs file | [ipfs-cluster/ipfs-cluster#1362](https://github.com/ipfs-cluster/ipfs-cluster/issues/1362) | [ipfs-cluster/ipfs-cluster#1372](https://github.com/ipfs-cluster/ipfs-cluster/issues/1372)
* Support a leveldb-datastore backend | [ipfs-cluster/ipfs-cluster#1364](https://github.com/ipfs-cluster/ipfs-cluster/issues/1364) | [ipfs-cluster/ipfs-cluster#1373](https://github.com/ipfs-cluster/ipfs-cluster/issues/1373)
* Speed up pin/ls by not filtering when not needed | [ipfs-cluster/ipfs-cluster#1405](https://github.com/ipfs-cluster/ipfs-cluster/issues/1405)

##### Bug fixes

* Badger datastore takes too much size | [ipfs-cluster/ipfs-cluster#1320](https://github.com/ipfs-cluster/ipfs-cluster/issues/1320) | [ipfs-cluster/ipfs-cluster#1370](https://github.com/ipfs-cluster/ipfs-cluster/issues/1370)
* Fix: error-type responses from the IPFS proxy not understood by ipfs | [ipfs-cluster/ipfs-cluster#1366](https://github.com/ipfs-cluster/ipfs-cluster/issues/1366) | [ipfs-cluster/ipfs-cluster#1371](https://github.com/ipfs-cluster/ipfs-cluster/issues/1371)
* Fix: adding with cid-version=1 does not automagically set raw-leaves | [ipfs-cluster/ipfs-cluster#1358](https://github.com/ipfs-cluster/ipfs-cluster/issues/1358) | [ipfs-cluster/ipfs-cluster#1359](https://github.com/ipfs-cluster/ipfs-cluster/issues/1359)
* Tests: close datastore on test node shutdown | [ipfs-cluster/ipfs-cluster#1389](https://github.com/ipfs-cluster/ipfs-cluster/issues/1389)
* Fix ipfs-cluster-ctl not using dns name when talking to remote https endpoints | [ipfs-cluster/ipfs-cluster#1403](https://github.com/ipfs-cluster/ipfs-cluster/issues/1403) | [ipfs-cluster/ipfs-cluster#1404](https://github.com/ipfs-cluster/ipfs-cluster/issues/1404)


##### Other changes

* Dependency upgrades | [ipfs-cluster/ipfs-cluster#1378](https://github.com/ipfs-cluster/ipfs-cluster/issues/1378) | [ipfs-cluster/ipfs-cluster#1395](https://github.com/ipfs-cluster/ipfs-cluster/issues/1395)
* Update compose to use the latest go-ipfs | [ipfs-cluster/ipfs-cluster#1363](https://github.com/ipfs-cluster/ipfs-cluster/issues/1363)
* Update IRC links to point to new Matrix channel | [ipfs-cluster/ipfs-cluster#1361](https://github.com/ipfs-cluster/ipfs-cluster/issues/1361)

#### Upgrading notices

##### Configuration changes

Configurations are fully backwards compatible.

The `cluster.disable_repinning` setting now defaults to true on new generated configurations.

The `datastore.badger` section now includes settings to control (and disable) automatic GC:

```json
   "badger": {
      "gc_discard_ratio": 0.2,
      "gc_interval": "15m0s",
      "gc_sleep": "10s",
	  ...
   }
```

**When not present, these settings take their defaults**, so GC will
automatically be enabled on nodes that upgrade keeping their previous
configurations.

GC can be disabled by setting `gc_interval` to `"0s"`. A GC cycle is made by
multiple GC rounds. Setting `gc_sleep` to `"0s"` will result in a single GC
round.

Finally, nodes initializing with `--datastore leveldb` will obtain a
`datastore.leveldb` section (instead of a `badger` one). Configurations can
only include one datastore section, either `badger` or `leveldb`. Currently we
offer no way to convert states between the two datastore backends.

##### REST API

Pin options (`POST /add` and `POST /pins` endpoints) now take an `origins`
query parameter as an additional pin option. It can be set to a
comma-separated list of full peer multiaddresses to which IPFS can connect to
fetch the content. Only the first 10 multiaddresses will be taken into
account.

The response of `POST /add?format=car` endpoint when adding a CAR file (a single
pin progress object) always had the "size" field set to 0. This is now set to
the unixfs FileSize property, when the root of added CAR correspond to a
unixfs node of type File. In any other case, it stays at 0.

The `GET /pins` endpoint reports pin status for all pins in the pinset by
default and optionally takes a `filter` query param. Before, it would include
a full GlobalPinInfo object for a pin as long as the status of the CID in one
of the peers matched the filter, so the object could include statuses for
other cluster peers for that CID which did not match the filter. Starting on
this version, the returned statuses will be fully limited to those of the
peers matching the filter.

On the same endpoint, a new `unexpectedly_unpinned` pin status has been
added, which can also be used as a filter. Previously, pins in this state were
reported as `pin_error`. Note the `error` filter does not match
`unexpectedly_unpinned` status as it did before, which should be queried
directly (or without any filter).

##### Go APIs

The PinTracker interface has been updated so that the `StatusAll` method takes
a TrackerStatus filter. The stateless pintracker implementation has been
updated accordingly.

##### Other

Docker containers now support `IPFS_CLUSTER_DATASTORE` to set the datastore
type during initialization (similar to `IPFS_CLUSTER_CONSENSUS`).

Due to the deprecation of the multicodecs repository, we no longer serialize
metrics by prepending the msgpack multicodec code to the bytes and instead
encode the metrics directly. This means older peers will not know how to
deserialize metrics from newer peers, and vice-versa. While peers will keep
working (particularly follower peers will keep tracking content etc), peers
will not include other peers with different versions in their "peerset and
many operations that rely on this will not work as intended or show partial
views.

---

### v0.13.3 - 2021-05-14

IPFS Cluster v0.13.3 brings two new features: CAR file imports and crdt-commit batching.

The first one allows to upload CAR files directly to the Cluster using the
existing Add endpoint with a new option set: `/add?format=car`. The endpoint
remains fully backwards compatible. CAR files are a simple wrapper around a
collection of IPFS blocks making up a DAG. Thus, this enables arbitrary DAG
imports directly through the Cluster REST API, taking advantange of the rest
of its features like basic-auth access control, libp2p endpoint and multipeer
block-put when adding.

The second feature unlocks large escalability improvements for pin ingestion
with the crdt "consensus" component. By default, each pin or unpin requests
results in an insertion to the crdt-datastore-DAG that maintains and syncs the
state between nodes, creating a new root. Batching allows to group multiple
updates in a single crdt DAG-node. This reduces the number of broadcasts, the
depth of the DAG, the breadth of the DAG and the syncing times when the
Cluster is ingesting many pins, removing most of the overhead in the
process. The batches are automatically committed when reaching a certain age or
a certain size, both configurable.

Additionally, improvements to timeout behaviors have been introduced.

For more details, check the list below and the latest documentation on the
[website](https://ipfscluster.io).

#### List of changes

##### Features

* Support adding CAR files | [ipfs-cluster/ipfs-cluster#1343](https://github.com/ipfs-cluster/ipfs-cluster/issues/1343)
* CRDT batching support | [ipfs-cluster/ipfs-cluster#1008](https://github.com/ipfs-cluster/ipfs-cluster/issues/1008) | [ipfs-cluster/ipfs-cluster#1346](https://github.com/ipfs-cluster/ipfs-cluster/issues/1346) | [ipfs-cluster/ipfs-cluster#1356](https://github.com/ipfs-cluster/ipfs-cluster/issues/1356)

##### Bug fixes

* Improve timeouts and timeout faster when dialing | [ipfs-cluster/ipfs-cluster#1350](https://github.com/ipfs-cluster/ipfs-cluster/issues/1350) | [ipfs-cluster/ipfs-cluster#1351](https://github.com/ipfs-cluster/ipfs-cluster/issues/1351)

##### Other changes

* Dependency upgrades | [ipfs-cluster/ipfs-cluster#1357](https://github.com/ipfs-cluster/ipfs-cluster/issues/1357)

#### Upgrading notices

##### Configuration changes

The `crdt` section of the configuration now has a `batching` subsection which controls batching settings:

```json
"batching": {
    "max_batch_size": 0,
    "max_batch_age": "0s"
}
```

An additional, hidden `max_queue_size` option exists, with default to
`50000`. The meanings of the options are documented on the reference (website)
and the code.

Batching is disabled by default. To be enabled, both `max_batch_size` and
`max_batch_age` need to be set to positive values.

The `cluster` section of the configuration has a new `dial_peer_timeout`
option, which defaults to "3s". It controls the default dial timeout when
libp2p is attempting to open a connection to a peer.

##### REST API

The `/add` endpoint now understands a new query parameter `?format=`, which
can be set to `unixfs` (default), or `car` (when uploading a CAR file). CAR
files should have a single root. Additional parts in multipart uploads for CAR
files are ignored.

##### Go APIs

The `AddParams` object that controls API options for the Add endpoint has been
updated with the new `Format` option.

##### Other

Nothing.



---

### v0.13.2 - 2021-04-06

IPFS Cluster v0.13.2 is a maintenance release addressing bugs and adding a
couple of small features. It is fully compatible with the previous release.

#### List of changes

##### Features

* Make mDNS failures non-fatal | [ipfs-cluster/ipfs-cluster#1193](https://github.com/ipfs-cluster/ipfs-cluster/issues/1193) | [ipfs-cluster/ipfs-cluster#1310](https://github.com/ipfs-cluster/ipfs-cluster/issues/1310)
* Add `--wait` flag to `ipfs-cluster-ctl add` command | [ipfs-cluster/ipfs-cluster#1285](https://github.com/ipfs-cluster/ipfs-cluster/issues/1285) | [ipfs-cluster/ipfs-cluster#1301](https://github.com/ipfs-cluster/ipfs-cluster/issues/1301)

##### Bug fixes

* Stop using secio in REST API libp2p server and client | [ipfs-cluster/ipfs-cluster#1315](https://github.com/ipfs-cluster/ipfs-cluster/issues/1315) | [ipfs-cluster/ipfs-cluster#1316](https://github.com/ipfs-cluster/ipfs-cluster/issues/1316)
* CID status wrongly reported as REMOTE | [ipfs-cluster/ipfs-cluster#1319](https://github.com/ipfs-cluster/ipfs-cluster/issues/1319) | [ipfs-cluster/ipfs-cluster#1331](https://github.com/ipfs-cluster/ipfs-cluster/issues/1331)


##### Other changes

* Dependency upgrades | [ipfs-cluster/ipfs-cluster#1335](https://github.com/ipfs-cluster/ipfs-cluster/issues/1335)
* Use cid.Cid as map keys in Pintracker | [ipfs-cluster/ipfs-cluster#1322](https://github.com/ipfs-cluster/ipfs-cluster/issues/1322)

#### Upgrading notices

##### Configuration changes

No configuration changes in this release.

##### REST API

The REST API server and clients will no longer negotiate the secio
security. This transport was already the lowest priority one and should have
not been used. This however, may break 3rd party clients which only supported
secio.


##### Go APIs

Nothing.

##### Other

Nothing.

---

### v0.13.1 - 2021-01-14

IPFS Cluster v0.13.1 is a maintenance release with some bugfixes and updated
dependencies. It should be fully backwards compatible.

This release deprecates `secio` (as required by libp2p), but this was already
the lowest priority security transport and `tls` would have been used by default.
The new `noise` transport becomes the preferred option.

#### List of changes

##### Features

* Support for multiple architectures added to the Docker container | [ipfs-cluster/ipfs-cluster#1085](https://github.com/ipfs-cluster/ipfs-cluster/issues/1085) | [ipfs-cluster/ipfs-cluster#1196](https://github.com/ipfs-cluster/ipfs-cluster/issues/1196)
* Add `--name` and `--expire` to `ipfs-cluster-ctl pin update` | [ipfs-cluster/ipfs-cluster#1184](https://github.com/ipfs-cluster/ipfs-cluster/issues/1184) | [ipfs-cluster/ipfs-cluster#1195](https://github.com/ipfs-cluster/ipfs-cluster/issues/1195)
* Failover client integrated in `ipfs-cluster-ctl` | [ipfs-cluster/ipfs-cluster#1222](https://github.com/ipfs-cluster/ipfs-cluster/issues/1222) | [ipfs-cluster/ipfs-cluster#1250](https://github.com/ipfs-cluster/ipfs-cluster/issues/1250)
* `ipfs-cluster-ctl health alerts` lists the last expired metrics seen by the peer | [ipfs-cluster/ipfs-cluster#165](https://github.com/ipfs-cluster/ipfs-cluster/issues/165) | [ipfs-cluster/ipfs-cluster#978](https://github.com/ipfs-cluster/ipfs-cluster/issues/978)

##### Bug fixes

* IPFS Proxy: pin progress objects wrongly includes non empty `Hash` key | [ipfs-cluster/ipfs-cluster#1286](https://github.com/ipfs-cluster/ipfs-cluster/issues/1286) | [ipfs-cluster/ipfs-cluster#1287](https://github.com/ipfs-cluster/ipfs-cluster/issues/1287)
* CRDT: Fix pubsub peer validation check | [ipfs-cluster/ipfs-cluster#1288](https://github.com/ipfs-cluster/ipfs-cluster/issues/1288)

##### Other changes

* Typos | [ipfs-cluster/ipfs-cluster#1181](https://github.com/ipfs-cluster/ipfs-cluster/issues/1181) | [ipfs-cluster/ipfs-cluster#1183](https://github.com/ipfs-cluster/ipfs-cluster/issues/1183)
* Reduce default pin_timeout to 2 minutes | [ipfs-cluster/ipfs-cluster#1160](https://github.com/ipfs-cluster/ipfs-cluster/issues/1160)
* Dependency upgrades | [ipfs-cluster/ipfs-cluster#1125](https://github.com/ipfs-cluster/ipfs-cluster/issues/1125) | [ipfs-cluster/ipfs-cluster#1238](https://github.com/ipfs-cluster/ipfs-cluster/issues/1238)
* Remove `secio` security transport | [ipfs-cluster/ipfs-cluster#1214](https://github.com/ipfs-cluster/ipfs-cluster/issues/1214) | [ipfs-cluster/ipfs-cluster#1227](https://github.com/ipfs-cluster/ipfs-cluster/issues/1227)

#### Upgrading notices

##### Configuration changes

The new default for `ipfs_http.pin_timeout` is `2m`. This is the time that
needs to pass for a pin operation to error and it starts counting from the
last block pinned.

##### REST API

A new `/health/alerts` endpoint exists to support `ipfs-cluster-ctl health alerts`.

##### Go APIs

The definition of `types.Alert` has changed. This type was not exposed to the
outside before. RPC endpoints affected are only used locally.

##### Other

Nothing.

---

### v0.13.0 - 2020-05-19

IPFS Cluster v0.13.0 provides many improvements and bugfixes on multiple fronts.

First, this release takes advantange of all the major features that have
landed in libp2p and IPFS lands (via ipfs-lite) during the last few months,
including the dual-DHT and faster block exchange with Bitswap. On the
downside, **QUIC support for private networks has been temporally dropped**,
which means we cannot use the transport for Cluster peers anymore. We have disabled
QUIC for the time being until private network support is re-added.

Secondly, `go-ds-crdt` has received major improvements since the last version,
resolving some bugs and increasing performance. Because of this, **cluster
peers in CRDT mode running older versions will be unable to process updates
sent by peers running the newer versions**. This means, for example, that
followers on v0.12.1 and earlier will be unable to receive updates from
trusted peers on v0.13.0 and later. However, peers running v0.13.0 will still
understand updates sent from older peers.

Finally, we have resolved some bugs and added a few very useful features,
which are detailed in the list below. We recommend everyone to upgrade as soon
as possible for a swifter experience with IPFS Cluster.

#### List of changes

##### Features

* Support multiple listen interfaces | [ipfs-cluster/ipfs-cluster#1000](https://github.com/ipfs-cluster/ipfs-cluster/issues/1000) | [ipfs-cluster/ipfs-cluster#1010](https://github.com/ipfs-cluster/ipfs-cluster/issues/1010) | [ipfs-cluster/ipfs-cluster#1002](https://github.com/ipfs-cluster/ipfs-cluster/issues/1002)
* Show expiration information in `ipfs-cluster-ctl pin ls` | [ipfs-cluster/ipfs-cluster#998](https://github.com/ipfs-cluster/ipfs-cluster/issues/998) | [ipfs-cluster/ipfs-cluster#1024](https://github.com/ipfs-cluster/ipfs-cluster/issues/1024) | [ipfs-cluster/ipfs-cluster#1066](https://github.com/ipfs-cluster/ipfs-cluster/issues/1066)
* Show pin names in `ipfs-cluster-ctl status` (and API endpoint) | [ipfs-cluster/ipfs-cluster#1129](https://github.com/ipfs-cluster/ipfs-cluster/issues/1129)
* Allow updating expiration when doing `pin update` | [ipfs-cluster/ipfs-cluster#996](https://github.com/ipfs-cluster/ipfs-cluster/issues/996) | [ipfs-cluster/ipfs-cluster#1065](https://github.com/ipfs-cluster/ipfs-cluster/issues/1065) | [ipfs-cluster/ipfs-cluster#1013](https://github.com/ipfs-cluster/ipfs-cluster/issues/1013)
* Add "direct" pin mode. Cluster supports direct pins | [ipfs-cluster/ipfs-cluster#1009](https://github.com/ipfs-cluster/ipfs-cluster/issues/1009) | [ipfs-cluster/ipfs-cluster#1083](https://github.com/ipfs-cluster/ipfs-cluster/issues/1083)
* Better badger defaults for less memory usage | [ipfs-cluster/ipfs-cluster#1027](https://github.com/ipfs-cluster/ipfs-cluster/issues/1027)
* Print configuration (without sensitive values) when enabling debug for `ipfs-cluster-service` | [ipfs-cluster/ipfs-cluster#937](https://github.com/ipfs-cluster/ipfs-cluster/issues/937) | [ipfs-cluster/ipfs-cluster#959](https://github.com/ipfs-cluster/ipfs-cluster/issues/959)
* `ipfs-cluster-follow <cluster> list` works fully offline (without needing IPFS to run) | [ipfs-cluster/ipfs-cluster#1129](https://github.com/ipfs-cluster/ipfs-cluster/issues/1129)

##### Bug fixes

* Fix adding when using CidV1 | [ipfs-cluster/ipfs-cluster#1016](https://github.com/ipfs-cluster/ipfs-cluster/issues/1016) | [ipfs-cluster/ipfs-cluster#1006](https://github.com/ipfs-cluster/ipfs-cluster/issues/1006)
* Fix too many requests error on `ipfs-cluster-follow <cluster> list` | [ipfs-cluster/ipfs-cluster#1013](https://github.com/ipfs-cluster/ipfs-cluster/issues/1013) | [ipfs-cluster/ipfs-cluster#1129](https://github.com/ipfs-cluster/ipfs-cluster/issues/1129)
* Fix repinning not working reliably on collaborative clusters with replication factors set | [ipfs-cluster/ipfs-cluster#1064](https://github.com/ipfs-cluster/ipfs-cluster/issues/1064) | [ipfs-cluster/ipfs-cluster#1127](https://github.com/ipfs-cluster/ipfs-cluster/issues/1127)
* Fix underflow in repo size metric | [ipfs-cluster/ipfs-cluster#1120](https://github.com/ipfs-cluster/ipfs-cluster/issues/1120) | [ipfs-cluster/ipfs-cluster#1121](https://github.com/ipfs-cluster/ipfs-cluster/issues/1121)
* Fix adding keeps going if all BlockPut failed | [ipfs-cluster/ipfs-cluster#1131](https://github.com/ipfs-cluster/ipfs-cluster/issues/1131)

##### Other changes

* Update license files | [ipfs-cluster/ipfs-cluster#1014](https://github.com/ipfs-cluster/ipfs-cluster/issues/1014)
* Fix typos | [ipfs-cluster/ipfs-cluster#999](https://github.com/ipfs-cluster/ipfs-cluster/issues/999) | [ipfs-cluster/ipfs-cluster#1001](https://github.com/ipfs-cluster/ipfs-cluster/issues/1001) | [ipfs-cluster/ipfs-cluster#1075](https://github.com/ipfs-cluster/ipfs-cluster/issues/1075)
* Lots of dependency upgrades | [ipfs-cluster/ipfs-cluster#1020](https://github.com/ipfs-cluster/ipfs-cluster/issues/1020) | [ipfs-cluster/ipfs-cluster#1051](https://github.com/ipfs-cluster/ipfs-cluster/issues/1051) | [ipfs-cluster/ipfs-cluster#1073](https://github.com/ipfs-cluster/ipfs-cluster/issues/1073) | [ipfs-cluster/ipfs-cluster#1074](https://github.com/ipfs-cluster/ipfs-cluster/issues/1074)
* Adjust codecov thresholds | [ipfs-cluster/ipfs-cluster#1022](https://github.com/ipfs-cluster/ipfs-cluster/issues/1022)
* Fix all staticcheck warnings | [ipfs-cluster/ipfs-cluster#1071](https://github.com/ipfs-cluster/ipfs-cluster/issues/1071) | [ipfs-cluster/ipfs-cluster#1128](https://github.com/ipfs-cluster/ipfs-cluster/issues/1128)
* Detach RPC protocol version from Cluster releases | [ipfs-cluster/ipfs-cluster#1093](https://github.com/ipfs-cluster/ipfs-cluster/issues/1093)
* Trim paths on Makefile build command | [ipfs-cluster/ipfs-cluster#1012](https://github.com/ipfs-cluster/ipfs-cluster/issues/1012) | [ipfs-cluster/ipfs-cluster#1015](https://github.com/ipfs-cluster/ipfs-cluster/issues/1015)
* Add contexts to HTTP requests in the client | [ipfs-cluster/ipfs-cluster#1019](https://github.com/ipfs-cluster/ipfs-cluster/issues/1019)


#### Upgrading notices

##### Configuration changes

* The default options in the `datastore/badger/badger_options` have changed
  and should reduce memory usage significantly:
  * `truncate` is set to `true`.
  * `value_log_loading_mode` is set to `0` (FileIO).
  * `max_table_size` is set to `16777216`.
* `api/ipfsproxy/listen_multiaddress`, `api/rest/http_listen_multiaddress` and
  `api/rest/libp2p_listen_multiaddress` now support an array of multiaddresses
  rather than a single one (a single one still works). This allows, for
  example, listening on both IPv6 and IPv4 interfaces.

##### REST API

The `POST /pins/{hash}` endpoint (`pin add`) now supports a `mode` query
parameter than can be set to `recursive` or `direct`. The responses including
Pin objects (`GET /allocations`, `pin ls`) include a `mode` field set
accordingly.

The IPFS proxy `/pin/add` endpoint now supports `recursive=false` for direct pins.

The `/pins` endpoint now return `GlobalPinInfo` objects that include a `name`
field for the pin name. The same objects do not embed redundant information
anymore for each peer in the `peer_map`: `cid` and `peer` are omitted.

##### Go APIs

The `ipfscluster.IPFSConnector` component signature for `PinLsCid` has changed
and receives a full `api.Pin` object, rather than a Cid. The RPC endpoint has
changed accordingly, but since this is a private endpoint, it does not affect
interoperability between peers.

The `api.GlobalPinInfo` type now maps every peer to a new `api.PinInfoShort`
type, that does not include any redundant information (Cid, Peer), as the
`PinInfo` type did. The `Cid` is available as a top-level field. The `Peer`
corresponds to the map key. A new `Name` top-level field contains the Pin
Name.

The `api.PinInfo` file includes also a new `Name` field.

##### Other

From this release, IPFS Cluster peers running in different minor versions will
remain compatible at the RPC layer (before, all cluster peers had to be
running on precisely the same minor version to be able to communicate). This
means that v0.13.0 peers are still compatible with v0.12.x peers (with the
caveat for CRDT-peers mentioned at the top). `ipfs-cluster-ctl --enc=json id`
shows information about the RPC protocol used.

Since the QUIC libp2p transport does not support private networks at this
point, it has been disabled, even though we keep the QUIC endpoint among the
default listeners.

---

### v0.12.1 - 2019-12-24

IPFS Cluster v0.12.1 is a maintenance release fixing issues on `ipfs-cluster-follow`.

#### List of changes

##### Bug fixes

* follow: the `info` command panics when ipfs is offline | [ipfs-cluster/ipfs-cluster#991](https://github.com/ipfs-cluster/ipfs-cluster/issues/991) | [ipfs-cluster/ipfs-cluster#993](https://github.com/ipfs-cluster/ipfs-cluster/issues/993)
* follow: the gateway url is not set on Run&Init command | [ipfs-cluster/ipfs-cluster#992](https://github.com/ipfs-cluster/ipfs-cluster/issues/992) | [ipfs-cluster/ipfs-cluster#993](https://github.com/ipfs-cluster/ipfs-cluster/issues/993)
* follow: disallow trusted peers for RepoGCLocal operation | [ipfs-cluster/ipfs-cluster#993](https://github.com/ipfs-cluster/ipfs-cluster/issues/993)

---

### v0.12.0 - 2019-12-20

IPFS Cluster v0.12.0 brings many useful features and makes it very easy to
create and participate on collaborative clusters.

The new `ipfs-cluster-follow` command provides a very simple way of joining
one or several clusters as a follower (a peer without permissions to pin/unpin
anything). `ipfs-cluster-follow` peers are initialize using a configuration
"template" distributed over IPFS or HTTP, which is then optimized and secured.

`ipfs-cluster-follow` is limited in scope and attempts to be very
straightforward to use. `ipfs-cluster-service` continues to offer power users
the full set of options to running peers of all kinds (followers or not).

We have additionally added many new features: pin with an expiration date, the
ability to trigger garbage collection on IPFS daemons, improvements on
NAT-traversal and connectivity etc.

Users planning to setup public collaborative clusters should upgrade to this
release, which improves the user experience and comes with documentation on
how to setup and join these clusters
(https://ipfscluster.io/documentation/collaborative).


#### List of changes

##### Features

* cluster: `--local` flag for add: adds only to the local peer instead of multiple destinations | [ipfs-cluster/ipfs-cluster#848](https://github.com/ipfs-cluster/ipfs-cluster/issues/848) | [ipfs-cluster/ipfs-cluster#907](https://github.com/ipfs-cluster/ipfs-cluster/issues/907)
* cluster: `RecoverAll` operation can trigger recover operation in all peers.
* ipfsproxy: log HTTP requests | [ipfs-cluster/ipfs-cluster#574](https://github.com/ipfs-cluster/ipfs-cluster/issues/574) | [ipfs-cluster/ipfs-cluster#915](https://github.com/ipfs-cluster/ipfs-cluster/issues/915)
* api: `health/metrics` returns list of available metrics | [ipfs-cluster/ipfs-cluster#374](https://github.com/ipfs-cluster/ipfs-cluster/issues/374) | [ipfs-cluster/ipfs-cluster#924](https://github.com/ipfs-cluster/ipfs-cluster/issues/924)
* service: `init --randomports` sets random, unused ports on initialization | [ipfs-cluster/ipfs-cluster#794](https://github.com/ipfs-cluster/ipfs-cluster/issues/794) | [ipfs-cluster/ipfs-cluster#926](https://github.com/ipfs-cluster/ipfs-cluster/issues/926)
* cluster: support pin expiration | [ipfs-cluster/ipfs-cluster#481](https://github.com/ipfs-cluster/ipfs-cluster/issues/481) | [ipfs-cluster/ipfs-cluster#923](https://github.com/ipfs-cluster/ipfs-cluster/issues/923)
* cluster: quic, autorelay, autonat, TLS handshake support | [ipfs-cluster/ipfs-cluster#614](https://github.com/ipfs-cluster/ipfs-cluster/issues/614) | [ipfs-cluster/ipfs-cluster#932](https://github.com/ipfs-cluster/ipfs-cluster/issues/932) | [ipfs-cluster/ipfs-cluster#973](https://github.com/ipfs-cluster/ipfs-cluster/issues/973) | [ipfs-cluster/ipfs-cluster#975](https://github.com/ipfs-cluster/ipfs-cluster/issues/975)
* cluster: `health/graph` improvements | [ipfs-cluster/ipfs-cluster#800](https://github.com/ipfs-cluster/ipfs-cluster/issues/800) | [ipfs-cluster/ipfs-cluster#925](https://github.com/ipfs-cluster/ipfs-cluster/issues/925) | [ipfs-cluster/ipfs-cluster#954](https://github.com/ipfs-cluster/ipfs-cluster/issues/954)
* cluster: `ipfs-cluster-ctl ipfs gc` triggers GC on cluster peers | [ipfs-cluster/ipfs-cluster#628](https://github.com/ipfs-cluster/ipfs-cluster/issues/628) | [ipfs-cluster/ipfs-cluster#777](https://github.com/ipfs-cluster/ipfs-cluster/issues/777) | [ipfs-cluster/ipfs-cluster#739](https://github.com/ipfs-cluster/ipfs-cluster/issues/739) | [ipfs-cluster/ipfs-cluster#945](https://github.com/ipfs-cluster/ipfs-cluster/issues/945) | [ipfs-cluster/ipfs-cluster#961](https://github.com/ipfs-cluster/ipfs-cluster/issues/961)
* cluster: advertise external addresses as soon as known | [ipfs-cluster/ipfs-cluster#949](https://github.com/ipfs-cluster/ipfs-cluster/issues/949) | [ipfs-cluster/ipfs-cluster#950](https://github.com/ipfs-cluster/ipfs-cluster/issues/950)
* cluster: skip contacting remote-allocations (peers) for recover/status operations | [ipfs-cluster/ipfs-cluster#935](https://github.com/ipfs-cluster/ipfs-cluster/issues/935) | [ipfs-cluster/ipfs-cluster#947](https://github.com/ipfs-cluster/ipfs-cluster/issues/947)
* restapi: support listening on a unix socket | [ipfs-cluster/ipfs-cluster#969](https://github.com/ipfs-cluster/ipfs-cluster/issues/969)
* config: support `peer_addresses` | [ipfs-cluster/ipfs-cluster#791](https://github.com/ipfs-cluster/ipfs-cluster/issues/791)
* pintracker: remove `mappintracker`. Upgrade `stateless` for prime-time | [ipfs-cluster/ipfs-cluster#944](https://github.com/ipfs-cluster/ipfs-cluster/issues/944) | [ipfs-cluster/ipfs-cluster#929](https://github.com/ipfs-cluster/ipfs-cluster/issues/929)
* service: `--loglevel` supports specifying levels for multiple components | [ipfs-cluster/ipfs-cluster#938](https://github.com/ipfs-cluster/ipfs-cluster/issues/938) | [ipfs-cluster/ipfs-cluster#960](https://github.com/ipfs-cluster/ipfs-cluster/issues/960)
* ipfs-cluster-follow: a new CLI tool to run follower cluster peers | [ipfs-cluster/ipfs-cluster#976](https://github.com/ipfs-cluster/ipfs-cluster/issues/976)

##### Bug fixes

* restapi/client: Fix out of bounds error on load balanced client | [ipfs-cluster/ipfs-cluster#951](https://github.com/ipfs-cluster/ipfs-cluster/issues/951)
* service: disable libp2p restapi on CRDT clusters | [ipfs-cluster/ipfs-cluster#968](https://github.com/ipfs-cluster/ipfs-cluster/issues/968)
* observations: Fix pprof index links | [ipfs-cluster/ipfs-cluster#965](https://github.com/ipfs-cluster/ipfs-cluster/issues/965)

##### Other changes

* Spelling fix in changelog | [ipfs-cluster/ipfs-cluster#920](https://github.com/ipfs-cluster/ipfs-cluster/issues/920)
* Tests: multiple fixes | [ipfs-cluster/ipfs-cluster#919](https://github.com/ipfs-cluster/ipfs-cluster/issues/919) | [ipfs-cluster/ipfs-cluster#943](https://github.com/ipfs-cluster/ipfs-cluster/issues/943) | [ipfs-cluster/ipfs-cluster#953](https://github.com/ipfs-cluster/ipfs-cluster/issues/953) | [ipfs-cluster/ipfs-cluster#956](https://github.com/ipfs-cluster/ipfs-cluster/issues/956)
* Stateless tracker: increase default queue size | [ipfs-cluster/ipfs-cluster#377](https://github.com/ipfs-cluster/ipfs-cluster/issues/377) | [ipfs-cluster/ipfs-cluster#917](https://github.com/ipfs-cluster/ipfs-cluster/issues/917)
* Upgrade to Go1.13 | [ipfs-cluster/ipfs-cluster#934](https://github.com/ipfs-cluster/ipfs-cluster/issues/934)
* Dockerfiles: improvements | [ipfs-cluster/ipfs-cluster#946](https://github.com/ipfs-cluster/ipfs-cluster/issues/946)
* cluster: support multiple informers on initialization | [ipfs-cluster/ipfs-cluster#940](https://github.com/ipfs-cluster/ipfs-cluster/issues/940) | 962
* cmdutils: move some methods to cmdutils | [ipfs-cluster/ipfs-cluster#970](https://github.com/ipfs-cluster/ipfs-cluster/issues/970)


#### Upgrading notices


##### Configuration changes

* `cluster` section:
  * A new `peer_addresses` key allows specifying additional peer addresses in the configuration (similar to the `peerstore` file). These are treated as libp2p bootstrap addreses (do not mix with Raft bootstrap process). This setting is mostly useful for CRDT collaborative clusters, as template configurations can be distributed including bootstrap peers (usually the same as trusted peers). The values are the full multiaddress of these peers: `/ip4/x.x.x.x/tcp/1234/p2p/Qmxxx...`.
  * `listen_multiaddress` can now be set to be an array providing multiple listen multiaddresses, the new defaults being `/tcp/9096` and `/udp/9096/quic`.
  * `enable_relay_hop` (true by default), lets the cluster peer act as a relay for other cluster peers behind NATs. This is only for the Cluster network. As a reminder, while this setting is problematic on IPFS (due to the amount of traffic the HOP peers start relaying), the cluster-peers networks are smaller and do not move huge amounts of content around.
  * The `ipfs_sync_interval` option disappears as the stateless tracker does not keep a state that can lose synchronization with IPFS.
* `ipfshttp` section:
  * A new `repogc_timeout` key specifies the timeout for garbage collection operations on IPFS. It is set to 24h by default.


##### REST API

The `pin/add` and `add` endpoints support two new query parameters to indicate pin expirations: `expire-at` (with an expected value in RFC3339 format) and `expire-in` (with an expected value in Go's time format, i.e. `12h`). `expire-at` has preference.

A new `/ipfs/gc` endpoint has been added to trigger GC in the IPFS daemons attached to Cluster peers. It supports the `local` parameter to limit the operation to the local peer.


##### Go APIs

There are few changes to Go APIs. The `RepoGC` and `RepoGCLocal` methods have been added, the `mappintracker` module has been removed and the `stateless` module has changed the signature of the constructor.

##### Other

The IPFS Proxy now intercepts the `/repo/gc` endpoint and triggers a cluster-wide GC operation.

The `ipfs-cluster-follow` application is an easy to use way to run one or several cluster peers in follower mode using remote configuration templates. It is fully independent from `ipfs-cluster-service` and `ipfs-cluster-ctl` and acts as both a peer (`run` subcommand) and a client (`list` subcommand). The purpose is to facilitate IPFS Cluster usage without having to deal with the configuration and flags etc.

That said, the configuration layout and folder is the same for both `ipfs-cluster-service` and `ipfs-cluster-follow` and they can be run one in place of the other. In the same way, remote-source configurations usually used for `ipfs-cluster-follow` can be replaced with local ones usually used by `ipfs-cluster-service`.

The removal of the `map pintracker` has resulted in a simplification of some operations. `StateSync` (regularly run every `state_sync_interval`) does not trigger repinnings now, but only checks for pin expirations. `RecoverAllLocal` (regularly run every `pin_recover_interval`) will now trigger repinnings when necessary (i.e. when things that were expected to be on IPFS are not). On very large pinsets, this operation can trigger a memory spike as the full recursive pinset from IPFS is requested and loaded on memory (before this happened on `StateSync`).

---

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


* crdt: introduce crdt-based consensus component | [ipfs-cluster/ipfs-cluster#685](https://github.com/ipfs-cluster/ipfs-cluster/issues/685) | [ipfs-cluster/ipfs-cluster#804](https://github.com/ipfs-cluster/ipfs-cluster/issues/804) | [ipfs-cluster/ipfs-cluster#787](https://github.com/ipfs-cluster/ipfs-cluster/issues/787) | [ipfs-cluster/ipfs-cluster#798](https://github.com/ipfs-cluster/ipfs-cluster/issues/798) | [ipfs-cluster/ipfs-cluster#805](https://github.com/ipfs-cluster/ipfs-cluster/issues/805) | [ipfs-cluster/ipfs-cluster#811](https://github.com/ipfs-cluster/ipfs-cluster/issues/811) | [ipfs-cluster/ipfs-cluster#816](https://github.com/ipfs-cluster/ipfs-cluster/issues/816) | [ipfs-cluster/ipfs-cluster#820](https://github.com/ipfs-cluster/ipfs-cluster/issues/820) | [ipfs-cluster/ipfs-cluster#856](https://github.com/ipfs-cluster/ipfs-cluster/issues/856) | [ipfs-cluster/ipfs-cluster#857](https://github.com/ipfs-cluster/ipfs-cluster/issues/857) | [ipfs-cluster/ipfs-cluster#834](https://github.com/ipfs-cluster/ipfs-cluster/issues/834) | [ipfs-cluster/ipfs-cluster#856](https://github.com/ipfs-cluster/ipfs-cluster/issues/856) | [ipfs-cluster/ipfs-cluster#867](https://github.com/ipfs-cluster/ipfs-cluster/issues/867) | [ipfs-cluster/ipfs-cluster#874](https://github.com/ipfs-cluster/ipfs-cluster/issues/874) | [ipfs-cluster/ipfs-cluster#885](https://github.com/ipfs-cluster/ipfs-cluster/issues/885) | [ipfs-cluster/ipfs-cluster#899](https://github.com/ipfs-cluster/ipfs-cluster/issues/899) | [ipfs-cluster/ipfs-cluster#906](https://github.com/ipfs-cluster/ipfs-cluster/issues/906) | [ipfs-cluster/ipfs-cluster#918](https://github.com/ipfs-cluster/ipfs-cluster/issues/918)
* configs: separate identity and configuration | [ipfs-cluster/ipfs-cluster#760](https://github.com/ipfs-cluster/ipfs-cluster/issues/760) | [ipfs-cluster/ipfs-cluster#766](https://github.com/ipfs-cluster/ipfs-cluster/issues/766) | [ipfs-cluster/ipfs-cluster#780](https://github.com/ipfs-cluster/ipfs-cluster/issues/780)
* configs: support running with a remote `service.json` (http) | [ipfs-cluster/ipfs-cluster#868](https://github.com/ipfs-cluster/ipfs-cluster/issues/868)
* configs: support a `follower_mode` option | [ipfs-cluster/ipfs-cluster#803](https://github.com/ipfs-cluster/ipfs-cluster/issues/803) | [ipfs-cluster/ipfs-cluster#864](https://github.com/ipfs-cluster/ipfs-cluster/issues/864)
* service/configs: do not load API components if no config present | [ipfs-cluster/ipfs-cluster#452](https://github.com/ipfs-cluster/ipfs-cluster/issues/452) | [ipfs-cluster/ipfs-cluster#836](https://github.com/ipfs-cluster/ipfs-cluster/issues/836)
* service: add `ipfs-cluster-service init --peers` flag to initialize with given peers | [ipfs-cluster/ipfs-cluster#835](https://github.com/ipfs-cluster/ipfs-cluster/issues/835) | [ipfs-cluster/ipfs-cluster#839](https://github.com/ipfs-cluster/ipfs-cluster/issues/839) | [ipfs-cluster/ipfs-cluster#870](https://github.com/ipfs-cluster/ipfs-cluster/issues/870)
* cluster: RPC auth: block rpc endpoints for non trusted peers | [ipfs-cluster/ipfs-cluster#775](https://github.com/ipfs-cluster/ipfs-cluster/issues/775) | [ipfs-cluster/ipfs-cluster#710](https://github.com/ipfs-cluster/ipfs-cluster/issues/710) | [ipfs-cluster/ipfs-cluster#666](https://github.com/ipfs-cluster/ipfs-cluster/issues/666) | [ipfs-cluster/ipfs-cluster#773](https://github.com/ipfs-cluster/ipfs-cluster/issues/773) | [ipfs-cluster/ipfs-cluster#905](https://github.com/ipfs-cluster/ipfs-cluster/issues/905)
* cluster: introduce connection manager | [ipfs-cluster/ipfs-cluster#791](https://github.com/ipfs-cluster/ipfs-cluster/issues/791)
* cluster: support new `PinUpdate` option for new pins | [ipfs-cluster/ipfs-cluster#869](https://github.com/ipfs-cluster/ipfs-cluster/issues/869) | [ipfs-cluster/ipfs-cluster#732](https://github.com/ipfs-cluster/ipfs-cluster/issues/732)
* cluster: trigger `Recover` automatically on a configurable interval | [ipfs-cluster/ipfs-cluster#831](https://github.com/ipfs-cluster/ipfs-cluster/issues/831) | [ipfs-cluster/ipfs-cluster#887](https://github.com/ipfs-cluster/ipfs-cluster/issues/887)
* cluster: enable mDNS discovery for peers | [ipfs-cluster/ipfs-cluster#882](https://github.com/ipfs-cluster/ipfs-cluster/issues/882) | [ipfs-cluster/ipfs-cluster#900](https://github.com/ipfs-cluster/ipfs-cluster/issues/900)
* IPFS Proxy: Support `pin/update` | [ipfs-cluster/ipfs-cluster#732](https://github.com/ipfs-cluster/ipfs-cluster/issues/732) | [ipfs-cluster/ipfs-cluster#768](https://github.com/ipfs-cluster/ipfs-cluster/issues/768) | [ipfs-cluster/ipfs-cluster#887](https://github.com/ipfs-cluster/ipfs-cluster/issues/887)
* monitor: Accrual failure detection. Leaderless re-pinning | [ipfs-cluster/ipfs-cluster#413](https://github.com/ipfs-cluster/ipfs-cluster/issues/413) | [ipfs-cluster/ipfs-cluster#713](https://github.com/ipfs-cluster/ipfs-cluster/issues/713) | [ipfs-cluster/ipfs-cluster#714](https://github.com/ipfs-cluster/ipfs-cluster/issues/714) | [ipfs-cluster/ipfs-cluster#812](https://github.com/ipfs-cluster/ipfs-cluster/issues/812) | [ipfs-cluster/ipfs-cluster#813](https://github.com/ipfs-cluster/ipfs-cluster/issues/813) | [ipfs-cluster/ipfs-cluster#814](https://github.com/ipfs-cluster/ipfs-cluster/issues/814) | [ipfs-cluster/ipfs-cluster#815](https://github.com/ipfs-cluster/ipfs-cluster/issues/815)
* datastore: Expose badger configuration | [ipfs-cluster/ipfs-cluster#771](https://github.com/ipfs-cluster/ipfs-cluster/issues/771) | [ipfs-cluster/ipfs-cluster#776](https://github.com/ipfs-cluster/ipfs-cluster/issues/776)
* IPFSConnector: pin timeout start counting from last received block | [ipfs-cluster/ipfs-cluster#497](https://github.com/ipfs-cluster/ipfs-cluster/issues/497) | [ipfs-cluster/ipfs-cluster#738](https://github.com/ipfs-cluster/ipfs-cluster/issues/738)
* IPFSConnector: remove pin method options | [ipfs-cluster/ipfs-cluster#875](https://github.com/ipfs-cluster/ipfs-cluster/issues/875)
* IPFSConnector: `unpin_disable` removes the ability to unpin anything from ipfs (experimental) | [ipfs-cluster/ipfs-cluster#793](https://github.com/ipfs-cluster/ipfs-cluster/issues/793) | [ipfs-cluster/ipfs-cluster#832](https://github.com/ipfs-cluster/ipfs-cluster/issues/832)
* REST API Client: Load-balancing Go client | [ipfs-cluster/ipfs-cluster#448](https://github.com/ipfs-cluster/ipfs-cluster/issues/448) | [ipfs-cluster/ipfs-cluster#737](https://github.com/ipfs-cluster/ipfs-cluster/issues/737)
* REST API: Return allocation objects on pin/unpin | [ipfs-cluster/ipfs-cluster#843](https://github.com/ipfs-cluster/ipfs-cluster/issues/843)
* REST API: Support request logging | [ipfs-cluster/ipfs-cluster#574](https://github.com/ipfs-cluster/ipfs-cluster/issues/574) | [ipfs-cluster/ipfs-cluster#894](https://github.com/ipfs-cluster/ipfs-cluster/issues/894)
* Adder: improve error handling. Keep adding while at least one allocation works | [ipfs-cluster/ipfs-cluster#852](https://github.com/ipfs-cluster/ipfs-cluster/issues/852) | [ipfs-cluster/ipfs-cluster#871](https://github.com/ipfs-cluster/ipfs-cluster/issues/871)
* Adder: support user-given allocations for the `Add` operation | [ipfs-cluster/ipfs-cluster#761](https://github.com/ipfs-cluster/ipfs-cluster/issues/761) | [ipfs-cluster/ipfs-cluster#890](https://github.com/ipfs-cluster/ipfs-cluster/issues/890)
* ctl: support adding pin metadata | [ipfs-cluster/ipfs-cluster#670](https://github.com/ipfs-cluster/ipfs-cluster/issues/670) | [ipfs-cluster/ipfs-cluster#891](https://github.com/ipfs-cluster/ipfs-cluster/issues/891)


##### Bug fixes

* REST API: Fix `/allocations` when filter unset | [ipfs-cluster/ipfs-cluster#762](https://github.com/ipfs-cluster/ipfs-cluster/issues/762)
* REST API: Fix DELETE returning 500 when pin does not exist | [ipfs-cluster/ipfs-cluster#742](https://github.com/ipfs-cluster/ipfs-cluster/issues/742) | [ipfs-cluster/ipfs-cluster#854](https://github.com/ipfs-cluster/ipfs-cluster/issues/854)
* REST API: Return JSON body on 404s | [ipfs-cluster/ipfs-cluster#657](https://github.com/ipfs-cluster/ipfs-cluster/issues/657) | [ipfs-cluster/ipfs-cluster#879](https://github.com/ipfs-cluster/ipfs-cluster/issues/879)
* service: connectivity fixes | [ipfs-cluster/ipfs-cluster#787](https://github.com/ipfs-cluster/ipfs-cluster/issues/787) | [ipfs-cluster/ipfs-cluster#792](https://github.com/ipfs-cluster/ipfs-cluster/issues/792)
* service: fix using `/dnsaddr` peers | [ipfs-cluster/ipfs-cluster#818](https://github.com/ipfs-cluster/ipfs-cluster/issues/818)
* service: reading empty lines on peerstore panics | [ipfs-cluster/ipfs-cluster#886](https://github.com/ipfs-cluster/ipfs-cluster/issues/886)
* service/ctl: fix parsing string lists | [ipfs-cluster/ipfs-cluster#876](https://github.com/ipfs-cluster/ipfs-cluster/issues/876) | [ipfs-cluster/ipfs-cluster#841](https://github.com/ipfs-cluster/ipfs-cluster/issues/841)
* IPFSConnector: `pin/ls` does handle base32 and base58 cids properly | [ipfs-cluster/ipfs-cluster#808](https://github.com/ipfs-cluster/ipfs-cluster/issues/808) [ipfs-cluster/ipfs-cluster#809](https://github.com/ipfs-cluster/ipfs-cluster/issues/809)
* configs: some config keys not matching ENV vars names | [ipfs-cluster/ipfs-cluster#837](https://github.com/ipfs-cluster/ipfs-cluster/issues/837) | [ipfs-cluster/ipfs-cluster#778](https://github.com/ipfs-cluster/ipfs-cluster/issues/778)
* raft: delete removed raft peers from peerstore | [ipfs-cluster/ipfs-cluster#840](https://github.com/ipfs-cluster/ipfs-cluster/issues/840) | [ipfs-cluster/ipfs-cluster#846](https://github.com/ipfs-cluster/ipfs-cluster/issues/846)
* cluster: peers forgotten after being down | [ipfs-cluster/ipfs-cluster#648](https://github.com/ipfs-cluster/ipfs-cluster/issues/648) | [ipfs-cluster/ipfs-cluster#860](https://github.com/ipfs-cluster/ipfs-cluster/issues/860)
* cluster: State sync should not keep tracking when queue is full | [ipfs-cluster/ipfs-cluster#377](https://github.com/ipfs-cluster/ipfs-cluster/issues/377) | [ipfs-cluster/ipfs-cluster#901](https://github.com/ipfs-cluster/ipfs-cluster/issues/901)
* cluster: avoid random order on peer lists and listen multiaddresses | [ipfs-cluster/ipfs-cluster#327](https://github.com/ipfs-cluster/ipfs-cluster/issues/327) | [ipfs-cluster/ipfs-cluster#878](https://github.com/ipfs-cluster/ipfs-cluster/issues/878)
* cluster: fix recover and allocation re-assignment to existing pins | [ipfs-cluster/ipfs-cluster#912](https://github.com/ipfs-cluster/ipfs-cluster/issues/912) | [ipfs-cluster/ipfs-cluster#888](https://github.com/ipfs-cluster/ipfs-cluster/issues/888)

##### Other changes

* cluster: Dependency updates | [ipfs-cluster/ipfs-cluster#769](https://github.com/ipfs-cluster/ipfs-cluster/issues/769) | [ipfs-cluster/ipfs-cluster#789](https://github.com/ipfs-cluster/ipfs-cluster/issues/789) | [ipfs-cluster/ipfs-cluster#795](https://github.com/ipfs-cluster/ipfs-cluster/issues/795) | [ipfs-cluster/ipfs-cluster#822](https://github.com/ipfs-cluster/ipfs-cluster/issues/822) | [ipfs-cluster/ipfs-cluster#823](https://github.com/ipfs-cluster/ipfs-cluster/issues/823) | [ipfs-cluster/ipfs-cluster#828](https://github.com/ipfs-cluster/ipfs-cluster/issues/828) | [ipfs-cluster/ipfs-cluster#830](https://github.com/ipfs-cluster/ipfs-cluster/issues/830) | [ipfs-cluster/ipfs-cluster#853](https://github.com/ipfs-cluster/ipfs-cluster/issues/853) | [ipfs-cluster/ipfs-cluster#839](https://github.com/ipfs-cluster/ipfs-cluster/issues/839)
* cluster: Set `[]peer.ID` as type for user allocations | [ipfs-cluster/ipfs-cluster#767](https://github.com/ipfs-cluster/ipfs-cluster/issues/767)
* cluster: RPC: Split services among components | [ipfs-cluster/ipfs-cluster#773](https://github.com/ipfs-cluster/ipfs-cluster/issues/773)
* cluster: Multiple improvements to tests | [ipfs-cluster/ipfs-cluster#360](https://github.com/ipfs-cluster/ipfs-cluster/issues/360) | [ipfs-cluster/ipfs-cluster#502](https://github.com/ipfs-cluster/ipfs-cluster/issues/502) | [ipfs-cluster/ipfs-cluster#779](https://github.com/ipfs-cluster/ipfs-cluster/issues/779) | [ipfs-cluster/ipfs-cluster#833](https://github.com/ipfs-cluster/ipfs-cluster/issues/833) | [ipfs-cluster/ipfs-cluster#863](https://github.com/ipfs-cluster/ipfs-cluster/issues/863) | [ipfs-cluster/ipfs-cluster#883](https://github.com/ipfs-cluster/ipfs-cluster/issues/883) | [ipfs-cluster/ipfs-cluster#884](https://github.com/ipfs-cluster/ipfs-cluster/issues/884) | [ipfs-cluster/ipfs-cluster#797](https://github.com/ipfs-cluster/ipfs-cluster/issues/797) | [ipfs-cluster/ipfs-cluster#892](https://github.com/ipfs-cluster/ipfs-cluster/issues/892)
* cluster: Remove Gx | [ipfs-cluster/ipfs-cluster#765](https://github.com/ipfs-cluster/ipfs-cluster/issues/765) | [ipfs-cluster/ipfs-cluster#781](https://github.com/ipfs-cluster/ipfs-cluster/issues/781)
* cluster: Use `/p2p/` instead of `/ipfs/` in multiaddresses | [ipfs-cluster/ipfs-cluster#431](https://github.com/ipfs-cluster/ipfs-cluster/issues/431) | [ipfs-cluster/ipfs-cluster#877](https://github.com/ipfs-cluster/ipfs-cluster/issues/877)
* cluster: consolidate parsing of pin options | [ipfs-cluster/ipfs-cluster#913](https://github.com/ipfs-cluster/ipfs-cluster/issues/913)
* REST API: Replace regexps with `strings.HasPrefix` | [ipfs-cluster/ipfs-cluster#806](https://github.com/ipfs-cluster/ipfs-cluster/issues/806) | [ipfs-cluster/ipfs-cluster#807](https://github.com/ipfs-cluster/ipfs-cluster/issues/807)
* docker: use GOPROXY to build containers | [ipfs-cluster/ipfs-cluster#872](https://github.com/ipfs-cluster/ipfs-cluster/issues/872)
* docker: support `IPFS_CLUSTER_CONSENSUS` flag and other improvements | [ipfs-cluster/ipfs-cluster#882](https://github.com/ipfs-cluster/ipfs-cluster/issues/882)
* ctl: increase space for peernames | [ipfs-cluster/ipfs-cluster#887](https://github.com/ipfs-cluster/ipfs-cluster/issues/887)
* ctl: improve replication factor 0 explanation | [ipfs-cluster/ipfs-cluster#755](https://github.com/ipfs-cluster/ipfs-cluster/issues/755) | [ipfs-cluster/ipfs-cluster#909](https://github.com/ipfs-cluster/ipfs-cluster/issues/909)

#### Upgrading notices


##### Configuration changes

This release introduces a number of backwards-compatible configuration changes:

* The `service.json` file no longer includes `ID` and `PrivateKey`, which are
  now part of an `identity.json` file. However things should work as before if
  they do. Running `ipfs-cluster-service daemon` on a older configuration will
  automatically write an `identity.json` file with the old credentials so that
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

* Switch to go.mod | [ipfs-cluster/ipfs-cluster#706](https://github.com/ipfs-cluster/ipfs-cluster/issues/706) | [ipfs-cluster/ipfs-cluster#707](https://github.com/ipfs-cluster/ipfs-cluster/issues/707) | [ipfs-cluster/ipfs-cluster#708](https://github.com/ipfs-cluster/ipfs-cluster/issues/708)
* Remove basic monitor | [ipfs-cluster/ipfs-cluster#689](https://github.com/ipfs-cluster/ipfs-cluster/issues/689) | [ipfs-cluster/ipfs-cluster#726](https://github.com/ipfs-cluster/ipfs-cluster/issues/726)
* Support `nocopy` when adding URLs | [ipfs-cluster/ipfs-cluster#735](https://github.com/ipfs-cluster/ipfs-cluster/issues/735)

##### Bug fixes

* Mitigate long header attack | [ipfs-cluster/ipfs-cluster#636](https://github.com/ipfs-cluster/ipfs-cluster/issues/636) | [ipfs-cluster/ipfs-cluster#712](https://github.com/ipfs-cluster/ipfs-cluster/issues/712)
* Fix download link in README | [ipfs-cluster/ipfs-cluster#723](https://github.com/ipfs-cluster/ipfs-cluster/issues/723)
* Fix `peers ls` error when peers down | [ipfs-cluster/ipfs-cluster#715](https://github.com/ipfs-cluster/ipfs-cluster/issues/715) | [ipfs-cluster/ipfs-cluster#719](https://github.com/ipfs-cluster/ipfs-cluster/issues/719)
* Nil pointer panic on `ipfs-cluster-ctl add` | [ipfs-cluster/ipfs-cluster#727](https://github.com/ipfs-cluster/ipfs-cluster/issues/727) | [ipfs-cluster/ipfs-cluster#728](https://github.com/ipfs-cluster/ipfs-cluster/issues/728)
* Fix `enc=json` output on `ipfs-cluster-ctl add` | [ipfs-cluster/ipfs-cluster#729](https://github.com/ipfs-cluster/ipfs-cluster/issues/729)
* Add SSL CAs to Docker container | [ipfs-cluster/ipfs-cluster#730](https://github.com/ipfs-cluster/ipfs-cluster/issues/730) | [ipfs-cluster/ipfs-cluster#731](https://github.com/ipfs-cluster/ipfs-cluster/issues/731)
* Remove duplicate import | [ipfs-cluster/ipfs-cluster#734](https://github.com/ipfs-cluster/ipfs-cluster/issues/734)
* Fix version json object | [ipfs-cluster/ipfs-cluster#743](https://github.com/ipfs-cluster/ipfs-cluster/issues/743) | [ipfs-cluster/ipfs-cluster#752](https://github.com/ipfs-cluster/ipfs-cluster/issues/752)

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
Raft, IPFS Cluster 0.10.0 prepares the ground with substantial under-the-hood
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

Note that on this release we have also removed a lot of backwards-compatibility
code for things older than version 0.8.0, which kept things working but
printed respective warnings. If you're upgrading from an old release, consider
comparing your configuration with the new default one.


#### List of changes

##### Features

  * Add full support for environment variables in configurations and initialization | [ipfs-cluster/ipfs-cluster#656](https://github.com/ipfs-cluster/ipfs-cluster/issues/656) | [ipfs-cluster/ipfs-cluster#663](https://github.com/ipfs-cluster/ipfs-cluster/issues/663) | [ipfs-cluster/ipfs-cluster#667](https://github.com/ipfs-cluster/ipfs-cluster/issues/667)
  * Switch to codecov | [ipfs-cluster/ipfs-cluster#683](https://github.com/ipfs-cluster/ipfs-cluster/issues/683)
  * Add auto-resolving IPFS paths | [ipfs-cluster/ipfs-cluster#450](https://github.com/ipfs-cluster/ipfs-cluster/issues/450) | [ipfs-cluster/ipfs-cluster#634](https://github.com/ipfs-cluster/ipfs-cluster/issues/634)
  * Support user-defined allocations | [ipfs-cluster/ipfs-cluster#646](https://github.com/ipfs-cluster/ipfs-cluster/issues/646) | [ipfs-cluster/ipfs-cluster#647](https://github.com/ipfs-cluster/ipfs-cluster/issues/647)
  * Support user-defined metadata in pin objects | [ipfs-cluster/ipfs-cluster#681](https://github.com/ipfs-cluster/ipfs-cluster/issues/681)
  * Make normal types serializable and remove `*Serial` types | [ipfs-cluster/ipfs-cluster#654](https://github.com/ipfs-cluster/ipfs-cluster/issues/654) | [ipfs-cluster/ipfs-cluster#688](https://github.com/ipfs-cluster/ipfs-cluster/issues/688) | [ipfs-cluster/ipfs-cluster#700](https://github.com/ipfs-cluster/ipfs-cluster/issues/700)
  * Support IPFS paths in the IPFS proxy | [ipfs-cluster/ipfs-cluster#480](https://github.com/ipfs-cluster/ipfs-cluster/issues/480) | [ipfs-cluster/ipfs-cluster#690](https://github.com/ipfs-cluster/ipfs-cluster/issues/690)
  * Use go-datastore as backend for the cluster state | [ipfs-cluster/ipfs-cluster#655](https://github.com/ipfs-cluster/ipfs-cluster/issues/655)
  * Upgrade dependencies | [ipfs-cluster/ipfs-cluster#675](https://github.com/ipfs-cluster/ipfs-cluster/issues/675) | [ipfs-cluster/ipfs-cluster#679](https://github.com/ipfs-cluster/ipfs-cluster/issues/679) | [ipfs-cluster/ipfs-cluster#686](https://github.com/ipfs-cluster/ipfs-cluster/issues/686) | [ipfs-cluster/ipfs-cluster#687](https://github.com/ipfs-cluster/ipfs-cluster/issues/687)
  * Adopt MIT+Apache 2 License (no more sign-off required) | [ipfs-cluster/ipfs-cluster#692](https://github.com/ipfs-cluster/ipfs-cluster/issues/692)
  * Add codecov configurtion file | [ipfs-cluster/ipfs-cluster#693](https://github.com/ipfs-cluster/ipfs-cluster/issues/693)
  * Additional tests for basic auth | [ipfs-cluster/ipfs-cluster#645](https://github.com/ipfs-cluster/ipfs-cluster/issues/645) | [ipfs-cluster/ipfs-cluster#694](https://github.com/ipfs-cluster/ipfs-cluster/issues/694)

##### Bug fixes

  * Fix docker compose tests | [ipfs-cluster/ipfs-cluster#696](https://github.com/ipfs-cluster/ipfs-cluster/issues/696)
  * Hide `ipfsproxy.extract_headers_ttl` and `ipfsproxy.extract_headers_path` options by default | [ipfs-cluster/ipfs-cluster#699](https://github.com/ipfs-cluster/ipfs-cluster/issues/699)

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

  * Integrate [OpenCensus](https://opencensus.io) tracing and metrics into IPFS Cluster codebase | [ipfs-cluster/ipfs-cluster#486](https://github.com/ipfs-cluster/ipfs-cluster/issues/486) | [ipfs-cluster/ipfs-cluster#658](https://github.com/ipfs-cluster/ipfs-cluster/issues/658) | [ipfs-cluster/ipfs-cluster#659](https://github.com/ipfs-cluster/ipfs-cluster/issues/659) | [ipfs-cluster/ipfs-cluster#676](https://github.com/ipfs-cluster/ipfs-cluster/issues/676) | [ipfs-cluster/ipfs-cluster#671](https://github.com/ipfs-cluster/ipfs-cluster/issues/671) | [ipfs-cluster/ipfs-cluster#674](https://github.com/ipfs-cluster/ipfs-cluster/issues/674)

##### Bug Fixes

No bugs were fixed from the previous release.

##### Deprecated

  * The snap distribution of IPFS Cluster has been removed | [ipfs-cluster/ipfs-cluster#593](https://github.com/ipfs-cluster/ipfs-cluster/issues/593) | [ipfs-cluster/ipfs-cluster#649](https://github.com/ipfs-cluster/ipfs-cluster/issues/649).

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

  * IPFS Proxy extraction to its own `API` component: `ipfsproxy` | [ipfs-cluster/ipfs-cluster#453](https://github.com/ipfs-cluster/ipfs-cluster/issues/453) | [ipfs-cluster/ipfs-cluster#576](https://github.com/ipfs-cluster/ipfs-cluster/issues/576) | [ipfs-cluster/ipfs-cluster#616](https://github.com/ipfs-cluster/ipfs-cluster/issues/616) | [ipfs-cluster/ipfs-cluster#617](https://github.com/ipfs-cluster/ipfs-cluster/issues/617)
  * Add full CORS handling to `restapi` | [ipfs-cluster/ipfs-cluster#639](https://github.com/ipfs-cluster/ipfs-cluster/issues/639) | [ipfs-cluster/ipfs-cluster#640](https://github.com/ipfs-cluster/ipfs-cluster/issues/640)
  * `restapi` configuration section entries can be overridden from environment variables | [ipfs-cluster/ipfs-cluster#609](https://github.com/ipfs-cluster/ipfs-cluster/issues/609)
  * Update to `go-ipfs-files` 2.0 | [ipfs-cluster/ipfs-cluster#613](https://github.com/ipfs-cluster/ipfs-cluster/issues/613)
  * Tests for the `/monitor/metrics` endpoint | [ipfs-cluster/ipfs-cluster#587](https://github.com/ipfs-cluster/ipfs-cluster/issues/587) | [ipfs-cluster/ipfs-cluster#622](https://github.com/ipfs-cluster/ipfs-cluster/issues/622)
  * Support `stream-channels=fase` query parameter in `/add` | [ipfs-cluster/ipfs-cluster#632](https://github.com/ipfs-cluster/ipfs-cluster/issues/632) | [ipfs-cluster/ipfs-cluster#633](https://github.com/ipfs-cluster/ipfs-cluster/issues/633)
  * Support server side `/pins` filtering  | [ipfs-cluster/ipfs-cluster#445](https://github.com/ipfs-cluster/ipfs-cluster/issues/445) | [ipfs-cluster/ipfs-cluster#478](https://github.com/ipfs-cluster/ipfs-cluster/issues/478) | [ipfs-cluster/ipfs-cluster#627](https://github.com/ipfs-cluster/ipfs-cluster/issues/627)
  * `ipfs-cluster-ctl add --no-stream` option | [ipfs-cluster/ipfs-cluster#632](https://github.com/ipfs-cluster/ipfs-cluster/issues/632) | [ipfs-cluster/ipfs-cluster#637](https://github.com/ipfs-cluster/ipfs-cluster/issues/637)
  * Upgrade dependencies and libp2p to version 6.0.29 | [ipfs-cluster/ipfs-cluster#624](https://github.com/ipfs-cluster/ipfs-cluster/issues/624)

##### Bug fixes

 * Respect IPFS daemon response headers on non-proxied calls | [ipfs-cluster/ipfs-cluster#382](https://github.com/ipfs-cluster/ipfs-cluster/issues/382) | [ipfs-cluster/ipfs-cluster#623](https://github.com/ipfs-cluster/ipfs-cluster/issues/623) | [ipfs-cluster/ipfs-cluster#638](https://github.com/ipfs-cluster/ipfs-cluster/issues/638)
 * Fix `ipfs-cluster-ctl` usage with HTTPs and `/dns*` hostnames | [ipfs-cluster/ipfs-cluster#626](https://github.com/ipfs-cluster/ipfs-cluster/issues/626)
 * Minor fixes in sharness | [ipfs-cluster/ipfs-cluster#641](https://github.com/ipfs-cluster/ipfs-cluster/issues/641) | [ipfs-cluster/ipfs-cluster#643](https://github.com/ipfs-cluster/ipfs-cluster/issues/643)
 * Fix error handling when parsing the configuration | [ipfs-cluster/ipfs-cluster#642](https://github.com/ipfs-cluster/ipfs-cluster/issues/642)



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

  * Clean (rotate) the state when running `init` | [ipfs-cluster/ipfs-cluster#532](https://github.com/ipfs-cluster/ipfs-cluster/issues/532) | [ipfs-cluster/ipfs-cluster#553](https://github.com/ipfs-cluster/ipfs-cluster/issues/553)
  * Configurable REST API headers and CORS defaults | [ipfs-cluster/ipfs-cluster#578](https://github.com/ipfs-cluster/ipfs-cluster/issues/578)
  * Upgrade libp2p and other deps | [ipfs-cluster/ipfs-cluster#580](https://github.com/ipfs-cluster/ipfs-cluster/issues/580) | [ipfs-cluster/ipfs-cluster#590](https://github.com/ipfs-cluster/ipfs-cluster/issues/590) | [ipfs-cluster/ipfs-cluster#592](https://github.com/ipfs-cluster/ipfs-cluster/issues/592) | [ipfs-cluster/ipfs-cluster#598](https://github.com/ipfs-cluster/ipfs-cluster/issues/598) | [ipfs-cluster/ipfs-cluster#599](https://github.com/ipfs-cluster/ipfs-cluster/issues/599)
  * Use `gossipsub` to broadcast metrics | [ipfs-cluster/ipfs-cluster#573](https://github.com/ipfs-cluster/ipfs-cluster/issues/573)
  * Download gx and gx-go from IPFS preferentially | [ipfs-cluster/ipfs-cluster#577](https://github.com/ipfs-cluster/ipfs-cluster/issues/577) | [ipfs-cluster/ipfs-cluster#581](https://github.com/ipfs-cluster/ipfs-cluster/issues/581)
  * Expose peer metrics in the API + ctl commands | [ipfs-cluster/ipfs-cluster#449](https://github.com/ipfs-cluster/ipfs-cluster/issues/449) | [ipfs-cluster/ipfs-cluster#572](https://github.com/ipfs-cluster/ipfs-cluster/issues/572) | [ipfs-cluster/ipfs-cluster#589](https://github.com/ipfs-cluster/ipfs-cluster/issues/589) | [ipfs-cluster/ipfs-cluster#587](https://github.com/ipfs-cluster/ipfs-cluster/issues/587)
  * Add a `docker-compose.yml` template, which creates a two peer cluster | [ipfs-cluster/ipfs-cluster#585](https://github.com/ipfs-cluster/ipfs-cluster/issues/585) | [ipfs-cluster/ipfs-cluster#588](https://github.com/ipfs-cluster/ipfs-cluster/issues/588)
  * Support overwriting configuration values in the `cluster` section with environmental values | [ipfs-cluster/ipfs-cluster#575](https://github.com/ipfs-cluster/ipfs-cluster/issues/575) | [ipfs-cluster/ipfs-cluster#596](https://github.com/ipfs-cluster/ipfs-cluster/issues/596)
  * Set snaps to `classic` confinement mode and revert it since approval never arrived | [ipfs-cluster/ipfs-cluster#579](https://github.com/ipfs-cluster/ipfs-cluster/issues/579) | [ipfs-cluster/ipfs-cluster#594](https://github.com/ipfs-cluster/ipfs-cluster/issues/594)
* Use Go's reverse proxy library in the proxy endpoint | [ipfs-cluster/ipfs-cluster#570](https://github.com/ipfs-cluster/ipfs-cluster/issues/570) | [ipfs-cluster/ipfs-cluster#605](https://github.com/ipfs-cluster/ipfs-cluster/issues/605)


##### Bug fixes

  * `/add` endpoints improvements and IPFS Companion compatibility | [ipfs-cluster/ipfs-cluster#582](https://github.com/ipfs-cluster/ipfs-cluster/issues/582) | [ipfs-cluster/ipfs-cluster#569](https://github.com/ipfs-cluster/ipfs-cluster/issues/569)
  * Fix adding with spaces in the name parameter | [ipfs-cluster/ipfs-cluster#583](https://github.com/ipfs-cluster/ipfs-cluster/issues/583)
  * Escape filter query parameter | [ipfs-cluster/ipfs-cluster#586](https://github.com/ipfs-cluster/ipfs-cluster/issues/586)
  * Fix some race conditions | [ipfs-cluster/ipfs-cluster#597](https://github.com/ipfs-cluster/ipfs-cluster/issues/597)
  * Improve pin deserialization efficiency | [ipfs-cluster/ipfs-cluster#601](https://github.com/ipfs-cluster/ipfs-cluster/issues/601)
  * Do not error remote pins | [ipfs-cluster/ipfs-cluster#600](https://github.com/ipfs-cluster/ipfs-cluster/issues/600) | [ipfs-cluster/ipfs-cluster#603](https://github.com/ipfs-cluster/ipfs-cluster/issues/603)
  * Clean up testing folders in `rest` and `rest/client` after tests | [ipfs-cluster/ipfs-cluster#607](https://github.com/ipfs-cluster/ipfs-cluster/issues/607)

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

  * Move commands to the `cmd/` folder | [ipfs-cluster/ipfs-cluster#485](https://github.com/ipfs-cluster/ipfs-cluster/issues/485) | [ipfs-cluster/ipfs-cluster#521](https://github.com/ipfs-cluster/ipfs-cluster/issues/521) | [ipfs-cluster/ipfs-cluster#556](https://github.com/ipfs-cluster/ipfs-cluster/issues/556)
  * Dependency upgrades: `go-dot`, `go-libp2p`, `cid` | [ipfs-cluster/ipfs-cluster#533](https://github.com/ipfs-cluster/ipfs-cluster/issues/533) | [ipfs-cluster/ipfs-cluster#537](https://github.com/ipfs-cluster/ipfs-cluster/issues/537) | [ipfs-cluster/ipfs-cluster#535](https://github.com/ipfs-cluster/ipfs-cluster/issues/535) | [ipfs-cluster/ipfs-cluster#544](https://github.com/ipfs-cluster/ipfs-cluster/issues/544) | [ipfs-cluster/ipfs-cluster#561](https://github.com/ipfs-cluster/ipfs-cluster/issues/561)
  * Build with go-1.11 | [ipfs-cluster/ipfs-cluster#558](https://github.com/ipfs-cluster/ipfs-cluster/issues/558)
  * Peer names in `PinInfo` | [ipfs-cluster/ipfs-cluster#446](https://github.com/ipfs-cluster/ipfs-cluster/issues/446) | [ipfs-cluster/ipfs-cluster#531](https://github.com/ipfs-cluster/ipfs-cluster/issues/531)
  * Wrap API client in an interface | [ipfs-cluster/ipfs-cluster#447](https://github.com/ipfs-cluster/ipfs-cluster/issues/447) | [ipfs-cluster/ipfs-cluster#523](https://github.com/ipfs-cluster/ipfs-cluster/issues/523) | [ipfs-cluster/ipfs-cluster#564](https://github.com/ipfs-cluster/ipfs-cluster/issues/564)
  * `Makefile`: add `prcheck` target and fix `make all` | [ipfs-cluster/ipfs-cluster#536](https://github.com/ipfs-cluster/ipfs-cluster/issues/536) | [ipfs-cluster/ipfs-cluster#542](https://github.com/ipfs-cluster/ipfs-cluster/issues/542) | [ipfs-cluster/ipfs-cluster#539](https://github.com/ipfs-cluster/ipfs-cluster/issues/539)
  * Docker: speed up [re]builds | [ipfs-cluster/ipfs-cluster#529](https://github.com/ipfs-cluster/ipfs-cluster/issues/529)
  * Re-enable keep-alives on servers | [ipfs-cluster/ipfs-cluster#548](https://github.com/ipfs-cluster/ipfs-cluster/issues/548) | [ipfs-cluster/ipfs-cluster#560](https://github.com/ipfs-cluster/ipfs-cluster/issues/560)

##### Bugfixes

  * Fix adding to cluster with unhealthy peers | [ipfs-cluster/ipfs-cluster#543](https://github.com/ipfs-cluster/ipfs-cluster/issues/543) | [ipfs-cluster/ipfs-cluster#549](https://github.com/ipfs-cluster/ipfs-cluster/issues/549)
  * Fix Snap builds and pushes: multiple architectures re-enabled | [ipfs-cluster/ipfs-cluster#520](https://github.com/ipfs-cluster/ipfs-cluster/issues/520) | [ipfs-cluster/ipfs-cluster#554](https://github.com/ipfs-cluster/ipfs-cluster/issues/554) | [ipfs-cluster/ipfs-cluster#557](https://github.com/ipfs-cluster/ipfs-cluster/issues/557) | [ipfs-cluster/ipfs-cluster#562](https://github.com/ipfs-cluster/ipfs-cluster/issues/562) | [ipfs-cluster/ipfs-cluster#565](https://github.com/ipfs-cluster/ipfs-cluster/issues/565)
  * Docs: Typos in Readme and some improvements | [ipfs-cluster/ipfs-cluster#547](https://github.com/ipfs-cluster/ipfs-cluster/issues/547) | [ipfs-cluster/ipfs-cluster#567](https://github.com/ipfs-cluster/ipfs-cluster/issues/567)
  * Fix tests in `stateless` PinTracker | [ipfs-cluster/ipfs-cluster#552](https://github.com/ipfs-cluster/ipfs-cluster/issues/552) | [ipfs-cluster/ipfs-cluster#563](https://github.com/ipfs-cluster/ipfs-cluster/issues/563)

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

The release also includes most of the needed code for the [Sharding feature](https://ipfscluster.io/developer/rfcs/dag-sharding-rfc/), but it is not yet usable/enabled, pending features from go-ipfs.

The 0.5.0 release additionally includes a new experimental PinTracker implementation: the `stateless` pin tracker. The stateless pin tracker relies on the IPFS pinset and the cluster state to keep track of pins, rather than keeping an in-memory copy of the cluster pinset, thus reducing the memory usage when having huge pinsets. It can be enabled with `ipfs-cluster-service daemon --pintracker stateless`.

The last major feature is the use of a DHT as routing layer for cluster peers. This means that peers should be able to discover each others as long as they are connected to one cluster peer. This simplifies the setup requirements for starting a cluster and helps avoiding situations which make the cluster unhealthy.

This release requires a state upgrade migration. It can be performed with `ipfs-cluster-service state upgrade` or simply launching the daemon with `ipfs-cluster-service daemon --upgrade`.

#### List of changes

##### Features

  * Libp2p upgrades (up to v6) | [ipfs-cluster/ipfs-cluster#456](https://github.com/ipfs-cluster/ipfs-cluster/issues/456) | [ipfs-cluster/ipfs-cluster#482](https://github.com/ipfs-cluster/ipfs-cluster/issues/482)
  * Support `/dns` multiaddresses for `node_multiaddress` | [ipfs-cluster/ipfs-cluster#462](https://github.com/ipfs-cluster/ipfs-cluster/issues/462) | [ipfs-cluster/ipfs-cluster#463](https://github.com/ipfs-cluster/ipfs-cluster/issues/463)
  * Increase `state_sync_interval` to 10 minutes | [ipfs-cluster/ipfs-cluster#468](https://github.com/ipfs-cluster/ipfs-cluster/issues/468) | [ipfs-cluster/ipfs-cluster#469](https://github.com/ipfs-cluster/ipfs-cluster/issues/469)
  * Auto-interpret libp2p addresses in `rest/client`'s `APIAddr` configuration option | [ipfs-cluster/ipfs-cluster#498](https://github.com/ipfs-cluster/ipfs-cluster/issues/498)
  * Resolve `APIAddr` (for `/dnsaddr` usage) in `rest/client` | [ipfs-cluster/ipfs-cluster#498](https://github.com/ipfs-cluster/ipfs-cluster/issues/498)
  * Support for adding content to Cluster and sharding (sharding is disabled) | [ipfs-cluster/ipfs-cluster#484](https://github.com/ipfs-cluster/ipfs-cluster/issues/484) | [ipfs-cluster/ipfs-cluster#503](https://github.com/ipfs-cluster/ipfs-cluster/issues/503) | [ipfs-cluster/ipfs-cluster#495](https://github.com/ipfs-cluster/ipfs-cluster/issues/495) | [ipfs-cluster/ipfs-cluster#504](https://github.com/ipfs-cluster/ipfs-cluster/issues/504) | [ipfs-cluster/ipfs-cluster#509](https://github.com/ipfs-cluster/ipfs-cluster/issues/509) | [ipfs-cluster/ipfs-cluster#511](https://github.com/ipfs-cluster/ipfs-cluster/issues/511) | [ipfs-cluster/ipfs-cluster#518](https://github.com/ipfs-cluster/ipfs-cluster/issues/518)
  * `stateless` PinTracker [ipfs-cluster/ipfs-cluster#308](https://github.com/ipfs-cluster/ipfs-cluster/issues/308) | [ipfs-cluster/ipfs-cluster#460](https://github.com/ipfs-cluster/ipfs-cluster/issues/460)
  * Add `size-only=true` to `repo/stat` calls | [ipfs-cluster/ipfs-cluster#507](https://github.com/ipfs-cluster/ipfs-cluster/issues/507)
  * Enable DHT-based peer discovery and routing for cluster peers | [ipfs-cluster/ipfs-cluster#489](https://github.com/ipfs-cluster/ipfs-cluster/issues/489) | [ipfs-cluster/ipfs-cluster#508](https://github.com/ipfs-cluster/ipfs-cluster/issues/508)
  * Gx-go upgrade | [ipfs-cluster/ipfs-cluster#517](https://github.com/ipfs-cluster/ipfs-cluster/issues/517)

##### Bugfixes

  * Fix type for constants | [ipfs-cluster/ipfs-cluster#455](https://github.com/ipfs-cluster/ipfs-cluster/issues/455)
  * Gofmt fix | [ipfs-cluster/ipfs-cluster#464](https://github.com/ipfs-cluster/ipfs-cluster/issues/464)
  * Fix tests for forked repositories | [ipfs-cluster/ipfs-cluster#465](https://github.com/ipfs-cluster/ipfs-cluster/issues/465) | [ipfs-cluster/ipfs-cluster#472](https://github.com/ipfs-cluster/ipfs-cluster/issues/472)
  * Fix resolve panic on `rest/client` | [ipfs-cluster/ipfs-cluster#498](https://github.com/ipfs-cluster/ipfs-cluster/issues/498)
  * Fix remote pins stuck in error state | [ipfs-cluster/ipfs-cluster#500](https://github.com/ipfs-cluster/ipfs-cluster/issues/500) | [ipfs-cluster/ipfs-cluster#460](https://github.com/ipfs-cluster/ipfs-cluster/issues/460)
  * Fix running some tests with `-race` | [ipfs-cluster/ipfs-cluster#340](https://github.com/ipfs-cluster/ipfs-cluster/issues/340) | [ipfs-cluster/ipfs-cluster#458](https://github.com/ipfs-cluster/ipfs-cluster/issues/458)
  * Fix ipfs proxy `/add` endpoint | [ipfs-cluster/ipfs-cluster#495](https://github.com/ipfs-cluster/ipfs-cluster/issues/495) | [ipfs-cluster/ipfs-cluster#81](https://github.com/ipfs-cluster/ipfs-cluster/issues/81) | [ipfs-cluster/ipfs-cluster#505](https://github.com/ipfs-cluster/ipfs-cluster/issues/505)
  * Fix ipfs proxy not hijacking `repo/stat` | [ipfs-cluster/ipfs-cluster#466](https://github.com/ipfs-cluster/ipfs-cluster/issues/466) | [ipfs-cluster/ipfs-cluster#514](https://github.com/ipfs-cluster/ipfs-cluster/issues/514)
  * Fix some godoc comments | [ipfs-cluster/ipfs-cluster#519](https://github.com/ipfs-cluster/ipfs-cluster/issues/519)

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

The IPFS Cluster version 0.4.0 includes breaking changes and a considerable number of new features causing them. The documentation (particularly that affecting the configuration and startup of peers) has been updated accordingly in https://ipfscluster.io . Be sure to also read it if you are upgrading.

There are four main developments in this release:

* Refactorings around the `consensus` component, removing dependencies to the main component and allowing separate initialization: this has prompted to re-approach how we handle the peerset, the peer addresses and the peer's startup when using bootstrap. We have gained finer control of Raft, which has allowed us to provide a clearer configuration and a better start up procedure, specially when bootstrapping. The configuration file no longer mutates while cluster is running.
* Improvements to the `pintracker`: our pin tracker is now able to cancel ongoing pins when receiving an unpin request for the same CID, and vice-versa. It will also optimize multiple pin requests (by only queuing and triggering them once) and can now report
whether an item is pinning (a request to ipfs is ongoing) vs. pin-queued (waiting for a worker to perform the request to ipfs).
* Broadcasting of monitoring metrics using PubSub: we have added a new `monitor` implementation that uses PubSub (rather than RPC broadcasting). With the upcoming improvements to PubSub this means that we can do efficient broadcasting of metrics while at the same time not requiring peers to have RPC permissions, which is preparing the ground for collaborative clusters.
* We have launched the IPFS Cluster website: https://ipfscluster.io . We moved most of the documentation over there, expanded it and updated it.

#### List of changes

##### Features

  * Consensus refactorings | [ipfs-cluster/ipfs-cluster#398](https://github.com/ipfs-cluster/ipfs-cluster/issues/398) | [ipfs-cluster/ipfs-cluster#371](https://github.com/ipfs-cluster/ipfs-cluster/issues/371)
  * Pintracker revamp | [ipfs-cluster/ipfs-cluster#308](https://github.com/ipfs-cluster/ipfs-cluster/issues/308) | [ipfs-cluster/ipfs-cluster#383](https://github.com/ipfs-cluster/ipfs-cluster/issues/383) | [ipfs-cluster/ipfs-cluster#408](https://github.com/ipfs-cluster/ipfs-cluster/issues/408) | [ipfs-cluster/ipfs-cluster#415](https://github.com/ipfs-cluster/ipfs-cluster/issues/415) | [ipfs-cluster/ipfs-cluster#421](https://github.com/ipfs-cluster/ipfs-cluster/issues/421) | [ipfs-cluster/ipfs-cluster#427](https://github.com/ipfs-cluster/ipfs-cluster/issues/427) | [ipfs-cluster/ipfs-cluster#432](https://github.com/ipfs-cluster/ipfs-cluster/issues/432)
  * Pubsub monitoring | [ipfs-cluster/ipfs-cluster#400](https://github.com/ipfs-cluster/ipfs-cluster/issues/400)
  * Force killing cluster with double CTRL-C | [ipfs-cluster/ipfs-cluster#258](https://github.com/ipfs-cluster/ipfs-cluster/issues/258) | [ipfs-cluster/ipfs-cluster#358](https://github.com/ipfs-cluster/ipfs-cluster/issues/358)
  * 3x faster testsuite | [ipfs-cluster/ipfs-cluster#339](https://github.com/ipfs-cluster/ipfs-cluster/issues/339) | [ipfs-cluster/ipfs-cluster#350](https://github.com/ipfs-cluster/ipfs-cluster/issues/350)
  * Introduce `disable_repinning` option | [ipfs-cluster/ipfs-cluster#369](https://github.com/ipfs-cluster/ipfs-cluster/issues/369) | [ipfs-cluster/ipfs-cluster#387](https://github.com/ipfs-cluster/ipfs-cluster/issues/387)
  * Documentation moved to website and fixes | [ipfs-cluster/ipfs-cluster#390](https://github.com/ipfs-cluster/ipfs-cluster/issues/390) | [ipfs-cluster/ipfs-cluster#391](https://github.com/ipfs-cluster/ipfs-cluster/issues/391) | [ipfs-cluster/ipfs-cluster#393](https://github.com/ipfs-cluster/ipfs-cluster/issues/393) | [ipfs-cluster/ipfs-cluster#347](https://github.com/ipfs-cluster/ipfs-cluster/issues/347)
  * Run Docker container with `daemon --upgrade` by default | [ipfs-cluster/ipfs-cluster#394](https://github.com/ipfs-cluster/ipfs-cluster/issues/394)
  * Remove the `ipfs-cluster-ctl peers add` command (bootstrap should be used to add peers) | [ipfs-cluster/ipfs-cluster#397](https://github.com/ipfs-cluster/ipfs-cluster/issues/397)
  * Add tests using HTTPs endpoints | [ipfs-cluster/ipfs-cluster#191](https://github.com/ipfs-cluster/ipfs-cluster/issues/191) | [ipfs-cluster/ipfs-cluster#403](https://github.com/ipfs-cluster/ipfs-cluster/issues/403)
  * Set `refs` as default `pinning_method` and `10` as default `concurrent_pins` | [ipfs-cluster/ipfs-cluster#420](https://github.com/ipfs-cluster/ipfs-cluster/issues/420)
  * Use latest `gx` and `gx-go`. Be more verbose when installing | [ipfs-cluster/ipfs-cluster#418](https://github.com/ipfs-cluster/ipfs-cluster/issues/418)
  * Makefile: Properly retrigger builds on source change | [ipfs-cluster/ipfs-cluster#426](https://github.com/ipfs-cluster/ipfs-cluster/issues/426)
  * Improvements to StateSync() | [ipfs-cluster/ipfs-cluster#429](https://github.com/ipfs-cluster/ipfs-cluster/issues/429)
  * Rename `ipfs-cluster-data` folder to `raft` | [ipfs-cluster/ipfs-cluster#430](https://github.com/ipfs-cluster/ipfs-cluster/issues/430)
  * Officially support go 1.10 | [ipfs-cluster/ipfs-cluster#439](https://github.com/ipfs-cluster/ipfs-cluster/issues/439)
  * Update to libp2p 5.0.17 | [ipfs-cluster/ipfs-cluster#440](https://github.com/ipfs-cluster/ipfs-cluster/issues/440)

##### Bugsfixes:

  * Don't keep peers /ip*/ addresses if we know DNS addresses for them | [ipfs-cluster/ipfs-cluster#381](https://github.com/ipfs-cluster/ipfs-cluster/issues/381)
  * Running cluster with wrong configuration path gives misleading error | [ipfs-cluster/ipfs-cluster#343](https://github.com/ipfs-cluster/ipfs-cluster/issues/343) | [ipfs-cluster/ipfs-cluster#370](https://github.com/ipfs-cluster/ipfs-cluster/issues/370) | [ipfs-cluster/ipfs-cluster#373](https://github.com/ipfs-cluster/ipfs-cluster/issues/373)
  * Do not fail when running with `daemon --upgrade` and no state is present | [ipfs-cluster/ipfs-cluster#395](https://github.com/ipfs-cluster/ipfs-cluster/issues/395)
  * IPFS Proxy: handle arguments passed as part of the url | [ipfs-cluster/ipfs-cluster#380](https://github.com/ipfs-cluster/ipfs-cluster/issues/380) | [ipfs-cluster/ipfs-cluster#392](https://github.com/ipfs-cluster/ipfs-cluster/issues/392)
  * WaitForUpdates() may return before state is fully synced | [ipfs-cluster/ipfs-cluster#378](https://github.com/ipfs-cluster/ipfs-cluster/issues/378)
  * Configuration mutates no more and shadowing is no longer necessary | [ipfs-cluster/ipfs-cluster#235](https://github.com/ipfs-cluster/ipfs-cluster/issues/235)
  * Govet fixes | [ipfs-cluster/ipfs-cluster#417](https://github.com/ipfs-cluster/ipfs-cluster/issues/417)
  * Fix release changelog when having RC tags
  * Fix lock file not being removed on cluster force-kill | [ipfs-cluster/ipfs-cluster#423](https://github.com/ipfs-cluster/ipfs-cluster/issues/423) | [ipfs-cluster/ipfs-cluster#437](https://github.com/ipfs-cluster/ipfs-cluster/issues/437)
  * Fix indirect pins not being correctly parsed | [ipfs-cluster/ipfs-cluster#428](https://github.com/ipfs-cluster/ipfs-cluster/issues/428) | [ipfs-cluster/ipfs-cluster#436](https://github.com/ipfs-cluster/ipfs-cluster/issues/436)
  * Enable NAT support in libp2p host | [ipfs-cluster/ipfs-cluster#346](https://github.com/ipfs-cluster/ipfs-cluster/issues/346) | [ipfs-cluster/ipfs-cluster#441](https://github.com/ipfs-cluster/ipfs-cluster/issues/441)
  * Fix pubsub monitor not working on ARM | [ipfs-cluster/ipfs-cluster#433](https://github.com/ipfs-cluster/ipfs-cluster/issues/433) | [ipfs-cluster/ipfs-cluster#443](https://github.com/ipfs-cluster/ipfs-cluster/issues/443)

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
  * `--no-status` for `ipfs-cluster-ctl pin add/rm` allows to speed up adding and removing by not fetching the status one second afterwards. Useful for ingesting pinsets to cluster | [ipfs-cluster/ipfs-cluster#286](https://github.com/ipfs-cluster/ipfs-cluster/issues/286) | [ipfs-cluster/ipfs-cluster#329](https://github.com/ipfs-cluster/ipfs-cluster/issues/329)
  * `--wait` flag for `ipfs-cluster-ctl pin add/rm` allows to wait until a CID is fully pinned or unpinned [ipfs-cluster/ipfs-cluster#338](https://github.com/ipfs-cluster/ipfs-cluster/issues/338) | [ipfs-cluster/ipfs-cluster#348](https://github.com/ipfs-cluster/ipfs-cluster/issues/348) | [ipfs-cluster/ipfs-cluster#363](https://github.com/ipfs-cluster/ipfs-cluster/issues/363)
  * Support `refs` pinning method. Parallel pinning | [ipfs-cluster/ipfs-cluster#326](https://github.com/ipfs-cluster/ipfs-cluster/issues/326) | [ipfs-cluster/ipfs-cluster#331](https://github.com/ipfs-cluster/ipfs-cluster/issues/331)
  * Double default timeouts for `ipfs-cluster-ctl` | [ipfs-cluster/ipfs-cluster#323](https://github.com/ipfs-cluster/ipfs-cluster/issues/323) | [ipfs-cluster/ipfs-cluster#334](https://github.com/ipfs-cluster/ipfs-cluster/issues/334)
  * Better error messages during startup | [ipfs-cluster/ipfs-cluster#167](https://github.com/ipfs-cluster/ipfs-cluster/issues/167) | [ipfs-cluster/ipfs-cluster#344](https://github.com/ipfs-cluster/ipfs-cluster/issues/344) | [ipfs-cluster/ipfs-cluster#353](https://github.com/ipfs-cluster/ipfs-cluster/issues/353)
  * REST API client now provides an `IPFS()` method which returns a `go-ipfs-api` shell instance pointing to the proxy endpoint | [ipfs-cluster/ipfs-cluster#269](https://github.com/ipfs-cluster/ipfs-cluster/issues/269) | [ipfs-cluster/ipfs-cluster#356](https://github.com/ipfs-cluster/ipfs-cluster/issues/356)
  * REST http-api-over-libp2p. Server, client, `ipfs-cluster-ctl` support added | [ipfs-cluster/ipfs-cluster#305](https://github.com/ipfs-cluster/ipfs-cluster/issues/305) | [ipfs-cluster/ipfs-cluster#349](https://github.com/ipfs-cluster/ipfs-cluster/issues/349)
  * Added support for priority pins and non-recursive pins (sharding-related) | [ipfs-cluster/ipfs-cluster#341](https://github.com/ipfs-cluster/ipfs-cluster/issues/341) | [ipfs-cluster/ipfs-cluster#342](https://github.com/ipfs-cluster/ipfs-cluster/issues/342)
  * Documentation fixes | [ipfs-cluster/ipfs-cluster#328](https://github.com/ipfs-cluster/ipfs-cluster/issues/328) | [ipfs-cluster/ipfs-cluster#357](https://github.com/ipfs-cluster/ipfs-cluster/issues/357)

* Bugfixes
  * Print lock path in logs | [ipfs-cluster/ipfs-cluster#332](https://github.com/ipfs-cluster/ipfs-cluster/issues/332) | [ipfs-cluster/ipfs-cluster#333](https://github.com/ipfs-cluster/ipfs-cluster/issues/333)

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
  * Pre-built binaries panic on start | [ipfs-cluster/ipfs-cluster#320](https://github.com/ipfs-cluster/ipfs-cluster/issues/320)

---

### v0.3.3 - 2018-02-12

This release includes additional `ipfs-cluster-service state` subcommands and the connectivity graph feature.

* Features
  * `ipfs-cluster-service daemon --upgrade` allows to automatically run migrations before starting | [ipfs-cluster/ipfs-cluster#300](https://github.com/ipfs-cluster/ipfs-cluster/issues/300) | [ipfs-cluster/ipfs-cluster#307](https://github.com/ipfs-cluster/ipfs-cluster/issues/307)
  * `ipfs-cluster-service state version` reports the shared state format version | [ipfs-cluster/ipfs-cluster#298](https://github.com/ipfs-cluster/ipfs-cluster/issues/298) | [ipfs-cluster/ipfs-cluster#307](https://github.com/ipfs-cluster/ipfs-cluster/issues/307)
  * `ipfs-cluster-service health graph` generates a .dot graph file of cluster connectivity | [ipfs-cluster/ipfs-cluster#17](https://github.com/ipfs-cluster/ipfs-cluster/issues/17) | [ipfs-cluster/ipfs-cluster#291](https://github.com/ipfs-cluster/ipfs-cluster/issues/291) | [ipfs-cluster/ipfs-cluster#311](https://github.com/ipfs-cluster/ipfs-cluster/issues/311)

* Bugfixes
  * Do not upgrade state if already up to date | [ipfs-cluster/ipfs-cluster#296](https://github.com/ipfs-cluster/ipfs-cluster/issues/296) | [ipfs-cluster/ipfs-cluster#307](https://github.com/ipfs-cluster/ipfs-cluster/issues/307)
  * Fix `ipfs-cluster-service daemon` failing with `unknown allocation strategy` error | [ipfs-cluster/ipfs-cluster#314](https://github.com/ipfs-cluster/ipfs-cluster/issues/314) | [ipfs-cluster/ipfs-cluster#315](https://github.com/ipfs-cluster/ipfs-cluster/issues/315)

APIs have not changed in this release. The `/health/graph` endpoint has been added.

---

### v0.3.2 - 2018-01-25

This release includes a number of bufixes regarding the upgrade and import of state, along with two important features:
  * Commands to export and import the internal cluster state: these allow to perform easy and human-readable dumps of the shared cluster state while offline, and eventually restore it in a different peer or cluster.
  * The introduction of `replication_factor_min` and `replication_factor_max` parameters for every Pin (along with the deprecation of `replication_factor`). The defaults are specified in the configuration. For more information on the usage and behavior of these new options, check the IPFS cluster guide.

* Features
  * New `ipfs-cluster-service state export/import/cleanup` commands | [ipfs-cluster/ipfs-cluster#240](https://github.com/ipfs-cluster/ipfs-cluster/issues/240) | [ipfs-cluster/ipfs-cluster#290](https://github.com/ipfs-cluster/ipfs-cluster/issues/290)
  * New min/max replication factor control | [ipfs-cluster/ipfs-cluster#277](https://github.com/ipfs-cluster/ipfs-cluster/issues/277) | [ipfs-cluster/ipfs-cluster#292](https://github.com/ipfs-cluster/ipfs-cluster/issues/292)
  * Improved migration code | [ipfs-cluster/ipfs-cluster#283](https://github.com/ipfs-cluster/ipfs-cluster/issues/283)
  * `ipfs-cluster-service version` output simplified (see below) | [ipfs-cluster/ipfs-cluster#274](https://github.com/ipfs-cluster/ipfs-cluster/issues/274)
  * Testing improvements:
    * Added tests for Dockerfiles | [ipfs-cluster/ipfs-cluster#200](https://github.com/ipfs-cluster/ipfs-cluster/issues/200) | [ipfs-cluster/ipfs-cluster#282](https://github.com/ipfs-cluster/ipfs-cluster/issues/282)
    * Enabled Jenkins testing and made it work | [ipfs-cluster/ipfs-cluster#256](https://github.com/ipfs-cluster/ipfs-cluster/issues/256) | [ipfs-cluster/ipfs-cluster#294](https://github.com/ipfs-cluster/ipfs-cluster/issues/294)
  * Documentation improvements:
    * Guide contains more details on state upgrade procedures | [ipfs-cluster/ipfs-cluster#270](https://github.com/ipfs-cluster/ipfs-cluster/issues/270)
    * ipfs-cluster-ctl exit status are documented on the README | [ipfs-cluster/ipfs-cluster#178](https://github.com/ipfs-cluster/ipfs-cluster/issues/178)

* Bugfixes
  * Force cleanup after sharness tests | [ipfs-cluster/ipfs-cluster#181](https://github.com/ipfs-cluster/ipfs-cluster/issues/181) | [ipfs-cluster/ipfs-cluster#288](https://github.com/ipfs-cluster/ipfs-cluster/issues/288)
  * Fix state version validation on start | [ipfs-cluster/ipfs-cluster#293](https://github.com/ipfs-cluster/ipfs-cluster/issues/293)
  * Wait until last index is applied before attempting snapshot on shutdown | [ipfs-cluster/ipfs-cluster#275](https://github.com/ipfs-cluster/ipfs-cluster/issues/275)
  * Snaps from master not pushed due to bad credentials
  * Fix overpinning or underpinning of CIDs after re-join | [ipfs-cluster/ipfs-cluster#222](https://github.com/ipfs-cluster/ipfs-cluster/issues/222)
  * Fix unmarshaling state on top of an existing one | [ipfs-cluster/ipfs-cluster#297](https://github.com/ipfs-cluster/ipfs-cluster/issues/297)
  * Fix catching up on imported state | [ipfs-cluster/ipfs-cluster#297](https://github.com/ipfs-cluster/ipfs-cluster/issues/297)

These release is compatible with previous versions of ipfs-cluster on the API level, with the exception of the `ipfs-cluster-service version` command, which returns `x.x.x-shortcommit` rather than `ipfs-cluster-service version 0.3.1`. The former output is still available as `ipfs-cluster-service --version`.

The `replication_factor` option is deprecated, but still supported and will serve as a shortcut to set both `replication_factor_min` and `replication_factor_max` to the same value. This affects the configuration file, the REST API and the `ipfs-cluster-ctl pin add` command.

---

### v0.3.1 - 2017-12-11

This release includes changes around the consensus state management, so that upgrades can be performed when the internal format changes. It also comes with several features and changes to support a live deployment and integration with IPFS pin-bot, including a REST API client for Go.

* Features
 * `ipfs-cluster-service state upgrade` | [ipfs-cluster/ipfs-cluster#194](https://github.com/ipfs-cluster/ipfs-cluster/issues/194)
 * `ipfs-cluster-test` Docker image runs with `ipfs:master` | [ipfs-cluster/ipfs-cluster#155](https://github.com/ipfs-cluster/ipfs-cluster/issues/155) | [ipfs-cluster/ipfs-cluster#259](https://github.com/ipfs-cluster/ipfs-cluster/issues/259)
 * `ipfs-cluster` Docker image only runs `ipfs-cluster-service` (and not the ipfs daemon anymore) | [ipfs-cluster/ipfs-cluster#197](https://github.com/ipfs-cluster/ipfs-cluster/issues/197) | [ipfs-cluster/ipfs-cluster#155](https://github.com/ipfs-cluster/ipfs-cluster/issues/155) | [ipfs-cluster/ipfs-cluster#259](https://github.com/ipfs-cluster/ipfs-cluster/issues/259)
 * Support for DNS multiaddresses for cluster peers | [ipfs-cluster/ipfs-cluster#155](https://github.com/ipfs-cluster/ipfs-cluster/issues/155) | [ipfs-cluster/ipfs-cluster#259](https://github.com/ipfs-cluster/ipfs-cluster/issues/259)
 * Add configuration section and options for `pin_tracker` | [ipfs-cluster/ipfs-cluster#155](https://github.com/ipfs-cluster/ipfs-cluster/issues/155) | [ipfs-cluster/ipfs-cluster#259](https://github.com/ipfs-cluster/ipfs-cluster/issues/259)
 * Add `local` flag to Status, Sync, Recover endpoints which allows to run this operations only in the peer receiving the request | [ipfs-cluster/ipfs-cluster#155](https://github.com/ipfs-cluster/ipfs-cluster/issues/155) | [ipfs-cluster/ipfs-cluster#259](https://github.com/ipfs-cluster/ipfs-cluster/issues/259)
 * Add Pin names | [ipfs-cluster/ipfs-cluster#249](https://github.com/ipfs-cluster/ipfs-cluster/issues/249)
 * Add Peer names | [ipfs-cluster/ipfs-cluster#250](https://github.com/ipfs-cluster/ipfs-cluster/issues/250)
 * New REST API Client module `github.com/ipfs-cluster/ipfs-cluster/api/rest/client` allows to integrate against cluster | [ipfs-cluster/ipfs-cluster#260](https://github.com/ipfs-cluster/ipfs-cluster/issues/260) | [ipfs-cluster/ipfs-cluster#263](https://github.com/ipfs-cluster/ipfs-cluster/issues/263) | [ipfs-cluster/ipfs-cluster#266](https://github.com/ipfs-cluster/ipfs-cluster/issues/266)
 * A few rounds addressing code quality issues | [ipfs-cluster/ipfs-cluster#264](https://github.com/ipfs-cluster/ipfs-cluster/issues/264)

This release should stay backwards compatible with the previous one. Nevertheless, some REST API endpoints take the `local` flag, and matching new Go public functions have been added (`RecoverAllLocal`, `SyncAllLocal`...).

---

### v0.3.0 - 2017-11-15

This release introduces Raft 1.0.0 and incorporates deep changes to the management of the cluster peerset.

* Features
  * Upgrade Raft to 1.0.0 | [ipfs-cluster/ipfs-cluster#194](https://github.com/ipfs-cluster/ipfs-cluster/issues/194) | [ipfs-cluster/ipfs-cluster#196](https://github.com/ipfs-cluster/ipfs-cluster/issues/196)
  * Support Snaps | [ipfs-cluster/ipfs-cluster#234](https://github.com/ipfs-cluster/ipfs-cluster/issues/234) | [ipfs-cluster/ipfs-cluster#228](https://github.com/ipfs-cluster/ipfs-cluster/issues/228) | [ipfs-cluster/ipfs-cluster#232](https://github.com/ipfs-cluster/ipfs-cluster/issues/232)
  * Rotating backups for ipfs-cluster-data | [ipfs-cluster/ipfs-cluster#233](https://github.com/ipfs-cluster/ipfs-cluster/issues/233)
  * Bring documentation up to date with the code [ipfs-cluster/ipfs-cluster#223](https://github.com/ipfs-cluster/ipfs-cluster/issues/223)

Bugfixes:
  * Fix docker startup | [ipfs-cluster/ipfs-cluster#216](https://github.com/ipfs-cluster/ipfs-cluster/issues/216) | [ipfs-cluster/ipfs-cluster#217](https://github.com/ipfs-cluster/ipfs-cluster/issues/217)
  * Fix configuration save | [ipfs-cluster/ipfs-cluster#213](https://github.com/ipfs-cluster/ipfs-cluster/issues/213) | [ipfs-cluster/ipfs-cluster#214](https://github.com/ipfs-cluster/ipfs-cluster/issues/214)
  * Forward progress updates with IPFS-Proxy | [ipfs-cluster/ipfs-cluster#224](https://github.com/ipfs-cluster/ipfs-cluster/issues/224) | [ipfs-cluster/ipfs-cluster#231](https://github.com/ipfs-cluster/ipfs-cluster/issues/231)
  * Delay ipfs connect swarms on boot and safeguard against panic condition | [ipfs-cluster/ipfs-cluster#238](https://github.com/ipfs-cluster/ipfs-cluster/issues/238)
  * Multiple minor fixes | [ipfs-cluster/ipfs-cluster#236](https://github.com/ipfs-cluster/ipfs-cluster/issues/236)
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
* Note that `--bootstrap` and `--leave` flags when calling `ipfs-cluster-service` will be stored permanently in the configuration (see [ipfs-cluster/ipfs-cluster#235](https://github.com/ipfs-cluster/ipfs-cluster/issues/235)).

---

### v0.2.1 - 2017-10-26

This is a maintenance release with some important bugfixes.

* Fixes:
  * Dockerfile runs `ipfs-cluster-service` instead of `ctl` | [ipfs-cluster/ipfs-cluster#194](https://github.com/ipfs-cluster/ipfs-cluster/issues/194) | [ipfs-cluster/ipfs-cluster#196](https://github.com/ipfs-cluster/ipfs-cluster/issues/196)
  * Peers and bootstrap entries in the configuration are ignored | [ipfs-cluster/ipfs-cluster#203](https://github.com/ipfs-cluster/ipfs-cluster/issues/203) | [ipfs-cluster/ipfs-cluster#204](https://github.com/ipfs-cluster/ipfs-cluster/issues/204)
  * Informers do not work on 32-bit architectures | [ipfs-cluster/ipfs-cluster#202](https://github.com/ipfs-cluster/ipfs-cluster/issues/202) | [ipfs-cluster/ipfs-cluster#205](https://github.com/ipfs-cluster/ipfs-cluster/issues/205)
  * Replication factor entry in the configuration is ignored | [ipfs-cluster/ipfs-cluster#208](https://github.com/ipfs-cluster/ipfs-cluster/issues/208) | [ipfs-cluster/ipfs-cluster#209](https://github.com/ipfs-cluster/ipfs-cluster/issues/209)

The fix for 32-bit architectures has required a change in the `IPFSConnector` interface (`FreeSpace()` and `Reposize()` return `uint64` now). The current implementation by the `ipfshttp` module has changed accordingly.


---

### v0.2.0 - 2017-10-23

* Features:
  * Basic authentication support added to API component | [ipfs-cluster/ipfs-cluster#121](https://github.com/ipfs-cluster/ipfs-cluster/issues/121) | [ipfs-cluster/ipfs-cluster#147](https://github.com/ipfs-cluster/ipfs-cluster/issues/147) | [ipfs-cluster/ipfs-cluster#179](https://github.com/ipfs-cluster/ipfs-cluster/issues/179)
  * Copy peers to bootstrap when leaving a cluster | [ipfs-cluster/ipfs-cluster#170](https://github.com/ipfs-cluster/ipfs-cluster/issues/170) | [ipfs-cluster/ipfs-cluster#112](https://github.com/ipfs-cluster/ipfs-cluster/issues/112)
  * New configuration format | [ipfs-cluster/ipfs-cluster#162](https://github.com/ipfs-cluster/ipfs-cluster/issues/162) | [ipfs-cluster/ipfs-cluster#177](https://github.com/ipfs-cluster/ipfs-cluster/issues/177)
  * Freespace disk metric implementation. It's now the default. | [ipfs-cluster/ipfs-cluster#142](https://github.com/ipfs-cluster/ipfs-cluster/issues/142) | [ipfs-cluster/ipfs-cluster#99](https://github.com/ipfs-cluster/ipfs-cluster/issues/99)

* Fixes:
  * IPFS Connector should use only POST | [ipfs-cluster/ipfs-cluster#176](https://github.com/ipfs-cluster/ipfs-cluster/issues/176) | [ipfs-cluster/ipfs-cluster#161](https://github.com/ipfs-cluster/ipfs-cluster/issues/161)
  * `ipfs-cluster-ctl` exit status with error responses | [ipfs-cluster/ipfs-cluster#174](https://github.com/ipfs-cluster/ipfs-cluster/issues/174)
  * Sharness tests and update testing container | [ipfs-cluster/ipfs-cluster#171](https://github.com/ipfs-cluster/ipfs-cluster/issues/171)
  * Update Dockerfiles | [ipfs-cluster/ipfs-cluster#154](https://github.com/ipfs-cluster/ipfs-cluster/issues/154) | [ipfs-cluster/ipfs-cluster#185](https://github.com/ipfs-cluster/ipfs-cluster/issues/185)
  * `ipfs-cluster-service`: Do not run service with unknown subcommands | [ipfs-cluster/ipfs-cluster#186](https://github.com/ipfs-cluster/ipfs-cluster/issues/186)

This release introduces some breaking changes affecting configuration files and `go` integrations:

* Config: The old configuration format is no longer valid and cluster will fail to start from it. Configuration file needs to be re-initialized with `ipfs-cluster-service init`.
* Go: The `restapi` component has been renamed to `rest` and some of its public methods have been renamed.
* Go: Initializers (`New<Component>(...)`) for most components have changed to accept a `Config` object. Some initializers have been removed.

---

Note, when adding changelog entries, write links to issues as `@<issuenumber>` and then replace them with links with the following command:

```
sed -i -r 's/@([0-9]+)/[ipfs\/ipfs-cluster#\1](https:\/\/github.com\/ipfs\/ipfs-cluster\/issues\/\1)/g' CHANGELOG.md
```
