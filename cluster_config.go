package ipfscluster

import (
	"bytes"
	crand "crypto/rand"
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

	// ReplicationFactor indicates the number of nodes that must pin content.
	// For exampe, a replication_factor of 2 will prompt cluster to choose
	// two nodes for each pinned hash. A replication_factor -1 will
	// use every available node for each pin.
	ReplicationFactor int

	// MonitorPingInterval is frequency by which a cluster peer pings the
	// monitoring component. The ping metric has a TTL set to the double
	// of this value.
	MonitorPingInterval time.Duration
}

// configJSON represents a Cluster configuration as it will look when it is
// saved using JSON. Most configuration keys are converted into simple types
// like strings, and key names aim to be self-explanatory for the user.
type configJSON struct {
	ID                  string   `json:"id"`
	Peername            string   `json:"peername"`
	PrivateKey          string   `json:"private_key"`
	Secret              string   `json:"secret"`
	Peers               []string `json:"peers"`
	Bootstrap           []string `json:"bootstrap"`
	LeaveOnShutdown     bool     `json:"leave_on_shutdown"`
	ListenMultiaddress  string   `json:"listen_multiaddress"`
	StateSyncInterval   string   `json:"state_sync_interval"`
	IPFSSyncInterval    string   `json:"ipfs_sync_interval"`
	ReplicationFactor   int      `json:"replication_factor"`
	MonitorPingInterval string   `json:"monitor_ping_interval"`
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
	clusterSecret, err := generateClusterSecret()
	if err != nil {
		return err
	}
	cfg.Secret = clusterSecret
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

	if cfg.ReplicationFactor < -1 {
		return errors.New("cluster.replication_factor is invalid")
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
	cfg.ReplicationFactor = DefaultReplicationFactor
	cfg.MonitorPingInterval = DefaultMonitorPingInterval
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

	clusterPeers := make([]ma.Multiaddr, len(jcfg.Peers))
	for i := 0; i < len(jcfg.Peers); i++ {
		maddr, err2 := ma.NewMultiaddr(jcfg.Peers[i])
		if err2 != nil {
			err = fmt.Errorf("error parsing multiaddress for peer %s: %s",
				jcfg.Peers[i], err2)
			return err
		}
		clusterPeers[i] = maddr
	}
	cfg.Peers = clusterPeers

	bootstrap := make([]ma.Multiaddr, len(jcfg.Bootstrap))
	for i := 0; i < len(jcfg.Bootstrap); i++ {
		maddr, err2 := ma.NewMultiaddr(jcfg.Bootstrap[i])
		if err2 != nil {
			err = fmt.Errorf("error parsing multiaddress for peer %s: %s",
				jcfg.Bootstrap[i], err2)
			return err
		}
		bootstrap[i] = maddr
	}
	cfg.Bootstrap = bootstrap

	clusterAddr, err := ma.NewMultiaddr(jcfg.ListenMultiaddress)
	if err != nil {
		err = fmt.Errorf("error parsing cluster_listen_multiaddress: %s", err)
		return err
	}
	cfg.ListenAddr = clusterAddr

	if rf := jcfg.ReplicationFactor; rf == 0 {
		logger.Warning("Replication factor set to -1 (pin everywhere)")
		cfg.ReplicationFactor = -1
	} else {
		cfg.ReplicationFactor = rf
	}

	// Validation will detect problems here
	interval, _ := time.ParseDuration(jcfg.StateSyncInterval)
	cfg.StateSyncInterval = interval

	interval, _ = time.ParseDuration(jcfg.IPFSSyncInterval)
	cfg.IPFSSyncInterval = interval

	interval, _ = time.ParseDuration(jcfg.MonitorPingInterval)
	cfg.MonitorPingInterval = interval

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
	jcfg.Secret = EncodeClusterSecret(cfg.Secret)
	jcfg.Peers = clusterPeers
	jcfg.Bootstrap = bootstrap
	jcfg.ReplicationFactor = cfg.ReplicationFactor
	jcfg.LeaveOnShutdown = cfg.LeaveOnShutdown
	jcfg.ListenMultiaddress = cfg.ListenAddr.String()
	jcfg.StateSyncInterval = cfg.StateSyncInterval.String()
	jcfg.IPFSSyncInterval = cfg.IPFSSyncInterval.String()
	jcfg.MonitorPingInterval = cfg.MonitorPingInterval.String()

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

// EncodeClusterSecret converts a byte slice to its hex string representation.
func EncodeClusterSecret(secretBytes []byte) string {
	return hex.EncodeToString(secretBytes)
}

func generateClusterSecret() ([]byte, error) {
	secretBytes := make([]byte, 32)
	_, err := crand.Read(secretBytes)
	if err != nil {
		return nil, fmt.Errorf("error reading from rand: %v", err)
	}
	return secretBytes, nil
}

func clusterSecretToKey(secret []byte) (string, error) {
	var key bytes.Buffer
	key.WriteString("/key/swarm/psk/1.0.0/\n")
	key.WriteString("/base16/\n")
	key.WriteString(EncodeClusterSecret(secret))

	return key.String(), nil
}

// BackupState backs up a state according to this configuration's options
//func (cfg *Config) BackupState(state state.State) error {
//	if cfg.BaseDir == "" {
// 		msg := "ClusterConfig BaseDir unset. Skipping backup"
// 		logger.Warning(msg)
// 		return errors.New(msg)
// 	}

// 	folder := filepath.Join(cfg.BaseDir, "backups")
// 	err := os.MkdirAll(folder, 0700)
// 	if err != nil {
// 		logger.Error(err)
// 		logger.Error("skipping backup")
// 		return errors.New("skipping backup")
// 	}
// 	fname := time.Now().UTC().Format("20060102_15:04:05")
// 	f, err := os.Create(filepath.Join(folder, fname))
// 	if err != nil {
// 		logger.Error(err)
// 		return err
// 	}
// 	defer f.Close()
// 	err = state.Snapshot(f)
// 	if err != nil {
// 		logger.Error(err)
// 		return err
// 	}
// 	return nil
// }
