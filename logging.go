package ipfscluster

import (
	logging "github.com/ipfs/go-log/v2"
)

var logger = logging.Logger("cluster")

// LoggingFacilities provides a list of logging identifiers
// used by cluster and their default logging level.
var LoggingFacilities = map[string]string{
	"cluster":      "INFO",
	"restapi":      "INFO",
	"restapilog":   "INFO",
	"pinsvcapi":    "INFO",
	"pinsvcapilog": "INFO",
	"ipfsproxy":    "INFO",
	"ipfsproxylog": "INFO",
	"ipfshttp":     "INFO",
	"monitor":      "INFO",
	"dsstate":      "INFO",
	"raft":         "INFO",
	"crdt":         "INFO",
	"pintracker":   "INFO",
	"diskinfo":     "INFO",
	"tags":         "INFO",
	"apitypes":     "INFO",
	"config":       "INFO",
	"shardingdags": "INFO",
	"singledags":   "INFO",
	"adder":        "INFO",
	"optracker":    "INFO",
	"pstoremgr":    "INFO",
	"allocator":    "INFO",
}

// LoggingFacilitiesExtra provides logging identifiers
// used in ipfs-cluster dependencies, which may be useful
// to display. Along with their default value.
var LoggingFacilitiesExtra = map[string]string{
	"p2p-gorpc":   "ERROR",
	"swarm2":      "ERROR",
	"libp2p-raft": "FATAL",
	"raftlib":     "ERROR",
	"badger":      "INFO",
	"badger3":     "INFO",
	"pebble":      "WARN", // pebble logs with INFO and FATAL only
}

// SetFacilityLogLevel sets the log level for a given module
func SetFacilityLogLevel(f, l string) {
	/*
		case "debug", "DEBUG":
			*l = DebugLevel
		case "info", "INFO", "": // make the zero value useful
			*l = InfoLevel
		case "warn", "WARN":
			*l = WarnLevel
		case "error", "ERROR":
			*l = ErrorLevel
		case "dpanic", "DPANIC":
			*l = DPanicLevel
		case "panic", "PANIC":
			*l = PanicLevel
		case "fatal", "FATAL":
			*l = FatalLevel
	*/
	logging.SetLogLevel(f, l)
}
