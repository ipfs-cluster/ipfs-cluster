package ipfscluster

import (
	"log"
	"strings"

	logging "github.com/ipfs/go-log"
)

var logger = logging.Logger("cluster")
var raftStdLogger = log.New(&logForwarder{}, "", 0)
var raftLogger = logging.Logger("raft")

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

// implements the writer interface
type logForwarder struct{}

// Write forwards to our go-log logger.
// According to https://golang.org/pkg/log/#Logger.Output
// it is called per line.
func (fw *logForwarder) Write(p []byte) (n int, err error) {
	t := strings.TrimSuffix(string(p), "\n")
	switch {
	case strings.Contains(t, "[DEBUG]"):
		raftLogger.Debug(strings.TrimPrefix(t, "[DEBUG] raft: "))
	case strings.Contains(t, "[WARN]"):
		raftLogger.Warning(strings.TrimPrefix(t, "[WARN]  raft: "))
	case strings.Contains(t, "[ERR]"):
		raftLogger.Error(strings.TrimPrefix(t, "[ERR] raft: "))
	case strings.Contains(t, "[INFO]"):
		raftLogger.Info(strings.TrimPrefix(t, "[INFO] raft: "))
	default:
		raftLogger.Debug(t)
	}
	return len(p), nil
}
