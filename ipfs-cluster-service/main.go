package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"

	//	_ "net/http/pprof"

	logging "github.com/ipfs/go-log"
	cli "github.com/urfave/cli"

	ipfscluster "github.com/ipfs/ipfs-cluster"
	"github.com/ipfs/ipfs-cluster/state/mapstate"
)

// ProgramName of this application
const programName = `ipfs-cluster-service`

// flag defaults
const (
	defaultAllocation = "disk-freespace"
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
	// go func() {
	//	log.Println(http.ListenAndServe("localhost:6060", nil))
	// }()

	app := cli.NewApp()
	app.Name = programName
	app.Usage = "IPFS Cluster node"
	app.Description = Description
	//app.Copyright = "© Protocol Labs, Inc."
	app.Version = ipfscluster.Version
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "config, c",
			Value:  DefaultPath,
			Usage:  "path to the configuration and data `FOLDER`",
			EnvVar: "IPFS_CLUSTER_PATH",
		},
		cli.BoolFlag{
			Name:  "force, f",
			Usage: "forcefully proceed with some actions. i.e. overwriting configuration",
		},
		cli.BoolFlag{
			Name:  "debug, d",
			Usage: "enable full debug logging (very verbose)",
		},
		cli.StringFlag{
			Name:  "loglevel, l",
			Value: defaultLogLevel,
			Usage: "set the loglevel for cluster components only [critical, error, warning, info, debug]",
		},
	}

	app.Commands = []cli.Command{
		{
			Name:  "init",
			Usage: "create a default configuration and exit",
			Description: fmt.Sprintf(`
This command will initialize a new service.json configuration file
for %s.

By default, %s requires a cluster secret. This secret will be
automatically generated, but can be manually provided with --custom-secret
(in which case it will be prompted), or by setting the CLUSTER_SECRET
environment variable.

The private key for the libp2p node is randomly generated in all cases.

Note that the --force first-level-flag allows to overwrite an existing
configuration.
`, programName, programName),
			ArgsUsage: " ",
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "custom-secret, s",
					Usage: "prompt for the cluster secret",
				},
			},
			Action: func(c *cli.Context) error {
				userSecret, userSecretDefined := userProvidedSecret(c.Bool("custom-secret"))
				cfgMgr, cfgs := makeConfigs()
				defer cfgMgr.Shutdown() // wait for saves

				// Generate defaults for all registered components
				err := cfgMgr.Default()
				checkErr("generating default configuration", err)

				// Set user secret
				if userSecretDefined {
					cfgs.clusterCfg.Secret = userSecret
				}

				// Save
				saveConfig(cfgMgr, c.GlobalBool("force"))
				return nil
			},
		},
		{
			Name:  "daemon",
			Usage: "run the IPFS Cluster peer (default)",
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "upgrade, u",
					Usage: "run necessary state migrations before starting cluster service",
				},
				cli.StringSliceFlag{
					Name:  "bootstrap, j",
					Usage: "join a cluster providing an existing peers multiaddress(es)",
				},
				cli.BoolFlag{
					Name:   "leave, x",
					Usage:  "remove peer from cluster on exit. Overrides \"leave_on_shutdown\"",
					Hidden: true,
				},
				cli.StringFlag{
					Name:  "alloc, a",
					Value: defaultAllocation,
					Usage: "allocation strategy to use [disk-freespace,disk-reposize,numpin].",
				},
			},
			Action: daemon,
		},
		{
			Name:  "state",
			Usage: "Manage ipfs-cluster-state",
			Subcommands: []cli.Command{
				{
					Name:  "version",
					Usage: "display the shared state format version",
					Action: func(c *cli.Context) error {
						fmt.Printf("%d\n", mapstate.Version)
						return nil
					},
				},
				{
					Name:  "upgrade",
					Usage: "upgrade the IPFS Cluster state to the current version",
					Description: `
This command upgrades the internal state of the ipfs-cluster node 
specified in the latest raft snapshot. The state format is migrated from the 
version of the snapshot to the version supported by the current cluster version. 
To successfully run an upgrade of an entire cluster, shut down each peer without
removal, upgrade state using this command, and restart every peer.
`,
					Action: func(c *cli.Context) error {
						err := locker.lock()
						checkErr("acquiring execution lock", err)
						defer locker.tryUnlock()

						err = upgrade()
						checkErr("upgrading state", err)
						return nil
					},
				},
				{
					Name:  "export",
					Usage: "save the IPFS Cluster state to a json file",
					Description: `
This command reads the current cluster state and saves it as json for 
human readability and editing.  Only state formats compatible with this
version of ipfs-cluster-service can be exported.  By default this command
prints the state to stdout.
`,
					Flags: []cli.Flag{
						cli.StringFlag{
							Name:  "file, f",
							Value: "",
							Usage: "sets an output file for exported state",
						},
					},
					Action: func(c *cli.Context) error {
						err := locker.lock()
						checkErr("acquiring execution lock", err)
						defer locker.tryUnlock()

						var w io.WriteCloser
						outputPath := c.String("file")
						if outputPath == "" {
							// Output to stdout
							w = os.Stdout
						} else {
							// Create the export file
							w, err = os.Create(outputPath)
							checkErr("creating output file", err)
						}
						defer w.Close()

						err = export(w)
						checkErr("exporting state", err)
						return nil
					},
				},
				{
					Name:  "import",
					Usage: "load an IPFS Cluster state from an exported state file",
					Description: `
This command reads in an exported state file storing the state as a persistent
snapshot to be loaded as the cluster state when the cluster peer is restarted.
If an argument is provided, cluster will treat it as the path of the file to
import.  If no argument is provided cluster will read json from stdin
`,
					Action: func(c *cli.Context) error {
						err := locker.lock()
						checkErr("acquiring execution lock", err)
						defer locker.tryUnlock()

						if !c.GlobalBool("force") {
							if !yesNoPrompt("The peer's state will be replaced.  Run with -h for details.  Continue? [y/n]:") {
								return nil
							}
						}

						// Get the importing file path
						importFile := c.Args().First()
						var r io.ReadCloser
						if importFile == "" {
							r = os.Stdin
							logger.Info("Reading from stdin, Ctrl-D to finish")
						} else {
							r, err = os.Open(importFile)
							checkErr("reading import file", err)
						}
						defer r.Close()
						err = stateImport(r)
						checkErr("importing state", err)
						logger.Info("the given state has been correctly imported to this peer.  Make sure all peers have consistent states")
						return nil
					},
				},
				{
					Name:  "cleanup",
					Usage: "cleanup persistent consensus state so cluster can start afresh",
					Description: `
This command removes the persistent state that is loaded on startup to determine this peer's view of the
cluster state.  While it removes the existing state from the load path, one invocation does not permanently remove
this state from disk.  This command renames cluster's data folder to <data-folder-name>.old.0, and rotates other
deprecated data folders to <data-folder-name>.old.<n+1>, etc for some rotation factor before permanatly deleting 
the mth data folder (m currently defaults to 5)
`,
					Action: func(c *cli.Context) error {
						err := locker.lock()
						checkErr("acquiring execution lock", err)
						defer locker.tryUnlock()

						if !c.GlobalBool("force") {
							if !yesNoPrompt("The peer's state will be removed from the load path.  Existing pins may be lost.  Continue? [y/n]:") {
								return nil
							}
						}

						cfgMgr, cfgs := makeConfigs()
						err = cfgMgr.LoadJSONFromFile(configPath)
						checkErr("reading configuration", err)

						err = cleanupState(cfgs.consensusCfg)
						checkErr("Cleaning up consensus data", err)
						logger.Warningf("the %s folder has been rotated.  Next start will use an empty state", cfgs.consensusCfg.GetDataFolder())
						return nil
					},
				},
			},
		},
		{
			Name:  "version",
			Usage: "Print the ipfs-cluster version",
			Action: func(c *cli.Context) error {
				if c := ipfscluster.Commit; len(c) >= 8 {
					fmt.Printf("%s-%s\n", ipfscluster.Version, c)
					return nil
				}

				fmt.Printf("%s\n", ipfscluster.Version)
				return nil
			},
		},
	}

	app.Before = func(c *cli.Context) error {
		absPath, err := filepath.Abs(c.String("config"))
		if err != nil {
			return err
		}

		configPath = filepath.Join(absPath, DefaultConfigFile)

		setupLogLevel(c.String("loglevel"))
		if c.Bool("debug") {
			setupDebug()
		}

		locker = &lock{path: absPath}

		return nil
	}

	app.Action = run

	app.Run(os.Args)
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
