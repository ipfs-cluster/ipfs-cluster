// Package cmdutils contains utilities to facilitate building of command line
// applications launching cluster peers.
package cmdutils

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	ipfscluster "github.com/ipfs/ipfs-cluster"
	host "github.com/libp2p/go-libp2p-host"
	dht "github.com/libp2p/go-libp2p-kad-dht"
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

// HandleSignals orderly shutsdown an IPFS Cluster peer
// on SIGINT, SIGTERM, SIGHUP. It forces command termination
// on the 3rd-signal count.
func HandleSignals(
	ctx context.Context,
	cancel context.CancelFunc,
	cluster *ipfscluster.Cluster,
	host host.Host,
	dht *dht.IpfsDHT,
) error {
	signalChan := make(chan os.Signal, 20)
	signal.Notify(
		signalChan,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGHUP,
	)

	var ctrlcCount int
	for {
		select {
		case <-signalChan:
			ctrlcCount++
			handleCtrlC(ctx, cluster, ctrlcCount)
		case <-cluster.Done():
			cancel()
			dht.Close()
			host.Close()
			return nil
		}
	}
}

func handleCtrlC(ctx context.Context, cluster *ipfscluster.Cluster, ctrlcCount int) {
	switch ctrlcCount {
	case 1:
		go func() {
			if err := cluster.Shutdown(ctx); err != nil {
				ErrorOut("error shutting down cluster: %s", err)
				os.Exit(1)
			}
		}()
	case 2:
		ErrorOut(`


!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!
Shutdown is taking too long! Press Ctrl-c again to manually kill cluster.
Note that this may corrupt the local cluster state.
!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!


`)
	case 3:
		ErrorOut("exiting cluster NOW")
		os.Exit(1)
	}
}

// ErrorOut formats something and prints it to sdterr.
func ErrorOut(m string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, m, a...)
}
