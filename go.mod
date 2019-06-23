module github.com/ipfs/ipfs-cluster

require (
	contrib.go.opencensus.io/exporter/jaeger v0.1.0
	contrib.go.opencensus.io/exporter/prometheus v0.1.0
	github.com/AndreasBriese/bbloom v0.0.0-20190306092124-e2d15f34fcf9 // indirect
	github.com/blang/semver v3.5.1+incompatible
	github.com/btcsuite/btcd v0.0.0-20190614013741-962a206e94e9 // indirect
	github.com/dgraph-io/badger/v2 v2.0.0-rc2
	github.com/dgryski/go-farm v0.0.0-20190423205320-6a90982ecee2 // indirect
	github.com/dustin/go-humanize v1.0.0
	github.com/gogo/protobuf v1.2.1
	github.com/golang/protobuf v1.3.1
	github.com/google/uuid v1.1.1
	github.com/gorilla/mux v1.7.2
	github.com/hashicorp/go-hclog v0.9.2
	github.com/hashicorp/raft v1.1.0
	github.com/hashicorp/raft-boltdb v0.0.0-20190605210249-ef2e128ed477
	github.com/hsanjuan/ipfs-lite v0.1.3
	github.com/imdario/mergo v0.3.7
	github.com/ipfs/go-block-format v0.0.2
	github.com/ipfs/go-cid v0.0.2
	github.com/ipfs/go-datastore v0.0.5
	github.com/ipfs/go-ds-badger v0.0.4
	github.com/ipfs/go-ds-crdt v0.1.0
	github.com/ipfs/go-fs-lock v0.0.1
	github.com/ipfs/go-ipfs-api v0.0.1
	github.com/ipfs/go-ipfs-chunker v0.0.1
	github.com/ipfs/go-ipfs-ds-help v0.0.1
	github.com/ipfs/go-ipfs-files v0.0.3
	github.com/ipfs/go-ipfs-posinfo v0.0.1
	github.com/ipfs/go-ipfs-util v0.0.1
	github.com/ipfs/go-ipld-cbor v0.0.2
	github.com/ipfs/go-ipld-format v0.0.2
	github.com/ipfs/go-log v0.0.1
	github.com/ipfs/go-merkledag v0.1.0
	github.com/ipfs/go-mfs v0.1.1
	github.com/ipfs/go-path v0.0.7
	github.com/ipfs/go-unixfs v0.1.0
	github.com/jung-kurt/gofpdf v1.4.2 // indirect
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/lanzafame/go-libp2p-ocgorpc v0.1.0
	github.com/libp2p/go-libp2p v0.1.1
	github.com/libp2p/go-libp2p-connmgr v0.1.0
	github.com/libp2p/go-libp2p-consensus v0.0.1
	github.com/libp2p/go-libp2p-core v0.0.4
	github.com/libp2p/go-libp2p-crypto v0.1.0
	github.com/libp2p/go-libp2p-gorpc v0.1.0
	github.com/libp2p/go-libp2p-gostream v0.1.1
	github.com/libp2p/go-libp2p-host v0.1.0
	github.com/libp2p/go-libp2p-http v0.1.2
	github.com/libp2p/go-libp2p-interface-pnet v0.1.0
	github.com/libp2p/go-libp2p-kad-dht v0.1.0
	github.com/libp2p/go-libp2p-metrics v0.1.0 // indirect
	github.com/libp2p/go-libp2p-peer v0.2.0
	github.com/libp2p/go-libp2p-peerstore v0.1.2-0.20190621130618-cfa9bb890c1a
	github.com/libp2p/go-libp2p-pnet v0.1.0
	github.com/libp2p/go-libp2p-protocol v0.1.0
	github.com/libp2p/go-libp2p-pubsub v0.1.0
	github.com/libp2p/go-libp2p-raft v0.1.1
	github.com/libp2p/go-ws-transport v0.1.0
	github.com/multiformats/go-multiaddr v0.0.4
	github.com/multiformats/go-multiaddr-dns v0.0.2
	github.com/multiformats/go-multiaddr-net v0.0.1
	github.com/multiformats/go-multicodec v0.1.6
	github.com/multiformats/go-multihash v0.0.6
	github.com/pkg/errors v0.8.1
	github.com/prometheus/client_golang v1.0.0
	github.com/rs/cors v1.6.0
	github.com/ugorji/go/codec v1.1.5-pre
	github.com/urfave/cli v1.20.0
	github.com/zenground0/go-dot v0.0.0-20180912213407-94a425d4984e
	go.opencensus.io v0.22.0
	golang.org/x/crypto v0.0.0-20190621222207-cc06ce4a13d4 // indirect
	golang.org/x/exp v0.0.0-20190510132918-efd6b22b2522 // indirect
	golang.org/x/image v0.0.0-20190622003408-7e034cad6442 // indirect
	golang.org/x/net v0.0.0-20190620200207-3b0461eec859 // indirect
	golang.org/x/sys v0.0.0-20190621203818-d432491b9138 // indirect
	gonum.org/v1/gonum v0.0.0-20190608115022-c5f01565d866
	gonum.org/v1/plot v0.0.0-20190515093506-e2840ee46a6b
)

replace github.com/libp2p/go-libp2p-core => github.com/libp2p/go-libp2p-core v0.0.3
