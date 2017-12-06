package raft

import (
	"log"
	"strings"

	logging "github.com/ipfs/go-log"
)

const (
	debug = iota
	info
	warn
	err
)

// This provides a custom logger for Raft which intercepts Raft log messages
// and rewrites us to our own logger (for "raft" facility).
type logForwarder struct {
	last map[int]*lastMsg
}

type lastMsg struct {
	msg    string
	tipped bool
}

var raftStdLogger = log.New(&logForwarder{}, "", 0)
var raftLogger = logging.Logger("raft")

// Write forwards to our go-log logger.
// According to https://golang.org/pkg/log/#Logger.Output
// it is called per line.
func (fw *logForwarder) Write(p []byte) (n int, e error) {
	t := strings.TrimSuffix(string(p), "\n")

	switch {
	case strings.Contains(t, "[DEBUG]"):
		if !fw.repeated(debug, t) {
			fw.log(debug, strings.TrimPrefix(t, "[DEBUG] raft: "))
		}
	case strings.Contains(t, "[WARN]"):
		if !fw.repeated(warn, t) {
			fw.log(warn, strings.TrimPrefix(t, "[WARN]  raft: "))
		}
	case strings.Contains(t, "[ERR]"):
		if !fw.repeated(err, t) {
			fw.log(err, strings.TrimPrefix(t, "[ERR] raft: "))
		}
	case strings.Contains(t, "[INFO]"):
		if !fw.repeated(info, t) {
			fw.log(info, strings.TrimPrefix(t, "[INFO] raft: "))
		}
	default:
		fw.log(debug, t)
	}
	return len(p), nil
}

func (fw *logForwarder) repeated(t int, msg string) bool {
	if fw.last == nil {
		fw.last = make(map[int]*lastMsg)
	}

	last, ok := fw.last[t]
	if !ok || last.msg != msg {
		fw.last[t] = &lastMsg{msg, false}
		return false
	}
	if !last.tipped {
		fw.log(t, "NOTICE: The last RAFT log message repeats and will only be logged once")
		last.tipped = true
	}
	return true
}

func (fw *logForwarder) log(t int, msg string) {
	switch t {
	case debug:
		raftLogger.Debug(msg)
	case info:
		raftLogger.Info(msg)
	case warn:
		raftLogger.Warning(msg)
	case err:
		raftLogger.Error(msg)
	default:
		raftLogger.Debug(msg)
	}
}
