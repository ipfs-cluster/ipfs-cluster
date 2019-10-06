package main

import (
	"testing"

	ma "github.com/multiformats/go-multiaddr"
)

func TestRandomPorts(t *testing.T) {
	m1, _ := ma.NewMultiaddr("/ip4/0.0.0.0/tcp/9096")
	m2, _ := ma.NewMultiaddr("/ip4/0.0.0.0/tcp/9096")

	assignRandomPorts([]*ma.Multiaddr{&m1})

	v1, err := m1.ValueForProtocol(ma.P_TCP)
	if err != nil {
		t.Fatal(err)
	}

	v2, err := m2.ValueForProtocol(ma.P_TCP)
	if err != nil {
		t.Fatal(err)
	}

	if v1 == v2 {
		t.Error("expected different ports")
	}
}
