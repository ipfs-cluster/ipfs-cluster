package ipfscluster

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/ipfs-cluster/ipfs-cluster/config"

	pnet "github.com/libp2p/go-libp2p/core/pnet"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/kelseyhightower/envconfig"
)

const configKey = "cluster"

// DefaultListenAddrs contains TCP and QUIC listen addresses.
var DefaultListenAddrs = []string{
	"/ip4/0.0.0.0/tcp/9096",
	"/ip4/0.0.0.0/udp/9096/quic",
}

// Configuration defaults
const (
	DefaultEnableRelayHop          = true
	DefaultStateSyncInterval       = 5 * time.Minute
	DefaultPinRecoverInterval      = 12 * time.Minute
	DefaultMonitorPingInterval     = 15 * time.Second
	DefaultPeerWatchInterval       = 5 * time.Second
	DefaultReplicationFactor       = -1
	DefaultLeaveOnShutdown         = false
	DefaultPinOnlyOnTrustedPeers   = false
	DefaultPinOnlyOnUntrustedPeers = false
	DefaultDisableRepinning        = true
	DefaultPeerstoreFile           = "peerstore"
	DefaultConnMgrHighWater        = 400
	DefaultConnMgrLowWater         = 100
	DefaultConnMgrGracePeriod      = 2 * time.Minute
	DefaultDialPeerTimeout         = 3 * time.Second
	DefaultFollowerMode            = false
	DefaultMDNSInterval            = 10 * time.Second
)

// ConnMgrConfig configures the libp2p host connection manager.
type ConnMgrConfig struct {
	HighWater   int
	LowWater    int
	GracePeriod time.Duration
}

// Config is the configuration object containing customizable variables to
// initialize the main ipfs-cluster component. It implements the
// config.ComponentConfig interface.
type Config struct {
	config.Saver

	// User-defined peername for use as human-readable identifier.
	Peername string

	// Cluster secret for private network. Peers will be in the same cluster if and
	// only if they have the same ClusterSecret. The cluster secret must be exactly
	// 64 characters and contain only hexadecimal characters (`[0-9a-f]`).
	Secret pnet.PSK

	// RPCPolicy defines access control to RPC endpoints.
	RPCPolicy map[string]RPCEndpointType

	// Leave Cluster on shutdown. Politely informs other peers
	// of the departure and removes itself from the consensus
	// peer set. The Cluster size will be reduced by one.
	LeaveOnShutdown bool

	// Listen parameters for the Cluster libp2p Host. Used by
	// the RPC and Consensus components.
	ListenAddr []ma.Multiaddr

	// Enables HOP relay for the node. If this is enabled, the node will act as
	// an intermediate (Hop Relay) node in relay circuits for connected peers.
	EnableRelayHop bool

	// ConnMgr holds configuration values for the connection manager for
	// the libp2p host.
	// FIXME: This only applies to ipfs-cluster-service.
	ConnMgr ConnMgrConfig

	// Sets the default dial timeout for libp2p connections to other
	// peers.
	DialPeerTimeout time.Duration

	// Time between syncs of the consensus state to the
	// tracker state. Normally states are synced anyway, but this helps
	// when new nodes are joining the cluster. Reduce for faster
	// consistency, increase with larger states.
	StateSyncInterval time.Duration

	// Time between automatic runs of the "recover" operation
	// which will retry to pin/unpin items in error state.
	PinRecoverInterval time.Duration

	// ReplicationFactorMax indicates the target number of nodes
	// that should pin content. For exampe, a replication_factor of
	// 3 will have cluster allocate each pinned hash to 3 peers if
	// possible.
	// See also ReplicationFactorMin. A ReplicationFactorMax of -1
	// will allocate to every available node.
	ReplicationFactorMax int

	// ReplicationFactorMin indicates the minimum number of healthy
	// nodes pinning content. If the number of nodes available to pin
	// is less than this threshold, an error will be returned.
	// In the case of peer health issues, content pinned will be
	// re-allocated if the threshold is crossed.
	// For exampe, a ReplicationFactorMin of 2 will allocate at least
	// two peer to hold content, and return an error if this is not
	// possible.
	ReplicationFactorMin int

	// MonitorPingInterval is the frequency with which a cluster peer
	// sends a "ping" metric. The metric has a TTL set to the double of
	// this value. This metric sends information about this peer to other
	// peers.
	MonitorPingInterval time.Duration

	// PeerWatchInterval is the frequency that we use to watch for changes
	// in the consensus peerset and save new peers to the configuration
	// file. This also affects how soon we realize that we have
	// been removed from a cluster.
	PeerWatchInterval time.Duration

	// MDNSInterval controls the time between mDNS broadcasts to the
	// network announcing the peer addresses. Set to 0 to disable
	// mDNS.
	MDNSInterval time.Duration

	// PinOnlyOnTrustedPeers limits allocations to trusted peers only.
	PinOnlyOnTrustedPeers bool

	// PinOnlyOnUntrustedPeers limits allocations to untrusted peers only.
	PinOnlyOnUntrustedPeers bool

	// If true, DisableRepinning, ensures that no repinning happens
	// when a node goes down.
	// This is useful when doing certain types of maintenance, or simply
	// when not wanting to rely on the monitoring system which needs a revamp.
	DisableRepinning bool

	// FollowerMode disables broadcast requests from this peer
	// (sync, recover, status) and disallows pinset management
	// operations (Pin/Unpin).
	FollowerMode bool

	// Peerstore file specifies the file on which we persist the
	// libp2p host peerstore addresses. This file is regularly saved.
	PeerstoreFile string

	// PeerAddresses stores additional addresses for peers that may or may
	// not be in the peerstore file. These are considered high priority
	// when bootstrapping the initial cluster connections.
	PeerAddresses []ma.Multiaddr

	// Tracing flag used to skip tracing specific paths when not enabled.
	Tracing bool
}

// configJSON represents a Cluster configuration as it will look when it is
// saved using JSON. Most configuration keys are converted into simple types
// like strings, and key names aim to be self-explanatory for the user.
type configJSON struct {
	ID                      string             `json:"id,omitempty"`
	Peername                string             `json:"peername"`
	PrivateKey              string             `json:"private_key,omitempty" hidden:"true"`
	Secret                  string             `json:"secret" hidden:"true"`
	LeaveOnShutdown         bool               `json:"leave_on_shutdown"`
	ListenMultiaddress      config.Strings     `json:"listen_multiaddress"`
	EnableRelayHop          bool               `json:"enable_relay_hop"`
	ConnectionManager       *connMgrConfigJSON `json:"connection_manager"`
	DialPeerTimeout         string             `json:"dial_peer_timeout"`
	StateSyncInterval       string             `json:"state_sync_interval"`
	PinRecoverInterval      string             `json:"pin_recover_interval"`
	ReplicationFactorMin    int                `json:"replication_factor_min"`
	ReplicationFactorMax    int                `json:"replication_factor_max"`
	MonitorPingInterval     string             `json:"monitor_ping_interval"`
	PeerWatchInterval       string             `json:"peer_watch_interval"`
	MDNSInterval            string             `json:"mdns_interval"`
	PinOnlyOnTrustedPeers   bool               `json:"pin_only_on_trusted_peers"`
	PinOnlyOnUntrustedPeers bool               `json:"pin_only_on_untrusted_peers"`
	DisableRepinning        bool               `json:"disable_repinning"`
	FollowerMode            bool               `json:"follower_mode,omitempty"`
	PeerstoreFile           string             `json:"peerstore_file,omitempty"`
	PeerAddresses           []string           `json:"peer_addresses"`
}

// connMgrConfigJSON configures the libp2p host connection manager.
type connMgrConfigJSON struct {
	HighWater   int    `json:"high_water"`
	LowWater    int    `json:"low_water"`
	GracePeriod string `json:"grace_period"`
}

// ConfigKey returns a human-readable string to identify
// a cluster Config.
func (cfg *Config) ConfigKey() string {
	return configKey
}

// Default fills in all the Config fields with
// default working values. This means, it will
// generate a Secret.
func (cfg *Config) Default() error {
	cfg.setDefaults()

	clusterSecret := make([]byte, 32)
	n, err := rand.Read(clusterSecret)
	if err != nil {
		return err
	}
	if n != 32 {
		return errors.New("did not generate 32-byte secret")
	}

	cfg.Secret = clusterSecret
	return nil
}

// ApplyEnvVars fills in any Config fields found
// as environment variables.
func (cfg *Config) ApplyEnvVars() error {
	jcfg, err := cfg.toConfigJSON()
	if err != nil {
		return err
	}

	err = envconfig.Process(cfg.ConfigKey(), jcfg)
	if err != nil {
		return err
	}

	return cfg.applyConfigJSON(jcfg)
}

// Validate will check that the values of this config
// seem to be working ones.
func (cfg *Config) Validate() error {
	if cfg.ListenAddr == nil {
		return errors.New("cluster.listen_multiaddress is undefined")
	}

	if len(cfg.ListenAddr) == 0 {
		return errors.New("cluster.listen_multiaddress is empty")
	}

	if cfg.ConnMgr.LowWater <= 0 {
		return errors.New("cluster.connection_manager.low_water is invalid")
	}

	if cfg.ConnMgr.HighWater <= 0 {
		return errors.New("cluster.connection_manager.high_water is invalid")
	}

	if cfg.ConnMgr.LowWater > cfg.ConnMgr.HighWater {
		return errors.New("cluster.connection_manager.low_water is greater than high_water")
	}

	if cfg.ConnMgr.GracePeriod == 0 {
		return errors.New("cluster.connection_manager.grace_period is invalid")
	}

	if cfg.DialPeerTimeout <= 0 {
		return errors.New("cluster.dial_peer_timeout is invalid")
	}

	if cfg.StateSyncInterval <= 0 {
		return errors.New("cluster.state_sync_interval is invalid")
	}

	if cfg.PinRecoverInterval <= 0 {
		return errors.New("cluster.pin_recover_interval is invalid")
	}

	if cfg.MonitorPingInterval <= 0 {
		return errors.New("cluster.monitoring_interval is invalid")
	}

	if cfg.PeerWatchInterval <= 0 {
		return errors.New("cluster.peer_watch_interval is invalid")
	}

	if cfg.PinOnlyOnTrustedPeers && cfg.PinOnlyOnUntrustedPeers {
		return errors.New("cluster.pin_only_on_trusted_peers and pin_only_on_untrusted_peers cannot both be true")
	}

	rfMax := cfg.ReplicationFactorMax
	rfMin := cfg.ReplicationFactorMin

	if err := isReplicationFactorValid(rfMin, rfMax); err != nil {
		return err
	}

	return isRPCPolicyValid(cfg.RPCPolicy)
}

func isReplicationFactorValid(rplMin, rplMax int) error {
	// check Max and Min are correct
	if rplMin == 0 || rplMax == 0 {
		return errors.New("cluster.replication_factor_min and max must be set")
	}

	if rplMin > rplMax {
		return errors.New("cluster.replication_factor_min is larger than max")
	}

	if rplMin < -1 {
		return errors.New("cluster.replication_factor_min is wrong")
	}

	if rplMax < -1 {
		return errors.New("cluster.replication_factor_max is wrong")
	}

	if (rplMin == -1 && rplMax != -1) || (rplMin != -1 && rplMax == -1) {
		return errors.New("cluster.replication_factor_min and max must be -1 when one of them is")
	}
	return nil
}

func isRPCPolicyValid(p map[string]RPCEndpointType) error {
	rpcComponents := []interface{}{
		&ClusterRPCAPI{},
		&PinTrackerRPCAPI{},
		&IPFSConnectorRPCAPI{},
		&ConsensusRPCAPI{},
		&PeerMonitorRPCAPI{},
	}

	total := 0
	for _, c := range rpcComponents {
		t := reflect.TypeOf(c)
		for i := 0; i < t.NumMethod(); i++ {
			total++
			method := t.Method(i)
			name := fmt.Sprintf("%s.%s", RPCServiceID(c), method.Name)
			_, ok := p[name]
			if !ok {
				return fmt.Errorf("RPCPolicy is missing the %s method", name)
			}
		}
	}
	if len(p) != total {
		logger.Warn("defined RPC policy has more entries than needed")
	}
	return nil
}

// this just sets non-generated defaults
func (cfg *Config) setDefaults() {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = ""
	}
	cfg.Peername = hostname

	listenAddrs := []ma.Multiaddr{}
	for _, m := range DefaultListenAddrs {
		addr, _ := ma.NewMultiaddr(m)
		listenAddrs = append(listenAddrs, addr)
	}
	cfg.ListenAddr = listenAddrs
	cfg.EnableRelayHop = DefaultEnableRelayHop
	cfg.ConnMgr = ConnMgrConfig{
		HighWater:   DefaultConnMgrHighWater,
		LowWater:    DefaultConnMgrLowWater,
		GracePeriod: DefaultConnMgrGracePeriod,
	}
	cfg.DialPeerTimeout = DefaultDialPeerTimeout
	cfg.LeaveOnShutdown = DefaultLeaveOnShutdown
	cfg.StateSyncInterval = DefaultStateSyncInterval
	cfg.PinRecoverInterval = DefaultPinRecoverInterval
	cfg.ReplicationFactorMin = DefaultReplicationFactor
	cfg.ReplicationFactorMax = DefaultReplicationFactor
	cfg.MonitorPingInterval = DefaultMonitorPingInterval
	cfg.PeerWatchInterval = DefaultPeerWatchInterval
	cfg.MDNSInterval = DefaultMDNSInterval
	cfg.PinOnlyOnTrustedPeers = DefaultPinOnlyOnTrustedPeers
	cfg.PinOnlyOnUntrustedPeers = DefaultPinOnlyOnUntrustedPeers
	cfg.DisableRepinning = DefaultDisableRepinning
	cfg.FollowerMode = DefaultFollowerMode
	cfg.PeerstoreFile = "" // empty so it gets omitted.
	cfg.PeerAddresses = []ma.Multiaddr{}
	cfg.RPCPolicy = DefaultRPCPolicy
}

// LoadJSON receives a raw json-formatted configuration and
// sets the Config fields from it. Note that it should be JSON
// as generated by ToJSON().
func (cfg *Config) LoadJSON(raw []byte) error {
	jcfg := &configJSON{}
	err := json.Unmarshal(raw, jcfg)
	if err != nil {
		logger.Error("Error unmarshaling cluster config")
		return err
	}

	cfg.setDefaults()

	return cfg.applyConfigJSON(jcfg)
}

func (cfg *Config) applyConfigJSON(jcfg *configJSON) error {
	config.SetIfNotDefault(jcfg.PeerstoreFile, &cfg.PeerstoreFile)

	config.SetIfNotDefault(jcfg.Peername, &cfg.Peername)

	clusterSecret, err := DecodeClusterSecret(jcfg.Secret)
	if err != nil {
		err = fmt.Errorf("error loading cluster secret from config: %s", err)
		return err
	}
	cfg.Secret = clusterSecret

	var listenAddrs []ma.Multiaddr
	for _, addr := range jcfg.ListenMultiaddress {
		listenAddr, err := ma.NewMultiaddr(addr)
		if err != nil {
			err = fmt.Errorf("error parsing a listen_multiaddress: %s", err)
			return err
		}
		listenAddrs = append(listenAddrs, listenAddr)
	}

	cfg.ListenAddr = listenAddrs
	cfg.EnableRelayHop = jcfg.EnableRelayHop
	if conman := jcfg.ConnectionManager; conman != nil {
		cfg.ConnMgr = ConnMgrConfig{
			HighWater: jcfg.ConnectionManager.HighWater,
			LowWater:  jcfg.ConnectionManager.LowWater,
		}
		err = config.ParseDurations("cluster",
			&config.DurationOpt{Duration: jcfg.ConnectionManager.GracePeriod, Dst: &cfg.ConnMgr.GracePeriod, Name: "connection_manager.grace_period"},
		)
		if err != nil {
			return err
		}
	}

	rplMin := jcfg.ReplicationFactorMin
	rplMax := jcfg.ReplicationFactorMax
	config.SetIfNotDefault(rplMin, &cfg.ReplicationFactorMin)
	config.SetIfNotDefault(rplMax, &cfg.ReplicationFactorMax)

	err = config.ParseDurations("cluster",
		&config.DurationOpt{Duration: jcfg.DialPeerTimeout, Dst: &cfg.DialPeerTimeout, Name: "dial_peer_timeout"},
		&config.DurationOpt{Duration: jcfg.StateSyncInterval, Dst: &cfg.StateSyncInterval, Name: "state_sync_interval"},
		&config.DurationOpt{Duration: jcfg.PinRecoverInterval, Dst: &cfg.PinRecoverInterval, Name: "pin_recover_interval"},
		&config.DurationOpt{Duration: jcfg.MonitorPingInterval, Dst: &cfg.MonitorPingInterval, Name: "monitor_ping_interval"},
		&config.DurationOpt{Duration: jcfg.PeerWatchInterval, Dst: &cfg.PeerWatchInterval, Name: "peer_watch_interval"},
		&config.DurationOpt{Duration: jcfg.MDNSInterval, Dst: &cfg.MDNSInterval, Name: "mdns_interval"},
	)
	if err != nil {
		return err
	}

	// PeerAddresses
	peerAddrs := []ma.Multiaddr{}
	for _, addr := range jcfg.PeerAddresses {
		peerAddr, err := ma.NewMultiaddr(addr)
		if err != nil {
			err = fmt.Errorf("error parsing peer_addresses: %s", err)
			return err
		}
		peerAddrs = append(peerAddrs, peerAddr)
	}
	cfg.PeerAddresses = peerAddrs
	cfg.LeaveOnShutdown = jcfg.LeaveOnShutdown
	cfg.PinOnlyOnTrustedPeers = jcfg.PinOnlyOnTrustedPeers
	cfg.PinOnlyOnUntrustedPeers = jcfg.PinOnlyOnUntrustedPeers
	cfg.DisableRepinning = jcfg.DisableRepinning
	cfg.FollowerMode = jcfg.FollowerMode

	return cfg.Validate()
}

// ToJSON generates a human-friendly version of Config.
func (cfg *Config) ToJSON() (raw []byte, err error) {
	jcfg, err := cfg.toConfigJSON()
	if err != nil {
		return
	}

	raw, err = json.MarshalIndent(jcfg, "", "    ")
	return
}

func (cfg *Config) toConfigJSON() (jcfg *configJSON, err error) {
	// Multiaddress String() may panic
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%s", r)
		}
	}()

	jcfg = &configJSON{}

	// Set all configuration fields
	jcfg.Peername = cfg.Peername
	jcfg.Secret = EncodeProtectorKey(cfg.Secret)
	jcfg.ReplicationFactorMin = cfg.ReplicationFactorMin
	jcfg.ReplicationFactorMax = cfg.ReplicationFactorMax
	jcfg.LeaveOnShutdown = cfg.LeaveOnShutdown
	var listenAddrs config.Strings
	for _, addr := range cfg.ListenAddr {
		listenAddrs = append(listenAddrs, addr.String())
	}
	jcfg.ListenMultiaddress = config.Strings(listenAddrs)
	jcfg.EnableRelayHop = cfg.EnableRelayHop
	jcfg.ConnectionManager = &connMgrConfigJSON{
		HighWater:   cfg.ConnMgr.HighWater,
		LowWater:    cfg.ConnMgr.LowWater,
		GracePeriod: cfg.ConnMgr.GracePeriod.String(),
	}
	jcfg.DialPeerTimeout = cfg.DialPeerTimeout.String()
	jcfg.StateSyncInterval = cfg.StateSyncInterval.String()
	jcfg.PinRecoverInterval = cfg.PinRecoverInterval.String()
	jcfg.MonitorPingInterval = cfg.MonitorPingInterval.String()
	jcfg.PeerWatchInterval = cfg.PeerWatchInterval.String()
	jcfg.MDNSInterval = cfg.MDNSInterval.String()
	jcfg.PinOnlyOnTrustedPeers = cfg.PinOnlyOnTrustedPeers
	jcfg.PinOnlyOnUntrustedPeers = cfg.PinOnlyOnUntrustedPeers
	jcfg.DisableRepinning = cfg.DisableRepinning
	jcfg.PeerstoreFile = cfg.PeerstoreFile
	jcfg.PeerAddresses = []string{}
	for _, addr := range cfg.PeerAddresses {
		jcfg.PeerAddresses = append(jcfg.PeerAddresses, addr.String())
	}
	jcfg.FollowerMode = cfg.FollowerMode

	return
}

// GetPeerstorePath returns the full path of the
// PeerstoreFile, obtained by concatenating that value
// with BaseDir of the configuration, if set.
// An empty string is returned when BaseDir is not set.
func (cfg *Config) GetPeerstorePath() string {
	if cfg.BaseDir == "" {
		return ""
	}

	filename := DefaultPeerstoreFile
	if cfg.PeerstoreFile != "" {
		filename = cfg.PeerstoreFile
	}

	return filepath.Join(cfg.BaseDir, filename)
}

// ToDisplayJSON returns JSON config as a string.
func (cfg *Config) ToDisplayJSON() ([]byte, error) {
	jcfg, err := cfg.toConfigJSON()
	if err != nil {
		return nil, err
	}
	return config.DisplayJSON(jcfg)
}

// DecodeClusterSecret parses a hex-encoded string, checks that it is exactly
// 32 bytes long and returns its value as a byte-slice.x
func DecodeClusterSecret(hexSecret string) ([]byte, error) {
	secret, err := hex.DecodeString(hexSecret)
	if err != nil {
		return nil, err
	}
	switch secretLen := len(secret); secretLen {
	case 0:
		logger.Warn("Cluster secret is empty, cluster will start on unprotected network.")
		return nil, nil
	case 32:
		return secret, nil
	default:
		return nil, fmt.Errorf("input secret is %d bytes, cluster secret should be 32", secretLen)
	}
}
