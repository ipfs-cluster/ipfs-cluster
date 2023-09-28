package config

import (
	"encoding/json"
	"os"
	"testing"
)

var identityTestJSON = []byte(`{
    "id": "12D3KooWCLKKDpG1EndPjLYzsqzoqEUBwF2CNyCVdvMVci2x1ppS",
    "private_key": "CAESQOlnuLttGxDhIak0xsgEoHpQEHYrxOA5cCaJ1rFDP8hCJWOSP00t7dIw++QWdEKL9JJaOWzQD414N5tvsnATICs=",
    "ipfs_id": "12D3KooWLM4CwdZHVJQzVJF4PeQGJruWF37dZGzUTcKuyBhfC5SH",
    "ipfs_private_key": "CAESQFBl0Fpe98WZnNkxsqmVro+gpaKDvAqAdQoNuL4j+E7hnHGQhV0X7A6UNVQctokiXib8aJ89YQ2U71nhY1yJqeI=",
    "ipfs_private_network_secret": "f34099216870b8757e7bccda047c492bf2af8315a7ca6e236dca1eaadc50c12f"
}`)

var (
	ID                       = "12D3KooWCLKKDpG1EndPjLYzsqzoqEUBwF2CNyCVdvMVci2x1ppS"
	PrivateKey               = "CAESQOlnuLttGxDhIak0xsgEoHpQEHYrxOA5cCaJ1rFDP8hCJWOSP00t7dIw++QWdEKL9JJaOWzQD414N5tvsnATICs="
	IPFSID                   = "12D3KooWLM4CwdZHVJQzVJF4PeQGJruWF37dZGzUTcKuyBhfC5SH"
	IPFSPrivateKey           = "CAESQFBl0Fpe98WZnNkxsqmVro+gpaKDvAqAdQoNuL4j+E7hnHGQhV0X7A6UNVQctokiXib8aJ89YQ2U71nhY1yJqeI="
	IPFSPrivateNetworkSecret = "f34099216870b8757e7bccda047c492bf2af8315a7ca6e236dca1eaadc50c12f"
)

func TestLoadJSON(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		ident := &Identity{}
		err := ident.LoadJSON(identityTestJSON)
		if err != nil {
			t.Fatal(err)
		}
	})

	loadJSON := func(t *testing.T, f func(j *identityJSON)) (*Identity, error) {
		ident := &Identity{}
		j := &identityJSON{}
		json.Unmarshal(identityTestJSON, j)
		f(j)
		tst, err := json.Marshal(j)
		if err != nil {
			return ident, err
		}
		err = ident.LoadJSON(tst)
		if err != nil {
			return ident, err
		}
		return ident, nil
	}

	t.Run("bad id", func(t *testing.T) {
		_, err := loadJSON(t, func(j *identityJSON) { j.ID = "abc" })
		if err == nil {
			t.Error("expected error decoding ID")
		}
	})

	t.Run("bad private key", func(t *testing.T) {
		_, err := loadJSON(t, func(j *identityJSON) { j.PrivateKey = "abc" })
		if err == nil {
			t.Error("expected error parsing private key")
		}
	})

	t.Run("empty ipfs json", func(t *testing.T) {
		_, err := loadJSON(t, func(j *identityJSON) {
			j.IPFSID = ""
			j.IPFSPrivateKey = ""
			j.IPFSPrivateNetworkSecret = ""
		})
		if err != nil {
			t.Error("json configurations without IPFS keys should parse fine")
		}
	})

	t.Run("secret set without ipfs_id", func(t *testing.T) {
		_, err := loadJSON(t, func(j *identityJSON) {
			j.IPFSID = ""
			j.IPFSPrivateKey = ""
		})
		if err == nil {
			t.Error("expected validation error due to missing ipfs key")
		}
		t.Log(err)
	})

	t.Run("secret wrong length", func(t *testing.T) {
		_, err := loadJSON(t, func(j *identityJSON) {
			j.IPFSPrivateNetworkSecret = "f3409921"
		})
		if err == nil {
			t.Error("expected validation error due to secret wrong length")
		}
		t.Log(err)
	})
}

func TestToJSON(t *testing.T) {
	ident := &Identity{}
	err := ident.LoadJSON(identityTestJSON)
	if err != nil {
		t.Fatal(err)
	}
	newjson, err := ident.ToJSON()
	if err != nil {
		t.Fatal(err)
	}
	ident2 := &Identity{}
	err = ident2.LoadJSON(newjson)
	if err != nil {
		t.Fatal(err)
	}

	if !ident.Equals(ident2) {
		t.Error("did not load to the same identity")
	}
}

func TestApplyEnvVars(t *testing.T) {
	os.Setenv("CLUSTER_ID", ID)
	os.Setenv("CLUSTER_PRIVATEKEY", PrivateKey)
	os.Setenv("CLUSTER_IPFSID", IPFSID)
	os.Setenv("CLUSTER_IPFSPRIVATEKEY", IPFSPrivateKey)
	os.Setenv("CLUSTER_IPFSPRIVATENETWORKSECRET", IPFSPrivateNetworkSecret)

	ident, err := NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	err = ident.ApplyEnvVars()
	if err != nil {
		t.Fatal(err)
	}

	ident2 := &Identity{}
	err = ident2.LoadJSON(identityTestJSON)
	if err != nil {
		t.Fatal(err)
	}

	if !ident.Equals(ident2) {
		t.Error("failed to override identity with env var")
	}
}

func TestValidate(t *testing.T) {
	ident := &Identity{}

	if ident.Validate() == nil {
		t.Fatal("expected error validating")
	}

	ident, err := NewIdentity()
	if err != nil {
		t.Fatal(err)
	}

	if ident.Validate() != nil {
		t.Error("expected to validate without error")
	}

	ident.ID = ""
	if ident.Validate() == nil {
		t.Fatal("expected error validating")
	}

}
