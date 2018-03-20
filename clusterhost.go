package ipfscluster

import (
	"context"
	"encoding/hex"

	libp2p "github.com/libp2p/go-libp2p"
	host "github.com/libp2p/go-libp2p-host"
	ipnet "github.com/libp2p/go-libp2p-interface-pnet"
	pnet "github.com/libp2p/go-libp2p-pnet"
	ma "github.com/multiformats/go-multiaddr"
)

// NewClusterHost creates a libp2p Host with the options from the
// provided cluster configuration.
func NewClusterHost(ctx context.Context, cfg *Config) (host.Host, error) {
	var prot ipnet.Protector
	var err error

	// Create protector if we have a secret.
	if cfg.Secret != nil && len(cfg.Secret) > 0 {
		var key [32]byte
		copy(key[:], cfg.Secret)
		prot, err = pnet.NewV1ProtectorFromBytes(&key)
		if err != nil {
			return nil, err
		}
	}

	return libp2p.New(
		ctx,
		libp2p.Identity(cfg.PrivateKey),
		libp2p.ListenAddrs([]ma.Multiaddr{cfg.ListenAddr}...),
		libp2p.PrivateNetwork(prot),
		// FIXME: Enable when libp2p >= 5.0.16
		// https://github.com/libp2p/go-libp2p/pull/293
		//libp2p.NATPortMap(),
	)
}

// EncodeProtectorKey converts a byte slice to its hex string representation.
func EncodeProtectorKey(secretBytes []byte) string {
	return hex.EncodeToString(secretBytes)
}
