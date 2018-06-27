package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"go.opencensus.io/trace"

	"github.com/ipfs/ipfs-cluster/api"

	cid "github.com/ipfs/go-cid"
	files "github.com/ipfs/go-ipfs-files"
	peer "github.com/libp2p/go-libp2p-peer"
)

// ID returns information about the cluster Peer.
func (c *defaultClient) ID(ctx context.Context) (api.ID, error) {
	ctx, span := trace.StartSpan(ctx, "client/ID")
	defer span.End()

	var id api.IDSerial
	err := c.do(ctx, "GET", "/id", nil, nil, &id)
	return id.ToID(), err
}

// Peers requests ID information for all cluster peers.
func (c *defaultClient) Peers(ctx context.Context) ([]api.ID, error) {
	ctx, span := trace.StartSpan(ctx, "client/Peers")
	defer span.End()

	var ids []api.IDSerial
	err := c.do(ctx, "GET", "/peers", nil, nil, &ids)
	result := make([]api.ID, len(ids))
	for i, id := range ids {
		result[i] = id.ToID()
	}
	return result, err
}

type peerAddBody struct {
	PeerID string `json:"peer_id"`
}

// PeerAdd adds a new peer to the cluster.
func (c *defaultClient) PeerAdd(ctx context.Context, pid peer.ID) (api.ID, error) {
	ctx, span := trace.StartSpan(ctx, "client/PeerAdd")
	defer span.End()

	pidStr := peer.IDB58Encode(pid)
	body := peerAddBody{pidStr}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.Encode(body)

	var id api.IDSerial
	err := c.do(ctx, "POST", "/peers", nil, &buf, &id)
	return id.ToID(), err
}

// PeerRm removes a current peer from the cluster
func (c *defaultClient) PeerRm(ctx context.Context, id peer.ID) error {
	ctx, span := trace.StartSpan(ctx, "client/PeerRm")
	defer span.End()

	return c.do(ctx, "DELETE", fmt.Sprintf("/peers/%s", id.Pretty()), nil, nil, nil)
}

// Pin tracks a Cid with the given replication factor and a name for
// human-friendliness.
func (c *defaultClient) Pin(ctx context.Context, ci cid.Cid, replicationFactorMin, replicationFactorMax int, name string) error {
	ctx, span := trace.StartSpan(ctx, "client/Pin")
	defer span.End()

	escName := url.QueryEscape(name)
	err := c.do(
		ctx,
		"POST",
		fmt.Sprintf(
			"/pins/%s?replication-min=%d&replication-max=%d&name=%s",
			ci.String(),
			replicationFactorMin,
			replicationFactorMax,
			escName,
		),
		nil,
		nil,
		nil,
	)
	return err
}

// Unpin untracks a Cid from cluster.
func (c *defaultClient) Unpin(ctx context.Context, ci cid.Cid) error {
	ctx, span := trace.StartSpan(ctx, "client/Unpin")
	defer span.End()
	return c.do(ctx, "DELETE", fmt.Sprintf("/pins/%s", ci.String()), nil, nil, nil)
}

// Allocations returns the consensus state listing all tracked items and
// the peers that should be pinning them.
func (c *defaultClient) Allocations(ctx context.Context, filter api.PinType) ([]api.Pin, error) {
	ctx, span := trace.StartSpan(ctx, "client/Allocations")
	defer span.End()

	var pins []api.PinSerial

	types := []api.PinType{
		api.DataType,
		api.MetaType,
		api.ClusterDAGType,
		api.ShardType,
	}

	var strFilter []string

	if filter == api.AllType {
		strFilter = []string{"all"}
	} else {
		for _, t := range types {
			if t&filter > 0 { // the filter includes this type
				strFilter = append(strFilter, t.String())
			}
		}
	}

	f := url.QueryEscape(strings.Join(strFilter, ","))
	err := c.do(ctx, "GET", fmt.Sprintf("/allocations?filter=%s", f), nil, nil, &pins)
	result := make([]api.Pin, len(pins))
	for i, p := range pins {
		result[i] = p.ToPin()
	}
	return result, err
}

// Allocation returns the current allocations for a given Cid.
func (c *defaultClient) Allocation(ctx context.Context, ci cid.Cid) (api.Pin, error) {
	ctx, span := trace.StartSpan(ctx, "client/Allocation")
	defer span.End()

	var pin api.PinSerial
	err := c.do(ctx, "GET", fmt.Sprintf("/allocations/%s", ci.String()), nil, nil, &pin)
	return pin.ToPin(), err
}

// Status returns the current ipfs state for a given Cid. If local is true,
// the information affects only the current peer, otherwise the information
// is fetched from all cluster peers.
func (c *defaultClient) Status(ctx context.Context, ci cid.Cid, local bool) (api.GlobalPinInfo, error) {
	ctx, span := trace.StartSpan(ctx, "client/Status")
	defer span.End()

	var gpi api.GlobalPinInfoSerial
	err := c.do(ctx, "GET", fmt.Sprintf("/pins/%s?local=%t", ci.String(), local), nil, nil, &gpi)
	return gpi.ToGlobalPinInfo(), err
}

// StatusAll gathers Status() for all tracked items. If a filter is
// provided, only entries matching the given filter statuses
// will be returned. A filter can be built by merging TrackerStatuses with
// a bitwise OR operation (st1 | st2 | ...). A "0" filter value (or
// api.TrackerStatusUndefined), means all.
func (c *defaultClient) StatusAll(ctx context.Context, filter api.TrackerStatus, local bool) ([]api.GlobalPinInfo, error) {
	ctx, span := trace.StartSpan(ctx, "client/StatusAll")
	defer span.End()

	var gpis []api.GlobalPinInfoSerial

	filterStr := ""
	if filter != api.TrackerStatusUndefined { // undefined filter means "all"
		filterStr = filter.String()
		if filterStr == "" {
			return nil, errors.New("invalid filter value")
		}
	}

	err := c.do(ctx, "GET", fmt.Sprintf("/pins?local=%t&filter=%s", local, url.QueryEscape(filterStr)), nil, nil, &gpis)
	result := make([]api.GlobalPinInfo, len(gpis))
	for i, p := range gpis {
		result[i] = p.ToGlobalPinInfo()
	}
	return result, err
}

// Sync makes sure the state of a Cid corresponds to the state reported by
// the ipfs daemon, and returns it. If local is true, this operation only
// happens on the current peer, otherwise it happens on every cluster peer.
func (c *defaultClient) Sync(ctx context.Context, ci cid.Cid, local bool) (api.GlobalPinInfo, error) {
	ctx, span := trace.StartSpan(ctx, "client/Sync")
	defer span.End()

	var gpi api.GlobalPinInfoSerial
	err := c.do(ctx, "POST", fmt.Sprintf("/pins/%s/sync?local=%t", ci.String(), local), nil, nil, &gpi)
	return gpi.ToGlobalPinInfo(), err
}

// SyncAll triggers Sync() operations for all tracked items. It only returns
// informations for items that were de-synced or have an error state. If
// local is true, the operation is limited to the current peer. Otherwise
// it happens on every cluster peer.
func (c *defaultClient) SyncAll(ctx context.Context, local bool) ([]api.GlobalPinInfo, error) {
	ctx, span := trace.StartSpan(ctx, "client/SyncAll")
	defer span.End()

	var gpis []api.GlobalPinInfoSerial
	err := c.do(ctx, "POST", fmt.Sprintf("/pins/sync?local=%t", local), nil, nil, &gpis)
	result := make([]api.GlobalPinInfo, len(gpis))
	for i, p := range gpis {
		result[i] = p.ToGlobalPinInfo()
	}
	return result, err
}

// Recover retriggers pin or unpin ipfs operations for a Cid in error state.
// If local is true, the operation is limited to the current peer, otherwise
// it happens on every cluster peer.
func (c *defaultClient) Recover(ctx context.Context, ci cid.Cid, local bool) (api.GlobalPinInfo, error) {
	ctx, span := trace.StartSpan(ctx, "client/Recover")
	defer span.End()

	var gpi api.GlobalPinInfoSerial
	err := c.do(ctx, "POST", fmt.Sprintf("/pins/%s/recover?local=%t", ci.String(), local), nil, nil, &gpi)
	return gpi.ToGlobalPinInfo(), err
}

// RecoverAll triggers Recover() operations on all tracked items. If local is
// true, the operation is limited to the current peer. Otherwise, it happens
// everywhere.
func (c *defaultClient) RecoverAll(ctx context.Context, local bool) ([]api.GlobalPinInfo, error) {
	ctx, span := trace.StartSpan(ctx, "client/RecoverAll")
	defer span.End()

	var gpis []api.GlobalPinInfoSerial
	err := c.do(ctx, "POST", fmt.Sprintf("/pins/recover?local=%t", local), nil, nil, &gpis)
	result := make([]api.GlobalPinInfo, len(gpis))
	for i, p := range gpis {
		result[i] = p.ToGlobalPinInfo()
	}
	return result, err
}

// Version returns the ipfs-cluster peer's version.
func (c *defaultClient) Version(ctx context.Context) (api.Version, error) {
	ctx, span := trace.StartSpan(ctx, "client/Version")
	defer span.End()

	var ver api.Version
	err := c.do(ctx, "GET", "/version", nil, nil, &ver)
	return ver, err
}

// GetConnectGraph returns an ipfs-cluster connection graph.
// The serialized version, strings instead of pids, is returned
func (c *defaultClient) GetConnectGraph(ctx context.Context) (api.ConnectGraphSerial, error) {
	ctx, span := trace.StartSpan(ctx, "client/GetConnectGraph")
	defer span.End()

	var graphS api.ConnectGraphSerial
	err := c.do(ctx, "GET", "/health/graph", nil, nil, &graphS)
	return graphS, err
}

// Metrics returns a map with the latest valid metrics of the given name
// for the current cluster peers.
func (c *defaultClient) Metrics(ctx context.Context, name string) ([]api.Metric, error) {
	ctx, span := trace.StartSpan(ctx, "client/Metrics")
	defer span.End()

	if name == "" {
		return nil, errors.New("bad metric name")
	}
	var metrics []api.Metric
	err := c.do(ctx, "GET", fmt.Sprintf("/monitor/metrics/%s", name), nil, nil, &metrics)
	return metrics, err
}

// WaitFor is a utility function that allows for a caller to wait for a
// paticular status for a CID (as defined by StatusFilterParams).
// It returns the final status for that CID and an error, if there was.
//
// WaitFor works by calling Status() repeatedly and checking that all
// peers have transitioned to the target TrackerStatus or are Remote.
// If an error of some type happens, WaitFor returns immediately with an
// empty GlobalPinInfo.
func WaitFor(ctx context.Context, c Client, fp StatusFilterParams) (api.GlobalPinInfo, error) {
	ctx, span := trace.StartSpan(ctx, "client/WaitFor")
	defer span.End()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sf := newStatusFilter()

	go sf.pollStatus(ctx, c, fp)
	go sf.filter(ctx, fp)

	var status api.GlobalPinInfo

	for {
		select {
		case <-ctx.Done():
			return api.GlobalPinInfo{}, ctx.Err()
		case err := <-sf.Err:
			return api.GlobalPinInfo{}, err
		case st, ok := <-sf.Out:
			if !ok { // channel closed
				return status, nil
			}
			status = st
		}
	}
}

// StatusFilterParams contains the parameters required
// to filter a stream of status results.
type StatusFilterParams struct {
	Cid       cid.Cid
	Local     bool
	Target    api.TrackerStatus
	CheckFreq time.Duration
}

type statusFilter struct {
	In, Out chan api.GlobalPinInfo
	Done    chan struct{}
	Err     chan error
}

func newStatusFilter() *statusFilter {
	return &statusFilter{
		In:   make(chan api.GlobalPinInfo),
		Out:  make(chan api.GlobalPinInfo),
		Done: make(chan struct{}),
		Err:  make(chan error),
	}
}

func (sf *statusFilter) filter(ctx context.Context, fp StatusFilterParams) {
	defer close(sf.Done)
	defer close(sf.Out)

	for {
		select {
		case <-ctx.Done():
			sf.Err <- ctx.Err()
			return
		case gblPinInfo, more := <-sf.In:
			if !more {
				return
			}
			ok, err := statusReached(fp.Target, gblPinInfo)
			if err != nil {
				sf.Err <- err
				return
			}

			sf.Out <- gblPinInfo
			if !ok {
				continue
			}
			return
		}
	}
}

func (sf *statusFilter) pollStatus(ctx context.Context, c Client, fp StatusFilterParams) {
	ticker := time.NewTicker(fp.CheckFreq)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			sf.Err <- ctx.Err()
			return
		case <-ticker.C:
			gblPinInfo, err := c.Status(ctx, fp.Cid, fp.Local)
			if err != nil {
				sf.Err <- err
				return
			}
			logger.Debugf("pollStatus: status: %#v", gblPinInfo)
			sf.In <- gblPinInfo
		case <-sf.Done:
			close(sf.In)
			return
		}
	}
}

func statusReached(target api.TrackerStatus, gblPinInfo api.GlobalPinInfo) (bool, error) {
	for _, pinInfo := range gblPinInfo.PeerMap {
		switch pinInfo.Status {
		case target:
			continue
		case api.TrackerStatusUndefined, api.TrackerStatusClusterError, api.TrackerStatusPinError, api.TrackerStatusUnpinError:
			return false, fmt.Errorf("error has occurred while attempting to reach status: %s", target.String())
		case api.TrackerStatusRemote:
			if target == api.TrackerStatusPinned {
				continue // to next pinInfo
			}
			return false, nil
		default:
			return false, nil
		}
	}
	return true, nil
}

// logic drawn from go-ipfs-cmds/cli/parse.go: appendFile
func makeSerialFile(fpath string, params *api.AddParams) (files.Node, error) {
	if fpath == "." {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		cwd, err = filepath.EvalSymlinks(cwd)
		if err != nil {
			return nil, err
		}
		fpath = cwd
	}

	fpath = filepath.ToSlash(filepath.Clean(fpath))

	stat, err := os.Lstat(fpath)
	if err != nil {
		return nil, err
	}

	if stat.IsDir() {
		if !params.Recursive {
			return nil, fmt.Errorf("%s is a directory, but Recursive option is not set", fpath)
		}
	}

	return files.NewSerialFile(fpath, params.Hidden, stat)
}

// Add imports files to the cluster from the given paths. A path can
// either be a local filesystem location or an web url (http:// or https://).
// In the latter case, the destination will be downloaded with a GET request.
// The AddParams allow to control different options, like enabling the
// sharding the resulting DAG across the IPFS daemons of multiple cluster
// peers. The output channel will receive regular updates as the adding
// process progresses.
func (c *defaultClient) Add(
	ctx context.Context,
	paths []string,
	params *api.AddParams,
	out chan<- *api.AddedOutput,
) error {
	ctx, span := trace.StartSpan(ctx, "client/Add")
	defer span.End()

	addFiles := make([]files.DirEntry, len(paths), len(paths))
	for i, p := range paths {
		u, err := url.Parse(p)
		if err != nil {
			close(out)
			return fmt.Errorf("error parsing path: %s", err)
		}
		name := path.Base(p)
		var addFile files.Node
		if strings.HasPrefix(u.Scheme, "http") {
			addFile = files.NewWebFile(u)
			name = path.Base(u.Path)
		} else {
			addFile, err = makeSerialFile(p, params)
			if err != nil {
				close(out)
				return err
			}
		}
		addFiles[i] = files.FileEntry(name, addFile)
	}

	sliceFile := files.NewSliceDirectory(addFiles)
	// If `form` is set to true, the multipart data will have
	// a Content-Type of 'multipart/form-data', if `form` is false,
	// the Content-Type will be 'multipart/mixed'.
	return c.AddMultiFile(ctx, files.NewMultiFileReader(sliceFile, true), params, out)
}

// AddMultiFile imports new files from a MultiFileReader. See Add().
func (c *defaultClient) AddMultiFile(
	ctx context.Context,
	multiFileR *files.MultiFileReader,
	params *api.AddParams,
	out chan<- *api.AddedOutput,
) error {
	ctx, span := trace.StartSpan(ctx, "client/AddMultiFile")
	defer span.End()

	defer close(out)

	headers := make(map[string]string)
	headers["Content-Type"] = "multipart/form-data; boundary=" + multiFileR.Boundary()

	// This method must run with StreamChannels set.
	params.StreamChannels = true
	queryStr := params.ToQueryString()

	// our handler decodes an AddedOutput and puts it
	// in the out channel.
	handler := func(dec *json.Decoder) error {
		if out == nil {
			return nil
		}
		var obj api.AddedOutput
		err := dec.Decode(&obj)
		if err != nil {
			return err
		}
		out <- &obj
		return nil
	}

	err := c.doStream(ctx,
		"POST",
		"/add?"+queryStr,
		headers,
		multiFileR,
		handler,
	)
	return err
}
