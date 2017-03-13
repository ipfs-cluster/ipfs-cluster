package mapstate

import (
	"encoding/json"
	"errors"

	"github.com/ipfs/ipfs-cluster/api"
)

type mapStateV1 struct {
	Version int
	PinMap  map[string]struct{}
}

func (st *MapState) migrateFrom(version int, snap []byte) error {
	switch version {
	case 1:
		var mstv1 mapStateV1
		err := json.Unmarshal(snap, &mstv1)
		if err != nil {
			return err
		}
		for k := range mstv1.PinMap {
			st.PinMap[k] = api.PinSerial{
				Cid:               k,
				Allocations:       []string{},
				ReplicationFactor: -1,
			}
		}
		return nil
	default:
		return errors.New("version migration not supported")
	}
}
