package ipfscluster

import (
	"context"
	"errors"
	"sync"

	host "github.com/libp2p/go-libp2p-host"
	inet "github.com/libp2p/go-libp2p-net"
	peer "github.com/libp2p/go-libp2p-peer"
)

//ClusterP2PProtocol is used to send libp2p messages between cluster members
const ClusterP2PProtocol = "/ipfscluster/0.0.1/rpc"

// Remote is a Cluster component used to handle communication with remote
// Cluster nodes
type Libp2pRemote struct {
	ctx context.Context

	host host.Host

	rpcCh chan RPC

	shutdownLock sync.Mutex
	shutdown     bool
	shutdownCh   chan struct{}
	wg           sync.WaitGroup
}

func NewLibp2pRemote() *Libp2pRemote {
	ctx := context.Background()

	r := &Libp2pRemote{
		ctx:        ctx,
		rpcCh:      make(chan RPC),
		shutdownCh: make(chan struct{}),
	}

	return r
}

func (r *Libp2pRemote) SetHost(h host.Host) {
	r.host = h
	r.host.SetStreamHandler(ClusterP2PProtocol, func(s inet.Stream) {
		sWrap := wrapStream(s)
		defer s.Close()
		err := r.handleRemoteRPC(sWrap)
		if err != nil {
			logger.Error("error handling remote RPC:", err)
		}
	})
}

func (r *Libp2pRemote) Shutdown() error {
	r.shutdownLock.Lock()
	defer r.shutdownLock.Unlock()
	if r.shutdown {
		logger.Debug("already shutdown")
		return nil
	}
	logger.Info("shutting down Remote component")
	//r.shutdownCh <- struct{}{}
	r.shutdown = true
	//<-r.shutdownCh
	return nil
}

func (r *Libp2pRemote) RpcChan() <-chan RPC {
	return r.rpcCh
}

func (r *Libp2pRemote) handleRemoteRPC(s *streamWrap) error {
	logger.Debugf("%s: handling remote RPC", r.host.ID().Pretty())
	rpcType, err := s.r.ReadByte()
	if err != nil {
		return err
	}

	logger.Debugf("RPC type is %d", rpcType)
	rpc, err := r.decodeRPC(s, int(rpcType))
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(r.ctx)
	defer cancel()
	resp := MakeRPC(ctx, r.rpcCh, rpc, true)
	return r.sendStreamResponse(s, resp)
}

func (r *Libp2pRemote) decodeRPC(s *streamWrap, rpcType int) (RPC, error) {
	var err error
	switch RPCOpToType[rpcType] {
	case CidRPCType:
		var rpc *CidRPC
		err = s.dec.Decode(&rpc)
		if err != nil {
			goto DECODE_ERROR
		}
		logger.Debugf("%+v", rpc)
		return rpc, nil
	case GenericRPCType:
		var rpc *GenericRPC
		err = s.dec.Decode(&rpc)
		if err != nil {
			goto DECODE_ERROR
		}
		logger.Debugf("%+v", rpc)
		return rpc, nil
	default:
		err = errors.New("Unknown or unsupported RPCType when trying to decode")
		goto DECODE_ERROR
	}

DECODE_ERROR:
	logger.Error("error decoding RPC request from Stream:", err)
	errResp := RPCResponse{
		Data:  nil,
		Error: errors.New("error decoding request"),
	}
	r.sendStreamResponse(s, errResp)
	return nil, err
}

func (r *Libp2pRemote) sendStreamResponse(s *streamWrap, resp RPCResponse) error {
	if err := s.enc.Encode(resp); err != nil {
		logger.Error("error encoding response:", err)
		return err
	}
	if err := s.w.Flush(); err != nil {
		logger.Error("error flushing response:", err)
		return err
	}
	return nil
}

func (r *Libp2pRemote) MakeRemoteRPC(rpc RPC, node peer.ID) (RPCResponse, error) {
	ctx, cancel := context.WithCancel(r.ctx)
	defer cancel()
	var resp RPCResponse

	if r.host == nil {
		return resp, errors.New("no host set")
	}

	if node == r.host.ID() {
		// libp2p cannot dial itself
		return MakeRPC(ctx, r.rpcCh, rpc, true), nil
	}

	s, err := r.host.NewStream(ctx, node, ClusterP2PProtocol)
	if err != nil {
		return resp, err
	}
	defer s.Close()
	sWrap := wrapStream(s)

	logger.Debugf("sending remote RPC %d to %s", rpc.Op(), node)
	if err := sWrap.w.WriteByte(byte(rpc.Op())); err != nil {
		return resp, err
	}

	if err := sWrap.enc.Encode(rpc); err != nil {
		return resp, err
	}

	if err := sWrap.w.Flush(); err != nil {
		return resp, err
	}

	logger.Debug("Waiting for response from %s", node)
	if err := sWrap.dec.Decode(&resp); err != nil {
		return resp, err
	}
	return resp, nil
}
