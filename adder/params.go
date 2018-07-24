package adder

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"

	"github.com/ipfs/ipfs-cluster/api"
)

// DefaultShardSize is the shard size for params objects created with DefaultParams().
var DefaultShardSize = uint64(100 * 1024 * 1024) // 100 MB

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

// ParamsFromQuery parses the Params object from
// a URL.Query().
func ParamsFromQuery(query url.Values) (*Params, error) {
	params := DefaultParams()

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
