package ipfscluster

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"

	host "github.com/libp2p/go-libp2p-host"
	peerstore "github.com/libp2p/go-libp2p-peerstore"

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
		err = fmt.Errorf("Invalid peer ID in multiaddress: %s: %s", pid, err)
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

// openConns is a workaround for
// https://github.com/libp2p/go-libp2p-swarm/issues/15
// which break our tests.
// It should open connections for peers where they haven't
// yet been opened. By randomly sleeping we reduce the
// chance that peers will open 2 connections simultaneously when
// being started at the same time.
func openConns(ctx context.Context, h host.Host) {
	rand.Seed(time.Now().UnixNano())
	time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond)
	peers := h.Peerstore().Peers()
	for _, p := range peers {
		peerInfo := h.Peerstore().PeerInfo(p)
		if p == h.ID() {
			continue // do not connect to ourselves
		}
		// ignore any errors here
		h.Connect(ctx, peerInfo)
	}
}

// connect to a peer ID.
func connectToPeer(ctx context.Context, h host.Host, id peer.ID, addr ma.Multiaddr) error {
	err := h.Connect(ctx, peerstore.PeerInfo{
		ID:    id,
		Addrs: []ma.Multiaddr{addr},
	})
	return err
}

// return the local multiaddresses used to communicate to a peer.
func localMultiaddrsTo(h host.Host, pid peer.ID) []ma.Multiaddr {
	var addrs []ma.Multiaddr
	conns := h.Network().ConnsToPeer(pid)
	logger.Debugf("conns to %s are: %s", pid, conns)
	for _, conn := range conns {
		addrs = append(addrs, multiaddrJoin(conn.LocalMultiaddr(), h.ID()))
	}
	return addrs
}

func getLocalMultiaddrTo(ctx context.Context, h host.Host, pid peer.ID, addr ma.Multiaddr) (ma.Multiaddr, error) {
	// We need to force a connection from us
	h.Network().ClosePeer(pid)
	err := connectToPeer(ctx, h, pid, addr)
	if err != nil {
		return nil, err
	}
	lAddrs := localMultiaddrsTo(h, pid)
	if len(lAddrs) == 0 {
		return nil, errors.New("No connections to peer exist")
	}
	return lAddrs[0], nil
}
