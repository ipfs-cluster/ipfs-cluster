package ipfscluster

import (
	"errors"
	"fmt"

	"github.com/ipfs/ipfs-cluster/api"

	ma "gx/ipfs/QmSWLfmj5frN9xVLMMN846dMDriy5wN5jeghUm7aTW3DAG/go-multiaddr"
	peer "gx/ipfs/QmZcUPvPhD1Xvk6mwijYF8AfR3mG31S1YsEfHG4khrFPRr/go-libp2p-peer"
	host "gx/ipfs/QmbzbRyd22gcW92U1rA2yKagB3myMYhk45XBknJ49F9XWJ/go-libp2p-host"
)

// The copy functions below are used in calls to Cluste.multiRPC()
// func copyPIDsToIfaces(in []peer.ID) []interface{} {
// 	ifaces := make([]interface{}, len(in), len(in))
// 	for i := range in {
// 		ifaces[i] = &in[i]
// 	}
// 	return ifaces
// }

func copyIDSerialsToIfaces(in []api.IDSerial) []interface{} {
	ifaces := make([]interface{}, len(in), len(in))
	for i := range in {
		ifaces[i] = &in[i]
	}
	return ifaces
}

func copyPinInfoSerialToIfaces(in []api.PinInfoSerial) []interface{} {
	ifaces := make([]interface{}, len(in), len(in))
	for i := range in {
		ifaces[i] = &in[i]
	}
	return ifaces
}

func copyPinInfoSerialSliceToIfaces(in [][]api.PinInfoSerial) []interface{} {
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
		err = fmt.Errorf("invalid peer multiaddress: %s: %s", addr, err)
		logger.Error(err)
		return "", nil, err
	}

	ipfs, _ := ma.NewMultiaddr("/ipfs/" + pid)
	decapAddr := addr.Decapsulate(ipfs)

	peerID, err := peer.IDB58Decode(pid)
	if err != nil {
		err = fmt.Errorf("invalid peer ID in multiaddress: %s: %s", pid, err)
		logger.Error(err)
		return "", nil, err
	}
	return peerID, decapAddr, nil
}

func multiaddrJoin(addr ma.Multiaddr, p peer.ID) ma.Multiaddr {
	pidAddr, err := ma.NewMultiaddr("/ipfs/" + peer.IDB58Encode(p))
	// let this break badly
	if err != nil {
		panic("called multiaddrJoin with bad peer!")
	}
	return addr.Encapsulate(pidAddr)
}

func peersFromMultiaddrs(addrs []ma.Multiaddr) []peer.ID {
	var pids []peer.ID
	for _, addr := range addrs {
		pid, _, err := multiaddrSplit(addr)
		if err != nil {
			continue
		}
		pids = append(pids, pid)
	}
	return pids
}

// // connect to a peer ID.
// func connectToPeer(ctx context.Context, h host.Host, id peer.ID, addr ma.Multiaddr) error {
// 	err := h.Connect(ctx, peerstore.PeerInfo{
// 		ID:    id,
// 		Addrs: []ma.Multiaddr{addr},
// 	})
// 	return err
// }

// // return the local multiaddresses used to communicate to a peer.
// func localMultiaddrsTo(h host.Host, pid peer.ID) []ma.Multiaddr {
// 	var addrs []ma.Multiaddr
// 	conns := h.Network().ConnsToPeer(pid)
// 	logger.Debugf("conns to %s are: %s", pid, conns)
// 	for _, conn := range conns {
// 		addrs = append(addrs, multiaddrJoin(conn.LocalMultiaddr(), h.ID()))
// 	}
// 	return addrs
// }

// If we have connections open to that PID and they are using a different addr
// then we return the one we are using, otherwise the one provided
func getRemoteMultiaddr(h host.Host, pid peer.ID, addr ma.Multiaddr) ma.Multiaddr {
	conns := h.Network().ConnsToPeer(pid)
	if len(conns) > 0 {
		return multiaddrJoin(conns[0].RemoteMultiaddr(), pid)
	}
	return multiaddrJoin(addr, pid)
}

func pinInfoSliceToSerial(pi []api.PinInfo) []api.PinInfoSerial {
	pis := make([]api.PinInfoSerial, len(pi), len(pi))
	for i, v := range pi {
		pis[i] = v.ToSerial()
	}
	return pis
}

func globalPinInfoSliceToSerial(gpi []api.GlobalPinInfo) []api.GlobalPinInfoSerial {
	gpis := make([]api.GlobalPinInfoSerial, len(gpi), len(gpi))
	for i, v := range gpi {
		gpis[i] = v.ToSerial()
	}
	return gpis
}

func logError(fmtstr string, args ...interface{}) error {
	msg := fmt.Sprintf(fmtstr, args...)
	logger.Error(msg)
	return errors.New(msg)
}
