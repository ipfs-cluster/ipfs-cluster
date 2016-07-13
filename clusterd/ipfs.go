package main

import (
	"io"
	"log"
	"net/http"
)

// ipfsHandlerFunc implements a basic 'pass through' proxy for an ipfs daemon
func (c *Cluster) ipfsHandlerFunc(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[8:]
	switch path {
	case "pin/add":
		log.Println("pin request")
	default:
		log.Printf("path: %s", path)
	}

	url := *r.URL
	url.Host = "localhost:5001"
	url.Scheme = "http"

	req, err := http.NewRequest(r.Method, url.String(), r.Body)
	if err != nil {
		log.Printf("error creating request: ", err)
		http.Error(w, "error forwaring request", 501)
		return
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("error forwarding request: ", err)
		http.Error(w, "error forwaring request", 501)
		return
	}

	for k, v := range resp.Header {
		for _, s := range v {
			w.Header().Add(k, s)
		}
	}

	io.Copy(w, resp.Body)
}
