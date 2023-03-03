package ipfscluster

import (
	"context"
	"encoding/hex"

	config "github.com/ipfs-cluster/ipfs-cluster/config"
	ds "github.com/ipfs/go-datastore"
	namespace "github.com/ipfs/go-datastore/namespace"
	ipns "github.com/ipfs/go-ipns"
	libp2p "github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	dual "github.com/libp2p/go-libp2p-kad-dht/dual"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	record "github.com/libp2p/go-libp2p-record"
	crypto "github.com/libp2p/go-libp2p/core/crypto"
	host "github.com/libp2p/go-libp2p/core/host"
	network "github.com/libp2p/go-libp2p/core/network"
	peer "github.com/libp2p/go-libp2p/core/peer"
	corepnet "github.com/libp2p/go-libp2p/core/pnet"
	routing "github.com/libp2p/go-libp2p/core/routing"
	"github.com/libp2p/go-libp2p/p2p/host/autorelay"
	connmgr "github.com/libp2p/go-libp2p/p2p/net/connmgr"
	identify "github.com/libp2p/go-libp2p/p2p/protocol/identify"
	noise "github.com/libp2p/go-libp2p/p2p/security/noise"
	libp2ptls "github.com/libp2p/go-libp2p/p2p/security/tls"
	libp2pquic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	tcp "github.com/libp2p/go-libp2p/p2p/transport/tcp"
	websocket "github.com/libp2p/go-libp2p/p2p/transport/websocket"
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

	var h host.Host
	var idht *dual.DHT
	// a channel to wait until these variables have been set
	// (or left unset on errors). Mostly to avoid reading while writing.
	hostAndDHTReady := make(chan struct{})
	defer close(hostAndDHTReady)

	hostGetter := func() host.Host {
		<-hostAndDHTReady // closed when we finish NewClusterHost
		return h
	}

	dhtGetter := func() *dual.DHT {
		<-hostAndDHTReady // closed when we finish NewClusterHost
		return idht
	}

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
		libp2p.EnableAutoRelayWithPeerSource(newPeerSource(hostGetter, dhtGetter)),
		libp2p.EnableHolePunching(),
	}

	if cfg.EnableRelayHop {
		opts = append(opts, libp2p.EnableRelayService())
	}

	h, err = newHost(
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

// Inspired in Kubo's
// https://github.com/ipfs/go-ipfs/blob/9327ee64ce96ca6da29bb2a099e0e0930b0d9e09/core/node/libp2p/relay.go#L79-L103
// and https://github.com/ipfs/go-ipfs/blob/9327ee64ce96ca6da29bb2a099e0e0930b0d9e09/core/node/libp2p/routing.go#L242-L317
// but simplified and adapted:
//   - Everytime we need peers for relays we do a DHT lookup.
//   - We return the peers from that lookup.
//   - No need to do it async, since we have to wait for the full lookup to
//     return anyways. We put them on a buffered channel and be done.
func newPeerSource(hostGetter func() host.Host, dhtGetter func() *dual.DHT) autorelay.PeerSource {
	return func(ctx context.Context, numPeers int) <-chan peer.AddrInfo {
		// make a channel to return, and put items from numPeers on
		// that channel up to numPeers. Then close it.
		r := make(chan peer.AddrInfo, numPeers)
		defer close(r)

		// Because the Host, DHT are initialized after relay, we need to
		// obtain them indirectly this way.
		h := hostGetter()
		if h == nil { // context canceled etc.
			return r
		}
		idht := dhtGetter()
		if idht == nil { // context canceled etc.
			return r
		}

		// length of closest peers is K.
		closestPeers, err := idht.WAN.GetClosestPeers(ctx, h.ID().String())
		if err != nil { // Bail out. Usually a "no peers found".
			return r
		}

		logger.Debug("peerSource: %d closestPeers for %d requested", len(closestPeers), numPeers)

		for _, p := range closestPeers {
			addrs := h.Peerstore().Addrs(p)
			if len(addrs) == 0 {
				continue
			}
			dhtPeer := peer.AddrInfo{ID: p, Addrs: addrs}
			// Attempt to put peers on r if we have space,
			// otherwise return (we reached numPeers)
			select {
			case r <- dhtPeer:
			case <-ctx.Done():
				return r
			default:
				return r
			}
		}
		// We are here if numPeers > closestPeers
		return r
	}
}

// EncodeProtectorKey converts a byte slice to its hex string representation.
func EncodeProtectorKey(secretBytes []byte) string {
	return hex.EncodeToString(secretBytes)
}
