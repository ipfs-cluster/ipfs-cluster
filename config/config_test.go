package config

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

var mockJSON = []byte(`{
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
  },
  "datastore": {
    "mock": {
      "a": "b"
    }
  }
}`)

type mockCfg struct {
	Saver
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

func (m *mockCfg) ToDisplayJSON() ([]byte, error) {
	return []byte(`
	{
		"a":"b"
	}
	`), nil
}

func setupConfigManager() *Manager {
	cfg := NewManager()
	mockCfg := &mockCfg{}
	cfg.RegisterComponent(Cluster, mockCfg)
	for _, sect := range SectionTypes() {
		cfg.RegisterComponent(sect, mockCfg)
	}
	return cfg
}

func TestManager_ToJSON(t *testing.T) {
	cfgMgr := setupConfigManager()
	err := cfgMgr.Default()
	if err != nil {
		t.Fatal(err)
	}
	got, err := cfgMgr.ToJSON()
	if err != nil {
		t.Error(err)
	}

	if !bytes.Equal(got, mockJSON) {
		t.Errorf("mismatch between got: %s and want: %s", got, mockJSON)
	}
}

func TestLoadFromHTTPSourceRedirect(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/config", func(w http.ResponseWriter, r *http.Request) {
		json := fmt.Sprintf(`{ "source" : "http://%s/config" }`, r.Host)
		w.Write([]byte(json))
	})
	s := httptest.NewServer(mux)
	defer s.Close()

	cfgMgr := NewManager()
	err := cfgMgr.LoadJSONFromHTTPSource(s.URL + "/config")
	if err != errSourceRedirect {
		t.Fatal("expected errSourceRedirect")
	}
}

func TestLoadFromHTTPSource(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/config", func(w http.ResponseWriter, r *http.Request) {
		w.Write(mockJSON)
	})
	s := httptest.NewServer(mux)
	defer s.Close()

	cfgMgr := setupConfigManager()
	err := cfgMgr.LoadJSONFromHTTPSource(s.URL + "/config")
	if err != nil {
		t.Fatal("unexpected error")
	}

	cfgMgr.Source = ""
	newJSON, err := cfgMgr.ToJSON()
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(newJSON, mockJSON) {
		t.Error("generated json different than loaded")
	}
}

func TestSaveWithSource(t *testing.T) {
	cfgMgr := setupConfigManager()
	cfgMgr.Default()
	cfgMgr.Source = "http://a.b.c"
	newJSON, err := cfgMgr.ToJSON()
	if err != nil {
		t.Fatal(err)
	}

	expected := []byte(`{
  "source": "http://a.b.c"
}`)

	if !bytes.Equal(newJSON, expected) {
		t.Error("should have generated a source-only json")
	}
}

func TestDefaultJSONMarshalWithoutHiddenFields(t *testing.T) {
	type s struct {
		A string `json:"a_key"`
		B string `json:"b_key" hidden:"true"`
	}
	cfg := s{
		A: "hi",
		B: "there",
	}

	expected := `{
  "a_key": "hi",
  "b_key": "XXX_hidden_XXX"
}`

	res, err := DisplayJSON(&cfg)
	if err != nil {
		t.Fatal(err)
	}

	if string(res) != expected {
		t.Error("result does not match expected")
		t.Error(string(res))
	}
}
