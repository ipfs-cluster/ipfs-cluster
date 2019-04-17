package ipfshttp

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	logging "github.com/ipfs/go-log"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/test"
)

func init() {
	_ = logging.Logger
}

func testIPFSConnector(t *testing.T) (*Connector, *test.IpfsMock) {
	mock := test.NewIpfsMock(t)
	nodeMAddr, _ := ma.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d",
		mock.Addr, mock.Port))

	cfg := &Config{}
	cfg.Default()
	cfg.NodeAddr = nodeMAddr
	cfg.ConnectSwarmsDelay = 0

	ipfs, err := NewConnector(cfg)
	if err != nil {
		t.Fatal("creating an IPFSConnector should work: ", err)
	}

	ipfs.SetClient(test.NewMockRPCClient(t))
	return ipfs, mock
}

func TestNewConnector(t *testing.T) {
	ctx := context.Background()
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown(ctx)
}

func TestIPFSID(t *testing.T) {
	ctx := context.Background()
	ipfs, mock := testIPFSConnector(t)
	defer ipfs.Shutdown(ctx)
	id, err := ipfs.ID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if id.ID != test.PeerID1 {
		t.Error("expected testPeerID")
	}
	if len(id.Addresses) != 1 {
		t.Error("expected 1 address")
	}
	if id.Error != "" {
		t.Error("expected no error")
	}
	mock.Close()
	id, err = ipfs.ID(ctx)
	if err == nil {
		t.Error("expected an error")
	}
}

func testPin(t *testing.T, method string) {
	ctx := context.Background()
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown(ctx)

	ipfs.config.PinMethod = method

	c := test.Cid1
	err := ipfs.Pin(ctx, c, -1)
	if err != nil {
		t.Error("expected success pinning cid")
	}
	pinSt, err := ipfs.PinLsCid(ctx, c)
	if err != nil {
		t.Fatal("expected success doing ls")
	}
	if !pinSt.IsPinned(-1) {
		t.Error("cid should have been pinned")
	}

	c2 := test.ErrorCid
	err = ipfs.Pin(ctx, c2, -1)
	if err == nil {
		t.Error("expected error pinning cid")
	}
}

func TestIPFSPin(t *testing.T) {
	t.Run("method=pin", func(t *testing.T) { testPin(t, "pin") })
	t.Run("method=refs", func(t *testing.T) { testPin(t, "refs") })
}

func TestIPFSUnpin(t *testing.T) {
	ctx := context.Background()
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown(ctx)
	c := test.Cid1
	err := ipfs.Unpin(ctx, c)
	if err != nil {
		t.Error("expected success unpinning non-pinned cid")
	}
	ipfs.Pin(ctx, c, -1)
	err = ipfs.Unpin(ctx, c)
	if err != nil {
		t.Error("expected success unpinning pinned cid")
	}
}

func TestIPFSPinLsCid(t *testing.T) {
	ctx := context.Background()
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown(ctx)
	c := test.Cid1
	c2 := test.Cid2

	ipfs.Pin(ctx, c, -1)
	ips, err := ipfs.PinLsCid(ctx, c)
	if err != nil || !ips.IsPinned(-1) {
		t.Error("c should appear pinned")
	}

	ips, err = ipfs.PinLsCid(ctx, c2)
	if err != nil || ips != api.IPFSPinStatusUnpinned {
		t.Error("c2 should appear unpinned")
	}
}

func TestIPFSPinLs(t *testing.T) {
	ctx := context.Background()
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown(ctx)
	c := test.Cid1
	c2 := test.Cid2

	ipfs.Pin(ctx, c, -1)
	ipfs.Pin(ctx, c2, -1)
	ipsMap, err := ipfs.PinLs(ctx, "")
	if err != nil {
		t.Error("should not error")
	}

	if len(ipsMap) != 2 {
		t.Fatal("the map does not contain expected keys")
	}

	if !ipsMap[test.Cid1.String()].IsPinned(-1) || !ipsMap[test.Cid2.String()].IsPinned(-1) {
		t.Error("c1 and c2 should appear pinned")
	}
}

func TestIPFSShutdown(t *testing.T) {
	ctx := context.Background()
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	if err := ipfs.Shutdown(ctx); err != nil {
		t.Error("expected a clean shutdown")
	}
	if err := ipfs.Shutdown(ctx); err != nil {
		t.Error("expected a second clean shutdown")
	}
}

func TestConnectSwarms(t *testing.T) {
	// In order to interactively test uncomment the following.
	// Otherwise there is no good way to test this with the
	// ipfs mock
	// logging.SetDebugLogging()

	ctx := context.Background()
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown(ctx)
	time.Sleep(time.Second)
}

func TestSwarmPeers(t *testing.T) {
	ctx := context.Background()
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown(ctx)

	swarmPeers, err := ipfs.SwarmPeers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(swarmPeers) != 2 {
		t.Fatal("expected 2 swarm peers")
	}
	if swarmPeers[0] != test.PeerID4 {
		t.Error("unexpected swarm peer")
	}
	if swarmPeers[1] != test.PeerID5 {
		t.Error("unexpected swarm peer")
	}
}

func TestBlockPut(t *testing.T) {
	ctx := context.Background()
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown(ctx)

	data := []byte(test.Cid4Data)
	err := ipfs.BlockPut(ctx, &api.NodeWithMeta{
		Data:   data,
		Cid:    test.Cid4,
		Format: "raw",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestBlockGet(t *testing.T) {
	ctx := context.Background()
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown(ctx)

	shardCid := test.ShardCid
	// Fail when getting before putting
	_, err := ipfs.BlockGet(ctx, shardCid)
	if err == nil {
		t.Fatal("expected to fail getting unput block")
	}

	// Put and then successfully get
	err = ipfs.BlockPut(ctx, &api.NodeWithMeta{
		Data:   test.ShardData,
		Cid:    test.ShardCid,
		Format: "cbor",
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := ipfs.BlockGet(ctx, shardCid)
	if err != nil {
		t.Error(err)
	}
	if !bytes.Equal(data, test.ShardData) {
		t.Fatal("unexpected data returned")
	}
}

func TestRepoStat(t *testing.T) {
	ctx := context.Background()
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown(ctx)

	s, err := ipfs.RepoStat(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// See the ipfs mock implementation
	if s.RepoSize != 0 {
		t.Error("expected 0 bytes of size")
	}

	c := test.Cid1
	err = ipfs.Pin(ctx, c, -1)
	if err != nil {
		t.Error("expected success pinning cid")
	}

	s, err = ipfs.RepoStat(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if s.RepoSize != 1000 {
		t.Error("expected 1000 bytes of size")
	}
}

func TestResolve(t *testing.T) {
	ctx := context.Background()
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown(ctx)

	s, err := ipfs.Resolve(ctx, test.PathIPFS2)
	if err != nil {
		t.Error(err)
	}
	if !s.Equals(test.CidResolved) {
		t.Errorf("expected different cid, expected: %s, found: %s\n", test.CidResolved, s.String())
	}
}

func TestConfigKey(t *testing.T) {
	ctx := context.Background()
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown(ctx)

	v, err := ipfs.ConfigKey("Datastore/StorageMax")
	if err != nil {
		t.Fatal(err)
	}
	sto, ok := v.(string)
	if !ok {
		t.Fatal("error converting to string")
	}
	if sto != "10G" {
		t.Error("StorageMax shouold be 10G")
	}

	v, err = ipfs.ConfigKey("Datastore")
	_, ok = v.(map[string]interface{})
	if !ok {
		t.Error("should have returned the whole Datastore config object")
	}

	_, err = ipfs.ConfigKey("")
	if err == nil {
		t.Error("should not work with an empty path")
	}

	_, err = ipfs.ConfigKey("Datastore/abc")
	if err == nil {
		t.Error("should not work with a bad path")
	}
}
