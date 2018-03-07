package importer

import (
	"context"
	"errors"
	"fmt"

	"github.com/ipfs/ipfs-cluster/api"

	cid "github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
)

var errUninit = errors.New("DAGService output channel uninitialized")

// outDAGService will "add" a node by sending through the outChan
type outDAGService struct {
	membership map[string]struct{}
	outChan    chan<- *api.NodeWithMeta
}

// Get always returns errNotFound
func (ods *outDAGService) Get(ctx context.Context, key *cid.Cid) (ipld.Node, error) {
	return nil, errNotFound
}

// GetMany returns an output channel that always emits an error
func (ods *outDAGService) GetMany(ctx context.Context, keys []*cid.Cid) <-chan *ipld.NodeOption {
	out := make(chan *ipld.NodeOption, 1)
	out <- &ipld.NodeOption{Err: fmt.Errorf("failed to fetch all nodes")}
	close(out)
	return out
}

// Add passes the provided node through the output channel
func (ods *outDAGService) Add(ctx context.Context, node ipld.Node) error {
	id := node.Cid().String()
	_, ok := ods.membership[id]
	if ok { // already added don't add again
		return nil
	}
	ods.membership[id] = struct{}{}

	// Send node on output channel
	if ods.outChan == nil {
		return errUninit
	}
	size, err := node.Size()
	if err != nil {
		return err
	}
	nodeSerial := api.NodeWithMeta{
		Cid:  id,
		Data: node.RawData(),
		Size: size,
	}
	select {
	case ods.outChan <- &nodeSerial:
		return nil
	case <-ctx.Done():
		close(ods.outChan)
		return errors.New("canceled context preempted dagservice add")
	}
}

// AddMany passes the provided nodes through the output channel
func (ods *outDAGService) AddMany(ctx context.Context, nodes []ipld.Node) error {
	for _, node := range nodes {
		err := ods.Add(ctx, node)
		if err != nil {
			return err
		}
	}
	return nil
}

// Remove is a nop
func (ods *outDAGService) Remove(ctx context.Context, key *cid.Cid) error {
	return nil
}

// RemoveMany is a nop
func (ods *outDAGService) RemoveMany(ctx context.Context, keys []*cid.Cid) error {
	return nil
}
