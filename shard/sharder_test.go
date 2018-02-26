package shard

import (
	"bytes"
	"context"
	//	"strconv"
	"testing"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	blocks "github.com/ipfs/go-block-format"
	cid "github.com/ipfs/go-cid"
	dag "github.com/ipfs/go-ipfs/merkledag"
	cbor "github.com/ipfs/go-ipld-cbor"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/ipfs-cluster/api"
	crypto "github.com/libp2p/go-libp2p-crypto"
	host "github.com/libp2p/go-libp2p-host"
	peer "github.com/libp2p/go-libp2p-peer"
	peerstore "github.com/libp2p/go-libp2p-peerstore"
	swarm "github.com/libp2p/go-libp2p-swarm"
	basichost "github.com/libp2p/go-libp2p/p2p/host/basic"
	ma "github.com/multiformats/go-multiaddr"
	mh "github.com/multiformats/go-multihash"
)

func init() {
	ipld.Register(cid.DagProtobuf, dag.DecodeProtobufBlock)
	ipld.Register(cid.Raw, dag.DecodeRawBlock)
	ipld.Register(cid.DagCBOR, cbor.DecodeBlock) // need to decode CBOR
}

var nodeDataSet1 = [][]byte{[]byte(`Dag Node 1`), []byte(`Dag Node 2`), []byte(`Dag Node 3`)}
var nodeDataSet2 = [][]byte{[]byte(`Dag Node A`), []byte(`Dag Node B`), []byte(`Dag Node C`)}

// mockRPC simulates the sharder's connection with the rest of the cluster.
// It keeps track of an ordered list of ipfs block puts for use by tests
// that verify a sequence of dag-nodes is correctly added with the right
// metadata.
type mockRPC struct {
	orderedPuts map[int]api.BlockWithFormat
	Host        host.Host
}

// NewMockRPCClient creates a mock ipfs-cluster RPC server and returns a client
// to it.  A testing host is created so that the server can be called by the
// pid returned in Allocate
func NewMockRPCClient(t *testing.T, mock *mockRPC) *rpc.Client {
	h := makeTestingHost()
	return NewMockRPCClientWithHost(t, h, mock)
}

// NewMockRPCClientWithHost returns a mock ipfs-cluster RPC client initialized
// with a given host.
func NewMockRPCClientWithHost(t *testing.T, h host.Host, mock *mockRPC) *rpc.Client {
	s := rpc.NewServer(h, "sharder-mock")
	c := rpc.NewClientWithServer(h, "sharder-mock", s)
	mock.Host = h
	err := s.RegisterName("Cluster", mock)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func makeTestingHost() host.Host {
	priv, pub, _ := crypto.GenerateKeyPair(crypto.RSA, 2048)
	pid, _ := peer.IDFromPublicKey(pub)
	maddr, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	ps := peerstore.NewPeerstore()
	ps.AddPubKey(pid, pub)
	ps.AddPrivKey(pid, priv)
	ps.AddAddr(pid, maddr, peerstore.PermanentAddrTTL)
	mock_network, _ := swarm.NewNetwork(context.Background(),
		[]ma.Multiaddr{maddr},
		pid,
		ps,
		nil,
	)

	return basichost.New(mock_network)
}

// GetInformerMetrics does nothing as mock allocator does not check metrics
func (mock *mockRPC) GetInformerMetrics(in struct{}, out *[]api.Metric) error {
	return nil
}

// All pins get allocated to the mockRPC's server host
func (mock *mockRPC) Allocate(in api.AllocateInfo, out *[]peer.ID) error {
	*out = []peer.ID{mock.Host.ID()}
	return nil
}

// Record the ordered sequence of BlockPut calls for later validation
func (mock *mockRPC) IPFSBlockPut(in api.BlockWithFormat, out *string) error {
	mock.orderedPuts[len(mock.orderedPuts)] = in
	return nil
}

// Tests don't currently check Pin calls.  For now this is a NOP.
// TODO: once the sharder Pinning is stabalized (support for pinning to
// specific peers and non-recursive pinning through RPC) we should validate
// pinning calls alongside block put calls
func (mock *mockRPC) Pin(in api.PinSerial, out *struct{}) error {
	return nil
}

// Check that allocations go to the correct server.  This will require setting
// up more than one server and doing some libp2p trickery.  TODO after single
// host tests are complete

// Create a new sharder and register a mock RPC for testing
func testNewSharder(t *testing.T) (*Sharder, *mockRPC) {
	mockRPC := &mockRPC{}
	mockRPC.orderedPuts = make(map[int]api.BlockWithFormat)
	client := NewMockRPCClient(t, mockRPC)
	cfg := &Config{
		AllocSize: DefaultAllocSize,
	}
	sharder, err := NewSharder(cfg)
	if err != nil {
		t.Fatal(err)
	}
	sharder.SetClient(client)
	return sharder, mockRPC
}

// Simply test that 3 input nodes are added and that the shard node
// and clusterDAG root take the correct form
func TestAddAndFinalizeShard(t *testing.T) {
	sharder, mockRPC := testNewSharder(t)
	// Create 3 ipld protobuf nodes and add to sharding session
	nodes := make([]*dag.ProtoNode, 3)
	cids := make([]string, 3)
	sessionID := "testAddShard"
	for i, data := range nodeDataSet1 {
		nodes[i] = dag.NodeWithData(data)
		nodes[i].SetPrefix(nil)
		cids[i] = nodes[i].Cid().String()
		logger.Debugf("Cid of node%d: %s", i, cids[i])
		size, err := nodes[i].Size()
		if err != nil {
			t.Fatal(err)
		}
		err = sharder.AddNode(size, nodes[i].RawData(), nodes[i].Cid().String(), sessionID)
		if err != nil {
			t.Fatal(err)
		}
	}
	err := sharder.Finalize(sessionID)
	if err != nil {
		t.Fatal(err)
	}

	if len(mockRPC.orderedPuts) != len(nodes)+2 { //data nodes, 1 shard node 1 shard root
		t.Errorf("unexpected number of block puts called: %d", len(mockRPC.orderedPuts))
	}
	// Verify correct node data sent to ipfs
	verifyNodePuts(t, nodeDataSet1, cids, mockRPC.orderedPuts, []int{0, 1, 2})

	shardNode := cborDataToNode(t, mockRPC.orderedPuts[len(nodes)])
	// Traverse shard node to verify all expected links are there
	for i, link := range shardNode.Links() {
		// TODO remove dependence on link order, and make use of the
		// link number info that exists somewhere within the cbor object
		// but apparently not the ipld links (is this a bug in ipld cbor?)
		/*i, err := strconv.Atoi(link.Name)
		if err != nil || i >= 3 {
			t.Errorf("Unexpected link name :%s:", link.Name)
			continue
		}*/
		if link.Cid.String() != cids[i] {
			t.Errorf("Link %d should point to %s.  Instead points to %s", i, cids[i], link.Cid.String())
		}
	}

	rootNode := cborDataToNode(t, mockRPC.orderedPuts[len(nodes)+1])
	// Verify that clusterDAG root points to shard node
	links := rootNode.Links()
	if len(links) != 1 {
		t.Fatalf("Expected 1 link in root got %d", len(links))
	}
	if links[0].Cid.String() != shardNode.Cid().String() {
		t.Errorf("clusterDAG expected to link to %s, instead links to %s",
			shardNode.Cid().String(), links[0].Cid.String())
	}
}

// verifyNodePuts takes in a slice of byte slices containing the underlying data
// of added nodes, an ordered slice of the cids of these nodes, a map between
// IPFSBlockPut call order and arguments, and a slice determining which
// IPFSBlockPut calls to verify.
func verifyNodePuts(t *testing.T,
	dataSet [][]byte,
	cids []string,
	orderedPuts map[int]api.BlockWithFormat,
	toVerify []int) {
	if len(cids) != len(toVerify) || len(dataSet) != len(toVerify) {
		t.Error("Malformed verifyNodePuts arguments")
		return
	}
	for j, i := range toVerify {
		if orderedPuts[i].Format != "v0" {
			t.Errorf("Expecting blocks in v0 format, instead: %s", orderedPuts[i].Format)
			continue
		}
		data := orderedPuts[i].Data
		c, err := cid.Decode(cids[j])
		if err != nil {
			t.Error(err)
			continue
		}
		blk, err := blocks.NewBlockWithCid(data, c)
		if err != nil {
			t.Error(err)
			continue
		}
		dataNode, err := ipld.Decode(blk)
		if err != nil {
			t.Error(err)
			continue
		}
		if bytes.Equal(dataNode.RawData(), dataSet[j]) {
			t.Error(err)
		}
	}
}

func cborDataToNode(t *testing.T, putInfo api.BlockWithFormat) ipld.Node {
	if putInfo.Format != "cbor" {
		t.Fatalf("Unexpected shard node format %s", putInfo.Format)
	}
	shardCid, err := cid.NewPrefixV1(cid.DagCBOR, mh.SHA2_256).Sum(putInfo.Data)
	if err != nil {
		t.Fatal(err)
	}
	shardBlk, err := blocks.NewBlockWithCid(putInfo.Data, shardCid)
	if err != nil {
		t.Fatal(err)
	}
	shardNode, err := ipld.Decode(shardBlk)
	if err != nil {
		t.Fatal(err)
	}
	return shardNode
}

// Interleave two shard sessions to test isolation between concurrent file
// shards.
func TestInterleaveSessions(t *testing.T) {
	// Make sharder and add data
	sharder, mockRPC := testNewSharder(t)
	nodes1 := make([]*dag.ProtoNode, 3)
	cids1 := make([]string, 3)
	nodes2 := make([]*dag.ProtoNode, 3)
	cids2 := make([]string, 3)

	sessionID1 := "interleave1"
	sessionID2 := "interleave2"
	for i := 0; i < 6; i++ {
		var nodes []*dag.ProtoNode
		var cids []string
		var dataSet [][]byte
		var sessionID string
		if i%2 == 0 { // Add to session 1
			nodes = nodes1
			cids = cids1
			dataSet = nodeDataSet1
			sessionID = sessionID1
		} else {
			nodes = nodes2
			cids = cids2
			dataSet = nodeDataSet2
			sessionID = sessionID2
		}
		j := i / 2
		nodes[j] = dag.NodeWithData(dataSet[j])
		nodes[j].SetPrefix(nil)
		cids[j] = nodes[j].Cid().String()
		size, err := nodes[j].Size()
		if err != nil {
			t.Fatal(err)
		}
		err = sharder.AddNode(size, nodes[j].RawData(), cids[j],
			sessionID)
		if err != nil {
			t.Fatal(err)
		}
	}
	err := sharder.Finalize(sessionID1)
	if err != nil {
		t.Fatal(err)
	}
	err = sharder.Finalize(sessionID2)
	if err != nil {
		t.Fatal(err)
	}

	if len(mockRPC.orderedPuts) != len(nodes1)+len(nodes2)+4 {
		t.Errorf("Unexpected number of block puts called: %d", len(mockRPC.orderedPuts))
	}
	verifyNodePuts(t, nodeDataSet1, cids1, mockRPC.orderedPuts, []int{0, 2, 4})
	verifyNodePuts(t, nodeDataSet2, cids2, mockRPC.orderedPuts, []int{1, 3, 5})

	// verify clusterDAG for session 1
	shardNode1 := cborDataToNode(t, mockRPC.orderedPuts[6])
	for i, link := range shardNode1.Links() {
		if link.Cid.String() != cids1[i] {
			t.Errorf("Link %d should point to %s.  Instead points to %s", i, cids1[i], link.Cid.String())
		}
	}
	rootNode1 := cborDataToNode(t, mockRPC.orderedPuts[7])
	links := rootNode1.Links()
	if len(links) != 1 {
		t.Errorf("Expected 1 link in root got %d", len(links))
	}
	if links[0].Cid.String() != shardNode1.Cid().String() {
		t.Errorf("clusterDAG expected to link to %s, instead links to %s",
			shardNode1.Cid().String(), links[0].Cid.String())
	}

	// verify clusterDAG for session 2
	shardNode2 := cborDataToNode(t, mockRPC.orderedPuts[8])
	for i, link := range shardNode2.Links() {
		if link.Cid.String() != cids2[i] {
			t.Errorf("Link %d should point to %s.  Instead points to %s", i, cids2[i], link.Cid.String())
		}
	}
	rootNode2 := cborDataToNode(t, mockRPC.orderedPuts[9])
	links = rootNode2.Links()
	if len(links) != 1 {
		t.Errorf("Expected 1 link in root got %d", len(links))
	}
	if links[0].Cid.String() != shardNode2.Cid().String() {
		t.Errorf("clusterDAG expected to link to %s, instead links to %s",
			shardNode2.Cid().String(), links[0].Cid.String())
	}
}

// Test that by adding in enough nodes multiple shard nodes will be created

// Test many tiny dag nodes so that a shard node is too big to fit all links
// and itself must be broken down into a tree of shard nodes.
