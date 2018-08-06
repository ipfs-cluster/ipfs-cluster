package api

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
)

// DefaultShardSize is the shard size for params objects created with DefaultParams().
var DefaultShardSize = uint64(100 * 1024 * 1024) // 100 MB

// AddParams contains all of the configurable parameters needed to specify the
// importing process of a file being added to an ipfs-cluster
type AddParams struct {
	PinOptions

	Layout    string
	Chunker   string
	RawLeaves bool
	Hidden    bool
	Shard     bool
}

// DefaultAddParams returns a AddParams object with standard defaults
func DefaultAddParams() *AddParams {
	return &AddParams{
		Layout:    "", // corresponds to balanced layout
		Chunker:   "size-262144",
		RawLeaves: false,
		Hidden:    false,
		Shard:     false,
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
	case "trickle":
	case "balanced":
	case "":
		// nothing
	default:
		return nil, errors.New("parameter trickle invalid")
	}
	params.Layout = layout

	chunker := query.Get("chunker")
	params.Chunker = chunker
	name := query.Get("name")
	params.Name = name

	err := parseBoolParam(query, "raw", &params.RawLeaves)
	if err != nil {
		return nil, err
	}
	err = parseBoolParam(query, "hidden", &params.Hidden)
	if err != nil {
		return nil, err
	}
	err = parseBoolParam(query, "shard", &params.Shard)
	if err != nil {
		return nil, err
	}
	err = parseIntParam(query, "repl_min", &params.ReplicationFactorMin)
	if err != nil {
		return nil, err
	}
	err = parseIntParam(query, "repl_max", &params.ReplicationFactorMax)
	if err != nil {
		return nil, err
	}

	if v := query.Get("shard_size"); v != "" {
		shardSize, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return nil, errors.New("parameter shard_size is invalid")
		}
		params.ShardSize = shardSize
	}

	return params, nil
}

// ToQueryString returns a url query string (key=value&key2=value2&...)
func (p *AddParams) ToQueryString() string {
	fmtStr := "repl_min=%d&repl_max=%d&name=%s&"
	fmtStr += "shard=%t&shard_size=%d&"
	fmtStr += "layout=%s&chunker=%s&raw=%t&hidden=%t"
	query := fmt.Sprintf(
		fmtStr,
		p.ReplicationFactorMin,
		p.ReplicationFactorMax,
		p.Name,
		p.Shard,
		p.ShardSize,
		p.Layout,
		p.Chunker,
		p.RawLeaves,
		p.Hidden,
	)
	return query
}

// Equals checks if p equals p2.
func (p *AddParams) Equals(p2 *AddParams) bool {
	return p.ReplicationFactorMin == p2.ReplicationFactorMin &&
		p.ReplicationFactorMax == p2.ReplicationFactorMax &&
		p.Name == p2.Name &&
		p.Shard == p2.Shard &&
		p.ShardSize == p2.ShardSize &&
		p.Layout == p2.Layout &&
		p.Chunker == p2.Chunker &&
		p.RawLeaves == p2.RawLeaves &&
		p.Hidden == p2.Hidden
}
