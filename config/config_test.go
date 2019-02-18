// Package config provides interfaces and utilities for different Cluster
// components to register, read, write and validate configuration sections
// stored in a central configuration file.
package config_test

import (
	"bytes"
	"testing"

	"github.com/ipfs/ipfs-cluster/config"
)

type mockCfg struct {
	config.Saver
}

func (m *mockCfg) ConfigKey() string {
	return "mock"
}

func (m *mockCfg) LoadJSON([]byte) error {
	return nil
}

func (m *mockCfg) ToJSON() ([]byte, error) {
	return []byte(`{"a":"b"}`), nil
}

func (m *mockCfg) Default() error {
	return nil
}

func (m *mockCfg) ApplyEnvVars() error {
	return nil
}

func (m *mockCfg) Validate() error {
	return nil
}

func setupConfigManager() *config.Manager {
	cfg := config.NewManager()
	mockCfg := &mockCfg{}
	cfg.RegisterComponent(config.Cluster, mockCfg)
	for _, sect := range config.SectionTypes() {
		cfg.RegisterComponent(sect, mockCfg)
	}
	return cfg
}

func TestManager_ToJSON(t *testing.T) {
	want := []byte(`{
  "cluster": {
    "a": "b"
  },
  "consensus": {
    "mock": {
      "a": "b"
    }
  },
  "api": {
    "mock": {
      "a": "b"
    }
  },
  "ipfs_connector": {
    "mock": {
      "a": "b"
    }
  },
  "state": {
    "mock": {
      "a": "b"
    }
  },
  "pin_tracker": {
    "mock": {
      "a": "b"
    }
  },
  "monitor": {
    "mock": {
      "a": "b"
    }
  },
  "allocator": {
    "mock": {
      "a": "b"
    }
  },
  "informer": {
    "mock": {
      "a": "b"
    }
  },
  "observations": {
    "mock": {
      "a": "b"
    }
  }
}`)
	cfgMgr := setupConfigManager()
	err := cfgMgr.Default()
	if err != nil {
		t.Fatal(err)
	}
	got, err := cfgMgr.ToJSON()
	if err != nil {
		t.Error(err)
	}

	if !bytes.Equal(got, want) {
		t.Errorf("mismatch between got: %s and want: %s", got, want)
	}
}
