package ipfsproxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/kelseyhightower/envconfig"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/ipfs-cluster/ipfs-cluster/config"
)

const (
	configKey         = "ipfsproxy"
	envConfigKey      = "cluster_ipfsproxy"
	minMaxHeaderBytes = 4096
)

// DefaultListenAddrs contains the default listeners for the proxy.
var DefaultListenAddrs = []string{
	"/ip4/127.0.0.1/tcp/9095",
}

// Default values for Config.
const (
	DefaultNodeAddr           = "/ip4/127.0.0.1/tcp/5001"
	DefaultNodeHTTPS          = false
	DefaultReadTimeout        = 0
	DefaultReadHeaderTimeout  = 5 * time.Second
	DefaultWriteTimeout       = 0
	DefaultIdleTimeout        = 60 * time.Second
	DefaultExtractHeadersPath = "/api/v0/version"
	DefaultExtractHeadersTTL  = 5 * time.Minute
	DefaultMaxHeaderBytes     = minMaxHeaderBytes
)

// Config allows to customize behavior of IPFSProxy.
// It implements the config.ComponentConfig interface.
type Config struct {
	config.Saver

	// Listen parameters for the IPFS Proxy.
	ListenAddr []ma.Multiaddr

	// Host/Port for the IPFS daemon.
	NodeAddr ma.Multiaddr

	// Should we talk to the IPFS API over HTTPS? (experimental, untested)
	NodeHTTPS bool

	// LogFile is path of the file that would save Proxy API logs. If this
	// path is empty, logs would be sent to standard output. This path
	// should either be absolute or relative to cluster base directory. Its
	// default value is empty.
	LogFile string

	// Maximum duration before timing out reading a full request
	ReadTimeout time.Duration

	// Maximum duration before timing out reading the headers of a request
	ReadHeaderTimeout time.Duration

	// Maximum duration before timing out write of the response
	WriteTimeout time.Duration

	// Maximum cumulative size of HTTP request headers in bytes
	// accepted by the server
	MaxHeaderBytes int

	// Server-side amount of time a Keep-Alive connection will be
	// kept idle before being reused
	IdleTimeout time.Duration

	// A list of custom headers that should be extracted from
	// IPFS daemon responses and re-used in responses from hijacked paths.
	// This is only useful if the user has configured custom headers
	// in the IPFS daemon. CORS-related headers are already
	// taken care of by the proxy.
	ExtractHeadersExtra []string

	// If the user wants to extract some extra custom headers configured
	// on the IPFS daemon so that they are used in hijacked responses,
	// this request path will be used. Defaults to /version. This will
	// trigger a single request to extract those headers and remember them
	// for future requests (until TTL expires).
	ExtractHeadersPath string

	// Establishes how long we should remember extracted headers before we
	// refresh them with a new request. 0 means always.
	ExtractHeadersTTL time.Duration

	// Tracing flag used to skip tracing specific paths when not enabled.
	Tracing bool
}

type jsonConfig struct {
	ListenMultiaddress config.Strings `json:"listen_multiaddress"`
	NodeMultiaddress   string         `json:"node_multiaddress"`
	NodeHTTPS          bool           `json:"node_https,omitempty"`

	LogFile string `json:"log_file"`

	ReadTimeout       string `json:"read_timeout"`
	ReadHeaderTimeout string `json:"read_header_timeout"`
	WriteTimeout      string `json:"write_timeout"`
	IdleTimeout       string `json:"idle_timeout"`
	MaxHeaderBytes    int    `json:"max_header_bytes"`

	ExtractHeadersExtra []string `json:"extract_headers_extra,omitempty"`
	ExtractHeadersPath  string   `json:"extract_headers_path,omitempty"`
	ExtractHeadersTTL   string   `json:"extract_headers_ttl,omitempty"`
}

// getLogPath gets full path of the file where proxy logs should be
// saved.
func (cfg *Config) getLogPath() string {
	if filepath.IsAbs(cfg.LogFile) {
		return cfg.LogFile
	}

	if cfg.BaseDir == "" {
		return ""
	}

	return filepath.Join(cfg.BaseDir, cfg.LogFile)
}

// ConfigKey provides a human-friendly identifier for this type of Config.
func (cfg *Config) ConfigKey() string {
	return configKey
}

// Default sets the fields of this Config to sensible default values.
func (cfg *Config) Default() error {
	proxy := make([]ma.Multiaddr, 0, len(DefaultListenAddrs))
	for _, def := range DefaultListenAddrs {
		a, err := ma.NewMultiaddr(def)
		if err != nil {
			return err
		}
		proxy = append(proxy, a)
	}
	node, err := ma.NewMultiaddr(DefaultNodeAddr)
	if err != nil {
		return err
	}
	cfg.ListenAddr = proxy
	cfg.NodeAddr = node
	cfg.LogFile = ""
	cfg.ReadTimeout = DefaultReadTimeout
	cfg.ReadHeaderTimeout = DefaultReadHeaderTimeout
	cfg.WriteTimeout = DefaultWriteTimeout
	cfg.IdleTimeout = DefaultIdleTimeout
	cfg.ExtractHeadersExtra = nil
	cfg.ExtractHeadersPath = DefaultExtractHeadersPath
	cfg.ExtractHeadersTTL = DefaultExtractHeadersTTL
	cfg.MaxHeaderBytes = DefaultMaxHeaderBytes

	return nil
}

// ApplyEnvVars fills in any Config fields found
// as environment variables.
func (cfg *Config) ApplyEnvVars() error {
	jcfg, err := cfg.toJSONConfig()
	if err != nil {
		return err
	}

	err = envconfig.Process(envConfigKey, jcfg)
	if err != nil {
		return err
	}

	return cfg.applyJSONConfig(jcfg)
}

// Validate checks that the fields of this Config have sensible values,
// at least in appearance.
func (cfg *Config) Validate() error {
	var err error
	if len(cfg.ListenAddr) == 0 {
		err = errors.New("ipfsproxy.listen_multiaddress not set")
	}
	if cfg.NodeAddr == nil {
		err = errors.New("ipfsproxy.node_multiaddress not set")
	}

	if cfg.ReadTimeout < 0 {
		err = errors.New("ipfsproxy.read_timeout is invalid")
	}

	if cfg.ReadHeaderTimeout < 0 {
		err = errors.New("ipfsproxy.read_header_timeout is invalid")
	}

	if cfg.WriteTimeout < 0 {
		err = errors.New("ipfsproxy.write_timeout is invalid")
	}

	if cfg.IdleTimeout < 0 {
		err = errors.New("ipfsproxy.idle_timeout invalid")
	}

	if cfg.ExtractHeadersPath == "" {
		err = errors.New("ipfsproxy.extract_headers_path should not be empty")
	}

	if cfg.ExtractHeadersTTL < 0 {
		err = errors.New("ipfsproxy.extract_headers_ttl is invalid")
	}

	if cfg.MaxHeaderBytes < minMaxHeaderBytes {
		err = fmt.Errorf("ipfsproxy.max_header_size must be greater or equal to %d", minMaxHeaderBytes)
	}

	return err
}

// LoadJSON parses a JSON representation of this Config as generated by ToJSON.
func (cfg *Config) LoadJSON(raw []byte) error {
	jcfg := &jsonConfig{}
	err := json.Unmarshal(raw, jcfg)
	if err != nil {
		logger.Error("Error unmarshaling ipfsproxy config")
		return err
	}

	err = cfg.Default()
	if err != nil {
		return fmt.Errorf("error setting config to default values: %s", err)
	}

	return cfg.applyJSONConfig(jcfg)
}

func (cfg *Config) applyJSONConfig(jcfg *jsonConfig) error {
	if addresses := jcfg.ListenMultiaddress; len(addresses) > 0 {
		cfg.ListenAddr = make([]ma.Multiaddr, 0, len(addresses))
		for _, a := range addresses {
			proxyAddr, err := ma.NewMultiaddr(a)
			if err != nil {
				return fmt.Errorf("error parsing proxy listen_multiaddress: %s", err)
			}
			cfg.ListenAddr = append(cfg.ListenAddr, proxyAddr)
		}
	}
	if jcfg.NodeMultiaddress != "" {
		nodeAddr, err := ma.NewMultiaddr(jcfg.NodeMultiaddress)
		if err != nil {
			return fmt.Errorf("error parsing ipfs node_multiaddress: %s", err)
		}
		cfg.NodeAddr = nodeAddr
	}
	config.SetIfNotDefault(jcfg.NodeHTTPS, &cfg.NodeHTTPS)

	config.SetIfNotDefault(jcfg.LogFile, &cfg.LogFile)

	err := config.ParseDurations(
		"ipfsproxy",
		&config.DurationOpt{Duration: jcfg.ReadTimeout, Dst: &cfg.ReadTimeout, Name: "read_timeout"},
		&config.DurationOpt{Duration: jcfg.ReadHeaderTimeout, Dst: &cfg.ReadHeaderTimeout, Name: "read_header_timeout"},
		&config.DurationOpt{Duration: jcfg.WriteTimeout, Dst: &cfg.WriteTimeout, Name: "write_timeout"},
		&config.DurationOpt{Duration: jcfg.IdleTimeout, Dst: &cfg.IdleTimeout, Name: "idle_timeout"},
		&config.DurationOpt{Duration: jcfg.ExtractHeadersTTL, Dst: &cfg.ExtractHeadersTTL, Name: "extract_header_ttl"},
	)
	if err != nil {
		return err
	}

	if jcfg.MaxHeaderBytes == 0 {
		cfg.MaxHeaderBytes = DefaultMaxHeaderBytes
	} else {
		cfg.MaxHeaderBytes = jcfg.MaxHeaderBytes
	}

	if extra := jcfg.ExtractHeadersExtra; len(extra) > 0 {
		cfg.ExtractHeadersExtra = extra
	}
	config.SetIfNotDefault(jcfg.ExtractHeadersPath, &cfg.ExtractHeadersPath)

	return cfg.Validate()
}

// ToJSON generates a human-friendly JSON representation of this Config.
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

	jcfg = &jsonConfig{}

	addresses := make([]string, 0, len(cfg.ListenAddr))
	for _, a := range cfg.ListenAddr {
		addresses = append(addresses, a.String())
	}

	// Set all configuration fields
	jcfg.ListenMultiaddress = addresses
	jcfg.NodeMultiaddress = cfg.NodeAddr.String()
	jcfg.ReadTimeout = cfg.ReadTimeout.String()
	jcfg.ReadHeaderTimeout = cfg.ReadHeaderTimeout.String()
	jcfg.WriteTimeout = cfg.WriteTimeout.String()
	jcfg.IdleTimeout = cfg.IdleTimeout.String()
	jcfg.MaxHeaderBytes = cfg.MaxHeaderBytes
	jcfg.NodeHTTPS = cfg.NodeHTTPS
	jcfg.LogFile = cfg.LogFile

	jcfg.ExtractHeadersExtra = cfg.ExtractHeadersExtra
	if cfg.ExtractHeadersPath != DefaultExtractHeadersPath {
		jcfg.ExtractHeadersPath = cfg.ExtractHeadersPath
	}
	if ttl := cfg.ExtractHeadersTTL; ttl != DefaultExtractHeadersTTL {
		jcfg.ExtractHeadersTTL = ttl.String()
	}

	return
}

// ToDisplayJSON returns JSON config as a string.
func (cfg *Config) ToDisplayJSON() ([]byte, error) {
	jcfg, err := cfg.toJSONConfig()
	if err != nil {
		return nil, err
	}

	return config.DisplayJSON(jcfg)
}
