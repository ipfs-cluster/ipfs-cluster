module github.com/ipfs/ipfs-cluster

require (
	contrib.go.opencensus.io/exporter/jaeger v0.1.0
	contrib.go.opencensus.io/exporter/prometheus v0.1.0
	github.com/AndreasBriese/bbloom v0.0.0-20190306092124-e2d15f34fcf9 // indirect
	github.com/ajstarks/svgo v0.0.0-20181006003313-6ce6a3bcf6cd // indirect
	github.com/armon/go-metrics v0.0.0-20190430140413-ec5e00d3c878 // indirect
	github.com/blang/semver v3.5.1+incompatible
	github.com/boltdb/bolt v1.3.1 // indirect
	github.com/btcsuite/btcd v0.0.0-20190427004231-96897255fd17 // indirect
	github.com/coreos/go-semver v0.3.0 // indirect
	github.com/dgryski/go-farm v0.0.0-20190423205320-6a90982ecee2 // indirect
	github.com/dustin/go-humanize v1.0.0
	github.com/fogleman/gg v1.3.0 // indirect
	github.com/gogo/protobuf v1.2.1
	github.com/golang/protobuf v1.3.1
	github.com/google/go-cmp v0.3.0 // indirect
	github.com/google/uuid v1.1.1
	github.com/gopherjs/gopherjs v0.0.0-20190430165422-3e4dfb77656c // indirect
	github.com/gorilla/mux v1.7.2
	github.com/hashicorp/go-msgpack v0.5.5 // indirect
	github.com/hashicorp/go-uuid v1.0.1 // indirect
	github.com/hashicorp/raft v1.0.1
	github.com/hashicorp/raft-boltdb v0.0.0-20171010151810-6e5ba93211ea
	github.com/hsanjuan/go-libp2p-gostream v0.0.31
	github.com/hsanjuan/go-libp2p-http v0.0.2
	github.com/hsanjuan/ipfs-lite v0.0.3
	github.com/ipfs/go-bitswap v0.0.5 // indirect
	github.com/ipfs/go-block-format v0.0.2
	github.com/ipfs/go-cid v0.0.2
	github.com/ipfs/go-datastore v0.0.5
	github.com/ipfs/go-ds-badger v0.0.3
	github.com/ipfs/go-ds-crdt v0.0.7
	github.com/ipfs/go-fs-lock v0.0.1
	github.com/ipfs/go-ipfs-api v0.0.1
	github.com/ipfs/go-ipfs-blockstore v0.0.1
	github.com/ipfs/go-ipfs-chunker v0.0.1
	github.com/ipfs/go-ipfs-config v0.0.3 // indirect
	github.com/ipfs/go-ipfs-ds-help v0.0.1
	github.com/ipfs/go-ipfs-files v0.0.3
	github.com/ipfs/go-ipfs-posinfo v0.0.1
	github.com/ipfs/go-ipfs-util v0.0.1
	github.com/ipfs/go-ipld-cbor v0.0.2
	github.com/ipfs/go-ipld-format v0.0.2
	github.com/ipfs/go-log v0.0.1
	github.com/ipfs/go-merkledag v0.0.3
	github.com/ipfs/go-mfs v0.0.7
	github.com/ipfs/go-path v0.0.3
	github.com/ipfs/go-unixfs v0.0.5
	github.com/jbenet/goprocess v0.1.3 // indirect
	github.com/jung-kurt/gofpdf v1.4.1 // indirect
	github.com/kelseyhightower/envconfig v1.3.0
	github.com/lanzafame/go-libp2p-ocgorpc v0.0.2
	github.com/libp2p/go-libp2p v0.0.23
	github.com/libp2p/go-libp2p-consensus v0.0.1
	github.com/libp2p/go-libp2p-crypto v0.0.2
	github.com/libp2p/go-libp2p-gorpc v0.0.2
	github.com/libp2p/go-libp2p-host v0.0.3
	github.com/libp2p/go-libp2p-interface-pnet v0.0.1
	github.com/libp2p/go-libp2p-kad-dht v0.0.10
	github.com/libp2p/go-libp2p-peer v0.1.1
	github.com/libp2p/go-libp2p-peerstore v0.0.5
	github.com/libp2p/go-libp2p-pnet v0.0.1
	github.com/libp2p/go-libp2p-protocol v0.0.1
	github.com/libp2p/go-libp2p-pubsub v0.0.3
	github.com/libp2p/go-libp2p-raft v0.0.2
	github.com/libp2p/go-maddr-filter v0.0.3 // indirect
	github.com/libp2p/go-mplex v0.0.2 // indirect
	github.com/libp2p/go-ws-transport v0.0.2
	github.com/multiformats/go-multiaddr v0.0.2
	github.com/multiformats/go-multiaddr-dns v0.0.2
	github.com/multiformats/go-multiaddr-net v0.0.1
	github.com/multiformats/go-multicodec v0.1.6
	github.com/multiformats/go-multihash v0.0.5
	github.com/onsi/ginkgo v1.8.0 // indirect
	github.com/onsi/gomega v1.5.0 // indirect
	github.com/pkg/errors v0.8.1
	github.com/polydawn/refmt v0.0.0-20190408063855-01bf1e26dd14 // indirect
	github.com/prometheus/client_golang v0.9.3
	github.com/prometheus/procfs v0.0.0-20190516194456-169873baca24 // indirect
	github.com/rs/cors v1.6.0
	github.com/smartystreets/assertions v0.0.0-20190401211740-f487f9de1cd3 // indirect
	github.com/smartystreets/goconvey v0.0.0-20190330032615-68dc04aab96a // indirect
	github.com/ugorji/go v1.1.4
	github.com/urfave/cli v1.20.0
	github.com/warpfork/go-wish v0.0.0-20190328234359-8b3e70f8e830 // indirect
	github.com/zenground0/go-dot v0.0.0-20180912213407-94a425d4984e
	go.opencensus.io v0.21.0
	go4.org v0.0.0-20190313082347-94abd6928b1d // indirect
	golang.org/x/crypto v0.0.0-20190513172903-22d7a77e9e5f // indirect
	golang.org/x/exp v0.0.0-20190510132918-efd6b22b2522 // indirect
	golang.org/x/image v0.0.0-20190516052701-61b8692d9a5c // indirect
	golang.org/x/net v0.0.0-20190514140710-3ec191127204 // indirect
	golang.org/x/sync v0.0.0-20190423024810-112230192c58 // indirect
	golang.org/x/sys v0.0.0-20190516110030-61b9204099cb // indirect
	golang.org/x/text v0.3.2 // indirect
	golang.org/x/xerrors v0.0.0-20190513163551-3ee3066db522 // indirect
	gonum.org/v1/gonum v0.0.0-20190517105223-d76380b39392
	gonum.org/v1/netlib v0.0.0-20190331212654-76723241ea4e // indirect
	gonum.org/v1/plot v0.0.0-20190515093506-e2840ee46a6b
	google.golang.org/api v0.5.0 // indirect
	google.golang.org/genproto v0.0.0-20190516172635-bb713bdc0e52 // indirect
)
