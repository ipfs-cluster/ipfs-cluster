package ipfscluster

import (
	"context"
	"encoding/hex"

	ds "github.com/ipfs/go-datastore"
	namespace "github.com/ipfs/go-datastore/namespace"
	ipns "github.com/ipfs/go-ipns"
	config "github.com/ipfs-cluster/ipfs-cluster/config"
	libp2p "github.com/libp2p/go-libp2p"
	connmgr "github.com/libp2p/go-libp2p-connmgr"
	crypto "github.com/libp2p/go-libp2p-core/crypto"
	host "github.com/libp2p/go-libp2p-core/host"
	network "github.com/libp2p/go-libp2p-core/network"
	corepnet "github.com/libp2p/go-libp2p-core/pnet"
	routing "github.com/libp2p/go-libp2p-core/routing"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	dual "github.com/libp2p/go-libp2p-kad-dht/dual"
	noise "github.com/libp2p/go-libp2p-noise"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	libp2pquic "github.com/libp2p/go-libp2p-quic-transport"
	record "github.com/libp2p/go-libp2p-record"
	libp2ptls "github.com/libp2p/go-libp2p-tls"
	identify "github.com/libp2p/go-libp2p/p2p/protocol/identify"
	tcp "github.com/libp2p/go-tcp-transport"
	websocket "github.com/libp2p/go-ws-transport"
)

const dhtNamespace = "dht"

var _ = libp2pquic.NewTransport

func init() {
	// Cluster peers should advertise their public IPs as soon as they
	// learn about them. Default for this is 4, which prevents clusters
	// with less than 4 peers to advertise an external address they know
	// of, therefore they cannot be remembered by other peers asap. This
	// affects dockerized setups mostly. This may announce non-dialable
	// NATed addresses too eagerly, but they should progressively be
	// cleaned up.
	identify.ActivationThresh = 1
}

// NewClusterHost creates a fully-featured libp2p Host with the options from
// the provided cluster configuration. Using that host, it creates pubsub and
// a DHT instances (persisting to the given datastore), for shared use by all
// cluster components. The returned host uses the DHT for routing. Relay and
// NATService are additionally setup for this host.
func NewClusterHost(
	ctx context.Context,
	ident *config.Identity,
	cfg *Config,
	ds ds.Datastore,
) (host.Host, *pubsub.PubSub, *dual.DHT, error) {

	// Set the default dial timeout for all libp2p connections.  It is not
	// very good to touch this global variable here, but the alternative
	// is to used a modify context everywhere, even if the user supplies
	// it.
	network.DialPeerTimeout = cfg.DialPeerTimeout

	connman, err := connmgr.NewConnManager(cfg.ConnMgr.LowWater, cfg.ConnMgr.HighWater, connmgr.WithGracePeriod(cfg.ConnMgr.GracePeriod))
	if err != nil {
		return nil, nil, nil, err
	}

	var idht *dual.DHT
	opts := []libp2p.Option{
		libp2p.ListenAddrs(cfg.ListenAddr...),
		libp2p.NATPortMap(),
		libp2p.ConnectionManager(connman),
		libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
			idht, err = newDHT(ctx, h, ds)
			return idht, err
		}),
		libp2p.EnableNATService(),
		libp2p.EnableRelay(),
		libp2p.EnableAutoRelay(),
		libp2p.EnableHolePunching(),
	}

	if cfg.EnableRelayHop {
		opts = append(opts, libp2p.EnableRelayService())
	}

	h, err := newHost(
		ctx,
		cfg.Secret,
		ident.PrivateKey,
		opts...,
	)
	if err != nil {
		return nil, nil, nil, err
	}

	psub, err := newPubSub(ctx, h)
	if err != nil {
		h.Close()
		return nil, nil, nil, err
	}

	return h, psub, idht, nil
}

// newHost creates a base cluster host without dht, pubsub, relay or nat etc.
// mostly used for testing.
func newHost(ctx context.Context, psk corepnet.PSK, priv crypto.PrivKey, opts ...libp2p.Option) (host.Host, error) {
	finalOpts := []libp2p.Option{
		libp2p.Identity(priv),
	}
	finalOpts = append(finalOpts, baseOpts(psk)...)
	finalOpts = append(finalOpts, opts...)

	h, err := libp2p.New(
		finalOpts...,
	)
	if err != nil {
		return nil, err
	}

	return h, nil
}

func baseOpts(psk corepnet.PSK) []libp2p.Option {
	return []libp2p.Option{
		libp2p.PrivateNetwork(psk),
		libp2p.EnableNATService(),
		libp2p.Security(noise.ID, noise.New),
		libp2p.Security(libp2ptls.ID, libp2ptls.New),
		// TODO: quic does not support private networks
		// libp2p.DefaultTransports,
		libp2p.NoTransports,
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.Transport(websocket.New),
	}
}

func newDHT(ctx context.Context, h host.Host, store ds.Datastore, extraopts ...dual.Option) (*dual.DHT, error) {
	opts := []dual.Option{
		dual.DHTOption(dht.NamespacedValidator("pk", record.PublicKeyValidator{})),
		dual.DHTOption(dht.NamespacedValidator("ipns", ipns.Validator{KeyBook: h.Peerstore()})),
		dual.DHTOption(dht.Concurrency(10)),
	}

	opts = append(opts, extraopts...)

	if batchingDs, ok := store.(ds.Batching); ok {
		dhtDatastore := namespace.Wrap(batchingDs, ds.NewKey(dhtNamespace))
		opts = append(opts, dual.DHTOption(dht.Datastore(dhtDatastore)))
		logger.Debug("enabling DHT record persistence to datastore")
	}

	return dual.New(ctx, h, opts...)
}

func newPubSub(ctx context.Context, h host.Host) (*pubsub.PubSub, error) {
	return pubsub.NewGossipSub(
		ctx,
		h,
		pubsub.WithMessageSigning(true),
		pubsub.WithStrictSignatureVerification(true),
	)
}

// EncodeProtectorKey converts a byte slice to its hex string representation.
func EncodeProtectorKey(secretBytes []byte) string {
	return hex.EncodeToString(secretBytes)
}
