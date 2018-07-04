package adder

import (
	"context"
	"mime/multipart"

	logging "github.com/ipfs/go-log"
)

var logger = logging.Logger("adder")

type Adder interface {
	FromMultipart(context.Context, *multipart.Reader, *Params) error
}
