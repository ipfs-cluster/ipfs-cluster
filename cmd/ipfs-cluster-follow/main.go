// The ipfs-cluster-follow application.
package main

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"

	"github.com/ipfs/ipfs-cluster/api/rest/client"
	"github.com/ipfs/ipfs-cluster/cmdutils"
	"github.com/ipfs/ipfs-cluster/version"
	"github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"

	semver "github.com/blang/semver"
	logging "github.com/ipfs/go-log"
	cli "github.com/urfave/cli/v2"
)

const (
	// ProgramName of this application
	programName     = "ipfs-cluster-follow"
	clusterNameFlag = "clusterName"
	logLevel        = "info"
)

// We store a commit id here
var commit string

// Description provides a short summary of the functionality of this tool
var Description = fmt.Sprintf(`
%s helps running IPFS Cluster follower peers.

Follower peers subscribe to a Cluster controlled by a set of "trusted
peers". They collaborate in pinning items as dictated by the trusted peers and
do not have the power to make Cluster-wide modifications to the pinset.

Follower peers cannot access information nor trigger actions in other peers.

%s can be used to follow different clusters by launching it 
with different options. Each Cluster has an identity, a configuration 
and a datastore associated to it, which are kept under 
"~/%s/<cluster_name>".

For feedback, bug reports or any additional information, visit
https://github.com/ipfs/ipfs-cluster.


EXAMPLES:

List configured follower peers:

$ %s

Display information for a follower peer:

$ %s <clusterName> info

Initialize a follower peer:

$ %s <clusterName> init <example.url>

Launch a follower peer (will stay running):

$ %s <clusterName> run

List items in the pinset for a given cluster:

$ %s <clusterName> list

Getting help and usage info:

$ %s --help
$ %s <clusterName> --help
$ %s <clusterName> info --help
$ %s <clusterName> init --help
$ %s <clusterName> run --help
$ %s <clusterName> list --help

`,
	programName,
	programName,
	DefaultFolder,
	programName,
	programName,
	programName,
	programName,
	programName,
	programName,
	programName,
	programName,
	programName,
	programName,
	programName,
)

var logger = logging.Logger("clusterfollow")

// Default location for the configurations and data
var (
	// DefaultFolder is the name of the cluster folder
	DefaultFolder = ".ipfs-cluster-follow"
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

func main() {
	app := cli.NewApp()
	app.Name = programName
	app.Usage = "IPFS Cluster Follower"
	app.UsageText = fmt.Sprintf("%s [global options] <clusterName> [subcommand]...", programName)
	app.Description = Description
	//app.Copyright = "© Protocol Labs, Inc."
	app.Version = version.Version.String()
	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:    "config, c",
			Value:   DefaultPath,
			Usage:   "path to the followers configuration and data `FOLDER`",
			EnvVars: []string{"IPFS_CLUSTER_PATH"},
		},
	}

	app.Action = func(c *cli.Context) error {
		if !c.Args().Present() {
			return listCmd(c)
		}

		clusterName := c.Args().Get(0)
		clusterApp := cli.NewApp()
		clusterApp.Name = fmt.Sprintf("%s %s", programName, clusterName)
		clusterApp.HelpName = clusterApp.Name
		clusterApp.Usage = fmt.Sprintf("Follower peer management for \"%s\"", clusterName)
		clusterApp.UsageText = fmt.Sprintf("%s %s [subcommand]", programName, clusterName)
		clusterApp.Action = infoCmd
		clusterApp.HideVersion = true
		clusterApp.Flags = []cli.Flag{
			&cli.StringFlag{ // pass clusterName to subcommands
				Name:   clusterNameFlag,
				Value:  clusterName,
				Hidden: true,
			},
		}
		clusterApp.Commands = []*cli.Command{
			{
				Name:      "info",
				Usage:     "displays information for this peer",
				ArgsUsage: "",
				Description: fmt.Sprintf(`
This command display useful information for "%s"'s follower peer.
`, clusterName),
				Action: infoCmd,
			},
			{
				Name:      "init",
				Usage:     "initializes the follower peer",
				ArgsUsage: "<template_URL>",
				Description: fmt.Sprintf(`
This command initializes a follower peer for the cluster named "%s". You
will need to pass the peer configuration URL. The command will generate a new
peer identity and leave things ready to run "%s %s run".

An error will be returned if a configuration folder for a cluster peer with
this name already exists. If you wish to re-initialize from scratch, delete
this folder first.
`, clusterName, programName, clusterName),
				Action: initCmd,
			},
			{
				Name:      "run",
				Usage:     "runs the follower peer",
				ArgsUsage: "",
				Description: fmt.Sprintf(`

This commands runs a "%s" cluster follower peer. The peer should have already
been initialized with "init" alternatively the --init flag needs to be
passed.

Before running, ensure that you have connectivity and that the IPFS daemon is
running.

You can obtain more information about this follower peer by running
"%s %s" (without any arguments).

The peer will stay running in the foreground until manually stopped.
`, clusterName, programName, clusterName),
				Action: runCmd,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "init",
						Usage: "initialize cluster peer with the given URL before running",
					},
				},
			},
			{
				Name:      "list",
				Usage:     "list items in the peers' pinset",
				ArgsUsage: "",
				Description: `

This commands lists all the items pinned by this follower cluster peer on IPFS.

If the peer is currently running, it will display status information for each
pin (such as PINNING). If not, it will just display the current list of pins
as obtained from the internal state on disk.
`,
				Action: pinsetCmd,
			},
		}
		return clusterApp.RunAsSubcommand(c)
	}

	app.Run(os.Args)
}

// build paths returns the path to the configuration folder,
// the identify and the service.json files.
func buildPaths(c *cli.Context, clusterName string) (string, string, string) {
	absPath, err := filepath.Abs(c.String("config"))
	if err != nil {
		cmdutils.ErrorOut("error getting aboslute path for %s: %s", err, clusterName)
		os.Exit(1)
	}

	// ~/.ipfs-cluster-follow/clusterName
	absPath = filepath.Join(absPath, clusterName)
	// ~/.ipfs-cluster-follow/clusterName/service.json
	configPath = filepath.Join(absPath, DefaultConfigFile)
	// ~/.ipfs-cluster-follow/clusterName/indentity.json
	identityPath = filepath.Join(absPath, DefaultIdentityFile)

	return absPath, configPath, identityPath
}

func socketAddress(absPath, clusterName string) (multiaddr.Multiaddr, error) {
	socket := fmt.Sprintf("/unix/%s", filepath.Join(absPath, "api-socket"))
	ma, err := multiaddr.NewMultiaddr(socket)
	if err != nil {
		return nil, errors.Wrapf(err, "error parsing socket: %s", socket)
	}
	return ma, nil
}

func getClient(absPath, clusterName string) (client.Client, error) {
	endp, err := socketAddress(absPath, clusterName)
	if err != nil {
		return nil, err
	}
	cfg := client.Config{
		APIAddr: endp,
	}
	return client.NewDefaultClient(&cfg)
}
