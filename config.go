package ipfscluster

import (
	"encoding/json"
	"io/ioutil"
)

type Config struct {
	IPFSHost             string   `json:"ipfs_host"`
	IPFSPort             int      `json:"ipfs_port"`
	APIListenAddr string   `json:"cluster_api_listen_addr"`
	APIListenPort int      `json:"cluster_api_listen_port"`
	IPFSAPIListenAddr    string   `json:"ipfs_api_listen_addr"`
	IPFSAPIListenPort    int      `json:"ipfs_api_listen_port"`
	ConsensusListenAddr  string   `json:"consensus_listen_addr"`
	ConsensusListenPort  int      `json:"consensus_listen_port"`
	ClusterPeers         []string `json:"cluster_peers"`
	ID                   string   `json:"id"`
	PrivateKey           string   `json:"private_key"`
	RaftFolder           string   `json:"raft_folder"`
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
