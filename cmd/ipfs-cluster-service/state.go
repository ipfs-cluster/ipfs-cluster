package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"

	ipfscluster "github.com/ipfs/ipfs-cluster"
	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/consensus/raft"
	"github.com/ipfs/ipfs-cluster/pstoremgr"
	"github.com/ipfs/ipfs-cluster/state/mapstate"
	"go.opencensus.io/trace"
)

var errNoSnapshot = errors.New("no snapshot found")

func upgrade(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "daemon/upgrade")
	defer span.End()

	newState, current, err := restoreStateFromDisk(ctx)
	if err != nil {
		return err
	}

	if current {
		logger.Warning("Skipping migration of up-to-date state")
		return nil
	}

	cfgMgr, cfgs := makeConfigs()

	err = cfgMgr.LoadJSONFromFile(configPath)
	if err != nil {
		return err
	}

	pm := pstoremgr.New(nil, cfgs.clusterCfg.GetPeerstorePath())
	raftPeers := append(ipfscluster.PeersFromMultiaddrs(pm.LoadPeerstore()), cfgs.clusterCfg.ID)
	return raft.SnapshotSave(cfgs.consensusCfg, newState, raftPeers)
}

func export(ctx context.Context, w io.Writer) error {
	ctx, span := trace.StartSpan(ctx, "daemon/export")
	defer span.End()

	stateToExport, _, err := restoreStateFromDisk(ctx)
	if err != nil {
		return err
	}

	return exportState(ctx, stateToExport, w)
}

// restoreStateFromDisk returns a mapstate containing the latest
// snapshot, a flag set to true when the state format has the
// current version and an error
func restoreStateFromDisk(ctx context.Context) (*mapstate.MapState, bool, error) {
	ctx, span := trace.StartSpan(ctx, "daemon/restoreStateFromDisk")
	defer span.End()

	cfgMgr, cfgs := makeConfigs()

	err := cfgMgr.LoadJSONFromFile(configPath)
	if err != nil {
		return nil, false, err
	}

	r, snapExists, err := raft.LastStateRaw(cfgs.consensusCfg)
	if !snapExists {
		err = errNoSnapshot
	}
	if err != nil {
		return nil, false, err
	}

	stateFromSnap := mapstate.NewMapState()
	// duplicate reader to both check version and migrate
	var buf bytes.Buffer
	r2 := io.TeeReader(r, &buf)
	raw, err := ioutil.ReadAll(r2)
	if err != nil {
		return nil, false, err
	}
	err = stateFromSnap.Unmarshal(raw)
	if err != nil {
		return nil, false, err
	}
	if stateFromSnap.GetVersion() == mapstate.Version {
		return stateFromSnap, true, nil
	}

	err = stateFromSnap.Migrate(ctx, &buf)
	if err != nil {
		return nil, false, err
	}

	return stateFromSnap, false, nil
}

func stateImport(ctx context.Context, r io.Reader) error {
	ctx, span := trace.StartSpan(ctx, "daemon/stateImport")
	defer span.End()

	cfgMgr, cfgs := makeConfigs()

	err := cfgMgr.LoadJSONFromFile(configPath)
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
		err = stateToImport.Add(ctx, pS.ToPin())
		if err != nil {
			return err
		}
	}

	pm := pstoremgr.New(nil, cfgs.clusterCfg.GetPeerstorePath())
	raftPeers := append(ipfscluster.PeersFromMultiaddrs(pm.LoadPeerstore()), cfgs.clusterCfg.ID)
	return raft.SnapshotSave(cfgs.consensusCfg, stateToImport, raftPeers)
}

func validateVersion(ctx context.Context, cfg *ipfscluster.Config, cCfg *raft.Config) error {
	ctx, span := trace.StartSpan(ctx, "daemon/validateVersion")
	defer span.End()

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
func exportState(ctx context.Context, state *mapstate.MapState, w io.Writer) error {
	ctx, span := trace.StartSpan(ctx, "daemon/exportState")
	defer span.End()

	// Serialize pins
	pins := state.List(ctx)
	pinSerials := make([]api.PinSerial, len(pins), len(pins))
	for i, pin := range pins {
		pinSerials[i] = pin.ToSerial()
	}

	// Write json to output file
	enc := json.NewEncoder(w)
	enc.SetIndent("", "    ")
	return enc.Encode(pinSerials)
}

// CleanupState cleans the state
func cleanupState(cCfg *raft.Config) error {
	err := raft.CleanupRaft(cCfg.GetDataFolder(), cCfg.BackupsRotate)
	if err == nil {
		logger.Warningf("the %s folder has been rotated.  Next start will use an empty state", cCfg.GetDataFolder())
	}

	return err
}
