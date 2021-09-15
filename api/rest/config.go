package rest

import (
	"net/http"
	"time"

	ma "github.com/multiformats/go-multiaddr"

	"github.com/ipfs/ipfs-cluster/api/common"
)

const configKey = "restapi"
const envConfigKey = "cluster_restapi"

const minMaxHeaderBytes = 4096

// Default values for Config
const (
	DefaultReadTimeout       = 0
	DefaultReadHeaderTimeout = 5 * time.Second
	DefaultWriteTimeout      = 0
	DefaultIdleTimeout       = 120 * time.Second
	DefaultMaxHeaderBytes    = minMaxHeaderBytes
)

// These are the default values for Config.
var (
	// DefaultHTTPListenAddrs contains default listen addresses for the HTTP API.
	DefaultHTTPListenAddrs = []string{"/ip4/127.0.0.1/tcp/9094"}
	DefaultHeaders         = map[string][]string{}
)

// CORS defaults
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

// Config is used to intialize the API object and allows to customize the
// behaviour of it. It implements the config.ComponentConfig interface.
type Config struct {
	common.Config
}

// ConfigKey returns a human-friendly identifier for this type of
// Config.
func (cfg *Config) ConfigKey() string {
	return configKey
}

// Default initializes this Config with working values.
func (cfg *Config) Default() error {
	cfg.Config.ConfigKey = configKey
	cfg.EnvConfigKey = envConfigKey
	cfg.Logger = logger
	cfg.RequestLogger = apiLogger
	cfg.DefaultFunc = defaultFunc
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
