package main

import (
	"errors"
	"io/ioutil"

	ipfscluster "github.com/ipfs/ipfs-cluster"
	"github.com/ipfs/ipfs-cluster/consensus/raft"
	"github.com/ipfs/ipfs-cluster/state/mapstate"
)

func upgrade() error  {
	// Load configs
	cfg, clusterCfg, _, _, consensusCfg, _, _, _, _ := makeConfigs()
	
	// Execution lock
	err := locker.lock()
	if err != nil {
		return err
	}
	defer locker.tryUnlock()

	err = cfg.LoadJSONFromFile(configPath)
	if err != nil {
		return err
	}

	newState := mapstate.NewMapState()

	// Get the last state
	r, snapExists, err := raft.LastStateRaw(consensusCfg)
	if err != nil {
		return err
	}
	if !snapExists {
		logger.Error("no raft state currently exists to upgrade from")
		return errors.New("no snapshot could be found")
	}

	// Restore the state from snapshot
	err = newState.Restore(r)
	if err != nil {
		return err
	}

	// Reset with SnapshotSave
	err = raft.SnapshotSave(consensusCfg, newState, clusterCfg.ID)
	if err != nil {
		return err
	}
	return nil
}

func validateVersion(cfg *ipfscluster.Config, cCfg *raft.Config) error {
	state := mapstate.NewMapState()
	r, snapExists, err := raft.LastStateRaw(cCfg)
	if !snapExists && err != nil {
		logger.Error("error before reading latest snapshot.")
		return err
	} else if snapExists && err != nil {
		logger.Error("error after reading last snapshot. Snapshot potentially corrupt.")
		return err
	} else if snapExists && err == nil {
		raw, err := ioutil.ReadAll(r)
		if err != nil {
			return err
		}
		err = state.Unmarshal(raw)
		if err != nil {
			logger.Error("error unmarshalling snapshot. Snapshot potentially corrupt.")
			return err
		}
		if state.GetVersion() != state.Version {
			logger.Error("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
			logger.Error("Out of date ipfs-cluster state is saved.")
			logger.Error("To migrate to the new version, run ipfs-cluster-service state upgrade.")
			logger.Error("To launch a node without this state, rename the consensus data directory.")
			logger.Error("Hint, the default is .ipfs-cluster/ipfs-cluster-data.")
			logger.Error("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
			return errors.New("outdated state version stored")
		}
	} // !snapExists && err == nil // no existing state, no check needed
	return nil
}
