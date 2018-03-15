package client

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	p2phttp "github.com/hsanjuan/go-libp2p-http"
	logging "github.com/ipfs/go-log"
	libp2p "github.com/libp2p/go-libp2p"
	host "github.com/libp2p/go-libp2p-host"
	peer "github.com/libp2p/go-libp2p-peer"
	peerstore "github.com/libp2p/go-libp2p-peerstore"
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
	transport http.RoundTripper
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

	tr := c.defaultTransport()

	switch {
	case c.config.PeerAddr != nil:
		pid, addr, err := multiaddrSplit(c.config.PeerAddr)
		if err != nil {
			return err
		}

		h, err := libp2p.New(c.ctx)
		if err != nil {
			return err
		}

		// This should resolve addr too.
		h.Peerstore().AddAddr(pid, addr, peerstore.PermanentAddrTTL)
		tr.RegisterProtocol("libp2p", p2phttp.NewTransport(h))
		c.net = "libp2p"
		c.p2p = h
		c.hostname = pid.Pretty()
	case c.config.SSL:
		tr.TLSClientConfig = tlsConfig(c.config.NoVerifyCert)
		c.net = "https"
	default:
		c.net = "http"
	}

	c.client = &http.Client{
		Transport: tr,
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

// This is essentially a http.DefaultTransport. We should not mess
// with it since it's a global variable, so we create our own.
// TODO: Allow more configuration options.
func (c *Client) defaultTransport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}
