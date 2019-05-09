package main

import (
	"fmt"
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

func main() {
	rpcComponents := []interface{}{
		&cluster.ClusterRPCAPI{},
		&cluster.PinTrackerRPCAPI{},
		&cluster.IPFSConnectorRPCAPI{},
		&cluster.ConsensusRPCAPI{},
		&cluster.PeerMonitorRPCAPI{},
	}

	fmt.Println(`
// The below generated policy keeps the endpoint types
// from the exiting one, marking new endpoints as NEW. Copy-paste this policy
// into rpc_auth.go and set the NEW endpoints to their correct type.`)
	fmt.Println()

	fmt.Println("var RPCPolicy = map[string]RPCEndpointType{")

	for _, c := range rpcComponents {
		t := reflect.TypeOf(c)

		fmt.Println("        //", cluster.RPCServiceID(c), "methods")
		for i := 0; i < t.NumMethod(); i++ {
			method := t.Method(i)
			name := fmt.Sprintf("%s.%s", cluster.RPCServiceID(c), method.Name)
			rpcT, ok := cluster.DefaultRPCPolicy[name]
			rpcTStr := "NEW"
			if ok {
				rpcTStr = rpcTypeStr(rpcT)
			}

			fmt.Printf("        \"%s\": %s,\n", name, rpcTStr)
		}
		fmt.Println()
	}

	fmt.Println("}")
}
