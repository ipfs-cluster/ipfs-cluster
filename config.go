package ipfscluster

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
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
	fmt.Println(path)
	config := &Config{}
	file, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	json.Unmarshal(file, config)
	fmt.Printf("%+v", config)
	return config, nil
}
