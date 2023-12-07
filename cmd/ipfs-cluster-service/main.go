// The ipfs-cluster-service application.
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	ipfslite "github.com/hsanjuan/ipfs-lite"
	ipfscluster "github.com/ipfs-cluster/ipfs-cluster"
	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/ipfs-cluster/ipfs-cluster/cmdutils"
	"github.com/ipfs-cluster/ipfs-cluster/consensus/crdt"
	"github.com/ipfs-cluster/ipfs-cluster/pstoremgr"
	"github.com/ipfs-cluster/ipfs-cluster/version"
	peer "github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"

	semver "github.com/blang/semver"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	dscrdt "github.com/ipfs/go-ds-crdt"
	logging "github.com/ipfs/go-log/v2"
	cli "github.com/urfave/cli"
)

// ProgramName of this application
const programName = "ipfs-cluster-service"

// flag defaults
const (
	defaultLogLevel  = "info"
	defaultConsensus = "crdt"
	defaultDatastore = "pebble"
)

const (
	stateCleanupPrompt           = "The peer state will be removed.  Existing pins may be lost."
	configurationOverwritePrompt = "The configuration file will be overwritten."
)

// We store a commit id here
var commit string

// Description provides a short summary of the functionality of this tool
var Description = fmt.Sprintf(`
%s runs an IPFS Cluster peer.

A peer participates in the cluster consensus, follows a distributed log
of pinning and unpinning requests and manages pinning operations to a
configured IPFS daemon.

This peer also provides an API for cluster management, an IPFS Proxy API which
forwards requests to IPFS and a number of components for internal communication
using LibP2P. This is a simplified view of the components:

             +------------------+
             | ipfs-cluster-ctl |
             +---------+--------+
                       |
                       | HTTP(s)
ipfs-cluster-service   |                           HTTP
+----------+--------+--v--+----------------------+      +-------------+
| RPC      | Peer 1 | API | IPFS Connector/Proxy +------> IPFS daemon |
+----^-----+--------+-----+----------------------+      +-------------+
     | libp2p
     |
+----v-----+--------+-----+----------------------+      +-------------+
| RPC      | Peer 2 | API | IPFS Connector/Proxy +------> IPFS daemon |
+----^-----+--------+-----+----------------------+      +-------------+
     |
     |
+----v-----+--------+-----+----------------------+      +-------------+
| RPC      | Peer 3 | API | IPFS Connector/Proxy +------> IPFS daemon |
+----------+--------+-----+----------------------+      +-------------+


%s needs valid configuration and identity files to run.
These are independent from IPFS. The identity includes its own
libp2p key-pair. They can be initialized with "init" and their
default locations are ~/%s/%s
and ~/%s/%s.

For feedback, bug reports or any additional information, visit
https://github.com/ipfs-cluster/ipfs-cluster.


EXAMPLES:

Initial configuration:

$ ipfs-cluster-service init

Launch a cluster:

$ ipfs-cluster-service daemon

Launch a peer and join existing cluster:

$ ipfs-cluster-service daemon --bootstrap /ip4/192.168.1.2/tcp/9096/p2p/QmPSoSaPXpyunaBwHs1rZBKYSqRV4bLRk32VGYLuvdrypL

Customize logs using --loglevel flag. To customize component-level
logging pass a comma-separated list of component-identifer:log-level
pair or without identifier for overall loglevel. Valid loglevels
are critical, error, warning, notice, info and debug.

$ ipfs-cluster-service --loglevel info,cluster:debug,pintracker:debug daemon
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
	app.Usage = "IPFS Cluster peer"
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
			Name:   "loglevel, l",
			EnvVar: "IPFS_CLUSTER_LOG_LEVEL",
			Usage:  "set overall and component-wise log levels",
		},
	}

	app.Before = func(c *cli.Context) error {
		absPath, err := filepath.Abs(c.String("config"))
		if err != nil {
			return err
		}

		configPath = filepath.Join(absPath, DefaultConfigFile)
		identityPath = filepath.Join(absPath, DefaultIdentityFile)

		err = setupLogLevel(c.Bool("debug"), c.String("loglevel"))
		if err != nil {
			return err
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

If the optional [source-url] is given, the generated configuration file
will refer to it. The source configuration will be fetched from its source
URL during the launch of the daemon. If not, a default standard configuration
file will be created.

In the latter case, a cluster secret will be generated as required
by %s. Alternatively, this secret can be manually
provided with --custom-secret (in which case it will be prompted), or
by setting the CLUSTER_SECRET environment variable.

The --consensus flag allows to select an alternative consensus components for
in the newly-generated configuration.

Note that the --force flag allows to overwrite an existing
configuration with default values. To generate a new identity, please
remove the %s file first and clean any Raft state.

By default, an empty peerstore file will be created too. Initial contents can
be provided with the --peers flag. Depending on the chosen consensus, the
"trusted_peers" list in the "crdt" configuration section and the
"init_peerset" list in the "raft" configuration section will be prefilled to
the peer IDs in the given multiaddresses.
`,

				DefaultConfigFile,
				DefaultIdentityFile,
				programName,
				programName,
				DefaultIdentityFile,
			),
			ArgsUsage: "[http-source-url]",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "consensus",
					Usage: "select consensus: 'crdt' or 'raft'",
					Value: defaultConsensus,
				},
				cli.StringFlag{
					Name:  "datastore",
					Usage: "select datastore: 'badger', 'badger3', 'leveldb' or 'pebble'",
					Value: defaultDatastore,
				},
				cli.BoolFlag{
					Name:  "custom-secret, s",
					Usage: "prompt for the cluster secret (when no source specified)",
				},
				cli.StringFlag{
					Name:  "peers",
					Usage: "comma-separated list of multiaddresses to init with (see help)",
				},
				cli.BoolFlag{
					Name:  "force, f",
					Usage: "overwrite configuration without prompting",
				},
				cli.BoolFlag{
					Name:  "randomports",
					Usage: "configure random ports to listen on instead of defaults",
				},
			},
			Action: func(c *cli.Context) error {
				consensus := c.String("consensus")
				switch consensus {
				case "raft", "crdt":
				default:
					checkErr("choosing consensus", errors.New("flag value must be set to 'raft' or 'crdt'"))
				}

				datastore := c.String("datastore")
				switch datastore {
				case "leveldb", "badger", "badger3", "pebble":
				default:
					checkErr("choosing datastore", errors.New("flag value must be set to 'leveldb', 'badger', 'badger3' or 'pebble'"))
				}

				cfgHelper := cmdutils.NewConfigHelper(configPath, identityPath, consensus, datastore)
				defer cfgHelper.Manager().Shutdown() // wait for saves

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
					if !c.Bool("force") {
						if !yesNoPrompt(confirm) {
							return nil
						}
					}
				}

				// Set url. If exists, it will be the only thing saved.
				cfgHelper.Manager().Source = c.Args().First()

				// Generate defaults for all registered components
				err := cfgHelper.Manager().Default()
				checkErr("generating default configuration", err)

				if c.Bool("randomports") {
					cfgs := cfgHelper.Configs()

					cfgs.Cluster.ListenAddr, err = cmdutils.RandomizePorts(cfgs.Cluster.ListenAddr)
					checkErr("randomizing ports", err)
					cfgs.Restapi.HTTPListenAddr, err = cmdutils.RandomizePorts(cfgs.Restapi.HTTPListenAddr)
					checkErr("randomizing ports", err)
					cfgs.Ipfsproxy.ListenAddr, err = cmdutils.RandomizePorts(cfgs.Ipfsproxy.ListenAddr)
					checkErr("randomizing ports", err)
					cfgs.Pinsvcapi.HTTPListenAddr, err = cmdutils.RandomizePorts(cfgs.Pinsvcapi.HTTPListenAddr)
					checkErr("randomizing ports", err)
				}
				err = cfgHelper.Manager().ApplyEnvVars()
				checkErr("applying environment variables to configuration", err)

				userSecret, userSecretDefined := userProvidedSecret(c.Bool("custom-secret") && !c.Args().Present())
				// Set user secret
				if userSecretDefined {
					cfgHelper.Configs().Cluster.Secret = userSecret
				}

				peersOpt := c.String("peers")
				var multiAddrs []ma.Multiaddr
				if peersOpt != "" {
					addrs := strings.Split(peersOpt, ",")

					for _, addr := range addrs {
						addr = strings.TrimSpace(addr)
						multiAddr, err := ma.NewMultiaddr(addr)
						checkErr("parsing peer multiaddress: "+addr, err)
						multiAddrs = append(multiAddrs, multiAddr)
					}

					peers := ipfscluster.PeersFromMultiaddrs(multiAddrs)
					cfgHelper.Configs().Crdt.TrustAll = false
					cfgHelper.Configs().Crdt.TrustedPeers = peers
					cfgHelper.Configs().Raft.InitPeerset = peers
				}

				// Save config. Creates the folder.
				// Sets BaseDir in components.
				checkErr("saving default configuration", cfgHelper.SaveConfigToDisk())
				out("configuration written to %s.\n", configPath)

				if !identityExists {
					ident := cfgHelper.Identity()
					err := ident.Default()
					checkErr("generating an identity", err)

					err = ident.ApplyEnvVars()
					checkErr("applying environment variables to the identity", err)

					err = cfgHelper.SaveIdentityToDisk()
					checkErr("saving "+DefaultIdentityFile, err)
					out("new identity written to %s\n", identityPath)
				}

				// Initialize peerstore file - even if empty
				peerstorePath := cfgHelper.Configs().Cluster.GetPeerstorePath()
				peerManager := pstoremgr.New(context.Background(), nil, peerstorePath)
				addrInfos, err := peer.AddrInfosFromP2pAddrs(multiAddrs...)
				checkErr("getting AddrInfos from peer multiaddresses", err)
				err = peerManager.SavePeerstore(addrInfos)
				checkErr("saving peers to peerstore", err)
				if l := len(multiAddrs); l > 0 {
					out("peerstore written to %s with %d entries.\n", peerstorePath, len(multiAddrs))
				} else {
					out("new empty peerstore written to %s.\n", peerstorePath)
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
				cli.StringFlag{
					Name:  "bootstrap, j",
					Usage: "join a cluster providing a comma-separated list of existing peers multiaddress(es)",
				},
				cli.BoolFlag{
					Name:   "leave, x",
					Usage:  "remove peer from cluster on exit. Overrides \"leave_on_shutdown\"",
					Hidden: true,
				},
				cli.BoolFlag{
					Name:  "stats",
					Usage: "enable stats collection",
				},
				cli.BoolFlag{
					Name:  "tracing",
					Usage: "enable tracing collection",
				},
				cli.BoolFlag{
					Name:  "no-trust",
					Usage: "do not trust bootstrap peers (only for \"crdt\" consensus)",
				},
			},
			Action: daemon,
		},
		{
			Name:  "state",
			Usage: "Manages the peer's persistent state (pinset)",
			Subcommands: []cli.Command{
				{
					Name:  "crdt",
					Usage: "CRDT-state commands",
					Before: func(c *cli.Context) error {
						// Load all the configurations and identity
						cfgHelper, err := cmdutils.NewLoadedConfigHelper(configPath, identityPath)
						cfgs := cfgHelper.Configs()
						checkErr("loading configurations", err)
						defer cfgHelper.Manager().Shutdown()

						if cfgHelper.GetConsensus() != cfgs.Crdt.ConfigKey() {
							checkErr("", errors.New("crdt subcommands can only be run on peers initialized with crdt consensus"))
						}
						return nil
					},

					Subcommands: []cli.Command{
						{
							Name:  "info",
							Usage: "Print information about the CRDT store",
							Description: `
This commands prints basic information: current heads, dirty flag etc.
`,
							Flags: []cli.Flag{},
							Action: func(c *cli.Context) error {
								locker.lock()
								defer locker.tryUnlock()

								crdt := getCrdt()
								info := crdt.InternalStats()
								fmt.Printf(
									"Number of heads: %d. Current max-height: %d. Dirty: %t\nHeads: %s",
									len(info.Heads),
									info.MaxHeight,
									crdt.IsDirty(),
									info.Heads,
								)
								return nil
							},
						},
						{
							Name:  "dot",
							Usage: "Write the CRDT-DAG as DOT file",
							Description: `
This command generates a DOT file representing the CRDT-DAG of this node.
The DOT file can then be visualized, converted to SVG etc.

This is a debugging command to visualize how the DAG looks like, whether there
is a lot of branching etc. large DAGs will generate large DOT files.
Use with caution!
`,
							Flags: []cli.Flag{
								cli.StringFlag{
									Name:  "file, f",
									Value: "",
									Usage: "writes to file instead of stdout",
								},
							},
							Action: func(c *cli.Context) error {
								locker.lock()
								defer locker.tryUnlock()

								crdt := getCrdt()

								var err error
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

								// 256KiB of buffer size.
								buf := bufio.NewWriterSize(w, 1<<18)
								defer buf.Flush()

								logger.Info("initiating CDRT-DAG DOT file export. Export might take a long time on large graphs")
								checkErr("generating graph", crdt.DotDAG(buf))
								logger.Info("dot file ")
								return nil

							},
						},
						{
							Name:  "mark-dirty",
							Usage: "Marks the CRDT-store as dirty",
							Description: `
Marking the CRDT store as dirty will force-run a Repair operation on the next
run (i.e. next time the cluster peer is started).
`,
							Flags: []cli.Flag{},
							Action: func(c *cli.Context) error {
								locker.lock()
								defer locker.tryUnlock()

								crdt := getCrdt()
								crdt.MarkDirty()
								fmt.Println("Datastore marked 'dirty'")
								return nil
							},
						},
						{
							Name:  "mark-clean",
							Usage: "Marks the CRDT-store as clean",
							Description: `
This command remove the dirty-mark on the CRDT-store, which means no
DAG operations will be run.
`,
							Flags: []cli.Flag{},
							Action: func(c *cli.Context) error {
								locker.lock()
								defer locker.tryUnlock()

								crdt := getCrdt()
								crdt.MarkClean()
								fmt.Println("Datastore marked 'clean'")
								return nil

							},
						},
					},
				},
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
					},
					Action: func(c *cli.Context) error {
						locker.lock()
						defer locker.tryUnlock()

						mgr := getStateManager()

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

						buf := bufio.NewWriter(w)
						defer func() {
							buf.Flush()
							w.Close()
						}()
						checkErr("exporting state", mgr.ExportState(buf))
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
						cli.IntFlag{
							Name:  "replication-min, rmin",
							Value: 0,
							Usage: "Overwrite replication-factor-min for all pins on import",
						},
						cli.IntFlag{
							Name:  "replication-max, rmax",
							Value: 0,
							Usage: "Overwrite replication-factor-max for all pins on import",
						},
						cli.StringFlag{
							Name:  "allocations, allocs",
							Usage: "Overwrite allocations for all pins on import. Comma-separated list of peer IDs",
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

						// importState allows overwriting of some options on import
						opts := api.PinOptions{
							ReplicationFactorMin: c.Int("replication-min"),
							ReplicationFactorMax: c.Int("replication-max"),
							UserAllocations:      api.StringsToPeers(strings.Split(c.String("allocations"), ",")),
						}

						mgr := getStateManager()

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

						buf := bufio.NewReader(r)

						checkErr("importing state", mgr.ImportState(buf, opts))
						logger.Info("state successfully imported.  Make sure all peers have consistent states")
						return nil
					},
				},
				{
					Name:  "cleanup",
					Usage: "remove persistent data",
					Description: `
This command removes any persisted consensus data in this peer, including the
current pinset (state). The next start of the peer will be like a first start
to all effects. Peers may need to bootstrap and sync from scratch after this.
`,
					Flags: []cli.Flag{
						cli.BoolFlag{
							Name:  "force, f",
							Usage: "skip confirmation prompt",
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

						mgr := getStateManager()
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

func setupLogLevel(debug bool, l string) error {
	// if debug is set to true, log everything in debug level
	if debug {
		ipfscluster.SetFacilityLogLevel("*", "DEBUG")
		return nil
	}

	compLogLevel := strings.Split(l, ",")
	var logLevel string
	compLogFacs := make(map[string]string)
	// get overall log level and component-wise log levels from arguments
	for _, cll := range compLogLevel {
		if cll == "" {
			continue
		}
		identifierToLevel := strings.Split(cll, ":")
		var lvl string
		var comp string
		switch len(identifierToLevel) {
		case 1:
			lvl = identifierToLevel[0]
			comp = "all"
		case 2:
			lvl = identifierToLevel[1]
			comp = identifierToLevel[0]
		default:
			return errors.New("log level not in expected format \"identifier:loglevel\" or \"loglevel\"")
		}

		_, ok := compLogFacs[comp]
		if ok {
			fmt.Printf("overwriting existing %s log level\n", comp)
		}
		compLogFacs[comp] = lvl
	}

	logLevel, ok := compLogFacs["all"]
	if !ok {
		logLevel = defaultLogLevel
	} else {
		delete(compLogFacs, "all")
	}

	// log service with logLevel
	ipfscluster.SetFacilityLogLevel("service", logLevel)

	logfacs := make(map[string]string)

	// fill component-wise log levels
	for identifier, level := range compLogFacs {
		logfacs[identifier] = level
	}

	// Set the values for things not set by the user or for
	// things set by "all".
	for key := range ipfscluster.LoggingFacilities {
		if _, ok := logfacs[key]; !ok {
			logfacs[key] = logLevel
		}
	}

	// For Extra facilities, set the defaults per logging.go unless
	// manually set
	for key, defaultLvl := range ipfscluster.LoggingFacilitiesExtra {
		if _, ok := logfacs[key]; !ok {
			logfacs[key] = defaultLvl
		}
	}

	for identifier, level := range logfacs {
		ipfscluster.SetFacilityLogLevel(identifier, level)
	}

	return nil
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

func getStateManager() cmdutils.StateManager {
	cfgHelper, err := cmdutils.NewLoadedConfigHelper(
		configPath,
		identityPath,
	)
	checkErr("loading configurations", err)
	cfgHelper.Manager().Shutdown()
	mgr, err := cmdutils.NewStateManagerWithHelper(cfgHelper)
	checkErr("creating state manager", err)
	return mgr
}

func getCrdt() *dscrdt.Datastore {
	// Load all the configurations and identity
	cfgHelper, err := cmdutils.NewLoadedConfigHelper(configPath, identityPath)
	checkErr("loading configurations", err)
	defer cfgHelper.Manager().Shutdown()

	// Get a state manager and the datastore
	mgr, err := cmdutils.NewStateManagerWithHelper(cfgHelper)
	checkErr("creating state manager", err)
	store, err := mgr.GetStore()
	checkErr("opening datastore", err)
	batching, ok := store.(datastore.Batching)
	if !ok {
		checkErr("", errors.New("no batching store"))
	}

	crdtNs := cfgHelper.Configs().Crdt.DatastoreNamespace

	var blocksDatastore datastore.Batching = namespace.Wrap(
		batching,
		datastore.NewKey(crdtNs).ChildString(crdt.BlocksNs),
	)

	ipfs, err := ipfslite.New(
		context.Background(),
		blocksDatastore,
		nil,
		nil,
		nil,
		&ipfslite.Config{
			Offline: true,
		},
	)
	checkErr("creating ipfs-lite offline node", err)

	opts := dscrdt.DefaultOptions()
	opts.RepairInterval = 0
	crdt, err := dscrdt.New(
		batching,
		datastore.NewKey(crdtNs),
		ipfs,
		nil,
		opts,
	)
	checkErr("creating crdt node", err)
	return crdt
}
