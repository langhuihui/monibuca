package m7s

import (
	"context"

	"google.golang.org/protobuf/types/known/emptypb"
	"m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pb"
)

func (s *Server) StreamSnap(ctx context.Context, req *pb.StreamSnapRequest) (res *pb.StreamSnapShot, err error) {
	result, err := s.Call(req)
	if err != nil {
		return nil, err
	}
	puber := result.(*Publisher)
	return puber.SnapShot(), nil
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
	_, err = s.Call(req)
	return &pb.StopSubscribeResponse{
		Success: err == nil,
	}, err
}


func (s *Server) StreamList(ctx context.Context, req *pb.StreamListRequest) (res *pb.StreamListResponse, err error) {
	var result any
	result, err = s.Call(req)
	return result.(*pb.StreamListResponse), err
}