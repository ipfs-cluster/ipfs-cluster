package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/libp2p/go-libp2p/core/protocol"

	humanize "github.com/dustin/go-humanize"
)

type addedOutputQuiet struct {
	api.AddedOutput
	quiet bool
}

func jsonFormatObject(resp interface{}) {
	switch r := resp.(type) {
	case nil:
		return
	case []addedOutputQuiet:
		// print original objects as in JSON it makes
		// no sense to have a human "quiet" output
		var actual []api.AddedOutput
		for _, s := range r {
			actual = append(actual, s.AddedOutput)
		}
		jsonFormatPrint(actual)
	default:
		jsonFormatPrint(resp)
	}
}

func jsonFormatPrint(obj interface{}) {
	print := func(o interface{}) {
		j, err := json.MarshalIndent(o, "", "    ")
		checkErr("generating json output", err)
		fmt.Printf("%s\n", j)
	}

	switch r := obj.(type) {
	case chan api.Pin:
		for o := range r {
			print(o)
		}
	case chan api.GlobalPinInfo:
		for o := range r {
			print(o)
		}
	case chan api.ID:
		for o := range r {
			print(o)
		}
	default:
		print(obj)
	}

}

func textFormatObject(resp interface{}) {
	switch r := resp.(type) {
	case nil:
		return
	case string:
		fmt.Println(resp)
	case api.ID:
		textFormatPrintID(r)
	case api.GlobalPinInfo:
		textFormatPrintGPInfo(r)
	case api.Pin:
		textFormatPrintPin(r)
	case api.AddedOutput:
		textFormatPrintAddedOutput(r)
	case addedOutputQuiet:
		textFormatPrintAddedOutputQuiet(r)
	case api.Version:
		textFormatPrintVersion(r)
	case api.Error:
		textFormatPrintError(r)
	case api.Metric:
		textFormatPrintMetric(r)
	case api.Alert:
		textFormatPrintAlert(r)
	case api.BandwidthByProtocol:
		textFormatPrintBandwidthByProtocol(r)
	case chan api.ID:
		for item := range r {
			textFormatObject(item)
		}
	case chan api.GlobalPinInfo:
		for item := range r {
			textFormatObject(item)
		}
	case chan api.Pin:
		for item := range r {
			textFormatObject(item)
		}
	case []api.AddedOutput:
		for _, item := range r {
			textFormatObject(item)
		}
	case []addedOutputQuiet:
		for _, item := range r {
			textFormatObject(item)
		}
	case []api.Metric:
		for _, item := range r {
			textFormatObject(item)
		}
	case api.GlobalRepoGC:
		textFormatPrintGlobalRepoGC(r)
	case []string:
		for _, item := range r {
			textFormatObject(item)
		}
	case []api.Alert:
		for _, item := range r {
			textFormatObject(item)
		}
	default:
		checkErr("", errors.New("unsupported type returned"+reflect.TypeOf(r).String()))
	}
}

func textFormatPrintID(obj api.ID) {
	if obj.Error != "" {
		fmt.Printf("%s | ERROR: %s\n", obj.ID, obj.Error)
		return
	}

	fmt.Printf(
		"%s | %s | Sees %d other peers\n",
		obj.ID,
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
	fmt.Printf("  > IPFS: %s\n", obj.IPFS.ID)
	for _, a := range ipfsAddrs {
		fmt.Printf("    - %s\n", a)
	}
}

func textFormatPrintGPInfo(obj api.GlobalPinInfo) {
	var b strings.Builder

	peers := make([]string, 0, len(obj.PeerMap))
	for k := range obj.PeerMap {
		peers = append(peers, k)
	}
	sort.Strings(peers)

	fmt.Fprintf(&b, "%s", obj.Cid)
	if obj.Name != "" {
		fmt.Fprintf(&b, " | %s", obj.Name)
	}

	b.WriteString(":\n")

	for _, k := range peers {
		v := obj.PeerMap[k]
		if len(v.PeerName) > 0 {
			fmt.Fprintf(&b, "    > %-20s : %s", v.PeerName, strings.ToUpper(v.Status.String()))
		} else {
			fmt.Fprintf(&b, "    > %-20s : %s", k, strings.ToUpper(v.Status.String()))
		}
		if v.Error != "" {
			fmt.Fprintf(&b, ": %s", v.Error)
		}
		txt, _ := v.TS.MarshalText()
		fmt.Fprintf(&b, " | %s", txt)
		fmt.Fprintf(&b, " | Attempts: %d", v.AttemptCount)
		fmt.Fprintf(&b, " | Priority: %t", v.PriorityPin)
		fmt.Fprintf(&b, "\n")
	}
	fmt.Print(b.String())
}

func textFormatPrintVersion(obj api.Version) {
	fmt.Println(obj.Version)
}

func textFormatPrintPin(obj api.Pin) {
	t := strings.ToUpper(obj.Type.String())
	if obj.Mode == api.PinModeDirect {
		t = t + "-DIRECT"
	}

	fmt.Printf("%s | %s | %s | ", obj.Cid, obj.Name, t)

	if obj.IsPinEverywhere() {
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
		fmt.Printf(" no")
	} else {
		fmt.Printf(" yes")
	}
	expireAt := "∞"
	if !obj.ExpireAt.IsZero() {
		expireAt = obj.ExpireAt.Format("2006-01-02 15:04:05")
	}
	fmt.Printf(" | Exp: %s", expireAt)

	added := "unknown"
	if !obj.Timestamp.IsZero() {
		added = obj.Timestamp.Format("2006-01-02 15:04:05")
	}
	fmt.Printf(" | Added: %s\n", added)
}

func textFormatPrintAddedOutput(obj api.AddedOutput) {
	fmt.Printf("added %s %s\n", obj.Cid, obj.Name)
}

func textFormatPrintAddedOutputQuiet(obj addedOutputQuiet) {
	if obj.quiet {
		fmt.Printf("%s\n", obj.AddedOutput.Cid)
	} else {
		textFormatPrintAddedOutput(obj.AddedOutput)
	}
}

func textFormatPrintMetric(obj api.Metric) {
	v := obj.Value
	if obj.Name == "freespace" && obj.Weight > 0 {
		v = humanize.Bytes(uint64(obj.Weight))
	}

	fmt.Printf("%s | %s: %s | Expires in: %s\n", obj.Peer, obj.Name, v, humanize.Time(time.Unix(0, obj.Expire)))
}

func textFormatPrintAlert(obj api.Alert) {
	fmt.Printf("%s: %s. Expired at: %s. Triggered at: %s\n",
		obj.Peer,
		obj.Name,
		humanize.Time(time.Unix(0, obj.Expire)),
		humanize.Time(obj.TriggeredAt),
	)
}

func textFormatPrintBandwidthByProtocol(obj api.BandwidthByProtocol) {
	var keys []string
	for k := range obj {
		keys = append(keys, string(k))
	}

	sort.Strings(keys)

	for _, k := range keys {
		stat := obj[protocol.ID(k)]
		fmt.Printf("%25s: In: %7s. Out: %7s. RateIn: %.2fB/s. RateOut: %.2fB/s\n",
			k, humanize.Bytes(uint64(stat.TotalIn)), humanize.Bytes(uint64(stat.TotalOut)),
			stat.RateIn, stat.RateOut,
		)
	}
}

func textFormatPrintGlobalRepoGC(obj api.GlobalRepoGC) {
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

func textFormatPrintError(obj api.Error) {
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
