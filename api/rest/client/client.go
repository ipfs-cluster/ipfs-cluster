package client

import (
	"context"
	"net/http"
	"time"

	logging "github.com/ipfs/go-log"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr-net"
)

// Configuration defaults
var (
	DefaultTimeout  = 60 * time.Second
	DefaultAPIAddr  = "/ip4/127.0.0.1/tcp/9094"
	DefaultLogLevel = "info"
)

var loggingFacility = "apiclient"
var logger = logging.Logger(loggingFacility)

// Config allows to configure the parameters to connect
// to the ipfs-cluster REST API.
type Config struct {
	// Enable SSL support
	SSL bool
	// Skip certificate verification (insecure)
	NoVerifyCert bool

	// Username and password for basic authentication
	Username string
	Password string

	// The ipfs-cluster REST API endpoint
	APIAddr ma.Multiaddr

	// Define timeout for network operations
	Timeout time.Duration

	// Specifies if we attempt to re-use connections to the same
	// hosts.
	DisableKeepAlives bool

	// LogLevel defines the verbosity of the logging facility
	LogLevel string
}

// Client provides methods to interact with the ipfs-cluster API. Use
// NewClient() to create one.
type Client struct {
	ctx       context.Context
	cancel    func()
	config    *Config
	transport http.RoundTripper
	urlPrefix string
	client    *http.Client
}

// NewClient initializes a client given a Config.
func NewClient(cfg *Config) (*Client, error) {
	var urlPrefix = ""

	var tr http.RoundTripper
	if cfg.SSL {
		tr = newTLSTransport(cfg.NoVerifyCert)
		urlPrefix += "https://"
	} else {
		tr = http.DefaultTransport
		urlPrefix += "http://"
	}

	if cfg.APIAddr == nil {
		cfg.APIAddr, _ = ma.NewMultiaddr(DefaultAPIAddr)
	}
	_, host, err := manet.DialArgs(cfg.APIAddr)
	if err != nil {
		return nil, err
	}
	urlPrefix += host

	if lvl := cfg.LogLevel; lvl != "" {
		logging.SetLogLevel(loggingFacility, lvl)
	} else {
		logging.SetLogLevel(loggingFacility, DefaultLogLevel)
	}

	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultTimeout
	}

	ctx, cancel := context.WithCancel(context.Background())

	client := &http.Client{
		Transport: tr,
		Timeout:   cfg.Timeout,
	}

	return &Client{
		ctx:       ctx,
		cancel:    cancel,
		urlPrefix: urlPrefix,
		transport: tr,
		config:    cfg,
		client:    client,
	}, nil
}
