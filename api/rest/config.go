package rest

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/ipfs/ipfs-cluster/config"

	crypto "github.com/libp2p/go-libp2p-crypto"
	peer "github.com/libp2p/go-libp2p-peer"
	ma "github.com/multiformats/go-multiaddr"
)

const configKey = "restapi"

// These are the default values for Config
const (
	DefaultHTTPListenAddr    = "/ip4/127.0.0.1/tcp/9094"
	DefaultReadTimeout       = 30 * time.Second
	DefaultReadHeaderTimeout = 5 * time.Second
	DefaultWriteTimeout      = 60 * time.Second
	DefaultIdleTimeout       = 120 * time.Second
)

// Config is used to intialize the API object and allows to
// customize the behaviour of it. It implements the config.ComponentConfig
// interface.
type Config struct {
	config.Saver

	// Listen address for the HTTP REST API endpoint.
	HTTPListenAddr ma.Multiaddr

	// TLS configuration for the HTTP listener
	TLS *tls.Config

	// pathSSLCertFile is a path to a certificate file used to secure the
	// HTTP API endpoint. We track it so we can write it in the JSON.
	pathSSLCertFile string

	// pathSSLKeyFile is a path to the private key corresponding to the
	// SSLKeyFile. We track it so we can write it in the JSON.
	pathSSLKeyFile string

	// Maximum duration before timing out reading a full request
	ReadTimeout time.Duration

	// Maximum duration before timing out reading the headers of a request
	ReadHeaderTimeout time.Duration

	// Maximum duration before timing out write of the response
	WriteTimeout time.Duration

	// Server-side amount of time a Keep-Alive connection will be
	// kept idle before being reused
	IdleTimeout time.Duration

	// Listen address for the Libp2p REST API endpoint.
	Libp2pListenAddr ma.Multiaddr

	// ID and PrivateKey are used to create a libp2p host if we
	// want the API component to do it (not by default).
	ID         peer.ID
	PrivateKey crypto.PrivKey

	// BasicAuthCreds is a map of username-password pairs
	// which are authorized to use Basic Authentication
	BasicAuthCreds map[string]string
}

type jsonConfig struct {
	ListenMultiaddress     string `json:"listen_multiaddress"` // backwards compat
	HTTPListenMultiaddress string `json:"http_listen_multiaddress"`
	SSLCertFile            string `json:"ssl_cert_file,omitempty"`
	SSLKeyFile             string `json:"ssl_key_file,omitempty"`
	ReadTimeout            string `json:"read_timeout"`
	ReadHeaderTimeout      string `json:"read_header_timeout"`
	WriteTimeout           string `json:"write_timeout"`
	IdleTimeout            string `json:"idle_timeout"`

	Libp2pListenMultiaddress string `json:"libp2p_listen_multiaddress,omitempty"`
	ID                       string `json:"ID,omitempty"`
	PrivateKey               string `json:"PrivateKey,omitempty"`

	BasicAuthCreds map[string]string `json:"basic_auth_credentials"`
}

// ConfigKey returns a human-friendly identifier for this type of
// Config.
func (cfg *Config) ConfigKey() string {
	return configKey
}

// Default initializes this Config with working values.
func (cfg *Config) Default() error {
	// http
	httpListen, _ := ma.NewMultiaddr(DefaultHTTPListenAddr)
	cfg.HTTPListenAddr = httpListen
	cfg.pathSSLCertFile = ""
	cfg.pathSSLKeyFile = ""
	cfg.ReadTimeout = DefaultReadTimeout
	cfg.ReadHeaderTimeout = DefaultReadHeaderTimeout
	cfg.WriteTimeout = DefaultWriteTimeout
	cfg.IdleTimeout = DefaultIdleTimeout

	// libp2p
	cfg.ID = ""
	cfg.PrivateKey = nil
	cfg.Libp2pListenAddr = nil

	// Auth
	cfg.BasicAuthCreds = nil

	return nil
}

// Validate makes sure that all fields in this Config have
// working values, at least in appearance.
func (cfg *Config) Validate() error {
	if cfg.ReadTimeout < 0 {
		return errors.New("restapi.read_timeout is invalid")
	}

	if cfg.ReadHeaderTimeout < 0 {
		return errors.New("restapi.read_header_timeout is invalid")
	}

	if cfg.WriteTimeout < 0 {
		return errors.New("restapi.write_timeout is invalid")
	}

	if cfg.IdleTimeout < 0 {
		return errors.New("restapi.idle_timeout invalid")
	}

	if cfg.BasicAuthCreds != nil && len(cfg.BasicAuthCreds) == 0 {
		return errors.New("restapi.basic_auth_creds should be null or have at least one entry")
	}

	if (cfg.pathSSLCertFile != "" || cfg.pathSSLKeyFile != "") && cfg.TLS == nil {
		return errors.New("error loading SSL certificate or key")
	}

	if cfg.ID != "" || cfg.PrivateKey != nil || cfg.Libp2pListenAddr != nil {
		// if one is set, all should be
		if cfg.ID == "" || cfg.PrivateKey == nil || cfg.Libp2pListenAddr == nil {
			return errors.New("all ID, private_key and libp2p_listen_multiaddress should be set")
		}
		if !cfg.ID.MatchesPrivateKey(cfg.PrivateKey) {
			return errors.New("restapi.ID does not match private_key")
		}
	}

	return nil
}

// LoadJSON parses a raw JSON byte slice created by ToJSON() and sets the
// configuration fields accordingly.
func (cfg *Config) LoadJSON(raw []byte) error {
	jcfg := &jsonConfig{}
	err := json.Unmarshal(raw, jcfg)
	if err != nil {
		logger.Error("Error unmarshaling restapi config")
		return err
	}

	cfg.Default()

	err = cfg.loadHTTPOptions(jcfg)
	if err != nil {
		return err
	}
	err = cfg.loadLibp2pOptions(jcfg)
	if err != nil {
		return err
	}

	// Other options
	cfg.BasicAuthCreds = jcfg.BasicAuthCreds

	return cfg.Validate()
}

func (cfg *Config) loadHTTPOptions(jcfg *jsonConfig) error {
	// Deal with legacy ListenMultiaddress parameter
	httpListen := jcfg.ListenMultiaddress
	if httpListen != "" {
		logger.Warning("restapi.listen_multiaddress has been replaced with http_listen_multiaddress")
	}
	if l := jcfg.HTTPListenMultiaddress; l != "" {
		httpListen = l
	}

	if httpListen != "" {
		httpAddr, err := ma.NewMultiaddr(httpListen)
		if err != nil {
			err = fmt.Errorf("error parsing restapi.http_listen_multiaddress: %s", err)
			return err
		}
		cfg.HTTPListenAddr = httpAddr
	}

	cert := jcfg.SSLCertFile
	key := jcfg.SSLKeyFile
	cfg.pathSSLCertFile = cert
	cfg.pathSSLKeyFile = key

	if cert != "" || key != "" {
		// if one is missing, newTLSConfig will
		// error loudly
		if !filepath.IsAbs(cert) {
			cert = filepath.Join(cfg.BaseDir, cert)
		}
		if !filepath.IsAbs(key) {
			key = filepath.Join(cfg.BaseDir, key)
		}
		logger.Debug(cfg.BaseDir)
		logger.Debug(cert, key)
		tlsCfg, err := newTLSConfig(cert, key)
		if err != nil {
			return err
		}
		cfg.TLS = tlsCfg
	}

	return config.ParseDurations(
		"restapi",
		&config.DurationOpt{jcfg.ReadTimeout, &cfg.ReadTimeout, "read_timeout"},
		&config.DurationOpt{jcfg.ReadHeaderTimeout, &cfg.ReadHeaderTimeout, "read_header_timeout"},
		&config.DurationOpt{jcfg.WriteTimeout, &cfg.WriteTimeout, "write_timeout"},
		&config.DurationOpt{jcfg.IdleTimeout, &cfg.IdleTimeout, "idle_timeout"},
	)
}

func (cfg *Config) loadLibp2pOptions(jcfg *jsonConfig) error {
	if libp2pListen := jcfg.Libp2pListenMultiaddress; libp2pListen != "" {
		libp2pAddr, err := ma.NewMultiaddr(libp2pListen)
		if err != nil {
			err = fmt.Errorf("error parsing restapi.libp2p_listen_multiaddress: %s", err)
			return err
		}
		cfg.Libp2pListenAddr = libp2pAddr
	}

	if jcfg.PrivateKey != "" {
		pkb, err := base64.StdEncoding.DecodeString(jcfg.PrivateKey)
		if err != nil {
			return fmt.Errorf("error decoding restapi.private_key: %s", err)
		}
		pKey, err := crypto.UnmarshalPrivateKey(pkb)
		if err != nil {
			return fmt.Errorf("error parsing restapi.private_key ID: %s", err)
		}
		cfg.PrivateKey = pKey
	}

	if jcfg.ID != "" {
		id, err := peer.IDB58Decode(jcfg.ID)
		if err != nil {
			return fmt.Errorf("error parsing restapi.ID: %s", err)
		}
		cfg.ID = id
	}
	return nil
}

// ToJSON produce a human-friendly JSON representation of the Config
// object.
func (cfg *Config) ToJSON() (raw []byte, err error) {
	// Multiaddress String() may panic
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%s", r)
		}
	}()

	jcfg := &jsonConfig{}
	jcfg.HTTPListenMultiaddress = cfg.HTTPListenAddr.String()
	jcfg.SSLCertFile = cfg.pathSSLCertFile
	jcfg.SSLKeyFile = cfg.pathSSLKeyFile
	jcfg.ReadTimeout = cfg.ReadTimeout.String()
	jcfg.ReadHeaderTimeout = cfg.ReadHeaderTimeout.String()
	jcfg.WriteTimeout = cfg.WriteTimeout.String()
	jcfg.IdleTimeout = cfg.IdleTimeout.String()

	if cfg.ID != "" {
		jcfg.ID = peer.IDB58Encode(cfg.ID)
	}
	if cfg.PrivateKey != nil {
		pkeyBytes, err := cfg.PrivateKey.Bytes()
		if err == nil {
			pKey := base64.StdEncoding.EncodeToString(pkeyBytes)
			jcfg.PrivateKey = pKey
		}
	}
	if cfg.Libp2pListenAddr != nil {
		jcfg.Libp2pListenMultiaddress = cfg.Libp2pListenAddr.String()
	}

	jcfg.BasicAuthCreds = cfg.BasicAuthCreds

	raw, err = config.DefaultJSONMarshal(jcfg)
	return
}

func newTLSConfig(certFile, keyFile string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, errors.New("Error loading TLS certficate/key: " + err.Error())
	}
	// based on https://github.com/denji/golang-tls
	return &tls.Config{
		MinVersion:               tls.VersionTLS12,
		CurvePreferences:         []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
		PreferServerCipherSuites: true,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
			tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_RSA_WITH_AES_256_CBC_SHA,
		},
		Certificates: []tls.Certificate{cert},
	}, nil
}
