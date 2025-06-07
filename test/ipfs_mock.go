package test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/ipfs-cluster/ipfs-cluster/datastore/inmem"
	"github.com/ipfs-cluster/ipfs-cluster/state"
	"github.com/ipfs-cluster/ipfs-cluster/state/dsstate"
	"github.com/multiformats/go-multicodec"

	cid "github.com/ipfs/go-cid"
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
	reqCounter chan string

	reqCountsMux sync.Mutex // guards access to reqCounts
	reqCounts    map[string]int

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

type mockPinLsAllResp struct {
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

type mockDagPutResp struct {
	Cid cid.Cid
}

type mockRepoGCResp struct {
	Key   cid.Cid `json:",omitempty"`
	Error string  `json:",omitempty"`
}

// NewIpfsMock returns a new mock.
func NewIpfsMock(t *testing.T) *IpfsMock {
	store := inmem.New()
	st, err := dsstate.New(context.Background(), store, "", dsstate.DefaultHandle())
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
		m.reqCountsMux.Lock()
		m.reqCounts[str]++
		m.reqCountsMux.Unlock()
	}
}

// GetCount allows to get the number of times and endpoint was called.
func (m *IpfsMock) GetCount(path string) int {
	m.reqCountsMux.Lock()
	defer m.reqCountsMux.Unlock()
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
			ID: PeerID1.String(),
			Addresses: []string{
				"/ip4/0.0.0.0/tcp/1234",
				"/ip6/::/tcp/1234",
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
		c, err := api.DecodeCid(arg)
		if err != nil {
			goto ERROR
		}
		mode := extractMode(r.URL)
		opts := api.PinOptions{
			Mode: mode,
		}
		pinObj := api.PinWithOpts(c, opts)
		m.pinMap.Add(ctx, pinObj)
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
		c, err := api.DecodeCid(arg)
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
		from, err := api.DecodeCid(fromStr)
		if err != nil {
			goto ERROR
		}
		to, err := api.DecodeCid(toStr)
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
		query := r.URL.Query()
		stream := query.Get("stream") == "true"

		arg, ok := extractCid(r.URL)
		if !ok {
			pins := make(chan api.Pin, 10)

			go func() {
				m.pinMap.List(ctx, pins)
			}()

			if stream {
				for p := range pins {
					j, _ := json.Marshal(api.IPFSPinInfo{
						Cid:  api.Cid(p.Cid),
						Type: p.Mode.ToIPFSPinStatus(),
					})
					w.Write(j)
				}
				break
			} else {
				rMap := make(map[string]mockPinType)
				for p := range pins {
					rMap[p.Cid.String()] = mockPinType{p.Mode.String()}
				}
				j, _ := json.Marshal(mockPinLsAllResp{rMap})
				w.Write(j)
				break
			}
		}

		cidStr := arg
		c, err := api.DecodeCid(cidStr)
		if err != nil {
			goto ERROR
		}

		pinObj, err := m.pinMap.Get(ctx, c)
		if err != nil && err != state.ErrNotFound {
			goto ERROR
		}
		if err == state.ErrNotFound {
			w.WriteHeader(http.StatusInternalServerError)
			resp := ipfsErr{0, fmt.Sprintf("Path '%s' is not pinned", cidStr)}
			j, _ := json.Marshal(resp)
			w.Write(j)
			return
		}

		if stream {
			if c.Equals(Cid4) {
				// this a v1 cid. Do not return default-base32 but base58btc encoding of it
				w.Write([]byte(`{ "Cid": "zCT5htkdztJi3x4zBNHo8TRvGHPLTdHUdCLKgTGMgQcRKSLoWxK1", "Type": "recursive" }`))
				break
			}
			j, _ := json.Marshal(api.IPFSPinInfo{
				Cid:  api.Cid(pinObj.Cid),
				Type: pinObj.Mode.ToIPFSPinStatus(),
			})
			w.Write(j)
		} else {
			if c.Equals(Cid4) {
				// this a v1 cid. Do not return default-base32 but base58btc encoding of it
				w.Write([]byte(`{ "Keys": { "zCT5htkdztJi3x4zBNHo8TRvGHPLTdHUdCLKgTGMgQcRKSLoWxK1": { "Type": "recursive" }}}`))
				break
			}
			rMap := make(map[string]mockPinType)
			rMap[cidStr] = mockPinType{pinObj.Mode.String()}
			j, _ := json.Marshal(mockPinLsAllResp{rMap})
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
			Peer: PeerID4.String(),
		}
		peer2 := mockIpfsPeer{
			Peer: PeerID5.String(),
		}
		resp := mockSwarmPeersResp{
			Peers: []mockIpfsPeer{peer1, peer2},
		}
		j, _ := json.Marshal(resp)
		w.Write(j)
	case "block/put":
		w.Header().Set("Trailer", "X-Stream-Error")

		query := r.URL.Query()
		mc := multicodec.Raw
		if cdcstr := query.Get("cid-codec"); cdcstr != "" {
			mc.Set(cdcstr)
		}

		mhType := multicodec.Sha2_256
		if mh := query.Get("mhtype"); mh != "" {
			mhType.Set(mh)
		}
		mhLen := -1
		if l := query.Get("mhlen"); l != "" {
			mhLen, _ = strconv.Atoi(l)
		}

		// Get the data and retun the hash
		mpr, err := r.MultipartReader()
		if err != nil {
			goto ERROR
		}

		w.WriteHeader(http.StatusOK)

		for {
			part, err := mpr.NextPart()
			if err == io.EOF {
				return
			}
			if err != nil {
				w.Header().Set("X-Stream-Error", err.Error())
				return
			}
			data, err := io.ReadAll(part)
			if err != nil {
				w.Header().Set("X-Stream-Error", err.Error())
				return
			}
			// Parse cid from data and format and add to mock block-store
			builder := cid.V1Builder{
				Codec:    uint64(mc),
				MhType:   uint64(mhType),
				MhLength: mhLen,
			}

			c, err := builder.Sum(data)
			if err != nil {
				w.Header().Set("X-Stream-Error", err.Error())
				return
			}
			m.BlockStore[c.String()] = data

			resp := mockBlockPutResp{
				Key: c.String(),
			}
			j, _ := json.Marshal(resp)
			w.Write(j)
		}
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
	case "dag/put":
		// DAG-put is a fake implementation as we are not going to
		// parse the input and we are just going to hash it and return
		// a response.
		w.Header().Set("Trailer", "X-Stream-Error")

		query := r.URL.Query()
		storeCodec := query.Get("store-codec")
		codec := multicodec.DagJson
		if storeCodec != "" {
			codec.Set(storeCodec)
		}
		hashFunc := query.Get("hash")
		hash := multicodec.Sha2_256
		if hashFunc != "" {
			hash.Set(hashFunc)
		}

		// Get the data and retun the hash
		mpr, err := r.MultipartReader()
		if err != nil {
			goto ERROR
		}

		w.WriteHeader(http.StatusOK)

		for {
			part, err := mpr.NextPart()
			if err == io.EOF {
				return
			}
			if err != nil {
				w.Header().Set("X-Stream-Error", err.Error())
				return
			}
			data, err := io.ReadAll(part)
			if err != nil {
				w.Header().Set("X-Stream-Error", err.Error())
				return
			}
			// Parse cid from data and format and add to mock block-store
			builder := cid.V1Builder{
				Codec:    uint64(codec),
				MhType:   uint64(hash),
				MhLength: -1,
			}

			c, err := builder.Sum(data)
			if err != nil {
				w.Header().Set("X-Stream-Error", err.Error())
				return
			}

			resp := mockDagPutResp{
				Cid: c,
			}
			j, _ := json.Marshal(resp)
			w.Write(j)
		}
	case "repo/gc":
		// It assumes `/repo/gc` with parameter `stream-errors=true`
		enc := json.NewEncoder(w)
		resp := []mockRepoGCResp{
			{
				Key: Cid1.Cid,
			},
			{
				Key: Cid2.Cid,
			},
			{
				Key: Cid3.Cid,
			},
			{
				Key: Cid4.Cid,
			},
			{
				Error: "no link by that name",
			},
		}

		for _, r := range resp {
			if err := enc.Encode(&r); err != nil {
				goto ERROR
			}
		}

	case "repo/stat":
		sizeOnly := r.URL.Query().Get("size-only")
		pinsCh := make(chan api.Pin, 10)
		go func() {
			m.pinMap.List(ctx, pinsCh)
		}()

		var pins []api.Pin
		for p := range pinsCh {
			pins = append(pins, p)
		}

		len := len(pins)
		numObjs := uint64(len)
		if sizeOnly == "true" {
			numObjs = 0
		}
		resp := mockRepoStatResp{
			RepoSize:   uint64(len) * 1000,
			NumObjects: numObjs,
			StorageMax: 10000000000, // 10 GB
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

func extractMode(u *url.URL) api.PinMode {
	return api.PinModeFromString(u.Query().Get("type"))
}
