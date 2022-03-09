module github.com/ipfs/ipfs-cluster

require (
	contrib.go.opencensus.io/exporter/jaeger v0.2.1
	contrib.go.opencensus.io/exporter/prometheus v0.4.0
	github.com/blang/semver v3.5.1+incompatible
	github.com/dgraph-io/badger v1.6.2
	github.com/dustin/go-humanize v1.0.0
	github.com/google/uuid v1.3.0
	github.com/gorilla/handlers v1.5.1
	github.com/gorilla/mux v1.8.0
	github.com/hashicorp/go-hclog v1.1.0
	github.com/hashicorp/raft v1.1.1
	github.com/hashicorp/raft-boltdb v0.0.0-20190605210249-ef2e128ed477
	github.com/hsanjuan/ipfs-lite v1.2.0
	github.com/imdario/mergo v0.3.12
	github.com/ipfs/go-block-format v0.0.3
	github.com/ipfs/go-cid v0.1.0
	github.com/ipfs/go-datastore v0.5.1
	github.com/ipfs/go-ds-badger v0.3.0
	github.com/ipfs/go-ds-crdt v0.3.2
	github.com/ipfs/go-ds-leveldb v0.5.0
	github.com/ipfs/go-fs-lock v0.0.7
	github.com/ipfs/go-ipfs-api v0.3.0
	github.com/ipfs/go-ipfs-chunker v0.0.5
	github.com/ipfs/go-ipfs-cmds v0.6.0
	github.com/ipfs/go-ipfs-config v0.19.0
	github.com/ipfs/go-ipfs-ds-help v1.1.0
	github.com/ipfs/go-ipfs-files v0.1.1
	github.com/ipfs/go-ipfs-pinner v0.2.1
	github.com/ipfs/go-ipfs-posinfo v0.0.1
	github.com/ipfs/go-ipld-cbor v0.0.6
	github.com/ipfs/go-ipld-format v0.2.0
	github.com/ipfs/go-ipns v0.1.2
	github.com/ipfs/go-log/v2 v2.5.0
	github.com/ipfs/go-merkledag v0.5.1
	github.com/ipfs/go-mfs v0.1.3-0.20210507195338-96fbfa122164
	github.com/ipfs/go-path v0.2.2
	github.com/ipfs/go-unixfs v0.3.1
	github.com/ipld/go-car v0.3.3
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/kishansagathiya/go-dot v0.1.0
	github.com/lanzafame/go-libp2p-ocgorpc v0.1.1
	github.com/libp2p/go-libp2p v0.17.0
	github.com/libp2p/go-libp2p-connmgr v0.2.4
	github.com/libp2p/go-libp2p-consensus v0.0.1
	github.com/libp2p/go-libp2p-core v0.13.0
	github.com/libp2p/go-libp2p-gorpc v0.1.4
	github.com/libp2p/go-libp2p-gostream v0.3.1
	github.com/libp2p/go-libp2p-http v0.2.1
	github.com/libp2p/go-libp2p-kad-dht v0.15.0
	github.com/libp2p/go-libp2p-noise v0.3.0
	github.com/libp2p/go-libp2p-peerstore v0.6.0
	github.com/libp2p/go-libp2p-pubsub v0.6.1
	github.com/libp2p/go-libp2p-quic-transport v0.15.2
	github.com/libp2p/go-libp2p-raft v0.1.8
	github.com/libp2p/go-libp2p-record v0.1.3
	github.com/libp2p/go-libp2p-tls v0.3.1
	github.com/libp2p/go-tcp-transport v0.4.0
	github.com/libp2p/go-ws-transport v0.5.0
	github.com/multiformats/go-multiaddr v0.5.0
	github.com/multiformats/go-multiaddr-dns v0.3.1
	github.com/multiformats/go-multihash v0.1.0
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.12.1
	github.com/rs/cors v1.8.2
	github.com/syndtr/goleveldb v1.0.0
	github.com/tv42/httpunix v0.0.0-20150427012821-b75d8614f926
	github.com/ugorji/go/codec v1.2.7
	github.com/urfave/cli v1.22.5
	github.com/urfave/cli/v2 v2.3.0
	go.opencensus.io v0.23.0
	go.uber.org/multierr v1.8.0
	golang.org/x/crypto v0.0.0-20210813211128-0a44fdfbc16e
	google.golang.org/protobuf v1.27.1
)

require (
	github.com/AndreasBriese/bbloom v0.0.0-20190825152654-46b345b51c96 // indirect
	github.com/alecthomas/units v0.0.0-20210927113745-59d0afb8317a // indirect
	github.com/armon/go-metrics v0.0.0-20190430140413-ec5e00d3c878 // indirect
	github.com/benbjohnson/clock v1.1.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/boltdb/bolt v1.3.1 // indirect
	github.com/btcsuite/btcd v0.22.0-beta // indirect
	github.com/cenkalti/backoff v2.2.1+incompatible // indirect
	github.com/cespare/xxhash v1.1.0 // indirect
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/cheekybits/genny v1.0.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.0-20190314233015-f79a8a8ca69d // indirect
	github.com/crackcomm/go-gitignore v0.0.0-20170627025303-887ab5e44cc3 // indirect
	github.com/cskr/pubsub v1.0.2 // indirect
	github.com/davidlazar/go-crypto v0.0.0-20200604182044-b73af7476f6c // indirect
	github.com/dgraph-io/ristretto v0.0.2 // indirect
	github.com/fatih/color v1.7.0 // indirect
	github.com/felixge/httpsnoop v1.0.1 // indirect
	github.com/flynn/noise v1.0.0 // indirect
	github.com/francoispqt/gojay v1.2.13 // indirect
	github.com/fsnotify/fsnotify v1.4.9 // indirect
	github.com/go-kit/log v0.1.0 // indirect
	github.com/go-logfmt/logfmt v0.5.0 // indirect
	github.com/go-task/slim-sprig v0.0.0-20210107165309-348f09dbbbc0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/golang/snappy v0.0.0-20180518054509-2e65f85255db // indirect
	github.com/google/gopacket v1.1.19 // indirect
	github.com/gorilla/websocket v1.4.2 // indirect
	github.com/hashicorp/errwrap v1.0.0 // indirect
	github.com/hashicorp/go-immutable-radix v1.0.0 // indirect
	github.com/hashicorp/go-msgpack v0.5.5 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/huin/goupnp v1.0.2 // indirect
	github.com/ipfs/bbloom v0.0.4 // indirect
	github.com/ipfs/go-bitfield v1.0.0 // indirect
	github.com/ipfs/go-bitswap v0.5.1 // indirect
	github.com/ipfs/go-blockservice v0.2.1 // indirect
	github.com/ipfs/go-cidutil v0.0.2 // indirect
	github.com/ipfs/go-fetcher v1.6.1 // indirect
	github.com/ipfs/go-ipfs-blockstore v1.1.2 // indirect
	github.com/ipfs/go-ipfs-delay v0.0.1 // indirect
	github.com/ipfs/go-ipfs-exchange-interface v0.1.0 // indirect
	github.com/ipfs/go-ipfs-exchange-offline v0.1.1 // indirect
	github.com/ipfs/go-ipfs-pq v0.0.2 // indirect
	github.com/ipfs/go-ipfs-provider v0.7.1 // indirect
	github.com/ipfs/go-ipfs-util v0.0.2 // indirect
	github.com/ipfs/go-ipld-legacy v0.1.0 // indirect
	github.com/ipfs/go-log v1.0.5 // indirect
	github.com/ipfs/go-metrics-interface v0.0.1 // indirect
	github.com/ipfs/go-peertaskqueue v0.7.0 // indirect
	github.com/ipfs/go-verifcid v0.0.1 // indirect
	github.com/ipfs/interface-go-ipfs-core v0.4.0 // indirect
	github.com/ipld/go-codec-dagpb v1.3.0 // indirect
	github.com/ipld/go-ipld-prime v0.14.3-0.20211207234443-319145880958 // indirect
	github.com/jackpal/go-nat-pmp v1.0.2 // indirect
	github.com/jbenet/go-temp-err-catcher v0.1.0 // indirect
	github.com/jbenet/goprocess v0.1.4 // indirect
	github.com/klauspost/compress v1.11.7 // indirect
	github.com/klauspost/cpuid/v2 v2.0.9 // indirect
	github.com/koron/go-ssdp v0.0.2 // indirect
	github.com/libp2p/go-addr-util v0.1.0 // indirect
	github.com/libp2p/go-buffer-pool v0.0.2 // indirect
	github.com/libp2p/go-cidranger v1.1.0 // indirect
	github.com/libp2p/go-conn-security-multistream v0.3.0 // indirect
	github.com/libp2p/go-eventbus v0.2.1 // indirect
	github.com/libp2p/go-flow-metrics v0.0.3 // indirect
	github.com/libp2p/go-libp2p-asn-util v0.1.0 // indirect
	github.com/libp2p/go-libp2p-autonat v0.7.0 // indirect
	github.com/libp2p/go-libp2p-blankhost v0.3.0 // indirect
	github.com/libp2p/go-libp2p-discovery v0.6.0 // indirect
	github.com/libp2p/go-libp2p-kbucket v0.4.7 // indirect
	github.com/libp2p/go-libp2p-loggables v0.1.0 // indirect
	github.com/libp2p/go-libp2p-mplex v0.4.1 // indirect
	github.com/libp2p/go-libp2p-nat v0.1.0 // indirect
	github.com/libp2p/go-libp2p-pnet v0.2.0 // indirect
	github.com/libp2p/go-libp2p-routing-helpers v0.2.3 // indirect
	github.com/libp2p/go-libp2p-swarm v0.9.0 // indirect
	github.com/libp2p/go-libp2p-transport-upgrader v0.6.0 // indirect
	github.com/libp2p/go-libp2p-yamux v0.7.0 // indirect
	github.com/libp2p/go-maddr-filter v0.1.0 // indirect
	github.com/libp2p/go-mplex v0.3.0 // indirect
	github.com/libp2p/go-msgio v0.1.0 // indirect
	github.com/libp2p/go-nat v0.1.0 // indirect
	github.com/libp2p/go-netroute v0.1.6 // indirect
	github.com/libp2p/go-openssl v0.0.7 // indirect
	github.com/libp2p/go-reuseport v0.1.0 // indirect
	github.com/libp2p/go-reuseport-transport v0.1.0 // indirect
	github.com/libp2p/go-sockaddr v0.1.1 // indirect
	github.com/libp2p/go-stream-muxer-multistream v0.3.0 // indirect
	github.com/libp2p/go-yamux/v2 v2.3.0 // indirect
	github.com/libp2p/zeroconf/v2 v2.1.1 // indirect
	github.com/lucas-clemente/quic-go v0.24.0 // indirect
	github.com/marten-seemann/qtls-go1-16 v0.1.4 // indirect
	github.com/marten-seemann/qtls-go1-17 v0.1.0 // indirect
	github.com/marten-seemann/tcp v0.0.0-20210406111302-dfbc87cc63fd // indirect
	github.com/mattn/go-colorable v0.1.4 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.1 // indirect
	github.com/miekg/dns v1.1.43 // indirect
	github.com/mikioh/tcpinfo v0.0.0-20190314235526-30a79bb1804b // indirect
	github.com/mikioh/tcpopt v0.0.0-20190314235656-172688c1accc // indirect
	github.com/minio/blake2b-simd v0.0.0-20160723061019-3f5f724cb5b1 // indirect
	github.com/minio/sha256-simd v1.0.0 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mr-tron/base58 v1.2.0 // indirect
	github.com/multiformats/go-base32 v0.0.3 // indirect
	github.com/multiformats/go-base36 v0.1.0 // indirect
	github.com/multiformats/go-multiaddr-fmt v0.1.0 // indirect
	github.com/multiformats/go-multibase v0.0.3 // indirect
	github.com/multiformats/go-multicodec v0.3.0 // indirect
	github.com/multiformats/go-multistream v0.2.2 // indirect
	github.com/multiformats/go-varint v0.0.6 // indirect
	github.com/nxadm/tail v1.4.8 // indirect
	github.com/onsi/ginkgo v1.16.4 // indirect
	github.com/opentracing/opentracing-go v1.2.0 // indirect
	github.com/polydawn/refmt v0.0.0-20201211092308-30ac6d18308e // indirect
	github.com/prometheus/client_model v0.2.0 // indirect
	github.com/prometheus/common v0.32.1 // indirect
	github.com/prometheus/procfs v0.7.3 // indirect
	github.com/prometheus/statsd_exporter v0.21.0 // indirect
	github.com/russross/blackfriday/v2 v2.0.1 // indirect
	github.com/shurcooL/sanitized_anchor_name v1.0.0 // indirect
	github.com/spacemonkeygo/spacelog v0.0.0-20180420211403-2296661a0572 // indirect
	github.com/spaolacci/murmur3 v1.1.0 // indirect
	github.com/uber/jaeger-client-go v2.25.0+incompatible // indirect
	github.com/whyrusleeping/cbor-gen v0.0.0-20200123233031-1cdf64d27158 // indirect
	github.com/whyrusleeping/chunker v0.0.0-20181014151217-fe64bd25879f // indirect
	github.com/whyrusleeping/go-keyspace v0.0.0-20160322163242-5b898ac5add1 // indirect
	github.com/whyrusleeping/multiaddr-filter v0.0.0-20160516205228-e903e4adabd7 // indirect
	github.com/whyrusleeping/tar-utils v0.0.0-20180509141711-8c6c8ba81d5c // indirect
	github.com/whyrusleeping/timecache v0.0.0-20160911033111-cfcb2f1abfee // indirect
	go.uber.org/atomic v1.9.0 // indirect
	go.uber.org/zap v1.19.1 // indirect
	go4.org v0.0.0-20200411211856-f5505b9728dd // indirect
	golang.org/x/mod v0.4.2 // indirect
	golang.org/x/net v0.0.0-20210813160813-60bc85c4be6d // indirect
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c // indirect
	golang.org/x/sys v0.0.0-20220114195835-da31bd327af9 // indirect
	golang.org/x/tools v0.1.5 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	google.golang.org/api v0.30.0 // indirect
	google.golang.org/genproto v0.0.0-20200825200019-8632dd797987 // indirect
	google.golang.org/grpc v1.40.0 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	lukechampine.com/blake3 v1.1.6 // indirect
)

go 1.17
