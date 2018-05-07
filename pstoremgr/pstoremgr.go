// Package pstoremgr provides a Manager that simplifies handling
// addition, listing and removal of cluster peer multiaddresses from
// the libp2p Host. This includes resolving DNS addresses, decapsulating
// and encapsulating the /p2p/ (/ipfs/) protocol as needed, listing, saving
// and loading addresses.
package pstoremgr

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/ipfs/ipfs-cluster/api"

	logging "github.com/ipfs/go-log"
	host "github.com/libp2p/go-libp2p-host"
	peer "github.com/libp2p/go-libp2p-peer"
	peerstore "github.com/libp2p/go-libp2p-peerstore"
	ma "github.com/multiformats/go-multiaddr"
	madns "github.com/multiformats/go-multiaddr-dns"
)

var logger = logging.Logger("pstoremgr")

// Timeouts for network operations triggered by the Manager
var (
	DNSTimeout     = 2 * time.Second
	ConnectTimeout = 10 * time.Second
)

// Manager provides utilities for handling cluster peer addresses
// and storing them in a libp2p Host peerstore.
type Manager struct {
	ctx           context.Context
	host          host.Host
	peerstoreLock sync.Mutex
	peerstorePath string
}

// New creates a Manager with the given libp2p Host and peerstorePath.
// The path indicates the place to persist and read peer addresses from.
// If empty, these operations (LoadPeerstore, SavePeerstore) will no-op.
func New(h host.Host, peerstorePath string) *Manager {
	return &Manager{
		ctx:           context.Background(),
		host:          h,
		peerstorePath: peerstorePath,
	}
}

// ImportPeer adds a new peer address to the host's peerstore, optionally
// dialing to it. It will resolve any DNS multiaddresses before adding them.
// The address is expected to include the /ipfs/<peerID> protocol part.
func (pm *Manager) ImportPeer(addr ma.Multiaddr, connect bool) error {
	if pm.host == nil {
		return nil
	}

	logger.Debugf("adding peer address %s", addr)
	pid, decapAddr, err := api.Libp2pMultiaddrSplit(addr)
	if err != nil {
		return err
	}
	pm.host.Peerstore().AddAddr(pid, decapAddr, peerstore.PermanentAddrTTL)

	// dns multiaddresses need to be resolved because libp2p only does that
	// on explicit bhost.Connect().
	if madns.Matches(addr) {
		ctx, cancel := context.WithTimeout(pm.ctx, DNSTimeout)
		defer cancel()
		resolvedAddrs, err := madns.Resolve(ctx, addr)
		if err != nil {
			logger.Error(err)
			return err
		}
		pm.ImportPeers(resolvedAddrs, connect)
	}
	if connect {
		ctx, cancel := context.WithTimeout(pm.ctx, ConnectTimeout)
		defer cancel()
		pm.host.Network().DialPeer(ctx, pid)
	}
	return nil
}

// RmPeer clear all addresses for a given peer ID from the host's peerstore.
func (pm *Manager) RmPeer(pid peer.ID) error {
	if pm.host == nil {
		return nil
	}

	logger.Debugf("forgetting peer %s", pid.Pretty())
	pm.host.Peerstore().ClearAddrs(pid)
	return nil
}

// if the peer has dns addresses, return only those, otherwise
// return all. In all cases, encapsulate the peer ID.
func (pm *Manager) filteredPeerAddrs(p peer.ID) []ma.Multiaddr {
	all := pm.host.Peerstore().Addrs(p)
	peerAddrs := []ma.Multiaddr{}
	peerDNSAddrs := []ma.Multiaddr{}
	peerPart, _ := ma.NewMultiaddr(fmt.Sprintf("/ipfs/%s", peer.IDB58Encode(p)))

	for _, a := range all {
		encAddr := a.Encapsulate(peerPart)
		if madns.Matches(encAddr) {
			peerDNSAddrs = append(peerDNSAddrs, encAddr)
		} else {
			peerAddrs = append(peerAddrs, encAddr)
		}
	}

	if len(peerDNSAddrs) > 0 {
		return peerDNSAddrs
	}

	return peerAddrs
}

// PeersAddresses returns the list of multiaddresses (encapsulating the
// /ipfs/<peerID> part) for the given set of peers. For peers for which
// we know DNS multiaddresses, we only return those. Otherwise, we return
// all the multiaddresses known for that peer.
func (pm *Manager) PeersAddresses(peers []peer.ID) []ma.Multiaddr {
	if pm.host == nil {
		return nil
	}

	if peers == nil {
		return nil
	}

	var addrs []ma.Multiaddr
	for _, p := range peers {
		if p == pm.host.ID() {
			continue
		}
		addrs = append(addrs, pm.filteredPeerAddrs(p)...)
	}
	return addrs
}

// ImportPeers calls ImportPeer for every address in the given slice, using the
// given connect parameter.
func (pm *Manager) ImportPeers(addrs []ma.Multiaddr, connect bool) error {
	for _, a := range addrs {
		pm.ImportPeer(a, connect)
	}
	return nil
}

// ImportPeersFromPeerstore reads the peerstore file and calls ImportPeers with
// the addresses obtained from it.
func (pm *Manager) ImportPeersFromPeerstore(connect bool) error {
	return pm.ImportPeers(pm.LoadPeerstore(), connect)
}

// LoadPeerstore parses the peerstore file and returns the list
// of addresses read from it.
func (pm *Manager) LoadPeerstore() (addrs []ma.Multiaddr) {
	if pm.peerstorePath == "" {
		return
	}
	pm.peerstoreLock.Lock()
	defer pm.peerstoreLock.Unlock()

	f, err := os.Open(pm.peerstorePath)
	if err != nil {
		return // nothing to load
	}

	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		addrStr := scanner.Text()
		if addrStr[0] != '/' {
			// skip anything that is not going to be a multiaddress
			continue
		}
		addr, err := ma.NewMultiaddr(addrStr)
		if err != nil {
			logger.Error(
				"error parsing multiaddress from %s: %s",
				pm.peerstorePath,
				err,
			)
		}
		addrs = append(addrs, addr)
	}
	if err := scanner.Err(); err != nil {
		logger.Errorf("reading %s: %s", pm.peerstorePath, err)
	}
	return addrs
}

// SavePeerstore stores a slice of multiaddresses in the peerstore file, one
// per line.
func (pm *Manager) SavePeerstore(addrs []ma.Multiaddr) {
	if pm.peerstorePath == "" {
		return
	}

	pm.peerstoreLock.Lock()
	defer pm.peerstoreLock.Unlock()

	f, err := os.Create(pm.peerstorePath)
	if err != nil {
		logger.Errorf(
			"could not save peer addresses to %s: %s",
			pm.peerstorePath,
			err,
		)
		return
	}
	defer f.Close()

	for _, a := range addrs {
		f.Write([]byte(fmt.Sprintf("%s\n", a.String())))
	}
}

// SavePeerstoreForPeers calls PeersAddresses and then saves the peerstore
// file using the result.
func (pm *Manager) SavePeerstoreForPeers(peers []peer.ID) {
	pm.SavePeerstore(pm.PeersAddresses(peers))
}
