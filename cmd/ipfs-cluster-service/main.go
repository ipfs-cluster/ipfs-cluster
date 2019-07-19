// The ipfs-cluster-service application.
package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	ipfscluster "github.com/ipfs/ipfs-cluster"
	"github.com/ipfs/ipfs-cluster/config"
	"github.com/ipfs/ipfs-cluster/pstoremgr"
	"github.com/ipfs/ipfs-cluster/version"
	peer "github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"

	semver "github.com/blang/semver"
	logging "github.com/ipfs/go-log"
	cli "github.com/urfave/cli"
)

// ProgramName of this application
const programName = `ipfs-cluster-service`

// flag defaults
const (
	defaultConsensus  = "raft"
	defaultAllocation = "disk-freespace"
	defaultPinTracker = "map"
	defaultLogLevel   = "info"
)

const (
	stateCleanupPrompt           = "The peer state will be removed.  Existing pins may be lost."
	configurationOverwritePrompt = "The configuration file will be overwritten."
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


%s needs valid configuration and identity files to run.
These are independent from IPFS. The identity includes its own
libp2p key-pair. They can be initialized with "init" and their
default locations are ~/%s/%s
and ~/%s/%s.

For feedback, bug reports or any additional information, visit
https://github.com/ipfs/ipfs-cluster.


EXAMPLES:

Initial configuration:

$ ipfs-cluster-service init

Launch a cluster:

$ ipfs-cluster-service daemon

Launch a peer and join existing cluster:

$ ipfs-cluster-service daemon --bootstrap /ip4/192.168.1.2/tcp/9096/ipfs/QmPSoSaPXpyunaBwHs1rZBKYSqRV4bLRk32VGYLuvdrypL
`,
	programName,
	programName,
	DefaultFolder,
	DefaultConfigFile,
	DefaultFolder,
	DefaultIdentityFile,
)

var logger = logging.Logger("service")

// Default location for the configurations and data
var (
	// DefaultFolder is the name of the cluster folder
	DefaultFolder = ".ipfs-cluster"
	// DefaultPath is set on init() to $HOME/DefaultFolder
	// and holds all the ipfs-cluster data
	DefaultPath string
	// The name of the configuration file inside DefaultPath
	DefaultConfigFile = "service.json"
	// The name of the identity file inside DefaultPath
	DefaultIdentityFile = "identity.json"
)

var (
	configPath   string
	identityPath string
)

func init() {
	// Set build information.
	if build, err := semver.NewBuildVersion(commit); err == nil {
		version.Version.Build = []string{"git" + build}
	}

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

	DefaultPath = filepath.Join(home, DefaultFolder)
}

func out(m string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, m, a...)
}

func checkErr(doing string, err error, args ...interface{}) {
	if err != nil {
		if len(args) > 0 {
			doing = fmt.Sprintf(doing, args...)
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
	app := cli.NewApp()
	app.Name = programName
	app.Usage = "IPFS Cluster node"
	app.Description = Description
	//app.Copyright = "© Protocol Labs, Inc."
	app.Version = version.Version.String()
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

	app.Before = func(c *cli.Context) error {
		absPath, err := filepath.Abs(c.String("config"))
		if err != nil {
			return err
		}

		configPath = filepath.Join(absPath, DefaultConfigFile)
		identityPath = filepath.Join(absPath, DefaultIdentityFile)

		setupLogLevel(c.String("loglevel"))
		if c.Bool("debug") {
			setupDebug()
		}

		locker = &lock{path: absPath}

		return nil
	}

	app.Commands = []cli.Command{
		{
			Name:  "init",
			Usage: "Creates a configuration and generates an identity",
			Description: fmt.Sprintf(`
This command will initialize a new %s configuration file and, if it
does already exist, generate a new %s for %s.

By default, %s requires a cluster secret. This secret will be
automatically generated, but can be manually provided with --custom-secret
(in which case it will be prompted), or by setting the CLUSTER_SECRET
environment variable.

Note that the --force first-level-flag allows to overwrite an existing
configuration with default values. To generate a new identity, please
remove the %s file first and clean any Raft state.
`,
				DefaultConfigFile,
				DefaultIdentityFile,
				programName,
				programName,
				DefaultIdentityFile,
			),
			ArgsUsage: " ",
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "custom-secret, s",
					Usage: "prompt for the cluster secret",
				},
				cli.StringFlag{
					Name:  "peers",
					Usage: "comma-separated list of multiaddresses to init with",
				},
			},
			Action: func(c *cli.Context) error {
				userSecret, userSecretDefined := userProvidedSecret(c.Bool("custom-secret"))

				cfgMgr, cfgs := makeConfigs()
				defer cfgMgr.Shutdown() // wait for saves

				configExists := false
				if _, err := os.Stat(configPath); !os.IsNotExist(err) {
					configExists = true
				}

				identityExists := false
				if _, err := os.Stat(identityPath); !os.IsNotExist(err) {
					identityExists = true
				}

				if configExists || identityExists {
					// cluster might be running
					// acquire lock for config folder
					locker.lock()
					defer locker.tryUnlock()
				}

				if configExists {
					confirm := fmt.Sprintf(
						"%s Continue? [y/n]:",
						configurationOverwritePrompt,
					)

					// --force allows override of the prompt
					if !c.GlobalBool("force") {
						if !yesNoPrompt(confirm) {
							return nil
						}
					}
				}

				// Generate defaults for all registered components
				err := cfgMgr.Default()
				checkErr("generating default configuration", err)

				err = cfgMgr.ApplyEnvVars()
				checkErr("applying environment variables to configuration", err)

				// Set user secret
				if userSecretDefined {
					cfgs.clusterCfg.Secret = userSecret
				}

				peersOpt := c.String("peers")
				multiAddrs := []ma.Multiaddr{}
				if peersOpt != "" {
					addrs := strings.Split(peersOpt, ",")

					for _, addr := range addrs {
						addr = strings.TrimSpace(addr)
						multiAddr, err := ma.NewMultiaddr(addr)
						checkErr("parsing peer multiaddress: "+addr, err)
						multiAddrs = append(multiAddrs, multiAddr)
					}

					peers := ipfscluster.PeersFromMultiaddrs(multiAddrs)
					cfgs.crdtCfg.TrustedPeers = peers
					cfgs.raftCfg.InitPeerset = peers
				}

				// Save
				saveConfig(cfgMgr)

				if !identityExists {
					// Create a new identity and save it
					ident, err := config.NewIdentity()
					checkErr("generating an identity", err)

					err = ident.ApplyEnvVars()
					checkErr("applying environment variables to the identity", err)

					err = ident.SaveJSON(identityPath)
					checkErr("saving "+DefaultIdentityFile, err)
					out("new identity written to %s\n", identityPath)
				}
				if peersOpt != "" {
					_, _, cfgs := makeAndLoadConfigs()

					peerstorePath := cfgs.clusterCfg.GetPeerstorePath()
					peerManager := pstoremgr.New(context.Background(), nil, peerstorePath)

					addrInfos, err := peer.AddrInfosFromP2pAddrs(multiAddrs...)
					checkErr("getting address infos from peer multiaddresses", err)
					peerManager.SavePeerstore(addrInfos)
					out("peerstore written to %s\n", peerstorePath)
				}

				return nil
			},
		},
		{
			Name:  "daemon",
			Usage: "Runs the IPFS Cluster peer (default)",
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "upgrade, u",
					Usage: "run state migrations before starting (deprecated/unused)",
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
					Name:  "consensus",
					Value: defaultConsensus,
					Usage: "shared state management provider [raft,crdt]",
				},
				cli.StringFlag{
					Name:  "alloc, a",
					Value: defaultAllocation,
					Usage: "allocation strategy to use [disk-freespace,disk-reposize,numpin].",
				},
				cli.StringFlag{
					Name:   "pintracker",
					Value:  defaultPinTracker,
					Hidden: true,
					Usage:  "pintracker to use [map,stateless].",
				},
				cli.BoolFlag{
					Name:  "stats",
					Usage: "enable stats collection",
				},
				cli.BoolFlag{
					Name:  "tracing",
					Usage: "enable tracing collection",
				},
			},
			Action: daemon,
		},
		{
			Name:  "state",
			Usage: "Manages the peer's consensus state (pinset)",
			Subcommands: []cli.Command{
				{
					Name:  "export",
					Usage: "save the state to a JSON file",
					Description: `
This command dumps the current cluster pinset (state) as a JSON file. The
resulting file can be used to migrate, restore or backup a Cluster peer.
By default, the state will be printed to stdout.
`,
					Flags: []cli.Flag{
						cli.StringFlag{
							Name:  "file, f",
							Value: "",
							Usage: "writes to an output file",
						},
						cli.StringFlag{
							Name:  "consensus",
							Value: "raft",
							Usage: "consensus component to export data from [raft, crdt]",
						},
					},
					Action: func(c *cli.Context) error {
						locker.lock()
						defer locker.tryUnlock()

						var w io.WriteCloser
						var err error
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

						cfgMgr, ident, cfgs := makeAndLoadConfigs()
						defer cfgMgr.Shutdown()
						mgr := newStateManager(c.String("consensus"), ident, cfgs)
						checkErr("exporting state", mgr.ExportState(w))
						logger.Info("state successfully exported")
						return nil
					},
				},
				{
					Name:  "import",
					Usage: "load the state from a file produced by 'export'",
					Description: `
This command reads in an exported pinset (state) file and replaces the
existing one. This can be used, for example, to restore a Cluster peer from a
backup.

If an argument is provided, it will be treated it as the path of the file
to import. If no argument is provided, stdin will be used.
`,
					Flags: []cli.Flag{
						cli.BoolFlag{
							Name:  "force, f",
							Usage: "skips confirmation prompt",
						},
						cli.StringFlag{
							Name:  "consensus",
							Value: "raft",
							Usage: "consensus component to export data from [raft, crdt]",
						},
					},
					Action: func(c *cli.Context) error {
						locker.lock()
						defer locker.tryUnlock()

						confirm := "The pinset (state) of this peer "
						confirm += "will be replaced. Continue? [y/n]:"
						if !c.Bool("force") && !yesNoPrompt(confirm) {
							return nil
						}

						// Get the importing file path
						importFile := c.Args().First()
						var r io.ReadCloser
						var err error
						if importFile == "" {
							r = os.Stdin
							fmt.Println("reading from stdin, Ctrl-D to finish")
						} else {
							r, err = os.Open(importFile)
							checkErr("reading import file", err)
						}
						defer r.Close()

						cfgMgr, ident, cfgs := makeAndLoadConfigs()
						defer cfgMgr.Shutdown()
						mgr := newStateManager(c.String("consensus"), ident, cfgs)
						checkErr("importing state", mgr.ImportState(r))
						logger.Info("state successfully imported.  Make sure all peers have consistent states")
						return nil
					},
				},
				{
					Name:  "cleanup",
					Usage: "remove persistent data",
					Description: `
This command removes any persisted consensus data in this peer, including the
current pinset (state). The next start of the peer will be like the first start
to all effects. Peers may need to bootstrap and sync from scratch after this.
`,
					Flags: []cli.Flag{
						cli.BoolFlag{
							Name:  "force, f",
							Usage: "skip confirmation prompt",
						},
						cli.StringFlag{
							Name:  "consensus",
							Value: "raft",
							Usage: "consensus component to export data from [raft, crdt]",
						},
					},
					Action: func(c *cli.Context) error {
						locker.lock()
						defer locker.tryUnlock()

						confirm := fmt.Sprintf(
							"%s Continue? [y/n]:",
							stateCleanupPrompt,
						)
						if !c.Bool("force") && !yesNoPrompt(confirm) {
							return nil
						}

						cfgMgr, ident, cfgs := makeAndLoadConfigs()
						defer cfgMgr.Shutdown()
						mgr := newStateManager(c.String("consensus"), ident, cfgs)
						checkErr("cleaning state", mgr.Clean())
						logger.Info("data correctly cleaned up")
						return nil
					},
				},
			},
		},
		{
			Name:  "version",
			Usage: "Prints the ipfs-cluster version",
			Action: func(c *cli.Context) error {
				fmt.Printf("%s\n", version.Version)
				return nil
			},
		},
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
	if enterSecret {
		secret := promptUser("Enter cluster secret (32-byte hex string): ")
		decodedSecret, err := ipfscluster.DecodeClusterSecret(secret)
		checkErr("parsing user-provided secret", err)
		return decodedSecret, true
	}

	return nil, false
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
