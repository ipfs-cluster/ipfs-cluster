// The ipfs-cluster-ctl application.
package main

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"go.opencensus.io/exporter/jaeger"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/api/rest/client"
	uuid "github.com/satori/go.uuid"

	cid "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	peer "github.com/libp2p/go-libp2p-peer"
	ma "github.com/multiformats/go-multiaddr"
	cli "github.com/urfave/cli"
)

const programName = `ipfs-cluster-ctl`

// Version is the cluster-ctl tool version. It should match
// the IPFS cluster's version
const Version = "0.9.0"

var (
	defaultHost          = "/ip4/127.0.0.1/tcp/9094"
	defaultTimeout       = 0
	defaultUsername      = ""
	defaultPassword      = ""
	defaultWaitCheckFreq = time.Second
	defaultAddParams     = api.DefaultAddParams()
)

var logger = logging.Logger("cluster-ctl")

var tracer *jaeger.Exporter

var globalClient client.Client

// Description provides a short summary of the functionality of this tool
var Description = fmt.Sprintf(`
%s is a tool to manage IPFS Cluster nodes.
Use "%s help" to list all available commands and
"%s help <command>" to get usage information for a
specific one.

%s uses the IPFS Cluster API to perform requests and display
responses in a user-readable format. The location of the IPFS
Cluster server is assumed to be %s, but can be
configured with the --host option. To use the secure libp2p-http
API endpoint, use "--host" with the full cluster libp2p listener
address (including the "/ipfs/<peerID>" part), and --secret (the
32-byte cluster secret as it appears in the cluster configuration).

For feedback, bug reports or any additional information, visit
https://github.com/ipfs/ipfs-cluster.
`,
	programName,
	programName,
	programName,
	programName,
	defaultHost)

type peerAddBody struct {
	Addr string `json:"peer_multiaddress"`
}

func out(m string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, m, a...)
}

func checkErr(doing string, err error) {
	if err != nil {
		out("error %s: %s\n", doing, err)
		os.Exit(1)
	}
}

func main() {
	ctx := context.Background()

	app := cli.NewApp()
	app.Name = programName
	app.Usage = "CLI for IPFS Cluster"
	app.Description = Description
	app.Version = Version
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "host, l",
			Value: defaultHost,
			Usage: "Cluster's HTTP or LibP2P-HTTP API endpoint",
		},
		cli.StringFlag{
			Name:  "secret",
			Value: "",
			Usage: "cluster secret (32 byte pnet-key) as needed. Only when using the LibP2P endpoint",
		},
		cli.BoolFlag{
			Name:  "https, s",
			Usage: "use https to connect to the API",
		},
		cli.BoolFlag{
			Name:  "no-check-certificate",
			Usage: "do not verify server TLS certificate. only valid with --https flag",
		},
		cli.StringFlag{
			Name:  "encoding, enc",
			Value: "text",
			Usage: "output format encoding [text, json]",
		},
		cli.IntFlag{
			Name:  "timeout, t",
			Value: defaultTimeout,
			Usage: "number of seconds to wait before timing out a request",
		},
		cli.BoolFlag{
			Name:  "debug, d",
			Usage: "set debug log level",
		},
		cli.StringFlag{
			Name: "basic-auth",
			Usage: `<username>[:<password>] specify BasicAuth credentials for server that
requires authorization. implies --https, which you can disable with --force-http`,
			EnvVar: "CLUSTER_CREDENTIALS",
		},
		cli.BoolFlag{
			Name:  "force-http, f",
			Usage: "force HTTP. only valid when using BasicAuth",
		},
	}

	app.Before = func(c *cli.Context) error {
		cfg := &client.Config{}

		if c.Bool("debug") {
			logging.SetLogLevel("cluster-ctl", "debug")
			cfg.LogLevel = "debug"
			logger.Debug("debug level enabled")
		}

		addr, err := ma.NewMultiaddr(c.String("host"))
		checkErr("parsing host multiaddress", err)

		cfg.APIAddr = addr
		if hexSecret := c.String("secret"); hexSecret != "" {
			secret, err := hex.DecodeString(hexSecret)
			checkErr("parsing secret", err)
			cfg.ProtectorKey = secret
		}

		cfg.Timeout = time.Duration(c.Int("timeout")) * time.Second

		if client.IsPeerAddress(cfg.APIAddr) && c.Bool("https") {
			logger.Warning("Using libp2p-http. SSL flags will be ignored")
		}

		cfg.SSL = c.Bool("https")
		cfg.NoVerifyCert = c.Bool("no-check-certificate")
		user, pass := parseCredentials(c.String("basic-auth"))
		cfg.Username = user
		cfg.Password = pass
		if user != "" && !cfg.SSL && !c.Bool("force-http") {
			logger.Warning("SSL automatically enabled with basic auth credentials. Set \"force-http\" to disable")
			cfg.SSL = true
		}

		enc := c.String("encoding")
		if enc != "text" && enc != "json" {
			checkErr("", errors.New("unsupported encoding"))
		}

		globalClient, err = client.NewDefaultClient(cfg)
		checkErr("creating API client", err)

		// TODO: need to figure out best way to configure tracing for ctl
		// leaving the following as it is still useful for local debugging.
		// tracingCfg := &observations.Config{}
		// tracingCfg.Default()
		// tracingCfg.EnableTracing = true
		// tracingCfg.TracingServiceName = "cluster-ctl"
		// tracingCfg.TracingSamplingProb = 1
		// tracer = observations.SetupTracing(tracingCfg)
		return nil
	}
	app.After = func(c *cli.Context) error {
		// TODO: need to figure out best way to configure tracing for ctl
		// leaving the following as it is still useful for local debugging.
		// tracer.Flush()
		return nil
	}

	app.Commands = []cli.Command{
		{
			Name:  "id",
			Usage: "Retrieve peer information",
			Description: `
This command displays information about the peer that the tool is contacting
(usually running in localhost).
`,
			Flags: []cli.Flag{},
			Action: func(c *cli.Context) error {
				resp, cerr := globalClient.ID(ctx)
				formatResponse(c, resp, cerr)
				return nil
			},
		},
		{
			Name:        "peers",
			Usage:       "List and manage IPFS Cluster peers",
			Description: "List and manage IPFS Cluster peers",
			Subcommands: []cli.Command{
				{
					Name:  "ls",
					Usage: "list the nodes participating in the IPFS Cluster",
					Description: `
This command provides a list of the ID information of all the peers in the Cluster.
`,
					Flags:     []cli.Flag{},
					ArgsUsage: " ",
					Action: func(c *cli.Context) error {
						resp, cerr := globalClient.Peers(ctx)
						formatResponse(c, resp, cerr)
						return nil
					},
				},
				{
					Name:  "rm",
					Usage: "remove a peer from the Cluster",
					Description: `
This command removes a peer from the cluster. If the peer is online, it will
automatically shut down. All other cluster peers should be online for the
operation to succeed, otherwise some nodes may be left with an outdated list of
cluster peers.
`,
					ArgsUsage: "<peer ID>",
					Flags:     []cli.Flag{},
					Action: func(c *cli.Context) error {
						pid := c.Args().First()
						p, err := peer.IDB58Decode(pid)
						checkErr("parsing peer ID", err)
						cerr := globalClient.PeerRm(ctx, p)
						formatResponse(c, nil, cerr)
						return nil
					},
				},
			},
		},
		{
			Name:      "add",
			Usage:     "Add a file or directory to ipfs and pin it in the cluster",
			ArgsUsage: "<path>",
			Description: `
Add allows to add and replicate content to several ipfs daemons, performing
a Cluster Pin operation on success. It takes elements from local paths as
well as from web URLs (accessed with a GET request). Providing several
arguments will automatically set --wrap-in-directory.

Cluster Add is equivalent to "ipfs add" in terms of DAG building, and supports
the same options for adjusting the chunker, the DAG layout etc. However,
it will allocate the content and send it directly to the allocated peers (among
which may not necessarily be the local ipfs daemon).

Once the adding process is finished, the content is fully added to all
allocations and pinned in them. This makes cluster add slower than a local
ipfs add, but the result is a fully replicated CID on completion.
If you prefer faster adding, add directly to the local IPFS and trigger a
cluster "pin add".

`,
			/*
				Cluster Add supports handling huge files and sharding the resulting DAG among
				several ipfs daemons (--shard). In this case, a single ipfs daemon will not
				contain the full dag, but only parts of it (shards). Desired shard size can
				be provided with the --shard-size flag.

				We recommend setting a --name for sharded pins. Otherwise, it will be
				automatically generated.
			*/
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "recursive, r",
					Usage: "Add directory paths recursively",
				},
				cli.BoolFlag{
					Name:  "quiet, q",
					Usage: "Write only hashes to output (one per line)",
				},
				cli.BoolFlag{
					Name:  "quieter, Q",
					Usage: "Write only final hash to output",
				},
				cli.BoolFlag{
					Name:  "no-stream",
					Usage: "Buffer output locally. Produces a valid JSON array with --enc=json.",
				},
				cli.StringFlag{
					Name:  "layout",
					Value: defaultAddParams.Layout,
					Usage: "Dag layout to use for dag generation: balanced or trickle",
				},
				cli.BoolFlag{
					Name:  "wrap-with-directory, w",
					Usage: "Wrap a with a directory object.",
				},
				cli.BoolFlag{
					Name:  "hidden, H",
					Usage: "Include files that are hidden.  Only takes effect on recursive add",
				},
				cli.StringFlag{
					Name:  "chunker, s",
					Usage: "'size-<size>' or 'rabin-<min>-<avg>-<max>'",
					Value: defaultAddParams.Chunker,
				},
				cli.BoolFlag{
					Name:  "raw-leaves",
					Usage: "Use raw blocks for leaves (experimental)",
				},
				cli.IntFlag{
					Name:  "cid-version",
					Usage: "CID version. Non default implies raw-leaves",
					Value: defaultAddParams.CidVersion,
				},
				cli.StringFlag{
					Name:  "hash",
					Usage: "Hash function to use. Implies cid-version=1.",
					Value: defaultAddParams.HashFun,
				},
				cli.StringFlag{
					Name:  "name, n",
					Value: defaultAddParams.Name,
					Usage: "Sets a name for this pin",
				},
				cli.IntFlag{
					Name:  "replication-min, rmin",
					Value: defaultAddParams.ReplicationFactorMin,
					Usage: "Sets the minimum replication factor for pinning this file",
				},
				cli.IntFlag{
					Name:  "replication-max, rmax",
					Value: defaultAddParams.ReplicationFactorMax,
					Usage: "Sets the maximum replication factor for pinning this file",
				},
				// TODO: Uncomment when sharding is supported.
				// cli.BoolFlag{
				//	Name:  "shard",
				//	Usage: "Break the file into pieces (shards) and distributed among peers",
				// },
				// cli.Uint64Flag{
				//	Name:  "shard-size",
				//	Value: defaultAddParams.ShardSize,
				//	Usage: "Sets the maximum replication factor for pinning this file",
				// },
				// TODO: Figure progress over total bar.
				// cli.BoolFlag{
				//	Name:  "progress, p",
				//	Usage: "Stream progress data",
				// },

			},
			Action: func(c *cli.Context) error {
				shard := c.Bool("shard")
				name := c.String("name")
				if shard && name == "" {
					randName, err := uuid.NewV4()
					if err != nil {
						return err
					}
					// take only first letters
					name = "sharded-" + strings.Split(randName.String(), "-")[0]
				}

				// Read arguments (paths)
				paths := make([]string, c.NArg(), c.NArg())
				for i, path := range c.Args() {
					paths[i] = path
				}

				if len(paths) == 0 {
					checkErr("", errors.New("need at least one path"))
				}

				// Setup AddParams
				p := api.DefaultAddParams()
				p.ReplicationFactorMin = c.Int("replication-min")
				p.ReplicationFactorMax = c.Int("replication-max")
				p.Name = name
				//p.Shard = shard
				//p.ShardSize = c.Uint64("shard-size")
				p.Shard = false
				p.Recursive = c.Bool("recursive")
				p.Layout = c.String("layout")
				p.Chunker = c.String("chunker")
				p.RawLeaves = c.Bool("raw-leaves")
				p.Hidden = c.Bool("hidden")
				p.Wrap = c.Bool("wrap-with-directory") || len(paths) > 1
				p.CidVersion = c.Int("cid-version")
				p.HashFun = c.String("hash")
				if p.HashFun != defaultAddParams.HashFun {
					p.CidVersion = 1
				}
				if p.CidVersion > 0 {
					p.RawLeaves = true
				}

				out := make(chan *api.AddedOutput, 1)
				var wg sync.WaitGroup
				wg.Add(1)
				go func() {
					defer wg.Done()

					var buffered []*addedOutputQuiet
					var lastBuf = make([]*addedOutputQuiet, 1, 1)
					var qq = c.Bool("quieter")
					var q = c.Bool("quiet") || qq
					var bufferResults = c.Bool("no-stream")
					for v := range out {
						added := &addedOutputQuiet{v, q}
						lastBuf[0] = added
						if bufferResults {
							buffered = append(buffered, added)
							continue
						}
						if !qq { // print things
							formatResponse(c, added, nil)
						}
					}
					if lastBuf[0].added == nil {
						return // no elements at all
					}
					if bufferResults { // we buffered.
						if qq { // [last elem]
							formatResponse(c, lastBuf, nil)
							return
						}
						// [all elems]
						formatResponse(c, buffered, nil)
					} else if qq { // we already printed unless Quieter
						formatResponse(c, lastBuf[0], nil)
						return
					}
				}()

				cerr := globalClient.Add(ctx, paths, p, out)
				wg.Wait()
				formatResponse(c, nil, cerr)
				return cerr
			},
		},
		{
			Name:        "pin",
			Usage:       "Pin and unpin and list items in IPFS Cluster",
			Description: "Pin and unpin and list items in IPFS Cluster",
			Subcommands: []cli.Command{
				{
					Name:  "add",
					Usage: "Cluster Pin",
					Description: `
This command tells IPFS Cluster to start managing a CID. Depending on
the pinning strategy, this will trigger IPFS pin requests. The CID will
become part of the Cluster's state and will tracked from this point.

When the request has succeeded, the command returns the status of the CID
in the cluster and should be part of the list offered by "pin ls".

An optional replication factor can be provided: -1 means "pin everywhere"
and 0 means use cluster's default setting. Positive values indicate how many
peers should pin this content.

An optional allocations argument can be provided, allocations should be a
comma-separated list of peer IDs on which we want to pin. Peers in allocations
are prioritized over automatically-determined ones, but replication factors
would stil be respected.
`,
					ArgsUsage: "<CID>",
					Flags: []cli.Flag{
						cli.IntFlag{
							Name:  "replication, r",
							Value: 0,
							Usage: "Sets a custom replication factor (overrides -rmax and -rmin)",
						},
						cli.IntFlag{
							Name:  "replication-min, rmin",
							Value: 0,
							Usage: "Sets the minimum replication factor for this pin",
						},
						cli.IntFlag{
							Name:  "replication-max, rmax",
							Value: 0,
							Usage: "Sets the maximum replication factor for this pin",
						},
						cli.StringSliceFlag{
							Name:  "allocations, allocs",
							Usage: "Optional comma-separated list of peer IDs",
						},
						cli.StringFlag{
							Name:  "name, n",
							Value: "",
							Usage: "Sets a name for this pin",
						},
						cli.BoolFlag{
							Name:  "no-status, ns",
							Usage: "Prevents fetching pin status after pinning (faster, quieter)",
						},
						cli.BoolFlag{
							Name:  "wait, w",
							Usage: "Wait for all nodes to report a status of pinned before returning",
						},
						cli.DurationFlag{
							Name:  "wait-timeout, wt",
							Value: 0,
							Usage: "How long to --wait (in seconds), default is indefinitely",
						},
					},
					Action: func(c *cli.Context) error {
						arg := c.Args().First()
						rpl := c.Int("replication")
						rplMin := c.Int("replication-min")
						rplMax := c.Int("replication-max")
						if rpl != 0 {
							rplMin = rpl
							rplMax = rpl
						}

						opts := api.PinOptions{
							ReplicationFactorMin: rplMin,
							ReplicationFactorMax: rplMax,
							Name:                 c.String("name"),
							UserAllocations:      c.StringSlice("allocations"),
						}

						pin, cerr := globalClient.PinPath(ctx, arg, opts)
						if cerr != nil {
							formatResponse(c, nil, cerr)
							return nil
						}
						handlePinResponseFormatFlags(
							ctx,
							c,
							pin,
							api.TrackerStatusPinned,
						)
						return nil
					},
				},
				{
					Name:  "rm",
					Usage: "Cluster Unpin",
					Description: `
This command tells IPFS Cluster to no longer manage a CID. This will
trigger unpinning operations in all the IPFS nodes holding the content.

When the request has succeeded, the command returns the status of the CID
in the cluster. The CID should disappear from the list offered by "pin ls",
although unpinning operations in the cluster may take longer or fail.
`,
					ArgsUsage: "<CID>",
					Flags: []cli.Flag{
						cli.BoolFlag{
							Name:  "no-status, ns",
							Usage: "Prevents fetching pin status after unpinning (faster, quieter)",
						},
						cli.BoolFlag{
							Name:  "wait, w",
							Usage: "Wait for all nodes to report a status of unpinned before returning",
						},
						cli.DurationFlag{
							Name:  "wait-timeout, wt",
							Value: 0,
							Usage: "How long to --wait (in seconds), default is indefinitely",
						},
					},
					Action: func(c *cli.Context) error {
						arg := c.Args().First()
						pin, cerr := globalClient.UnpinPath(ctx, arg)
						if cerr != nil {
							formatResponse(c, nil, cerr)
							return nil
						}
						handlePinResponseFormatFlags(
							ctx,
							c,
							pin,
							api.TrackerStatusUnpinned,
						)
						return nil
					},
				},
				{
					Name:  "ls",
					Usage: "List items in the cluster pinset",
					Description: `
This command will list the CIDs which are tracked by IPFS Cluster and to
which peers they are currently allocated. This list does not include
any monitoring information about the IPFS status of the CIDs, it
merely represents the list of pins which are part of the shared state of
the cluster. For IPFS-status information about the pins, use "status".

The filter only takes effect when listing all pins. The possible values are:
  - all
  - pin
  - meta-pin
  - clusterdag-pin
  - shard-pin
`,
					ArgsUsage: "[CID]",
					Flags: []cli.Flag{
						cli.StringFlag{
							Name:  "filter",
							Usage: "Comma separated list of pin types. See help above.",
							Value: "pin",
						},
					},
					Action: func(c *cli.Context) error {
						cidStr := c.Args().First()
						if cidStr != "" {
							ci, err := cid.Decode(cidStr)
							checkErr("parsing cid", err)
							resp, cerr := globalClient.Allocation(ctx, ci)
							formatResponse(c, resp, cerr)
						} else {
							var filter api.PinType
							strFilter := strings.Split(c.String("filter"), ",")
							for _, f := range strFilter {
								filter |= api.PinTypeFromString(f)
							}

							resp, cerr := globalClient.Allocations(ctx, filter)
							formatResponse(c, resp, cerr)
						}
						return nil
					},
				},
			},
		},
		{
			Name:  "status",
			Usage: "Retrieve the status of tracked items",
			Description: `
This command retrieves the status of the CIDs tracked by IPFS
Cluster, including which member is pinning them and any errors.
If a CID is provided, the status will be only fetched for a single
item.  Metadata CIDs are included in the status response

The status of a CID may not be accurate. A manual sync can be triggered
with "sync".

When the --local flag is passed, it will only fetch the status from the
contacted cluster peer. By default, status will be fetched from all peers.

When the --filter flag is passed, it will only fetch the peer information
where status of the pin matches at least one of the filter values (a comma
separated list). The following are valid status values:

` + trackerStatusAllString(),
			ArgsUsage: "[CID]",
			Flags: []cli.Flag{
				localFlag(),
				cli.StringFlag{
					Name:  "filter",
					Usage: "comma-separated list of filters",
				},
			},
			Action: func(c *cli.Context) error {
				cidStr := c.Args().First()
				if cidStr != "" {
					ci, err := cid.Decode(cidStr)
					checkErr("parsing cid", err)
					resp, cerr := globalClient.Status(ctx, ci, c.Bool("local"))
					formatResponse(c, resp, cerr)
				} else {
					filterFlag := c.String("filter")
					filter := api.TrackerStatusFromString(c.String("filter"))
					if filter == api.TrackerStatusUndefined && filterFlag != "" {
						checkErr("parsing filter flag", errors.New("invalid filter name"))
					}
					resp, cerr := globalClient.StatusAll(ctx, filter, c.Bool("local"))
					formatResponse(c, resp, cerr)
				}
				return nil
			},
		},
		{
			Name:  "sync",
			Usage: "Sync status of tracked items",
			Description: `
This command asks Cluster peers to verify that the current status of tracked
CIDs is accurate by triggering queries to the IPFS daemons that pin them.
If a CID is provided, the sync and recover operations will be limited to
that single item.

Unless providing a specific CID, the command will output only items which
have changed status because of the sync or are in error state in some node,
therefore, the output should be empty if no operations were performed.

CIDs in error state may be manually recovered with "recover".

When the --local flag is passed, it will only trigger sync
operations on the contacted peer. By default, all peers will sync.
`,
			ArgsUsage: "[CID]",
			Flags: []cli.Flag{
				localFlag(),
			},
			Action: func(c *cli.Context) error {
				cidStr := c.Args().First()
				if cidStr != "" {
					ci, err := cid.Decode(cidStr)
					checkErr("parsing cid", err)
					resp, cerr := globalClient.Sync(ctx, ci, c.Bool("local"))
					formatResponse(c, resp, cerr)
				} else {
					resp, cerr := globalClient.SyncAll(ctx, c.Bool("local"))
					formatResponse(c, resp, cerr)
				}
				return nil
			},
		},
		{
			Name:  "recover",
			Usage: "Recover tracked items in error state",
			Description: `
This command asks Cluster peers to re-track or re-forget CIDs in
error state, usually because the IPFS pin or unpin operation has failed.

The command will wait for any operations to succeed and will return the status
of the item upon completion. Note that, when running on the full sets of tracked
CIDs (without argument), it may take a considerably long time.

When the --local flag is passed, it will only trigger recover
operations on the contacted peer (as opposed to on every peer).
`,
			ArgsUsage: "[CID]",
			Flags: []cli.Flag{
				localFlag(),
			},
			Action: func(c *cli.Context) error {
				cidStr := c.Args().First()
				if cidStr != "" {
					ci, err := cid.Decode(cidStr)
					checkErr("parsing cid", err)
					resp, cerr := globalClient.Recover(ctx, ci, c.Bool("local"))
					formatResponse(c, resp, cerr)
				} else {
					resp, cerr := globalClient.RecoverAll(ctx, c.Bool("local"))
					formatResponse(c, resp, cerr)
				}
				return nil
			},
		},

		{
			Name:  "version",
			Usage: "Retrieve cluster version",
			Description: `
This command retrieves the IPFS Cluster version and can be used
to check that it matches the CLI version (shown by -v).
`,
			ArgsUsage: " ",
			Flags:     []cli.Flag{},
			Action: func(c *cli.Context) error {
				resp, cerr := globalClient.Version(ctx)
				formatResponse(c, resp, cerr)
				return nil
			},
		},
		{
			Name:        "health",
			Usage:       "Cluster monitoring information",
			Description: "Cluster monitoring information",
			Subcommands: []cli.Command{
				{
					Name:  "graph",
					Usage: "create a graph displaying connectivity of cluster peers",
					Description: `
This command queries all connected cluster peers and their ipfs peers to generate a
graph of the connections.  Output is a dot file encoding the cluster's connection state.
`,
					Flags: []cli.Flag{
						cli.StringFlag{
							Name:  "file, f",
							Value: "",
							Usage: "sets an output dot-file for the connectivity graph",
						},
						cli.BoolFlag{
							Name:  "all-ipfs-peers",
							Usage: "causes the graph to mark nodes for ipfs peers not directly in the cluster",
						},
					},
					Action: func(c *cli.Context) error {
						resp, cerr := globalClient.GetConnectGraph(ctx)
						if cerr != nil {
							formatResponse(c, resp, cerr)
							return nil
						}
						var w io.WriteCloser
						var err error
						outputPath := c.String("file")
						if outputPath == "" {
							w = os.Stdout
						} else {
							w, err = os.Create(outputPath)
							checkErr("creating output file", err)
						}
						defer w.Close()
						err = makeDot(resp, w, c.Bool("all-ipfs-peers"))
						checkErr("printing graph", err)

						return nil
					},
				},
				{
					Name:  "metrics",
					Usage: "List latest metrics logged by this peer",
					Description: `
This commands displays the latest valid metrics of the given type logged
by this peer for all current cluster peers.

Currently supported metrics depend on the informer component used,
but usually are:

- freespace
- ping
`,
					ArgsUsage: "<metric name>",
					Action: func(c *cli.Context) error {
						metric := c.Args().First()
						if metric == "" {
							checkErr("", errors.New("provide a metric name"))
						}

						resp, cerr := globalClient.Metrics(ctx, metric)
						formatResponse(c, resp, cerr)
						return nil
					},
				},
			},
		},
		{
			Name:      "commands",
			Usage:     "List all commands",
			ArgsUsage: " ",
			Hidden:    true,
			Action: func(c *cli.Context) error {
				walkCommands(c.App.Commands, "ipfs-cluster-ctl")
				return nil
			},
		},
	}

	app.Run(os.Args)
}

func parseFlag(t int) cli.IntFlag {
	return cli.IntFlag{
		Name:   "parseAs",
		Value:  t,
		Hidden: true,
	}
}

func localFlag() cli.BoolFlag {
	return cli.BoolFlag{
		Name:  "local",
		Usage: "run operation only on the contacted peer",
	}
}

func walkCommands(cmds []cli.Command, parentHelpName string) {
	for _, c := range cmds {
		h := c.HelpName
		// Sometimes HelpName is empty
		if h == "" {
			h = fmt.Sprintf("%s %s", parentHelpName, c.FullName())
		}
		fmt.Println(h)
		walkCommands(c.Subcommands, h)
	}
}

func formatResponse(c *cli.Context, resp interface{}, err error) {
	enc := c.GlobalString("encoding")
	if resp == nil && err == nil {
		return
	}

	if err != nil {
		cerr, ok := err.(*api.Error)
		if !ok {
			checkErr("", err)
		}
		switch enc {
		case "text":
			textFormatPrintError(cerr)
		case "json":
			jsonFormatPrint(cerr)
		default:
			checkErr("", errors.New("unsupported encoding selected"))
		}
		if cerr.Code == 0 {
			os.Exit(1) // problem with the call
		} else {
			os.Exit(2) // call went fine, response has an error
		}
	}

	switch enc {
	case "text":
		textFormatObject(resp)
	case "json":
		jsonFormatObject(resp)
	default:
		checkErr("", errors.New("unsupported encoding selected"))
	}
}

func parseCredentials(userInput string) (string, string) {
	credentials := strings.SplitN(userInput, ":", 2)
	switch len(credentials) {
	case 1:
		// only username passed in (with no trailing `:`), return empty password
		return credentials[0], ""
	case 2:
		return credentials[0], credentials[1]
	default:
		err := fmt.Errorf("invalid <username>[:<password>] input")
		checkErr("parsing credentials", err)
		return "", ""
	}
}

func handlePinResponseFormatFlags(
	ctx context.Context,
	c *cli.Context,
	pin *api.Pin,
	target api.TrackerStatus,
) {

	var status *api.GlobalPinInfo
	var cerr error

	if c.Bool("wait") {
		status, cerr = waitFor(pin.Cid, target, c.Duration("wait-timeout"))
		checkErr("waiting for pin status", cerr)
	}

	if c.Bool("no-status") {
		formatResponse(c, pin, nil)
		return
	}

	if status.Cid == cid.Undef { // no status from "wait"
		time.Sleep(time.Second)
		status, cerr = globalClient.Status(ctx, pin.Cid, false)
	}
	formatResponse(c, status, cerr)
}

func waitFor(
	ci cid.Cid,
	target api.TrackerStatus,
	timeout time.Duration,
) (*api.GlobalPinInfo, error) {

	ctx := context.Background()

	if timeout > defaultWaitCheckFreq {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	fp := client.StatusFilterParams{
		Cid:       ci,
		Local:     false,
		Target:    target,
		CheckFreq: defaultWaitCheckFreq,
	}

	return client.WaitFor(ctx, globalClient, fp)
}

// func setupTracing(config tracingConfig) {
// 	if !config.Enable {
// 		return
// 	}

// 	agentEndpointURI := "0.0.0.0:6831"
// 	collectorEndpointURI := "http://0.0.0.0:14268"

// 	if config.JaegerAgentEndpoint != "" {
// 		agentEndpointURI = config.JaegerAgentEndpoint
// 	}
// 	if config.JaegerCollectorEndpoint != "" {
// 		collectorEndpointURI = config.JaegerCollectorEndpoint
// 	}

// 	je, err := jaeger.NewExporter(jaeger.Options{
// 		AgentEndpoint:     agentEndpointURI,
// 		CollectorEndpoint: collectorEndpointURI,
// 		ServiceName:       "ipfs-cluster-ctl",
// 	})
// 	if err != nil {
// 		log.Fatalf("Failed to create the Jaeger exporter: %v", err)
// 	}
// 	// Register/enable the trace exporter
// 	trace.RegisterExporter(je)

// 	// For demo purposes, set the trace sampling probability to be high
// 	trace.ApplyConfig(trace.Config{DefaultSampler: trace.ProbabilitySampler(1.0)})
// }
