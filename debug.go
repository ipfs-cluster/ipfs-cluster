// +build debug,!silent

package ipfscluster

func init() {
	l := "DEBUG"
	SetFacilityLogLevel("cluster", l)
	SetFacilityLogLevel("restapi", l)
	SetFacilityLogLevel("ipfshttp", l)
	//SetFacilityLogLevel("raft", l)
	//SetFacilityLogLevel("p2p-gorpc", l)
	//SetFacilityLogLevel("swarm2", l)
	//SetFacilityLogLevel("libp2p-raft", l)
}
