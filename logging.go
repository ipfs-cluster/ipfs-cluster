package ipfscluster

import logging "gx/ipfs/QmSpJByNKFX1sCsHBEp3R73FL4NF6FnQTEGyNAXHm2GS52/go-log"

var logger = logging.Logger("cluster")

var facilities = []string{
	"cluster",
	"restapi",
	"ipfshttp",
	"monitor",
	"consensus",
	"raft",
	"pintracker",
	"ascendalloc",
	"diskinfo",
}

// SetFacilityLogLevel sets the log level for a given module
func SetFacilityLogLevel(f, l string) {
	/*
		CRITICAL Level = iota
		ERROR
		WARNING
		NOTICE
		INFO
		DEBUG
	*/
	logging.SetLogLevel(f, l)
}
