package ipfscluster

import (
	"bufio"
	"bytes"
	"log"
	"strings"
	"time"

	logging "github.com/ipfs/go-log"
)

var logger = logging.Logger("cluster")
var raftStdLogger = makeRaftLogger()
var raftLogger = logging.Logger("raft")

// SetLogLevel sets the level in the logs
func SetLogLevel(l string) {
	/*
		CRITICAL Level = iota
		ERROR
		WARNING
		NOTICE
		INFO
		DEBUG
	*/
	logging.SetLogLevel("cluster", l)
	//logging.SetLogLevel("raft", l)
	//logging.SetLogLevel("p2p-gorpc", l)
	//logging.SetLogLevel("swarm2", l)
	//logging.SetLogLevel("libp2p-raft", l)
}

// This redirects Raft output to our logger
func makeRaftLogger() *log.Logger {
	var buf bytes.Buffer
	rLogger := log.New(&buf, "", 0)
	reader := bufio.NewReader(&buf)
	go func() {
		for {
			t, err := reader.ReadString('\n')
			if err != nil {
				time.Sleep(time.Second)
				continue
			}
			t = strings.TrimSuffix(t, "\n")

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
		}
	}()
	return rLogger
}
