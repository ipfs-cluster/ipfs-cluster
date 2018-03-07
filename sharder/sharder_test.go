package sharder

import (
	"bytes"
	"context"
	"encoding/binary"
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
	orderedPuts map[int]api.NodeWithMeta
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
	mockNetwork, _ := swarm.NewNetwork(context.Background(),
		[]ma.Multiaddr{maddr},
		pid,
		ps,
		nil,
	)

	return basichost.New(mockNetwork)
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
func (mock *mockRPC) IPFSBlockPut(in api.NodeWithMeta, out *string) error {
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
	mockRPC.orderedPuts = make(map[int]api.NodeWithMeta)
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
	sessionID := ""
	for i, data := range nodeDataSet1 {
		nodes[i] = dag.NodeWithData(data)
		nodes[i].SetPrefix(nil)
		cids[i] = nodes[i].Cid().String()
		logger.Debugf("Cid of node%d: %s", i, cids[i])
		size, err := nodes[i].Size()
		if err != nil {
			t.Fatal(err)
		}
		sessionID, err = sharder.AddNode(size, nodes[i].RawData(), cids[i], sessionID)
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
	links := shardNode.Links()
	for _, c := range cids {
		if !linksContain(links, c) {
			t.Errorf("expected cid %s not in shard node", c)
		}
	}

	rootNode := cborDataToNode(t, mockRPC.orderedPuts[len(nodes)+1])
	// Verify that clusterDAG root points to shard node
	links = rootNode.Links()
	if len(links) != 1 {
		t.Fatalf("Expected 1 link in root got %d", len(links))
	}
	if links[0].Cid.String() != shardNode.Cid().String() {
		t.Errorf("clusterDAG expected to link to %s, instead links to %s",
			shardNode.Cid().String(), links[0].Cid.String())
	}
}

// helper function determining whether a cid is referenced in a slice of links
func linksContain(links []*ipld.Link, c string) bool {
	for _, link := range links {
		if link.Cid.String() == c {
			return true
		}
	}
	return false
}

// verifyNodePuts takes in a slice of byte slices containing the underlying data
// of added nodes, an ordered slice of the cids of these nodes, a map between
// IPFSBlockPut call order and arguments, and a slice determining which
// IPFSBlockPut calls to verify.
func verifyNodePuts(t *testing.T,
	dataSet [][]byte,
	cids []string,
	orderedPuts map[int]api.NodeWithMeta,
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

func cborDataToNode(t *testing.T, putInfo api.NodeWithMeta) ipld.Node {
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

	sessionID1 := ""
	sessionID2 := ""
	for i := 0; i < 6; i++ {
		j := i / 2
		if i%2 == 0 { // session 1
			nodes1[j] = dag.NodeWithData(nodeDataSet1[j])
			nodes1[j].SetPrefix(nil)
			cids1[j] = nodes1[j].Cid().String()
			size, err := nodes1[j].Size()
			if err != nil {
				t.Fatal(err)
			}
			sessionID1, err = sharder.AddNode(size, nodes1[j].RawData(), cids1[j], sessionID1)
			if err != nil {
				t.Fatal(err)
			}
		} else { // session 2
			nodes2[j] = dag.NodeWithData(nodeDataSet2[j])
			nodes2[j].SetPrefix(nil)
			cids2[j] = nodes2[j].Cid().String()
			size, err := nodes2[j].Size()
			if err != nil {
				t.Fatal(err)
			}
			sessionID2, err = sharder.AddNode(size, nodes2[j].RawData(), cids2[j], sessionID2)
			if err != nil {
				t.Fatal(err)
			}
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
	links1 := shardNode1.Links()
	for _, c := range cids1 {
		if !linksContain(links1, c) {
			t.Errorf("expected cid %s not in links of shard node of session 1", c)
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
	// Traverse shard node to verify all expected links are there
	links2 := shardNode2.Links()
	for _, c := range cids2 {
		if !linksContain(links2, c) {
			t.Errorf("expected cid %s not in links of shard node of session 2", c)
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

func getManyLinksDataSet(t *testing.T) [][]byte {
	numberData := 2*MaxLinks + MaxLinks/2
	dataSet := make([][]byte, numberData)
	for i := range dataSet {
		buf := new(bytes.Buffer)
		err := binary.Write(buf, binary.LittleEndian, uint16(i))
		if err != nil {
			t.Fatal(err)
		}
		dataSet[i] = buf.Bytes()
	}
	return dataSet
}

// Test many tiny dag nodes so that a shard node is too big to fit all links
// and itself must be broken down into a tree of shard nodes.
func TestManyLinks(t *testing.T) {
	sharder, mockRPC := testNewSharder(t)
	dataSet := getManyLinksDataSet(t)
	nodes := make([]*dag.ProtoNode, len(dataSet))
	cids := make([]string, len(dataSet))
	sessionID := ""

	for i, data := range dataSet {
		nodes[i] = dag.NodeWithData(data)
		nodes[i].SetPrefix(nil)
		cids[i] = nodes[i].Cid().String()
		size, err := nodes[i].Size()
		if err != nil {
			t.Fatal(err)
		}
		sessionID, err = sharder.AddNode(size, nodes[i].RawData(), cids[i], sessionID)
		if err != nil {
			t.Fatal(err)
		}

	}
	err := sharder.Finalize(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	index := make([]int, len(nodes))
	for i := range index {
		index[i] = i
	}
	// data nodes, 3 shard dag leaves, 1 shard dag root, 1 root dag node
	if len(mockRPC.orderedPuts) != len(nodes)+5 {
		t.Errorf("unexpected number of block puts called: %d", len(mockRPC.orderedPuts))
	}
	verifyNodePuts(t, dataSet, cids, mockRPC.orderedPuts, index)

	// all shardNode leaves but the last should be filled to capacity
	shardNodeLeaf1 := cborDataToNode(t, mockRPC.orderedPuts[len(nodes)+1])
	leafLinks1 := shardNodeLeaf1.Links()
	shardNodeLeaf2 := cborDataToNode(t, mockRPC.orderedPuts[len(nodes)+2])
	leafLinks2 := shardNodeLeaf2.Links()
	shardNodeLeaf3 := cborDataToNode(t, mockRPC.orderedPuts[len(nodes)+3])
	leafLinks3 := shardNodeLeaf3.Links()

	if len(leafLinks1) != MaxLinks {
		t.Errorf("First leaf should have max links, not %d",
			len(leafLinks1))
	}
	if len(leafLinks2) != MaxLinks {
		t.Errorf("Second leaf should have max links, not %d",
			len(leafLinks2))
	}
	if len(leafLinks3) != MaxLinks/2 {
		t.Errorf("Third leaf should have half max links, not %d",
			len(leafLinks3))
	}

	// shardNodeRoot must point to shardNode leaves
	shardNodeIndirect := cborDataToNode(t, mockRPC.orderedPuts[len(nodes)])
	links := shardNodeIndirect.Links()
	if len(links) != 3 {
		t.Fatalf("Expected 3 links in indirect got %d", len(links))
	}
	if !linksContain(links, shardNodeLeaf1.Cid().String()) ||
		!linksContain(links, shardNodeLeaf2.Cid().String()) ||
		!linksContain(links, shardNodeLeaf3.Cid().String()) {
		t.Errorf("Unexpected shard leaf nodes in shard root node")
	}

	// clusterDAG root should only point to shardNode root
	clusterRoot := cborDataToNode(t, mockRPC.orderedPuts[len(nodes)+4])
	links = clusterRoot.Links()
	if len(links) != 1 {
		t.Fatalf("Expected 1 link in root got %d", len(links))
	}
	if links[0].Cid.String() != shardNodeIndirect.Cid().String() {
		t.Errorf("clusterDAG expected to link to %s, instead links to %s",
			shardNodeIndirect.Cid().String(),
			links[0].Cid.String())
	}
}

// Test that by adding in enough nodes multiple shard nodes will be created
func TestMultipleShards(t *testing.T) {
	sharder, mockRPC := testNewSharder(t)
	sharder.allocSize = IPFSChunkSize + IPFSChunkSize/2
	sharder.allocSize = uint64(300000)
	nodes := make([]*dag.ProtoNode, 4)
	cids := make([]string, 4)
	sessionID := ""

	for i := range nodes {
		data := repeatData(90000, i)
		nodes[i] = dag.NodeWithData(data)
		nodes[i].SetPrefix(nil)
		cids[i] = nodes[i].Cid().String()
		size, err := nodes[i].Size()
		if err != nil {
			t.Fatal(err)
		}
		sessionID, err = sharder.AddNode(size, nodes[i].RawData(), cids[i], sessionID)
		if err != nil {
			t.Fatal(err)
		}
	}
	err := sharder.Finalize(sessionID)
	if err != nil {
		t.Fatal(err)
	}

	if len(mockRPC.orderedPuts) != len(nodes)+3 { //data nodes, 2 shard nodes, 1 root
		t.Errorf("unexpected number of block puts called: %d", len(mockRPC.orderedPuts))
	}

	// First shard node contains first 3 cids
	shardNode1 := cborDataToNode(t, mockRPC.orderedPuts[len(nodes)-1])
	links := shardNode1.Links()
	for _, c := range cids[:len(cids)-1] {
		if !linksContain(links, c) {
			t.Errorf("expected cid %s not in shard node", c)
		}
	}

	// Second shard node only points to final cid
	shardNode2 := cborDataToNode(t, mockRPC.orderedPuts[len(nodes)+1])
	links = shardNode2.Links()
	if len(links) != 1 || links[0].Cid.String() != cids[len(nodes)-1] {
		t.Errorf("unexpected links in second shard node")
	}
}

// repeatData takes in a byte value and a number of times this value should be
// repeated and returns a byte slice of this value repeated
func repeatData(byteCount int, digit int) []byte {
	data := make([]byte, byteCount)
	for i := range data {
		data[i] = byte(digit)
	}
	return data
}
