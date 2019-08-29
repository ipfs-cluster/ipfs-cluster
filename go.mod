module github.com/ipfs/ipfs-cluster

require (
	contrib.go.opencensus.io/exporter/jaeger v0.1.0
	contrib.go.opencensus.io/exporter/prometheus v0.1.0
	github.com/AndreasBriese/bbloom v0.0.0-20190825152654-46b345b51c96 // indirect
	github.com/blang/semver v3.5.1+incompatible
	github.com/btcsuite/btcd v0.0.0-20190824003749-130ea5bddde3 // indirect
	github.com/cenkalti/backoff v2.2.1+incompatible // indirect
	github.com/dgraph-io/badger v1.6.0
	github.com/dustin/go-humanize v1.0.0
	github.com/gogo/protobuf v1.2.1
	github.com/golang/protobuf v1.3.2
	github.com/google/go-cmp v0.3.1 // indirect
	github.com/google/uuid v1.1.1
	github.com/gopherjs/gopherjs v0.0.0-20190812055157-5d271430af9f // indirect
	github.com/gorilla/handlers v1.4.2
	github.com/gorilla/mux v1.7.3
	github.com/gorilla/websocket v1.4.1 // indirect
	github.com/hashicorp/go-hclog v0.9.2
	github.com/hashicorp/golang-lru v0.5.3 // indirect
	github.com/hashicorp/raft v1.1.0
	github.com/hashicorp/raft-boltdb v0.0.0-20190605210249-ef2e128ed477
	github.com/hsanjuan/ipfs-lite v0.1.4
	github.com/imdario/mergo v0.3.7
	github.com/ipfs/go-bitswap v0.1.8 // indirect
	github.com/ipfs/go-block-format v0.0.2
	github.com/ipfs/go-blockservice v0.1.2 // indirect
	github.com/ipfs/go-cid v0.0.3
	github.com/ipfs/go-datastore v0.1.0
	github.com/ipfs/go-ds-badger v0.0.5
	github.com/ipfs/go-ds-crdt v0.1.4
	github.com/ipfs/go-fs-lock v0.0.1
	github.com/ipfs/go-ipfs-api v0.0.1
	github.com/ipfs/go-ipfs-blockstore v0.1.0 // indirect
	github.com/ipfs/go-ipfs-chunker v0.0.1
	github.com/ipfs/go-ipfs-ds-help v0.0.1
	github.com/ipfs/go-ipfs-files v0.0.3
	github.com/ipfs/go-ipfs-posinfo v0.0.1
	github.com/ipfs/go-ipfs-provider v0.2.1 // indirect
	github.com/ipfs/go-ipfs-util v0.0.1
	github.com/ipfs/go-ipld-cbor v0.0.3
	github.com/ipfs/go-ipld-format v0.0.2
	github.com/ipfs/go-log v0.0.1
	github.com/ipfs/go-merkledag v0.2.3
	github.com/ipfs/go-mfs v0.1.1
	github.com/ipfs/go-path v0.0.7
	github.com/ipfs/go-unixfs v0.2.1
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/lanzafame/go-libp2p-ocgorpc v0.1.1
	github.com/libp2p/go-libp2p v0.3.1
	github.com/libp2p/go-libp2p-blankhost v0.1.4 // indirect
	github.com/libp2p/go-libp2p-connmgr v0.1.0
	github.com/libp2p/go-libp2p-consensus v0.0.1
	github.com/libp2p/go-libp2p-core v0.2.2
	github.com/libp2p/go-libp2p-crypto v0.1.0
	github.com/libp2p/go-libp2p-gorpc v0.1.0
	github.com/libp2p/go-libp2p-gostream v0.1.2
	github.com/libp2p/go-libp2p-host v0.1.0
	github.com/libp2p/go-libp2p-http v0.1.3
	github.com/libp2p/go-libp2p-interface-pnet v0.1.0
	github.com/libp2p/go-libp2p-kad-dht v0.1.1
	github.com/libp2p/go-libp2p-peer v0.2.0
	github.com/libp2p/go-libp2p-peerstore v0.1.3
	github.com/libp2p/go-libp2p-pnet v0.1.0
	github.com/libp2p/go-libp2p-protocol v0.1.0
	github.com/libp2p/go-libp2p-pubsub v0.1.1
	github.com/libp2p/go-libp2p-raft v0.1.2
	github.com/libp2p/go-libp2p-record v0.1.1 // indirect
	github.com/libp2p/go-ws-transport v0.1.0
	github.com/mattn/go-isatty v0.0.9 // indirect
	github.com/miekg/dns v1.1.16 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.1 // indirect
	github.com/multiformats/go-multiaddr v0.0.4
	github.com/multiformats/go-multiaddr-dns v0.0.3
	github.com/multiformats/go-multiaddr-net v0.0.1
	github.com/multiformats/go-multicodec v0.1.6
	github.com/multiformats/go-multihash v0.0.7
	github.com/onsi/ginkgo v1.9.0 // indirect
	github.com/onsi/gomega v1.6.0 // indirect
	github.com/pkg/errors v0.8.1
	github.com/polydawn/refmt v0.0.0-20190807091052-3d65705ee9f1 // indirect
	github.com/prometheus/client_golang v1.1.0
	github.com/prometheus/client_model v0.0.0-20190812154241-14fe0d1b01d4 // indirect
	github.com/prometheus/procfs v0.0.4 // indirect
	github.com/rs/cors v1.6.0
	github.com/smartystreets/assertions v1.0.1 // indirect
	github.com/smartystreets/goconvey v0.0.0-20190731233626-505e41936337 // indirect
	github.com/stretchr/testify v1.4.0 // indirect
	github.com/ugorji/go/codec v1.1.7
	github.com/urfave/cli v1.20.0
	github.com/whyrusleeping/mdns v0.0.0-20190826153040-b9b60ed33aa9 // indirect
	github.com/zenground0/go-dot v0.0.0-20180912213407-94a425d4984e
	go.opencensus.io v0.22.0
	golang.org/x/crypto v0.0.0-20190820162420-60c769a6c586 // indirect
	golang.org/x/exp v0.0.0-20190731235908-ec7cb31e5a56 // indirect
	golang.org/x/image v0.0.0-20190828090100-23ea20f87cfc // indirect
	golang.org/x/net v0.0.0-20190827160401-ba9fcec4b297 // indirect
	golang.org/x/sys v0.0.0-20190826190057-c7b8b68b1456 // indirect
	golang.org/x/xerrors v0.0.0-20190717185122-a985d3407aa7 // indirect
	gonum.org/v1/gonum v0.0.0-20190704103327-70ddf0df3d53
	gonum.org/v1/plot v0.0.0-20190615073203-9aa86143727f
	google.golang.org/api v0.9.0 // indirect
	google.golang.org/genproto v0.0.0-20190819201941-24fa4b261c55 // indirect
	google.golang.org/grpc v1.23.0 // indirect
)
