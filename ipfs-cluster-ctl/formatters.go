package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ipfs/ipfs-cluster/api"
)

const (
	formatNone = iota
	formatID
	formatGPInfo
	formatString
	formatVersion
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

	fmt.Printf("%s | %d peers\n", obj.ID, len(obj.ClusterPeers))
	fmt.Println("  > Addresses:")
	for _, a := range obj.Addresses {
		fmt.Printf("    - %s\n", a)
	}
	if obj.IPFS.Error != "" {
		fmt.Printf("  > IPFS ERROR: %s\n", obj.IPFS.Error)
		return
	}
	fmt.Printf("  > IPFS: %s\n", obj.IPFS.ID)
	for _, a := range obj.IPFS.Addresses {
		fmt.Printf("    - %s\n", a)
	}
}

func textFormatPrintGPinfo(obj *api.GlobalPinInfoSerial) {
	fmt.Printf("%s:\n", obj.Cid)
	for k, v := range obj.PeerMap {
		if v.Error != "" {
			fmt.Printf("  - %s ERROR: %s\n", k, v.Error)
			continue
		}
		fmt.Printf("    > Peer %s: %s | %s\n", k, strings.ToUpper(v.Status), v.TS)
	}
}

func textFormatPrintVersion(obj *api.Version) {
	fmt.Println(obj.Version)
}
