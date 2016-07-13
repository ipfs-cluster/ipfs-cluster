package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	api "github.com/ipfs/go-ipfs-api"
	"golang.org/x/net/context"
)

const ClusterVersion = "0.0.0"

type Cluster struct {
	shell   *api.Shell
	ipfsapi string
}

func NewCluster(ipfsapi string) *Cluster {
	return &Cluster{
		shell:   api.NewShell(ipfsapi),
		ipfsapi: ipfsapi,
	}
}

func respondJson(w http.ResponseWriter, i interface{}) {
	enc := json.NewEncoder(w)
	err := enc.Encode(i)
	if err != nil {
		log.Println("error responding: ", err)
	}
}

func (c *Cluster) GetStatus() (interface{}, error) {
	status := make(map[string]interface{})

	_, _, err := c.shell.Version()
	status["online"] = (err == nil)

	return status, nil
}

func (c *Cluster) apiHandlerFunc(w http.ResponseWriter, r *http.Request) {
	path := strings.Split(r.URL.Path, "/")[1:]
	if len(path) == 0 {
		w.WriteHeader(404)
		return
	}
	switch path[0] {
	case "version":
		respondJson(w, map[string]interface{}{"version": ClusterVersion})
	case "status":
		out, err := c.GetStatus()
		if err != nil {
			w.WriteHeader(503)
			log.Println("get status error: ", err)
		}

		respondJson(w, out)
	case "join":
		host := r.URL.Query().Get("host")
		panic("not yet implemented")
	default:
		w.WriteHeader(404)
	}
}

func (c *Cluster) StartAPIServer(ctx context.Context, addr string) error {
	smux := http.NewServeMux()
	smux.HandleFunc("/", c.apiHandlerFunc)
	go func() {
		err := http.ListenAndServe(addr, smux)
		if err != nil {
			panic(err)
		}
	}()
	return nil
}

func (c *Cluster) StartIPFSHandler(ctx context.Context, addr string) error {
	smux := http.NewServeMux()
	smux.HandleFunc("/", c.ipfsHandlerFunc)
	go func() {
		err := http.ListenAndServe(addr, smux)
		if err != nil {
			panic(err)
		}
	}()
	return nil
}

func (c *Cluster) Start() error {
	ctx, cancel := context.WithCancel(context.Background())
	err := c.StartIPFSHandler(ctx, "localhost:5101")
	if err != nil {
		return err
	}

	err = c.StartAPIServer(ctx, "localhost:5100")
	if err != nil {
		return err
	}

	_ = cancel

	// start clusterd messaging protocol server

	// join to other nodes in cluster

	// hang and serve
	<-ctx.Done()
	return nil
}

func main() {
	c := NewCluster("localhost:5001")
	err := c.Start()
	if err != nil {
		log.Fatal(err)
	}
}
