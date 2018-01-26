package test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/state/mapstate"

	cid "github.com/ipfs/go-cid"
)

// IpfsMock is an ipfs daemon mock which should sustain the functionality used by ipfscluster.
type IpfsMock struct {
	server *httptest.Server
	Addr   string
	Port   int
	pinMap *mapstate.MapState
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

type mockSwarmPeersResp struct {
	Peers []mockIpfsPeer
}

type mockIpfsPeer struct {
	Peer string
}

// NewIpfsMock returns a new mock.
func NewIpfsMock() *IpfsMock {
	st := mapstate.NewMapState()
	m := &IpfsMock{
		pinMap: st,
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
	endp := strings.TrimPrefix(p, "/api/v0/")
	var cidStr string
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
	case "add":
		c, _ := cid.Decode(TestCid3)
		// add also pins
		m.pinMap.Add(api.PinCid(c))
		_, fheader, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "no file in /add", 500)
			return
		}

		query := r.URL.Query()
		progress, ok := query["progress"]
		if ok && len(progress) > 0 && progress[0] != "false" {
			progressResp := mockAddResp{
				Name:  fheader.Filename,
				Bytes: 4,
			}
			j, _ := json.Marshal(progressResp)
			w.Write(j)
		}

		resp := mockAddResp{
			Name: fheader.Filename,
			Hash: TestCid3,
		}
		j, _ := json.Marshal(resp)
		w.Write(j)
	case "pin/add":
		query := r.URL.Query()
		arg, ok := query["arg"]
		if !ok || len(arg) != 1 {
			goto ERROR
		}
		cidStr = arg[0]
		if cidStr == ErrorCid {
			goto ERROR
		}
		c, err := cid.Decode(cidStr)
		if err != nil {
			goto ERROR
		}
		m.pinMap.Add(api.PinCid(c))
		resp := mockPinResp{
			Pins: []string{cidStr},
		}
		j, _ := json.Marshal(resp)
		w.Write(j)
	case "pin/rm":
		query := r.URL.Query()
		arg, ok := query["arg"]
		if !ok || len(arg) != 1 {
			goto ERROR
		}
		cidStr = arg[0]
		c, err := cid.Decode(cidStr)
		if err != nil {
			goto ERROR
		}
		m.pinMap.Rm(c)
		resp := mockPinResp{
			Pins: []string{cidStr},
		}
		j, _ := json.Marshal(resp)
		w.Write(j)
	case "pin/ls":
		query := r.URL.Query()
		arg, ok := query["arg"]
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
		if len(arg) != 1 {
			goto ERROR
		}
		cidStr = arg[0]

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
		query := r.URL.Query()
		arg, ok := query["arg"]
		if !ok {
			goto ERROR
		}
		addr := arg[0]
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
	case "repo/stat":
		len := len(m.pinMap.List())
		resp := mockRepoStatResp{
			RepoSize:   uint64(len) * 1000,
			NumObjects: uint64(len),
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
