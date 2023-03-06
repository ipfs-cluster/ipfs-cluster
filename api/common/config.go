package common

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	logging "github.com/ipfs/go-log/v2"
	crypto "github.com/libp2p/go-libp2p/core/crypto"
	peer "github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/kelseyhightower/envconfig"
	"github.com/rs/cors"

	"github.com/ipfs-cluster/ipfs-cluster/config"
)

const minMaxHeaderBytes = 4096

const defaultMaxHeaderBytes = minMaxHeaderBytes

// Config provides common API configuration values and allows to customize its
// behavior. It implements most of the config.ComponentConfig interface
// (except the Default() and ConfigKey() methods). Config should be embedded
// in a Config object that implements the missing methods and sets the
// meta options.
type Config struct {
	config.Saver

	// These are meta-options and should be set by actual Config
	// implementations as early as possible.
	DefaultFunc   func(*Config) error
	ConfigKey     string
	EnvConfigKey  string
	Logger        *logging.ZapEventLogger
	RequestLogger *logging.ZapEventLogger
	APIErrorFunc  func(err error, status int) error

	// Listen address for the HTTP REST API endpoint.
	HTTPListenAddr []ma.Multiaddr

	// TLS configuration for the HTTP listener
	TLS *tls.Config

	// pathSSLCertFile is a path to a certificate file used to secure the
	// HTTP API endpoint. We track it so we can write it in the JSON.
	PathSSLCertFile string

	// pathSSLKeyFile is a path to the private key corresponding to the
	// SSLKeyFile. We track it so we can write it in the JSON.
	PathSSLKeyFile string

	// Maximum duration before timing out reading a full request
	ReadTimeout time.Duration

	// Maximum duration before timing out reading the headers of a request
	ReadHeaderTimeout time.Duration

	// Maximum duration before timing out write of the response
	WriteTimeout time.Duration

	// Server-side amount of time a Keep-Alive connection will be
	// kept idle before being reused
	IdleTimeout time.Duration

	// Maximum cumulative size of HTTP request headers in bytes
	// accepted by the server
	MaxHeaderBytes int

	// Listen address for the Libp2p REST API endpoint.
	Libp2pListenAddr []ma.Multiaddr

	// ID and PrivateKey are used to create a libp2p host if we
	// want the API component to do it (not by default).
	ID         peer.ID
	PrivateKey crypto.PrivKey

	// BasicAuthCredentials is a map of username-password pairs
	// which are authorized to use Basic Authentication
	BasicAuthCredentials map[string]string

	// HTTPLogFile is path of the file that would save HTTP API logs. If this
	// path is empty, HTTP logs would be sent to standard output. This path
	// should either be absolute or relative to cluster base directory. Its
	// default value is empty.
	HTTPLogFile string

	// Headers provides customization for the headers returned
	// by the API on existing routes.
	Headers map[string][]string

	// CORS header management
	CORSAllowedOrigins   []string
	CORSAllowedMethods   []string
	CORSAllowedHeaders   []string
	CORSExposedHeaders   []string
	CORSAllowCredentials bool
	CORSMaxAge           time.Duration

	// Tracing flag used to skip tracing specific paths when not enabled.
	Tracing bool
}

type jsonConfig struct {
	HTTPListenMultiaddress config.Strings `json:"http_listen_multiaddress"`
	SSLCertFile            string         `json:"ssl_cert_file,omitempty"`
	SSLKeyFile             string         `json:"ssl_key_file,omitempty"`
	ReadTimeout            string         `json:"read_timeout"`
	ReadHeaderTimeout      string         `json:"read_header_timeout"`
	WriteTimeout           string         `json:"write_timeout"`
	IdleTimeout            string         `json:"idle_timeout"`
	MaxHeaderBytes         int            `json:"max_header_bytes"`

	Libp2pListenMultiaddress config.Strings `json:"libp2p_listen_multiaddress,omitempty"`
	ID                       string         `json:"id,omitempty"`
	PrivateKey               string         `json:"private_key,omitempty" hidden:"true"`

	BasicAuthCredentials map[string]string   `json:"basic_auth_credentials"  hidden:"true"`
	HTTPLogFile          string              `json:"http_log_file"`
	Headers              map[string][]string `json:"headers"`

	CORSAllowedOrigins   []string `json:"cors_allowed_origins"`
	CORSAllowedMethods   []string `json:"cors_allowed_methods"`
	CORSAllowedHeaders   []string `json:"cors_allowed_headers"`
	CORSExposedHeaders   []string `json:"cors_exposed_headers"`
	CORSAllowCredentials bool     `json:"cors_allow_credentials"`
	CORSMaxAge           string   `json:"cors_max_age"`
}

// GetHTTPLogPath gets full path of the file where http logs should be
// saved.
func (cfg *Config) GetHTTPLogPath() string {
	if filepath.IsAbs(cfg.HTTPLogFile) {
		return cfg.HTTPLogFile
	}

	if cfg.BaseDir == "" {
		return ""
	}

	return filepath.Join(cfg.BaseDir, cfg.HTTPLogFile)
}

// ApplyEnvVars fills in any Config fields found as environment variables.
func (cfg *Config) ApplyEnvVars() error {
	jcfg, err := cfg.toJSONConfig()
	if err != nil {
		return err
	}

	err = envconfig.Process(cfg.EnvConfigKey, jcfg)
	if err != nil {
		return err
	}
	return cfg.applyJSONConfig(jcfg)
}

// Validate makes sure that all fields in this Config have
// working values, at least in appearance.
func (cfg *Config) Validate() error {
	if cfg.Logger == nil || cfg.RequestLogger == nil {
		return errors.New("config loggers not set")
	}

	switch {
	case cfg.ReadTimeout < 0:
		return errors.New(cfg.ConfigKey + ".read_timeout is invalid")
	case cfg.ReadHeaderTimeout < 0:
		return errors.New(cfg.ConfigKey + ".read_header_timeout is invalid")
	case cfg.WriteTimeout < 0:
		return errors.New(cfg.ConfigKey + ".write_timeout is invalid")
	case cfg.IdleTimeout < 0:
		return errors.New(cfg.ConfigKey + ".idle_timeout invalid")
	case cfg.MaxHeaderBytes < minMaxHeaderBytes:
		return fmt.Errorf(cfg.ConfigKey+".max_header_bytes must be not less then %d", minMaxHeaderBytes)
	case cfg.BasicAuthCredentials != nil && len(cfg.BasicAuthCredentials) == 0:
		return errors.New(cfg.ConfigKey + ".basic_auth_creds should be null or have at least one entry")
	case (cfg.PathSSLCertFile != "" || cfg.PathSSLKeyFile != "") && cfg.TLS == nil:
		return errors.New(cfg.ConfigKey + ": missing TLS configuration")
	case (cfg.CORSMaxAge < 0):
		return errors.New(cfg.ConfigKey + ".cors_max_age is invalid")
	}

	return cfg.validateLibp2p()
}

func (cfg *Config) validateLibp2p() error {
	if cfg.ID != "" || cfg.PrivateKey != nil || len(cfg.Libp2pListenAddr) > 0 {
		// if one is set, all should be
		if cfg.ID == "" || cfg.PrivateKey == nil || len(cfg.Libp2pListenAddr) == 0 {
			return errors.New("all ID, private_key and libp2p_listen_multiaddress should be set")
		}
		if !cfg.ID.MatchesPrivateKey(cfg.PrivateKey) {
			return errors.New(cfg.ConfigKey + ".ID does not match private_key")
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
		cfg.Logger.Error(cfg.ConfigKey + ": error unmarshaling config")
		return err
	}

	if cfg.DefaultFunc == nil {
		return errors.New("default config generation not set. This is a bug")
	}
	cfg.DefaultFunc(cfg)

	return cfg.applyJSONConfig(jcfg)
}

func (cfg *Config) applyJSONConfig(jcfg *jsonConfig) error {
	err := cfg.loadHTTPOptions(jcfg)
	if err != nil {
		return err
	}

	err = cfg.loadLibp2pOptions(jcfg)
	if err != nil {
		return err
	}

	// Other options
	cfg.BasicAuthCredentials = jcfg.BasicAuthCredentials
	cfg.HTTPLogFile = jcfg.HTTPLogFile
	cfg.Headers = jcfg.Headers

	return cfg.Validate()
}

func (cfg *Config) loadHTTPOptions(jcfg *jsonConfig) error {
	if addresses := jcfg.HTTPListenMultiaddress; len(addresses) > 0 {
		cfg.HTTPListenAddr = make([]ma.Multiaddr, 0, len(addresses))
		for _, addr := range addresses {
			httpAddr, err := ma.NewMultiaddr(addr)
			if err != nil {
				err = fmt.Errorf("error parsing %s.http_listen_multiaddress: %s", cfg.ConfigKey, err)
				return err
			}
			cfg.HTTPListenAddr = append(cfg.HTTPListenAddr, httpAddr)
		}
	}

	err := cfg.tlsOptions(jcfg)
	if err != nil {
		return err
	}

	if jcfg.MaxHeaderBytes == 0 {
		cfg.MaxHeaderBytes = defaultMaxHeaderBytes
	} else {
		cfg.MaxHeaderBytes = jcfg.MaxHeaderBytes
	}

	// CORS
	cfg.CORSAllowedOrigins = jcfg.CORSAllowedOrigins
	cfg.CORSAllowedMethods = jcfg.CORSAllowedMethods
	cfg.CORSAllowedHeaders = jcfg.CORSAllowedHeaders
	cfg.CORSExposedHeaders = jcfg.CORSExposedHeaders
	cfg.CORSAllowCredentials = jcfg.CORSAllowCredentials
	if jcfg.CORSMaxAge == "" { // compatibility
		jcfg.CORSMaxAge = "0s"
	}

	return config.ParseDurations(
		cfg.ConfigKey,
		&config.DurationOpt{Duration: jcfg.ReadTimeout, Dst: &cfg.ReadTimeout, Name: "read_timeout"},
		&config.DurationOpt{Duration: jcfg.ReadHeaderTimeout, Dst: &cfg.ReadHeaderTimeout, Name: "read_header_timeout"},
		&config.DurationOpt{Duration: jcfg.WriteTimeout, Dst: &cfg.WriteTimeout, Name: "write_timeout"},
		&config.DurationOpt{Duration: jcfg.IdleTimeout, Dst: &cfg.IdleTimeout, Name: "idle_timeout"},
		&config.DurationOpt{Duration: jcfg.CORSMaxAge, Dst: &cfg.CORSMaxAge, Name: "cors_max_age"},
	)
}

func (cfg *Config) tlsOptions(jcfg *jsonConfig) error {
	cert := jcfg.SSLCertFile
	key := jcfg.SSLKeyFile

	if cert+key == "" {
		return nil
	}

	cfg.PathSSLCertFile = cert
	cfg.PathSSLKeyFile = key

	if !filepath.IsAbs(cert) {
		cert = filepath.Join(cfg.BaseDir, cert)
	}

	if !filepath.IsAbs(key) {
		key = filepath.Join(cfg.BaseDir, key)
	}

	cfg.Logger.Debug("baseDir: ", cfg.BaseDir)
	cfg.Logger.Debug("cert path: ", cert)
	cfg.Logger.Debug("key path: ", key)

	tlsCfg, err := newTLSConfig(cert, key)
	if err != nil {
		return err
	}
	cfg.TLS = tlsCfg
	return nil
}

func (cfg *Config) loadLibp2pOptions(jcfg *jsonConfig) error {
	if addresses := jcfg.Libp2pListenMultiaddress; len(addresses) > 0 {
		cfg.Libp2pListenAddr = make([]ma.Multiaddr, 0, len(addresses))
		for _, addr := range addresses {
			libp2pAddr, err := ma.NewMultiaddr(addr)
			if err != nil {
				err = fmt.Errorf("error parsing %s.libp2p_listen_multiaddress: %s", cfg.ConfigKey, err)
				return err
			}
			cfg.Libp2pListenAddr = append(cfg.Libp2pListenAddr, libp2pAddr)
		}
	}

	if jcfg.PrivateKey != "" {
		pkb, err := base64.StdEncoding.DecodeString(jcfg.PrivateKey)
		if err != nil {
			return fmt.Errorf("error decoding %s.private_key: %s", cfg.ConfigKey, err)
		}
		pKey, err := crypto.UnmarshalPrivateKey(pkb)
		if err != nil {
			return fmt.Errorf("error parsing %s.private_key ID: %s", cfg.ConfigKey, err)
		}
		cfg.PrivateKey = pKey
	}

	if jcfg.ID != "" {
		id, err := peer.Decode(jcfg.ID)
		if err != nil {
			return fmt.Errorf("error parsing %s.ID: %s", cfg.ConfigKey, err)
		}
		cfg.ID = id
	}
	return nil
}

// ToJSON produce a human-friendly JSON representation of the Config
// object.
func (cfg *Config) ToJSON() (raw []byte, err error) {
	jcfg, err := cfg.toJSONConfig()
	if err != nil {
		return
	}

	raw, err = config.DefaultJSONMarshal(jcfg)
	return
}

func (cfg *Config) toJSONConfig() (jcfg *jsonConfig, err error) {
	// Multiaddress String() may panic
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%s", r)
		}
	}()

	httpAddresses := make([]string, 0, len(cfg.HTTPListenAddr))
	for _, addr := range cfg.HTTPListenAddr {
		httpAddresses = append(httpAddresses, addr.String())
	}

	libp2pAddresses := make([]string, 0, len(cfg.Libp2pListenAddr))
	for _, addr := range cfg.Libp2pListenAddr {
		libp2pAddresses = append(libp2pAddresses, addr.String())
	}

	jcfg = &jsonConfig{
		HTTPListenMultiaddress: httpAddresses,
		SSLCertFile:            cfg.PathSSLCertFile,
		SSLKeyFile:             cfg.PathSSLKeyFile,
		ReadTimeout:            cfg.ReadTimeout.String(),
		ReadHeaderTimeout:      cfg.ReadHeaderTimeout.String(),
		WriteTimeout:           cfg.WriteTimeout.String(),
		IdleTimeout:            cfg.IdleTimeout.String(),
		MaxHeaderBytes:         cfg.MaxHeaderBytes,
		BasicAuthCredentials:   cfg.BasicAuthCredentials,
		HTTPLogFile:            cfg.HTTPLogFile,
		Headers:                cfg.Headers,
		CORSAllowedOrigins:     cfg.CORSAllowedOrigins,
		CORSAllowedMethods:     cfg.CORSAllowedMethods,
		CORSAllowedHeaders:     cfg.CORSAllowedHeaders,
		CORSExposedHeaders:     cfg.CORSExposedHeaders,
		CORSAllowCredentials:   cfg.CORSAllowCredentials,
		CORSMaxAge:             cfg.CORSMaxAge.String(),
	}

	if cfg.ID != "" {
		jcfg.ID = cfg.ID.String()
	}
	if cfg.PrivateKey != nil {
		pkeyBytes, err := crypto.MarshalPrivateKey(cfg.PrivateKey)
		if err == nil {
			pKey := base64.StdEncoding.EncodeToString(pkeyBytes)
			jcfg.PrivateKey = pKey
		}
	}
	if len(libp2pAddresses) > 0 {
		jcfg.Libp2pListenMultiaddress = libp2pAddresses
	}

	return
}

// CorsOptions returns cors.Options setup from the configured values.
func (cfg *Config) CorsOptions() *cors.Options {
	maxAgeSeconds := int(cfg.CORSMaxAge / time.Second)

	return &cors.Options{
		AllowedOrigins:   cfg.CORSAllowedOrigins,
		AllowedMethods:   cfg.CORSAllowedMethods,
		AllowedHeaders:   cfg.CORSAllowedHeaders,
		ExposedHeaders:   cfg.CORSExposedHeaders,
		AllowCredentials: cfg.CORSAllowCredentials,
		MaxAge:           maxAgeSeconds,
		Debug:            false,
	}
}

// ToDisplayJSON returns JSON config as a string.
func (cfg *Config) ToDisplayJSON() ([]byte, error) {
	jcfg, err := cfg.toJSONConfig()
	if err != nil {
		return nil, err
	}

	return config.DisplayJSON(jcfg)
}

// LogWriter returns a writer to write logs to. If a log path is configured,
// it creates a file.  Otherwise, uses the given logger.
func (cfg *Config) LogWriter() (io.Writer, error) {
	if cfg.HTTPLogFile != "" {
		f, err := os.OpenFile(cfg.GetHTTPLogPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, err
		}
		return f, nil
	}
	return logWriter{
		logger: cfg.RequestLogger,
	}, nil
}

func newTLSConfig(certFile, keyFile string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, errors.New("error loading TLS certificate/key: " + err.Error())
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
