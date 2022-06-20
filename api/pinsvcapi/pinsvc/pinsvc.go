// Package pinsvc contains type definitions for the Pinning Services API
package pinsvc

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	types "github.com/ipfs-cluster/ipfs-cluster/api"
)

func init() {
	// intialize trackerStatusString
	stringStatus = make(map[string]Status)
	for k, v := range statusString {
		stringStatus[v] = k
	}
}

// APIError is returned by the API as a body when an error
// occurs. It implements the error interface.
type APIError struct {
	Details APIErrorDetails `json:"error"`
}

// APIErrorDetails contains details about the APIError.
type APIErrorDetails struct {
	Reason  string `json:"reason"`
	Details string `json:"details,omitempty"`
}

func (apiErr APIError) Error() string {
	return apiErr.Details.Reason
}

// PinName is a string limited to 255 chars when serializing JSON.
type PinName string

// MarshalJSON converts the string to JSON.
func (pname PinName) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(pname))
}

// UnmarshalJSON reads the JSON string and errors if over 256 chars.
func (pname *PinName) UnmarshalJSON(data []byte) error {
	if len(data) > 257 { // "a_string" 255 + 2 for quotes
		return errors.New("pin name is over 255 chars")
	}
	var v string
	err := json.Unmarshal(data, &v)
	*pname = PinName(v)
	return err
}

// Pin contains basic information about a Pin and pinning options.
type Pin struct {
	Cid     types.Cid         `json:"cid"`
	Name    PinName           `json:"name,omitempty"`
	Origins []types.Multiaddr `json:"origins,omitempty"`
	Meta    map[string]string `json:"meta,omitempty"`
}

// Defined returns if the pinis empty (Cid not set).
func (p Pin) Defined() bool {
	return p.Cid.Defined()
}

// MatchesName returns in a pin status matches a name option with a given
// match strategy.
func (p Pin) MatchesName(nameOpt string, strategy MatchingStrategy) bool {
	if nameOpt == "" {
		return true
	}
	name := string(p.Name)

	switch strategy {
	case MatchingStrategyUndefined:
		return true

	case MatchingStrategyExact:
		return nameOpt == name
	case MatchingStrategyIexact:
		return strings.EqualFold(name, nameOpt)
	case MatchingStrategyPartial:
		return strings.Contains(name, nameOpt)
	case MatchingStrategyIpartial:
		return strings.Contains(strings.ToLower(name), strings.ToLower(nameOpt))
	default:
		return true
	}
}

// MatchesMeta returns true if the pin status metadata matches the given.  The
// metadata should have all the keys in the given metaOpts and the values
// should, be the same (metadata map includes metaOpts).
func (p Pin) MatchesMeta(metaOpts map[string]string) bool {
	for k, v := range metaOpts {
		if p.Meta[k] != v {
			return false
		}
	}
	return true
}

// Status represents a pin status, which defines the current state of the pin
// in the system.
type Status int

// Values for the Status type.
const (
	StatusUndefined Status = 0
	StatusQueued           = 1 << iota
	StatusPinned
	StatusPinning
	StatusFailed
)

var statusString = map[Status]string{
	StatusUndefined: "undefined",
	StatusQueued:    "queued",
	StatusPinned:    "pinned",
	StatusPinning:   "pinning",
	StatusFailed:    "failed",
}

// values autofilled in init()
var stringStatus map[string]Status

// String converts a Status into a readable string.
// If the given Status is a filter (with several
// bits set), it will return a comma-separated list.
func (st Status) String() string {
	var values []string

	// simple and known composite values
	if v, ok := statusString[st]; ok {
		return v
	}

	// other filters
	for k, v := range statusString {
		if st&k > 0 {
			values = append(values, v)
		}
	}

	return strings.Join(values, ",")
}

// Match returns true if the tracker status matches the given filter.
func (st Status) Match(filter Status) bool {
	return filter == StatusUndefined ||
		st == StatusUndefined ||
		st&filter > 0
}

// MarshalJSON uses the string representation of Status for JSON
// encoding.
func (st Status) MarshalJSON() ([]byte, error) {
	return json.Marshal(st.String())
}

// UnmarshalJSON sets a tracker status from its JSON representation.
func (st *Status) UnmarshalJSON(data []byte) error {
	var v string
	err := json.Unmarshal(data, &v)
	if err != nil {
		return err
	}
	*st = StatusFromString(v)
	return nil
}

// StatusFromString parses a string and returns the matching
// Status value. The string can be a comma-separated list
// representing a Status filter. Unknown status names are
// ignored.
func StatusFromString(str string) Status {
	values := strings.Split(strings.Replace(str, " ", "", -1), ",")
	status := StatusUndefined
	for _, v := range values {
		st, ok := stringStatus[v]
		if ok {
			status |= st
		}
	}
	return status
}

// MatchingStrategy defines a type of match for filtering pin lists.
type MatchingStrategy int

// Values for MatchingStrategy.
const (
	MatchingStrategyUndefined MatchingStrategy = iota
	MatchingStrategyExact
	MatchingStrategyIexact
	MatchingStrategyPartial
	MatchingStrategyIpartial
)

// MatchingStrategyFromString converts a string to its MatchingStrategy value.
func MatchingStrategyFromString(str string) MatchingStrategy {
	switch str {
	case "exact":
		return MatchingStrategyExact
	case "iexact":
		return MatchingStrategyIexact
	case "partial":
		return MatchingStrategyPartial
	case "ipartial":
		return MatchingStrategyIpartial
	default:
		return MatchingStrategyUndefined
	}
}

// PinStatus provides information about a Pin stored by the Pinning API.
type PinStatus struct {
	RequestID string            `json:"requestid"`
	Status    Status            `json:"status"`
	Created   time.Time         `json:"created"`
	Pin       Pin               `json:"pin"`
	Delegates []types.Multiaddr `json:"delegates"`
	Info      map[string]string `json:"info,omitempty"`
}

// PinList is the result of a call to List pins
type PinList struct {
	Count   uint64      `json:"count"`
	Results []PinStatus `json:"results"`
}

// ListOptions represents possible options given to the List endpoint.
type ListOptions struct {
	Cids             []types.Cid
	Name             string
	MatchingStrategy MatchingStrategy
	Status           Status
	Before           time.Time
	After            time.Time
	Limit            uint64
	Meta             map[string]string
}

// FromQuery parses ListOptions from url.Values.
func (lo *ListOptions) FromQuery(q url.Values) error {
	cidq := q.Get("cid")
	if len(cidq) > 0 {
		for _, cstr := range strings.Split(cidq, ",") {
			c, err := types.DecodeCid(cstr)
			if err != nil {
				return fmt.Errorf("error decoding cid %s: %w", cstr, err)
			}
			lo.Cids = append(lo.Cids, c)
		}
	}

	n := q.Get("name")
	if len(n) > 255 {
		return fmt.Errorf("error in 'name' query param: longer than 255 chars")
	}
	lo.Name = n

	lo.MatchingStrategy = MatchingStrategyFromString(q.Get("match"))
	if lo.MatchingStrategy == MatchingStrategyUndefined {
		lo.MatchingStrategy = MatchingStrategyExact // default
	}
	statusStr := q.Get("status")
	lo.Status = StatusFromString(statusStr)
	// FIXME: This is a bit lazy, as "invalidxx,pinned" would result in a
	// valid "pinned" filter.
	if statusStr != "" && lo.Status == StatusUndefined {
		return fmt.Errorf("error decoding 'status' query param: no valid filter")
	}

	if bef := q.Get("before"); bef != "" {
		err := lo.Before.UnmarshalText([]byte(bef))
		if err != nil {
			return fmt.Errorf("error decoding 'before' query param: %s: %w", bef, err)
		}
	}

	if after := q.Get("after"); after != "" {
		err := lo.After.UnmarshalText([]byte(after))
		if err != nil {
			return fmt.Errorf("error decoding 'after' query param: %s: %w", after, err)
		}
	}

	if v := q.Get("limit"); v != "" {
		lim, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return fmt.Errorf("error parsing 'limit' query param: %s: %w", v, err)
		}
		lo.Limit = lim
	} else {
		lo.Limit = 10 // implicit default
	}

	if meta := q.Get("meta"); meta != "" {
		err := json.Unmarshal([]byte(meta), &lo.Meta)
		if err != nil {
			return fmt.Errorf("error unmarshalling 'meta' query param: %s: %w", meta, err)
		}
	}

	return nil
}
