package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	cid "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	peer "github.com/libp2p/go-libp2p-peer"
	ma "github.com/multiformats/go-multiaddr"
	cli "github.com/urfave/cli"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/api/rest/client"
)

const programName = `ipfs-cluster-ctl`

// Version is the cluster-ctl tool version. It should match
// the IPFS cluster's version
const Version = "0.3.0"

var (
	defaultHost     = "/ip4/127.0.0.1/tcp/9094"
	defaultTimeout  = 60
	defaultUsername = ""
	defaultPassword = ""
)

var logger = logging.Logger("cluster-ctl")

var globalClient *client.Client

// Description provides a short summary of the functionality of this tool
var Description = fmt.Sprintf(`
%s is a tool to manage IPFS Cluster nodes.
Use "%s help" to list all available commands and
"%s help <command>" to get usage information for a
specific one.

%s uses the IPFS Cluster API to perform requests and display
responses in a user-readable format. The location of the IPFS
Cluster server is assumed to be %s, but can be
configured with the --host option.

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
	app := cli.NewApp()
	app.Name = programName
	app.Usage = "CLI for IPFS Cluster"
	app.Description = Description
	app.Version = Version
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "host, l",
			Value: defaultHost,
			Usage: "multiaddress of the IPFS Cluster service API",
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
		addr, err := ma.NewMultiaddr(c.String("host"))
		checkErr("parsing host multiaddress", err)
		cfg.APIAddr = addr

		cfg.Timeout = time.Duration(c.Int("timeout")) * time.Second
		cfg.SSL = c.Bool("https")
		cfg.NoVerifyCert = c.Bool("no-check-certificate")
		user, pass := parseCredentials(c.String("basic-auth"))
		cfg.Username = user
		cfg.Password = pass
		if user != "" && !cfg.SSL && !c.Bool("force-http") {
			logger.Warning("SSL automatically enabled with basic auth credentials. Set \"force-http\" to disable")
			cfg.SSL = true
		}

		if c.Bool("debug") {
			logging.SetLogLevel("cluster-ctl", "debug")
			logging.SetLogLevel("apiclient", "debug")
		}

		enc := c.String("encoding")
		if enc != "text" && enc != "json" {
			checkErr("", errors.New("unsupported encoding"))
		}

		globalClient, err = client.NewClient(cfg)
		checkErr("creating API client", err)
		return nil
	}

	app.Commands = []cli.Command{
		{
			Name:  "id",
			Usage: "retrieve peer information",
			Description: `
This command displays information about the peer that the tool is contacting
(usually running in localhost).
`,
			Flags: []cli.Flag{},
			Action: func(c *cli.Context) error {
				resp, cerr := globalClient.ID()
				formatResponse(c, resp, cerr)
				return nil
			},
		},
		{
			Name:        "peers",
			Description: "list and manage IPFS Cluster peers",
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
						resp, cerr := globalClient.Peers()
						formatResponse(c, resp, cerr)
						return nil
					},
				},
				{
					Name:  "add",
					Usage: "add a peer to the Cluster",
					Description: `
This command adds a new peer to the cluster. In order for the operation to
succeed, the new peer needs to be reachable and any other member of the cluster
should be online. The operation returns the ID information for the new peer.
`,
					ArgsUsage: "<multiaddress>",
					Flags:     []cli.Flag{},
					Action: func(c *cli.Context) error {
						addr := c.Args().First()
						if addr == "" {
							return cli.NewExitError("Error: a multiaddress argument is needed", 1)
						}
						maddr, err := ma.NewMultiaddr(addr)
						checkErr("parsing multiaddress", err)
						resp, cerr := globalClient.PeerAdd(maddr)
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
						cerr := globalClient.PeerRm(p)
						formatResponse(c, nil, cerr)
						return nil
					},
				},
			},
		},
		{
			Name:        "pin",
			Description: "add, remove or list items managed by IPFS Cluster",
			Subcommands: []cli.Command{
				{
					Name:  "add",
					Usage: "Track a CID (pin)",
					Description: `
This command tells IPFS Cluster to start managing a CID. Depending on
the pinning strategy, this will trigger IPFS pin requests. The CID will
become part of the Cluster's state and will tracked from this point.

When the request has succeeded, the command returns the status of the CID
in the cluster and should be part of the list offered by "pin ls".

An optional replication factor can be provided: -1 means "pin everywhere"
and 0 means use cluster's default setting. Positive values indicate how many
peers should pin this content.
`,
					ArgsUsage: "<CID>",
					Flags: []cli.Flag{
						cli.IntFlag{
							Name:  "replication, r",
							Value: 0,
							Usage: "Sets a custom replication factor for this pin",
						},
						cli.StringFlag{
							Name:  "name, n",
							Value: "",
							Usage: "Sets a name for this pin",
						},
					},
					Action: func(c *cli.Context) error {
						cidStr := c.Args().First()
						ci, err := cid.Decode(cidStr)
						checkErr("parsing cid", err)
						cerr := globalClient.Pin(ci, c.Int("replication"), c.String("name"))
						if cerr != nil {
							formatResponse(c, nil, cerr)
							return nil
						}
						time.Sleep(1000 * time.Millisecond)
						resp, cerr := globalClient.Status(ci, false)
						formatResponse(c, resp, cerr)
						return nil
					},
				},
				{
					Name:  "rm",
					Usage: "Stop tracking a CID (unpin)",
					Description: `
This command tells IPFS Cluster to no longer manage a CID. This will
trigger unpinning operations in all the IPFS nodes holding the content.

When the request has succeeded, the command returns the status of the CID
in the cluster. The CID should disappear from the list offered by "pin ls",
although unpinning operations in the cluster may take longer or fail.
`,
					ArgsUsage: "<CID>",
					Flags:     []cli.Flag{},
					Action: func(c *cli.Context) error {
						cidStr := c.Args().First()
						ci, err := cid.Decode(cidStr)
						checkErr("parsing cid", err)
						cerr := globalClient.Unpin(ci)
						if cerr != nil {
							formatResponse(c, nil, cerr)
							return nil
						}
						time.Sleep(1000 * time.Millisecond)
						resp, cerr := globalClient.Status(ci, false)
						formatResponse(c, resp, cerr)
						return nil
					},
				},
				{
					Name:  "ls",
					Usage: "List tracked CIDs",
					Description: `
This command will list the CIDs which are tracked by IPFS Cluster and to
which peers they are currently allocated. This list does not include
any monitoring information about the IPFS status of the CIDs, it
merely represents the list of pins which are part of the shared state of
the cluster. For IPFS-status information about the pins, use "status".
`,
					ArgsUsage: "[CID]",
					Flags:     []cli.Flag{},
					Action: func(c *cli.Context) error {
						cidStr := c.Args().First()
						if cidStr != "" {
							ci, err := cid.Decode(cidStr)
							checkErr("parsing cid", err)
							resp, cerr := globalClient.Allocation(ci)
							formatResponse(c, resp, cerr)
						} else {
							resp, cerr := globalClient.Allocations()
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
item.

The status of a CID may not be accurate. A manual sync can be triggered
with "sync".

When the --local flag is passed, it will only fetch the status from the
contacted cluster peer. By default, status will be fetched from all peers.
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
					resp, cerr := globalClient.Status(ci, c.Bool("local"))
					formatResponse(c, resp, cerr)
				} else {
					resp, cerr := globalClient.StatusAll(c.Bool("local"))
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
					resp, cerr := globalClient.Sync(ci, c.Bool("local"))
					formatResponse(c, resp, cerr)
				} else {
					resp, cerr := globalClient.SyncAll(c.Bool("local"))
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
					resp, cerr := globalClient.Recover(ci, c.Bool("local"))
					formatResponse(c, resp, cerr)
				} else {
					resp, cerr := globalClient.RecoverAll(c.Bool("local"))
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
				resp, cerr := globalClient.Version()
				formatResponse(c, resp, cerr)
				return nil
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

func formatResponse(c *cli.Context, resp interface{}, err *api.Error) {
	enc := c.GlobalString("encoding")
	if resp == nil && err == nil {
		return
	}

	if err != nil {
		switch enc {
		case "text":
			textFormatPrintError(err)
		case "json":
			jsonFormatPrint(err)
		default:
			checkErr("", errors.New("unsupported encoding selected"))
		}
		if err.Code == 0 {
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
