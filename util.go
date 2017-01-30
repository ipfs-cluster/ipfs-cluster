package ipfscluster

import (
	"fmt"

	peer "github.com/libp2p/go-libp2p-peer"
	ma "github.com/multiformats/go-multiaddr"
)

// The copy functions below are used in calls to Cluste.multiRPC()
func copyPIDsToIfaces(in []peer.ID) []interface{} {
	ifaces := make([]interface{}, len(in), len(in))
	for i := range in {
		ifaces[i] = &in[i]
	}
	return ifaces
}

func copyIDSerialsToIfaces(in []IDSerial) []interface{} {
	ifaces := make([]interface{}, len(in), len(in))
	for i := range in {
		ifaces[i] = &in[i]
	}
	return ifaces
}

func copyPinInfoToIfaces(in []PinInfo) []interface{} {
	ifaces := make([]interface{}, len(in), len(in))
	for i := range in {
		ifaces[i] = &in[i]
	}
	return ifaces
}

func copyPinInfoSliceToIfaces(in [][]PinInfo) []interface{} {
	ifaces := make([]interface{}, len(in), len(in))
	for i := range in {
		ifaces[i] = &in[i]
	}
	return ifaces
}

func copyEmptyStructToIfaces(in []struct{}) []interface{} {
	ifaces := make([]interface{}, len(in), len(in))
	for i := range in {
		ifaces[i] = &in[i]
	}
	return ifaces
}

func multiaddrSplit(addr ma.Multiaddr) (peer.ID, ma.Multiaddr, error) {
	pid, err := addr.ValueForProtocol(ma.P_IPFS)
	if err != nil {
		err = fmt.Errorf("Invalid peer multiaddress: %s: %s", addr, err)
		logger.Error(err)
		return "", nil, err
	}

	ipfs, _ := ma.NewMultiaddr("/ipfs/" + pid)
	decapAddr := addr.Decapsulate(ipfs)

	peerID, err := peer.IDB58Decode(pid)
	if err != nil {
		err = fmt.Errorf("Invalid peer ID in multiaddress: %s: %s", pid)
		logger.Error(err)
		return "", nil, err
	}
	return peerID, decapAddr, nil
}
