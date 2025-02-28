package m7s

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/mcuadros/go-defaults"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
	"m7s.live/v5/pb"
	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/pkg/util"
)

const (
	PullProxyStatusOffline byte = iota
	PullProxyStatusOnline
	PullProxyStatusPulling
	PullProxyStatusDisabled
)

type (
	IPullProxy interface {
		Pull()
	}
	PullProxy struct {
		server                         *Server `gorm:"-:all"`
		task.Work                      `gorm:"-:all" yaml:"-"`
		ID                             uint           `gorm:"primarykey"`
		CreatedAt, UpdatedAt           time.Time      `yaml:"-"`
		DeletedAt                      gorm.DeletedAt `yaml:"-"`
		Name                           string
		StreamPath                     string
		PullOnStart, Audio, StopOnIdle bool
		config.Pull                    `gorm:"embedded;embeddedPrefix:pull_"`
		config.Record                  `gorm:"embedded;embeddedPrefix:record_"`
		ParentID                       uint
		Type                           string
		Status                         byte
		Description                    string
		RTT                            time.Duration
		Handler                        IPullProxy `gorm:"-:all" yaml:"-"`
	}
	PullProxyManager struct {
		task.Manager[uint, *PullProxy]
	}
	PullProxyTask struct {
		task.TickTask
		PullProxy *PullProxy
		Plugin    *Plugin
	}
	HTTPPullProxy struct {
		TCPPullProxy
	}
	TCPPullProxy struct {
		PullProxyTask
		TCPAddr *net.TCPAddr
		URL     *url.URL
	}
)

func (d *PullProxy) GetKey() uint {
	return d.ID
}

func (d *PullProxy) GetStreamPath() string {
	if d.StreamPath == "" {
		return fmt.Sprintf("pull/%s/%d", d.Type, d.ID)
	}
	return d.StreamPath
}

func (d *PullProxy) Start() (err error) {
	for plugin := range d.server.Plugins.Range {
		if pullPlugin, ok := plugin.handler.(IPullProxyPlugin); ok && strings.EqualFold(d.Type, plugin.Meta.Name) {
			pullTask := pullPlugin.OnPullProxyAdd(d)
			if pullTask == nil {
				continue
			}
			if pullTask, ok := pullTask.(IPullProxy); ok {
				d.Handler = pullTask
			}
			if t, ok := pullTask.(task.ITask); ok {
				if ticker, ok := t.(task.IChannelTask); ok {
					t.OnStart(func() {
						ticker.Tick(nil)
					})
				}
				d.AddTask(t)
			} else {
				d.ChangeStatus(PullProxyStatusOnline)
			}
		}
	}
	return
}

func (d *PullProxy) ChangeStatus(status byte) {
	if d.Status == status {
		return
	}
	from := d.Status
	d.Info("device status changed", "from", from, "to", status)
	d.Status = status
	d.Update()
	switch status {
	case PullProxyStatusOnline:
		if d.PullOnStart && from == PullProxyStatusOffline {
			d.Handler.Pull()
		}
	}
}

func (d *PullProxy) Update() {
	if d.server.DB != nil {
		d.server.DB.Omit("deleted_at").Save(d)
	}
}

func (d *PullProxyTask) Dispose() {
	d.PullProxy.ChangeStatus(PullProxyStatusOffline)
	d.TickTask.Dispose()
	d.Plugin.Server.Streams.Call(func() error {
		if stream, ok := d.Plugin.Server.Streams.Get(d.PullProxy.GetStreamPath()); ok {
			stream.Stop(task.ErrStopByUser)
		}
		return nil
	})
}

func (d *PullProxy) InitializeWithServer(s *Server) {
	d.server = s
	d.Logger = s.Logger.With("pullProxy", d.ID, "type", d.Type, "name", d.Name)
	if d.Type == "" {
		u, err := url.Parse(d.URL)
		if err != nil {
			d.Logger.Error("parse pull url failed", "error", err)
			return
		}
		switch u.Scheme {
		case "srt", "rtsp", "rtmp":
			d.Type = u.Scheme
		default:
			ext := filepath.Ext(u.Path)
			switch ext {
			case ".m3u8":
				d.Type = "hls"
			case ".flv":
				d.Type = "flv"
			case ".mp4":
				d.Type = "mp4"
			}
		}
	}
}

func (d *PullProxyTask) Pull() {
	var pubConf = d.Plugin.config.Publish
	pubConf.PubAudio = d.PullProxy.Audio
	pubConf.DelayCloseTimeout = util.Conditional(d.PullProxy.StopOnIdle, time.Second*5, 0)
	d.Plugin.handler.Pull(d.PullProxy.GetStreamPath(), d.PullProxy.Pull, &pubConf)
}

func (d *HTTPPullProxy) Start() (err error) {
	d.URL, err = url.Parse(d.PullProxy.URL)
	if err != nil {
		return
	}
	if ips, err := net.LookupIP(d.URL.Hostname()); err != nil {
		return err
	} else if len(ips) == 0 {
		return fmt.Errorf("no IP found for host: %s", d.URL.Hostname())
	} else {
		d.TCPAddr, err = net.ResolveTCPAddr("tcp", net.JoinHostPort(ips[0].String(), d.URL.Port()))
		if err != nil {
			return err
		}
		if d.TCPAddr.Port == 0 {
			if d.URL.Scheme == "https" || d.URL.Scheme == "wss" {
				d.TCPAddr.Port = 443
			} else {
				d.TCPAddr.Port = 80
			}
		}
	}
	return d.PullProxyTask.Start()
}

func (d *TCPPullProxy) GetTickInterval() time.Duration {
	return time.Second * 10
}

func (d *TCPPullProxy) Tick(any) {
	startTime := time.Now()
	conn, err := net.DialTCP("tcp", nil, d.TCPAddr)
	if err != nil {
		d.PullProxy.ChangeStatus(PullProxyStatusOffline)
		return
	}
	conn.Close()
	d.PullProxy.RTT = time.Since(startTime)
	if d.PullProxy.Status == PullProxyStatusOffline {
		d.PullProxy.ChangeStatus(PullProxyStatusOnline)
	}
}

func (p *Publisher) processPullProxyOnStart() {
	s := p.Plugin.Server
	if pullProxy, ok := s.PullProxies.Find(func(pullProxy *PullProxy) bool {
		return pullProxy.GetStreamPath() == p.StreamPath
	}); ok {
		p.PullProxy = pullProxy
		if pullProxy.Status == PullProxyStatusOnline {
			pullProxy.ChangeStatus(PullProxyStatusPulling)
			if mp4Plugin, ok := s.Plugins.Get("MP4"); ok && pullProxy.FilePath != "" {
				mp4Plugin.Record(p, pullProxy.Record, nil)
			}
		}
	}
}

func (p *Publisher) processPullProxyOnDispose() {
	s := p.Plugin.Server
	if p.PullProxy != nil && p.PullProxy.Status == PullProxyStatusPulling && s.PullProxies.Has(p.PullProxy.GetKey()) {
		p.PullProxy.ChangeStatus(PullProxyStatusOnline)
	}
}

func (s *Server) GetPullProxyList(ctx context.Context, req *emptypb.Empty) (res *pb.PullProxyListResponse, err error) {
	res = &pb.PullProxyListResponse{}
	s.PullProxies.Call(func() error {
		for device := range s.PullProxies.Range {
			res.Data = append(res.Data, &pb.PullProxyInfo{
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
		return nil
	})
	return
}

func (s *Server) AddPullProxy(ctx context.Context, req *pb.PullProxyInfo) (res *pb.SuccessResponse, err error) {
	device := &PullProxy{
		server:      s,
		Name:        req.Name,
		Type:        req.Type,
		ParentID:    uint(req.ParentID),
		PullOnStart: req.PullOnStart,
		Description: req.Description,
		StreamPath:  req.StreamPath,
	}
	if device.Type == "" {
		var u *url.URL
		u, err = url.Parse(req.PullURL)
		if err != nil {
			s.Error("parse pull url failed", "error", err)
			return
		}
		switch u.Scheme {
		case "srt", "rtsp", "rtmp":
			device.Type = u.Scheme
		default:
			ext := filepath.Ext(u.Path)
			switch ext {
			case ".m3u8":
				device.Type = "hls"
			case ".flv":
				device.Type = "flv"
			case ".mp4":
				device.Type = "mp4"
			}
		}
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
	if req.StreamPath == "" {
		device.StreamPath = device.GetStreamPath()
	}
	s.PullProxies.Add(device)
	res = &pb.SuccessResponse{}
	return
}

func (s *Server) UpdatePullProxy(ctx context.Context, req *pb.PullProxyInfo) (res *pb.SuccessResponse, err error) {
	if s.DB == nil {
		err = pkg.ErrNoDB
		return
	}
	target := &PullProxy{
		server: s,
	}
	err = s.DB.First(target, req.ID).Error
	if err != nil {
		return
	}
	target.Name = req.Name
	target.URL = req.PullURL
	target.ParentID = uint(req.ParentID)
	target.Type = req.Type
	if target.Type == "" {
		var u *url.URL
		u, err = url.Parse(req.PullURL)
		if err != nil {
			s.Error("parse pull url failed", "error", err)
			return
		}
		switch u.Scheme {
		case "srt", "rtsp", "rtmp":
			target.Type = u.Scheme
		default:
			ext := filepath.Ext(u.Path)
			switch ext {
			case ".m3u8":
				target.Type = "hls"
			case ".flv":
				target.Type = "flv"
			case ".mp4":
				target.Type = "mp4"
			}
		}
	}
	target.PullOnStart = req.PullOnStart
	target.StopOnIdle = req.StopOnIdle
	target.Audio = req.Audio
	target.Description = req.Description
	target.Record.FilePath = req.RecordPath
	target.Record.Fragment = req.RecordFragment.AsDuration()
	target.RTT = time.Duration(int(req.Rtt)) * time.Millisecond
	target.StreamPath = req.StreamPath
	s.DB.Save(target)
	var needStopOld *PullProxy
	s.PullProxies.Call(func() error {
		if device, ok := s.PullProxies.Get(uint(req.ID)); ok {
			if target.URL != device.URL || device.Audio != target.Audio || device.StreamPath != target.StreamPath || device.Record.FilePath != target.Record.FilePath || device.Record.Fragment != target.Record.Fragment {
				device.Stop(task.ErrStopByUser)
				if pull, ok := device.server.Pulls.Get(device.StreamPath); ok {
					pull.Stop(task.ErrStopByUser)
				}
				needStopOld = device
				return nil
			}
			if device.PullOnStart != target.PullOnStart && target.PullOnStart && device.Handler != nil && device.Status == PullProxyStatusOnline {
				device.Handler.Pull()
			}
			device.Name = target.Name
			device.PullOnStart = target.PullOnStart
			device.StopOnIdle = target.StopOnIdle
			device.Description = target.Description
		}
		return nil
	})
	if needStopOld != nil {
		needStopOld.WaitStopped()
		s.PullProxies.Add(target)
	}
	res = &pb.SuccessResponse{}
	return
}

func (s *Server) RemovePullProxy(ctx context.Context, req *pb.RequestWithId) (res *pb.SuccessResponse, err error) {
	if s.DB == nil {
		err = pkg.ErrNoDB
		return
	}
	res = &pb.SuccessResponse{}
	if req.Id > 0 {
		tx := s.DB.Delete(&PullProxy{
			ID: uint(req.Id),
		})
		err = tx.Error
		s.PullProxies.Call(func() error {
			if device, ok := s.PullProxies.Get(uint(req.Id)); ok {
				device.Stop(task.ErrStopByUser)
				if pull, ok := device.server.Pulls.Get(device.StreamPath); ok {
					pull.Stop(task.ErrStopByUser)
				}
			}
			return nil
		})
		return
	} else if req.StreamPath != "" {
		var deviceList []PullProxy
		s.DB.Find(&deviceList, "stream_path=?", req.StreamPath)
		if len(deviceList) > 0 {
			for _, device := range deviceList {
				tx := s.DB.Delete(&PullProxy{}, device.ID)
				err = tx.Error
				s.PullProxies.Call(func() error {
					if device, ok := s.PullProxies.Get(uint(device.ID)); ok {
						device.Stop(task.ErrStopByUser)
						if pull, ok := device.server.Pulls.Get(device.StreamPath); ok {
							pull.Stop(task.ErrStopByUser)
						}
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
