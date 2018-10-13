package ipfsproxy

import (
	"errors"
	"time"

	"github.com/ipfs/ipfs-cluster/config"
	ma "github.com/multiformats/go-multiaddr"
)

// Config is used to initialize a Connector and allows to customize
// its behaviour. It implements the config.ComponentConfig interface.
type Config struct {
	config.Saver

	// Listen parameters for the IPFS Proxy. Used by the IPFS
	// connector component.
	ProxyAddr ma.Multiaddr

	// Host/Port for the IPFS daemon.
	NodeAddr ma.Multiaddr

	// Maximum duration before timing out reading a full request
	ProxyReadTimeout time.Duration
	// Maximum duration before timing out reading the headers of a request
	ProxyReadHeaderTimeout time.Duration

	// Maximum duration before timing out write of the response
	ProxyWriteTimeout time.Duration

	// Server-side amount of time a Keep-Alive connection will be
	// kept idle before being reused
	ProxyIdleTimeout time.Duration
}

// Validate checks that the fields of this Config have sensible values,
// at least in appearance.
func (cfg *Config) Validate() error {
	var err error
	if cfg.ProxyAddr == nil {
		err = errors.New("ipfshttp.proxy_listen_multiaddress not set")
	}
	if cfg.NodeAddr == nil {
		err = errors.New("ipfshttp.node_multiaddress not set")
	}

	if cfg.ProxyReadTimeout < 0 {
		err = errors.New("ipfshttp.proxy_read_timeout is invalid")
	}

	if cfg.ProxyReadHeaderTimeout < 0 {
		err = errors.New("ipfshttp.proxy_read_header_timeout is invalid")
	}

	if cfg.ProxyWriteTimeout < 0 {
		err = errors.New("ipfshttp.proxy_write_timeout is invalid")
	}

	if cfg.ProxyIdleTimeout < 0 {
		err = errors.New("ipfshttp.proxy_idle_timeout invalid")
	}

	return err
}
