package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/ipfs/ipfs-cluster/api"

	peer "github.com/libp2p/go-libp2p-core/peer"
)

func verifyOutput(t *testing.T, outStr string, trueStr string) {
	outLines := strings.Split(outStr, "\n")
	trueLines := strings.Split(trueStr, "\n")
	if len(outLines) != len(trueLines) {
		fmt.Printf("expected:\n-%s-\n\n\nactual:\n-%s-", trueStr, outStr)
		t.Fatal("Number of output lines does not match expectation")
	}
	for i := range outLines {
		if outLines[i] != trueLines[i] {
			t.Errorf("Difference in sorted outputs (%d): %s vs %s", i, outLines[i], trueLines[i])
		}
	}
}

var simpleIpfs = `digraph cluster {
/* The nodes of the connectivity graph */
/* The cluster-service peers */
subgraph  {
rank="min"
C0 [label=< <B>  </B> <BR/> <B> Qm*eqhEhD </B> > group="QmUBuxVHoNNjfmNpTad36UeaFQv3gXAtCv9r6KhmeqhEhD" color="orange" style="filled" colorscheme="x11" fontcolor="black" fontname="Arial" shape="box3d" peripheries="2" ]
C1 [label=< <B>  </B> <BR/> <B> Qm*cgHDQJ </B> > group="QmV35LjbEGPfN7KfMAJp43VV2enwXqqQf5esx4vUcgHDQJ" color="darkorange3" style="filled" colorscheme="x11" fontcolor="black" fontname="Arial" shape="box3d" ]
C2 [label=< <B>  </B> <BR/> <B> Qm*6MQmJu </B> > group="QmZ2ckU7G35MYyJgMTwMUnicsGqSy3YUxGBX7qny6MQmJu" color="darkorange3" style="filled" colorscheme="x11" fontcolor="black" fontname="Arial" shape="box3d" ]
}

/* The ipfs peers */
subgraph  {
rank="max"
I0 [label=< <B> IPFS </B> <BR/> <B> Qm*N5LSsq </B> > group="QmPFKAGZbUjdzt8BBx8VTWCe9UeUQVcoqHFehSPzN5LSsq" color="turquoise3" style="filled" colorscheme="x11" fontcolor="black" fontname="Arial" shape="cylinder" ]
I1 [label=< <B> IPFS </B> <BR/> <B> Qm*R3DZDV </B> > group="QmXbiVZd93SLiu9TAm21F2y9JwGiFLydbEVkPBaMR3DZDV" color="turquoise3" style="filled" colorscheme="x11" fontcolor="black" fontname="Arial" shape="cylinder" ]
I2 [label=< <B> IPFS </B> <BR/> <B> Qm*wbBsuL </B> > group="QmbU7273zH6jxwDe2nqRmEm2rp5PpqP2xeQr2xCmwbBsuL" color="turquoise3" style="filled" colorscheme="x11" fontcolor="black" fontname="Arial" shape="cylinder" ]
}

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
I2 -> I1
I2 -> I0
}`

var (
	pid1, _ = peer.Decode("QmUBuxVHoNNjfmNpTad36UeaFQv3gXAtCv9r6KhmeqhEhD")
	pid2, _ = peer.Decode("QmV35LjbEGPfN7KfMAJp43VV2enwXqqQf5esx4vUcgHDQJ")
	pid3, _ = peer.Decode("QmZ2ckU7G35MYyJgMTwMUnicsGqSy3YUxGBX7qny6MQmJu")
	pid4, _ = peer.Decode("QmXbiVZd93SLiu9TAm21F2y9JwGiFLydbEVkPBaMR3DZDV")
	pid5, _ = peer.Decode("QmPFKAGZbUjdzt8BBx8VTWCe9UeUQVcoqHFehSPzN5LSsq")
	pid6, _ = peer.Decode("QmbU7273zH6jxwDe2nqRmEm2rp5PpqP2xeQr2xCmwbBsuL")

	pid7, _ = peer.Decode("QmQsdAdCHs4PRLi5tcoLfasYppryqQENxgAy4b2aS8xccb")
	pid8, _ = peer.Decode("QmVV2enwXqqQf5esx4v36UeaFQvFehSPzNfi8aaaaaanM8")
	pid9, _ = peer.Decode("QmfCHNQ2vbUmAuJZhE2hEpgiJq4sL1XScWEKnUrVtWZdeD")
)

func TestSimpleIpfsGraphs(t *testing.T) {
	cg := api.ConnectGraph{
		ClusterID: pid1,
		ClusterLinks: map[string][]peer.ID{
			peer.Encode(pid1): {
				pid2,
				pid3,
			},
			peer.Encode(pid2): {
				pid1,
				pid3,
			},
			peer.Encode(pid3): {
				pid1,
				pid2,
			},
		},
		IPFSLinks: map[string][]peer.ID{
			peer.Encode(pid4): {
				pid5,
				pid6,
			},
			peer.Encode(pid5): {
				pid4,
				pid6,
			},
			peer.Encode(pid6): {
				pid4,
				pid5,
			},
		},
		ClustertoIPFS: map[string]peer.ID{
			peer.Encode(pid1): pid4,
			peer.Encode(pid2): pid5,
			peer.Encode(pid3): pid6,
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
subgraph  {
rank="min"
C0 [label=< <B>  </B> <BR/> <B> Qm*eqhEhD </B> > group="QmUBuxVHoNNjfmNpTad36UeaFQv3gXAtCv9r6KhmeqhEhD" color="orange" style="filled" colorscheme="x11" fontcolor="black" fontname="Arial" shape="box3d" peripheries="2" ]
C1 [label=< <B>  </B> <BR/> <B> Qm*cgHDQJ </B> > group="QmV35LjbEGPfN7KfMAJp43VV2enwXqqQf5esx4vUcgHDQJ" color="darkorange3" style="filled" colorscheme="x11" fontcolor="black" fontname="Arial" shape="box3d" ]
C2 [label=< <B>  </B> <BR/> <B> Qm*6MQmJu </B> > group="QmZ2ckU7G35MYyJgMTwMUnicsGqSy3YUxGBX7qny6MQmJu" color="darkorange3" style="filled" colorscheme="x11" fontcolor="black" fontname="Arial" shape="box3d" ]
}

/* The ipfs peers */
subgraph  {
rank="max"
I0 [label=< <B> IPFS </B> <BR/> <B> Qm*N5LSsq </B> > group="QmPFKAGZbUjdzt8BBx8VTWCe9UeUQVcoqHFehSPzN5LSsq" color="turquoise3" style="filled" colorscheme="x11" fontcolor="black" fontname="Arial" shape="cylinder" ]
I1 [label=< <B> IPFS </B> <BR/> <B> Qm*S8xccb </B> > group="QmQsdAdCHs4PRLi5tcoLfasYppryqQENxgAy4b2aS8xccb" color="turquoise3" style="filled" colorscheme="x11" fontcolor="black" fontname="Arial" shape="cylinder" ]
I2 [label=< <B> IPFS </B> <BR/> <B> Qm*aaanM8 </B> > group="QmVV2enwXqqQf5esx4v36UeaFQvFehSPzNfi8aaaaaanM8" color="turquoise3" style="filled" colorscheme="x11" fontcolor="black" fontname="Arial" shape="cylinder" ]
I3 [label=< <B> IPFS </B> <BR/> <B> Qm*R3DZDV </B> > group="QmXbiVZd93SLiu9TAm21F2y9JwGiFLydbEVkPBaMR3DZDV" color="turquoise3" style="filled" colorscheme="x11" fontcolor="black" fontname="Arial" shape="cylinder" ]
I4 [label=< <B> IPFS </B> <BR/> <B> Qm*wbBsuL </B> > group="QmbU7273zH6jxwDe2nqRmEm2rp5PpqP2xeQr2xCmwbBsuL" color="turquoise3" style="filled" colorscheme="x11" fontcolor="black" fontname="Arial" shape="cylinder" ]
I5 [label=< <B> IPFS </B> <BR/> <B> Qm*tWZdeD </B> > group="QmfCHNQ2vbUmAuJZhE2hEpgiJq4sL1XScWEKnUrVtWZdeD" color="turquoise3" style="filled" colorscheme="x11" fontcolor="black" fontname="Arial" shape="cylinder" ]
}

/* Edges representing active connections in the cluster */
/* The connections among cluster-service peers */
C0 -> C1
C0 -> C2
C1 -> C0
C1 -> C2
C2 -> C0
C2 -> C1

/* The connections between cluster peers and their ipfs daemons */
C0 -> I3
C1 -> I0
C2 -> I4

/* The swarm peer connections among ipfs daemons in the cluster */
I0 -> I3
I0 -> I4
I0 -> I1
I0 -> I2
I0 -> I5
I3 -> I0
I3 -> I4
I3 -> I1
I3 -> I2
I3 -> I5
I4 -> I3
I4 -> I0
I4 -> I1
I4 -> I2
I4 -> I5
}`

func TestIpfsAllGraphs(t *testing.T) {
	cg := api.ConnectGraph{
		ClusterID: pid1,
		ClusterLinks: map[string][]peer.ID{
			peer.Encode(pid1): {
				pid2,
				pid3,
			},
			peer.Encode(pid2): {
				pid1,
				pid3,
			},
			peer.Encode(pid3): {
				pid1,
				pid2,
			},
		},
		IPFSLinks: map[string][]peer.ID{
			peer.Encode(pid4): {
				pid5,
				pid6,
				pid7,
				pid8,
				pid9,
			},
			peer.Encode(pid5): {
				pid4,
				pid6,
				pid7,
				pid8,
				pid9,
			},
			peer.Encode(pid6): {
				pid4,
				pid5,
				pid7,
				pid8,
				pid9,
			},
		},
		ClustertoIPFS: map[string]peer.ID{
			peer.Encode(pid1): pid4,
			peer.Encode(pid2): pid5,
			peer.Encode(pid3): pid6,
		},
	}

	buf := new(bytes.Buffer)
	err := makeDot(&cg, buf, true)
	if err != nil {
		t.Fatal(err)
	}
	verifyOutput(t, buf.String(), allIpfs)
}
