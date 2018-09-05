// The ipfs-cluster-service application.
package main

import (
	"bufio"
	"fmt"
	"os"
	"os/user"
	"path/filepath"

	//	_ "net/http/pprof"

	logging "github.com/ipfs/go-log"
	cli "github.com/urfave/cli"

	ipfscluster "github.com/ipfs/ipfs-cluster"
)

// ProgramName of this application
const programName = `ipfs-cluster-service`

// flag defaults
const (
	defaultAllocation = "disk-freespace"
	defaultMonitor    = "pubsub"
	defaultPinTracker = "map"
	defaultLogLevel   = "info"
)

// We store a commit id here
var commit string

// Description provides a short summary of the functionality of this tool
var Description = fmt.Sprintf(`
%s runs an IPFS Cluster node.

A node participates in the cluster consensus, follows a distributed log
of pinning and unpinning requests and manages pinning operations to a
configured IPFS daemon.

This node also provides an API for cluster management, an IPFS Proxy API which
forwards requests to IPFS and a number of components for internal communication
using LibP2P. This is a simplified view of the components:

             +------------------+
             | ipfs-cluster-ctl |
             +---------+--------+
                       |
                       | HTTP(s)
ipfs-cluster-service   |                           HTTP
+----------+--------+--v--+----------------------+      +-------------+
| RPC/Raft | Peer 1 | API | IPFS Connector/Proxy +------> IPFS daemon |
+----^-----+--------+-----+----------------------+      +-------------+
     | libp2p
     |
+----v-----+--------+-----+----------------------+      +-------------+
| RPC/Raft | Peer 2 | API | IPFS Connector/Proxy +------> IPFS daemon |
+----^-----+--------+-----+----------------------+      +-------------+
     |
     |
+----v-----+--------+-----+----------------------+      +-------------+
| RPC/Raft | Peer 3 | API | IPFS Connector/Proxy +------> IPFS daemon |
+----------+--------+-----+----------------------+      +-------------+


%s needs a valid configuration to run. This configuration is
independent from IPFS and includes its own LibP2P key-pair. It can be
initialized with "init" and its default location is
 ~/%s/%s.

For feedback, bug reports or any additional information, visit
https://github.com/ipfs/ipfs-cluster.


EXAMPLES

Initial configuration:

$ ipfs-cluster-service init

Launch a cluster:

$ ipfs-cluster-service daemon

Launch a peer and join existing cluster:

$ ipfs-cluster-service daemon --bootstrap /ip4/192.168.1.2/tcp/9096/ipfs/QmPSoSaPXpyunaBwHs1rZBKYSqRV4bLRk32VGYLuvdrypL
`,
	programName,
	programName,
	DefaultPath,
	DefaultConfigFile)

var logger = logging.Logger("service")

// Default location for the configurations and data
var (
	// DefaultPath is initialized to $HOME/.ipfs-cluster
	// and holds all the ipfs-cluster data
	DefaultPath string
	// The name of the configuration file inside DefaultPath
	DefaultConfigFile = "service.json"
)

var (
	configPath string
)

func init() {
	// Set the right commit. The only way I could make this work
	ipfscluster.Commit = commit

	// We try guessing user's home from the HOME variable. This
	// allows HOME hacks for things like Snapcraft builds. HOME
	// should be set in all UNIX by the OS. Alternatively, we fall back to
	// usr.HomeDir (which should work on Windows etc.).
	home := os.Getenv("HOME")
	if home == "" {
		usr, err := user.Current()
		if err != nil {
			panic(fmt.Sprintf("cannot get current user: %s", err))
		}
		home = usr.HomeDir
	}

	DefaultPath = filepath.Join(home, ".ipfs-cluster")
}

func out(m string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, m, a...)
}

func checkErr(doing string, err error, args ...interface{}) {
	if err != nil {
		if len(args) > 0 {
			doing = fmt.Sprintf(doing, args)
		}
		out("error %s: %s\n", doing, err)
		err = locker.tryUnlock()
		if err != nil {
			out("error releasing execution lock: %s\n", err)
		}
		os.Exit(1)
	}
}

func main() {
	prepareApp().Run(os.Args)
}

// run daemon() by default, or error.
func run(c *cli.Context) error {
	cli.ShowAppHelp(c)
	os.Exit(1)
	return nil
}

func setupLogLevel(lvl string) {
	for f := range ipfscluster.LoggingFacilities {
		ipfscluster.SetFacilityLogLevel(f, lvl)
	}
	ipfscluster.SetFacilityLogLevel("service", lvl)
}

func setupDebug() {
	ipfscluster.SetFacilityLogLevel("*", "DEBUG")
}

func userProvidedSecret(enterSecret bool) ([]byte, bool) {
	var secret string
	if enterSecret {
		secret = promptUser("Enter cluster secret (32-byte hex string): ")
	} else if envSecret, envSecretDefined := os.LookupEnv("CLUSTER_SECRET"); envSecretDefined {
		secret = envSecret
	} else {
		return nil, false
	}

	decodedSecret, err := ipfscluster.DecodeClusterSecret(secret)
	checkErr("parsing user-provided secret", err)
	return decodedSecret, true
}

func promptUser(msg string) string {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Print(msg)
	scanner.Scan()
	return scanner.Text()
}

// Lifted from go-ipfs/cmd/ipfs/daemon.go
func yesNoPrompt(prompt string) bool {
	var s string
	for i := 0; i < 3; i++ {
		fmt.Printf("%s ", prompt)
		fmt.Scanf("%s", &s)
		switch s {
		case "y", "Y":
			return true
		case "n", "N":
			return false
		case "":
			return false
		}
		fmt.Println("Please press either 'y' or 'n'")
	}
	return false
}
