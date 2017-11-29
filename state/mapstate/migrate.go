package mapstate

import (
	"bytes"
	"errors"

	msgpack "github.com/multiformats/go-multicodec/msgpack"

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
		buf := bytes.NewBuffer(snap)
		dec := msgpack.Multicodec(msgpack.DefaultMsgpackHandle()).Decoder(buf)
		if err := dec.Decode(&mstv1); err != nil {
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
