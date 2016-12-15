package ipfscluster

import (
	"errors"
	"testing"
	"time"

	cid "gx/ipfs/QmcTcsTvfaeEBRFo1TkFgT8sRmgi1n1LTZpecfVP8fzpGD/go-cid"
)

type mockComponent struct {
	rpcCh       chan RPC
	returnError bool
}

func (c *mockComponent) Shutdown() error {
	return nil
}

func (c *mockComponent) RpcChan() <-chan RPC {
	return c.rpcCh
}

type mockApi struct {
	mockComponent
}

type mockConnector struct {
	mockComponent
}

func (ipfs *mockConnector) Pin(c *cid.Cid) error {
	if ipfs.returnError {
		return errors.New("")
	}
	return nil
}

func (ipfs *mockConnector) Unpin(c *cid.Cid) error {
	if ipfs.returnError {
		return errors.New("")
	}
	return nil
}

func (ipfs *mockConnector) IsPinned(c *cid.Cid) (bool, error) {
	if ipfs.returnError {
		return false, errors.New("")
	}
	return true, nil
}

func testingCluster(t *testing.T) (*Cluster, *mockApi, *mockConnector, *MapState, *MapPinTracker) {
	api := &mockApi{}
	api.rpcCh = make(chan RPC, 2)
	ipfs := &mockConnector{}
	ipfs.rpcCh = make(chan RPC, 2)
	cfg := testingConfig()
	st := NewMapState()
	tracker := NewMapPinTracker()

	cl, err := NewCluster(
		cfg,
		api,
		ipfs,
		st,
		tracker,
	)
	if err != nil {
		t.Fatal("cannot create cluster:", err)
	}
	time.Sleep(3 * time.Second) // make sure a leader is elected
	return cl, api, ipfs, st, tracker
}

func testClusterShutdown(t *testing.T) {
	cl, _, _, _, _ := testingCluster(t)
	err := cl.Shutdown()
	if err != nil {
		t.Error("cluster shutdown failed:", err)
	}
	cl.Shutdown()
	cl, _, _, _, _ = testingCluster(t)
	err = cl.Shutdown()
	if err != nil {
		t.Error("cluster shutdown failed:", err)
	}
}

func TestClusterLocalSync(t *testing.T) {
	cl, _, _, st, _ := testingCluster(t)
	defer cleanRaft()
	defer cl.Shutdown()
	_, err := cl.LocalSync()
	if err == nil {
		t.Error("expected an error as there is no state to sync")
	}

	c, _ := cid.Decode(testCid)
	err = cl.Pin(c)
	if err != nil {
		t.Fatal("pin should have worked:", err)
	}

	_, err = cl.LocalSync()
	if err != nil {
		t.Fatal("sync after pinning should have worked:", err)
	}

	// Modify state on the side so the sync does not
	// happen on an empty slide
	st.RmPin(c)
	_, err = cl.LocalSync()
	if err != nil {
		t.Fatal("sync with recover should have worked:", err)
	}
}

func TestClusterPin(t *testing.T) {
	cl, _, _, _, _ := testingCluster(t)
	defer cleanRaft()
	defer cl.Shutdown()

	c, _ := cid.Decode(testCid)
	err := cl.Pin(c)
	if err != nil {
		t.Fatal("pin should have worked:", err)
	}

	// test an error case
	cl.consensus.Shutdown()
	err = cl.Pin(c)
	if err == nil {
		t.Error("expected an error but things worked")
	}
}

func TestClusterUnpin(t *testing.T) {
	cl, _, _, _, _ := testingCluster(t)
	defer cleanRaft()
	defer cl.Shutdown()

	c, _ := cid.Decode(testCid)
	err := cl.Unpin(c)
	if err != nil {
		t.Fatal("pin should have worked:", err)
	}

	// test an error case
	cl.consensus.Shutdown()
	err = cl.Unpin(c)
	if err == nil {
		t.Error("expected an error but things worked")
	}
}

func TestClusterMembers(t *testing.T) {
	cl, _, _, _, _ := testingCluster(t)
	defer cleanRaft()
	defer cl.Shutdown()
	m := cl.Members()
	id := testingConfig().ID
	if len(m) != 1 || m[0].Pretty() != id {
		t.Error("bad Members()")
	}
}

func TestVersion(t *testing.T) {
	cl, _, _, _, _ := testingCluster(t)
	defer cleanRaft()
	defer cl.Shutdown()
	if cl.Version() != Version {
		t.Error("bad Version()")
	}
}

func TestClusterRun(t *testing.T) {
	cl, api, ipfs, _, tracker := testingCluster(t)
	defer cleanRaft()
	defer cl.Shutdown()
	// We sent RPCs all all types with one of the
	// RpcChannels and make sure there is always a response
	// We don't care about the value of that response now. We leave
	// that for end-to-end tests

	// Generic RPC
	for i := 0; i < NoopRPC; i++ {
		rpc := NewRPC(RPCOp(i), "something")
		switch i % 4 {
		case 0:
			ipfs.rpcCh <- rpc
		case 1:
			cl.consensus.rpcCh <- rpc
		case 2:
			api.rpcCh <- rpc
		case 3:
			tracker.rpcCh <- rpc
		}
		// Wait for a response
		timer := time.NewTimer(time.Second)
		select {
		case <-rpc.ResponseCh():
		case <-timer.C:
			t.Errorf("GenericRPC %d was not handled correctly by Cluster", i)
		}
	}

	// Cid RPC
	c, _ := cid.Decode(testCid)
	for i := 0; i < NoopRPC; i++ {
		rpc := NewRPC(RPCOp(i), c)
		switch i % 4 {
		case 0:
			ipfs.rpcCh <- rpc
		case 1:
			cl.consensus.rpcCh <- rpc
		case 2:
			api.rpcCh <- rpc
		case 3:
			tracker.rpcCh <- rpc
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-rpc.ResponseCh():
		case <-timer.C:
			t.Errorf("CidRPC %d was not handled correctly by Cluster", i)
		}
	}
}
