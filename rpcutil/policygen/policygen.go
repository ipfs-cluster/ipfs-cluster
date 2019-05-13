package main

import (
	"fmt"
	"os"
	"reflect"

	cluster "github.com/ipfs/ipfs-cluster"
)

func rpcTypeStr(t cluster.RPCEndpointType) string {
	switch t {
	case cluster.RPCClosed:
		return "RPCClosed"
	case cluster.RPCTrusted:
		return "RPCTrusted"
	case cluster.RPCOpen:
		return "RPCOpen"
	default:
		return "ERROR"
	}
}

var comments = map[string]string{
	"Cluster.PeerAdd":          "Used by Join()",
	"Cluster.Peers":            "Used by ConnectGraph()",
	"Cluster.Pins":             "Used in stateless tracker, ipfsproxy, restapi",
	"Cluster.SyncAllLocal":     "Called in broadcast from SyncAll()",
	"Cluster.SyncLocal":        "Called in broadcast from Sync()",
	"PinTracker.Recover":       "Called in broadcast from Recover()",
	"PinTracker.RecoverAll":    "Broadcast in RecoverAll unimplemented",
	"Pintracker.Status":        "Called in broadcast from Status()",
	"Pintracker.StatusAll":     "Called in broadcast from StatusAll()",
	"IPFSConnector.BlockPut":   "Called from Add()",
	"IPFSConnector.RepoStat":   "Called in broadcast from proxy/repo/stat",
	"IPFSConnector.SwarmPeers": "Called in ConnectGraph",
	"Consensus.AddPeer":        "Called by Raft/redirect to leader",
	"Consensus.LogPin":         "Called by Raft/redirect to leader",
	"Consensus.LogUnpin":       "Called by Raft/redirect to leader",
	"Consensus.RmPeer":         "Called by Raft/redirect to leader",
}

func main() {
	rpcComponents := []interface{}{
		&cluster.ClusterRPCAPI{},
		&cluster.PinTrackerRPCAPI{},
		&cluster.IPFSConnectorRPCAPI{},
		&cluster.ConsensusRPCAPI{},
		&cluster.PeerMonitorRPCAPI{},
	}

	fmt.Fprintln(os.Stderr, `
// The below generated policy keeps the endpoint types
// from the exiting one, marking new endpoints as NEW. Redirect stdout
// into rpc_policy.go and set the NEW endpoints to their correct type (make
// sure you have recompiled this binary with the current version of the code).
============================================================================`)
	fmt.Fprintln(os.Stderr)

	fmt.Println("package ipfscluster")
	fmt.Println()
	fmt.Println("// This file can be generated with rpcutil/policygen.")
	fmt.Println()
	fmt.Println(`
// DefaultRPCPolicy associates all rpc endpoints offered by cluster peers to an
// endpoint type. See rpcutil/policygen.go as a quick way to generate this
// without missing any endpoint.`)
	fmt.Println("var DefaultRPCPolicy = map[string]RPCEndpointType{")

	for _, c := range rpcComponents {
		t := reflect.TypeOf(c)

		fmt.Println("        //", cluster.RPCServiceID(c), "methods")
		for i := 0; i < t.NumMethod(); i++ {
			method := t.Method(i)
			name := cluster.RPCServiceID(c) + "." + method.Name
			rpcT, ok := cluster.DefaultRPCPolicy[name]
			rpcTStr := "NEW"
			if ok {
				rpcTStr = rpcTypeStr(rpcT)
			}
			comment, ok := comments[name]
			if ok {
				comment = "// " + comment
			}

			fmt.Printf("        \"%s\": %s, %s\n", name, rpcTStr, comment)
		}
		fmt.Println()
	}

	fmt.Println("}")
}
