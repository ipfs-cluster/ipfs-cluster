package mapstate

// To add a new state format
// - implement the previous format's "next" function to the new format
// - implement the new format's unmarshal function
// - add a case to the switch statement for the previous format version
// - update the code copying the from mapStateVx to mapState
import (
	"bytes"
	"errors"

	msgpack "github.com/multiformats/go-multicodec/msgpack"

	"github.com/ipfs/ipfs-cluster/api"
)

// Instances of migrateable can be read from a serialized format and migrated
// to other state formats
type migrateable interface {
	next() migrateable
	unmarshal([]byte) error
}

/* V1 */

type mapStateV1 struct {
	Version int
	PinMap  map[string]struct{}
}

// Unmarshal the serialization of a v1 state
func (st *mapStateV1) unmarshal(bs []byte) error {
	buf := bytes.NewBuffer(bs)
	dec := msgpack.Multicodec(msgpack.DefaultMsgpackHandle()).Decoder(buf)
	return dec.Decode(st)
}

// Migrate from v1 to v2
func (st *mapStateV1) next() migrateable {
	var mst2 mapStateV2
	mst2.PinMap = make(map[string]pinSerialV2)
	for k := range st.PinMap {
		mst2.PinMap[k] = pinSerialV2{
			Cid:               k,
			Allocations:       []string{},
			ReplicationFactor: -1,
		}
	}
	return &mst2
}

/* V2 */

type pinSerialV2 struct {
	Cid               string   `json:"cid"`
	Name              string   `json:"name"`
	Allocations       []string `json:"allocations"`
	ReplicationFactor int      `json:"replication_factor"`
}

type mapStateV2 struct {
	PinMap  map[string]pinSerialV2
	Version int
}

func (st *mapStateV2) unmarshal(bs []byte) error {
	buf := bytes.NewBuffer(bs)
	dec := msgpack.Multicodec(msgpack.DefaultMsgpackHandle()).Decoder(buf)
	return dec.Decode(st)
}

func (st *mapStateV2) next() migrateable {
	var mst3 mapStateV3
	mst3.PinMap = make(map[string]pinSerialV3)
	for k, v := range st.PinMap {
		mst3.PinMap[k] = pinSerialV3{
			Cid:                  v.Cid,
			Name:                 v.Name,
			Allocations:          v.Allocations,
			ReplicationFactorMin: v.ReplicationFactor,
			ReplicationFactorMax: v.ReplicationFactor,
		}
	}
	return &mst3
}

/* V3 */

type pinSerialV3 struct {
	Cid                  string   `json:"cid"`
	Name                 string   `json:"name"`
	Allocations          []string `json:"allocations"`
	ReplicationFactorMin int      `json:"replication_factor_min"`
	ReplicationFactorMax int      `json:"replication_factor_max"`
}

type mapStateV3 struct {
	PinMap  map[string]pinSerialV3
	Version int
}

func (st *mapStateV3) unmarshal(bs []byte) error {
	buf := bytes.NewBuffer(bs)
	dec := msgpack.Multicodec(msgpack.DefaultMsgpackHandle()).Decoder(buf)
	return dec.Decode(st)
}

func (st *mapStateV3) next() migrateable {
	var mst4 mapStateV4
	mst4.PinMap = make(map[string]api.PinSerial)
	for k, v := range st.PinMap {
		mst4.PinMap[k] = api.PinSerial{
			Cid:                  v.Cid,
			Name:                 v.Name,
			Allocations:          v.Allocations,
			ReplicationFactorMin: v.ReplicationFactorMin,
			ReplicationFactorMax: v.ReplicationFactorMax,
			Recursive:            true,
			Type:                 api.DataType,
			Parents:              nil,
			Clusterdag:           "",
		}
	}
	return &mst4
}

/* V4 */

type mapStateV4 struct {
	PinMap  map[string]api.PinSerial
	Version int
}

func (st *mapStateV4) unmarshal(bs []byte) error {
	buf := bytes.NewBuffer(bs)
	dec := msgpack.Multicodec(msgpack.DefaultMsgpackHandle()).Decoder(buf)
	return dec.Decode(st)
}

func (st *mapStateV4) next() migrateable {
	return nil
}

func finalCopy(st *MapState, internal *mapStateV4) {
	for k, v := range internal.PinMap {
		st.PinMap[k] = v
	}
}

func (st *MapState) migrateFrom(version int, snap []byte) error {
	var m, next migrateable
	switch version {
	case 1:
		var mst1 mapStateV1
		m = &mst1
	case 2:
		var mst2 mapStateV2
		m = &mst2
	case 3:
		var mst3 mapStateV3
		m = &mst3
	default:
		return errors.New("version migration not supported")
	}

	err := m.unmarshal(snap)
	if err != nil {
		return err
	}

	for {
		next = m.next()
		if next == nil {
			mst4, ok := m.(*mapStateV4)
			if !ok {
				return errors.New("migration ended prematurely")
			}
			finalCopy(st, mst4)
			return nil
		}
		m = next
	}
}
