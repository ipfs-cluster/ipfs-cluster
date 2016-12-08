package ipfscluster

import (
	"context"
	"strings"
	"testing"
	"time"
)

var (
	testCid  = "QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmmq"
	testCid2 = "QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmma"
	testCid3 = "QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmmb"
)

func TestMakeRPC(t *testing.T) {
	testCh := make(chan ClusterRPC, 1)
	testReq := RPC(MemberListRPC, nil)
	testResp := RPCResponse{
		Data:  "hey",
		Error: nil,
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Test wait for response
	go func() {
		req := <-testCh
		if req.Op() != MemberListRPC {
			t.Fatal("expected a MemberListRPC")
		}
		req.ResponseCh() <- testResp
	}()

	resp := MakeRPC(ctx, testCh, testReq, true)
	data, ok := resp.Data.(string)
	if !ok || data != "hey" {
		t.Error("bad response")
	}

	// Test not waiting for response
	resp = MakeRPC(ctx, testCh, testReq, false)
	if resp.Data != nil || resp.Error != nil {
		t.Error("expected empty response")
	}

	// Test full channel and cancel
	go func() {
		resp := MakeRPC(ctx, testCh, testReq, true)
		if resp.Error == nil || !strings.Contains(resp.Error.Error(), "timed out") {
			t.Fatal("the operation should have been waiting and then cancelled")
		}
	}()
	// Previous request still taking the channel
	time.Sleep(2 * time.Second)
	cancel()
}
