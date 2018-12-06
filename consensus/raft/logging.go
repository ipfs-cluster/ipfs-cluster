package raft

import (
	"log"
	"strings"
	"time"

	logging "github.com/ipfs/go-log"
)

const (
	debug = iota
	info
	warn
	err
)

const repeatPoolSize = 10
const repeatReset = time.Minute

// This provides a custom logger for Raft which intercepts Raft log messages
// and rewrites us to our own logger (for "raft" facility).
type logForwarder struct {
	lastMsgs map[int][]string
	lastTip  map[int]time.Time
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
	if fw.lastMsgs == nil {
		fw.lastMsgs = make(map[int][]string)
		fw.lastTip = make(map[int]time.Time)
	}

	// We haven't tipped about repeated log messages
	// in a while, do it and forget the list
	if time.Now().After(fw.lastTip[t].Add(repeatReset)) {
		fw.lastTip[t] = time.Now()
		fw.lastMsgs[t] = nil
		fw.log(t, "NOTICE: Some RAFT log messages repeat and will only be logged once")
	}

	var found string

	// Do we know about this message
	for _, lmsg := range fw.lastMsgs[t] {
		if lmsg == msg {
			found = lmsg
			break
		}
	}

	if found == "" { // new message. Add to slice.
		if len(fw.lastMsgs[t]) >= repeatPoolSize { // drop oldest
			fw.lastMsgs[t] = fw.lastMsgs[t][1:]
		}
		fw.lastMsgs[t] = append(fw.lastMsgs[t], msg)
		return false // not-repeated
	}

	// repeated, don't log
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
