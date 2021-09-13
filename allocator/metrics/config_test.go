package metrics

import (
	"os"
	"testing"
)

var cfgJSON = []byte(`
{
      "allocate_by": ["tag", "disk"]
}
`)

func TestLoadJSON(t *testing.T) {
	cfg := &Config{}
	err := cfg.LoadJSON(cfgJSON)
	if err != nil {
		t.Fatal(err)
	}
}

func TestToJSON(t *testing.T) {
	cfg := &Config{}
	cfg.LoadJSON(cfgJSON)
	newjson, err := cfg.ToJSON()
	if err != nil {
		t.Fatal(err)
	}
	cfg = &Config{}
	err = cfg.LoadJSON(newjson)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.AllocateBy) != 2 {
		t.Error("configuration was lost in serialization/deserialization")
	}
}

func TestDefault(t *testing.T) {
	cfg := &Config{}
	cfg.Default()
	if cfg.Validate() != nil {
		t.Fatal("error validating")
	}

	cfg.AllocateBy = nil
	if cfg.Validate() == nil {
		t.Fatal("expected error validating")
	}

}

func TestApplyEnvVars(t *testing.T) {
	os.Setenv("CLUSTER_METRICSALLOC_ALLOCATEBY", "a,b,c")
	cfg := &Config{}
	cfg.ApplyEnvVars()

	if len(cfg.AllocateBy) != 3 {
		t.Fatal("failed to override allocate_by with env var")
	}
}
