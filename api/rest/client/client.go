package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	shell "github.com/ipfs/go-ipfs-api"
	logging "github.com/ipfs/go-log"
	host "github.com/libp2p/go-libp2p-host"
	ma "github.com/multiformats/go-multiaddr"
	madns "github.com/multiformats/go-multiaddr-dns"
	manet "github.com/multiformats/go-multiaddr-net"
)

// Configuration defaults
var (
	DefaultTimeout   = 120 * time.Second
	DefaultAPIAddr   = "/ip4/127.0.0.1/tcp/9094"
	DefaultLogLevel  = "info"
	DefaultProxyPort = 9095
)

var loggingFacility = "apiclient"
var logger = logging.Logger(loggingFacility)

// Config allows to configure the parameters to connect
// to the ipfs-cluster REST API.
type Config struct {
	// Enable SSL support. Only valid without PeerAddr.
	SSL bool
	// Skip certificate verification (insecure)
	NoVerifyCert bool

	// Username and password for basic authentication
	Username string
	Password string

	// The ipfs-cluster REST API endpoint in multiaddress form
	// (takes precedence over host:port). Only valid without PeerAddr.
	APIAddr ma.Multiaddr

	// REST API endpoint host and port. Only valid without
	// APIAddr and PeerAddr
	Host string
	Port string

	// The ipfs-cluster REST API peer address (usually
	// the same as the cluster peer). This will use libp2p
	// to tunnel HTTP requests, thus getting encryption for
	// free. It overseeds APIAddr, Host/Port, and SSL configurations.
	PeerAddr ma.Multiaddr

	// If PeerAddr is provided, and the peer uses private networks
	// (pnet), then we need to provide the key. If the peer is the
	// cluster peer, this corresponds to the cluster secret.
	ProtectorKey []byte

	// ProxyAddr is used to obtain a go-ipfs-api Shell instance pointing
	// to the ipfs proxy endpoint of ipfs-cluster. If empty, the location
	// will be guessed from one of PeerAddr/APIAddr/Host,
	// and the port used will be ipfs-cluster's proxy default port (9095)
	ProxyAddr ma.Multiaddr

	// Define timeout for network operations
	Timeout time.Duration

	// Specifies if we attempt to re-use connections to the same
	// hosts.
	DisableKeepAlives bool

	// LogLevel defines the verbosity of the logging facility
	LogLevel string

	Context context.Context
}

// Client provides methods to interact with the ipfs-cluster API. Use
// NewClient() to create one.
type Client struct {
	ctx       context.Context
	cancel    func()
	config    *Config
	transport *http.Transport
	net       string
	hostname  string
	client    *http.Client
	p2p       host.Host
}

// NewClient initializes a client given a Config.
func NewClient(cfg *Config) (*Client, error) {
	ctx := context.Background()
	if cfg.Context != nil {
		ctx = cfg.Context
	}
	client := &Client{
		ctx:    ctx,
		config: cfg,
	}

	if client.config.Timeout == 0 {
		client.config.Timeout = DefaultTimeout
	}

	err := client.setupHTTPClient()
	if err != nil {
		return nil, err
	}

	err = client.setupHostname()
	if err != nil {
		return nil, err
	}

	err = client.setupProxy()
	if err != nil {
		return nil, err
	}

	if lvl := cfg.LogLevel; lvl != "" {
		logging.SetLogLevel(loggingFacility, lvl)
	} else {
		logging.SetLogLevel(loggingFacility, DefaultLogLevel)
	}

	return client, nil
}

func (c *Client) setupHTTPClient() error {
	var err error

	switch {
	case c.config.PeerAddr != nil:
		err = c.enableLibp2p()
	case c.config.SSL:
		err = c.enableTLS()
	default:
		c.defaultTransport()
	}

	if err != nil {
		return err
	}

	c.client = &http.Client{
		Transport: c.transport,
		Timeout:   c.config.Timeout,
	}
	return nil
}

func (c *Client) setupHostname() error {
	// When no host/port/multiaddress defined, we set the default
	if c.config.APIAddr == nil && c.config.Host == "" && c.config.Port == "" {
		var err error
		c.config.APIAddr, err = ma.NewMultiaddr(DefaultAPIAddr)
		if err != nil {
			return err
		}
	}

	// PeerAddr takes precedence over APIAddr. APIAddr takes precedence
	// over Host/Port. APIAddr is resolved and dial args
	// extracted.
	switch {
	case c.config.PeerAddr != nil:
		// Taken care of in setupHTTPClient
	case c.config.APIAddr != nil:
		// Resolve multiaddress just in case and extract host:port
		resolveCtx, cancel := context.WithTimeout(c.ctx, c.config.Timeout)
		defer cancel()
		resolved, err := madns.Resolve(resolveCtx, c.config.APIAddr)
		if err != nil {
			return err
		}
		c.config.APIAddr = resolved[0]
		_, c.hostname, err = manet.DialArgs(c.config.APIAddr)
		if err != nil {
			return err
		}
	default:
		c.hostname = fmt.Sprintf("%s:%s", c.config.Host, c.config.Port)
		apiAddr, err := ma.NewMultiaddr(
			fmt.Sprintf("/ip4/%s/tcp/%s", c.config.Host, c.config.Port),
		)
		if err != nil {
			return err
		}
		c.config.APIAddr = apiAddr
	}
	return nil
}

func (c *Client) setupProxy() error {
	if c.config.ProxyAddr != nil {
		return nil
	}

	// Guess location from PeerAddr or APIAddr
	port, err := ma.NewMultiaddr(fmt.Sprintf("/tcp/%d", DefaultProxyPort))
	if err != nil {
		return err
	}
	var paddr ma.Multiaddr
	switch {
	case c.config.PeerAddr != nil:
		paddr = ma.Split(c.config.PeerAddr)[0].Encapsulate(port)
	case c.config.APIAddr != nil: // Host/Port setupHostname sets APIAddr
		paddr = ma.Split(c.config.APIAddr)[0].Encapsulate(port)
	default:
		return errors.New("cannot find proxy address")
	}

	ctx, cancel := context.WithTimeout(c.ctx, c.config.Timeout)
	defer cancel()
	resolved, err := madns.Resolve(ctx, paddr)
	if err != nil {
		return err
	}

	c.config.ProxyAddr = resolved[0]
	return nil
}

// IPFS returns an instance of go-ipfs-api's Shell, pointing to the
// configured ProxyAddr (or to the default ipfs-cluster's IPFS proxy port).
// It re-uses this Client's HTTP client, thus will be constrained by
// the same configurations affecting it (timeouts...).
func (c *Client) IPFS() *shell.Shell {
	return shell.NewShellWithClient(c.config.ProxyAddr.String(), c.client)
}
