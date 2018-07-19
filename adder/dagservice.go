package adder

import (
	"context"
	"errors"
	"fmt"

	"github.com/ipfs/ipfs-cluster/api"

	cid "github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
)

var errNotFound = errors.New("dagservice: block not found")

func isNotFound(err error) bool {
	return err == errNotFound
}

// adderDAGService implemengs a DAG Service and
// outputs any nodes added using this service to an Added.
type adderDAGService struct {
	addedSet  *cid.Set
	addedChan chan<- *api.NodeWithMeta
}

func newAdderDAGService(ch chan *api.NodeWithMeta) ipld.DAGService {
	set := cid.NewSet()

	return &adderDAGService{
		addedSet:  set,
		addedChan: ch,
	}
}

// Add passes the provided node through the output channel
func (dag *adderDAGService) Add(ctx context.Context, node ipld.Node) error {
	// FIXME ? This set will grow in memory.
	// Maybe better to use a bloom filter
	ok := dag.addedSet.Visit(node.Cid())
	if !ok {
		// don't re-add
		return nil
	}

	size, err := node.Size()
	if err != nil {
		return err
	}
	nodeSerial := api.NodeWithMeta{
		Cid:     node.Cid().String(),
		Data:    node.RawData(),
		CumSize: size,
	}

	select {
	case dag.addedChan <- &nodeSerial:
		return nil
	case <-ctx.Done():
		close(dag.addedChan)
		return errors.New("canceled context preempted dagservice add")
	}
}

// AddMany passes the provided nodes through the output channel
func (dag *adderDAGService) AddMany(ctx context.Context, nodes []ipld.Node) error {
	for _, node := range nodes {
		err := dag.Add(ctx, node)
		if err != nil {
			return err
		}
	}
	return nil
}

// Get always returns errNotFound
func (dag *adderDAGService) Get(ctx context.Context, key *cid.Cid) (ipld.Node, error) {
	return nil, errNotFound
}

// GetMany returns an output channel that always emits an error
func (dag *adderDAGService) GetMany(ctx context.Context, keys []*cid.Cid) <-chan *ipld.NodeOption {
	out := make(chan *ipld.NodeOption, 1)
	out <- &ipld.NodeOption{Err: fmt.Errorf("failed to fetch all nodes")}
	close(out)
	return out
}

// Remove is a nop
func (dag *adderDAGService) Remove(ctx context.Context, key *cid.Cid) error {
	return nil
}

// RemoveMany is a nop
func (dag *adderDAGService) RemoveMany(ctx context.Context, keys []*cid.Cid) error {
	return nil
}

// // printDAGService will "add" a node by printing it.  printDAGService cannot Get nodes
// // that have already been seen and calls to Remove are noops.  Nodes are
// // recorded after being added so that they will only be printed once.
// type printDAGService struct {
// 	ads ipld.DAGService
// }

// func newPDagService() *printDAGService {
// 	ch := make(chan *api.NodeWithMeta)
// 	ads := newAdderDAGService(ch)

// 	go func() {
// 		for n := range ch {
// 			fmt.Printf(n.Cid, " | ", n.Size)
// 		}
// 	}()

// 	return &printDAGService{
// 		ads: ads,
// 	}
// }
