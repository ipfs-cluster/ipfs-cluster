package main

import (
	"fmt"
	"go/format"
	"os"
	"reflect"
	"strings"

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
// from the existing one, marking new endpoints as NEW. Redirect stdout
// into rpc_policy.go and set the NEW endpoints to their correct type (make
// sure you have recompiled this binary with the current version of the code).
============================================================================`)
	fmt.Fprintln(os.Stderr)

	var rpcPolicyDotGo strings.Builder

	rpcPolicyDotGo.WriteString("package ipfscluster\n\n")
	rpcPolicyDotGo.WriteString("// This file can be generated with rpcutil/policygen.\n\n")
	rpcPolicyDotGo.WriteString(`
// DefaultRPCPolicy associates all rpc endpoints offered by cluster peers to an
// endpoint type. See rpcutil/policygen.go as a quick way to generate this
// without missing any endpoint.`)
	rpcPolicyDotGo.WriteString("\nvar DefaultRPCPolicy = map[string]RPCEndpointType{\n")

	for _, c := range rpcComponents {
		t := reflect.TypeOf(c)

		rpcPolicyDotGo.WriteString("//  " + cluster.RPCServiceID(c) + " methods\n")
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

			fmt.Fprintf(&rpcPolicyDotGo, "\"%s\": %s, %s\n", name, rpcTStr, comment)
		}
		rpcPolicyDotGo.WriteString("\n")
	}

	rpcPolicyDotGo.WriteString("}\n")
	src, err := format.Source([]byte(rpcPolicyDotGo.String()))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(string(src))
}
