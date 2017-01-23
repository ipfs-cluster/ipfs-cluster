package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	cid "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
)

const programName = `ipfs-cluster-ctl`

var (
	defaultHost     = fmt.Sprintf("127.0.0.1:%d", 9094)
	defaultTimeout  = 60
	defaultProtocol = "http"
)

var logger = logging.Logger("ipfscluster")

// Description provides a short summary of the functionality of this tool
var Description = fmt.Sprintf(`
%s is a tool to manage IPFS Cluster server nodes.
Use "%s help" to list all available commands and "%s help <command>"
to get usage information for a specific one.

%s uses the IPFS Cluster API to perform requests and display
responses in a user-readable format. The location of the IPFS
Cluster server is assumed to be %s, but can be
configured with the -host option.

For feedback, bug reports or any additional information, visit
https://github.com/ipfs/ipfs-cluster.

`,
	programName,
	programName,
	programName,
	programName,
	defaultHost)

// Command line flags
var (
	hostFlag     string
	protocolFlag string
	timeoutFlag  int
	versionFlag  bool
	debugFlag    bool
)

type errorResp struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type cmd struct {
	Name      string
	Path      string
	Method    string
	Subcmd    bool
	ShortDesc string
	LongDesc  string
}

var cmds = map[string]cmd{
	"help": cmd{
		Name:      "help",
		ShortDesc: "Shows this help",
		LongDesc: `
Usage: help <cmd>

This command shows detailed usage instructions for other commands.
`},
	"member": cmd{
		Name:      "member",
		Subcmd:    true,
		ShortDesc: "List and manage cluster members",
		LongDesc: `
Usage: member ls

This command can be used to list and manage IPFS Cluster members.
`},
	"member ls": cmd{
		Name:      "memberList",
		Path:      "/members",
		Method:    "GET",
		ShortDesc: "List cluster members",
		LongDesc: `
Usage: member ls

This command lists the nodes participating in the IPFS Cluster.
`},
	"pin": cmd{
		Name:      "pin",
		Path:      "",
		Subcmd:    true,
		ShortDesc: "Manage tracked CIDs",
		LongDesc: `
Usage: pin add|rm|ls [cid]

This command allows to add, remove or list items managed (pinned) by
the Cluster.
`},
	"pin add": cmd{
		Name:      "pinAdd",
		Path:      "/pins/{param0}",
		Method:    "POST",
		ShortDesc: "Track a CID (pin)",
		LongDesc: `
Usage: pin add <cid>

This command tells IPFS Cluster to start managing a CID. Depending on
the pinning strategy, this will trigger IPFS pin requests. The CID will
become part of the Cluster's state and will tracked from this point.
`},
	"pin rm": cmd{
		Name:      "pinRm",
		Path:      "/pins/{param0}",
		Method:    "DELETE",
		Subcmd:    false,
		ShortDesc: "Stop tracking a CID (unpin)",
		LongDesc: `
Usage: pin rm <cid>

This command tells IPFS Cluster to no longer manage a CID. This will
trigger unpinning operations in all the IPFS nodes holding the content.
`},
	"pin ls": cmd{
		Name:      "pinLs",
		Path:      "/pins",
		Method:    "GET",
		ShortDesc: "List tracked CIDs",
		LongDesc: `
Usage: pin ls

This command will list the CIDs which are tracked by IPFS Cluster. This
list does not include information about tracking status or location, it
merely represents the list of pins which are part of the global state of
the cluster. For specific information, use "status".
`},
	/*
		member list
		pin list
		pin add cid
		pin rm cid
		status
		status cid
		sync
		sync cid

		version
	*/
	"status": cmd{
		Name:      "status",
		Path:      "/status/{param0}",
		Method:    "GET",
		Subcmd:    false,
		ShortDesc: "Retrieve status of tracked items",
		LongDesc: `
Usage: status [cid]

This command retrieves the status of the CIDs tracked by IPFS
Cluster, including which member is pinning them and any errors.
If a CID is provided, the status will be only fetched for a single
item.
`},
	"sync": cmd{
		Name:      "sync",
		Path:      "/status/{param0}",
		Method:    "POST",
		Subcmd:    false,
		ShortDesc: "Sync status and/or recover tracked items",
		LongDesc: `
Usage: sync [cid]

This command verifies that the current status tracked CIDs are accurate by
triggering queries to the IPFS daemons that pin them. When the CID is in
error state, either because pinning or unpinning failed, IPFS Cluster will
attempt to retry the operation. If a CID is provided, the sync and recover
operations will be limited to that single item.
`},
	"version": cmd{
		Name:      "version",
		Path:      "/version",
		Method:    "GET",
		ShortDesc: "Retrieve cluster version",
		LongDesc: `
Usage: version

This command retrieves the IPFS Cluster version. It is advisable that
it matches the one returned by -version.
`},
}

func init() {
	flag.Usage = func() {
		out("Usage: %s [options] <cmd> [cmd_options]\n", programName)
		out(Description)
		out("Options:\n")
		flag.PrintDefaults()
		out("\n")
	}
	flag.StringVar(&hostFlag, "host", defaultHost,
		"host:port of the IPFS Cluster Server API")
	flag.StringVar(&protocolFlag, "protocol", defaultProtocol,
		"protocol used by the API: usually http or https")
	flag.IntVar(&timeoutFlag, "timeout", defaultTimeout,
		"how many seconds before timing out a Cluster API request")
	flag.BoolVar(&versionFlag, "version", false,
		fmt.Sprintf("display %s version", programName))
	flag.BoolVar(&debugFlag, "debug", false,
		"set debug log level")
	flag.Parse()
	defaultHost = hostFlag
	defaultProtocol = protocolFlag
	if debugFlag {
		logging.SetLogLevel("ipfscluster", "debug")
	}
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
	cmd := getCmd(0, "")
	switch cmd.Name {
	case "help":
		cmdName := flag.Arg(0)
		if cmdName == "help" {
			cmdName = flag.Arg(1)
		}
		usage, ok := cmds[cmdName]
		if !ok {
			keys := make([]string, 0, len(cmds))
			for k := range cmds {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			out("%s - available commands:\n\n", programName)
			for _, k := range keys {
				if cmds[k].Subcmd == false {
					out("%s - %s\n", k, cmds[k].ShortDesc)
				}
			}
		} else {
			out(usage.LongDesc + "\n")
		}
	case "memberList":
		resp := request(cmd.Method, cmd.Path)
		formatResponse(resp)
	case "pinAdd":
		cidStr := flag.Arg(2)
		_, err := cid.Decode(cidStr)
		checkErr("parsing cid", err)
		resp := request(cmd.Method, cmd.Path, cidStr)
		formatResponse(resp)
	case "pinRm":
		cidStr := flag.Arg(2)
		_, err := cid.Decode(cidStr)
		checkErr("parsing cid", err)
		resp := request(cmd.Method, cmd.Path, cidStr)
		formatResponse(resp)
	case "pinLs":
		resp := request(cmd.Method, cmd.Path)
		formatResponse(resp)
	case "status":
		cidStr := flag.Arg(1)
		if cidStr != "" {
			_, err := cid.Decode(cidStr)
			checkErr("parsing cid", err)
		}
		resp := request(cmd.Method, cmd.Path, cidStr)
		formatResponse(resp)
	case "sync":
		cidStr := flag.Arg(1)
		if cidStr != "" {
			_, err := cid.Decode(cidStr)
			checkErr("parsing cid", err)
		}
		resp := request(cmd.Method, cmd.Path, cidStr)
		formatResponse(resp)
	case "version":
		resp := request(cmd.Method, cmd.Path)
		formatResponse(resp)
	default:
		err := fmt.Errorf("wrong command. Try \"%s help\" for help",
			programName)
		checkErr("", err)
		os.Exit(1)
	}
}

func getCmd(nArg int, prefix string) cmd {
	arg := flag.Arg(nArg)
	cmdStr := arg
	if prefix != "" {
		cmdStr = prefix + " " + arg
	}

	cmd, ok := cmds[cmdStr]
	if !ok {
		if arg != "" {
			out("error: command not found\n\n")
		}
		return cmds["help"]
	}
	if cmd.Subcmd {
		return getCmd(nArg+1, cmdStr)
	}
	return cmd

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
		out("Request accepted\n\n")
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
