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
			ReplicationFactorMin: -1,
			ReplicationFactorMax: -1,
			Name:                 "",
			ShardSize:            100 * 1024 * 1024, // FIXME
		},
	}
}

// ParamsFromQuery parses the Params object from
// a URL.Query().
// FIXME? Defaults?
func ParamsFromQuery(query url.Values) (*Params, error) {
	layout := query.Get("layout")
	switch layout {
	case "trickle":
	case "balanced":
	default:
		return nil, errors.New("parameter trickle invalid")
	}

	chunker := query.Get("chunker")
	name := query.Get("name")
	raw, err := strconv.ParseBool(query.Get("raw"))
	if err != nil {
		return nil, errors.New("parameter raw invalid")
	}
	hidden, err := strconv.ParseBool(query.Get("hidden"))
	if err != nil {
		return nil, errors.New("parameter hidden invalid")
	}
	shard, err := strconv.ParseBool(query.Get("shard"))
	if err != nil {
		return nil, errors.New("parameter shard invalid")
	}
	replMin, err := strconv.Atoi(query.Get("repl_min"))
	if err != nil || replMin < -1 {
		return nil, errors.New("parameter repl_min invalid")
	}
	replMax, err := strconv.Atoi(query.Get("repl_max"))
	if err != nil || replMax < -1 {
		return nil, errors.New("parameter repl_max invalid")
	}

	shardSize, err := strconv.ParseUint(query.Get("shard_size"), 10, 64)
	if err != nil {
		return nil, errors.New("parameter shard_size is invalid")
	}

	params := &Params{
		Layout:    layout,
		Chunker:   chunker,
		RawLeaves: raw,
		Hidden:    hidden,
		Shard:     shard,
		PinOptions: api.PinOptions{
			ReplicationFactorMin: replMin,
			ReplicationFactorMax: replMax,
			Name:                 name,
			ShardSize:            shardSize,
		},
	}
	return params, nil
}
