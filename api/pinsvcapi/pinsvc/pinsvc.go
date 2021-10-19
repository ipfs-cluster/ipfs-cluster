// Package pinsvc contains type definitions for the Pinning Services API
package pinsvc

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multiaddr"
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
	Reason  string `json:"reason"`
	Details string `json:"string"`
}

func (apiErr APIError) Error() string {
	return apiErr.Reason
}

// Pin contains basic information about a Pin and pinning options.
type Pin struct {
	Cid     cid.Cid               `json:"cid"`
	Name    string                `json:"name"`
	Origins []multiaddr.Multiaddr `json:"origins"`
	Meta    map[string]string     `json:"meta"`
}

type Status int

// Status values
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
	var status Status
	for _, v := range values {
		st, ok := stringStatus[v]
		if ok {
			status |= st
		}
	}
	return status
}

// PinStatus provides information about a Pin stored by the Pinning API.
type PinStatus struct {
	RequestID string                `json:"request_id"`
	Status    Status                `json:"status"`
	Created   time.Time             `json:"created"`
	Pin       Pin                   `json:"pin"`
	Delegates []multiaddr.Multiaddr `json:"delegates"`
	Info      map[string]string     `json:"info"`
}

// PinList is the result of a call to List pins
type PinList struct {
	Count   int         `json:"count"`
	Results []PinStatus `json:"results"`
}

// Match defines a type of match for filtering pin lists.
type Match int

// Values for matches.
const (
	MatchUndefined Match = iota
	MatchExact
	MatchIexact
	MatchPartial
	MatchIpartial
)

// MatchFromString converts a string to its Match value.
func MatchFromString(str string) Match {
	switch str {
	case "exact":
		return MatchExact
	case "iexact":
		return MatchIexact
	case "partial":
		return MatchPartial
	case "ipartial":
		return MatchIpartial
	default:
		return MatchUndefined
	}
}

// ListOptions represents possible options given to the List endpoint.
type ListOptions struct {
	Cids   []cid.Cid
	Name   string
	Match  Match
	Status Status
	Before time.Time
	After  time.Time
	Limit  int
	Meta   map[string]string
}

func (lo *ListOptions) FromQuery(q url.Values) error {
	for _, cstr := range strings.Split(q.Get("cid"), ",") {
		c, err := cid.Decode(cstr)
		if err != nil {
			return fmt.Errorf("error decoding cid %s: %w", cstr, err)
		}
		lo.Cids = append(lo.Cids, c)
	}

	lo.Name = q.Get("name")
	lo.Match = MatchFromString(q.Get("match"))
	lo.Status = StatusFromString(q.Get("status"))

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
		lo.Limit = int(lim)
	}

	if meta := q.Get("meta"); meta != "" {
		err := json.Unmarshal([]byte(meta), &lo.Meta)
		if err != nil {
			return fmt.Errorf("error unmarshalling 'meta' query param: %s: %w", meta, err)
		}
	}

	return nil
}
