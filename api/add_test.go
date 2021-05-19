package api

import (
	"net/url"
	"testing"
)

func TestAddParams_FromQuery(t *testing.T) {
	qStr := "layout=balanced&chunker=size-262144&name=test&raw-leaves=true&hidden=true&shard=true&replication-min=2&replication-max=4&shard-size=1"

	q, err := url.ParseQuery(qStr)
	if err != nil {
		t.Fatal(err)
	}

	p, err := AddParamsFromQuery(q)
	if err != nil {
		t.Fatal(err)
	}
	if p.Layout != "balanced" ||
		p.Chunker != "size-262144" ||
		p.Name != "test" ||
		!p.RawLeaves || !p.Hidden || !p.Shard ||
		p.ReplicationFactorMin != 2 ||
		p.ReplicationFactorMax != 4 ||
		p.ShardSize != 1 {
		t.Fatal("did not parse the query correctly")
	}
}

func TestAddParams_FromQueryRawLeaves(t *testing.T) {
	qStr := "cid-version=1"

	q, err := url.ParseQuery(qStr)
	if err != nil {
		t.Fatal(err)
	}

	p, err := AddParamsFromQuery(q)
	if err != nil {
		t.Fatal(err)
	}
	if !p.RawLeaves {
		t.Error("RawLeaves should be true with cid-version=1")
	}

	qStr = "cid-version=1&raw-leaves=false"

	q, err = url.ParseQuery(qStr)
	if err != nil {
		t.Fatal(err)
	}

	p, err = AddParamsFromQuery(q)
	if err != nil {
		t.Fatal(err)
	}
	if p.RawLeaves {
		t.Error("RawLeaves should be false when explicitally set")
	}

	qStr = "cid-version=0&raw-leaves=true"

	q, err = url.ParseQuery(qStr)
	if err != nil {
		t.Fatal(err)
	}

	p, err = AddParamsFromQuery(q)
	if err != nil {
		t.Fatal(err)
	}
	if !p.RawLeaves {
		t.Error("RawLeaves should be true when explicitly set")
	}
}

func TestAddParams_ToQueryString(t *testing.T) {
	p := DefaultAddParams()
	p.ReplicationFactorMin = 3
	p.ReplicationFactorMax = 6
	p.Name = "something"
	p.RawLeaves = true
	p.ShardSize = 1020
	qstr, err := p.ToQueryString()
	if err != nil {
		t.Fatal(err)
	}
	q, err := url.ParseQuery(qstr)
	if err != nil {
		t.Fatal(err)
	}

	p2, err := AddParamsFromQuery(q)
	if err != nil {
		t.Fatal(err)
	}

	if !p.Equals(p2) {
		t.Error("generated and parsed params should be equal")
	}
}
