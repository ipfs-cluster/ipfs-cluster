package adder

import (
	"context"
	"testing"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/test"
)

// import and receive all blocks
func TestToChannelOutput(t *testing.T) {
	file, err := test.GetTestingDirSerial()
	if err != nil {
		t.Fatal(err)
	}

	printChan, outChan, errChan := ToChannel(context.Background(), file,
		false, false, false, "")

	go func() { // listen on printChan so progress can be made
		for {
			_, ok := <-printChan
			if !ok {
				// channel closed, safe to exit
				return
			}
		}
	}()

	go listenErrCh(t, errChan)

	objs := make([]interface{}, 0)
	for obj := range outChan {
		objs = append(objs, obj)
	}
	testChannelOutput(t, objs, test.TestDirCids[:])
}

func TestToChannelPrint(t *testing.T) {
	file, err := test.GetTestingDirSerial()
	if err != nil {
		t.Fatal(err)
	}

	printChan, outChan, errChan := ToChannel(context.Background(), file,
		false, false, false, "")

	go listenErrCh(t, errChan)

	go func() { // listen on outChan so progress can be made
		for {
			_, ok := <-outChan
			if !ok {
				// channel closed, safe to exit
				return
			}
		}
	}()
	objs := make([]interface{}, 0)
	for obj := range printChan {
		objs = append(objs, obj)
	}
	testChannelOutput(t, objs, test.TestDirCids[:15])
}

// listenErrCh listens on the error channel until closed and raise any errors
// that show up
func listenErrCh(t *testing.T, errChan <-chan error) {
	for {
		err, ok := <-errChan
		if !ok {
			// channel closed, safe to exit
			return
		}
		t.Fatal(err)
	}
}

// testChannelOutput is a utility for shared functionality of output and print
// channel testing
func testChannelOutput(t *testing.T, objs []interface{}, expected []string) {
	check := make(map[string]struct{})
	for _, obj := range objs {
		var cid string
		switch obj := obj.(type) {
		case *api.AddedOutput:
			cid = obj.Hash
		case *api.NodeWithMeta:
			cid = obj.Cid
		}
		if _, ok := check[cid]; ok {
			t.Fatalf("Duplicate cid %s", cid)
		}
		check[cid] = struct{}{}
	}
	if len(check) != len(expected) {
		t.Fatalf("Witnessed cids: %v\nExpected cids: %v", check, test.TestDirCids[:15])
	}
	for cid := range check {
		if !contains(expected, cid) {
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
