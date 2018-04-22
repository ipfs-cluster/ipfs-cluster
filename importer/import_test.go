package importer

import (
	"context"
	"testing"

	"github.com/ipfs/ipfs-cluster/test"
)

// import and receive all blocks
func TestToChannelOutput(t *testing.T) {
	file, err := test.GetTestingDirSerial()
	if err != nil {
		t.Fatal(err)
	}

	printChan, outChan, errChan := ToChannel(context.Background(), file,
		false, false, false, false, "")

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
	if len(check) != len(test.TestDirCids) {
		t.Fatalf("Witnessed cids: %v\nExpected cids: %v", check, test.TestDirCids)
	}
	cidsS := test.TestDirCids[:]
	for cid := range check {
		if !contains(cidsS, cid) {
			t.Fatalf("Unexpected cid: %s", cid)
		}
	}
}

func TestToChannelPrint(t *testing.T) {
	file, err := test.GetTestingDirSerial()
	if err != nil {
		t.Fatal(err)
	}

	printChan, outChan, errChan := ToChannel(context.Background(), file,
		false, false, false, false, "")

	go func() { // listen on outChan so progress can be made
		for {
			_, ok := <-outChan
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
	for obj := range printChan {
		cid := obj.Hash
		if _, ok := check[cid]; ok {
			t.Fatalf("Duplicate cid %s", cid)
		}
		check[cid] = struct{}{}
	}
	if len(check) != len(test.TestDirCids[:15]) {
		t.Fatalf("Witnessed cids: %v\nExpected cids: %v", check, test.TestDirCids[:15])
	}
	cidsS := test.TestDirCids[:15]
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
