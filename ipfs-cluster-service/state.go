package main

import (
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"

	ipfscluster "github.com/ipfs/ipfs-cluster"
	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/consensus/raft"
	"github.com/ipfs/ipfs-cluster/state/mapstate"
)

var errNoSnapshot = errors.New("no snapshot found")

func upgrade() error {
	newState, err := restoreStateFromDisk()
	if err != nil {
		return err
	}

	cfg, clusterCfg, _, _, consensusCfg, _, _, _, _, _ := makeConfigs()

	err = cfg.LoadJSONFromFile(configPath)
	if err != nil {
		return err
	}

	raftPeers := append(ipfscluster.PeersFromMultiaddrs(clusterCfg.Peers), clusterCfg.ID)
	return raft.SnapshotSave(consensusCfg, newState, raftPeers)
}

func export(w io.Writer) error {
	stateToExport, err := restoreStateFromDisk()
	if err != nil {
		return err
	}

	return exportState(stateToExport, w)
}

func restoreStateFromDisk() (*mapstate.MapState, error) {
	cfg, _, _, _, consensusCfg, _, _, _, _, _ := makeConfigs()

	err := cfg.LoadJSONFromFile(configPath)
	if err != nil {
		return nil, err
	}

	r, snapExists, err := raft.LastStateRaw(consensusCfg)
	if !snapExists {
		err = errNoSnapshot
	}
	if err != nil {
		return nil, err
	}

	stateFromSnap := mapstate.NewMapState()
	err = stateFromSnap.Migrate(r)
	if err != nil {
		return nil, err
	}

	return stateFromSnap, nil

}

func stateImport(r io.Reader) error {
	cfg, clusterCfg, _, _, consensusCfg, _, _, _, _, _ := makeConfigs()

	err := cfg.LoadJSONFromFile(configPath)
	if err != nil {
		return err
	}

	pinSerials := make([]api.PinSerial, 0)
	dec := json.NewDecoder(r)
	err = dec.Decode(&pinSerials)
	if err != nil {
		return err
	}

	stateToImport := mapstate.NewMapState()
	for _, pS := range pinSerials {
		err = stateToImport.Add(pS.ToPin())
		if err != nil {
			return err
		}
	}
	raftPeers := append(ipfscluster.PeersFromMultiaddrs(clusterCfg.Peers), clusterCfg.ID)
	return raft.SnapshotSave(consensusCfg, stateToImport, raftPeers)
}

func validateVersion(cfg *ipfscluster.Config, cCfg *raft.Config) error {
	state := mapstate.NewMapState()
	r, snapExists, err := raft.LastStateRaw(cCfg)
	if !snapExists && err != nil {
		logger.Error("error before reading latest snapshot.")
	} else if snapExists && err != nil {
		logger.Error("error after reading last snapshot. Snapshot potentially corrupt.")
	} else if snapExists && err == nil {
		raw, err2 := ioutil.ReadAll(r)
		if err2 != nil {
			return err2
		}
		err2 = state.Unmarshal(raw)
		if err2 != nil {
			logger.Error("error unmarshalling snapshot. Snapshot potentially corrupt.")
			return err2
		}
		if state.GetVersion() != mapstate.Version {
			logger.Error("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
			logger.Error("Out of date ipfs-cluster state is saved.")
			logger.Error("To migrate to the new version, run ipfs-cluster-service state upgrade.")
			logger.Error("To launch a node without this state, rename the consensus data directory.")
			logger.Error("Hint, the default is .ipfs-cluster/ipfs-cluster-data.")
			logger.Error("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
			err = errors.New("outdated state version stored")
		}
	} // !snapExists && err == nil // no existing state, no check needed
	return err
}

// ExportState saves a json representation of a state
func exportState(state *mapstate.MapState, w io.Writer) error {
	// Serialize pins
	pins := state.List()
	pinSerials := make([]api.PinSerial, len(pins), len(pins))
	for i, pin := range pins {
		pinSerials[i] = pin.ToSerial()
	}

	// Write json to output file
	enc := json.NewEncoder(w)
	enc.SetIndent("", "    ")
	return enc.Encode(pinSerials)
}
