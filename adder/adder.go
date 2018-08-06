package adder

import (
	"context"
	"mime/multipart"

	"github.com/ipfs/ipfs-cluster/api"

	cid "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
)

var logger = logging.Logger("adder")

// Adder represents a module capable of adding content to IPFS Cluster.
type Adder interface {
	// FromMultipart adds from a multipart reader and returns
	// the resulting CID.
	FromMultipart(context.Context, *multipart.Reader, *api.AddParams) (*cid.Cid, error)
}
