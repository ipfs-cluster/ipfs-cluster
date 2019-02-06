// Package config provides interfaces and utilities for different Cluster
// components to register, read, write and validate configuration sections
// stored in a central configuration file.
package config_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	ipfscluster "github.com/ipfs/ipfs-cluster"
	"github.com/ipfs/ipfs-cluster/api/ipfsproxy"
	"github.com/ipfs/ipfs-cluster/api/rest"
	"github.com/ipfs/ipfs-cluster/config"
	"github.com/ipfs/ipfs-cluster/pintracker/maptracker"
)

func setupConfigManager() *config.Manager {
	cfg := config.NewManager()
	clusterCfg := &ipfscluster.Config{}
	apiCfg := &rest.Config{}
	ipfsproxyCfg := &ipfsproxy.Config{}
	maptrackerCfg := &maptracker.Config{}
	cfg.RegisterComponent(config.Cluster, clusterCfg)
	cfg.RegisterComponent(config.API, apiCfg)
	cfg.RegisterComponent(config.API, ipfsproxyCfg)
	cfg.RegisterComponent(config.PinTracker, maptrackerCfg)
	return cfg
}

func TestManager_ToJSON(t *testing.T) {
	var want1 bytes.Buffer
	json.Compact(&want1, []byte(`{
"api": {
"ipfsproxy": {
"listen_multiaddress": "/ip4/127.0.0.1/tcp/9095",
"node_multiaddress": "/ip4/127.0.0.1/tcp/5001",
"read_timeout": "0s",
"read_header_timeout": "5s",
"write_timeout": "0s",
"idle_timeout": "1m0s",
"extract_headers_path": "/api/v0/version",
"extract_headers_ttl": "5m0s"
},
"restapi": {
"http_listen_multiaddress": "/ip4/127.0.0.1/tcp/9094",
"read_timeout": "0s",
"read_header_timeout": "5s",
"write_timeout": "0s",
"idle_timeout": "2m0s",
"basic_auth_credentials": null,
"headers": {},
"cors_allowed_origins": [
"*"
],
"cors_allowed_methods": [
"GET"
],
"cors_allowed_headers": [],
"cors_exposed_headers": [
"Content-Type",
"X-Stream-Output",
"X-Chunked-Output",
"X-Content-Length"
],
"cors_allow_credentials": true,
"cors_max_age": "0s"
}
},
"pin_tracker": {
"maptracker": {
"max_pin_queue_size": 50000,
"concurrent_pins": 10
}
}
}`))
	tests := []struct {
		name    string
		want    []byte
		wantErr bool
	}{
		{
			"basic default config",
			want1.Bytes(),
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfgMgr := setupConfigManager()
			err := cfgMgr.Default()
			if err != nil {
				t.Fatal(err)
			}
			got, err := cfgMgr.ToJSON()
			if (err != nil) != tt.wantErr {
				t.Errorf("Manager.ToJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Extract non-cluster config sections as cluster section just with each run.

			// api section
			gotapi, err := extractJsonSection("api", got)
			if err != nil {
				t.Fatal(err)
			}
			wantapi, err := extractJsonSection("api", tt.want)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(gotapi, wantapi) {
				t.Errorf("api section not the same: got: %s want: %s", gotapi, wantapi)
			}

			// pintracker section
			gotpintracker, err := extractJsonSection("pin_tracker", got)
			if err != nil {
				t.Fatal(err)
			}
			wantpintracker, err := extractJsonSection("pin_tracker", tt.want)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(gotpintracker, wantpintracker) {
				t.Errorf("api section not the same: got: %s want: %s", gotpintracker, wantpintracker)
			}
		})
	}
}

func extractJsonSection(name string, rawSection []byte) ([]byte, error) {
	var section map[string]interface{}
	json.Unmarshal(rawSection, &section)
	jsection, err := json.Marshal(section[name].(map[string]interface{}))
	if err != nil {
		return nil, err
	}
	return jsection, nil
}
