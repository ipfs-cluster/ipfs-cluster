package ipfscluster

import (
	"testing"

	cid "github.com/ipfs/go-cid"
)

func TestRPC(t *testing.T) {
	c, err := cid.Decode(testCid)
	if err != nil {
		t.Fatal(err)
	}
	crpc := RPC(IPFSPinRPC, c)
	_, ok := crpc.(*CidClusterRPC)
	if !ok {
		t.Error("expected a CidClusterRPC")
	}

	if crpc.Op() != IPFSPinRPC {
		t.Error("unexpected Op() type")
	}

	if crpc.ResponseCh() == nil {
		t.Error("should have made the ResponseCh")
	}

	grpc := RPC(MemberListRPC, 3)
	_, ok = grpc.(*GenericClusterRPC)
	if !ok {
		t.Error("expected a GenericClusterRPC")
	}

	if grpc.Op() != MemberListRPC {
		t.Error("unexpected Op() type")
	}

	if grpc.ResponseCh() == nil {
		t.Error("should have created the ResponseCh")
	}
}
