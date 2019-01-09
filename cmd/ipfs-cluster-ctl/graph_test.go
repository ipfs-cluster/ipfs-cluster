package main

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"testing"

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

func TestSimpleIpfsGraphs(t *testing.T) {
	cg := api.ConnectGraphSerial{
		ClusterID: "QmUBuxVHoNNjfmNpTad36UeaFQv3gXAtCv9r6KhmeqhEhD",
		ClusterLinks: map[string][]string{
			"QmUBuxVHoNNjfmNpTad36UeaFQv3gXAtCv9r6KhmeqhEhD": []string{
				"QmV35LjbEGPfN7KfMAJp43VV2enwXqqQf5esx4vUcgHDQJ",
				"QmZ2ckU7G35MYyJgMTwMUnicsGqSy3YUxGBX7qny6MQmJu",
			},
			"QmV35LjbEGPfN7KfMAJp43VV2enwXqqQf5esx4vUcgHDQJ": []string{
				"QmUBuxVHoNNjfmNpTad36UeaFQv3gXAtCv9r6KhmeqhEhD",
				"QmZ2ckU7G35MYyJgMTwMUnicsGqSy3YUxGBX7qny6MQmJu",
			},
			"QmZ2ckU7G35MYyJgMTwMUnicsGqSy3YUxGBX7qny6MQmJu": []string{
				"QmUBuxVHoNNjfmNpTad36UeaFQv3gXAtCv9r6KhmeqhEhD",
				"QmV35LjbEGPfN7KfMAJp43VV2enwXqqQf5esx4vUcgHDQJ",
			},
		},
		IPFSLinks: map[string][]string{
			"QmXbiVZd93SLiu9TAm21F2y9JwGiFLydbEVkPBaMR3DZDV": []string{
				"QmPFKAGZbUjdzt8BBx8VTWCe9UeUQVcoqHFehSPzN5LSsq",
				"QmbU7273zH6jxwDe2nqRmEm2rp5PpqP2xeQr2xCmwbBsuL",
			},
			"QmPFKAGZbUjdzt8BBx8VTWCe9UeUQVcoqHFehSPzN5LSsq": []string{
				"QmXbiVZd93SLiu9TAm21F2y9JwGiFLydbEVkPBaMR3DZDV",
				"QmbU7273zH6jxwDe2nqRmEm2rp5PpqP2xeQr2xCmwbBsuL",
			},
			"QmbU7273zH6jxwDe2nqRmEm2rp5PpqP2xeQr2xCmwbBsuL": []string{
				"QmXbiVZd93SLiu9TAm21F2y9JwGiFLydbEVkPBaMR3DZDV",
				"QmPFKAGZbUjdzt8BBx8VTWCe9UeUQVcoqHFehSPzN5LSsq",
			},
		},
		ClustertoIPFS: map[string]string{
			"QmUBuxVHoNNjfmNpTad36UeaFQv3gXAtCv9r6KhmeqhEhD": "QmXbiVZd93SLiu9TAm21F2y9JwGiFLydbEVkPBaMR3DZDV",
			"QmV35LjbEGPfN7KfMAJp43VV2enwXqqQf5esx4vUcgHDQJ": "QmPFKAGZbUjdzt8BBx8VTWCe9UeUQVcoqHFehSPzN5LSsq",
			"QmZ2ckU7G35MYyJgMTwMUnicsGqSy3YUxGBX7qny6MQmJu": "QmbU7273zH6jxwDe2nqRmEm2rp5PpqP2xeQr2xCmwbBsuL",
		},
	}
	buf := new(bytes.Buffer)
	err := makeDot(cg, buf, false)
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
	cg := api.ConnectGraphSerial{
		ClusterID: "QmUBuxVHoNNjfmNpTad36UeaFQv3gXAtCv9r6KhmeqhEhD",
		ClusterLinks: map[string][]string{
			"QmUBuxVHoNNjfmNpTad36UeaFQv3gXAtCv9r6KhmeqhEhD": []string{
				"QmV35LjbEGPfN7KfMAJp43VV2enwXqqQf5esx4vUcgHDQJ",
				"QmZ2ckU7G35MYyJgMTwMUnicsGqSy3YUxGBX7qny6MQmJu",
			},
			"QmV35LjbEGPfN7KfMAJp43VV2enwXqqQf5esx4vUcgHDQJ": []string{
				"QmUBuxVHoNNjfmNpTad36UeaFQv3gXAtCv9r6KhmeqhEhD",
				"QmZ2ckU7G35MYyJgMTwMUnicsGqSy3YUxGBX7qny6MQmJu",
			},
			"QmZ2ckU7G35MYyJgMTwMUnicsGqSy3YUxGBX7qny6MQmJu": []string{
				"QmUBuxVHoNNjfmNpTad36UeaFQv3gXAtCv9r6KhmeqhEhD",
				"QmV35LjbEGPfN7KfMAJp43VV2enwXqqQf5esx4vUcgHDQJ",
			},
		},
		IPFSLinks: map[string][]string{
			"QmXbiVZd93SLiu9TAm21F2y9JwGiFLydbEVkPBaMR3DZDV": []string{
				"QmPFKAGZbUjdzt8BBx8VTWCe9UeUQVcoqHFehSPzN5LSsq",
				"QmbU7273zH6jxwDe2nqRmEm2rp5PpqP2xeQr2xCmwbBsuL",
				"QmQsdAdCHs4PRLi5tcoLfasYppryqQENxgAy4b2aS8xccb",
				"QmVV2enwXqqQf5esx4v36UeaFQvFehSPzNfi8aaaaaanM8",
				"QmfCHNQ2vbUmAuJZhE2hEpgiJq4sL1XScWEKnUrVtWZdeD",
			},
			"QmPFKAGZbUjdzt8BBx8VTWCe9UeUQVcoqHFehSPzN5LSsq": []string{
				"QmXbiVZd93SLiu9TAm21F2y9JwGiFLydbEVkPBaMR3DZDV",
				"QmbU7273zH6jxwDe2nqRmEm2rp5PpqP2xeQr2xCmwbBsuL",
				"QmQsdAdCHs4PRLi5tcoLfasYppryqQENxgAy4b2aS8xccb",
				"QmVV2enwXqqQf5esx4v36UeaFQvFehSPzNfi8aaaaaanM8",
				"QmfCHNQ2vbUmAuJZhE2hEpgiJq4sL1XScWEKnUrVtWZdeD",
			},
			"QmbU7273zH6jxwDe2nqRmEm2rp5PpqP2xeQr2xCmwbBsuL": []string{
				"QmXbiVZd93SLiu9TAm21F2y9JwGiFLydbEVkPBaMR3DZDV",
				"QmPFKAGZbUjdzt8BBx8VTWCe9UeUQVcoqHFehSPzN5LSsq",
				"QmQsdAdCHs4PRLi5tcoLfasYppryqQENxgAy4b2aS8xccb",
				"QmVV2enwXqqQf5esx4v36UeaFQvFehSPzNfi8aaaaaanM8",
				"QmfCHNQ2vbUmAuJZhE2hEpgiJq4sL1XScWEKnUrVtWZdeD",
			},
		},
		ClustertoIPFS: map[string]string{
			"QmUBuxVHoNNjfmNpTad36UeaFQv3gXAtCv9r6KhmeqhEhD": "QmXbiVZd93SLiu9TAm21F2y9JwGiFLydbEVkPBaMR3DZDV",
			"QmV35LjbEGPfN7KfMAJp43VV2enwXqqQf5esx4vUcgHDQJ": "QmPFKAGZbUjdzt8BBx8VTWCe9UeUQVcoqHFehSPzN5LSsq",
			"QmZ2ckU7G35MYyJgMTwMUnicsGqSy3YUxGBX7qny6MQmJu": "QmbU7273zH6jxwDe2nqRmEm2rp5PpqP2xeQr2xCmwbBsuL",
		},
	}

	buf := new(bytes.Buffer)
	err := makeDot(cg, buf, true)
	if err != nil {
		t.Fatal(err)
	}
	verifyOutput(t, buf.String(), allIpfs)
}
