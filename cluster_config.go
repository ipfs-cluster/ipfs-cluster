package ipfscluster

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/ipfs/ipfs-cluster/config"

	crypto "github.com/libp2p/go-libp2p-crypto"
	peer "github.com/libp2p/go-libp2p-peer"
	pnet "github.com/libp2p/go-libp2p-pnet"
	ma "github.com/multiformats/go-multiaddr"
)

const configKey = "cluster"

// Configuration defaults
const (
	DefaultConfigCrypto        = crypto.RSA
	DefaultConfigKeyLength     = 2048
	DefaultListenAddr          = "/ip4/0.0.0.0/tcp/9096"
	DefaultStateSyncInterval   = 60 * time.Second
	DefaultIPFSSyncInterval    = 130 * time.Second
	DefaultMonitorPingInterval = 15 * time.Second
	DefaultPeerWatchInterval   = 5 * time.Second
	DefaultReplicationFactor   = -1
	DefaultLeaveOnShutdown     = false
)

// Config is the configuration object containing customizable variables to
// initialize the main ipfs-cluster component. It implements the
// config.ComponentConfig interface.
type Config struct {
	config.Saver
	lock sync.Mutex

	// Libp2p ID and private key for Cluster communication (including)
	// the Consensus component.
	ID         peer.ID
	PrivateKey crypto.PrivKey

	// User-defined peername for use as human-readable identifier.
	Peername string

	// Cluster secret for private network. Peers will be in the same cluster if and
	// only if they have the same ClusterSecret. The cluster secret must be exactly
	// 64 characters and contain only hexadecimal characters (`[0-9a-f]`).
	Secret []byte

	// Peers is the list of peers in the Cluster. They are used
	// as the initial peers in the consensus. When bootstrapping a peer,
	// Peers will be filled in automatically for the next run upon
	// shutdown.
	Peers []ma.Multiaddr

	// Bootstrap peers multiaddresses. This peer will attempt to
	// join the clusters of the peers in this list after booting.
	// Leave empty for a single-peer-cluster.
	Bootstrap []ma.Multiaddr

	// Leave Cluster on shutdown. Politely informs other peers
	// of the departure and removes itself from the consensus
	// peer set. The Cluster size will be reduced by one.
	LeaveOnShutdown bool

	// Listen parameters for the Cluster libp2p Host. Used by
	// the RPC and Consensus components.
	ListenAddr ma.Multiaddr

	// Time between syncs of the consensus state to the
	// tracker state. Normally states are synced anyway, but this helps
	// when new nodes are joining the cluster. Reduce for faster
	// consistency, increase with larger states.
	StateSyncInterval time.Duration

	// Number of seconds between syncs of the local state and
	// the state of the ipfs daemon. This ensures that cluster
	// provides the right status for tracked items (for example
	// to detect that a pin has been removed. Reduce for faster
	// consistency, increase when the number of pinned items is very
	// large.
	IPFSSyncInterval time.Duration

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

	// MonitorPingInterval is the frequency with which a cluster peer pings
	// the monitoring component. The ping metric has a TTL set to the double
	// of this value.
	MonitorPingInterval time.Duration

	// PeerWatchInterval is the frequency that we use to watch for changes
	// in the consensus peerset and save new peers to the configuration
	// file. This also affects how soon we realize that we have
	// been removed from a cluster.
	PeerWatchInterval time.Duration
}

// configJSON represents a Cluster configuration as it will look when it is
// saved using JSON. Most configuration keys are converted into simple types
// like strings, and key names aim to be self-explanatory for the user.
type configJSON struct {
	ID                   string   `json:"id"`
	Peername             string   `json:"peername"`
	PrivateKey           string   `json:"private_key"`
	Secret               string   `json:"secret"`
	Peers                []string `json:"peers"`
	Bootstrap            []string `json:"bootstrap"`
	LeaveOnShutdown      bool     `json:"leave_on_shutdown"`
	ListenMultiaddress   string   `json:"listen_multiaddress"`
	StateSyncInterval    string   `json:"state_sync_interval"`
	IPFSSyncInterval     string   `json:"ipfs_sync_interval"`
	ReplicationFactor    int      `json:"replication_factor,omitempty"` // legacy
	ReplicationFactorMin int      `json:"replication_factor_min"`
	ReplicationFactorMax int      `json:"replication_factor_max"`
	MonitorPingInterval  string   `json:"monitor_ping_interval"`
	PeerWatchInterval    string   `json:"peer_watch_interval"`
}

// ConfigKey returns a human-readable string to identify
// a cluster Config.
func (cfg *Config) ConfigKey() string {
	return configKey
}

// Default fills in all the Config fields with
// default working values. This means, it will
// generate a valid random ID, PrivateKey and
// Secret.
func (cfg *Config) Default() error {
	cfg.setDefaults()

	// pid and private key generation --
	priv, pub, err := crypto.GenerateKeyPair(
		DefaultConfigCrypto,
		DefaultConfigKeyLength)
	if err != nil {
		return err
	}
	pid, err := peer.IDFromPublicKey(pub)
	if err != nil {
		return err
	}
	cfg.ID = pid
	cfg.PrivateKey = priv
	// --

	// cluster secret
	clusterSecret, err := pnet.GenerateV1Bytes()
	if err != nil {
		return err
	}
	cfg.Secret = (*clusterSecret)[:]
	// --
	return nil
}

// Validate will check that the values of this config
// seem to be working ones.
func (cfg *Config) Validate() error {
	if cfg.ID == "" {
		return errors.New("cluster.ID not set")
	}

	if cfg.PrivateKey == nil {
		return errors.New("no cluster.private_key set")
	}

	if !cfg.ID.MatchesPrivateKey(cfg.PrivateKey) {
		return errors.New("cluster.ID does not match the private_key")
	}

	if cfg.Peers == nil {
		return errors.New("cluster.peers is undefined")
	}

	if cfg.Bootstrap == nil {
		return errors.New("cluster.bootstrap is undefined")
	}

	if cfg.ListenAddr == nil {
		return errors.New("cluster.listen_addr is indefined")
	}

	if cfg.StateSyncInterval <= 0 {
		return errors.New("cluster.state_sync_interval is invalid")
	}

	if cfg.IPFSSyncInterval <= 0 {
		return errors.New("cluster.ipfs_sync_interval is invalid")
	}

	if cfg.MonitorPingInterval <= 0 {
		return errors.New("cluster.monitoring_interval is invalid")
	}

	if cfg.PeerWatchInterval <= 0 {
		return errors.New("cluster.peer_watch_interval is invalid")
	}

	rfMax := cfg.ReplicationFactorMax
	rfMin := cfg.ReplicationFactorMin

	return isReplicationFactorValid(rfMin, rfMax)
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

	if (rplMin == -1 && rplMax != -1) ||
		(rplMin != -1 && rplMax == -1) {
		return errors.New("cluster.replication_factor_min and max must be -1 when one of them is")
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

	addr, _ := ma.NewMultiaddr(DefaultListenAddr)
	cfg.ListenAddr = addr
	cfg.Peers = []ma.Multiaddr{}
	cfg.Bootstrap = []ma.Multiaddr{}
	cfg.LeaveOnShutdown = DefaultLeaveOnShutdown
	cfg.StateSyncInterval = DefaultStateSyncInterval
	cfg.IPFSSyncInterval = DefaultIPFSSyncInterval
	cfg.ReplicationFactorMin = DefaultReplicationFactor
	cfg.ReplicationFactorMax = DefaultReplicationFactor
	cfg.MonitorPingInterval = DefaultMonitorPingInterval
	cfg.PeerWatchInterval = DefaultPeerWatchInterval
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

	// Make sure all non-defined keys have good values.
	cfg.setDefaults()

	parseDuration := func(txt string) time.Duration {
		d, _ := time.ParseDuration(txt)
		if txt != "" && d == 0 {
			logger.Warningf("%s is not a valid duration. Default will be used", txt)
		}
		return d
	}

	id, err := peer.IDB58Decode(jcfg.ID)
	if err != nil {
		err = fmt.Errorf("error decoding cluster ID: %s", err)
		return err
	}
	cfg.ID = id

	config.SetIfNotDefault(jcfg.Peername, &cfg.Peername)

	pkb, err := base64.StdEncoding.DecodeString(jcfg.PrivateKey)
	if err != nil {
		err = fmt.Errorf("error decoding private_key: %s", err)
		return err
	}
	pKey, err := crypto.UnmarshalPrivateKey(pkb)
	if err != nil {
		err = fmt.Errorf("error parsing private_key ID: %s", err)
		return err
	}
	cfg.PrivateKey = pKey

	clusterSecret, err := DecodeClusterSecret(jcfg.Secret)
	if err != nil {
		err = fmt.Errorf("error loading cluster secret from config: %s", err)
		return err
	}
	cfg.Secret = clusterSecret

	parseMultiaddrs := func(strs []string) ([]ma.Multiaddr, error) {
		addrs := make([]ma.Multiaddr, len(strs))
		for i, p := range strs {
			maddr, err := ma.NewMultiaddr(p)
			if err != nil {
				m := "error parsing multiaddress for peer %s: %s"
				err = fmt.Errorf(m, p, err)
				return nil, err
			}
			addrs[i] = maddr
		}
		return addrs, nil
	}

	clusterPeers, err := parseMultiaddrs(jcfg.Peers)
	if err != nil {
		return err
	}
	cfg.Peers = clusterPeers

	bootstrap, err := parseMultiaddrs(jcfg.Bootstrap)
	if err != nil {
		return err
	}
	cfg.Bootstrap = bootstrap

	clusterAddr, err := ma.NewMultiaddr(jcfg.ListenMultiaddress)
	if err != nil {
		err = fmt.Errorf("error parsing cluster_listen_multiaddress: %s", err)
		return err
	}
	cfg.ListenAddr = clusterAddr

	rplMin := jcfg.ReplicationFactorMin
	rplMax := jcfg.ReplicationFactorMax
	if jcfg.ReplicationFactor != 0 { // read min and max
		rplMin = jcfg.ReplicationFactor
		rplMax = rplMin
	}
	config.SetIfNotDefault(rplMin, &cfg.ReplicationFactorMin)
	config.SetIfNotDefault(rplMax, &cfg.ReplicationFactorMax)

	stateSyncInterval := parseDuration(jcfg.StateSyncInterval)
	ipfsSyncInterval := parseDuration(jcfg.IPFSSyncInterval)
	monitorPingInterval := parseDuration(jcfg.MonitorPingInterval)
	peerWatchInterval := parseDuration(jcfg.PeerWatchInterval)

	config.SetIfNotDefault(stateSyncInterval, &cfg.StateSyncInterval)
	config.SetIfNotDefault(ipfsSyncInterval, &cfg.IPFSSyncInterval)
	config.SetIfNotDefault(monitorPingInterval, &cfg.MonitorPingInterval)
	config.SetIfNotDefault(peerWatchInterval, &cfg.PeerWatchInterval)

	cfg.LeaveOnShutdown = jcfg.LeaveOnShutdown

	return cfg.Validate()
}

// ToJSON generates a human-friendly version of Config.
func (cfg *Config) ToJSON() (raw []byte, err error) {
	// Multiaddress String() may panic
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%s", r)
		}
	}()

	jcfg := &configJSON{}

	// Private Key
	pkeyBytes, err := cfg.PrivateKey.Bytes()
	if err != nil {
		return
	}
	pKey := base64.StdEncoding.EncodeToString(pkeyBytes)

	// Peers
	clusterPeers := make([]string, len(cfg.Peers), len(cfg.Peers))
	for i := 0; i < len(cfg.Peers); i++ {
		clusterPeers[i] = cfg.Peers[i].String()
	}

	// Bootstrap peers
	bootstrap := make([]string, len(cfg.Bootstrap), len(cfg.Bootstrap))
	for i := 0; i < len(cfg.Bootstrap); i++ {
		bootstrap[i] = cfg.Bootstrap[i].String()
	}

	// Set all configuration fields
	jcfg.ID = cfg.ID.Pretty()
	jcfg.Peername = cfg.Peername
	jcfg.PrivateKey = pKey
	jcfg.Secret = EncodeProtectorKey(cfg.Secret)
	jcfg.Peers = clusterPeers
	jcfg.Bootstrap = bootstrap
	jcfg.ReplicationFactorMin = cfg.ReplicationFactorMin
	jcfg.ReplicationFactorMax = cfg.ReplicationFactorMax
	jcfg.LeaveOnShutdown = cfg.LeaveOnShutdown
	jcfg.ListenMultiaddress = cfg.ListenAddr.String()
	jcfg.StateSyncInterval = cfg.StateSyncInterval.String()
	jcfg.IPFSSyncInterval = cfg.IPFSSyncInterval.String()
	jcfg.MonitorPingInterval = cfg.MonitorPingInterval.String()
	jcfg.PeerWatchInterval = cfg.PeerWatchInterval.String()

	raw, err = json.MarshalIndent(jcfg, "", "    ")
	return
}

func (cfg *Config) savePeers(addrs []ma.Multiaddr) {
	cfg.lock.Lock()
	cfg.Peers = addrs
	cfg.lock.Unlock()
	cfg.NotifySave()
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
		logger.Warning("Cluster secret is empty, cluster will start on unprotected network.")
		return nil, nil
	case 32:
		return secret, nil
	default:
		return nil, fmt.Errorf("input secret is %d bytes, cluster secret should be 32", secretLen)
	}
}
