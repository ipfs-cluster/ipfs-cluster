package adder

import (
	"net/url"
	"testing"
)

func TestParamsFromQuery(t *testing.T) {
	qStr := "layout=balanced&chunker=size-262144&name=test&raw=true&hidden=true&shard=true&repl_min=2&repl_max=4&shard_size=1"

	q, err := url.ParseQuery(qStr)
	if err != nil {
		t.Fatal(err)
	}

	p, err := ParamsFromQuery(q)
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
