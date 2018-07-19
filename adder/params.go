package adder

import (
	"errors"
	"net/url"
	"strconv"

	"github.com/ipfs/ipfs-cluster/api"
)

// Params contains all of the configurable parameters needed to specify the
// importing process of a file being added to an ipfs-cluster
type Params struct {
	api.PinOptions

	Layout    string
	Chunker   string
	RawLeaves bool
	Hidden    bool
	Shard     bool
}

// DefaultParams returns a Params object with standard defaults
func DefaultParams() *Params {
	return &Params{
		Layout:    "", // corresponds to balanced layout
		Chunker:   "",
		RawLeaves: false,
		Hidden:    false,
		Shard:     false,
		PinOptions: api.PinOptions{
			ReplicationFactorMin: 0,
			ReplicationFactorMax: 0,
			Name:                 "",
			ShardSize:            100 * 1024 * 1024, // 100 MB
		},
	}
}

// ParamsFromQuery parses the Params object from
// a URL.Query().
func ParamsFromQuery(query url.Values) (*Params, error) {
	params := DefaultParams()

	layout := query.Get("layout")
	switch layout {
	case "trickle":
	case "balanced":
	default:
		return nil, errors.New("parameter trickle invalid")
	}
	params.Layout = layout

	chunker := query.Get("chunker")
	params.Chunker = chunker
	name := query.Get("name")
	params.Name = name

	if v := query.Get("raw"); v != "" {
		raw, err := strconv.ParseBool(v)
		if err != nil {
			return nil, errors.New("parameter raw invalid")
		}
		params.RawLeaves = raw
	}

	if v := query.Get("hidden"); v != "" {
		hidden, err := strconv.ParseBool(v)
		if err != nil {
			return nil, errors.New("parameter hidden invalid")
		}
		params.Hidden = hidden
	}

	if v := query.Get("shard"); v != "" {
		shard, err := strconv.ParseBool(v)
		if err != nil {
			return nil, errors.New("parameter shard invalid")
		}
		params.Shard = shard
	}

	if v := query.Get("repl_min"); v != "" {
		replMin, err := strconv.Atoi(v)
		if err != nil || replMin < -1 {
			return nil, errors.New("parameter repl_min invalid")
		}
		params.ReplicationFactorMin = replMin
	}

	if v := query.Get("repl_max"); v != "" {
		replMax, err := strconv.Atoi(v)
		if err != nil || replMax < -1 {
			return nil, errors.New("parameter repl_max invalid")
		}
		params.ReplicationFactorMax = replMax
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
