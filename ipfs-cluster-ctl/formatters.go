package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/ipfs/ipfs-cluster/api"
)

const (
	formatNone = iota
	formatID
	formatGPInfo
	formatString
	formatVersion
	formatPin
	formatError
)

type format int

func textFormat(body []byte, format int) {
	if len(body) < 2 {
		fmt.Println("")
	}

	slice := body[0] == '['
	if slice {
		textFormatSlice(body, format)
	} else {
		textFormatObject(body, format)
	}
}

func textFormatObject(body []byte, format int) {
	switch format {
	case formatID:
		var obj api.IDSerial
		textFormatDecodeOn(body, &obj)
		textFormatPrintIDSerial(&obj)
	case formatGPInfo:
		var obj api.GlobalPinInfoSerial
		textFormatDecodeOn(body, &obj)
		textFormatPrintGPinfo(&obj)
	case formatVersion:
		var obj api.Version
		textFormatDecodeOn(body, &obj)
		textFormatPrintVersion(&obj)
	case formatPin:
		var obj api.PinSerial
		textFormatDecodeOn(body, &obj)
		textFormatPrintPin(&obj)
	case formatError:
		var obj api.Error
		textFormatDecodeOn(body, &obj)
		textFormatPrintError(&obj)
	default:
		var obj interface{}
		textFormatDecodeOn(body, &obj)
		fmt.Printf("%s\n", obj)
	}
}

func textFormatSlice(body []byte, format int) {
	var rawMsg []json.RawMessage
	textFormatDecodeOn(body, &rawMsg)
	for _, raw := range rawMsg {
		textFormatObject(raw, format)
	}
}

func textFormatDecodeOn(body []byte, obj interface{}) {
	checkErr("decoding JSON", json.Unmarshal(body, obj))
}

func textFormatPrintIDSerial(obj *api.IDSerial) {
	if obj.Error != "" {
		fmt.Printf("%s | ERROR: %s\n", obj.ID, obj.Error)
		return
	}

	fmt.Printf("%s | Sees %d other peers\n", obj.ID, len(obj.ClusterPeers)-1)
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

func textFormatPrintGPinfo(obj *api.GlobalPinInfoSerial) {
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

func textFormatPrintVersion(obj *api.Version) {
	fmt.Println(obj.Version)
}

func textFormatPrintPin(obj *api.PinSerial) {
	fmt.Printf("%s | Allocations: ", obj.Cid)
	if obj.ReplicationFactor < 0 {
		fmt.Printf("[everywhere]\n")
	} else {
		var sortAlloc sort.StringSlice = obj.Allocations
		sortAlloc.Sort()
		fmt.Printf("%s\n", sortAlloc)
	}
}

func textFormatPrintError(obj *api.Error) {
	fmt.Printf("An error ocurred:\n")
	fmt.Printf("  Code: %d\n", obj.Code)
	fmt.Printf("  Message: %s\n", obj.Message)
}
