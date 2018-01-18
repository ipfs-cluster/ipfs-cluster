package main

import (
	"errors"
	"fmt"
	"io"
	"sort"

	peer "github.com/libp2p/go-libp2p-peer"
	dot "github.com/zenground0/go-dot"

	"github.com/ipfs/ipfs-cluster/api"
)

/*
   These functions are used to write an IPFS Cluster connectivity graph to a
   graphviz-style dot file.  Input an api.ConnectGraphSerial object, makeDot
   does some preprocessing and then passes all 3 link maps to a
   cluster-dotWriter which handles iterating over the link maps and writing
   dot file node and edge statements to make a dot-file graph.  Nodes are
   labeled with the go-libp2p-peer shortened peer id.  IPFS nodes are rendered
   with gold boundaries, Cluster nodes with blue.  Currently preprocessing
   consists of moving IPFS swarm peers not connected to any cluster peer to
   the IPFSLinks map in the event that the function was invoked with the
   allIpfs flag.  This allows all IPFS peers connected to the cluster to be
   rendered as nodes in the final graph.
*/

// nodeType specifies the type of node being represented in the dot file:
// either IPFS or Cluster
type nodeType int

const (
	tCluster nodeType = iota // The cluster node type
	tIpfs                    // The IPFS node type
)

var errUnfinishedWrite = errors.New("could not complete write of line to output")
var errUnknownNodeType = errors.New("unsupported node type. Expected cluster or ipfs")
var errCorruptOrdering = errors.New("expected pid to have an ordering within dot writer")

func makeDot(cg api.ConnectGraphSerial, w io.Writer, allIpfs bool) error {
	ipfsEdges := make(map[string][]string)
	for k, v := range cg.IPFSLinks {
		ipfsEdges[k] = make([]string, 0)
		for _, id := range v {
			if _, ok := cg.IPFSLinks[id]; ok || allIpfs {
				ipfsEdges[k] = append(ipfsEdges[k], id)
			}
			if allIpfs { // include all swarm peers in the graph
				if _, ok := ipfsEdges[id]; !ok {
					// if id in IPFSLinks this will be overwritten
					// if id not in IPFSLinks this will stay blank
					ipfsEdges[id] = make([]string, 0)
				}
			}
		}
	}

	dW := dotWriter{
		w:                w,
		dotGraph:         dot.NewGraph("cluster"),
		ipfsEdges:        ipfsEdges,
		clusterEdges:     cg.ClusterLinks,
		clusterIpfsEdges: cg.ClustertoIPFS,
		clusterNodes:     make(map[string]*dot.VertexDescription),
		ipfsNodes:        make(map[string]*dot.VertexDescription),
	}
	return dW.print()
}

type dotWriter struct {
	clusterNodes map[string]*dot.VertexDescription
	ipfsNodes    map[string]*dot.VertexDescription

	w        io.Writer
	dotGraph dot.DotGraph

	ipfsEdges        map[string][]string
	clusterEdges     map[string][]string
	clusterIpfsEdges map[string]string
}

// writes nodes to dot file output and creates and stores an ordering over nodes
func (dW *dotWriter) addNode(id string, nT nodeType) error {
	var node dot.VertexDescription
	node.Label = shortID(id)
	switch nT {
	case tCluster:
		node.ID = fmt.Sprintf("C%d", len(dW.clusterNodes))
		node.Color = "blue2"
		dW.clusterNodes[id] = &node
	case tIpfs:
		node.ID = fmt.Sprintf("I%d", len(dW.ipfsNodes))
		node.Color = "goldenrod"
		dW.ipfsNodes[id] = &node
	default:
		return errUnknownNodeType
	}
	dW.dotGraph.AddVertex(&node)

	return nil
}

func (dW *dotWriter) print() error {
	dW.dotGraph.AddComment("The nodes of the connectivity graph")
	dW.dotGraph.AddComment("The cluster-service peers")
	// Write cluster nodes, use sorted order for consistent labels
	for _, k := range sortedKeys(dW.clusterEdges) {
		err := dW.addNode(k, tCluster)
		if err != nil {
			return err
		}
	}
	dW.dotGraph.AddNewLine()

	dW.dotGraph.AddComment("The ipfs peers")
	// Write ipfs nodes, use sorted order for consistent labels
	for _, k := range sortedKeys(dW.ipfsEdges) {
		err := dW.addNode(k, tIpfs)
		if err != nil {
			return err
		}
	}
	dW.dotGraph.AddNewLine()

	dW.dotGraph.AddComment("Edges representing active connections in the cluster")
	dW.dotGraph.AddComment("The connections among cluster-service peers")
	// Write cluster edges
	for k, v := range dW.clusterEdges {
		for _, id := range v {
			toNode := dW.clusterNodes[k]
			fromNode := dW.clusterNodes[id]
			dW.dotGraph.AddEdge(toNode, fromNode, true)
		}
	}
	dW.dotGraph.AddNewLine()

	dW.dotGraph.AddComment("The connections between cluster peers and their ipfs daemons")
	// Write cluster to ipfs edges
	for k, id := range dW.clusterIpfsEdges {
		toNode := dW.clusterNodes[k]
		fromNode := dW.ipfsNodes[id]
		dW.dotGraph.AddEdge(toNode, fromNode, true)
	}
	dW.dotGraph.AddNewLine()

	dW.dotGraph.AddComment("The swarm peer connections among ipfs daemons in the cluster")
	// Write ipfs edges
	for k, v := range dW.ipfsEdges {
		for _, id := range v {
			toNode := dW.ipfsNodes[k]
			fromNode := dW.ipfsNodes[id]
			dW.dotGraph.AddEdge(toNode, fromNode, true)
		}
	}
	return dW.dotGraph.WriteDot(dW.w)
}

func sortedKeys(dict map[string][]string) []string {
	keys := make([]string, len(dict), len(dict))
	i := 0
	for k := range dict {
		keys[i] = k
		i++
	}
	sort.Strings(keys)
	return keys
}

// truncate the provided peer id string to the 3 last characters.  Odds of
// pairwise collisions are less than 1 in 200,000 so with 70 cluster peers
// the chances of a collision are still less than 1 in 100 (birthday paradox).
// As clusters grow bigger than this we can provide a flag for including
// more characters.
func shortID(peerString string) string {
	pid, err := peer.IDB58Decode(peerString)
	if err != nil {
		// We'll truncate ourselves
		// Should never get here but try to match with peer:String()
		if len(peerString) < 6 {

			return fmt.Sprintf("<peer.ID %s>", peerString)
		}
		return fmt.Sprintf("<peer.ID %s>", peerString[:6])
	}
	return pid.String()
}
