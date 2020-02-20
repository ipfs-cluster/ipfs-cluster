package main

import (
	"testing"

	"github.com/ipfs/ipfs-cluster/cmdutils"

	ma "github.com/multiformats/go-multiaddr"
)

func TestRandomPorts(t *testing.T) {
	port := "9096"
	m1, _ := ma.NewMultiaddr("/ip4/0.0.0.0/tcp/9096")
	m2, _ := ma.NewMultiaddr("/ip6/::/udp/9096")

	addresses, err := cmdutils.RandomizePorts([]ma.Multiaddr{m1, m2})
	if err != nil {
		t.Fatal(err)
	}

	v1, err := addresses[0].ValueForProtocol(ma.P_TCP)
	if err != nil {
		t.Fatal(err)
	}

	v2, err := addresses[1].ValueForProtocol(ma.P_UDP)
	if err != nil {
		t.Fatal(err)
	}

	if v1 == port {
		t.Error("expected different ipv4 ports")
	}

	if v2 == port {
		t.Error("expected different ipv6 ports")
	}
}
