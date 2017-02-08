package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	cid "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	peer "github.com/libp2p/go-libp2p-peer"
	ma "github.com/multiformats/go-multiaddr"
	cli "github.com/urfave/cli"
)

const programName = `ipfs-cluster-ctl`

// Version is the cluster-ctl tool version. It should match
// the IPFS cluster's version
const Version = "0.0.2"

var (
	defaultHost     = fmt.Sprintf("127.0.0.1:%d", 9094)
	defaultTimeout  = 60
	defaultProtocol = "http"
)

var logger = logging.Logger("cluster-ctl")

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

type errorResp struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

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
	app.UsageText = Description
	app.Version = Version
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "host, l",
			Value: defaultHost,
			Usage: "host:port of the IPFS Cluster service API",
		},
		cli.BoolFlag{
			Name:  "https, s",
			Usage: "use https to connect to the API",
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
	}

	app.Before = func(c *cli.Context) error {
		defaultHost = c.String("host")
		defaultTimeout = c.Int("timeout")
		if c.Bool("https") {
			defaultProtocol = "https"
		}
		if c.Bool("debug") {
			logging.SetLogLevel("cluster-ctl", "debug")
		}
		return nil
	}

	app.Commands = []cli.Command{
		{
			Name:  "id",
			Usage: "retrieve cluster member information",
			UsageText: `
This command will print out information about the cluster peer used
`,
			Flags: []cli.Flag{parseFlag(formatID)},
			Action: func(c *cli.Context) error {
				resp := request("GET", "/id", nil)
				formatResponse(c, resp)
				return nil
			},
		},
		{
			Name:  "peers",
			Usage: "list and manage IPFS Cluster peers",
			UsageText: `
This command can be used to list and manage IPFS Cluster peers.
`,
			Subcommands: []cli.Command{
				{
					Name:  "ls",
					Usage: "list the nodes participating in the IPFS Cluster",
					UsageText: `
This commands provides a list of the ID information of all the peers in the Cluster.
`,
					Flags: []cli.Flag{parseFlag(formatID)},
					Action: func(c *cli.Context) error {
						resp := request("GET", "/peers", nil)
						formatResponse(c, resp)
						return nil
					},
				},
				{
					Name:  "add",
					Usage: "add a peer to the Cluster",
					UsageText: `
This command adds a new peer to the cluster. In order for the operation to
succeed, the new peer needs to be reachable and any other member of the cluster
should be online. The operation returns the ID information for the new peer.
`,
					ArgsUsage: "<multiaddress>",
					Flags:     []cli.Flag{parseFlag(formatID)},
					Action: func(c *cli.Context) error {
						addr := c.Args().First()
						if addr == "" {
							return cli.NewExitError("Error: a multiaddress argument is needed", 1)
						}
						_, err := ma.NewMultiaddr(addr)
						checkErr("parsing multiaddress", err)
						addBody := peerAddBody{addr}
						var buf bytes.Buffer
						enc := json.NewEncoder(&buf)
						enc.Encode(addBody)
						resp := request("POST", "/peers", &buf)
						formatResponse(c, resp)
						return nil
					},
				},
				{
					Name:  "rm",
					Usage: "remove a peer from the Cluster",
					UsageText: `
This command removes a peer from the cluster. If the peer is online, it will
automatically shut down. All other cluster peers should be online for the
operation to succeed, otherwise some nodes may be left with an outdated list of
cluster peers.
`,
					ArgsUsage: "<peer ID>",
					Flags:     []cli.Flag{parseFlag(formatNone)},
					Action: func(c *cli.Context) error {
						pid := c.Args().First()
						_, err := peer.IDB58Decode(pid)
						checkErr("parsing peer ID", err)
						resp := request("DELETE", "/peers/"+pid, nil)
						formatResponse(c, resp)
						return nil
					},
				},
			},
		},
		{
			Name:  "pin",
			Usage: "add, remove or list items managed by IPFS Cluster",
			UsageText: `
This command allows to add, remove or list items managed (pinned) by
the Cluster.
`,
			Subcommands: []cli.Command{
				{
					Name:  "add",
					Usage: "Track a CID (pin)",
					UsageText: `
This command tells IPFS Cluster to start managing a CID. Depending on
the pinning strategy, this will trigger IPFS pin requests. The CID will
become part of the Cluster's state and will tracked from this point.

When the request has succeeded, the command returns the status of the CID
in the cluster and should be part of the list offered by "pin ls".
`,
					ArgsUsage: "<cid>",
					Flags:     []cli.Flag{parseFlag(formatGPInfo)},
					Action: func(c *cli.Context) error {
						cidStr := c.Args().First()
						_, err := cid.Decode(cidStr)
						checkErr("parsing cid", err)
						request("POST", "/pins/"+cidStr, nil)
						time.Sleep(500 * time.Millisecond)
						resp := request("GET", "/pins/"+cidStr, nil)
						formatResponse(c, resp)
						return nil
					},
				},
				{
					Name:  "rm",
					Usage: "Stop tracking a CID (unpin)",
					UsageText: `
This command tells IPFS Cluster to no longer manage a CID. This will
trigger unpinning operations in all the IPFS nodes holding the content.

When the request has succeeded, the command returns the status of the CID
in the cluster. The CID should disappear from the list offered by "pin ls",
although unpinning operations in the cluster may take longer or fail.
`,
					ArgsUsage: "<cid>",
					Flags:     []cli.Flag{parseFlag(formatGPInfo)},
					Action: func(c *cli.Context) error {
						cidStr := c.Args().First()
						_, err := cid.Decode(cidStr)
						checkErr("parsing cid", err)
						request("DELETE", "/pins/"+cidStr, nil)
						time.Sleep(500 * time.Millisecond)
						resp := request("GET", "/pins/"+cidStr, nil)
						formatResponse(c, resp)
						return nil
					},
				},
				{
					Name:  "ls",
					Usage: "List tracked CIDs",
					UsageText: `
This command will list the CIDs which are tracked by IPFS Cluster. This
list does not include information about tracking status or location, it
merely represents the list of pins which are part of the global state of
the cluster. For specific information, use "status".
`,
					Flags: []cli.Flag{parseFlag(formatString)},
					Action: func(c *cli.Context) error {
						resp := request("GET", "/pinlist", nil)
						formatResponse(c, resp)
						return nil
					},
				},
			},
		},
		{
			Name:  "status",
			Usage: "Retrieve the status of tracked items",
			UsageText: `
This command retrieves the status of the CIDs tracked by IPFS
Cluster, including which member is pinning them and any errors.
If a CID is provided, the status will be only fetched for a single
item.

The status of a CID may not be accurate. A manual sync can be triggered
with "sync".
`,
			ArgsUsage: "[cid]",
			Flags:     []cli.Flag{parseFlag(formatGPInfo)},
			Action: func(c *cli.Context) error {
				cidStr := c.Args().First()
				if cidStr != "" {
					_, err := cid.Decode(cidStr)
					checkErr("parsing cid", err)
				}
				resp := request("GET", "/pins/"+cidStr, nil)
				formatResponse(c, resp)
				return nil
			},
		},
		{
			Name:  "sync",
			Usage: "Sync status of tracked items",
			UsageText: `
This command asks Cluster peers to verify that the current status of tracked
CIDs is accurate by triggering queries to the IPFS daemons that pin them.
If a CID is provided, the sync and recover operations will be limited to 
that single item.

Unless providing a specific CID, the command will output only items which
have changed status because of the sync or are in error state in some node,
therefore, the output should be empty if no operations were performed.

CIDs in error state may be manually recovered with "recover".
`,
			ArgsUsage: "[cid]",
			Flags:     []cli.Flag{parseFlag(formatGPInfo)},
			Action: func(c *cli.Context) error {
				cidStr := c.Args().First()
				var resp *http.Response
				if cidStr != "" {
					_, err := cid.Decode(cidStr)
					checkErr("parsing cid", err)
					resp = request("POST", "/pins/"+cidStr+"/sync", nil)
				} else {
					resp = request("POST", "/pins/sync", nil)
				}
				formatResponse(c, resp)
				return nil
			},
		},
		{
			Name:  "recover",
			Usage: "Recover tracked items in error state",
			UsageText: `
This command asks Cluster peers to re-track or re-forget an item which is in
error state, usually because the IPFS pin or unpin operation has failed.

The command will wait for any operations to succeed and will return the status
of the item upon completion.
`,
			ArgsUsage: "<cid>",
			Flags:     []cli.Flag{parseFlag(formatGPInfo)},
			Action: func(c *cli.Context) error {
				cidStr := c.Args().First()
				var resp *http.Response
				if cidStr != "" {
					_, err := cid.Decode(cidStr)
					checkErr("parsing cid", err)
					resp = request("POST", "/pins/"+cidStr+"/recover", nil)
					formatResponse(c, resp)

				} else {
					return cli.NewExitError("A CID is required", 1)
				}
				return nil
			},
		},
		{
			Name:  "version",
			Usage: "Retrieve cluster version",
			UsageText: `
This command retrieves the IPFS Cluster version and can be used
to check that it matches the CLI version (shown by -v).
`,
			Flags: []cli.Flag{parseFlag(formatVersion)},
			Action: func(c *cli.Context) error {
				resp := request("GET", "/version", nil)
				formatResponse(c, resp)
				return nil
			},
		},
		{
			Name:   "commands",
			Usage:  "List all commands",
			Hidden: true,
			Action: func(c *cli.Context) error {
				walkCommands(c.App.Commands)
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

func walkCommands(cmds []cli.Command) {
	for _, c := range cmds {
		fmt.Println(c.HelpName)
		walkCommands(c.Subcommands)
	}
}

func request(method, path string, body io.Reader, args ...string) *http.Response {
	ctx, cancel := context.WithTimeout(context.Background(),
		time.Duration(defaultTimeout)*time.Second)
	defer cancel()

	u := defaultProtocol + "://" + defaultHost + path
	// turn /a/{param0}/{param1} into /a/this/that
	for i, a := range args {
		p := fmt.Sprintf("{param%d}", i)
		u = strings.Replace(u, p, a, 1)
	}
	u = strings.TrimSuffix(u, "/")

	logger.Debugf("%s: %s", method, u)

	r, err := http.NewRequest(method, u, body)
	checkErr("creating request", err)
	r.WithContext(ctx)

	client := &http.Client{}
	resp, err := client.Do(r)
	checkErr(fmt.Sprintf("performing request to %s", defaultHost), err)

	return resp
}

func formatResponse(c *cli.Context, r *http.Response) {
	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	checkErr("reading body", err)
	logger.Debugf("Body: %s", body)

	switch {
	case r.StatusCode > 399:
		var e errorResp
		err = json.Unmarshal(body, &e)
		checkErr("decoding error response", err)
		out("Error %d: %s", e.Code, e.Message)
	case r.StatusCode == http.StatusAccepted:
		out("%s", "Request accepted")
	case r.StatusCode == http.StatusNoContent:
		out("%s", "Request succeeded\n")
	default:
		enc := c.GlobalString("encoding")

		switch enc {
		case "text":
			textFormat(body, c.Int("parseAs"))
		default:
			var resp interface{}
			err = json.Unmarshal(body, &resp)
			checkErr("decoding response", err)
			prettyPrint(body)
		}
	}
}

// JSON output is nice and allows users to build on top.
func prettyPrint(buf []byte) {
	var dst bytes.Buffer
	err := json.Indent(&dst, buf, "", "  ")
	checkErr("indenting json", err)
	fmt.Printf("%s", dst.String())
}

/*
// old ugly pretty print
func prettyPrint(obj interface{}, indent int) {
	ind := func() string {
		var str string
		for i := 0; i < indent; i++ {
			str += " "
		}
		return str
	}

	switch obj.(type) {
	case []interface{}:
		slice := obj.([]interface{})
		for _, elem := range slice {
			prettyPrint(elem, indent+2)
		}
	case map[string]interface{}:
		m := obj.(map[string]interface{})
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := m[k]
			fmt.Printf(ind()+"%s: ", k)
			switch v.(type) {
			case []interface{}, map[string]interface{}:
				fmt.Println()
				prettyPrint(v, indent+4)
			default:
				prettyPrint(v, indent)
			}
		}
	default:
		fmt.Println(obj)
	}
}
*/
