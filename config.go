package ipfscluster

import (
	"encoding/base64"
	"encoding/json"
	"io/ioutil"

	crypto "github.com/libp2p/go-libp2p-crypto"
	peer "github.com/libp2p/go-libp2p-peer"
)

// Default parameters for the configuration
const (
	DefaultConfigCrypto    = crypto.RSA
	DefaultConfigKeyLength = 2048
	DefaultAPIAddr         = "127.0.0.1"
	DefaultAPIPort         = 9094
	DefaultIPFSAPIAddr     = "127.0.0.1"
	DefaultIPFSAPIPort     = 9095
	DefaultIPFSAddr        = "127.0.0.1"
	DefaultIPFSPort        = 5001
	DefaultClusterAddr     = "0.0.0.0"
	DefaultClusterPort     = 9096
)

type Config struct {
	// Libp2p ID and private key for Cluster communication (including)
	// the Consensus component.
	ID         string `json:"id"`
	PrivateKey string `json:"private_key"`

	// List of multiaddresses of the peers of this cluster.
	ClusterPeers []string `json:"cluster_peers"`

	// Listen parameters for the Cluster libp2p Host. Used by
	// the Remote RPC and Consensus components.
	ClusterAddr string `json:"cluster_addr"`
	ClusterPort int    `json:"cluster_port"`

	// Storage folder for snapshots, log store etc. Used by
	// the Consensus component.
	ConsensusDataFolder string `json:"consensus_data_folder"`

	// Listen parameters for the the Cluster HTTP API component.
	APIAddr string `json:"api_addr"`
	APIPort int    `json:"api_port"`

	// Listen parameters for the IPFS Proxy. Used by the IPFS
	// connector component.
	IPFSAPIAddr string `json:"ipfs_api_addr"`
	IPFSAPIPort int    `json:"ipfs_api_port"`

	// Host/Port for the IPFS daemon.
	IPFSAddr string `json:"ipfs_addr"`
	IPFSPort int    `json:"ipfs_port"`
}

func LoadConfig(path string) (*Config, error) {
	config := &Config{}
	file, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	json.Unmarshal(file, config)
	return config, nil
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
	privBytes, err := priv.Bytes()
	if err != nil {
		return nil, err
	}
	b64priv := base64.StdEncoding.EncodeToString(privBytes)

	return &Config{
		ID:                  peer.IDB58Encode(pid),
		PrivateKey:          b64priv,
		ClusterPeers:        []string{},
		ClusterAddr:         DefaultClusterAddr,
		ClusterPort:         DefaultClusterPort,
		ConsensusDataFolder: "ipfscluster-data",
		APIAddr:             DefaultAPIAddr,
		APIPort:             DefaultAPIPort,
		IPFSAPIAddr:         DefaultIPFSAPIAddr,
		IPFSAPIPort:         DefaultIPFSAPIPort,
		IPFSAddr:            DefaultIPFSAddr,
		IPFSPort:            DefaultIPFSPort,
	}, nil
}

func (c *Config) Save(path string) error {
	json, err := json.MarshalIndent(c, "", "    ")
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(path, json, 0600)
	return err
}
