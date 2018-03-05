package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/ipfs/ipfs-cluster/api"
)

func jsonFormatObject(resp interface{}) {
	switch resp.(type) {
	case nil:
		return
	case api.ID:
		jsonFormatPrint(resp.(api.ID).ToSerial())
	case api.GlobalPinInfo:
		jsonFormatPrint(resp.(api.GlobalPinInfo).ToSerial())
	case api.Pin:
		jsonFormatPrint(resp.(api.Pin).ToSerial())
	case api.AddedOutput:
		jsonFormatPrint(resp.(api.AddedOutput))
	case api.Version:
		jsonFormatPrint(resp.(api.Version))
	case api.Error:
		jsonFormatPrint(resp.(api.Error))
	case []api.ID:
		r := resp.([]api.ID)
		serials := make([]api.IDSerial, len(r), len(r))
		for i, item := range r {
			serials[i] = item.ToSerial()
		}
		jsonFormatPrint(serials)

	case []api.GlobalPinInfo:
		r := resp.([]api.GlobalPinInfo)
		serials := make([]api.GlobalPinInfoSerial, len(r), len(r))
		for i, item := range r {
			serials[i] = item.ToSerial()
		}
		jsonFormatPrint(serials)
	case []api.Pin:
		r := resp.([]api.Pin)
		serials := make([]api.PinSerial, len(r), len(r))
		for i, item := range r {
			serials[i] = item.ToSerial()
		}
		jsonFormatPrint(serials)
	case []api.AddedOutput:
		serials := resp.([]api.AddedOutput)
		jsonFormatPrint(serials)
	default:
		checkErr("", errors.New("unsupported type returned"))
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
	case api.ID:
		serial := resp.(api.ID).ToSerial()
		textFormatPrintIDSerial(&serial)
	case api.GlobalPinInfo:
		serial := resp.(api.GlobalPinInfo).ToSerial()
		textFormatPrintGPInfo(&serial)
	case api.Pin:
		serial := resp.(api.Pin).ToSerial()
		textFormatPrintPin(&serial)
	case api.AddedOutput:
		serial := resp.(api.AddedOutput)
		textFormatPrintAddedOutput(&serial)
	case api.Version:
		serial := resp.(api.Version)
		textFormatPrintVersion(&serial)
	case api.Error:
		serial := resp.(api.Error)
		textFormatPrintError(&serial)
	case []api.ID:
		for _, item := range resp.([]api.ID) {
			textFormatObject(item)
		}
	case []api.GlobalPinInfo:
		for _, item := range resp.([]api.GlobalPinInfo) {
			textFormatObject(item)
		}
	case []api.Pin:
		for _, item := range resp.([]api.Pin) {
			textFormatObject(item)
		}
	case []api.AddedOutput:
		for _, item := range resp.([]api.AddedOutput) {
			textFormatObject(item)
		}
	default:
		checkErr("", errors.New("unsupported type returned"))
	}
}

func textFormatPrintIDSerial(obj *api.IDSerial) {
	if obj.Error != "" {
		fmt.Printf("%s | ERROR: %s\n", obj.ID, obj.Error)
		return
	}

	fmt.Printf("%s | %s | Sees %d other peers\n", obj.ID, obj.Peername, len(obj.ClusterPeers)-1)
	addrs := make(sort.StringSlice, 0, len(obj.Addresses))
	for _, a := range obj.Addresses {
		addrs = append(addrs, string(a))
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
		ipfsAddrs = append(ipfsAddrs, string(a))
	}
	ipfsAddrs.Sort()
	fmt.Printf("  > IPFS: %s\n", obj.IPFS.ID)
	for _, a := range ipfsAddrs {
		fmt.Printf("    - %s\n", a)
	}
}

func textFormatPrintGPInfo(obj *api.GlobalPinInfoSerial) {
	fmt.Printf("%s :\n", obj.Cid)
	peers := make(sort.StringSlice, 0, len(obj.PeerMap))
	for k := range obj.PeerMap {
		peers = append(peers, k)
	}
	peers.Sort()

	for _, k := range peers {
		v := obj.PeerMap[k]
		if v.Error != "" {
			fmt.Printf("    > Peer %s : ERROR | %s\n", k, v.Error)
			continue
		}
		fmt.Printf("    > Peer %s : %s | %s\n", k, strings.ToUpper(v.Status), v.TS)
	}
}

func textFormatPrintPInfo(obj *api.PinInfoSerial) {
	gpinfo := api.GlobalPinInfoSerial{
		Cid: obj.Cid,
		PeerMap: map[string]api.PinInfoSerial{
			obj.Peer: *obj,
		},
	}
	textFormatPrintGPInfo(&gpinfo)
}

func textFormatPrintVersion(obj *api.Version) {
	fmt.Println(obj.Version)
}

func textFormatPrintPin(obj *api.PinSerial) {
	fmt.Printf("%s | %s | ", obj.Cid, obj.Name)

	if obj.ReplicationFactorMin < 0 {
		fmt.Printf("Repl. Factor: -1 | Allocations: [everywhere]\n")
	} else {
		var sortAlloc sort.StringSlice = obj.Allocations
		sortAlloc.Sort()
		fmt.Printf("Repl. Factor: %d--%d | Allocations: %s\n",
			obj.ReplicationFactorMin, obj.ReplicationFactorMax,
			sortAlloc)
	}
}

func textFormatPrintAddedOutput(obj *api.AddedOutput) {
	if obj.Hash != "" {
		fmt.Printf("adding %s %s\n", obj.Hash, obj.Name)
	}
}

func textFormatPrintError(obj *api.Error) {
	fmt.Printf("An error occurred:\n")
	fmt.Printf("  Code: %d\n", obj.Code)
	fmt.Printf("  Message: %s\n", obj.Message)
}
