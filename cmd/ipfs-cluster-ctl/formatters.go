package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ipfs/ipfs-cluster/api"

	peer "github.com/libp2p/go-libp2p-core/peer"

	humanize "github.com/dustin/go-humanize"
)

type addedOutputQuiet struct {
	*api.AddedOutput
	quiet bool
}

func jsonFormatObject(resp interface{}) {
	switch resp.(type) {
	case nil:
		return
	case []*addedOutputQuiet:
		// print original objects as in JSON it makes
		// no sense to have a human "quiet" output
		serials := resp.([]*addedOutputQuiet)
		var actual []*api.AddedOutput
		for _, s := range serials {
			actual = append(actual, s.AddedOutput)
		}
		jsonFormatPrint(actual)
	default:
		jsonFormatPrint(resp)
	}
}

func jsonFormatPrint(obj interface{}) {
	j, err := json.MarshalIndent(obj, "", "    ")
	checkErr("generating json output", err)
	fmt.Printf("%s\n", j)
}

func textFormatObject(resp interface{}) {
	switch resp.(type) {
	case nil:
		return
	case string:
		fmt.Println(resp)
	case *api.ID:
		textFormatPrintID(resp.(*api.ID))
	case *api.GlobalPinInfo:
		textFormatPrintGPInfo(resp.(*api.GlobalPinInfo))
	case *api.Pin:
		textFormatPrintPin(resp.(*api.Pin))
	case *api.AddedOutput:
		textFormatPrintAddedOutput(resp.(*api.AddedOutput))
	case *addedOutputQuiet:
		textFormatPrintAddedOutputQuiet(resp.(*addedOutputQuiet))
	case *api.Version:
		textFormatPrintVersion(resp.(*api.Version))
	case *api.Error:
		textFormatPrintError(resp.(*api.Error))
	case *api.Metric:
		textFormatPrintMetric(resp.(*api.Metric))
	case []*api.ID:
		for _, item := range resp.([]*api.ID) {
			textFormatObject(item)
		}
	case []*api.GlobalPinInfo:
		for _, item := range resp.([]*api.GlobalPinInfo) {
			textFormatObject(item)
		}
	case []*api.Pin:
		for _, item := range resp.([]*api.Pin) {
			textFormatObject(item)
		}
	case []*api.AddedOutput:
		for _, item := range resp.([]*api.AddedOutput) {
			textFormatObject(item)
		}
	case []*addedOutputQuiet:
		for _, item := range resp.([]*addedOutputQuiet) {
			textFormatObject(item)
		}
	case []*api.Metric:
		for _, item := range resp.([]*api.Metric) {
			textFormatObject(item)
		}
	case *api.GlobalRepoGC:
		textFormatPrintGlobalRepoGC(resp.(*api.GlobalRepoGC))
	case []string:
		for _, item := range resp.([]string) {
			textFormatObject(item)
		}
	default:
		checkErr("", errors.New("unsupported type returned"))
	}
}

func textFormatPrintID(obj *api.ID) {
	if obj.Error != "" {
		fmt.Printf("%s | ERROR: %s\n", obj.ID.Pretty(), obj.Error)
		return
	}

	fmt.Printf(
		"%s | %s | Sees %d other peers\n",
		obj.ID.Pretty(),
		obj.Peername,
		len(obj.ClusterPeers)-1,
	)

	addrs := make(sort.StringSlice, 0, len(obj.Addresses))
	for _, a := range obj.Addresses {
		addrs = append(addrs, a.String())
	}
	addrs.Sort()
	fmt.Println("  > Addresses:")
	for _, a := range addrs {
		fmt.Printf("    - %s\n", a)
	}
	if obj.IPFS.Error != "" {
		fmt.Printf("  > IPFS ERROR: %s\n", obj.IPFS.Error)
		return
	}

	ipfsAddrs := make(sort.StringSlice, 0, len(obj.Addresses))
	for _, a := range obj.IPFS.Addresses {
		ipfsAddrs = append(ipfsAddrs, a.String())
	}
	ipfsAddrs.Sort()
	fmt.Printf("  > IPFS: %s\n", obj.IPFS.ID.Pretty())
	for _, a := range ipfsAddrs {
		fmt.Printf("    - %s\n", a)
	}
}

func textFormatPrintGPInfo(obj *api.GlobalPinInfo) {
	fmt.Printf("%s :\n", obj.Cid)
	peers := make([]string, 0, len(obj.PeerMap))
	for k := range obj.PeerMap {
		peers = append(peers, k)
	}
	sort.Strings(peers)

	for _, k := range peers {
		v := obj.PeerMap[k]
		if len(v.PeerName) > 0 {
			fmt.Printf("    > %-20s : %s", v.PeerName, strings.ToUpper(v.Status.String()))
		} else {
			fmt.Printf("    > %-20s : %s", k, strings.ToUpper(v.Status.String()))
		}
		if v.Error != "" {
			fmt.Printf(": %s", v.Error)
		}
		txt, _ := v.TS.MarshalText()
		fmt.Printf(" | %s\n", txt)
	}
}

func textFormatPrintPInfo(obj *api.PinInfo) {
	gpinfo := api.GlobalPinInfo{
		Cid: obj.Cid,
		PeerMap: map[string]*api.PinInfo{
			peer.IDB58Encode(obj.Peer): obj,
		},
	}
	textFormatPrintGPInfo(&gpinfo)
}

func textFormatPrintVersion(obj *api.Version) {
	fmt.Println(obj.Version)
}

func textFormatPrintPin(obj *api.Pin) {
	fmt.Printf("%s | %s | %s | ", obj.Cid, obj.Name, strings.ToUpper(obj.Type.String()))

	if obj.ReplicationFactorMin < 0 {
		fmt.Printf("Repl. Factor: -1 | Allocations: [everywhere]")
	} else {
		sortAlloc := api.PeersToStrings(obj.Allocations)
		sort.Strings(sortAlloc)
		fmt.Printf("Repl. Factor: %d--%d | Allocations: %s",
			obj.ReplicationFactorMin, obj.ReplicationFactorMax,
			sortAlloc)
	}
	var recStr string
	switch obj.MaxDepth {
	case 0:
		recStr = "Direct"
	case -1:
		recStr = "Recursive"
	default:
		recStr = fmt.Sprintf("Recursive-%d", obj.MaxDepth)
	}

	fmt.Printf(" | %s", recStr)

	fmt.Printf(" | Metadata:")
	if len(obj.Metadata) == 0 {
		fmt.Printf(" no\n")
	} else {
		fmt.Printf(" yes\n")
	}
}

func textFormatPrintAddedOutput(obj *api.AddedOutput) {
	fmt.Printf("added %s %s\n", obj.Cid, obj.Name)
}

func textFormatPrintAddedOutputQuiet(obj *addedOutputQuiet) {
	if obj.quiet {
		fmt.Printf("%s\n", obj.AddedOutput.Cid)
	} else {
		textFormatPrintAddedOutput(obj.AddedOutput)
	}
}

func textFormatPrintMetric(obj *api.Metric) {
	if obj.Name == "freespace" {
		u, err := strconv.ParseUint(obj.Value, 10, 64)
		checkErr("parsing to uint64", err)
		fmt.Printf("%s | freespace: %s | Expires in: %s\n", peer.IDB58Encode(obj.Peer), humanize.Bytes(u), humanize.Time(time.Unix(0, obj.Expire)))
		return
	}

	fmt.Printf("%s | %s | Expires in: %s\n", peer.IDB58Encode(obj.Peer), obj.Name, humanize.Time(time.Unix(0, obj.Expire)))
}

func textFormatPrintGlobalRepoGC(obj *api.GlobalRepoGC) {
	peers := make(sort.StringSlice, 0, len(obj.PeerMap))
	for peer := range obj.PeerMap {
		peers = append(peers, peer)
	}
	peers.Sort()

	for _, peer := range peers {
		item := obj.PeerMap[peer]
		// If peer name is set, use it instead of peer ID.
		if len(item.Peername) > 0 {
			peer = item.Peername
		}
		if item.Error != "" {
			fmt.Printf("%-15s | ERROR: %s\n", peer, item.Error)
		} else {
			fmt.Printf("%-15s\n", peer)
		}

		fmt.Printf("  > CIDs:\n")
		for _, key := range item.Keys {
			if key.Error != "" {
				// key.Key will be empty
				fmt.Printf("    - ERROR: %s\n", key.Error)
				continue
			}

			fmt.Printf("    - %s\n", key.Key)
		}
	}
}

func textFormatPrintError(obj *api.Error) {
	fmt.Printf("An error occurred:\n")
	fmt.Printf("  Code: %d\n", obj.Code)
	fmt.Printf("  Message: %s\n", obj.Message)
}

func trackerStatusAllString() string {
	var strs []string
	for _, st := range api.TrackerStatusAll() {
		strs = append(strs, "  - "+st.String())
	}

	sort.Strings(strs)
	return strings.Join(strs, "\n")
}
