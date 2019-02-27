package main

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"testing"

	peer "github.com/libp2p/go-libp2p-peer"

	"github.com/ipfs/ipfs-cluster/api"
)

func verifyOutput(t *testing.T, outStr string, trueStr string) {
	// Because values are printed in the order of map keys we cannot enforce
	// an exact ordering.  Instead we split both strings and compare line by
	// line.
	outLines := strings.Split(outStr, "\n")
	trueLines := strings.Split(trueStr, "\n")
	sort.Strings(outLines)
	sort.Strings(trueLines)
	if len(outLines) != len(trueLines) {
		fmt.Printf("expected: %s\n actual: %s", trueStr, outStr)
		t.Fatal("Number of output lines does not match expectation")
	}
	for i := range outLines {
		if outLines[i] != trueLines[i] {
			t.Errorf("Difference in sorted outputs: %s vs %s", outLines[i], trueLines[i])
		}
	}
}

var simpleIpfs = `digraph cluster {

/* The nodes of the connectivity graph */
/* The cluster-service peers */
C0 [label="<peer.ID Qm*eqhEhD>" color="blue2"]
C1 [label="<peer.ID Qm*cgHDQJ>" color="blue2"]
C2 [label="<peer.ID Qm*6MQmJu>" color="blue2"]

/* The ipfs peers */
I0 [label="<peer.ID Qm*N5LSsq>" color="goldenrod"]
I1 [label="<peer.ID Qm*R3DZDV>" color="goldenrod"]
I2 [label="<peer.ID Qm*wbBsuL>" color="goldenrod"]

/* Edges representing active connections in the cluster */
/* The connections among cluster-service peers */
C0 -> C1
C0 -> C2
C1 -> C0
C1 -> C2
C2 -> C0
C2 -> C1

/* The connections between cluster peers and their ipfs daemons */
C0 -> I1
C1 -> I0
C2 -> I2

/* The swarm peer connections among ipfs daemons in the cluster */
I0 -> I1
I0 -> I2
I1 -> I0
I1 -> I2
I2 -> I0
I2 -> I1

 }`

var (
	pid1, _ = peer.IDB58Decode("QmUBuxVHoNNjfmNpTad36UeaFQv3gXAtCv9r6KhmeqhEhD")
	pid2, _ = peer.IDB58Decode("QmV35LjbEGPfN7KfMAJp43VV2enwXqqQf5esx4vUcgHDQJ")
	pid3, _ = peer.IDB58Decode("QmZ2ckU7G35MYyJgMTwMUnicsGqSy3YUxGBX7qny6MQmJu")
	pid4, _ = peer.IDB58Decode("QmXbiVZd93SLiu9TAm21F2y9JwGiFLydbEVkPBaMR3DZDV")
	pid5, _ = peer.IDB58Decode("QmPFKAGZbUjdzt8BBx8VTWCe9UeUQVcoqHFehSPzN5LSsq")
	pid6, _ = peer.IDB58Decode("QmbU7273zH6jxwDe2nqRmEm2rp5PpqP2xeQr2xCmwbBsuL")

	pid7, _ = peer.IDB58Decode("QmQsdAdCHs4PRLi5tcoLfasYppryqQENxgAy4b2aS8xccb")
	pid8, _ = peer.IDB58Decode("QmVV2enwXqqQf5esx4v36UeaFQvFehSPzNfi8aaaaaanM8")
	pid9, _ = peer.IDB58Decode("QmfCHNQ2vbUmAuJZhE2hEpgiJq4sL1XScWEKnUrVtWZdeD")
)

func TestSimpleIpfsGraphs(t *testing.T) {
	cg := api.ConnectGraph{
		ClusterID: pid1,
		ClusterLinks: map[string][]peer.ID{
			peer.IDB58Encode(pid1): []peer.ID{
				pid2,
				pid3,
			},
			peer.IDB58Encode(pid2): []peer.ID{
				pid1,
				pid3,
			},
			peer.IDB58Encode(pid3): []peer.ID{
				pid1,
				pid2,
			},
		},
		IPFSLinks: map[string][]peer.ID{
			peer.IDB58Encode(pid4): []peer.ID{
				pid5,
				pid6,
			},
			peer.IDB58Encode(pid5): []peer.ID{
				pid4,
				pid6,
			},
			peer.IDB58Encode(pid6): []peer.ID{
				pid4,
				pid5,
			},
		},
		ClustertoIPFS: map[string]peer.ID{
			peer.IDB58Encode(pid1): pid4,
			peer.IDB58Encode(pid2): pid5,
			peer.IDB58Encode(pid3): pid6,
		},
	}
	buf := new(bytes.Buffer)
	err := makeDot(&cg, buf, false)
	if err != nil {
		t.Fatal(err)
	}
	verifyOutput(t, buf.String(), simpleIpfs)
}

var allIpfs = `digraph cluster {

/* The nodes of the connectivity graph */
/* The cluster-service peers */
C0 [label="<peer.ID Qm*eqhEhD>" color="blue2"]
C1 [label="<peer.ID Qm*cgHDQJ>" color="blue2"]
C2 [label="<peer.ID Qm*6MQmJu>" color="blue2"]

/* The ipfs peers */
I0 [label="<peer.ID Qm*N5LSsq>" color="goldenrod"]
I1 [label="<peer.ID Qm*S8xccb>" color="goldenrod"]
I2 [label="<peer.ID Qm*aaanM8>" color="goldenrod"]
I3 [label="<peer.ID Qm*R3DZDV>" color="goldenrod"]
I4 [label="<peer.ID Qm*wbBsuL>" color="goldenrod"]
I5 [label="<peer.ID Qm*tWZdeD>" color="goldenrod"]

/* Edges representing active connections in the cluster */
/* The connections among cluster-service peers */
C2 -> C0
C2 -> C1
C0 -> C1
C0 -> C2
C1 -> C0
C1 -> C2

/* The connections between cluster peers and their ipfs daemons */
C0 -> I3
C1 -> I0
C2 -> I4

/* The swarm peer connections among ipfs daemons in the cluster */
I0 -> I1
I0 -> I2
I0 -> I3
I0 -> I4
I0 -> I5
I3 -> I0
I3 -> I1
I3 -> I2
I3 -> I4
I3 -> I5
I4 -> I0
I4 -> I1
I4 -> I2
I4 -> I3
I4 -> I5

 }`

func TestIpfsAllGraphs(t *testing.T) {
	cg := api.ConnectGraph{
		ClusterID: pid1,
		ClusterLinks: map[string][]peer.ID{
			peer.IDB58Encode(pid1): []peer.ID{
				pid2,
				pid3,
			},
			peer.IDB58Encode(pid2): []peer.ID{
				pid1,
				pid3,
			},
			peer.IDB58Encode(pid3): []peer.ID{
				pid1,
				pid2,
			},
		},
		IPFSLinks: map[string][]peer.ID{
			peer.IDB58Encode(pid4): []peer.ID{
				pid5,
				pid6,
				pid7,
				pid8,
				pid9,
			},
			peer.IDB58Encode(pid5): []peer.ID{
				pid4,
				pid6,
				pid7,
				pid8,
				pid9,
			},
			peer.IDB58Encode(pid6): []peer.ID{
				pid4,
				pid5,
				pid7,
				pid8,
				pid9,
			},
		},
		ClustertoIPFS: map[string]peer.ID{
			peer.IDB58Encode(pid1): pid4,
			peer.IDB58Encode(pid2): pid5,
			peer.IDB58Encode(pid3): pid6,
		},
	}

	buf := new(bytes.Buffer)
	err := makeDot(&cg, buf, true)
	if err != nil {
		t.Fatal(err)
	}
	verifyOutput(t, buf.String(), allIpfs)
}
