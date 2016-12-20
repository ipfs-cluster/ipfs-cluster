package ipfscluster

import (
	"context"
	"strings"
	"testing"
	"time"

	peer "github.com/libp2p/go-libp2p-peer"
)

var (
	testCid       = "QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmmq"
	testCid2      = "QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmma"
	testCid3      = "QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmmb"
	errorCid      = "QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmmc"
	testPeerID, _ = peer.IDB58Decode("QmXZrtE5jQwXNqCJMfHUTQkvhQ4ZAnqMnmzFMJfLewuabc")
)

func TestMakeRPC(t *testing.T) {
	testCh := make(chan RPC, 1)
	testReq := NewRPC(MemberListRPC, nil)
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

	// Test full channel and cancel on send
	go func() {
		resp := MakeRPC(ctx, testCh, testReq, true)
		if resp.Error == nil || !strings.Contains(resp.Error.Error(), "timed out") {
			t.Fatal("the operation should have been waiting and then cancelled")
		}
	}()
	// Previous request still taking the channel
	time.Sleep(1 * time.Second)
	cancel()

	// Test cancelled while waiting for response context
	ctx, cancel = context.WithCancel(context.Background())
	testCh = make(chan RPC, 1)
	go func() {
		resp := MakeRPC(ctx, testCh, testReq, true)
		if resp.Error == nil || !strings.Contains(resp.Error.Error(), "timed out") {
			t.Fatal("the operation should have been waiting and then cancelled")
		}
	}()
	time.Sleep(1 * time.Second)
	cancel()
	time.Sleep(1 * time.Second)
}

func simulateAnswer(ch <-chan RPC, answer interface{}, err error) {
	go func() {
		req := <-ch
		req.ResponseCh() <- RPCResponse{
			Data:  answer,
			Error: CastRPCError(err),
		}
	}()
}
