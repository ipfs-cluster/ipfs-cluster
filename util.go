package ipfscluster

import (
	"errors"
	"fmt"

	cid "github.com/ipfs/go-cid"
	peer "github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

// PeersFromMultiaddrs returns all the different peers in the given addresses.
// each peer only will appear once in the result, even if several
// multiaddresses for it are provided.
func PeersFromMultiaddrs(addrs []ma.Multiaddr) []peer.ID {
	var pids []peer.ID
	pm := make(map[peer.ID]struct{})
	for _, addr := range addrs {
		pinfo, err := peer.AddrInfoFromP2pAddr(addr)
		if err != nil {
			continue
		}
		_, ok := pm[pinfo.ID]
		if !ok {
			pm[pinfo.ID] = struct{}{}
			pids = append(pids, pinfo.ID)
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

func containsCid(list []cid.Cid, ci cid.Cid) bool {
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

func checkMatchingLengths(l ...int) bool {
	if len(l) <= 1 {
		return true
	}

	for i := 1; i < len(l); i++ {
		if l[i-1] != l[i] {
			return false
		}
	}
	return true
}

// // updatePinParents modifies the api.Pin input to give it the correct parents
// // so that previous additions to the pins parents are maintained after this
// // pin is committed to consensus.  If this pin carries new parents they are
// // merged with those already existing for this CID.
// func updatePinParents(pin *api.Pin, existing *api.Pin) {
// 	// no existing parents this pin is up to date
// 	if existing.Parents == nil || len(existing.Parents.Keys()) == 0 {
// 		return
// 	}
// 	for _, c := range existing.Parents.Keys() {
// 		pin.Parents.Add(c)
// 	}
// }
