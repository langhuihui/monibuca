package m7s

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	. "github.com/shirou/gopsutil/v3/net"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"m7s.live/m7s/v5/pb"
	"m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/util"
)

var localIP string

func (s *Server) SysInfo(context.Context, *emptypb.Empty) (res *pb.SysInfoResponse, err error) {
	if localIP == "" {
		if conn, err := net.Dial("udp", "114.114.114.114:80"); err == nil {
			localIP, _, _ = strings.Cut(conn.LocalAddr().String(), ":")
		}
	}
	res = &pb.SysInfoResponse{
		Version:   Version,
		LocalIP:   localIP,
		StartTime: timestamppb.New(s.StartTime),
	}
	return
}

func (s *Server) StreamSnap(ctx context.Context, req *pb.StreamSnapRequest) (res *pb.StreamSnapShot, err error) {
	s.Call(func() {
		if pub, ok := s.Streams.Get(req.StreamPath); ok {
			res = pub.SnapShot()
		} else {
			err = pkg.ErrNotFound
		}
	})
	return
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
	s.Call(func() {
		if subscriber, ok := s.Subscribers.Get(int(req.Id)); ok {
			subscriber.Stop(errors.New("stop by api"))
		} else {
			err = pkg.ErrNotFound
		}
	})
	return &pb.StopSubscribeResponse{
		Success: err == nil,
	}, err
}

func (s *Server) StreamList(_ context.Context, req *pb.StreamListRequest) (res *pb.StreamListResponse, err error) {
	s.Call(func() {
		var streams []*pb.StreamSummay
		for _, publisher := range s.Streams.Items {
			streams = append(streams, &pb.StreamSummay{
				Path: publisher.StreamPath,
			})
		}
		res = &pb.StreamListResponse{List: streams, Total: int32(s.Streams.Length), PageNum: req.PageNum, PageSize: req.PageSize}
	})
	return
}

func (s *Server) API_Summary_SSE(rw http.ResponseWriter, r *http.Request) {
	util.ReturnFetchValue(func() *pb.SummaryResponse {
		ret, _ := s.Summary(r.Context(), nil)
		return ret
	}, rw, r)
}

func (s *Server) Summary(context.Context, *emptypb.Empty) (res *pb.SummaryResponse, err error) {
	s.Call(func() {
		dur := time.Since(s.lastSummaryTime)
		if dur < time.Second {
			res = s.lastSummary
			return
		}
		v, _ := mem.VirtualMemory()
		d, _ := disk.Usage("/")
		nv, _ := IOCounters(true)
		res = &pb.SummaryResponse{
			Memory: &pb.Usage{
				Total: v.Total >> 20,
				Free:  v.Available >> 20,
				Used:  v.Used >> 20,
				Usage: float32(v.UsedPercent),
			},
			HardDisk: &pb.Usage{
				Total: d.Total >> 30,
				Free:  d.Free >> 30,
				Used:  d.Used >> 30,
				Usage: float32(d.UsedPercent),
			},
		}
		if cc, _ := cpu.Percent(time.Second, false); len(cc) > 0 {
			res.CpuUsage = float32(cc[0])
		}
		netWorks := []*pb.NetWorkInfo{}
		for i, n := range nv {
			info := &pb.NetWorkInfo{
				Name:    n.Name,
				Receive: n.BytesRecv,
				Sent:    n.BytesSent,
			}
			if s.lastSummary != nil && len(s.lastSummary.NetWork) > i {
				info.ReceiveSpeed = (n.BytesRecv - s.lastSummary.NetWork[i].Receive) / uint64(dur.Seconds())
				info.SentSpeed = (n.BytesSent - s.lastSummary.NetWork[i].Sent) / uint64(dur.Seconds())
			}
			netWorks = append(netWorks, info)
		}
		res.StreamCount = int32(s.Streams.Length)
		res.NetWork = netWorks
		s.lastSummary = res
		s.lastSummaryTime = time.Now()
	})
	return
}
