// +build !debug,silent

package ipfscluster

func init() {
	l := "CRITICAL"
	for _, f := range facilities {
		SetFacilityLogLevel(f, l)
	}

	SetFacilityLogLevel("p2p-gorpc", l)
	SetFacilityLogLevel("swarm2", l)
	SetFacilityLogLevel("libp2p-raft", l)
}
