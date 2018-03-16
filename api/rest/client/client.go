package client

import (
	"context"
	"fmt"
	"net/http"
	"time"

	logging "github.com/ipfs/go-log"
	host "github.com/libp2p/go-libp2p-host"
	peer "github.com/libp2p/go-libp2p-peer"
	ma "github.com/multiformats/go-multiaddr"
	madns "github.com/multiformats/go-multiaddr-dns"
	manet "github.com/multiformats/go-multiaddr-net"
)

// Configuration defaults
var (
	DefaultTimeout  = 120 * time.Second
	DefaultAPIAddr  = "/ip4/127.0.0.1/tcp/9094"
	DefaultLogLevel = "info"
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

	err := client.setupHTTPClient()
	if err != nil {
		return nil, err
	}

	err = client.setupHostname()
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
	if c.config.Timeout == 0 {
		c.config.Timeout = DefaultTimeout
	}

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
		c.config.APIAddr, _ = ma.NewMultiaddr(DefaultAPIAddr)
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
		c.config.APIAddr = resolved[0]
		_, c.hostname, err = manet.DialArgs(c.config.APIAddr)
		if err != nil {
			return err
		}
	default:
		c.hostname = fmt.Sprintf("%s:%s", c.config.Host, c.config.Port)
	}
	return nil
}

func multiaddrSplit(addr ma.Multiaddr) (peer.ID, ma.Multiaddr, error) {
	pid, err := addr.ValueForProtocol(ma.P_IPFS)
	if err != nil {
		err = fmt.Errorf("invalid peer multiaddress: %s: %s", addr, err)
		logger.Error(err)
		return "", nil, err
	}

	ipfs, _ := ma.NewMultiaddr("/ipfs/" + pid)
	decapAddr := addr.Decapsulate(ipfs)

	peerID, err := peer.IDB58Decode(pid)
	if err != nil {
		err = fmt.Errorf("invalid peer ID in multiaddress: %s: %s", pid, err)
		logger.Error(err)
		return "", nil, err
	}
	return peerID, decapAddr, nil
}
