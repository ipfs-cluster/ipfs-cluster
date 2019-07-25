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
	"sort"
	"sync"
	"time"

	logging "github.com/ipfs/go-log"
	host "github.com/libp2p/go-libp2p-core/host"
	net "github.com/libp2p/go-libp2p-core/network"
	peer "github.com/libp2p/go-libp2p-core/peer"
	peerstore "github.com/libp2p/go-libp2p-core/peerstore"
	pstoreutil "github.com/libp2p/go-libp2p-peerstore"
	ma "github.com/multiformats/go-multiaddr"
	madns "github.com/multiformats/go-multiaddr-dns"
)

var logger = logging.Logger("pstoremgr")

// PriorityTag is used to attach metadata to peers in the peerstore
// so they can be sorted.
var PriorityTag = "cluster"

// Timeouts for network operations triggered by the Manager.
var (
	DNSTimeout     = 5 * time.Second
	ConnectTimeout = 5 * time.Second
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
func New(ctx context.Context, h host.Host, peerstorePath string) *Manager {
	return &Manager{
		ctx:           ctx,
		host:          h,
		peerstorePath: peerstorePath,
	}
}

// ImportPeer adds a new peer address to the host's peerstore, optionally
// dialing to it. The address is expected to include the /ipfs/<peerID>
// protocol part or to be a /dnsaddr/multiaddress
// Peers are added with the given ttl.
func (pm *Manager) ImportPeer(addr ma.Multiaddr, connect bool, ttl time.Duration) (peer.ID, error) {
	if pm.host == nil {
		return "", nil
	}

	protos := addr.Protocols()
	if len(protos) > 0 && protos[0].Code == madns.DnsaddrProtocol.Code {
		// We need to pre-resolve this
		logger.Debugf("resolving %s", addr)
		ctx, cancel := context.WithTimeout(pm.ctx, DNSTimeout)
		defer cancel()

		resolvedAddrs, err := madns.Resolve(ctx, addr)
		if err != nil {
			return "", err
		}
		if len(resolvedAddrs) == 0 {
			return "", fmt.Errorf("%s: no resolved addresses", addr)
		}
		var pid peer.ID
		for _, add := range resolvedAddrs {
			pid, err = pm.ImportPeer(add, connect, ttl)
			if err != nil {
				return "", err
			}
		}
		return pid, nil // returns the last peer ID
	}

	pinfo, err := peer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		return "", err
	}

	// Do not add ourselves
	if pinfo.ID == pm.host.ID() {
		return pinfo.ID, nil
	}

	logger.Debugf("adding peer address %s", addr)
	pm.host.Peerstore().AddAddrs(pinfo.ID, pinfo.Addrs, ttl)

	if connect {
		go func() {
			ctx, cancel := context.WithTimeout(pm.ctx, ConnectTimeout)
			defer cancel()
			pm.host.Connect(ctx, *pinfo)
		}()
	}
	return pinfo.ID, nil
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
// return all.
func (pm *Manager) filteredPeerAddrs(p peer.ID) []ma.Multiaddr {
	all := pm.host.Peerstore().Addrs(p)
	peerAddrs := []ma.Multiaddr{}
	peerDNSAddrs := []ma.Multiaddr{}

	for _, a := range all {
		if madns.Matches(a) {
			peerDNSAddrs = append(peerDNSAddrs, a)
		} else {
			peerAddrs = append(peerAddrs, a)
		}
	}

	if len(peerDNSAddrs) > 0 {
		return peerDNSAddrs
	}

	return peerAddrs
}

// PeerInfos returns a slice of peerinfos for the given set of peers in order
// of priority. For peers for which we know DNS
// multiaddresses, we only include those. Otherwise, the AddrInfo includes all
// the multiaddresses known for that peer. Peers without addresses are not
// included.
func (pm *Manager) PeerInfos(peers []peer.ID) []peer.AddrInfo {
	if pm.host == nil {
		return nil
	}

	if peers == nil {
		return nil
	}

	var pinfos []peer.AddrInfo
	for _, p := range peers {
		if p == pm.host.ID() {
			continue
		}
		pinfo := peer.AddrInfo{
			ID:    p,
			Addrs: pm.filteredPeerAddrs(p),
		}
		if len(pinfo.Addrs) > 0 {
			pinfos = append(pinfos, pinfo)
		}
	}

	toSort := &peerSort{
		pinfos: pinfos,
		pstore: pm.host.Peerstore(),
	}
	// Sort from highest to lowest priority
	sort.Sort(toSort)

	return toSort.pinfos
}

// ImportPeers calls ImportPeer for every address in the given slice, using the
// given connect parameter. Peers are tagged with priority as given
// by their position in the list.
func (pm *Manager) ImportPeers(addrs []ma.Multiaddr, connect bool, ttl time.Duration) error {
	for i, a := range addrs {
		pid, err := pm.ImportPeer(a, connect, ttl)
		if err == nil {
			pm.SetPriority(pid, i)
		}
	}
	return nil
}

// ImportPeersFromPeerstore reads the peerstore file and calls ImportPeers with
// the addresses obtained from it.
func (pm *Manager) ImportPeersFromPeerstore(connect bool, ttl time.Duration) error {
	return pm.ImportPeers(pm.LoadPeerstore(), connect, ttl)
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
			logger.Errorf(
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
func (pm *Manager) SavePeerstore(pinfos []peer.AddrInfo) {
	if pm.peerstorePath == "" {
		return
	}

	pm.peerstoreLock.Lock()
	defer pm.peerstoreLock.Unlock()

	f, err := os.Create(pm.peerstorePath)
	if err != nil {
		logger.Warningf(
			"could not save peer addresses to %s: %s",
			pm.peerstorePath,
			err,
		)
		return
	}
	defer f.Close()

	for _, pinfo := range pinfos {
		addrs, err := peer.AddrInfoToP2pAddrs(&pinfo)
		if err != nil {
			logger.Warning(err)
			continue
		}
		for _, a := range addrs {
			f.Write([]byte(fmt.Sprintf("%s\n", a.String())))
		}
	}
}

// SavePeerstoreForPeers calls PeerInfos and then saves the peerstore
// file using the result.
func (pm *Manager) SavePeerstoreForPeers(peers []peer.ID) {
	pm.SavePeerstore(pm.PeerInfos(peers))
}

// Bootstrap attempts to get up to "count" connected peers by trying those
// in the peerstore in priority order. It returns the list of peers it managed
// to connect to.
func (pm *Manager) Bootstrap(count int) []peer.ID {
	knownPeers := pm.host.Peerstore().PeersWithAddrs()
	toSort := &peerSort{
		pinfos: pstoreutil.PeerInfos(pm.host.Peerstore(), knownPeers),
		pstore: pm.host.Peerstore(),
	}

	// Sort from highest to lowest priority
	sort.Sort(toSort)

	pinfos := toSort.pinfos
	lenKnown := len(pinfos)
	totalConns := 0
	connectedPeers := []peer.ID{}

	// keep conecting while we have peers in the store
	// and we have not reached count.
	for i := 0; i < lenKnown && totalConns < count; i++ {
		pinfo := pinfos[i]
		ctx, cancel := context.WithTimeout(pm.ctx, ConnectTimeout)
		defer cancel()

		if pm.host.Network().Connectedness(pinfo.ID) == net.Connected {
			// We are connected, assume success and do not try
			// to re-connect
			totalConns++
			continue
		}

		logger.Debugf("connecting to %s", pinfo.ID)
		err := pm.host.Connect(ctx, pinfo)
		if err != nil {
			logger.Debug(err)
			pm.SetPriority(pinfo.ID, 9999)
			continue
		}
		logger.Debugf("connected to %s", pinfo.ID)
		totalConns++
		connectedPeers = append(connectedPeers, pinfo.ID)
	}
	return connectedPeers
}

// SetPriority attaches a priority to a peer. 0 means more priority than
// 1. 1 means more priority than 2 etc.
func (pm *Manager) SetPriority(pid peer.ID, prio int) error {
	return pm.host.Peerstore().Put(pid, PriorityTag, prio)
}

// peerSort is used to sort a slice of PinInfos given the PriorityTag in the
// peerstore, from the lowest tag value (0 is the highest priority) to the
// highest, Peers without a valid priority tag are considered as having a tag
// with value 0, so they will be among the first elements in the resulting
// slice.
type peerSort struct {
	pinfos []peer.AddrInfo
	pstore peerstore.Peerstore
}

func (ps *peerSort) Len() int {
	return len(ps.pinfos)
}

func (ps *peerSort) Less(i, j int) bool {
	pinfo1 := ps.pinfos[i]
	pinfo2 := ps.pinfos[j]

	var prio1, prio2 int

	prio1iface, err := ps.pstore.Get(pinfo1.ID, PriorityTag)
	if err == nil {
		prio1 = prio1iface.(int)
	}
	prio2iface, err := ps.pstore.Get(pinfo2.ID, PriorityTag)
	if err == nil {
		prio2 = prio2iface.(int)
	}
	return prio1 < prio2
}

func (ps *peerSort) Swap(i, j int) {
	pinfo1 := ps.pinfos[i]
	pinfo2 := ps.pinfos[j]
	ps.pinfos[i] = pinfo2
	ps.pinfos[j] = pinfo1
}
