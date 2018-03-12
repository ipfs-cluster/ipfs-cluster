package importer

import (
	"context"
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/ipfs/go-ipfs-cmdkit/files"
)

const testDir = "testingData"

func getTestingDir() (files.File, error) {
	fpath := testDir
	stat, err := os.Lstat(fpath)
	if err != nil {
		return nil, err
	}

	if !stat.IsDir() {
		return nil, fmt.Errorf("testDir should be seen as directory")
	}

	return files.NewSerialFile(path.Base(fpath), fpath, false, stat)
}

var cids = [18]string{"QmUNLLsPACCz1vLxQVkXqqLX5R1X345qqfHbsf67hvA3Nn",
	"Qmbp4C4KkyjVzTpZ327Ub555FEHizhJS4M17f2zCCrQMAz",
	"QmYz38urZ99eVCxSccM63bGtDv54UWtBDWJdTxGch23khA",
	"QmUwG2mfhhfBEzVtvyEvva1kc8kU4CrXphdMCdAiFNWdxy",
	"QmRgkP4x7tXW9PyiPPxe3aqALQEs22nifkwzrm7wickdfr",
	"QmNpCHs9zrzP4aArBzRQgjNSbMC5hYqJa1ksmbyorSu44b",
	"QmQbg4sHm4zVHnqCS14YzNnQFVMqjZpu5XjkF7vtbzqkFW",
	"QmcUBApwNSDg2Q2J4NXzks1HewVGFohNpPyEud7bZfo5tE",
	"QmaKaB735eydQwnaJNuYbXRU1gVb4MJdzHp1rpUZJN67G6",
	"QmQ6n82dRtEJVHMDLW5BSrJ6bRDXkFXj2i7Vjxy4B2PH82",
	"QmPZLJ3CZYgxH4K1w5jdbAdxJynXn5TCB4kHy7u8uHC3fy",
	"QmUqmcdJJodX7mPoUv9u71HoNAiegBCiCqdUZd5ZrCyLbs",
	"QmbDaWrwxk93RfWSBL8ajTproNKbE2EghBLSk8199EBm1m",
	"QmXDmRPxV9KWYLhN7bqGn143GgiNxhy9Hqc3ZPJW63xgok",
	"QmaytnNarHzDp1ipGyC7zd7Hw2AePmtSpLLaygQ2e9Yvqe",
	"QmZwab9h6ADw3tv8pzXVF2yndgJpTWKrrMjJQqRkgYmCRH",
	"QmaNfMZDZjfqjHrFCc6tZwmkqbXs1fnY9AXZ81WUztFeXm",
	"QmY4qt6WG12qYwWeeTNhggqaNMJWp2NouuTSf79ukoobw8"}

// import and receive all blocks
func TestToChannel(t *testing.T) {
	file, err := getTestingDir()
	if err != nil {
		t.Fatal(err)
	}

	printChan, outChan, errChan := ToChannel(context.Background(), file,
		false, false, false, false, false, false, "")

	go func() { // listen on printChan so progress can be made
		for {
			_, ok := <-printChan
			if !ok {
				// channel closed, safe to exit
				return
			}
		}
	}()

	go func() { // listen for errors
		for {
			err, ok := <-errChan
			if !ok {
				// channel closed, safe to exit
				return
			}
			t.Fatal(err)
		}
	}()

	check := make(map[string]struct{})
	for obj := range outChan {
		cid := obj.Cid
		if _, ok := check[cid]; ok {
			t.Fatalf("Duplicate cid %s", cid)
		}
		check[cid] = struct{}{}
	}
	if len(check) != len(cids) {
		t.Fatalf("Witnessed cids: %v\nExpected cids: %v", check, cids)
	}
	cidsS := cids[:]
	for cid := range check {
		if !contains(cidsS, cid) {
			t.Fatalf("Unexpected cid: %s", cid)
		}
	}
}

func contains(slice []string, s string) bool {
	for _, a := range slice {
		if a == s {
			return true
		}
	}
	return false
}
