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
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	ipfscluster "github.com/ipfs-cluster/ipfs-cluster"
	ipfshttp "github.com/ipfs-cluster/ipfs-cluster/ipfsconn/ipfshttp"
	"github.com/ipfs/go-datastore"
	dual "github.com/libp2p/go-libp2p-kad-dht/dual"
	host "github.com/libp2p/go-libp2p/core/host"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
	"go.uber.org/multierr"
)

// RandomizePorts replaces TCP and UDP ports with random, but valid port
// values, on the given multiaddresses
func RandomizePorts(addrs []ma.Multiaddr) ([]ma.Multiaddr, error) {
	results := make([]ma.Multiaddr, 0, len(addrs))

	for _, m := range addrs {
		var prev string
		var err error
		var randomizedMultiaddr ma.Multiaddr
		ma.ForEach(m, func(c ma.Component) bool {
			code := c.Protocol().Code

			if code != ma.P_TCP && code != ma.P_UDP {
				randomizedMultiaddr = randomizedMultiaddr.AppendComponent(&c)
				prev = c.Value()
				return true
			}

			var ln io.Closer
			var port int

			// ip always comes in the prev component to /tcp or /udp
			ipStr := prev
			ip := net.ParseIP(ipStr)
			if ip.To16() != nil { // ipv6 needs bracketing
				ipStr = "[" + ipStr + "]"
			}

			if c.Protocol().Code == ma.P_UDP {
				ln, port, err = listenUDP(c.Protocol().Name, ipStr)
			} else {
				ln, port, err = listenTCP(c.Protocol().Name, ipStr)
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

			randomizedMultiaddr = randomizedMultiaddr.AppendComponent(c1)
			prev = c.Value()

			return true
		})
		if err != nil {
			return results, err
		}
		results = append(results, randomizedMultiaddr)
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
	dht *dual.DHT,
	store datastore.Datastore,
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
			return multierr.Combine(
				dht.Close(),
				host.Close(),
				store.Close(),
			)
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
// canceled.  The IPFS API location is determined by the default ipfshttp
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

func StopProcess(absPath string) error {
	pidFile := filepath.Join(absPath, "cluster.pid")

	if _, err := os.Stat(pidFile); os.IsNotExist(err) {
		return fmt.Errorf("ipfs-cluster-follow does not seem to be running (no PID file found)")
	}

	content, err := os.ReadFile(pidFile)
	if err != nil {
		return fmt.Errorf("failed to read PID file: %w", err)
	}

	pidStr := strings.TrimSpace(string(content))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return fmt.Errorf("invalid PID in file '%s': %w", pidStr, err)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("process with PID %d not found: %w", pid, err)
	}

	// send termination signal (os-specific)
	if err := terminateProcess(process, pid); err != nil {
		return fmt.Errorf("failed to send termination signal to process %d: %w", pid, err)
	}

	// Wait for process to exit (check PID file removal)
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			// Timeout: clean up PID file and report error
			os.Remove(pidFile)
			return fmt.Errorf("timeout waiting for process %d to exit after 30 seconds", pid)
		case <-ticker.C:
			// Check if PID file has been removed (process exited)
			if _, err := os.Stat(pidFile); os.IsNotExist(err) {
				time.Sleep(time.Second)
				fmt.Printf("ipfs-cluster-follower(PID: %d) exited successfully\n", pid)
				return nil
			}
		}
	}
}

func CreatePIDFile(absPath string) error {
	pidFile := filepath.Join(absPath, "cluster.pid")
	pid := os.Getpid()
	return os.WriteFile(pidFile, []byte(fmt.Sprintf("%d\n", pid)), 0644)
}

func RemovePIDFile(absPath string) error {
	pidFile := filepath.Join(absPath, "cluster.pid")
	if _, err := os.Stat(pidFile); os.IsNotExist(err) {
		return nil
	}
	return os.Remove(pidFile)
}
