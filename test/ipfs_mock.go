package test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/state/mapstate"

	cid "github.com/ipfs/go-cid"
	u "github.com/ipfs/go-ipfs-util"
)

// IpfsMock is an ipfs daemon mock which should sustain the functionality used by ipfscluster.
type IpfsMock struct {
	server     *httptest.Server
	Addr       string
	Port       int
	pinMap     *mapstate.MapState
	BlockStore map[string][]byte
}

type mockPinResp struct {
	Pins []string
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
func NewIpfsMock() *IpfsMock {
	st := mapstate.NewMapState()
	blocks := make(map[string][]byte)
	m := &IpfsMock{
		pinMap:     st,
		BlockStore: blocks,
	}
	ts := httptest.NewServer(http.HandlerFunc(m.handler))
	m.server = ts

	url, _ := url.Parse(ts.URL)
	h := strings.Split(url.Host, ":")
	i, _ := strconv.Atoi(h[1])

	m.Port = i
	m.Addr = h[0]
	return m

}

// FIXME: what if IPFS API changes?
func (m *IpfsMock) handler(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	w.Header().Set("Access-Control-Allow-Headers", "test-allow-header")
	w.Header().Set("Server", "ipfs-mock")
	endp := strings.TrimPrefix(p, "/api/v0/")
	switch endp {
	case "id":
		resp := mockIDResp{
			ID: TestPeerID1.Pretty(),
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
		if arg == ErrorCid {
			goto ERROR
		}
		c, err := cid.Decode(arg)
		if err != nil {
			goto ERROR
		}
		m.pinMap.Add(api.PinCid(c))
		resp := mockPinResp{
			Pins: []string{arg},
		}
		j, _ := json.Marshal(resp)
		w.Write(j)
	case "pin/rm":
		arg, ok := extractCid(r.URL)
		if !ok {
			goto ERROR
		}
		c, err := cid.Decode(arg)
		if err != nil {
			goto ERROR
		}
		m.pinMap.Rm(c)
		resp := mockPinResp{
			Pins: []string{arg},
		}
		j, _ := json.Marshal(resp)
		w.Write(j)
	case "pin/ls":
		arg, ok := extractCid(r.URL)
		if !ok {
			rMap := make(map[string]mockPinType)
			pins := m.pinMap.List()
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
		ok = m.pinMap.Has(c)
		if ok {
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
			Peer: TestPeerID4.Pretty(),
		}
		peer2 := mockIpfsPeer{
			Peer: TestPeerID5.Pretty(),
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
		len := len(m.pinMap.List())
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
		w.Write(j)
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
	m.server.Close()
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
