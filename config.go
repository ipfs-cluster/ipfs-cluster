package ipfscluster

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"sync"

	crypto "github.com/libp2p/go-libp2p-crypto"
	peer "github.com/libp2p/go-libp2p-peer"
	ma "github.com/multiformats/go-multiaddr"
)

// Default parameters for the configuration
const (
	DefaultConfigCrypto     = crypto.RSA
	DefaultConfigKeyLength  = 2048
	DefaultAPIAddr          = "/ip4/127.0.0.1/tcp/9094"
	DefaultIPFSProxyAddr    = "/ip4/127.0.0.1/tcp/9095"
	DefaultIPFSNodeAddr     = "/ip4/127.0.0.1/tcp/5001"
	DefaultClusterAddr      = "/ip4/0.0.0.0/tcp/9096"
	DefaultStateSyncSeconds = 15
)

// Config represents an ipfs-cluster configuration. It is used by
// Cluster components. An initialized version of it can be obtained with
// NewDefaultConfig().
type Config struct {
	// Libp2p ID and private key for Cluster communication (including)
	// the Consensus component.
	ID         peer.ID
	PrivateKey crypto.PrivKey

	// List of multiaddresses of the peers of this cluster.
	ClusterPeers []ma.Multiaddr
	pMux         sync.Mutex

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

	// Number of seconds between StateSync() operations
	StateSyncSeconds int

	// if a config has been loaded from disk, track the path
	// so it can be saved to the same place.
	path string
}

// JSONConfig represents a Cluster configuration as it will look when it is
// saved using JSON. Most configuration keys are converted into simple types
// like strings, and key names aim to be self-explanatory for the user.
type JSONConfig struct {
	// Libp2p ID and private key for Cluster communication (including)
	// the Consensus component.
	ID         string `json:"id"`
	PrivateKey string `json:"private_key"`

	// List of multiaddresses of the peers of this cluster. This list may
	// include the multiaddress of this node.
	ClusterPeers []string `json:"cluster_peers"`

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

	cfg.pMux.Lock()
	clusterPeers := make([]string, len(cfg.ClusterPeers), len(cfg.ClusterPeers))
	for i := 0; i < len(cfg.ClusterPeers); i++ {
		clusterPeers[i] = cfg.ClusterPeers[i].String()
	}
	cfg.pMux.Unlock()

	j = &JSONConfig{
		ID:                          cfg.ID.Pretty(),
		PrivateKey:                  pKey,
		ClusterPeers:                clusterPeers,
		ClusterListenMultiaddress:   cfg.ClusterAddr.String(),
		APIListenMultiaddress:       cfg.APIAddr.String(),
		IPFSProxyListenMultiaddress: cfg.IPFSProxyAddr.String(),
		IPFSNodeMultiaddress:        cfg.IPFSNodeAddr.String(),
		ConsensusDataFolder:         cfg.ConsensusDataFolder,
		StateSyncSeconds:            cfg.StateSyncSeconds,
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

	if jcfg.StateSyncSeconds <= 0 {
		jcfg.StateSyncSeconds = DefaultStateSyncSeconds
	}

	c = &Config{
		ID:                  id,
		PrivateKey:          pKey,
		ClusterPeers:        clusterPeers,
		ClusterAddr:         clusterAddr,
		APIAddr:             apiAddr,
		IPFSProxyAddr:       ipfsProxyAddr,
		IPFSNodeAddr:        ipfsNodeAddr,
		ConsensusDataFolder: jcfg.ConsensusDataFolder,
		StateSyncSeconds:    jcfg.StateSyncSeconds,
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

// Save stores a configuration as a JSON file in the given path.
func (cfg *Config) Save(path string) error {
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
		ID:                  pid,
		PrivateKey:          priv,
		ClusterPeers:        []ma.Multiaddr{},
		ClusterAddr:         clusterAddr,
		APIAddr:             apiAddr,
		IPFSProxyAddr:       ipfsProxyAddr,
		IPFSNodeAddr:        ipfsNodeAddr,
		ConsensusDataFolder: "ipfscluster-data",
		StateSyncSeconds:    DefaultStateSyncSeconds,
	}, nil
}

func (cfg *Config) addPeer(addr ma.Multiaddr) {
	cfg.pMux.Lock()
	defer cfg.pMux.Unlock()
	found := false
	for _, cpeer := range cfg.ClusterPeers {
		if cpeer.Equal(addr) {
			found = true
		}
	}
	if !found {
		cfg.ClusterPeers = append(cfg.ClusterPeers, addr)
	}
	logger.Debugf("add: cluster peers are now: %s", cfg.ClusterPeers)
}

func (cfg *Config) rmPeer(p peer.ID) {
	cfg.pMux.Lock()
	defer cfg.pMux.Unlock()
	foundPos := -1
	for i, addr := range cfg.ClusterPeers {
		cp, _, _ := multiaddrSplit(addr)
		if cp == p {
			foundPos = i
		}
	}
	if foundPos < 0 {
		return
	}

	// Delete preserving order
	copy(cfg.ClusterPeers[foundPos:], cfg.ClusterPeers[foundPos+1:])
	cfg.ClusterPeers[len(cfg.ClusterPeers)-1] = nil // or the zero value of T
	cfg.ClusterPeers = cfg.ClusterPeers[:len(cfg.ClusterPeers)-1]
	logger.Debugf("rm: cluster peers are now: %s", cfg.ClusterPeers)
}

func (cfg *Config) emptyPeers() {
	cfg.pMux.Lock()
	defer cfg.pMux.Unlock()
	cfg.ClusterPeers = []ma.Multiaddr{}
}
