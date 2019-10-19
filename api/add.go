package api

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"

	cid "github.com/ipfs/go-cid"
)

// DefaultShardSize is the shard size for params objects created with DefaultParams().
var DefaultShardSize = uint64(100 * 1024 * 1024) // 100 MB

// AddedOutput carries information for displaying the standard ipfs output
// indicating a node of a file has been added.
type AddedOutput struct {
	Name  string  `json:"name" codec:"n,omitempty"`
	Cid   cid.Cid `json:"cid" codec:"c"`
	Bytes uint64  `json:"bytes,omitempty" codec:"b,omitempty"`
	Size  uint64  `json:"size,omitempty" codec:"s,omitempty"`
}

// AddParams contains all of the configurable parameters needed to specify the
// importing process of a file being added to an ipfs-cluster
type AddParams struct {
	PinOptions

	Local          bool
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
	NoCopy         bool
}

// DefaultAddParams returns a AddParams object with standard defaults
func DefaultAddParams() *AddParams {
	return &AddParams{
		Local:          false,
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
		NoCopy:         false,
		PinOptions: PinOptions{
			ReplicationFactorMin: 0,
			ReplicationFactorMax: 0,
			Name:                 "",
			ShardSize:            DefaultShardSize,
			Metadata:             make(map[string]string),
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

	opts := &PinOptions{}
	err := opts.FromQuery(query)
	if err != nil {
		return nil, err
	}
	params.PinOptions = *opts
	params.PinUpdate = cid.Undef // hardcode as does not make sense for adding

	layout := query.Get("layout")
	switch layout {
	case "trickle", "balanced", "":
		// nothing
	default:
		return nil, errors.New("layout parameter invalid")
	}
	params.Layout = layout

	chunker := query.Get("chunker")
	if chunker != "" {
		params.Chunker = chunker
	}

	hashF := query.Get("hash")
	if hashF != "" {
		params.HashFun = hashF
	}

	err = parseBoolParam(query, "local", &params.Local)
	if err != nil {
		return nil, err
	}

	err = parseBoolParam(query, "recursive", &params.Recursive)
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

	err = parseIntParam(query, "cid-version", &params.CidVersion)
	if err != nil {
		return nil, err
	}

	err = parseBoolParam(query, "stream-channels", &params.StreamChannels)
	if err != nil {
		return nil, err
	}

	err = parseBoolParam(query, "nocopy", &params.NoCopy)
	if err != nil {
		return nil, err
	}

	return params, nil
}

// ToQueryString returns a url query string (key=value&key2=value2&...)
func (p *AddParams) ToQueryString() string {
	pinOptsQuery := p.PinOptions.ToQuery()
	query, _ := url.ParseQuery(pinOptsQuery)
	query.Set("shard", fmt.Sprintf("%t", p.Shard))
	query.Set("local", fmt.Sprintf("%t", p.Local))
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
	query.Set("nocopy", fmt.Sprintf("%t", p.NoCopy))
	return query.Encode()
}

// Equals checks if p equals p2.
func (p *AddParams) Equals(p2 *AddParams) bool {
	return p.PinOptions.Equals(&p2.PinOptions) &&
		p.Local == p2.Local &&
		p.Recursive == p2.Recursive &&
		p.Shard == p2.Shard &&
		p.Layout == p2.Layout &&
		p.Chunker == p2.Chunker &&
		p.RawLeaves == p2.RawLeaves &&
		p.Hidden == p2.Hidden &&
		p.Wrap == p2.Wrap &&
		p.CidVersion == p2.CidVersion &&
		p.HashFun == p2.HashFun &&
		p.StreamChannels == p2.StreamChannels &&
		p.NoCopy == p2.NoCopy
}
