package ipfscluster

import (
	"context"
	"encoding/hex"

	config "github.com/ipfs-cluster/ipfs-cluster/config"
	fd "github.com/ipfs-cluster/ipfs-cluster/internal/fd"

	humanize "github.com/dustin/go-humanize"
	ipns "github.com/ipfs/boxo/ipns"
	ds "github.com/ipfs/go-datastore"
	namespace "github.com/ipfs/go-datastore/namespace"
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
	autorelay "github.com/libp2p/go-libp2p/p2p/host/autorelay"
	p2pbhost "github.com/libp2p/go-libp2p/p2p/host/basic"
	rcmgr "github.com/libp2p/go-libp2p/p2p/host/resource-manager"
	connmgr "github.com/libp2p/go-libp2p/p2p/net/connmgr"
	identify "github.com/libp2p/go-libp2p/p2p/protocol/identify"
	noise "github.com/libp2p/go-libp2p/p2p/security/noise"
	libp2ptls "github.com/libp2p/go-libp2p/p2p/security/tls"
	libp2pquic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	tcp "github.com/libp2p/go-libp2p/p2p/transport/tcp"
	websocket "github.com/libp2p/go-libp2p/p2p/transport/websocket"
	mafilter "github.com/libp2p/go-maddr-filter"
	ma "github.com/multiformats/go-multiaddr"
	memory "github.com/pbnjay/memory"
	mamask "github.com/whyrusleeping/multiaddr-filter"
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

	rmgr, err := makeResourceMgr(cfg.ResourceMgr.Enabled, cfg.ResourceMgr.MemoryLimitBytes, cfg.ResourceMgr.FileDescriptorsLimit, cfg.ConnMgr.HighWater)
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

	addrsFactory, err := makeAddrsFactory(cfg.AnnounceAddr, cfg.NoAnnounceAddr)
	if err != nil {
		return nil, nil, nil, err
	}

	opts := []libp2p.Option{
		libp2p.ListenAddrs(cfg.ListenAddr...),
		libp2p.AddrsFactory(addrsFactory),
		libp2p.NATPortMap(),
		libp2p.ConnectionManager(connman),
		libp2p.ResourceManager(rmgr),
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

func makeAddrsFactory(announce []ma.Multiaddr, noAnnounce []ma.Multiaddr) (p2pbhost.AddrsFactory, error) {
	filters := mafilter.NewFilters()
	noAnnAddrs := map[string]bool{}
	for _, addr := range multiAddrstoStrings(noAnnounce) {
		f, err := mamask.NewMask(addr)
		if err == nil {
			filters.AddFilter(*f, mafilter.ActionDeny)
			continue
		}
		maddr, err := ma.NewMultiaddr(addr)
		if err != nil {
			return nil, err
		}
		noAnnAddrs[string(maddr.Bytes())] = true
	}

	return func(allAddrs []ma.Multiaddr) []ma.Multiaddr {
		var addrs []ma.Multiaddr
		if len(announce) > 0 {
			addrs = announce
		} else {
			addrs = allAddrs
		}

		var out []ma.Multiaddr
		for _, maddr := range addrs {
			// check for exact matches
			ok := noAnnAddrs[string(maddr.Bytes())]
			// check for /ipcidr matches
			if !ok && !filters.AddrBlocked(maddr) {
				out = append(out, maddr)
			}
		}
		return out
	}, nil
}

// mostly copy/pasted from https://github.com/ipfs/rainbow/blob/main/rcmgr.go
// which is itself copy-pasted from Kubo, because libp2p does not have
// a sane way of doing this.
func makeResourceMgr(enabled bool, maxMemory, maxFD uint64, connMgrHighWater int) (network.ResourceManager, error) {
	if !enabled {
		rmgr, err := rcmgr.NewResourceManager(rcmgr.NewFixedLimiter(rcmgr.InfiniteLimits))
		logger.Infof("go-libp2p Resource Manager DISABLED")
		return rmgr, err
	}

	// Auto-scaled limits based on available memory/fds.
	if maxMemory == 0 {
		maxMemory = uint64((float64(memory.TotalMemory()) * 0.25))
		if maxMemory < 1<<20 { // 1 GiB
			maxMemory = 1 << 20
		}
	}
	if maxFD == 0 {
		maxFD = fd.GetNumFDs() / 2
	}

	infiniteResourceLimits := rcmgr.InfiniteLimits.ToPartialLimitConfig().System
	maxMemoryMB := maxMemory / (1024 * 1024)

	// At least as of 2023-01-25, it's possible to open a connection that
	// doesn't ask for any memory usage with the libp2p Resource Manager/Accountant
	// (see https://github.com/libp2p/go-libp2p/issues/2010#issuecomment-1404280736).
	// As a result, we can't currently rely on Memory limits to full protect us.
	// Until https://github.com/libp2p/go-libp2p/issues/2010 is addressed,
	// we take a proxy now of restricting to 1 inbound connection per MB.
	// Note: this is more generous than go-libp2p's default autoscaled limits which do
	// 64 connections per 1GB
	// (see https://github.com/libp2p/go-libp2p/blob/master/p2p/host/resource-manager/limit_defaults.go#L357 ).
	systemConnsInbound := int(1 * maxMemoryMB)

	partialLimits := rcmgr.PartialLimitConfig{
		System: rcmgr.ResourceLimits{
			Memory: rcmgr.LimitVal64(maxMemory),
			FD:     rcmgr.LimitVal(maxFD),

			Conns:         rcmgr.Unlimited,
			ConnsInbound:  rcmgr.LimitVal(systemConnsInbound),
			ConnsOutbound: rcmgr.Unlimited,

			Streams:         rcmgr.Unlimited,
			StreamsOutbound: rcmgr.Unlimited,
			StreamsInbound:  rcmgr.Unlimited,
		},

		// Transient connections won't cause any memory to be accounted for by the resource manager/accountant.
		// Only established connections do.
		// As a result, we can't rely on System.Memory to protect us from a bunch of transient connection being opened.
		// We limit the same values as the System scope, but only allow the Transient scope to take 25% of what is allowed for the System scope.
		Transient: rcmgr.ResourceLimits{
			Memory: rcmgr.LimitVal64(maxMemory / 4),
			FD:     rcmgr.LimitVal(maxFD / 4),

			Conns:         rcmgr.Unlimited,
			ConnsInbound:  rcmgr.LimitVal(systemConnsInbound / 4),
			ConnsOutbound: rcmgr.Unlimited,

			Streams:         rcmgr.Unlimited,
			StreamsInbound:  rcmgr.Unlimited,
			StreamsOutbound: rcmgr.Unlimited,
		},

		// Lets get out of the way of the allow list functionality.
		// If someone specified "Swarm.ResourceMgr.Allowlist" we should let it go through.
		AllowlistedSystem: infiniteResourceLimits,

		AllowlistedTransient: infiniteResourceLimits,

		// Keep it simple by not having Service, ServicePeer, Protocol, ProtocolPeer, Conn, or Stream limits.
		ServiceDefault: infiniteResourceLimits,

		ServicePeerDefault: infiniteResourceLimits,

		ProtocolDefault: infiniteResourceLimits,

		ProtocolPeerDefault: infiniteResourceLimits,

		Conn: infiniteResourceLimits,

		Stream: infiniteResourceLimits,

		// Limit the resources consumed by a peer.
		// This doesn't protect us against intentional DoS attacks since an attacker can easily spin up multiple peers.
		// We specify this limit against unintentional DoS attacks (e.g., a peer has a bug and is sending too much traffic intentionally).
		// In that case we want to keep that peer's resource consumption contained.
		// To keep this simple, we only constrain inbound connections and streams.
		PeerDefault: rcmgr.ResourceLimits{
			Memory:          rcmgr.Unlimited64,
			FD:              rcmgr.Unlimited,
			Conns:           rcmgr.Unlimited,
			ConnsInbound:    rcmgr.DefaultLimit,
			ConnsOutbound:   rcmgr.Unlimited,
			Streams:         rcmgr.Unlimited,
			StreamsInbound:  rcmgr.DefaultLimit,
			StreamsOutbound: rcmgr.Unlimited,
		},
	}

	scalingLimitConfig := rcmgr.DefaultLimits
	libp2p.SetDefaultServiceLimits(&scalingLimitConfig)

	// Anything set above in partialLimits that had a value of rcmgr.DefaultLimit will be overridden.
	// Anything in scalingLimitConfig that wasn't defined in partialLimits above will be added (e.g., libp2p's default service limits).
	partialLimits = partialLimits.Build(scalingLimitConfig.Scale(int64(maxMemory), int(maxFD))).ToPartialLimitConfig()

	// Simple checks to override autoscaling ensuring limits make sense versus the connmgr values.
	// There are ways to break this, but this should catch most problems already.
	// We might improve this in the future.
	// See: https://github.com/ipfs/kubo/issues/9545
	if partialLimits.System.ConnsInbound > rcmgr.DefaultLimit {
		maxInboundConns := int(partialLimits.System.ConnsInbound)
		if connmgrHighWaterTimesTwo := connMgrHighWater * 2; maxInboundConns < connmgrHighWaterTimesTwo {
			maxInboundConns = connmgrHighWaterTimesTwo
		}

		if maxInboundConns < 800 {
			maxInboundConns = 800
		}

		// Scale System.StreamsInbound as well, but use the existing ratio of StreamsInbound to ConnsInbound
		if partialLimits.System.StreamsInbound > rcmgr.DefaultLimit {
			partialLimits.System.StreamsInbound = rcmgr.LimitVal(int64(maxInboundConns) * int64(partialLimits.System.StreamsInbound) / int64(partialLimits.System.ConnsInbound))
		}
		partialLimits.System.ConnsInbound = rcmgr.LimitVal(maxInboundConns)
	}

	logger.Infof("go-libp2p Resource Manager ENABLED: Limits based on max_memory: %s, max_fds: %d", humanize.Bytes(maxMemory), maxFD)

	// We already have a complete value thus pass in an empty ConcreteLimitConfig.
	limitCfg := partialLimits.Build(rcmgr.ConcreteLimitConfig{})

	mgr, err := rcmgr.NewResourceManager(rcmgr.NewFixedLimiter(limitCfg))
	if err != nil {
		return nil, err
	}

	return mgr, nil
}
