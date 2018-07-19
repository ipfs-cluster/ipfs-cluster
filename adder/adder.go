package adder

import (
	"context"
	"mime/multipart"

	cid "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
)

var logger = logging.Logger("adder")

type Adder interface {
	// FromMultipart adds from a multipart reader and returns
	// the resulting CID.
	FromMultipart(context.Context, *multipart.Reader, *Params) (*cid.Cid, error)
}
