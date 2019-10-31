package test

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/datastore/inmem"
	"github.com/ipfs/ipfs-cluster/state"
	"github.com/ipfs/ipfs-cluster/state/dsstate"

	cid "github.com/ipfs/go-cid"
	u "github.com/ipfs/go-ipfs-util"
	cors "github.com/rs/cors"
)

// Some values used by the ipfs mock
const (
	IpfsCustomHeaderName  = "X-Custom-Header"
	IpfsTimeHeaderName    = "X-Time-Now"
	IpfsCustomHeaderValue = "42"
	IpfsACAOrigin         = "myorigin"
	IpfsErrFromNotPinned  = "'from' cid was not recursively pinned already"
)

// IpfsMock is an ipfs daemon mock which should sustain the functionality used by ipfscluster.
type IpfsMock struct {
	server     *httptest.Server
	Addr       string
	Port       int
	pinMap     state.State
	BlockStore map[string][]byte
	reqCounts  map[string]int
	reqCounter chan string

	closeMux sync.Mutex
	closed   bool
}

type mockPinResp struct {
	Pins     []string
	Progress int `json:",omitempty"`
}

type mockPinType struct {
	Type string
}

type mockPinLsResp struct {
	Keys map[string]mockPinType
}

type ipfsErr struct {
	Code    int
	Message string
}

type mockIDResp struct {
	ID        string
	Addresses []string
}

type mockRepoStatResp struct {
	RepoSize   uint64
	NumObjects uint64
	StorageMax uint64
}

type mockConfigResp struct {
	Datastore struct {
		StorageMax string
	}
}

type mockAddResp struct {
	Name  string
	Hash  string
	Bytes uint64
}

type mockRefsResp struct {
	Ref string
	Err string
}

type mockSwarmPeersResp struct {
	Peers []mockIpfsPeer
}

type mockIpfsPeer struct {
	Peer string
}

type mockBlockPutResp struct {
	Key string
}

// NewIpfsMock returns a new mock.
func NewIpfsMock(t *testing.T) *IpfsMock {
	store := inmem.New()
	st, err := dsstate.New(store, "", dsstate.DefaultHandle())
	if err != nil {
		t.Fatal(err)
	}

	m := &IpfsMock{
		pinMap:     st,
		BlockStore: make(map[string][]byte),
		reqCounts:  make(map[string]int),
		reqCounter: make(chan string, 100),
	}

	go m.countRequests()

	mux := http.NewServeMux()
	mux.HandleFunc("/", m.handler)

	c := cors.New(cors.Options{
		AllowedOrigins:   []string{IpfsACAOrigin},
		AllowedMethods:   []string{"POST"},
		ExposedHeaders:   []string{"X-Stream-Output", "X-Chunked-Output", "X-Content-Length"},
		AllowCredentials: true, // because IPFS does it, even if for no reason.
	})
	corsHandler := c.Handler(mux)

	ts := httptest.NewServer(corsHandler)
	m.server = ts

	url, _ := url.Parse(ts.URL)
	h := strings.Split(url.Host, ":")
	i, _ := strconv.Atoi(h[1])

	m.Port = i
	m.Addr = h[0]
	return m

}

func (m *IpfsMock) countRequests() {
	for str := range m.reqCounter {
		m.reqCounts[str]++
	}
}

// GetCount allows to get the number of times and endpoint was called.
// Do not use concurrently to requests happening.
func (m *IpfsMock) GetCount(path string) int {
	return m.reqCounts[path]
}

// FIXME: what if IPFS API changes?
func (m *IpfsMock) handler(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	p := r.URL.Path
	w.Header().Set(IpfsCustomHeaderName, IpfsCustomHeaderValue)
	w.Header().Set("Server", "ipfs-mock")
	w.Header().Set(IpfsTimeHeaderName, fmt.Sprintf("%d", time.Now().Unix()))
	endp := strings.TrimPrefix(p, "/api/v0/")

	m.reqCounter <- endp

	switch endp {
	case "id":
		resp := mockIDResp{
			ID: PeerID1.Pretty(),
			Addresses: []string{
				"/ip4/0.0.0.0/tcp/1234",
			},
		}
		j, _ := json.Marshal(resp)
		w.Write(j)
	case "pin/add":
		arg, ok := extractCid(r.URL)
		if !ok {
			goto ERROR
		}
		if arg == ErrorCid.String() {
			goto ERROR
		}
		c, err := cid.Decode(arg)
		if err != nil {
			goto ERROR
		}
		m.pinMap.Add(ctx, api.PinCid(c))
		resp := mockPinResp{
			Pins: []string{arg},
		}

		if c.Equals(SlowCid1) {
			for i := 0; i <= 10; i++ {
				time.Sleep(1 * time.Second)
				resp.Progress = i
				j, _ := json.Marshal(resp)
				w.Write(j)
			}
		} else {
			j, _ := json.Marshal(resp)
			w.Write(j)
		}
	case "pin/rm":
		arg, ok := extractCid(r.URL)
		if !ok {
			goto ERROR
		}
		c, err := cid.Decode(arg)
		if err != nil {
			goto ERROR
		}
		m.pinMap.Rm(ctx, c)
		resp := mockPinResp{
			Pins: []string{arg},
		}
		j, _ := json.Marshal(resp)
		w.Write(j)
	case "pin/update":
		args := r.URL.Query()["arg"]
		if len(args) != 2 {
			goto ERROR
		}
		fromStr := args[0]
		toStr := args[1]
		from, err := cid.Decode(fromStr)
		if err != nil {
			goto ERROR
		}
		to, err := cid.Decode(toStr)
		if err != nil {
			goto ERROR
		}

		pin, err := m.pinMap.Get(ctx, from)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			resp := ipfsErr{0, IpfsErrFromNotPinned}
			j, _ := json.Marshal(resp)
			w.Write(j)
			return
		}
		pin.Cid = to
		err = m.pinMap.Add(ctx, pin)
		if err != nil {
			goto ERROR
		}

		resp := mockPinResp{
			Pins: []string{from.String(), to.String()},
		}

		j, _ := json.Marshal(resp)
		w.Write(j)
	case "pin/ls":
		arg, ok := extractCid(r.URL)
		if !ok {
			rMap := make(map[string]mockPinType)
			pins, err := m.pinMap.List(ctx)
			if err != nil {
				goto ERROR
			}
			for _, p := range pins {
				rMap[p.Cid.String()] = mockPinType{"recursive"}
			}
			j, _ := json.Marshal(mockPinLsResp{rMap})
			w.Write(j)
			break
		}

		cidStr := arg
		c, err := cid.Decode(cidStr)
		if err != nil {
			goto ERROR
		}

		ok, err = m.pinMap.Has(ctx, c)
		if err != nil {
			goto ERROR
		}
		if ok {
			if c.Equals(Cid4) { // this a v1 cid. Do not return default-base32
				w.Write([]byte(`{ "Keys": { "zb2rhiKhUepkTMw7oFfBUnChAN7ABAvg2hXUwmTBtZ6yxuc57": { "Type": "recursive" }}}`))
				return
			}

			rMap := make(map[string]mockPinType)
			rMap[cidStr] = mockPinType{"recursive"}
			j, _ := json.Marshal(mockPinLsResp{rMap})
			w.Write(j)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			resp := ipfsErr{0, fmt.Sprintf("Path '%s' is not pinned", cidStr)}
			j, _ := json.Marshal(resp)
			w.Write(j)
		}
	case "swarm/connect":
		arg, ok := extractCid(r.URL)
		if !ok {
			goto ERROR
		}
		addr := arg
		splits := strings.Split(addr, "/")
		pid := splits[len(splits)-1]
		resp := struct {
			Strings []string
		}{
			Strings: []string{fmt.Sprintf("connect %s success", pid)},
		}
		j, _ := json.Marshal(resp)
		w.Write(j)
	case "swarm/peers":
		peer1 := mockIpfsPeer{
			Peer: PeerID4.Pretty(),
		}
		peer2 := mockIpfsPeer{
			Peer: PeerID5.Pretty(),
		}
		resp := mockSwarmPeersResp{
			Peers: []mockIpfsPeer{peer1, peer2},
		}
		j, _ := json.Marshal(resp)
		w.Write(j)
	case "block/put":
		// Get the data and retun the hash
		mpr, err := r.MultipartReader()
		if err != nil {
			goto ERROR
		}
		part, err := mpr.NextPart()
		if err != nil {
			goto ERROR
		}
		data, err := ioutil.ReadAll(part)
		if err != nil {
			goto ERROR
		}
		// Parse cid from data and format and add to mock block-store
		query := r.URL.Query()
		format, ok := query["f"]
		if !ok || len(format) != 1 {
			goto ERROR
		}
		var c string
		hash := u.Hash(data)
		codec, ok := cid.Codecs[format[0]]
		if !ok {
			goto ERROR
		}
		if format[0] == "v0" {
			c = cid.NewCidV0(hash).String()
		} else {
			c = cid.NewCidV1(codec, hash).String()
		}
		m.BlockStore[c] = data

		resp := mockBlockPutResp{
			Key: c,
		}
		j, _ := json.Marshal(resp)
		w.Write(j)
	case "block/get":
		query := r.URL.Query()
		arg, ok := query["arg"]
		if !ok {
			goto ERROR
		}
		if len(arg) != 1 {
			goto ERROR
		}
		data, ok := m.BlockStore[arg[0]]
		if !ok {
			goto ERROR
		}
		w.Write(data)
	case "repo/stat":
		sizeOnly := r.URL.Query().Get("size-only")
		list, err := m.pinMap.List(ctx)
		if err != nil {
			goto ERROR
		}
		len := len(list)
		numObjs := uint64(len)
		if sizeOnly == "true" {
			numObjs = 0
		}
		resp := mockRepoStatResp{
			RepoSize:   uint64(len) * 1000,
			NumObjects: numObjs,
			StorageMax: 10000000000, //10 GB
		}
		j, _ := json.Marshal(resp)
		w.Write(j)
	case "resolve":
		w.Write([]byte("{\"Path\":\"" + "/ipfs/" + CidResolved.String() + "\"}"))
	case "config/show":
		resp := mockConfigResp{
			Datastore: struct {
				StorageMax string
			}{
				StorageMax: "10G",
			},
		}
		j, _ := json.Marshal(resp)
		w.Write(j)
	case "refs":
		arg, ok := extractCid(r.URL)
		if !ok {
			goto ERROR
		}
		resp := mockRefsResp{
			Ref: arg,
		}
		j, _ := json.Marshal(resp)
		if arg == SlowCid1.String() {
			for i := 0; i <= 5; i++ {
				time.Sleep(2 * time.Second)
				w.Write(j)
			}
		} else {
			w.Write(j)
		}
	case "version":
		w.Write([]byte("{\"Version\":\"m.o.c.k\"}"))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
	return
ERROR:
	w.WriteHeader(http.StatusInternalServerError)
}

// Close closes the mock server. It's important to call after each test or
// the listeners are left hanging around.
func (m *IpfsMock) Close() {
	m.closeMux.Lock()
	defer m.closeMux.Unlock()
	if !m.closed {
		m.closed = true
		m.server.Close()
		close(m.reqCounter)
	}
}

// extractCid extracts the cid argument from a url.URL, either via
// the query string parameters or from the url path itself.
func extractCid(u *url.URL) (string, bool) {
	arg := u.Query().Get("arg")
	if arg != "" {
		return arg, true
	}

	p := strings.TrimPrefix(u.Path, "/api/v0/")
	segs := strings.Split(p, "/")

	if len(segs) > 2 {
		return segs[len(segs)-1], true
	}
	return "", false
}
