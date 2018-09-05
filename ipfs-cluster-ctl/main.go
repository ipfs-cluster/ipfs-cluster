// The ipfs-cluster-ctl application.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/api/rest/client"

	cid "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	cli "github.com/urfave/cli"
)

const programName = `ipfs-cluster-ctl`

// Version is the cluster-ctl tool version. It should match
// the IPFS cluster's version
const Version = "0.5.0"

var (
	defaultHost          = "/ip4/127.0.0.1/tcp/9094"
	defaultTimeout       = 0
	defaultUsername      = ""
	defaultPassword      = ""
	defaultWaitCheckFreq = time.Second
	defaultAddParams     = api.DefaultAddParams()
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
	prepareApp().Run(os.Args)
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
	c *cli.Context,
	ci *cid.Cid,
	target api.TrackerStatus,
) {

	var status api.GlobalPinInfo
	var cerr error

	if c.Bool("wait") {
		status, cerr = waitFor(ci, target, c.Duration("wait-timeout"))
		checkErr("waiting for pin status", cerr)
	}

	if c.Bool("no-status") {
		return
	}

	if status.Cid == nil { // no status from "wait"
		time.Sleep(time.Second)
		status, cerr = globalClient.Status(ci, false)
	}
	formatResponse(c, status, cerr)
}

func waitFor(
	ci *cid.Cid,
	target api.TrackerStatus,
	timeout time.Duration,
) (api.GlobalPinInfo, error) {

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

	return globalClient.WaitFor(ctx, fp)
}
