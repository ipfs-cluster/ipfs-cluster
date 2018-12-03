package ipfscluster

import (
	"context"
	"encoding/hex"

	ma "gx/ipfs/QmT4U94DnD8FRfqr21obWY32HLM5VExccPKMjQHofeYqr9/go-multiaddr"
	libp2p "gx/ipfs/QmUDTcnDp2WssbmiDLC6aYurUeyt7QeRakHUQMxA2mZ5iB/go-libp2p"
	ipnet "gx/ipfs/QmW7Ump7YyBMr712Ta3iEVh3ZYcfVvJaPryfbCnyE826b4/go-libp2p-interface-pnet"
	pnet "gx/ipfs/QmY4Q5JC4vxLEi8EpVxJM4rcRryEVtH1zRKVTAm6BKV1pg/go-libp2p-pnet"
	host "gx/ipfs/QmdJfsSbKSZnMkfZ1kpopiyB9i3Hd6cp8VKWZmtWPa7Moc/go-libp2p-host"
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
		libp2p.NATPortMap(),
	)
}

// EncodeProtectorKey converts a byte slice to its hex string representation.
func EncodeProtectorKey(secretBytes []byte) string {
	return hex.EncodeToString(secretBytes)
}
