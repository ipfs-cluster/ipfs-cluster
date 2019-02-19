package observations

import (
	"os"
	"testing"
)

func TestApplyEnvVars(t *testing.T) {
	os.Setenv("CLUSTER_METRICS_ENABLESTATS", "true")
	mcfg := &MetricsConfig{}
	mcfg.Default()
	mcfg.ApplyEnvVars()

	if !mcfg.EnableStats {
		t.Fatal("failed to override enable_stats with env var")
	}

	os.Setenv("CLUSTER_TRACING_ENABLETRACING", "true")
	tcfg := &TracingConfig{}
	tcfg.Default()
	tcfg.ApplyEnvVars()

	if !tcfg.EnableTracing {
		t.Fatal("failed to override enable_tracing with env var")
	}
}
