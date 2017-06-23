package ipfscluster

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"sync"

	crypto "gx/ipfs/QmNiCwBNA8MWDADTFVq1BonUEJbS2SvjAoNkZZrhEwcuUi/go-libp2p-crypto"
	ma "gx/ipfs/QmSWLfmj5frN9xVLMMN846dMDriy5wN5jeghUm7aTW3DAG/go-multiaddr"
	peer "gx/ipfs/QmZcUPvPhD1Xvk6mwijYF8AfR3mG31S1YsEfHG4khrFPRr/go-libp2p-peer"
)

// Default parameters for the configuration
const (
	DefaultConfigCrypto              = crypto.RSA
	DefaultConfigKeyLength           = 2048
	DefaultAPIAddr                   = "/ip4/127.0.0.1/tcp/9094"
	DefaultIPFSProxyAddr             = "/ip4/127.0.0.1/tcp/9095"
	DefaultIPFSNodeAddr              = "/ip4/127.0.0.1/tcp/5001"
	DefaultClusterAddr               = "/ip4/0.0.0.0/tcp/9096"
	DefaultStateSyncSeconds          = 60
	DefaultIPFSSyncSeconds           = 130
	DefaultMonitoringIntervalSeconds = 15
)

// Config represents an ipfs-cluster configuration. It is used by
// Cluster components. An initialized version of it can be obtained with
// NewDefaultConfig().
type Config struct {
	// Libp2p ID and private key for Cluster communication (including)
	// the Consensus component.
	ID         peer.ID
	PrivateKey crypto.PrivKey

	// Swarm key for private network
	SwarmKeyPath string
	// Fingerprint for private network
	PNetFingerprint []byte

	// ClusterPeers is the list of peers in the Cluster. They are used
	// as the initial peers in the consensus. When bootstrapping a peer,
	// ClusterPeers will be filled in automatically for the next run upon
	// shutdown.
	ClusterPeers []ma.Multiaddr

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
	ClusterAddr ma.Multiaddr

	// Listen parameters for the the Cluster HTTP API component.
	APIAddr ma.Multiaddr

	// Listen parameters for the IPFS Proxy. Used by the IPFS
	// connector component.
	IPFSProxyAddr ma.Multiaddr

	// Host/Port for the IPFS daemon.
	IPFSNodeAddr ma.Multiaddr

	// Storage folder for snapshots, log store etc. Used by
	// the Consensus component.
	ConsensusDataFolder string

	// Number of seconds between automatic calls to StateSync().
	StateSyncSeconds int

	// Number of seconds between automatic calls to SyncAllLocal().
	IPFSSyncSeconds int

	// ReplicationFactor is the number of copies we keep for each pin
	ReplicationFactor int

	// MonitoringIntervalSeconds is the number of seconds that can
	// pass before a peer can be detected as down.
	MonitoringIntervalSeconds int

	// AllocationStrategy is used to decide on the
	// Informer/Allocator implementation to use.
	AllocationStrategy string

	// if a config has been loaded from disk, track the path
	// so it can be saved to the same place.
	path string

	saveMux sync.Mutex

	shadow *Config
}

// JSONConfig represents a Cluster configuration as it will look when it is
// saved using JSON. Most configuration keys are converted into simple types
// like strings, and key names aim to be self-explanatory for the user.
type JSONConfig struct {
	// Libp2p ID and private key for Cluster communication (including)
	// the Consensus component.
	ID         string `json:"id"`
	PrivateKey string `json:"private_key"`

	// ClusterPeers is the list of peers' multiaddresses in the Cluster.
	// They are used as the initial peers in the consensus. When
	// bootstrapping a peer, ClusterPeers will be filled in automatically.
	ClusterPeers []string `json:"cluster_peers"`

	// Bootstrap peers multiaddresses. This peer will attempt to
	// join the clusters of the peers in the list. ONLY when ClusterPeers
	// is empty. Otherwise it is ignored. Leave empty for a single-peer
	// cluster.
	Bootstrap []string `json:"bootstrap"`

	// Leave Cluster on shutdown. Politely informs other peers
	// of the departure and removes itself from the consensus
	// peer set. The Cluster size will be reduced by one.
	LeaveOnShutdown bool `json:"leave_on_shutdown"`

	// Listen address for the Cluster libp2p host. This is used for
	// interal RPC and Consensus communications between cluster peers.
	ClusterListenMultiaddress string `json:"cluster_multiaddress"`

	// Listen address for the the Cluster HTTP API component.
	// Tools like ipfs-cluster-ctl will connect to his endpoint to
	// manage cluster.
	APIListenMultiaddress string `json:"api_listen_multiaddress"`

	// Listen address for the IPFS Proxy, which forwards requests to
	// an IPFS daemon.
	IPFSProxyListenMultiaddress string `json:"ipfs_proxy_listen_multiaddress"`

	// API address for the IPFS daemon.
	IPFSNodeMultiaddress string `json:"ipfs_node_multiaddress"`

	// Storage folder for snapshots, log store etc. Used by
	// the Consensus component.
	ConsensusDataFolder string `json:"consensus_data_folder"`

	// Number of seconds between syncs of the consensus state to the
	// tracker state. Normally states are synced anyway, but this helps
	// when new nodes are joining the cluster
	StateSyncSeconds int `json:"state_sync_seconds"`

	// Number of seconds between syncs of the local state and
	// the state of the ipfs daemon. This ensures that cluster
	// provides the right status for tracked items (for example
	// to detect that a pin has been removed.
	IPFSSyncSeconds int `json:"ipfs_sync_seconds"`

	// ReplicationFactor indicates the number of nodes that must pin content.
	// For exampe, a replication_factor of 2 will prompt cluster to choose
	// two nodes for each pinned hash. A replication_factor -1 will
	// use every available node for each pin.
	ReplicationFactor int `json:"replication_factor"`

	// Number of seconds between monitoring checks which detect
	// if a peer is down and consenquently trigger a rebalance
	MonitoringIntervalSeconds int `json:"monitoring_interval_seconds"`

	// AllocationStrategy is used to set how pins are allocated to
	// different Cluster peers. Currently supports "reposize" and "pincount"
	// values.
	AllocationStrategy string `json:"allocation_strategy"`
}

// ToJSONConfig converts a Config object to its JSON representation which
// is focused on user presentation and easy understanding.
func (cfg *Config) ToJSONConfig() (j *JSONConfig, err error) {
	// Multiaddress String() may panic
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%s", r)
		}
	}()
	pkeyBytes, err := cfg.PrivateKey.Bytes()
	if err != nil {
		return
	}
	pKey := base64.StdEncoding.EncodeToString(pkeyBytes)

	clusterPeers := make([]string, len(cfg.ClusterPeers), len(cfg.ClusterPeers))
	for i := 0; i < len(cfg.ClusterPeers); i++ {
		clusterPeers[i] = cfg.ClusterPeers[i].String()
	}

	bootstrap := make([]string, len(cfg.Bootstrap), len(cfg.Bootstrap))
	for i := 0; i < len(cfg.Bootstrap); i++ {
		bootstrap[i] = cfg.Bootstrap[i].String()
	}

	j = &JSONConfig{
		ID:                          cfg.ID.Pretty(),
		PrivateKey:                  pKey,
		ClusterPeers:                clusterPeers,
		Bootstrap:                   bootstrap,
		LeaveOnShutdown:             cfg.LeaveOnShutdown,
		ClusterListenMultiaddress:   cfg.ClusterAddr.String(),
		APIListenMultiaddress:       cfg.APIAddr.String(),
		IPFSProxyListenMultiaddress: cfg.IPFSProxyAddr.String(),
		IPFSNodeMultiaddress:        cfg.IPFSNodeAddr.String(),
		ConsensusDataFolder:         cfg.ConsensusDataFolder,
		StateSyncSeconds:            cfg.StateSyncSeconds,
		IPFSSyncSeconds:             cfg.IPFSSyncSeconds,
		ReplicationFactor:           cfg.ReplicationFactor,
		MonitoringIntervalSeconds:   cfg.MonitoringIntervalSeconds,
		AllocationStrategy:          cfg.AllocationStrategy,
	}
	return
}

// ToConfig converts a JSONConfig to its internal Config representation,
// where options are parsed into their native types.
func (jcfg *JSONConfig) ToConfig() (c *Config, err error) {
	id, err := peer.IDB58Decode(jcfg.ID)
	if err != nil {
		err = fmt.Errorf("error decoding cluster ID: %s", err)
		return
	}

	pkb, err := base64.StdEncoding.DecodeString(jcfg.PrivateKey)
	if err != nil {
		err = fmt.Errorf("error decoding private_key: %s", err)
		return
	}
	pKey, err := crypto.UnmarshalPrivateKey(pkb)
	if err != nil {
		err = fmt.Errorf("error parsing private_key ID: %s", err)
		return
	}

	clusterPeers := make([]ma.Multiaddr, len(jcfg.ClusterPeers))
	for i := 0; i < len(jcfg.ClusterPeers); i++ {
		maddr, err := ma.NewMultiaddr(jcfg.ClusterPeers[i])
		if err != nil {
			err = fmt.Errorf("error parsing multiaddress for peer %s: %s",
				jcfg.ClusterPeers[i], err)
			return nil, err
		}
		clusterPeers[i] = maddr
	}

	bootstrap := make([]ma.Multiaddr, len(jcfg.Bootstrap))
	for i := 0; i < len(jcfg.Bootstrap); i++ {
		maddr, err := ma.NewMultiaddr(jcfg.Bootstrap[i])
		if err != nil {
			err = fmt.Errorf("error parsing multiaddress for peer %s: %s",
				jcfg.Bootstrap[i], err)
			return nil, err
		}
		bootstrap[i] = maddr
	}

	clusterAddr, err := ma.NewMultiaddr(jcfg.ClusterListenMultiaddress)
	if err != nil {
		err = fmt.Errorf("error parsing cluster_listen_multiaddress: %s", err)
		return
	}

	apiAddr, err := ma.NewMultiaddr(jcfg.APIListenMultiaddress)
	if err != nil {
		err = fmt.Errorf("error parsing api_listen_multiaddress: %s", err)
		return
	}
	ipfsProxyAddr, err := ma.NewMultiaddr(jcfg.IPFSProxyListenMultiaddress)
	if err != nil {
		err = fmt.Errorf("error parsing ipfs_proxy_listen_multiaddress: %s", err)
		return
	}
	ipfsNodeAddr, err := ma.NewMultiaddr(jcfg.IPFSNodeMultiaddress)
	if err != nil {
		err = fmt.Errorf("error parsing ipfs_node_multiaddress: %s", err)
		return
	}

	if jcfg.ReplicationFactor == 0 {
		logger.Warning("Replication factor set to -1 (pin everywhere)")
		jcfg.ReplicationFactor = -1
	}

	if jcfg.StateSyncSeconds <= 0 {
		jcfg.StateSyncSeconds = DefaultStateSyncSeconds
	}

	if jcfg.IPFSSyncSeconds <= 0 {
		jcfg.IPFSSyncSeconds = DefaultIPFSSyncSeconds
	}

	if jcfg.MonitoringIntervalSeconds <= 0 {
		jcfg.MonitoringIntervalSeconds = DefaultMonitoringIntervalSeconds
	}

	if jcfg.AllocationStrategy == "" {
		jcfg.AllocationStrategy = "reposize"
	}

	c = &Config{
		ID:                        id,
		PrivateKey:                pKey,
		ClusterPeers:              clusterPeers,
		Bootstrap:                 bootstrap,
		LeaveOnShutdown:           jcfg.LeaveOnShutdown,
		ClusterAddr:               clusterAddr,
		APIAddr:                   apiAddr,
		IPFSProxyAddr:             ipfsProxyAddr,
		IPFSNodeAddr:              ipfsNodeAddr,
		ConsensusDataFolder:       jcfg.ConsensusDataFolder,
		StateSyncSeconds:          jcfg.StateSyncSeconds,
		IPFSSyncSeconds:           jcfg.IPFSSyncSeconds,
		ReplicationFactor:         jcfg.ReplicationFactor,
		MonitoringIntervalSeconds: jcfg.MonitoringIntervalSeconds,
		AllocationStrategy:        jcfg.AllocationStrategy,
	}
	return
}

// LoadConfig reads a JSON configuration file from the given path,
// parses it and returns a new Config object.
func LoadConfig(path string) (*Config, error) {
	jcfg := &JSONConfig{}
	file, err := ioutil.ReadFile(path)
	if err != nil {
		logger.Error("error reading the configuration file: ", err)
		return nil, err
	}
	err = json.Unmarshal(file, jcfg)
	if err != nil {
		logger.Error("error parsing JSON: ", err)
		return nil, err
	}
	cfg, err := jcfg.ToConfig()
	if err != nil {
		logger.Error("error parsing configuration: ", err)
		return nil, err
	}
	cfg.path = path
	return cfg, nil
}

func loadSwarmKey(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		} else {
			return nil, err
		}
	}
	defer f.Close()
	if err != nil {
		return nil, err
	}

	return ioutil.ReadAll(f)
}

// Save stores a configuration as a JSON file in the given path.
// If no path is provided, it uses the path the configuration was
// loaded from.
func (cfg *Config) Save(path string) error {
	cfg.saveMux.Lock()
	defer cfg.saveMux.Unlock()

	cfg.unshadow()

	if path == "" {
		path = cfg.path
	}

	logger.Info("Saving configuration")
	jcfg, err := cfg.ToJSONConfig()
	if err != nil {
		logger.Error("error generating JSON config")
		return err
	}
	json, err := json.MarshalIndent(jcfg, "", "    ")
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(path, json, 0600)
	return err
}

// Shadow copies certain configuration values to a shadow configuration
// object. From the moment Shadow is called, configuration keys which are
// shadowed may be changed, but Save() will restore them to the original
// value stored in the shadow. Currently affects:
//
//   - AllocationStrategy
//   - LeaveOnShutdown
//   - Bootstrap
//
// which are options which can be overriden via ipfs-cluster-service flags.
// The shadow dissapears with every call to Save().
func (cfg *Config) Shadow() {
	cfg.shadow = &Config{
		AllocationStrategy: cfg.AllocationStrategy,
		LeaveOnShutdown:    cfg.LeaveOnShutdown,
		Bootstrap:          cfg.Bootstrap,
	}
}

func (cfg *Config) unshadow() {
	if cfg.shadow != nil {
		cfg.AllocationStrategy = cfg.shadow.AllocationStrategy
		cfg.LeaveOnShutdown = cfg.shadow.LeaveOnShutdown
		cfg.Bootstrap = cfg.shadow.Bootstrap
	}
	cfg.shadow = nil
}

// NewDefaultConfig returns a default configuration object with a randomly
// generated ID and private key.
func NewDefaultConfig() (*Config, error) {
	priv, pub, err := crypto.GenerateKeyPair(
		DefaultConfigCrypto,
		DefaultConfigKeyLength)
	if err != nil {
		return nil, err
	}
	pid, err := peer.IDFromPublicKey(pub)
	if err != nil {
		return nil, err
	}

	clusterAddr, _ := ma.NewMultiaddr(DefaultClusterAddr)
	apiAddr, _ := ma.NewMultiaddr(DefaultAPIAddr)
	ipfsProxyAddr, _ := ma.NewMultiaddr(DefaultIPFSProxyAddr)
	ipfsNodeAddr, _ := ma.NewMultiaddr(DefaultIPFSNodeAddr)

	return &Config{
		ID:                        pid,
		PrivateKey:                priv,
		SwarmKeyPath:              "",
		ClusterPeers:              []ma.Multiaddr{},
		Bootstrap:                 []ma.Multiaddr{},
		LeaveOnShutdown:           false,
		ClusterAddr:               clusterAddr,
		APIAddr:                   apiAddr,
		IPFSProxyAddr:             ipfsProxyAddr,
		IPFSNodeAddr:              ipfsNodeAddr,
		ConsensusDataFolder:       "ipfscluster-data",
		StateSyncSeconds:          DefaultStateSyncSeconds,
		IPFSSyncSeconds:           DefaultIPFSSyncSeconds,
		ReplicationFactor:         -1,
		MonitoringIntervalSeconds: DefaultMonitoringIntervalSeconds,
		AllocationStrategy:        "reposize",
	}, nil
}
