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
	Error
	Name  string
	Hash  string `json:",omitempty"`
	Bytes int64  `json:",omitempty"`
	Size  string `json:",omitempty"`
}

// AddParams contains all of the configurable parameters needed to specify the
// importing process of a file being added to an ipfs-cluster
type AddParams struct {
	PinOptions

	Recursive  bool
	Layout     string
	Chunker    string
	RawLeaves  bool
	Hidden     bool
	Wrap       bool
	Shard      bool
	Progress   bool
	CidVersion int
	HashFun    string
}

// DefaultAddParams returns a AddParams object with standard defaults
func DefaultAddParams() *AddParams {
	return &AddParams{
		Recursive:  false,
		Layout:     "", // corresponds to balanced layout
		Chunker:    "size-262144",
		RawLeaves:  false,
		Hidden:     false,
		Wrap:       false,
		Shard:      false,
		Progress:   false,
		CidVersion: 0,
		HashFun:    "sha2-256",
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
		return nil, errors.New("parameter trickle invalid")
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

	return params, nil
}

// ToQueryString returns a url query string (key=value&key2=value2&...)
func (p *AddParams) ToQueryString() string {
	fmtStr := "replication-min=%d&replication-max=%d&name=%s&"
	fmtStr += "shard=%t&shard-size=%d&recursive=%t&"
	fmtStr += "layout=%s&chunker=%s&raw-leaves=%t&hidden=%t&"
	fmtStr += "wrap-with-directory=%t&progress=%t&"
	fmtStr += "cid-version=%d&hash=%s"
	query := fmt.Sprintf(
		fmtStr,
		p.ReplicationFactorMin,
		p.ReplicationFactorMax,
		p.Name,
		p.Shard,
		p.ShardSize,
		p.Recursive,
		p.Layout,
		p.Chunker,
		p.RawLeaves,
		p.Hidden,
		p.Wrap,
		p.Progress,
		p.CidVersion,
		p.HashFun,
	)
	return query
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
		p.HashFun == p2.HashFun
}
