package ipfscluster

import logging "github.com/ipfs/go-log"

var logger = logging.Logger("cluster")

var (
	ansiGray   = "\033[0;37m"
	ansiYellow = "\033[0;33m"
)

func init() {
	// The whole purpose of this is to print the facility name in yellow
	// color in the logs because the current blue is very hard to read.
	logging.LogFormats["color"] = ansiGray +
		"%{time:15:04:05.000} %{color}%{level:5.5s} " +
		ansiYellow + "%{module:10.10s}: %{color:reset}%{message} " +
		ansiGray + "%{shortfile}%{color:reset}"
	logging.SetupLogging()
}

// LoggingFacilities provides a list of logging identifiers
// used by cluster and their default logging level.
var LoggingFacilities = map[string]string{
	"cluster":      "INFO",
	"restapi":      "INFO",
	"ipfsproxy":    "INFO",
	"ipfshttp":     "INFO",
	"monitor":      "INFO",
	"dsstate":      "INFO",
	"raft":         "INFO",
	"crdt":         "INFO",
	"pintracker":   "INFO",
	"ascendalloc":  "INFO",
	"diskinfo":     "INFO",
	"apitypes":     "INFO",
	"config":       "INFO",
	"shardingdags": "INFO",
	"localdags":    "INFO",
	"adder":        "INFO",
	"optracker":    "INFO",
	"pstoremgr":    "INFO",
}

// LoggingFacilitiesExtra provides logging identifiers
// used in ipfs-cluster dependencies, which may be useful
// to display. Along with their default value.
var LoggingFacilitiesExtra = map[string]string{
	"p2p-gorpc":   "CRITICAL",
	"swarm2":      "ERROR",
	"libp2p-raft": "CRITICAL",
	"raftlib":     "ERROR",
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
