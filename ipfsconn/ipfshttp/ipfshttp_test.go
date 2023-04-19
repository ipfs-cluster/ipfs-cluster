package ipfshttp

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	logging "github.com/ipfs/go-log/v2"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/ipfs-cluster/ipfs-cluster/test"
	merkledag "github.com/ipfs/boxo/ipld/merkledag"
)

func init() {
	_ = logging.Logger
	logging.SetLogLevel("*", "DEBUG")
}

func testIPFSConnector(t *testing.T) (*Connector, *test.IpfsMock) {
	mock := test.NewIpfsMock(t)
	nodeMAddr := ma.StringCast(fmt.Sprintf("/ip4/%s/tcp/%d", mock.Addr, mock.Port))

	cfg := &Config{}
	cfg.Default()
	cfg.NodeAddr = nodeMAddr
	cfg.ConnectSwarmsDelay = 0
	cfg.InformerTriggerInterval = 10

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
	if len(id.Addresses) != 2 {
		t.Error("expected 2 address")
	}
	if id.Error != "" {
		t.Error("expected no error")
	}
	mock.Close()
	_, err = ipfs.ID(ctx)
	if err == nil {
		t.Error("expected an error")
	}
}

func TestPin(t *testing.T) {
	ctx := context.Background()
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown(ctx)

	pin := api.PinCid(test.Cid1)
	pin.Origins = []api.Multiaddr{
		api.NewMultiaddrWithValue(ma.StringCast("/ip4/1.2.3.4/tcp/1234/p2p/12D3KooWKewdAMAU3WjYHm8qkAJc5eW6KHbHWNigWraXXtE1UCng")),
		api.NewMultiaddrWithValue(ma.StringCast("/ip4/2.3.3.4/tcp/1234/p2p/12D3KooWF6BgwX966ge5AVFs9Gd2wVTBmypxZVvaBR12eYnUmXkR")),
	}
	err := ipfs.Pin(ctx, pin)
	if err != nil {
		t.Error("expected success pinning cid:", err)
	}
	pinSt, err := ipfs.PinLsCid(ctx, pin)
	if err != nil {
		t.Fatal("expected success doing ls:", err)
	}
	if !pinSt.IsPinned(-1) {
		t.Error("cid should have been pinned")
	}

	pin2 := api.PinCid(test.ErrorCid)
	err = ipfs.Pin(ctx, pin2)
	if err == nil {
		t.Error("expected error pinning cid")
	}

	ipfs.config.PinTimeout = 5 * time.Second
	c4 := test.SlowCid1
	err = ipfs.Pin(ctx, api.PinCid(c4))
	if err == nil {
		t.Error("expected error pinning cid")
	}
}

func TestPinUpdate(t *testing.T) {
	ctx := context.Background()
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown(ctx)

	pin := api.PinCid(test.Cid1)
	pin.PinUpdate = test.Cid1
	err := ipfs.Pin(ctx, pin)
	if err != nil {
		t.Error("pin update should have worked even if not pinned")
	}

	err = ipfs.Pin(ctx, pin)
	if err != nil {
		t.Fatal(err)
	}

	// This should trigger the pin/update path
	pin.Cid = test.Cid2
	err = ipfs.Pin(ctx, pin)
	if err != nil {
		t.Fatal(err)
	}

	if mock.GetCount("pin/update") != 1 {
		t.Error("pin/update should have been called once")
	}

	if mock.GetCount("pin/add") != 1 {
		t.Error("pin/add should have been called once")
	}
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
	ipfs.Pin(ctx, api.PinCid(c))
	err = ipfs.Unpin(ctx, c)
	if err != nil {
		t.Error("expected success unpinning pinned cid")
	}
}

func TestIPFSUnpinDisabled(t *testing.T) {
	ctx := context.Background()
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown(ctx)
	ipfs.config.UnpinDisable = true
	err := ipfs.Pin(ctx, api.PinCid(test.Cid1))
	if err != nil {
		t.Fatal(err)
	}

	err = ipfs.Unpin(ctx, test.Cid1)
	if err == nil {
		t.Fatal("pin should be disabled")
	}
}

func TestIPFSPinLsCid(t *testing.T) {
	ctx := context.Background()
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown(ctx)
	c := test.Cid1
	c2 := test.Cid2

	pin := api.PinCid(c)
	ipfs.Pin(ctx, pin)
	ips, err := ipfs.PinLsCid(ctx, pin)
	if err != nil {
		t.Error(err)
	}

	if !ips.IsPinned(-1) {
		t.Error("c should appear pinned")
	}

	ips, err = ipfs.PinLsCid(ctx, api.PinCid(c2))
	if err != nil || ips != api.IPFSPinStatusUnpinned {
		t.Error("c2 should appear unpinned")
	}
}

func TestIPFSPinLsCid_DifferentEncoding(t *testing.T) {
	ctx := context.Background()
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown(ctx)
	c := test.Cid4 // ipfs mock treats this specially

	pin := api.PinCid(c)
	ipfs.Pin(ctx, pin)
	ips, err := ipfs.PinLsCid(ctx, pin)
	if err != nil {
		t.Error(err)
	}

	if !ips.IsPinned(-1) {
		t.Error("c should appear pinned")
	}
}

func collectPins(t *testing.T, pch <-chan api.IPFSPinInfo) []api.IPFSPinInfo {
	t.Helper()

	var pins []api.IPFSPinInfo
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
			return nil
		case p, ok := <-pch:
			if !ok {
				return pins
			}
			pins = append(pins, p)
		}
	}
}

func TestIPFSPinLs(t *testing.T) {
	ctx := context.Background()
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown(ctx)
	c := test.Cid1
	c2 := test.Cid2

	ipfs.Pin(ctx, api.PinCid(c))
	ipfs.Pin(ctx, api.PinCid(c2))
	pinCh := make(chan api.IPFSPinInfo, 10)
	go func() {
		err := ipfs.PinLs(ctx, []string{""}, pinCh)
		if err != nil {
			t.Error("should not error")
		}
	}()

	pins := collectPins(t, pinCh)

	if len(pins) != 2 {
		t.Fatal("the pin list does not contain the expected number of keys")
	}

	if !pins[0].Type.IsPinned(-1) || !pins[1].Type.IsPinned(-1) {
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

func TestBlockStream(t *testing.T) {
	ctx := context.Background()
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown(ctx)

	blocks := make(chan api.NodeWithMeta, 10)
	blocks <- api.NodeWithMeta{
		Data: []byte(test.Cid4Data),
		Cid:  test.Cid4,
	}

	// Because this has a different prefix,
	// it will produce a warning.
	blocks <- api.NodeWithMeta{
		Data: []byte(test.Cid5Data),
		Cid:  test.Cid5,
	}
	close(blocks)

	err := ipfs.BlockStream(ctx, blocks)
	if err != nil {
		t.Error(err)
	}

	// Try only adding v0 cid now
	blocks2 := make(chan api.NodeWithMeta, 1)
	blocks2 <- api.NodeWithMeta{
		Data: []byte(test.Cid5Data),
		Cid:  test.Cid5,
	}
	close(blocks2)

	err = ipfs.BlockStream(ctx, blocks2)
	if err != nil {
		t.Error(err)
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

	blocks := make(chan api.NodeWithMeta, 1)
	blocks <- api.NodeWithMeta{
		Data: test.ShardData,
		Cid:  test.ShardCid,
	}
	close(blocks)
	err = ipfs.BlockStream(ctx, blocks)
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
	err = ipfs.Pin(ctx, api.PinCid(c))
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
	if err != nil {
		t.Fatal(err)
	}
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

func TestRepoGC(t *testing.T) {
	ctx := context.Background()
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown(ctx)

	res, err := ipfs.RepoGC(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if res.Error != "" {
		t.Errorf("expected error to be empty: %s", res.Error)
	}

	if res.Keys == nil {
		t.Fatal("expected a non-nil array of IPFSRepoGC")
	}

	if len(res.Keys) < 5 {
		t.Fatal("expected at least five keys")
	}

	if !res.Keys[0].Key.Equals(test.Cid1) {
		t.Errorf("expected different cid, expected: %s, found: %s\n", test.Cid1, res.Keys[0].Key)
	}

	if !res.Keys[3].Key.Equals(test.Cid4) {
		t.Errorf("expected different cid, expected: %s, found: %s\n", test.Cid4, res.Keys[3].Key)
	}

	if res.Keys[4].Error != merkledag.ErrLinkNotFound.Error() {
		t.Errorf("expected different error, expected: %s, found: %s\n", merkledag.ErrLinkNotFound, res.Keys[4].Error)
	}
}
