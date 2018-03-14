package ipfscluster

import (
	"bytes"
	"context"
	"encoding/hex"
	"strings"

	libp2p "github.com/libp2p/go-libp2p"
	host "github.com/libp2p/go-libp2p-host"
	ipnet "github.com/libp2p/go-libp2p-interface-pnet"
	pnet "github.com/libp2p/go-libp2p-pnet"
	ma "github.com/multiformats/go-multiaddr"
)

// NewClusterHost creates a libp2p Host with the options in the from the
// provided cluster configuration.
func NewClusterHost(ctx context.Context, cfg *Config) (host.Host, error) {
	var prot ipnet.Protector

	// Create protector if we have a secret.
	if cfg.Secret != nil && len(cfg.Secret) > 0 {
		protKey, err := SecretToProtectorKey(cfg.Secret)
		if err != nil {
			return nil, err
		}
		prot, err = pnet.NewProtector(strings.NewReader(protKey))
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

// SecretToProtectorKey converts a private network secret
// provided as a byte-slice into the format expected by go-libp2p-pnet
func SecretToProtectorKey(secret []byte) (string, error) {
	var key bytes.Buffer
	key.WriteString("/key/swarm/psk/1.0.0/\n")
	key.WriteString("/base16/\n")
	key.WriteString(EncodeProtectorKey(secret))

	return key.String(), nil
}

// EncodeProtectorKey converts a byte slice to its hex string representation.
func EncodeProtectorKey(secretBytes []byte) string {
	return hex.EncodeToString(secretBytes)
}
