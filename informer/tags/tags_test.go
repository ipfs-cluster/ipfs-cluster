package tags

import (
	"context"
	"testing"
)

func Test(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{}
	cfg.Default()
	inf, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer inf.Shutdown(ctx)
	m := inf.GetMetrics(ctx)
	if len(m) != 1 || !m[0].Valid {
		t.Error("metric should be valid")
	}

	inf.config.Tags["x"] = "y"
	m = inf.GetMetrics(ctx)
	if len(m) != 2 {
		t.Error("there should be 2 metrics")
	}
}
