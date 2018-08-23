// Package client provides a Go Client for the IPFS Cluster API provided
// by the "api/rest" component. It supports both the HTTP(s) endpoint and
// the libp2p-http endpoint.
package client

import (
	"context"
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
	DefaultTimeout   = 0
	DefaultAPIAddr   = "/ip4/127.0.0.1/tcp/9094"
	DefaultLogLevel  = "info"
	DefaultProxyPort = 9095
	ResolveTimeout   = 30 * time.Second
	DefaultPort      = 9094
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
	// (takes precedence over host:port). It this address contains
	// an /ipfs/, /p2p/ or /dnsaddr, the API will be contacted
	// through a libp2p tunnel, thus getting encryption for
	// free. Using the libp2p tunnel will ignore any configurations.
	APIAddr ma.Multiaddr

	// PeerAddr is deprecated. It's aliased to APIAddr
	PeerAddr ma.Multiaddr

	// REST API endpoint host and port. Only valid without
	// APIAddr and PeerAddr
	Host string
	Port string

	// If PeerAddr is provided, and the peer uses private networks
	// (pnet), then we need to provide the key. If the peer is the
	// cluster peer, this corresponds to the cluster secret.
	ProtectorKey []byte

	// ProxyAddr is used to obtain a go-ipfs-api Shell instance pointing
	// to the ipfs proxy endpoint of ipfs-cluster. If empty, the location
	// will be guessed from one of APIAddr/Host,
	// and the port used will be ipfs-cluster's proxy default port (9095)
	ProxyAddr ma.Multiaddr

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
	transport *http.Transport
	net       string
	hostname  string
	client    *http.Client
	p2p       host.Host
}

// NewClient initializes a client given a Config.
func NewClient(cfg *Config) (*Client, error) {
	ctx := context.Background()
	client := &Client{
		ctx:    ctx,
		config: cfg,
	}

	if paddr := client.config.PeerAddr; paddr != nil {
		client.config.APIAddr = paddr
	}

	if client.config.Port == "" {
		client.config.Port = fmt.Sprintf("%d", DefaultPort)
	}

	err := client.setupAPIAddr()
	if err != nil {
		return nil, err
	}

	err = client.resolveAPIAddr()
	if err != nil {
		return nil, err
	}

	err = client.setupHTTPClient()
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

func (c *Client) setupAPIAddr() error {
	var addr ma.Multiaddr
	var err error
	if c.config.APIAddr == nil {
		if c.config.Host == "" { //default
			addr, err = ma.NewMultiaddr(DefaultAPIAddr)
		} else {
			addrStr := fmt.Sprintf("/dns4/%s/tcp/%s", c.config.Host, c.config.Port)
			addr, err = ma.NewMultiaddr(addrStr)
		}
		c.config.APIAddr = addr
		return err
	}

	return nil
}

func (c *Client) resolveAPIAddr() error {
	resolveCtx, cancel := context.WithTimeout(c.ctx, ResolveTimeout)
	defer cancel()
	resolved, err := madns.Resolve(resolveCtx, c.config.APIAddr)
	if err != nil {
		return err
	}

	if len(resolved) == 0 {
		return fmt.Errorf("resolving %s returned 0 results", c.config.APIAddr)
	}

	c.config.APIAddr = resolved[0]
	return nil
}

func (c *Client) setupHTTPClient() error {
	var err error

	switch {
	case IsPeerAddress(c.config.APIAddr):
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
	// Extract host:port form APIAddr or use Host:Port.
	// For libp2p, hostname is set in enableLibp2p()
	if !IsPeerAddress(c.config.APIAddr) {
		_, hostname, err := manet.DialArgs(c.config.APIAddr)
		if err != nil {
			return err
		}
		c.hostname = hostname
	}
	return nil
}

func (c *Client) setupProxy() error {
	if c.config.ProxyAddr != nil {
		return nil
	}

	// Guess location from  APIAddr
	port, err := ma.NewMultiaddr(fmt.Sprintf("/tcp/%d", DefaultProxyPort))
	if err != nil {
		return err
	}
	c.config.ProxyAddr = ma.Split(c.config.APIAddr)[0].Encapsulate(port)
	return nil
}

// IPFS returns an instance of go-ipfs-api's Shell, pointing to the
// configured ProxyAddr (or to the default ipfs-cluster's IPFS proxy port).
// It re-uses this Client's HTTP client, thus will be constrained by
// the same configurations affecting it (timeouts...).
func (c *Client) IPFS() *shell.Shell {
	return shell.NewShellWithClient(c.config.ProxyAddr.String(), c.client)
}

// IsPeerAddress detects if the given multiaddress identifies a libp2p peer,
// either because it has the /p2p/ protocol or because it uses /dnsaddr/
func IsPeerAddress(addr ma.Multiaddr) bool {
	if addr == nil {
		return false
	}
	pid, err := addr.ValueForProtocol(ma.P_IPFS)
	dnsaddr, err2 := addr.ValueForProtocol(madns.DnsaddrProtocol.Code)
	return (pid != "" && err == nil) || (dnsaddr != "" && err2 == nil)
}
