package raft

import (
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	hclog "github.com/hashicorp/go-hclog"
	logging "github.com/ipfs/go-log"
)

const (
	debug = iota
	info
	warn
	err
)

var raftLogger = logging.Logger("raftlib")

// this implements github.com/hashicorp/go-hclog
type hcLogToLogger struct {
	extraArgs []interface{}
	name      string
}

func (log *hcLogToLogger) formatArgs(args []interface{}) string {
	result := ""
	args = append(args, log.extraArgs)
	for i := 0; i < len(args); i = i + 2 {
		key, ok := args[i].(string)
		if !ok {
			continue
		}
		val := args[i+1]
		result += fmt.Sprintf(" %s=%s.", key, val)
	}
	return result
}

func (log *hcLogToLogger) format(msg string, args []interface{}) string {
	argstr := log.formatArgs(args)
	if len(argstr) > 0 {
		argstr = ". Args: " + argstr
	}
	name := log.name
	if len(name) > 0 {
		name += ": "
	}
	return name + msg + argstr
}

func (log *hcLogToLogger) Trace(msg string, args ...interface{}) {
	raftLogger.Debug(log.format(msg, args))
}

func (log *hcLogToLogger) Debug(msg string, args ...interface{}) {
	raftLogger.Debug(log.format(msg, args))
}

func (log *hcLogToLogger) Info(msg string, args ...interface{}) {
	raftLogger.Info(log.format(msg, args))
}

func (log *hcLogToLogger) Warn(msg string, args ...interface{}) {
	raftLogger.Warning(log.format(msg, args))
}

func (log *hcLogToLogger) Error(msg string, args ...interface{}) {
	raftLogger.Error(log.format(msg, args))
}

func (log *hcLogToLogger) IsTrace() bool {
	return true
}

func (log *hcLogToLogger) IsDebug() bool {
	return true
}

func (log *hcLogToLogger) IsInfo() bool {
	return true
}

func (log *hcLogToLogger) IsWarn() bool {
	return true
}

func (log *hcLogToLogger) IsError() bool {
	return true
}

func (log *hcLogToLogger) With(args ...interface{}) hclog.Logger {
	return &hcLogToLogger{extraArgs: args}
}

func (log *hcLogToLogger) Named(name string) hclog.Logger {
	return &hcLogToLogger{name: log.name + ": " + name}
}

func (log *hcLogToLogger) ResetNamed(name string) hclog.Logger {
	return &hcLogToLogger{name: name}
}

func (log *hcLogToLogger) SetLevel(level hclog.Level) {}

func (log *hcLogToLogger) StandardLogger(opts *hclog.StandardLoggerOptions) *log.Logger {
	return nil
}

func (log *hcLogToLogger) StandardWriter(opts *hclog.StandardLoggerOptions) io.Writer {
	return nil
}

const repeatPoolSize = 10
const repeatReset = time.Minute

// This provides a custom logger for Raft which intercepts Raft log messages
// and rewrites us to our own logger (for "raft" facility).
type logForwarder struct {
	lastMsgs map[int][]string
	lastTip  map[int]time.Time
}

var raftStdLogger = log.New(&logForwarder{}, "", 0)

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
