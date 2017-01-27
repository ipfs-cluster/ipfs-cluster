package ipfscluster

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"

	cid "github.com/ipfs/go-cid"
)

// This is an ipfs daemon mock which should sustain the functionality used by
// ipfscluster.

type ipfsMock struct {
	server *httptest.Server
	addr   string
	port   int
	pinMap *MapState
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

func newIpfsMock() *ipfsMock {
	st := NewMapState()
	m := &ipfsMock{
		pinMap: st,
	}
	ts := httptest.NewServer(http.HandlerFunc(m.handler))
	m.server = ts

	url, _ := url.Parse(ts.URL)
	h := strings.Split(url.Host, ":")
	i, _ := strconv.Atoi(h[1])

	m.port = i
	m.addr = h[0]
	return m

}

// FIXME: what if IPFS API changes?
func (m *ipfsMock) handler(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	endp := strings.TrimPrefix(p, "/api/v0/")
	var cidStr string
	switch endp {
	case "id":
		resp := ipfsIDResp{
			ID: testPeerID.Pretty(),
			Addresses: []string{
				"/ip4/0.0.0.0/tcp/1234",
			},
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
		if cidStr == errorCid {
			goto ERROR
		}
		c, err := cid.Decode(cidStr)
		if err != nil {
			goto ERROR
		}
		m.pinMap.AddPin(c)
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
		m.pinMap.RmPin(c)
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
			pins := m.pinMap.ListPins()
			for _, p := range pins {
				rMap[p.String()] = mockPinType{"recursive"}
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
		ok = m.pinMap.HasPin(c)
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
	case "version":
		w.Write([]byte("{\"Version\":\"m.o.c.k\"}"))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
	return
ERROR:
	w.WriteHeader(http.StatusInternalServerError)
}

func (m *ipfsMock) Close() {
	m.server.Close()
}
