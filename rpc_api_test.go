package ipfscluster

import (
	"errors"
	"testing"
	"time"

	rpc "github.com/hsanjuan/go-libp2p-rpc"
	cid "github.com/ipfs/go-cid"
	peer "github.com/libp2p/go-libp2p-peer"
)

var errBadCid = errors.New("this is an expected error when using errorCid")

type mockService struct{}

func mockRPCClient(t *testing.T) *rpc.Client {
	s := rpc.NewServer(nil, "mock")
	c := rpc.NewClientWithServer(nil, "mock", s)
	err := s.RegisterName("Cluster", &mockService{})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (mock *mockService) Pin(in *CidArg, out *struct{}) error {
	if in.Cid == errorCid {
		return errBadCid
	}
	return nil
}

func (mock *mockService) Unpin(in *CidArg, out *struct{}) error {
	if in.Cid == errorCid {
		return errBadCid
	}
	return nil
}

func (mock *mockService) PinList(in struct{}, out *[]string) error {
	*out = []string{testCid, testCid2, testCid3}
	return nil
}

func (mock *mockService) Version(in struct{}, out *string) error {
	*out = "0.0.mock"
	return nil
}

func (mock *mockService) MemberList(in struct{}, out *[]peer.ID) error {
	*out = []peer.ID{testPeerID}
	return nil
}

func (mock *mockService) Status(in struct{}, out *[]GlobalPinInfo) error {
	c1, _ := cid.Decode(testCid1)
	c2, _ := cid.Decode(testCid2)
	c3, _ := cid.Decode(testCid3)
	*out = []GlobalPinInfo{
		GlobalPinInfo{
			Cid: c1,
			Status: map[peer.ID]PinInfo{
				testPeerID: PinInfo{
					CidStr: testCid1,
					Peer:   testPeerID,
					IPFS:   Pinned,
					TS:     time.Now(),
				},
			},
		},
		GlobalPinInfo{
			Cid: c2,
			Status: map[peer.ID]PinInfo{
				testPeerID: PinInfo{
					CidStr: testCid2,
					Peer:   testPeerID,
					IPFS:   Pinning,
					TS:     time.Now(),
				},
			},
		},
		GlobalPinInfo{
			Cid: c3,
			Status: map[peer.ID]PinInfo{
				testPeerID: PinInfo{
					CidStr: testCid3,
					Peer:   testPeerID,
					IPFS:   PinError,
					TS:     time.Now(),
				},
			},
		},
	}
	return nil
}

func (mock *mockService) StatusCid(in *CidArg, out *GlobalPinInfo) error {
	if in.Cid == errorCid {
		return errBadCid
	}
	c1, _ := cid.Decode(testCid1)
	*out = GlobalPinInfo{
		Cid: c1,
		Status: map[peer.ID]PinInfo{
			testPeerID: PinInfo{
				CidStr: testCid1,
				Peer:   testPeerID,
				IPFS:   Pinned,
				TS:     time.Now(),
			},
		},
	}
	return nil
}

func (mock *mockService) GlobalSync(in struct{}, out *[]GlobalPinInfo) error {
	return mock.Status(in, out)
}

func (mock *mockService) GlobalSyncCid(in *CidArg, out *GlobalPinInfo) error {
	return mock.StatusCid(in, out)
}

func (mock *mockService) StateSync(in struct{}, out *[]PinInfo) error {
	*out = []PinInfo{}
	return nil
}

func (mock *mockService) Track(in *CidArg, out *struct{}) error {
	return nil
}

func (mock *mockService) Untrack(in *CidArg, out *struct{}) error {
	return nil
}
