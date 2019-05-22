package ipfscluster

import (
	"context"
	"encoding/hex"

	"github.com/ipfs/ipfs-cluster/config"
	libp2p "github.com/libp2p/go-libp2p"
	connmgr "github.com/libp2p/go-libp2p-connmgr"
	crypto "github.com/libp2p/go-libp2p-crypto"
	host "github.com/libp2p/go-libp2p-host"
	ipnet "github.com/libp2p/go-libp2p-interface-pnet"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pnet "github.com/libp2p/go-libp2p-pnet"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	routedhost "github.com/libp2p/go-libp2p/p2p/host/routed"
)

// NewClusterHost creates a libp2p Host with the options from the provided
// cluster configuration. Using that host, it creates pubsub and a DHT
// instances, for shared use by all cluster components. The returned host uses
// the DHT for routing. The resulting DHT is not bootstrapped.
func NewClusterHost(
	ctx context.Context,
	ident *config.Identity,
	cfg *Config,
) (host.Host, *pubsub.PubSub, *dht.IpfsDHT, error) {

	connman := connmgr.NewConnManager(cfg.ConnMgr.LowWater, cfg.ConnMgr.HighWater, cfg.ConnMgr.GracePeriod)

	h, err := newHost(
		ctx,
		cfg.Secret,
		ident.PrivateKey,
		libp2p.ListenAddrs(cfg.ListenAddr),
		libp2p.NATPortMap(),
		libp2p.ConnectionManager(connman),
	)
	if err != nil {
		return nil, nil, nil, err
	}

	psub, err := newPubSub(ctx, h)
	if err != nil {
		h.Close()
		return nil, nil, nil, err
	}

	idht, err := newDHT(ctx, h)
	if err != nil {
		h.Close()
		return nil, nil, nil, err
	}

	return routedHost(h, idht), psub, idht, nil
}

func newHost(ctx context.Context, secret []byte, priv crypto.PrivKey, opts ...libp2p.Option) (host.Host, error) {
	var prot ipnet.Protector
	var err error

	// Create protector if we have a secret.
	if secret != nil && len(secret) > 0 {
		var key [32]byte
		copy(key[:], secret)
		prot, err = pnet.NewV1ProtectorFromBytes(&key)
		if err != nil {
			return nil, err
		}
	}

	finalOpts := []libp2p.Option{
		libp2p.Identity(priv),
		libp2p.PrivateNetwork(prot),
	}
	finalOpts = append(finalOpts, opts...)

	return libp2p.New(
		ctx,
		finalOpts...,
	)
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
