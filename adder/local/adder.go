// Package local implements an ipfs-cluster Adder that chunks and adds content
// to a local peer, before pinning it.
package local

import (
	"context"
	"mime/multipart"

	"github.com/ipfs/ipfs-cluster/adder"
	"github.com/ipfs/ipfs-cluster/api"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	"github.com/ipfs/go-ipfs-cmdkit/files"
	logging "github.com/ipfs/go-log"
)

var logger = logging.Logger("addlocal")

type Adder struct {
	rpcClient *rpc.Client
}

func New(rpc *rpc.Client) *Adder {
	return &Adder{
		rpcClient: rpc,
	}
}

func (a *Adder) FromMultipart(ctx context.Context, r *multipart.Reader, p *adder.Params) error {
	f := &files.MultipartFile{
		Mediatype: "multipart/form-data",
		Reader:    r,
	}

	// TODO: it should send it to the best allocation
	localBlockPut := func(ctx context.Context, n *api.NodeWithMeta) (string, error) {
		retVal := n.Cid
		err := a.rpcClient.CallContext(
			ctx,
			"",
			"Cluster",
			"IPFSBlockPut",
			*n,
			&struct{}{},
		)
		return retVal, err
	}

	importer, err := adder.NewImporter(f, p)
	if err != nil {
		return err
	}

	lastCid, err := importer.Run(ctx, localBlockPut)
	if err != nil {
		return err
	}

	if lastCid == "" {
		panic("nothing imported")
	}

	// Finally, cluster pin the result
	pinS := api.PinSerial{
		Cid:      lastCid,
		Type:     int(api.DataType),
		MaxDepth: -1,
		PinOptions: api.PinOptions{
			ReplicationFactorMin: p.ReplicationFactorMin,
			ReplicationFactorMax: p.ReplicationFactorMax,
			Name:                 p.Name,
		},
	}
	return a.rpcClient.Call(
		"",
		"Cluster",
		"Pin",
		pinS,
		&struct{}{},
	)
}
