package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	cid "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	cli "github.com/urfave/cli"
)

const programName = `ipfs-cluster-ctl`

// Version
const Version = "0.0.1"

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
			Action: func(c *cli.Context) {
				resp := request("GET", "/id")
				formatResponse(resp)
			},
		},
		{
			Name:  "member",
			Usage: "list and manage IPFS Cluster members",
			UsageText: `
This command can be used to list and manage IPFS Cluster members.
`,
			Subcommands: []cli.Command{
				{
					Name:  "ls",
					Usage: "list the nodes participating in the IPFS Cluster",
					Action: func(c *cli.Context) {
						resp := request("GET", "/members")
						formatResponse(resp)
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
					Action: func(c *cli.Context) {
						cidStr := c.Args().First()
						_, err := cid.Decode(cidStr)
						checkErr("parsing cid", err)
						request("POST", "/pins/"+cidStr)
						time.Sleep(500 * time.Millisecond)
						resp := request("GET", "/status/"+cidStr)
						formatResponse(resp)
					},
				},
				{
					Name:  "rm",
					Usage: "Stop tracking a CID (unpin)",
					UsageText: `
This command tells IPFS Cluster to no longer manage a CID. This will
trigger unpinning operations in all the IPFS nodes holding the content.

When the request has succeeded, the command returns the status of the CID
in the cluster. The CID should dissapear from the list offered by "pin ls",
although unpinning operations in the cluster may take longer or fail.
`,
					ArgsUsage: "<cid>",
					Action: func(c *cli.Context) {
						cidStr := c.Args().First()
						_, err := cid.Decode(cidStr)
						checkErr("parsing cid", err)
						request("DELETE", "/pins/"+cidStr)
						time.Sleep(500 * time.Millisecond)
						resp := request("GET", "/status/"+cidStr)
						formatResponse(resp)
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
					Action: func(c *cli.Context) {
						resp := request("GET", "/pins")
						formatResponse(resp)
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
`,
			ArgsUsage: "[cid]",
			Action: func(c *cli.Context) {
				cidStr := c.Args().First()
				if cidStr != "" {
					_, err := cid.Decode(cidStr)
					checkErr("parsing cid", err)
				}
				resp := request("GET", "/status/"+cidStr)
				formatResponse(resp)
			},
		},
		{
			Name:  "sync",
			Usage: "Sync status and/or recover tracked items",
			UsageText: `
This command verifies that the current status tracked CIDs are accurate by
triggering queries to the IPFS daemons that pin them. When the CID is in
error state, either because pinning or unpinning failed, IPFS Cluster will
attempt to retry the operation. If a CID is provided, the sync and recover
operations will be limited to that single item.
`,
			ArgsUsage: "[cid]",
			Action: func(c *cli.Context) {
				cidStr := c.Args().First()
				if cidStr != "" {
					_, err := cid.Decode(cidStr)
					checkErr("parsing cid", err)
				}
				resp := request("POST", "/status/"+cidStr)
				formatResponse(resp)
			},
		},
		{
			Name:  "version",
			Usage: "Retrieve cluster version",
			UsageText: `
This command retrieves the IPFS Cluster version and can be used
to check that it matches the CLI version (shown by -v).
`,
			Action: func(c *cli.Context) {
				resp := request("GET", "/version")
				formatResponse(resp)
			},
		},
	}

	app.Run(os.Args)
}

func request(method, path string, args ...string) *http.Response {
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

	r, err := http.NewRequest(method, u, nil)
	checkErr("creating request", err)
	r.WithContext(ctx)

	client := &http.Client{}
	resp, err := client.Do(r)
	checkErr(fmt.Sprintf("performing request to %s", defaultHost), err)

	return resp
}

func formatResponse(r *http.Response) {
	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	checkErr("reading body", err)
	logger.Debugf("Body: %s", body)

	if r.StatusCode > 399 {
		var e errorResp
		err = json.Unmarshal(body, &e)
		checkErr("decoding error response", err)
		out("Error %d: %s", e.Code, e.Message)
	} else if r.StatusCode == http.StatusAccepted {
		out("%s", "request accepted")
	} else {
		var resp interface{}
		err = json.Unmarshal(body, &resp)
		checkErr("decoding response", err)
		prettyPrint(resp, 0)
	}
}

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
