// Package cmdutils contains utilities to facilitate building of command line
// applications launching cluster peers.
package cmdutils

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	ipfscluster "github.com/ipfs/ipfs-cluster"
	ipfshttp "github.com/ipfs/ipfs-cluster/ipfsconn/ipfshttp"
	host "github.com/libp2p/go-libp2p-host"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
)

// RandomizePorts replaces TCP and UDP ports with random, but valid port
// values, on the given multiaddresses
func RandomizePorts(addrs []ma.Multiaddr) ([]ma.Multiaddr, error) {
	results := make([]ma.Multiaddr, 0, len(addrs))

	for _, m := range addrs {
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

			var ln io.Closer
			var port int

			ip := prev
			if strings.Contains(ip, ":") { // ipv6 needs bracketing
				ip = "[" + ip + "]"
			}

			if c.Protocol().Code == ma.P_UDP {
				ln, port, err = listenUDP(c.Protocol().Name, ip)
			} else {
				ln, port, err = listenTCP(c.Protocol().Name, ip)
			}
			if err != nil {
				return false
			}
			defer ln.Close()

			var c1 *ma.Component
			c1, err = ma.NewComponent(c.Protocol().Name, fmt.Sprintf("%d", port))
			if err != nil {
				return false
			}

			components = append(components, c1)
			prev = c.Value()

			return true
		})
		if err != nil {
			return results, err
		}
		results = append(results, ma.Join(components...))
	}

	return results, nil
}

// returns the listener so it can be closed later and port
func listenTCP(name, ip string) (io.Closer, int, error) {
	ln, err := net.Listen(name, ip+":0")
	if err != nil {
		return nil, 0, err
	}

	return ln, ln.Addr().(*net.TCPAddr).Port, nil
}

// returns the listener so it can be cloesd later and port
func listenUDP(name, ip string) (io.Closer, int, error) {
	ln, err := net.ListenPacket(name, ip+":0")
	if err != nil {
		return nil, 0, err
	}

	return ln, ln.LocalAddr().(*net.UDPAddr).Port, nil
}

// HandleSignals orderly shuts down an IPFS Cluster peer
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

// WaitForIPFS hangs until IPFS API becomes available or the given context is
// cancelled.  The IPFS API location is determined by the default ipfshttp
// component configuration and can be overridden using environment variables
// that affect that configuration.  Note that we have to do this in the blind,
// since we want to wait for IPFS before we even fetch the IPFS component
// configuration (because the configuration might be hosted on IPFS itself)
func WaitForIPFS(ctx context.Context) error {
	ipfshttpCfg := ipfshttp.Config{}
	ipfshttpCfg.Default()
	ipfshttpCfg.ApplyEnvVars()
	ipfshttpCfg.ConnectSwarmsDelay = 0
	ipfshttpCfg.Tracing = false
	ipfscluster.SetFacilityLogLevel("ipfshttp", "critical")
	defer ipfscluster.SetFacilityLogLevel("ipfshttp", "info")
	ipfs, err := ipfshttp.NewConnector(&ipfshttpCfg)
	if err != nil {
		return errors.Wrap(err, "error creating an ipfshttp instance to wait for IPFS")
	}

	i := 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if i%10 == 0 {
				fmt.Printf("waiting for IPFS to become available on %s...\n", ipfshttpCfg.NodeAddr)
			}
			i++
			time.Sleep(time.Second)
			_, err := ipfs.ID(ctx)
			if err == nil {
				// sleep an extra second and quit
				time.Sleep(time.Second)
				return nil
			}
		}
	}
}
