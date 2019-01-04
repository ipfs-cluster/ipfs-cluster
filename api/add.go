package api

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
)

// DefaultShardSize is the shard size for params objects created with DefaultParams().
var DefaultShardSize = uint64(100 * 1024 * 1024) // 100 MB

// AddedOutput carries information for displaying the standard ipfs output
// indicating a node of a file has been added.
type AddedOutput struct {
	Name  string `json:"name"`
	Cid   string `json:"cid,omitempty"`
	Bytes uint64 `json:"bytes,omitempty"`
	Size  uint64 `json:"size,omitempty"`
}

// AddParams contains all of the configurable parameters needed to specify the
// importing process of a file being added to an ipfs-cluster
type AddParams struct {
	PinOptions

	Recursive      bool
	Layout         string
	Chunker        string
	RawLeaves      bool
	Hidden         bool
	Wrap           bool
	Shard          bool
	Progress       bool
	CidVersion     int
	HashFun        string
	StreamChannels bool
}

// DefaultAddParams returns a AddParams object with standard defaults
func DefaultAddParams() *AddParams {
	return &AddParams{
		Recursive:      false,
		Layout:         "", // corresponds to balanced layout
		Chunker:        "size-262144",
		RawLeaves:      false,
		Hidden:         false,
		Wrap:           false,
		Shard:          false,
		Progress:       false,
		CidVersion:     0,
		HashFun:        "sha2-256",
		StreamChannels: true,
		PinOptions: PinOptions{
			ReplicationFactorMin: 0,
			ReplicationFactorMax: 0,
			Name:                 "",
			ShardSize:            DefaultShardSize,
		},
	}
}

func parseBoolParam(q url.Values, name string, dest *bool) error {
	if v := q.Get(name); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("parameter %s invalid", name)
		}
		*dest = b
	}
	return nil
}

func parseIntParam(q url.Values, name string, dest *int) error {
	if v := q.Get(name); v != "" {
		i, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("parameter %s invalid", name)
		}
		*dest = i
	}
	return nil
}

// AddParamsFromQuery parses the AddParams object from
// a URL.Query().
func AddParamsFromQuery(query url.Values) (*AddParams, error) {
	params := DefaultAddParams()

	layout := query.Get("layout")
	switch layout {
	case "trickle", "balanced", "":
		// nothing
	default:
		return nil, errors.New("layout parameter invalid")
	}
	params.Layout = layout

	chunker := query.Get("chunker")
	params.Chunker = chunker
	name := query.Get("name")
	params.Name = name

	hashF := query.Get("hash")
	if hashF != "" {
		params.HashFun = hashF
	}

	err := parseBoolParam(query, "recursive", &params.Recursive)
	if err != nil {
		return nil, err
	}

	err = parseBoolParam(query, "raw-leaves", &params.RawLeaves)
	if err != nil {
		return nil, err
	}
	err = parseBoolParam(query, "hidden", &params.Hidden)
	if err != nil {
		return nil, err
	}
	err = parseBoolParam(query, "wrap-with-directory", &params.Wrap)
	if err != nil {
		return nil, err
	}
	err = parseBoolParam(query, "shard", &params.Shard)
	if err != nil {
		return nil, err
	}

	err = parseBoolParam(query, "progress", &params.Progress)
	if err != nil {
		return nil, err
	}

	err = parseIntParam(query, "replication-min", &params.ReplicationFactorMin)
	if err != nil {
		return nil, err
	}
	err = parseIntParam(query, "replication-max", &params.ReplicationFactorMax)
	if err != nil {
		return nil, err
	}

	err = parseIntParam(query, "cid-version", &params.CidVersion)
	if err != nil {
		return nil, err
	}

	if v := query.Get("shard-size"); v != "" {
		shardSize, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return nil, errors.New("parameter shard_size is invalid")
		}
		params.ShardSize = shardSize
	}

	err = parseBoolParam(query, "stream-channels", &params.StreamChannels)
	if err != nil {
		return nil, err
	}

	return params, nil
}

// ToQueryString returns a url query string (key=value&key2=value2&...)
func (p *AddParams) ToQueryString() string {
	query := url.Values{}
	query.Set("replication-min", fmt.Sprintf("%d", p.ReplicationFactorMin))
	query.Set("replication-max", fmt.Sprintf("%d", p.ReplicationFactorMax))
	query.Set("name", p.Name)
	query.Set("shard", fmt.Sprintf("%t", p.Shard))
	query.Set("shard-size", fmt.Sprintf("%d", p.ShardSize))
	query.Set("recursive", fmt.Sprintf("%t", p.Recursive))
	query.Set("layout", p.Layout)
	query.Set("chunker", p.Chunker)
	query.Set("raw-leaves", fmt.Sprintf("%t", p.RawLeaves))
	query.Set("hidden", fmt.Sprintf("%t", p.Hidden))
	query.Set("wrap-with-directory", fmt.Sprintf("%t", p.Wrap))
	query.Set("progress", fmt.Sprintf("%t", p.Progress))
	query.Set("cid-version", fmt.Sprintf("%d", p.CidVersion))
	query.Set("hash", p.HashFun)
	query.Set("stream-channels", fmt.Sprintf("%t", p.StreamChannels))
	return query.Encode()
}

// Equals checks if p equals p2.
func (p *AddParams) Equals(p2 *AddParams) bool {
	return p.ReplicationFactorMin == p2.ReplicationFactorMin &&
		p.ReplicationFactorMax == p2.ReplicationFactorMax &&
		p.Name == p2.Name &&
		p.Recursive == p2.Recursive &&
		p.Shard == p2.Shard &&
		p.ShardSize == p2.ShardSize &&
		p.Layout == p2.Layout &&
		p.Chunker == p2.Chunker &&
		p.RawLeaves == p2.RawLeaves &&
		p.Hidden == p2.Hidden &&
		p.Wrap == p2.Wrap &&
		p.CidVersion == p2.CidVersion &&
		p.HashFun == p2.HashFun &&
		p.StreamChannels == p2.StreamChannels
}
