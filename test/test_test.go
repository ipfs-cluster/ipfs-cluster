package test

import (
	"reflect"
	"testing"

	ipfscluster "github.com/ipfs-cluster/ipfs-cluster"
)

func TestIpfsMock(t *testing.T) {
	ipfsmock := NewIpfsMock(t)
	defer ipfsmock.Close()
}

// Test that our RPC mock resembles the original
func TestRPCMockValid(t *testing.T) {
	type tc struct {
		mock reflect.Type
		real reflect.Type
	}

	tcs := []tc{
		{
			real: reflect.TypeOf(&ipfscluster.ClusterRPCAPI{}),
			mock: reflect.TypeOf(&mockCluster{}),
		},
		{
			real: reflect.TypeOf(&ipfscluster.PinTrackerRPCAPI{}),
			mock: reflect.TypeOf(&mockPinTracker{}),
		},
		{
			real: reflect.TypeOf(&ipfscluster.IPFSConnectorRPCAPI{}),
			mock: reflect.TypeOf(&mockIPFSConnector{}),
		},
		{
			real: reflect.TypeOf(&ipfscluster.ConsensusRPCAPI{}),
			mock: reflect.TypeOf(&mockConsensus{}),
		},
		{
			real: reflect.TypeOf(&ipfscluster.PeerMonitorRPCAPI{}),
			mock: reflect.TypeOf(&mockPeerMonitor{}),
		},
	}

	for _, tc := range tcs {
		realT := tc.real
		mockT := tc.mock

		// Make sure all the methods we have match the original
		for i := 0; i < mockT.NumMethod(); i++ {
			method := mockT.Method(i)
			name := method.Name
			origMethod, ok := realT.MethodByName(name)
			if !ok {
				t.Fatalf("%s method not found in real RPC", name)
			}

			mType := method.Type
			oType := origMethod.Type

			if nout := mType.NumOut(); nout != 1 || nout != oType.NumOut() {
				t.Errorf("%s: more than 1 out parameter", name)
			}

			if mType.Out(0).Name() != "error" {
				t.Errorf("%s out param should be an error", name)
			}

			if nin := mType.NumIn(); nin != oType.NumIn() || nin != 4 {
				t.Fatalf("%s: num in parameter mismatch: %d vs. %d", name, nin, oType.NumIn())
			}

			for j := 1; j < 4; j++ {
				mn := mType.In(j).String()
				on := oType.In(j).String()
				if mn != on {
					t.Errorf("%s: name mismatch: %s vs %s", name, mn, on)
				}
			}
		}

		for i := 0; i < realT.NumMethod(); i++ {
			name := realT.Method(i).Name
			_, ok := mockT.MethodByName(name)
			if !ok {
				t.Logf("Warning: %s: unimplemented in mock rpc", name)
			}
		}
	}
}

// Test that testing directory is created without error
func TestGenerateTestDirs(t *testing.T) {
	sth := NewShardingTestHelper()
	defer sth.Clean(t)
	_, closer := sth.GetTreeMultiReader(t)
	closer.Close()
	_, closer = sth.GetRandFileMultiReader(t, 2)
	closer.Close()
}
