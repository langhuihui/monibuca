package m7s

import (
	"context"

	"google.golang.org/protobuf/types/known/emptypb"
	"m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/pb"
)

type StreamSnapShot struct {
	StreamPath string
	*Publisher
}

func (s *Server) StreamSnap(ctx context.Context, req *pb.StreamSnapRequest) (res *pb.StreamSnapShot, err error) {
	snap := &StreamSnapShot{StreamPath: req.StreamPath}
	err = sendPromiseToServer(s, snap)
	if snap.Publisher == nil {
		return nil, err
	}
	return snap.SnapShot(), nil
}

func (s *Server) Restart(ctx context.Context, req *pb.RequestWithId) (res *emptypb.Empty, err error) {
	if Servers[req.Id] != nil {
		Servers[req.Id].Stop(pkg.ErrRestart)
	}
	return &emptypb.Empty{}, err
}

func (s *Server) Shutdown(ctx context.Context, req *pb.RequestWithId) (res *emptypb.Empty, err error) {
	if Servers[req.Id] != nil {
		Servers[req.Id].Stop(pkg.ErrStopFromAPI)
	} else {
		return nil, pkg.ErrNotFound
	}
	return &emptypb.Empty{}, err
}

func (s *Server) StopSubscribe(ctx context.Context, req *pb.StopSubscribeRequest) (res *pb.StopSubscribeResponse, err error) {
	err = sendPromiseToServer(s, req)
	return &pb.StopSubscribeResponse{
		Success: err == nil,
	}, err
}
