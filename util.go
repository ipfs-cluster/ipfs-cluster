package ipfscluster

import (
	"bytes"
	"errors"
	"fmt"

	blake2b "golang.org/x/crypto/blake2b"

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

type distance [blake2b.Size256]byte

type distanceChecker struct {
	local      peer.ID
	otherPeers []peer.ID
	cache      map[peer.ID]distance
}

func (dc distanceChecker) isClosest(ci cid.Cid) bool {
	ciHash := convertKey(ci.KeyString())
	localPeerHash := dc.convertPeerID(dc.local)
	myDistance := xor(ciHash, localPeerHash)

	for _, p := range dc.otherPeers {
		peerHash := dc.convertPeerID(p)
		distance := xor(peerHash, ciHash)

		// if myDistance is larger than for other peers...
		if bytes.Compare(myDistance[:], distance[:]) > 0 {
			return false
		}
	}
	return true
}

// convertPeerID hashes a Peer ID (Multihash).
func (dc distanceChecker) convertPeerID(id peer.ID) distance {
	hash, ok := dc.cache[id]
	if ok {
		return hash
	}

	hashBytes := convertKey(string(id))
	dc.cache[id] = hashBytes
	return hashBytes
}

// convertKey hashes a key.
func convertKey(id string) distance {
	return blake2b.Sum256([]byte(id))
}

func xor(a, b distance) distance {
	var c distance
	for i := 0; i < len(c); i++ {
		c[i] = a[i] ^ b[i]
	}
	return c
}

// peersSubtract subtracts peers ID slice b from peers ID slice a.
func peersSubtract(a []peer.ID, b []peer.ID) []peer.ID {
	var result []peer.ID
	bMap := make(map[peer.ID]struct{}, len(b))

	for _, p := range b {
		bMap[p] = struct{}{}
	}

	for _, p := range a {
		_, ok := bMap[p]
		if ok {
			continue
		}
		result = append(result, p)
	}

	return result
}

func toMultiAddrs(addrs []string) ([]ma.Multiaddr, error) {
	var mAddrs []ma.Multiaddr
	for _, addr := range addrs {
		mAddr, err := ma.NewMultiaddr(addr)
		if err != nil {
			//err = fmt.Errorf("error parsing a listen_multiaddress: %s", err)
			return nil, err
		}
		mAddrs = append(mAddrs, mAddr)
	}

	return mAddrs, nil
}

func multiAddrstoStrings(mAddrs []ma.Multiaddr) []string {
	var addrs []string
	for _, addr := range mAddrs {
		addrs = append(addrs, addr.String())
	}

	return addrs
}
