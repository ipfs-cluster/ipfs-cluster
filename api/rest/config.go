package rest

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/ipfs/ipfs-cluster/config"

	ma "github.com/multiformats/go-multiaddr"
)

const configKey = "restapi"

// These are the default values for Config
const (
	DefaultListenAddr        = "/ip4/127.0.0.1/tcp/9094"
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

	// Listen parameters for the the Cluster HTTP API component.
	ListenAddr ma.Multiaddr

	// TLS configuration for the HTTP listener
	TLS *tls.Config

	// SSLCertFile is a path to a certificate file used to secure the HTTP
	// API endpoint. Leave empty to use plain HTTP instead.
	pathSSLCertFile string

	// SSLKeyFile is a path to the private key corresponding to the
	// SSLCertFile.
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

	// BasicAuthCreds is a map of username-password pairs
	// which are authorized to use Basic Authentication
	BasicAuthCreds map[string]string
}

type jsonConfig struct {
	ListenMultiaddress string            `json:"listen_multiaddress"`
	SSLCertFile        string            `json:"ssl_cert_file,omitempty"`
	SSLKeyFile         string            `json:"ssl_key_file,omitempty"`
	ReadTimeout        string            `json:"read_timeout"`
	ReadHeaderTimeout  string            `json:"read_header_timeout"`
	WriteTimeout       string            `json:"write_timeout"`
	IdleTimeout        string            `json:"idle_timeout"`
	BasicAuthCreds     map[string]string `json:"basic_auth_credentials"`
}

// ConfigKey returns a human-friendly identifier for this type of
// Config.
func (cfg *Config) ConfigKey() string {
	return configKey
}

// Default initializes this Config with working values.
func (cfg *Config) Default() error {
	listen, _ := ma.NewMultiaddr(DefaultListenAddr)
	cfg.ListenAddr = listen
	cfg.pathSSLCertFile = ""
	cfg.pathSSLKeyFile = ""
	cfg.ReadTimeout = DefaultReadTimeout
	cfg.ReadHeaderTimeout = DefaultReadHeaderTimeout
	cfg.WriteTimeout = DefaultWriteTimeout
	cfg.IdleTimeout = DefaultIdleTimeout
	cfg.BasicAuthCreds = nil

	return nil
}

// Validate makes sure that all fields in this Config have
// working values, at least in appearance.
func (cfg *Config) Validate() error {
	if cfg.ListenAddr == nil {
		return errors.New("restapi.listen_multiaddress not set")
	}

	if cfg.ReadTimeout <= 0 {
		return errors.New("restapi.read_timeout is invalid")
	}

	if cfg.ReadHeaderTimeout <= 0 {
		return errors.New("restapi.read_header_timeout is invalid")
	}

	if cfg.WriteTimeout <= 0 {
		return errors.New("restapi.write_timeout is invalid")
	}

	if cfg.IdleTimeout <= 0 {
		return errors.New("restapi.idle_timeout invalid")
	}

	if cfg.BasicAuthCreds != nil && len(cfg.BasicAuthCreds) == 0 {
		return errors.New("restapi.basic_auth_creds should be null or have at least one entry")
	}

	if (cfg.pathSSLCertFile != "" || cfg.pathSSLKeyFile != "") && cfg.TLS == nil {
		return errors.New("error loading SSL certificate or key")
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

	// A further improvement here below is that only non-zero fields
	// are assigned. In that case, make sure you have Defaulted
	// everything else.
	// cfg.Default()

	listen, err := ma.NewMultiaddr(jcfg.ListenMultiaddress)
	if err != nil {
		err = fmt.Errorf("error parsing listen_multiaddress: %s", err)
		return err
	}

	cfg.ListenAddr = listen
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

	// errors ignored as Validate() below will catch them
	t, _ := time.ParseDuration(jcfg.ReadTimeout)
	cfg.ReadTimeout = t

	t, _ = time.ParseDuration(jcfg.ReadHeaderTimeout)
	cfg.ReadHeaderTimeout = t

	t, _ = time.ParseDuration(jcfg.WriteTimeout)
	cfg.WriteTimeout = t

	t, _ = time.ParseDuration(jcfg.IdleTimeout)
	cfg.IdleTimeout = t

	cfg.BasicAuthCreds = jcfg.BasicAuthCreds

	return cfg.Validate()
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
	jcfg.ListenMultiaddress = cfg.ListenAddr.String()
	jcfg.SSLCertFile = cfg.pathSSLCertFile
	jcfg.SSLKeyFile = cfg.pathSSLKeyFile
	jcfg.ReadTimeout = cfg.ReadTimeout.String()
	jcfg.ReadHeaderTimeout = cfg.ReadHeaderTimeout.String()
	jcfg.WriteTimeout = cfg.WriteTimeout.String()
	jcfg.IdleTimeout = cfg.IdleTimeout.String()
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
