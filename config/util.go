package config

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
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
		sv.save = make(chan struct{}, 10)
	}

	// Non blocking, in case no one's listening
	select {
	case sv.save <- struct{}{}:
	default:
		logger.Warn("configuration save channel full")
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

// SetIfNotDefault sets dest to the value of src if src is not the default
// value of the type.
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
	case float64:
		n := src.(float64)
		if n != 0 {
			*dest.(*float64) = n
		}
	case bool:
		b := src.(bool)
		if b {
			*dest.(*bool) = b
		}
	}
}

// DurationOpt provides a datatype to use with ParseDurations
type DurationOpt struct {
	// The duration we need to parse
	Duration string
	// Where to store the result
	Dst *time.Duration
	// A variable name associated to it for helpful errors.
	Name string
}

// ParseDurations takes a time.Duration src and saves it to the given dst.
func ParseDurations(component string, args ...*DurationOpt) error {
	for _, arg := range args {
		if arg.Duration == "" {
			// don't do anything. Let the destination field
			// stay at its default.
			continue
		}
		t, err := time.ParseDuration(arg.Duration)
		if err != nil {
			return fmt.Errorf(
				"error parsing %s.%s: %s",
				component,
				arg.Name,
				err,
			)
		}
		*arg.Dst = t
	}
	return nil
}

type hiddenField struct{}

func (hf hiddenField) MarshalJSON() ([]byte, error) {
	return []byte(`"XXX_hidden_XXX"`), nil
}
func (hf hiddenField) UnmarshalJSON(b []byte) error { return nil }

// DisplayJSON takes pointer to a JSON-friendly configuration struct and
// returns the JSON-encoded representation of it filtering out any struct
// fields marked with the tag `hidden:"true"`, but keeping fields marked
// with `"json:omitempty"`.
func DisplayJSON(cfg interface{}) ([]byte, error) {
	cfg = reflect.Indirect(reflect.ValueOf(cfg)).Interface()
	origStructT := reflect.TypeOf(cfg)
	if origStructT.Kind() != reflect.Struct {
		panic("the given argument should be a struct")
	}

	hiddenFieldT := reflect.TypeOf(hiddenField{})

	// create a new struct type with same fields
	// but setting hidden fields as hidden.
	finalStructFields := []reflect.StructField{}
	for i := 0; i < origStructT.NumField(); i++ {
		f := origStructT.Field(i)
		hidden := f.Tag.Get("hidden") == "true"
		if f.PkgPath != "" { // skip unexported
			continue
		}
		if hidden {
			f.Type = hiddenFieldT
		}

		// remove omitempty from tag, ignore other tags except json
		var jsonTags []string
		for _, s := range strings.Split(f.Tag.Get("json"), ",") {
			if s != "omitempty" {
				jsonTags = append(jsonTags, s)
			}
		}
		f.Tag = reflect.StructTag(fmt.Sprintf("json:\"%s\"", strings.Join(jsonTags, ",")))

		finalStructFields = append(finalStructFields, f)
	}

	// Parse the original JSON into the new
	// struct and re-convert it to JSON.
	finalStructT := reflect.StructOf(finalStructFields)
	finalValue := reflect.New(finalStructT)
	data := finalValue.Interface()
	origJSON, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(origJSON, data)
	if err != nil {
		return nil, err
	}
	return DefaultJSONMarshal(data)
}

// Strings is a helper type that (un)marshals a single string to/from a single
// JSON string and a slice of strings to/from a JSON array of strings.
type Strings []string

// UnmarshalJSON conforms to the json.Unmarshaler interface.
func (o *Strings) UnmarshalJSON(data []byte) error {
	if data[0] == '[' {
		return json.Unmarshal(data, (*[]string)(o))
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	if len(value) == 0 {
		*o = []string{}
	} else {
		*o = []string{value}
	}
	return nil
}

// MarshalJSON conforms to the json.Marshaler interface.
func (o Strings) MarshalJSON() ([]byte, error) {
	switch len(o) {
	case 0:
		return json.Marshal([]string{})
	case 1:
		return json.Marshal(o[0])
	default:
		return json.Marshal([]string(o))
	}
}

var _ json.Unmarshaler = (*Strings)(nil)
var _ json.Marshaler = (*Strings)(nil)
