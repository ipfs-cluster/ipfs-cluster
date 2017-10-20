package config

import (
	"encoding/json"
	"time"
)

// Saver implements common functionality useful for ComponentConfigs
type Saver struct {
	save    chan struct{}
	BaseDir string
}

// NotifySave signals the SaveCh() channel in a non-blocking fashion.
func (sv *Saver) NotifySave() {
	if sv.save == nil {
		sv.save = make(chan struct{})
	}

	// Non blocking, in case no one's listening
	select {
	case sv.save <- struct{}{}:
	default:
		logger.Warning("configuration will not be saved")
	}
}

// SaveCh returns a channel which is signaled when a component wants
// to persist its configuration
func (sv *Saver) SaveCh() <-chan struct{} {
	if sv.save == nil {
		sv.save = make(chan struct{})
	}
	return sv.save
}

// SetBaseDir is a setter for BaseDir and implements
// part of the ComponentConfig interface.
func (sv *Saver) SetBaseDir(dir string) {
	sv.BaseDir = dir
}

// DefaultJSONMarshal produces pretty JSON with 2-space indentation
func DefaultJSONMarshal(v interface{}) ([]byte, error) {
	bs, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return bs, nil
}

// SetIfNotDefault dest to the value of src if src is not the default value.
// dest must be a pointer.
func SetIfNotDefault(src interface{}, dest interface{}) {
	switch src.(type) {
	case time.Duration:
		t := src.(time.Duration)
		if t != 0 {
			*dest.(*time.Duration) = t
		}
	case string:
		str := src.(string)
		if str != "" {
			*dest.(*string) = str
		}
	case uint64:
		n := src.(uint64)
		if n != 0 {
			*dest.(*uint64) = n
		}
	case int:
		n := src.(int)
		if n != 0 {
			*dest.(*int) = n
		}
	case bool:
		b := src.(bool)
		if b {
			*dest.(*bool) = b
		}
	}
}
