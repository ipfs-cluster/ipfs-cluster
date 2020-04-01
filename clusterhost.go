package ipfscluster

import (
	"context"
	"encoding/hex"

	"github.com/ipfs/ipfs-cluster/config"
	libp2p "github.com/libp2p/go-libp2p"
	autonat "github.com/libp2p/go-libp2p-autonat-svc"
	relay "github.com/libp2p/go-libp2p-circuit"
	connmgr "github.com/libp2p/go-libp2p-connmgr"
	corepnet "github.com/libp2p/go-libp2p-core/pnet"
	routing "github.com/libp2p/go-libp2p-core/routing"
	crypto "github.com/libp2p/go-libp2p-crypto"
	host "github.com/libp2p/go-libp2p-host"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	secio "github.com/libp2p/go-libp2p-secio"
	libp2ptls "github.com/libp2p/go-libp2p-tls"
	routedhost "github.com/libp2p/go-libp2p/p2p/host/routed"
	identify "github.com/libp2p/go-libp2p/p2p/protocol/identify"
)

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
// a DHT instances, for shared use by all cluster components. The returned
// host uses the DHT for routing. The resulting DHT is not bootstrapped. Relay
// and AutoNATService are additionally setup for this host.
func NewClusterHost(
	ctx context.Context,
	ident *config.Identity,
	cfg *Config,
) (host.Host, *pubsub.PubSub, *dht.IpfsDHT, error) {

	connman := connmgr.NewConnManager(cfg.ConnMgr.LowWater, cfg.ConnMgr.HighWater, cfg.ConnMgr.GracePeriod)

	relayOpts := []relay.RelayOpt{}
	if cfg.EnableRelayHop {
		relayOpts = append(relayOpts, relay.OptHop)
	}

	var idht *dht.IpfsDHT
	var err error
	opts := []libp2p.Option{
		libp2p.ListenAddrs(cfg.ListenAddr...),
		libp2p.NATPortMap(),
		libp2p.ConnectionManager(connman),
		libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
			idht, err = newDHT(ctx, h)
			return idht, err
		}),
		libp2p.EnableRelay(relayOpts...),
		libp2p.EnableAutoRelay(),
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

	// needed for auto relay
	_, err = autonat.NewAutoNATService(ctx, h, baseOpts(cfg.Secret)...)
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
		ctx,
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
		libp2p.Security(libp2ptls.ID, libp2ptls.New),
		libp2p.Security(secio.ID, secio.New),
		// TODO: quic does not support private networks
		//libp2p.Transport(libp2pquic.NewTransport),
		libp2p.DefaultTransports,
	}
}

func newDHT(ctx context.Context, h host.Host) (*dht.IpfsDHT, error) {
	return dht.New(ctx, h)
}

func newPubSub(ctx context.Context, h host.Host) (*pubsub.PubSub, error) {
	return pubsub.NewGossipSub(
		ctx,
		h,
		pubsub.WithMessageSigning(true),
		pubsub.WithStrictSignatureVerification(true),
	)
}

func routedHost(h host.Host, d *dht.IpfsDHT) host.Host {
	return routedhost.Wrap(h, d)
}

// EncodeProtectorKey converts a byte slice to its hex string representation.
func EncodeProtectorKey(secretBytes []byte) string {
	return hex.EncodeToString(secretBytes)
}
