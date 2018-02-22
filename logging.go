package ipfscluster

import logging "github.com/ipfs/go-log"

var logger = logging.Logger("cluster")

// LoggingFacilities provides a list of logging identifiers
// used by cluster and their default logging level.
var LoggingFacilities = map[string]string{
	"cluster":     "INFO",
	"restapi":     "INFO",
	"ipfshttp":    "INFO",
	"monitor":     "INFO",
	"mapstate":    "INFO",
	"consensus":   "INFO",
	"pintracker":  "INFO",
	"ascendalloc": "INFO",
	"diskinfo":    "INFO",
	"apitypes":    "INFO",
	"config":      "INFO",
	"sharder":     "INFO",
}

// LoggingFacilitiesExtra provides logging identifiers
// used in ipfs-cluster dependencies, which may be useful
// to display. Along with their default value.
var LoggingFacilitiesExtra = map[string]string{
	"p2p-gorpc":   "CRITICAL",
	"swarm2":      "ERROR",
	"libp2p-raft": "CRITICAL",
	"raft":        "ERROR",
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
