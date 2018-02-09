package importer

import (
	"context"
	"errors"
	"fmt"

	cid "github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
)

var errUninit = errors.New("DAGService output channel uninitialized")

// outDAGService will "add" a node by printing it.  pDAGService cannot Get nodes
// that have already been seen and calls to Remove are noops.  Nodes are
// recorded after being added so that they will only be printed once
type outDAGService struct {
	membership map[string]bool
	outChan    chan<- *ipld.Node
}

func (ods *outDAGService) Get(ctx context.Context, key *cid.Cid) (ipld.Node, error) {
	return nil, errNotFound
}

func (ods *outDAGService) GetMany(ctx context.Context, keys []*cid.Cid) <-chan *ipld.NodeOption {
	out := make(chan *ipld.NodeOption, len(keys))
	go func() {
		out <- &ipld.NodeOption{Err: fmt.Errorf("failed to fetch all nodes")}
		return
	}()
	return out
}

func (ods *outDAGService) Add(ctx context.Context, node ipld.Node) error {
	id := node.Cid().String()
	_, ok := ods.membership[id]
	if ok { // already added don't add again
		return nil
	}
	ods.membership[id] = true

	// Send node on output channel
	if ods.outChan == nil {
		return errUninit
	}
	select {
	case ods.outChan <- &node:
		return nil
	case <-ctx.Done():
		close(ods.outChan)
		return errors.New("canceled context preempted dagservice add")
	}
}

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
