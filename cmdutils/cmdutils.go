// Package cmdutils contains utilities to facilitate building of command line
// applications launching cluster peers.
package cmdutils

import (
	"fmt"
	"net"

	ma "github.com/multiformats/go-multiaddr"
)

// RandomizePorts replaces TCP and UDP ports with random, but valid port
// values.
func RandomizePorts(m ma.Multiaddr) (ma.Multiaddr, error) {
	var prev string

	var err error
	components := []ma.Multiaddr{}
	ma.ForEach(m, func(c ma.Component) bool {
		code := c.Protocol().Code

		if code != ma.P_TCP && code != ma.P_UDP {
			components = append(components, &c)
			prev = c.Value()
			return true
		}

		var ln net.Listener
		ln, err = net.Listen(c.Protocol().Name, prev+":")
		if err != nil {
			return false
		}
		defer ln.Close()

		var c1 *ma.Component
		c1, err = ma.NewComponent(c.Protocol().Name, fmt.Sprintf("%d", getPort(ln, code)))
		if err != nil {
			return false
		}

		components = append(components, c1)
		prev = c.Value()

		return true
	})

	return ma.Join(components...), err
}

func getPort(ln net.Listener, code int) int {
	if code == ma.P_TCP {
		return ln.Addr().(*net.TCPAddr).Port
	}
	if code == ma.P_UDP {
		return ln.Addr().(*net.UDPAddr).Port
	}
	return 0
}
