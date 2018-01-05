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
	mst2.PinMap = make(map[string]api.PinSerial)
	for k := range st.PinMap {
		mst2.PinMap[k] = api.PinSerial{
			Cid:               k,
			Allocations:       []string{},
			ReplicationFactor: -1,
		}
	}
	return &mst2
}

type mapStateV2 struct {
	PinMap  map[string]api.PinSerial
	Version int
}

func (st *mapStateV2) unmarshal(bs []byte) error {
	buf := bytes.NewBuffer(bs)
	dec := msgpack.Multicodec(msgpack.DefaultMsgpackHandle()).Decoder(buf)
	return dec.Decode(st)
}

// No migration possible, v2 is the latest state
func (st *mapStateV2) next() migrateable {
	return nil
}

func finalCopy(st *MapState, internal *mapStateV2) {
	for k := range internal.PinMap {
		st.PinMap[k] = internal.PinMap[k]
	}
}

func (st *MapState) migrateFrom(version int, snap []byte) error {
	var m, next migrateable
	switch version {
	case 1:
		var mst1 mapStateV1
		m = &mst1
		break

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
			mst2, ok := m.(*mapStateV2)
			if !ok {
				return errors.New("migration ended prematurely")
			}
			finalCopy(st, mst2)
			return nil
		}
		m = next
	}
}
