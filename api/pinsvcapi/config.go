package pinsvcapi

import (
	"net/http"
	"time"

	ma "github.com/multiformats/go-multiaddr"

	"github.com/ipfs/ipfs-cluster/api/common"
	"github.com/ipfs/ipfs-cluster/api/pinsvcapi/pinsvc"
)

const configKey = "restapi"
const envConfigKey = "cluster_restapi"

const minMaxHeaderBytes = 4096

// Default values for Config.
const (
	DefaultReadTimeout       = 0
	DefaultReadHeaderTimeout = 5 * time.Second
	DefaultWriteTimeout      = 0
	DefaultIdleTimeout       = 120 * time.Second
	DefaultMaxHeaderBytes    = minMaxHeaderBytes
)

// Default values for Config.
var (
	// DefaultHTTPListenAddrs contains default listen addresses for the HTTP API.
	DefaultHTTPListenAddrs = []string{"/ip4/127.0.0.1/tcp/9094"}
	DefaultHeaders         = map[string][]string{}
)

// CORS defaults.
var (
	DefaultCORSAllowedOrigins = []string{"*"}
	DefaultCORSAllowedMethods = []string{
		http.MethodGet,
	}
	// rs/cors this will set sensible defaults when empty:
	// {"Origin", "Accept", "Content-Type", "X-Requested-With"}
	DefaultCORSAllowedHeaders = []string{}
	DefaultCORSExposedHeaders = []string{
		"Content-Type",
		"X-Stream-Output",
		"X-Chunked-Output",
		"X-Content-Length",
	}
	DefaultCORSAllowCredentials = true
	DefaultCORSMaxAge           time.Duration // 0. Means always.
)

// Config fully implements the config.ComponentConfig interface. Use
// NewConfig() to instantiate. Config embeds a common.Config object.
type Config struct {
	common.Config
}

// NewConfig creates a Config object setting the necessary meta-fields in the
// common.Config embedded object.
func NewConfig() *Config {
	cfg := Config{}
	cfg.Config.ConfigKey = configKey
	cfg.EnvConfigKey = envConfigKey
	cfg.Logger = logger
	cfg.RequestLogger = apiLogger
	cfg.DefaultFunc = defaultFunc
	cfg.APIErrorFunc = func(err error, status int) error {
		return pinsvc.APIError{
			Reason: err.Error(),
		}
	}
	return &cfg
}

// ConfigKey returns a human-friendly identifier for this type of
// Config.
func (cfg *Config) ConfigKey() string {
	return configKey
}

// Default initializes this Config with working values.
func (cfg *Config) Default() error {
	return defaultFunc(&cfg.Config)
}

// Sets all defaults for this config.
func defaultFunc(cfg *common.Config) error {
	// http
	addrs := make([]ma.Multiaddr, 0, len(DefaultHTTPListenAddrs))
	for _, def := range DefaultHTTPListenAddrs {
		httpListen, err := ma.NewMultiaddr(def)
		if err != nil {
			return err
		}
		addrs = append(addrs, httpListen)
	}
	cfg.HTTPListenAddr = addrs
	cfg.PathSSLCertFile = ""
	cfg.PathSSLKeyFile = ""
	cfg.ReadTimeout = DefaultReadTimeout
	cfg.ReadHeaderTimeout = DefaultReadHeaderTimeout
	cfg.WriteTimeout = DefaultWriteTimeout
	cfg.IdleTimeout = DefaultIdleTimeout
	cfg.MaxHeaderBytes = DefaultMaxHeaderBytes

	// libp2p
	cfg.ID = ""
	cfg.PrivateKey = nil
	cfg.Libp2pListenAddr = nil

	// Auth
	cfg.BasicAuthCredentials = nil

	// Logs
	cfg.HTTPLogFile = ""

	// Headers
	cfg.Headers = DefaultHeaders

	cfg.CORSAllowedOrigins = DefaultCORSAllowedOrigins
	cfg.CORSAllowedMethods = DefaultCORSAllowedMethods
	cfg.CORSAllowedHeaders = DefaultCORSAllowedHeaders
	cfg.CORSExposedHeaders = DefaultCORSExposedHeaders
	cfg.CORSAllowCredentials = DefaultCORSAllowCredentials
	cfg.CORSMaxAge = DefaultCORSMaxAge

	return nil
}
