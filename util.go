package ipfscluster

import (
	"errors"
	"fmt"

	"github.com/ipfs/ipfs-cluster/api"

	cid "github.com/ipfs/go-cid"
	host "github.com/libp2p/go-libp2p-host"
	peer "github.com/libp2p/go-libp2p-peer"
	ma "github.com/multiformats/go-multiaddr"
)

// PeersFromMultiaddrs returns all the different peers in the given addresses.
// each peer only will appear once in the result, even if several
// multiaddresses for it are provided.
func PeersFromMultiaddrs(addrs []ma.Multiaddr) []peer.ID {
	var pids []peer.ID
	pm := make(map[peer.ID]struct{})
	for _, addr := range addrs {
		pid, _, err := api.Libp2pMultiaddrSplit(addr)
		if err != nil {
			continue
		}
		_, ok := pm[pid]
		if !ok {
			pm[pid] = struct{}{}
			pids = append(pids, pid)
		}
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
		return api.MustLibp2pMultiaddrJoin(conns[0].RemoteMultiaddr(), pid)
	}
	return api.MustLibp2pMultiaddrJoin(addr, pid)
}

func pinInfoSliceToSerial(pi []api.PinInfo) []api.PinInfoSerial {
	pis := make([]api.PinInfoSerial, len(pi), len(pi))
	for i, v := range pi {
		pis[i] = v.ToSerial()
	}
	return pis
}

// GlobalPinInfoSliceToSerial is a helper function for serializing a slice of
// api.GlobalPinInfos.
func GlobalPinInfoSliceToSerial(gpi []api.GlobalPinInfo) []api.GlobalPinInfoSerial {
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

func containsPeer(list []peer.ID, peer peer.ID) bool {
	for _, p := range list {
		if p == peer {
			return true
		}
	}
	return false
}

func containsCid(list []*cid.Cid, ci *cid.Cid) bool {
	for _, c := range list {
		if c.String() == ci.String() {
			return true
		}
	}
	return false
}

func minInt(x, y int) int {
	if x < y {
		return x
	}
	return y
}

// updatePinParents modifies the api.Pin input to give it the correct parents
// so that previous additions to the pins parents are maintained after this
// pin is committed to consensus.  If this pin carries new parents they are
// merged with those already existing for this CID.
func updatePinParents(pin *api.Pin, existing api.Pin) {
	// no existing parents this pin is up to date
	if existing.Parents == nil || len(existing.Parents.Keys()) == 0 {
		return
	}
	for _, c := range existing.Parents.Keys() {
		pin.Parents.Add(c)
	}
}
