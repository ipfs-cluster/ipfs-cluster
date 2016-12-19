package main

import (
	"fmt"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"

	logging "github.com/ipfs/go-log"

	ipfscluster "github.com/ipfs/ipfs-cluster"
)

func main() {
	logging.SetLogLevel("ipfs-cluster", "debug")
	signalChan := make(chan os.Signal, 1)
	cleanup := make(chan bool)
	signal.Notify(signalChan, os.Interrupt)

	usr, _ := user.Current()
	home := usr.HomeDir

	clusterCfg, err := ipfscluster.LoadConfig(filepath.Join(home, "ipfs-cluster.json"))
	if err != nil {
		fmt.Println(err)
		return
	}
	api, err := ipfscluster.NewRESTAPI(clusterCfg)
	if err != nil {
		fmt.Println(err)
		return
	}

	proxy, err := ipfscluster.NewIPFSHTTPConnector(clusterCfg)
	if err != nil {
		fmt.Println(err)
		return
	}

	state := ipfscluster.NewMapState()
	tracker := ipfscluster.NewMapPinTracker(clusterCfg)
	remote := ipfscluster.NewLibp2pRemote()

	cluster, err := ipfscluster.NewCluster(clusterCfg, api, proxy, state, tracker, remote)
	if err != nil {
		fmt.Println(err)
		return
	}

	go func() {
		<-signalChan
		fmt.Println("caught signal")
		cluster.Shutdown()
		cleanup <- true
	}()
	<-cleanup
}
