package main

import (
	"io"
	"net/http"
	"strings"
)

// ipfsHandlerFunc implements a basic 'pass through' proxy for an ipfs daemon
func (c *Cluster) ipfsHandlerFunc(w http.ResponseWriter, r *http.Request) {
	path := strings.Split(r.URL.Path, "/")[1:]
	if len(path) == 0 {
		w.WriteHeader(404)
	}

	_ = path

	url := *r.URL
	url.Host = "localhost:5001"
	url.Scheme = "http"

	req, err := http.NewRequest(r.Method, url.String(), r.Body)
	if err != nil {
		panic(err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		panic(err)
	}

	for k, v := range resp.Header {
		for _, s := range v {
			w.Header().Add(k, s)
		}
	}

	io.Copy(w, resp.Body)
}
