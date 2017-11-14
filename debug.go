// +build debug,!silent

package ipfscluster

func init() {
	l := "DEBUG"
	for _, f := range facilities {
		SetFacilityLogLevel(f, l)
	}

	//SetFacilityLogLevel("cluster", l)
	//SetFacilityLogLevel("consensus", l)
	//SetFacilityLogLevel("monitor", "INFO")
	//SetFacilityLogLevel("raft", l)
	SetFacilityLogLevel("p2p-gorpc", l)
	//SetFacilityLogLevel("swarm2", l)
	//SetFacilityLogLevel("libp2p-raft", l)
}
