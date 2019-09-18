package main

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/ipfs/ipfs-cluster/api"

	peer "github.com/libp2p/go-libp2p-core/peer"
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
subgraph  {
rank="min"
C0 [label=< <B>  </B> <BR/> <B> Qm*EhD </B> > group="QmUBuxVHoNNjfmNpTad36UeaFQv3gXAtCv9r6KhmeqhEhD" color="11" style="filled" colorscheme="brbg11" fontcolor="6" fontname="Ariel" shape="ellipse" peripheries="2" ]
C1 [label=< <B>  </B> <BR/> <B> Qm*DQJ </B> > group="QmV35LjbEGPfN7KfMAJp43VV2enwXqqQf5esx4vUcgHDQJ" color="9" style="filled" colorscheme="brbg11" fontcolor="6" fontname="Ariel" shape="ellipse" ]
C2 [label=< <B>  </B> <BR/> <B> Qm*mJu </B> > group="QmZ2ckU7G35MYyJgMTwMUnicsGqSy3YUxGBX7qny6MQmJu" color="9" style="filled" colorscheme="brbg11" fontcolor="6" fontname="Ariel" shape="ellipse" ]
}

/* The ipfs peers linked to cluster peers */
subgraph  {
rank="max"
I0 [label=< <B> IPFS </B> <BR/> <B> Qm*ZDV </B> > group="QmUBuxVHoNNjfmNpTad36UeaFQv3gXAtCv9r6KhmeqhEhD" color="1" style="filled" colorscheme="brbg11" fontcolor="6" fontname="Ariel" shape="box" ]
I1 [label=< <B> IPFS </B> <BR/> <B> Qm*Ssq </B> > group="QmV35LjbEGPfN7KfMAJp43VV2enwXqqQf5esx4vUcgHDQJ" color="1" style="filled" colorscheme="brbg11" fontcolor="6" fontname="Ariel" shape="box" ]
I2 [label=< <B> IPFS </B> <BR/> <B> Qm*suL </B> > group="QmZ2ckU7G35MYyJgMTwMUnicsGqSy3YUxGBX7qny6MQmJu" color="1" style="filled" colorscheme="brbg11" fontcolor="6" fontname="Ariel" shape="box" ]
}

/* The ipfs swarm peers */

/* Edges representing active connections in the cluster */
/* The connections among cluster-service peers */
C0 -> C1
C0 -> C2
C1 -> C0
C1 -> C2
C2 -> C0
C2 -> C1

/* The connections between cluster peers and their ipfs daemons */
C0 -> I0
C1 -> I1
C2 -> I2

/* The swarm peer connections among ipfs daemons in the cluster */
I1 -> I0
I1 -> I2
I0 -> I1
I0 -> I2
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
subgraph  {
rank="min"
C0 [label=< <B>  </B> <BR/> <B> Qm*EhD </B> > group="QmUBuxVHoNNjfmNpTad36UeaFQv3gXAtCv9r6KhmeqhEhD" color="11" style="filled" colorscheme="brbg11" fontcolor="6" fontname="Ariel" shape="ellipse" peripheries="2" ]
C1 [label=< <B>  </B> <BR/> <B> Qm*DQJ </B> > group="QmV35LjbEGPfN7KfMAJp43VV2enwXqqQf5esx4vUcgHDQJ" color="9" style="filled" colorscheme="brbg11" fontcolor="6" fontname="Ariel" shape="ellipse" ]
C2 [label=< <B>  </B> <BR/> <B> Qm*mJu </B> > group="QmZ2ckU7G35MYyJgMTwMUnicsGqSy3YUxGBX7qny6MQmJu" color="9" style="filled" colorscheme="brbg11" fontcolor="6" fontname="Ariel" shape="ellipse" ]
}

/* The ipfs peers linked to cluster peers */
subgraph  {
rank="max"
I0 [label=< <B> IPFS </B> <BR/> <B> Qm*ZDV </B> > group="QmUBuxVHoNNjfmNpTad36UeaFQv3gXAtCv9r6KhmeqhEhD" color="1" style="filled" colorscheme="brbg11" fontcolor="6" fontname="Ariel" shape="box" ]
I1 [label=< <B> IPFS </B> <BR/> <B> Qm*Ssq </B> > group="QmV35LjbEGPfN7KfMAJp43VV2enwXqqQf5esx4vUcgHDQJ" color="1" style="filled" colorscheme="brbg11" fontcolor="6" fontname="Ariel" shape="box" ]
I2 [label=< <B> IPFS </B> <BR/> <B> Qm*suL </B> > group="QmZ2ckU7G35MYyJgMTwMUnicsGqSy3YUxGBX7qny6MQmJu" color="1" style="filled" colorscheme="brbg11" fontcolor="6" fontname="Ariel" shape="box" ]
}

/* The ipfs swarm peers */
I3 [label=< <B> IPFS </B> <BR/> <B> Qm*ccb </B> > group="QmQsdAdCHs4PRLi5tcoLfasYppryqQENxgAy4b2aS8xccb" color="5" style="filled" colorscheme="brbg11" fontcolor="1" fontname="Ariel" shape="box" ]
I4 [label=< <B> IPFS </B> <BR/> <B> Qm*nM8 </B> > group="QmVV2enwXqqQf5esx4v36UeaFQvFehSPzNfi8aaaaaanM8" color="5" style="filled" colorscheme="brbg11" fontcolor="1" fontname="Ariel" shape="box" ]
I5 [label=< <B> IPFS </B> <BR/> <B> Qm*deD </B> > group="QmfCHNQ2vbUmAuJZhE2hEpgiJq4sL1XScWEKnUrVtWZdeD" color="5" style="filled" colorscheme="brbg11" fontcolor="1" fontname="Ariel" shape="box" ]

/* Edges representing active connections in the cluster */
/* The connections among cluster-service peers */
C2 -> C0
C2 -> C1
C0 -> C1
C0 -> C2
C1 -> C0
C1 -> C2

/* The connections between cluster peers and their ipfs daemons */
C0 -> I0
C1 -> I1
C2 -> I2

/* The swarm peer connections among ipfs daemons in the cluster */
I1 -> I0
I1 -> I2
I1 -> I3
I1 -> I4
I1 -> I5
I0 -> I1
I0 -> I2
I0 -> I3
I0 -> I4
I0 -> I5
I2 -> I0
I2 -> I1
I2 -> I3
I2 -> I4
I2 -> I5
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
