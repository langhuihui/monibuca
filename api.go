package m7s

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"m7s.live/v5/pkg/task"

	"github.com/mcuadros/go-defaults"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	. "github.com/shirou/gopsutil/v4/net"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gopkg.in/yaml.v3"
	"m7s.live/v5/pb"
	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/util"
)

var localIP string
var empty = &emptypb.Empty{}

func (s *Server) SysInfo(context.Context, *emptypb.Empty) (res *pb.SysInfoResponse, err error) {
	if localIP == "" {
		if conn, err := net.Dial("udp", "114.114.114.114:80"); err == nil {
			localIP, _, _ = strings.Cut(conn.LocalAddr().String(), ":")
		}
	}
	res = &pb.SysInfoResponse{
		Code:    0,
		Message: "success",
		Data: &pb.SysInfoData{
			Version:   Version,
			LocalIP:   localIP,
			StartTime: timestamppb.New(s.StartTime),
			GoVersion: runtime.Version(),
			Os:        runtime.GOOS,
			Arch:      runtime.GOARCH,
			Cpus:      int32(runtime.NumCPU()),
		},
	}
	for p := range s.Plugins.Range {
		res.Data.Plugins = append(res.Data.Plugins, &pb.PluginInfo{
			Name:        p.Meta.Name,
			Disabled:    p.Disabled,
			Description: p.GetDescriptions(),
		})
	}
	return
}

// /api/stream/annexb/{streamPath}
func (s *Server) api_Stream_AnnexB_(rw http.ResponseWriter, r *http.Request) {
	publisher, ok := s.Streams.Get(r.PathValue("streamPath"))
	if !ok || publisher.VideoTrack.AVTrack == nil {
		http.Error(rw, pkg.ErrNotFound.Error(), http.StatusNotFound)
		return
	}
	err := publisher.VideoTrack.WaitReady()
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	rw.Header().Set("Content-Type", "application/octet-stream")
	reader := pkg.NewAVRingReader(publisher.VideoTrack.AVTrack, "Origin")
	err = reader.StartRead(publisher.VideoTrack.GetIDR())
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	defer reader.StopRead()
	if reader.Value.Raw == nil {
		if err = reader.Value.Demux(publisher.VideoTrack.ICodecCtx); err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	var annexb pkg.AnnexB
	var t pkg.AVTrack

	t.ICodecCtx, t.SequenceFrame, err = annexb.ConvertCtx(publisher.VideoTrack.ICodecCtx)
	if t.ICodecCtx == nil {
		http.Error(rw, "unsupported codec", http.StatusInternalServerError)
		return
	}
	annexb.Mux(t.ICodecCtx, &reader.Value)
	_, err = annexb.WriteTo(rw)
}

func (s *Server) getStreamInfo(pub *Publisher) (res *pb.StreamInfoResponse, err error) {
	tmp, _ := json.Marshal(pub.GetDescriptions())
	res = &pb.StreamInfoResponse{
		Data: &pb.StreamInfo{
			Meta:        string(tmp),
			Path:        pub.StreamPath,
			State:       int32(pub.State),
			StartTime:   timestamppb.New(pub.StartTime),
			Subscribers: int32(pub.Subscribers.Length),
			PluginName:  pub.Plugin.Meta.Name,
			Type:        pub.Type,
			Speed:       float32(pub.Speed),
			StopOnIdle:  pub.DelayCloseTimeout > 0,
			IsPaused:    pub.Paused != nil,
			Gop:         int32(pub.GOP),
			BufferTime:  durationpb.New(pub.BufferTime),
		},
	}

	if t := pub.AudioTrack.AVTrack; t != nil {
		if t.ICodecCtx != nil {
			res.Data.AudioTrack = &pb.AudioTrackInfo{
				Codec: t.FourCC().String(),
				Meta:  t.GetInfo(),
				Bps:   uint32(t.BPS),
				Fps:   uint32(t.FPS),
				Delta: pub.AudioTrack.Delta.String(),
			}
			res.Data.AudioTrack.SampleRate = uint32(t.ICodecCtx.(pkg.IAudioCodecCtx).GetSampleRate())
			res.Data.AudioTrack.Channels = uint32(t.ICodecCtx.(pkg.IAudioCodecCtx).GetChannels())
		}
	}
	if t := pub.VideoTrack.AVTrack; t != nil {
		if t.ICodecCtx != nil {
			res.Data.VideoTrack = &pb.VideoTrackInfo{
				Codec: t.FourCC().String(),
				Meta:  t.GetInfo(),
				Bps:   uint32(t.BPS),
				Fps:   uint32(t.FPS),
				Delta: pub.VideoTrack.Delta.String(),
				Gop:   uint32(pub.GOP),
			}
			res.Data.VideoTrack.Width = uint32(t.ICodecCtx.(pkg.IVideoCodecCtx).Width())
			res.Data.VideoTrack.Height = uint32(t.ICodecCtx.(pkg.IVideoCodecCtx).Height())
		}
	}
	return
}

func (s *Server) StreamInfo(ctx context.Context, req *pb.StreamSnapRequest) (res *pb.StreamInfoResponse, err error) {
	s.Streams.Call(func() error {
		if pub, ok := s.Streams.Get(req.StreamPath); ok {
			res, err = s.getStreamInfo(pub)
		} else {
			err = pkg.ErrNotFound
		}
		return nil
	})
	return
}

func (s *Server) TaskTree(context.Context, *emptypb.Empty) (res *pb.TaskTreeResponse, err error) {
	var fillData func(m task.ITask) *pb.TaskTreeData
	fillData = func(m task.ITask) (res *pb.TaskTreeData) {
		res = &pb.TaskTreeData{Id: m.GetTaskID(), Pointer: uint64(uintptr(unsafe.Pointer(m.GetTask()))), State: uint32(m.GetState()), Type: uint32(m.GetTaskType()), Owner: m.GetOwnerType(), StartTime: timestamppb.New(m.GetTask().StartTime), Description: m.GetDescriptions()}
		if job, ok := m.(task.IJob); ok {
			if blockedTask := job.Blocked(); blockedTask != nil {
				res.Blocked = fillData(blockedTask)
			}
			for t := range job.RangeSubTask {
				res.Children = append(res.Children, fillData(t))
			}
		}
		return
	}
	res = &pb.TaskTreeResponse{Data: fillData(&Servers)}
	return
}

func (s *Server) StopTask(ctx context.Context, req *pb.RequestWithId64) (resp *pb.SuccessResponse, err error) {
	t := (*task.Task)(unsafe.Pointer(uintptr(req.Id)))
	if t == nil {
		return nil, pkg.ErrNotFound
	}
	t.Stop(task.ErrStopByUser)
	return &pb.SuccessResponse{}, nil
}

func (s *Server) RestartTask(ctx context.Context, req *pb.RequestWithId64) (resp *pb.SuccessResponse, err error) {
	t := (*task.Task)(unsafe.Pointer(uintptr(req.Id)))
	if t == nil {
		return nil, pkg.ErrNotFound
	}
	t.Stop(task.ErrRestart)
	return &pb.SuccessResponse{}, nil
}

func (s *Server) GetRecording(ctx context.Context, req *emptypb.Empty) (resp *pb.RecordingListResponse, err error) {
	if s.DB == nil {
		err = pkg.ErrNoDB
		return
	}
	s.Records.Call(func() error {
		resp = &pb.RecordingListResponse{}
		for record := range s.Records.Range {
			resp.Data = append(resp.Data, &pb.Recording{
				StreamPath: record.StreamPath,
				StartTime:  timestamppb.New(record.StartTime),
				Type:       reflect.TypeOf(record.recorder).String(),
			})
		}
		return nil
	})
	return
}

func (s *Server) GetSubscribers(context.Context, *pb.SubscribersRequest) (res *pb.SubscribersResponse, err error) {
	s.Streams.Call(func() error {
		var subscribers []*pb.SubscriberSnapShot
		for subscriber := range s.Subscribers.Range {
			meta, _ := json.Marshal(subscriber.GetDescriptions())
			snap := &pb.SubscriberSnapShot{
				Id:         subscriber.ID,
				StartTime:  timestamppb.New(subscriber.StartTime),
				Meta:       string(meta),
				Type:       subscriber.Type,
				PluginName: subscriber.Plugin.Meta.Name,
				SubMode:    int32(subscriber.SubMode),
				SyncMode:   int32(subscriber.SyncMode),
				BufferTime: durationpb.New(subscriber.BufferTime),
				RemoteAddr: subscriber.RemoteAddr,
			}
			if ar := subscriber.AudioReader; ar != nil {
				snap.AudioReader = &pb.RingReaderSnapShot{
					Sequence:  ar.Value.Sequence,
					Timestamp: ar.AbsTime,
					Delay:     ar.Delay,
					State:     int32(ar.State),
				}
			}
			if vr := subscriber.VideoReader; vr != nil {
				snap.VideoReader = &pb.RingReaderSnapShot{
					Sequence:  vr.Value.Sequence,
					Timestamp: vr.AbsTime,
					Delay:     vr.Delay,
					State:     int32(vr.State),
				}
			}
			subscribers = append(subscribers, snap)
		}
		res = &pb.SubscribersResponse{
			Data:  subscribers,
			Total: int32(s.Subscribers.Length),
		}
		return nil
	})
	return
}
func (s *Server) AudioTrackSnap(_ context.Context, req *pb.StreamSnapRequest) (res *pb.TrackSnapShotResponse, err error) {
	s.Streams.Call(func() error {
		if pub, ok := s.Streams.Get(req.StreamPath); ok && pub.HasAudioTrack() {
			data := &pb.TrackSnapShotData{}
			if pub.AudioTrack.Allocator != nil {
				for _, memlist := range pub.AudioTrack.Allocator.GetChildren() {
					var list []*pb.MemoryBlock
					for _, block := range memlist.GetBlocks() {
						list = append(list, &pb.MemoryBlock{
							S: uint32(block.Start),
							E: uint32(block.End),
						})
					}
					data.Memory = append(data.Memory, &pb.MemoryBlockGroup{List: list, Size: uint32(memlist.Size)})
				}
			}
			pub.AudioTrack.Ring.Do(func(v *pkg.AVFrame) {
				if len(v.Wraps) > 0 {
					var snap pb.TrackSnapShot
					snap.Sequence = v.Sequence
					snap.Timestamp = uint32(v.Timestamp / time.Millisecond)
					snap.WriteTime = timestamppb.New(v.WriteTime)
					snap.Wrap = make([]*pb.Wrap, len(v.Wraps))
					snap.KeyFrame = v.IDR
					data.RingDataSize += uint32(v.Wraps[0].GetSize())
					for i, wrap := range v.Wraps {
						snap.Wrap[i] = &pb.Wrap{
							Timestamp: uint32(wrap.GetTimestamp() / time.Millisecond),
							Size:      uint32(wrap.GetSize()),
							Data:      wrap.String(),
						}
					}
					data.Ring = append(data.Ring, &snap)
				}
			})
			res = &pb.TrackSnapShotResponse{
				Code:    0,
				Message: "success",
				Data:    data,
			}
		} else {
			err = pkg.ErrNotFound
		}
		return nil
	})
	return
}
func (s *Server) api_VideoTrack_SSE(rw http.ResponseWriter, r *http.Request) {
	streamPath := r.PathValue("streamPath")
	if r.URL.RawQuery != "" {
		streamPath += "?" + r.URL.RawQuery
	}
	suber, err := s.Subscribe(r.Context(), streamPath)
	sse := util.NewSSE(rw, r.Context())
	PlayBlock(suber, (func(frame *pkg.AVFrame) (err error))(nil), func(frame *pkg.AVFrame) (err error) {
		var snap pb.TrackSnapShot
		snap.Sequence = frame.Sequence
		snap.Timestamp = uint32(frame.Timestamp / time.Millisecond)
		snap.WriteTime = timestamppb.New(frame.WriteTime)
		snap.Wrap = make([]*pb.Wrap, len(frame.Wraps))
		snap.KeyFrame = frame.IDR
		for i, wrap := range frame.Wraps {
			snap.Wrap[i] = &pb.Wrap{
				Timestamp: uint32(wrap.GetTimestamp() / time.Millisecond),
				Size:      uint32(wrap.GetSize()),
				Data:      wrap.String(),
			}
		}
		return sse.WriteJSON(&snap)
	})
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}
}

func (s *Server) VideoTrackSnap(ctx context.Context, req *pb.StreamSnapRequest) (res *pb.TrackSnapShotResponse, err error) {
	s.Streams.Call(func() error {
		if pub, ok := s.Streams.Get(req.StreamPath); ok && pub.HasVideoTrack() {
			data := &pb.TrackSnapShotData{}
			if pub.VideoTrack.Allocator != nil {
				for _, memlist := range pub.VideoTrack.Allocator.GetChildren() {
					var list []*pb.MemoryBlock
					for _, block := range memlist.GetBlocks() {
						list = append(list, &pb.MemoryBlock{
							S: uint32(block.Start),
							E: uint32(block.End),
						})
					}
					data.Memory = append(data.Memory, &pb.MemoryBlockGroup{List: list, Size: uint32(memlist.Size)})
				}
			}
			pub.VideoTrack.Ring.Do(func(v *pkg.AVFrame) {
				//if v.TryRLock() {
				if len(v.Wraps) > 0 {
					var snap pb.TrackSnapShot
					snap.Sequence = v.Sequence
					snap.Timestamp = uint32(v.Timestamp / time.Millisecond)
					snap.WriteTime = timestamppb.New(v.WriteTime)
					snap.Wrap = make([]*pb.Wrap, len(v.Wraps))
					snap.KeyFrame = v.IDR
					data.RingDataSize += uint32(v.Wraps[0].GetSize())
					for i, wrap := range v.Wraps {
						snap.Wrap[i] = &pb.Wrap{
							Timestamp: uint32(wrap.GetTimestamp() / time.Millisecond),
							Size:      uint32(wrap.GetSize()),
							Data:      wrap.String(),
						}
					}
					data.Ring = append(data.Ring, &snap)
				}
				//v.RUnlock()
				//}
			})
			res = &pb.TrackSnapShotResponse{
				Code:    0,
				Message: "success",
				Data:    data,
			}
		} else {
			err = pkg.ErrNotFound
		}
		return nil
	})
	return
}

// Restart stops the server with a restart error and returns
// a success response. This method is used to restart the server
// gracefully.
func (s *Server) Restart(ctx context.Context, req *pb.RequestWithId) (res *pb.SuccessResponse, err error) {
	s.Stop(pkg.ErrRestart)
	return &pb.SuccessResponse{}, err
}

func (s *Server) Shutdown(ctx context.Context, req *pb.RequestWithId) (res *pb.SuccessResponse, err error) {
	s.Stop(task.ErrStopByUser)
	return &pb.SuccessResponse{}, err
}

func (s *Server) ChangeSubscribe(ctx context.Context, req *pb.ChangeSubscribeRequest) (res *pb.SuccessResponse, err error) {
	s.Streams.Call(func() error {
		if subscriber, ok := s.Subscribers.Get(req.Id); ok {
			if pub, ok := s.Streams.Get(req.StreamPath); ok {
				subscriber.Publisher.RemoveSubscriber(subscriber)
				subscriber.StreamPath = req.StreamPath
				pub.AddSubscriber(subscriber)
				return nil
			}
		}
		err = pkg.ErrNotFound
		return nil
	})
	return &pb.SuccessResponse{}, err
}

func (s *Server) StopSubscribe(ctx context.Context, req *pb.RequestWithId) (res *pb.SuccessResponse, err error) {
	s.Streams.Call(func() error {
		if subscriber, ok := s.Subscribers.Get(req.Id); ok {
			subscriber.Stop(errors.New("stop by api"))
		} else {
			err = pkg.ErrNotFound
		}
		return nil
	})
	return &pb.SuccessResponse{}, err
}

func (s *Server) PauseStream(ctx context.Context, req *pb.StreamSnapRequest) (res *pb.SuccessResponse, err error) {
	s.Streams.Call(func() error {
		if s, ok := s.Streams.Get(req.StreamPath); ok {
			s.Pause()
		}
		return nil
	})
	return &pb.SuccessResponse{}, err
}

func (s *Server) ResumeStream(ctx context.Context, req *pb.StreamSnapRequest) (res *pb.SuccessResponse, err error) {
	s.Streams.Call(func() error {
		if s, ok := s.Streams.Get(req.StreamPath); ok {
			s.Resume()
		}
		return nil
	})
	return &pb.SuccessResponse{}, err
}

func (s *Server) SetStreamSpeed(ctx context.Context, req *pb.SetStreamSpeedRequest) (res *pb.SuccessResponse, err error) {
	s.Streams.Call(func() error {
		if s, ok := s.Streams.Get(req.StreamPath); ok {
			s.Speed = float64(req.Speed)
		}
		return nil
	})
	return &pb.SuccessResponse{}, err
}

func (s *Server) SeekStream(ctx context.Context, req *pb.SeekStreamRequest) (res *pb.SuccessResponse, err error) {
	s.Streams.Call(func() error {
		if s, ok := s.Streams.Get(req.StreamPath); ok {
			s.Seek(time.Unix(int64(req.TimeStamp), 0))
		}
		return nil
	})
	return &pb.SuccessResponse{}, err
}

func (s *Server) StopPublish(ctx context.Context, req *pb.StreamSnapRequest) (res *pb.SuccessResponse, err error) {
	s.Streams.Call(func() error {
		if s, ok := s.Streams.Get(req.StreamPath); ok {
			s.Stop(task.ErrStopByUser)
		}
		return nil
	})
	return &pb.SuccessResponse{}, err
}

// /api/stream/list
func (s *Server) StreamList(_ context.Context, req *pb.StreamListRequest) (res *pb.StreamListResponse, err error) {
	s.Streams.Call(func() error {
		var streams []*pb.StreamInfo
		for publisher := range s.Streams.Range {
			info, err := s.getStreamInfo(publisher)
			if err != nil {
				continue
			}
			streams = append(streams, info.Data)
		}
		res = &pb.StreamListResponse{Data: streams, Total: int32(s.Streams.Length), PageNum: req.PageNum, PageSize: req.PageSize}
		return nil
	})
	return
}

func (s *Server) WaitList(context.Context, *emptypb.Empty) (res *pb.StreamWaitListResponse, err error) {
	s.Streams.Call(func() error {
		res = &pb.StreamWaitListResponse{
			List: make(map[string]int32),
		}
		for subs := range s.Waiting.Range {
			res.List[subs.StreamPath] = int32(subs.Length)
		}
		return nil
	})
	return
}

func (s *Server) Api_Summary_SSE(rw http.ResponseWriter, r *http.Request) {
	util.ReturnFetchValue(func() *pb.SummaryResponse {
		ret, _ := s.Summary(r.Context(), nil)
		return ret
	}, rw, r)
}

func (s *Server) Api_Stream_Position_SSE(rw http.ResponseWriter, r *http.Request) {
	streamPath := r.URL.Query().Get("streamPath")
	util.ReturnFetchValue(func() (t time.Time) {
		s.Streams.Call(func() error {
			if pub, ok := s.Streams.Get(streamPath); ok {
				t = pub.GetPosition()
			}
			return nil
		})
		return
	}, rw, r)
}

func (s *Server) Summary(context.Context, *emptypb.Empty) (res *pb.SummaryResponse, err error) {
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
	if cc, _ := cpu.Percent(0, false); len(cc) > 0 {
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
	res.PullCount = int32(s.Pulls.Length)
	res.PushCount = int32(s.Pushs.Length)
	res.SubscribeCount = int32(s.Subscribers.Length)
	res.RecordCount = int32(s.Records.Length)
	res.TransformCount = int32(s.Transforms.Length)
	res.NetWork = netWorks
	s.lastSummary = res
	s.lastSummaryTime = time.Now()
	return
}

// /api/config/json/{name}
func (s *Server) api_Config_JSON_(rw http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var conf *config.Config
	if name == "global" {
		conf = &s.Config
	} else {
		p, ok := s.Plugins.Get(name)
		if !ok {
			http.Error(rw, pkg.ErrNotFound.Error(), http.StatusNotFound)
			return
		}
		conf = &p.Config
	}
	rw.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(rw).Encode(conf.GetMap())
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) GetConfigFile(_ context.Context, req *emptypb.Empty) (res *pb.GetConfigFileResponse, err error) {
	res = &pb.GetConfigFileResponse{}
	res.Data = string(s.configFileContent)
	return
}

func (s *Server) UpdateConfigFile(_ context.Context, req *pb.UpdateConfigFileRequest) (res *pb.SuccessResponse, err error) {
	if s.configFileContent != nil {
		s.configFileContent = []byte(req.Content)
		os.WriteFile(filepath.Join(ExecDir, s.conf.(string)), s.configFileContent, 0644)
		res = &pb.SuccessResponse{}
	} else {
		err = pkg.ErrNotFound
	}
	return
}

func (s *Server) GetConfig(_ context.Context, req *pb.GetConfigRequest) (res *pb.GetConfigResponse, err error) {
	res = &pb.GetConfigResponse{
		Data: &pb.ConfigData{},
	}
	var conf *config.Config
	if req.Name == "global" {
		conf = &s.Config
	} else {
		p, ok := s.Plugins.Get(req.Name)
		if !ok {
			err = pkg.ErrNotFound
			return
		}
		conf = &p.Config
	}
	var mm []byte
	mm, err = yaml.Marshal(conf.File)
	if err != nil {
		return
	}
	res.Data.File = string(mm)

	mm, err = yaml.Marshal(conf.Modify)
	if err != nil {
		return
	}
	res.Data.Modified = string(mm)

	mm, err = yaml.Marshal(conf.GetMap())
	if err != nil {
		return
	}
	res.Data.Merged = string(mm)
	return
}

func (s *Server) ModifyConfig(_ context.Context, req *pb.ModifyConfigRequest) (res *pb.SuccessResponse, err error) {
	var conf *config.Config
	if req.Name == "global" {
		conf = &s.Config
		defer s.SaveConfig()
	} else {
		p, ok := s.Plugins.Get(req.Name)
		if !ok {
			err = pkg.ErrNotFound
			return
		}
		defer p.SaveConfig()
		conf = &p.Config
	}
	var modified map[string]any
	err = yaml.Unmarshal([]byte(req.Yaml), &modified)
	if err != nil {
		return
	}
	conf.ParseModifyFile(modified)
	return
}

func (s *Server) GetDeviceList(ctx context.Context, req *emptypb.Empty) (res *pb.DeviceListResponse, err error) {
	res = &pb.DeviceListResponse{}
	for device := range s.Devices.Range {
		res.Data = append(res.Data, &pb.DeviceInfo{
			Name:           device.Name,
			CreateTime:     timestamppb.New(device.CreatedAt),
			UpdateTime:     timestamppb.New(device.UpdatedAt),
			Type:           device.Type,
			PullURL:        device.URL,
			ParentID:       uint32(device.ParentID),
			Status:         uint32(device.Status),
			ID:             uint32(device.ID),
			PullOnStart:    device.PullOnStart,
			StopOnIdle:     device.StopOnIdle,
			Audio:          device.Audio,
			RecordPath:     device.Record.FilePath,
			RecordFragment: durationpb.New(device.Record.Fragment),
			Description:    device.Description,
			Rtt:            uint32(device.RTT.Milliseconds()),
			StreamPath:     device.GetStreamPath(),
		})
	}
	return
}

func (s *Server) AddDevice(ctx context.Context, req *pb.DeviceInfo) (res *pb.SuccessResponse, err error) {
	device := &Device{
		server:      s,
		Name:        req.Name,
		Type:        req.Type,
		ParentID:    uint(req.ParentID),
		PullOnStart: req.PullOnStart,
		Description: req.Description,
		StreamPath:  req.StreamPath,
	}
	defaults.SetDefaults(&device.Pull)
	defaults.SetDefaults(&device.Record)
	device.URL = req.PullURL
	device.Audio = req.Audio
	device.StopOnIdle = req.StopOnIdle
	device.Record.FilePath = req.RecordPath
	device.Record.Fragment = req.RecordFragment.AsDuration()
	if s.DB == nil {
		err = pkg.ErrNoDB
		return
	}
	s.DB.Create(device)
	s.Devices.Add(device)
	res = &pb.SuccessResponse{}
	return
}

func (s *Server) UpdateDevice(ctx context.Context, req *pb.DeviceInfo) (res *pb.SuccessResponse, err error) {
	if s.DB == nil {
		err = pkg.ErrNoDB
		return
	}
	target := &Device{}
	s.DB.First(target, req.ID)
	target.Name = req.Name
	target.URL = req.PullURL
	target.ParentID = uint(req.ParentID)
	target.Type = req.Type
	target.PullOnStart = req.PullOnStart
	target.StopOnIdle = req.StopOnIdle
	target.Audio = req.Audio
	target.Description = req.Description
	target.Record.FilePath = req.RecordPath
	target.Record.Fragment = req.RecordFragment.AsDuration()
	target.RTT = time.Duration(int(req.Rtt)) * time.Millisecond
	target.StreamPath = req.StreamPath
	s.DB.Save(target)
	res = &pb.SuccessResponse{}
	return
}

func (s *Server) RemoveDevice(ctx context.Context, req *pb.RequestWithId) (res *pb.SuccessResponse, err error) {
	if s.DB == nil {
		err = pkg.ErrNoDB
		return
	}
	res = &pb.SuccessResponse{}
	if req.Id > 0 {
		tx := s.DB.Delete(&Device{
			ID: uint(req.Id),
		})
		err = tx.Error
		s.Devices.Call(func() error {
			if device, ok := s.Devices.Get(uint(req.Id)); ok {
				device.Stop(task.ErrStopByUser)
			}
			return nil
		})
		return
	} else if req.StreamPath != "" {
		var deviceList []Device
		s.DB.Find(&deviceList, "stream_path=?", req.StreamPath)
		if len(deviceList) > 0 {
			for _, device := range deviceList {
				tx := s.DB.Delete(&Device{}, device.ID)
				err = tx.Error
				s.Devices.Call(func() error {
					if device, ok := s.Devices.Get(uint(device.ID)); ok {
						device.Stop(task.ErrStopByUser)
					}
					return nil
				})
			}
		}
		return
	} else {
		res.Message = "parameter wrong"
		return
	}
}

func (s *Server) GetStreamAlias(ctx context.Context, req *emptypb.Empty) (res *pb.StreamAliasListResponse, err error) {
	res = &pb.StreamAliasListResponse{}
	s.Streams.Call(func() error {
		for alias := range s.AliasStreams.Range {
			info := &pb.StreamAlias{
				StreamPath: alias.StreamPath,
				Alias:      alias.Alias,
				AutoRemove: alias.AutoRemove,
			}
			if s.Streams.Has(alias.Alias) {
				info.Status = 2
			} else if alias.Publisher != nil {
				info.Status = 1
			}
			res.Data = append(res.Data, info)
		}
		return nil
	})
	return
}

func (s *Server) SetStreamAlias(ctx context.Context, req *pb.SetStreamAliasRequest) (res *pb.SuccessResponse, err error) {
	res = &pb.SuccessResponse{}
	s.Streams.Call(func() error {
		if req.StreamPath != "" {
			u, err := url.Parse(req.StreamPath)
			if err != nil {
				return err
			}
			req.StreamPath = strings.TrimPrefix(u.Path, "/")
			publisher, canReplace := s.Streams.Get(req.StreamPath)
			if !canReplace {
				defer s.OnSubscribe(req.StreamPath, u.Query())
			}
			if aliasInfo, ok := s.AliasStreams.Get(req.Alias); ok { //modify alias
				aliasInfo.AutoRemove = req.AutoRemove
				if aliasInfo.StreamPath != req.StreamPath {
					aliasInfo.StreamPath = req.StreamPath
					if canReplace {
						if aliasInfo.Publisher != nil {
							aliasInfo.TransferSubscribers(publisher) // replace stream
						} else {
							s.Waiting.WakeUp(req.Alias, publisher)
						}
					}
				}
			} else { // create alias
				aliasInfo := &AliasStream{
					AutoRemove: req.AutoRemove,
					StreamPath: req.StreamPath,
					Alias:      req.Alias,
				}
				var pubId uint32
				s.AliasStreams.Add(aliasInfo)
				aliasStream, ok := s.Streams.Get(aliasInfo.Alias)
				if canReplace {
					aliasInfo.Publisher = publisher
					if ok {
						aliasStream.TransferSubscribers(publisher) // replace stream
					} else {
						s.Waiting.WakeUp(req.Alias, publisher)
					}
				} else {
					aliasInfo.Publisher = aliasStream
				}
				if aliasInfo.Publisher != nil {
					pubId = aliasInfo.Publisher.ID
				}
				s.Info("add alias", "alias", req.Alias, "streamPath", req.StreamPath, "replace", ok && canReplace, "pub", pubId)
			}
		} else {
			s.Info("remove alias", "alias", req.Alias)
			if aliasStream, ok := s.AliasStreams.Get(req.Alias); ok {
				s.AliasStreams.Remove(aliasStream)
				if aliasStream.Publisher != nil {
					if publisher, hasTarget := s.Streams.Get(req.Alias); hasTarget { // restore stream
						aliasStream.TransferSubscribers(publisher)
					}
				} else {
					var args url.Values
					for sub := range aliasStream.Publisher.SubscriberRange {
						if sub.StreamPath == req.Alias {
							aliasStream.Publisher.RemoveSubscriber(sub)
							s.Waiting.Wait(sub)
							args = sub.Args
						}
					}
					if args != nil {
						s.OnSubscribe(req.Alias, args)
					}
				}
			}
		}
		return nil
	})
	return
}
