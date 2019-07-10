// Package rpcutil provides utility methods to perform go-libp2p-gorpc calls,
// particularly gorpc.MultiCall().
package rpcutil

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ipfs/ipfs-cluster/api"

	peer "github.com/libp2p/go-libp2p-core/peer"
)

// CtxsWithTimeout returns n contexts, derived from the given parent
// using the given timeout.
func CtxsWithTimeout(
	parent context.Context,
	n int,
	timeout time.Duration,
) ([]context.Context, []context.CancelFunc) {

	ctxs := make([]context.Context, n, n)
	cancels := make([]context.CancelFunc, n, n)
	for i := 0; i < n; i++ {
		ctx, cancel := context.WithTimeout(parent, timeout)
		ctxs[i] = ctx
		cancels[i] = cancel
	}
	return ctxs, cancels
}

// CtxsWithCancel returns n cancellable contexts, derived from the given parent.
func CtxsWithCancel(
	parent context.Context,
	n int,
) ([]context.Context, []context.CancelFunc) {

	ctxs := make([]context.Context, n, n)
	cancels := make([]context.CancelFunc, n, n)
	for i := 0; i < n; i++ {
		ctx, cancel := context.WithCancel(parent)
		ctxs[i] = ctx
		cancels[i] = cancel
	}
	return ctxs, cancels
}

// MultiCancel calls all the provided CancelFuncs. It
// is useful with "defer Multicancel()"
func MultiCancel(cancels []context.CancelFunc) {
	for _, cancel := range cancels {
		cancel()
	}
}

// The copy functions below are used in calls to Cluster.multiRPC()

// CopyPIDsToIfaces converts a peer.ID slice to an empty interface
// slice using pointers to each elements of the original slice.
// Useful to handle gorpc.MultiCall() replies.
func CopyPIDsToIfaces(in []peer.ID) []interface{} {
	ifaces := make([]interface{}, len(in), len(in))
	for i := range in {
		ifaces[i] = &in[i]
	}
	return ifaces
}

// CopyIDsToIfaces converts an api.ID slice to an empty interface
// slice using pointers to each elements of the original slice.
// Useful to handle gorpc.MultiCall() replies.
func CopyIDsToIfaces(in []*api.ID) []interface{} {
	ifaces := make([]interface{}, len(in), len(in))
	for i := range in {
		in[i] = &api.ID{}
		ifaces[i] = in[i]
	}
	return ifaces
}

// CopyIDSliceToIfaces converts an api.ID slice of slices
// to an empty interface slice using pointers to each elements of the
// original slice. Useful to handle gorpc.MultiCall() replies.
func CopyIDSliceToIfaces(in [][]*api.ID) []interface{} {
	ifaces := make([]interface{}, len(in), len(in))
	for i := range in {
		ifaces[i] = &in[i]
	}
	return ifaces
}

// CopyPinInfoToIfaces converts an api.PinInfo slice to
// an empty interface slice using pointers to each elements of
// the original slice. Useful to handle gorpc.MultiCall() replies.
func CopyPinInfoToIfaces(in []*api.PinInfo) []interface{} {
	ifaces := make([]interface{}, len(in), len(in))
	for i := range in {
		in[i] = &api.PinInfo{}
		ifaces[i] = in[i]
	}
	return ifaces
}

// CopyPinInfoSliceToIfaces converts an api.PinInfo slice of slices
// to an empty interface slice using pointers to each elements of the original
// slice. Useful to handle gorpc.MultiCall() replies.
func CopyPinInfoSliceToIfaces(in [][]*api.PinInfo) []interface{} {
	ifaces := make([]interface{}, len(in), len(in))
	for i := range in {
		ifaces[i] = &in[i]
	}
	return ifaces
}

// CopyRepoGCSliceToIfaces converts an api.RepoGC slice to
// an empty interface slice using pointers to each elements of
// the original slice. Useful to handle gorpc.MultiCall() replies.
func CopyRepoGCSliceToIfaces(in []*api.RepoGC) []interface{} {
	ifaces := make([]interface{}, len(in), len(in))
	for i := range in {
		in[i] = &api.RepoGC{}
		ifaces[i] = in[i]
	}
	return ifaces
}

// CopyEmptyStructToIfaces converts an empty struct slice to an empty interface
// slice using pointers to each elements of the original slice.
// Useful to handle gorpc.MultiCall() replies.
func CopyEmptyStructToIfaces(in []struct{}) []interface{} {
	ifaces := make([]interface{}, len(in), len(in))
	for i := range in {
		ifaces[i] = &in[i]
	}
	return ifaces
}

// RPCDiscardReplies returns a []interface{} slice made from a []struct{}
// slice of then given length. Useful for RPC methods which have no response
// types (so they use empty structs).
func RPCDiscardReplies(n int) []interface{} {
	replies := make([]struct{}, n, n)
	return CopyEmptyStructToIfaces(replies)
}

// CheckErrs returns nil if all the errors in a slice are nil, otherwise
// it returns a single error formed by joining the error messages existing
// in the slice with a line-break.
func CheckErrs(errs []error) error {
	errMsg := ""

	for _, e := range errs {
		if e != nil {
			errMsg += fmt.Sprintf("%s\n", e.Error())
		}
	}

	if len(errMsg) > 0 {
		return errors.New(errMsg)
	}
	return nil
}
