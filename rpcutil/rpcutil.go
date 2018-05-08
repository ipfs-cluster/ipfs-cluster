// Package rpcutil provides utility methods to perform go-libp2p-gorpc calls,
// particularly gorpc.MultiCall().
package rpcutil

import (
	"context"
	"time"

	"github.com/ipfs/ipfs-cluster/api"

	peer "github.com/libp2p/go-libp2p-peer"
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

// The copy functions below are used in calls to Cluste.multiRPC()

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

// CopyIDSerialsToIfaces converts an api.IDSerial slice to an empty interface
// slice using pointers to each elements of the original slice.
// Useful to handle gorpc.MultiCall() replies.
func CopyIDSerialsToIfaces(in []api.IDSerial) []interface{} {
	ifaces := make([]interface{}, len(in), len(in))
	for i := range in {
		ifaces[i] = &in[i]
	}
	return ifaces
}

// CopyIDSerialSliceToIfaces converts an api.IDSerial slice of slices
// to an empty interface slice using pointers to each elements of the
// original slice. Useful to handle gorpc.MultiCall() replies.
func CopyIDSerialSliceToIfaces(in [][]api.IDSerial) []interface{} {
	ifaces := make([]interface{}, len(in), len(in))
	for i := range in {
		ifaces[i] = &in[i]
	}
	return ifaces
}

// CopyPinInfoSerialToIfaces converts an api.PinInfoSerial slice to
// an empty interface slice using pointers to each elements of
// the original slice. Useful to handle gorpc.MultiCall() replies.
func CopyPinInfoSerialToIfaces(in []api.PinInfoSerial) []interface{} {
	ifaces := make([]interface{}, len(in), len(in))
	for i := range in {
		ifaces[i] = &in[i]
	}
	return ifaces
}

// CopyPinInfoSerialSliceToIfaces converts an api.PinInfoSerial slice of slices
// to an empty interface slice using pointers to each elements of the original
// slice. Useful to handle gorpc.MultiCall() replies.
func CopyPinInfoSerialSliceToIfaces(in [][]api.PinInfoSerial) []interface{} {
	ifaces := make([]interface{}, len(in), len(in))
	for i := range in {
		ifaces[i] = &in[i]
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
