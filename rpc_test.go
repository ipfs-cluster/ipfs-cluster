package ipfscluster

import (
	"testing"

	cid "github.com/ipfs/go-cid"
)

func TestNewRPC(t *testing.T) {
	c, err := cid.Decode(testCid)
	if err != nil {
		t.Fatal(err)
	}
	crpc := NewRPC(IPFSPinRPC, c)
	_, ok := crpc.(*CidRPC)
	if !ok {
		t.Error("expected a CidRPC")
	}

	if crpc.Op() != IPFSPinRPC {
		t.Error("unexpected Op() type")
	}

	if crpc.ResponseCh() == nil {
		t.Error("should have made the ResponseCh")
	}

	grpc := NewRPC(MemberListRPC, 3)
	_, ok = grpc.(*GenericRPC)
	if !ok {
		t.Error("expected a GenericRPC")
	}

	if grpc.Op() != MemberListRPC {
		t.Error("unexpected Op() type")
	}

	if grpc.ResponseCh() == nil {
		t.Error("should have created the ResponseCh")
	}
}
