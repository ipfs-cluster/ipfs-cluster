package importer

import (
	"context"
	"errors"
	"fmt"

	cid "github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
)

var errNotFound = errors.New("dagservice: block not found")

// pDAGService will "add" a node by printing it.  pDAGService cannot Get nodes
// that have already been seen and calls to Remove are noops.  Nodes are
// recorded after being added so that they will only be printed once
type pDAGService struct {
	membership map[string]bool
}

func (pds *pDAGService) Get(ctx context.Context, key *cid.Cid) (ipld.Node, error) {
	return nil, errNotFound
}

func (pds *pDAGService) GetMany(ctx context.Context, keys []*cid.Cid) <-chan *ipld.NodeOption {
	out := make(chan *ipld.NodeOption, len(keys))
	go func() {
		out <- &ipld.NodeOption{Err: fmt.Errorf("failed to fetch all nodes")}
		return
	}()
	return out
}

func (pds *pDAGService) Add(ctx context.Context, node ipld.Node) error {
	id := node.Cid().String()
	_, ok := pds.membership[id]
	if ok { // already added don't add again
		return nil
	}
	pds.membership[id] = true
	fmt.Printf("%s\n", node.String())
	return nil
}

func (pds *pDAGService) AddMany(ctx context.Context, nodes []ipld.Node) error {
	for _, node := range nodes {
		err := pds.Add(ctx, node)
		if err != nil {
			return err
		}
	}
	return nil
}

// Remove is a nop
func (pds *pDAGService) Remove(ctx context.Context, key *cid.Cid) error {
	return nil
}

// RemoveMany is a nop
func (pds *pDAGService) RemoveMany(ctx context.Context, keys []*cid.Cid) error {
	return nil
}
