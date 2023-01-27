package sharding

// dag.go defines functions for constructing and parsing ipld-cbor nodes
// of the clusterDAG used to track sharded DAGs in ipfs-cluster

// Most logic goes into handling the edge cases in which clusterDAG
// metadata for a single shard cannot fit within a single shard node.  We
// make the following simplifying assumption: a single shard will not track
// more than 35,808,256 links (~2^25).  This is the limit at which the current
// shard node format would need 2 levels of indirect nodes to reference
// all of the links.  Note that this limit is only reached at shard sizes 7
// times the size of the current default and then only when files are all
// 1 byte in size.  In the future we may generalize the shard dag to multiple
// indirect nodes to accommodate much bigger shard sizes.  Also note that the
// move to using the identity hash function in cids of very small data
// will improve link density in shard nodes and further reduce the need for
// multiple levels of indirection.

import (
	"context"
	"fmt"

	cid "github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	ipld "github.com/ipfs/go-ipld-format"
	blocks "github.com/ipfs/go-libipfs/blocks"
	dag "github.com/ipfs/go-merkledag"
	mh "github.com/multiformats/go-multihash"
)

// go-merkledag does this, but it may be moved.
// We include for explicitness.
func init() {
	ipld.Register(cid.DagProtobuf, dag.DecodeProtobufBlock)
	ipld.Register(cid.Raw, dag.DecodeRawBlock)
	ipld.Register(cid.DagCBOR, cbor.DecodeBlock)
}

// MaxLinks is the max number of links that, when serialized fit into a block
const MaxLinks = 5984
const hashFn = mh.SHA2_256

// CborDataToNode parses cbor data into a clusterDAG node while making a few
// checks
func CborDataToNode(raw []byte, format string) (ipld.Node, error) {
	if format != "cbor" {
		return nil, fmt.Errorf("unexpected shard node format %s", format)
	}
	shardCid, err := cid.NewPrefixV1(cid.DagCBOR, hashFn).Sum(raw)
	if err != nil {
		return nil, err
	}
	shardBlk, err := blocks.NewBlockWithCid(raw, shardCid)
	if err != nil {
		return nil, err
	}
	shardNode, err := ipld.Decode(shardBlk)
	if err != nil {
		return nil, err
	}
	return shardNode, nil
}

func makeDAGSimple(ctx context.Context, dagObj map[string]cid.Cid) (ipld.Node, error) {
	node, err := cbor.WrapObject(
		dagObj,
		hashFn, mh.DefaultLengths[hashFn],
	)
	if err != nil {
		return nil, err
	}
	return node, err
}

// makeDAG parses a dagObj which stores all of the node-links a shardDAG
// is responsible for tracking.  In general a single node of links may exceed
// the capacity of an ipfs block.  In this case an indirect node in the
// shardDAG is constructed that references "leaf shardNodes" that themselves
// carry links to the data nodes being tracked. The head of the output slice
// is always the root of the shardDAG, i.e. the ipld node that should be
// recursively pinned to track the shard
func makeDAG(ctx context.Context, dagObj map[string]cid.Cid) ([]ipld.Node, error) {
	// FIXME: We have a 4MB limit on the block size enforced by bitswap:
	// https://github.com/libp2p/go-libp2p/core/blob/master/network/network.go#L23

	// No indirect node
	if len(dagObj) <= MaxLinks {
		n, err := makeDAGSimple(ctx, dagObj)
		return []ipld.Node{n}, err
	}
	// Indirect node required
	leafNodes := make([]ipld.Node, 0)       // shardNodes with links to data
	indirectObj := make(map[string]cid.Cid) // shardNode with links to shardNodes
	numFullLeaves := len(dagObj) / MaxLinks
	for i := 0; i <= numFullLeaves; i++ {
		leafObj := make(map[string]cid.Cid)
		for j := 0; j < MaxLinks; j++ {
			c, ok := dagObj[fmt.Sprintf("%d", i*MaxLinks+j)]
			if !ok { // finished with this leaf before filling all the way
				if i != numFullLeaves {
					panic("bad state, should never be here")
				}
				break
			}
			leafObj[fmt.Sprintf("%d", j)] = c
		}
		leafNode, err := makeDAGSimple(ctx, leafObj)
		if err != nil {
			return nil, err
		}
		indirectObj[fmt.Sprintf("%d", i)] = leafNode.Cid()
		leafNodes = append(leafNodes, leafNode)
	}
	indirectNode, err := makeDAGSimple(ctx, indirectObj)
	if err != nil {
		return nil, err
	}
	nodes := append([]ipld.Node{indirectNode}, leafNodes...)
	return nodes, nil
}

// TODO: decide whether this is worth including. Is precision important for
// most usecases?  Is being a little over the shard size a serious problem?
// Is precision worth the cost to maintain complex accounting for metadata
// size (cid sizes will vary in general, cluster dag cbor format may
// grow to vary unpredictably in size)
// byteCount returns the number of bytes the dagObj will occupy when
//serialized into an ipld DAG
/*func byteCount(obj dagObj) uint64 {
	// 1 byte map overhead
	// for each entry:
	//    1 byte indicating text
	//    1 byte*(number digits) for key
	//    2 bytes for link tag
	//    35 bytes for each cid
	count := 1
	for key := range obj {
		count += fixedPerLink
		count += len(key)
	}
	return uint64(count) + indirectCount(len(obj))
}

// indirectCount returns the number of bytes needed to serialize the indirect
// node structure of the shardDAG based on the number of links being tracked.
func indirectCount(linkNum int) uint64 {
	q := linkNum / MaxLinks
	if q == 0 { // no indirect node needed
		return 0
	}
	dummyIndirect := make(map[string]cid.Cid)
	for key := 0; key <= q; key++ {
		dummyIndirect[fmt.Sprintf("%d", key)] = nil
	}
	// Count bytes of entries of single indirect node and add the map
	// overhead for all leaf nodes other than the original
	return byteCount(dummyIndirect) + uint64(q)
}

// Return the number of bytes added to the total shard node metadata DAG when
// adding a new link to the given dagObj.
func deltaByteCount(obj dagObj) uint64 {
	linkNum := len(obj)
	q1 := linkNum / MaxLinks
	q2 := (linkNum + 1) / MaxLinks
	count := uint64(fixedPerLink)
	count += uint64(len(fmt.Sprintf("%d", len(obj))))

	// new shard nodes created by adding link
	if q1 != q2 {
		// first new leaf node created, i.e. indirect created too
		if q2 == 1 {
			count++                   // map overhead of indirect node
			count += 1 + fixedPerLink // fixedPerLink + len("0")
		}

		// added to indirect node
		count += fixedPerLink
		count += uint64(len(fmt.Sprintf("%d", q2)))

		// overhead of new leaf node
		count++
	}
	return count
}
*/
