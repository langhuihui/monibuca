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
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
	"m7s.live/v5/pb"
	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/task"
)

const (
	PushProxyStatusOffline byte = iota
	PushProxyStatusOnline
	PushProxyStatusPushing
	PushProxyStatusDisabled
)

type (
	IPushProxy interface {
		task.ITask
		GetBase() *BasePushProxy
		GetStreamPath() string
		GetConfig() *PushProxyConfig
		ChangeStatus(status byte)
		Push()
		GetKey() uint
	}
	PushProxyConfig struct {
		ID                   uint           `gorm:"primarykey"`
		CreatedAt, UpdatedAt time.Time      `yaml:"-"`
		DeletedAt            gorm.DeletedAt `yaml:"-"`
		Name                 string
		StreamPath           string
		Audio, PushOnStart   bool
		config.Push          `gorm:"embedded;embeddedPrefix:push_"`
		ParentID             uint
		Type                 string
		Status               byte
		Description          string
		RTT                  time.Duration
	}
	PushProxyFactory = func() IPushProxy
	PushProxyManager struct {
		task.WorkCollection[uint, IPushProxy]
	}
	BasePushProxy struct {
		*PushProxyConfig
		Plugin *Plugin
	}
	HTTPPushProxy struct {
		TCPPushProxy
	}
	TCPPushProxy struct {
		task.AsyncTickTask
		BasePushProxy
		TCPAddr *net.TCPAddr
		URL     *url.URL
	}
)

func (d *PushProxyConfig) GetKey() uint {
	return d.ID
}

func (d *PushProxyConfig) GetConfig() *PushProxyConfig {
	return d
}

func (d *PushProxyConfig) GetStreamPath() string {
	if d.StreamPath == "" {
		return fmt.Sprintf("push/%s/%d", d.Type, d.ID)
	}
	return d.StreamPath
}

func (s *Server) createPushProxy(conf *PushProxyConfig) (pushProxy IPushProxy, err error) {
	for plugin := range s.Plugins.Range {
		if plugin.Meta.NewPushProxy != nil && strings.EqualFold(conf.Type, plugin.Meta.Name) {
			pushProxy = plugin.Meta.NewPushProxy()
			base := pushProxy.GetBase()
			base.PushProxyConfig = conf
			base.Plugin = plugin
			s.PushProxies.AddTask(pushProxy, plugin.Logger.With("pushProxyId", conf.ID, "pushProxyType", conf.Type, "pushProxyName", conf.Name))
			return
		}
	}
	return
}

func (b *BasePushProxy) GetBase() *BasePushProxy {
	return b
}

func NewHTTPPushProxy() IPushProxy {
	return &HTTPPushProxy{}
}

func (d *BasePushProxy) ChangeStatus(status byte) {
	if d.Status == status {
		return
	}
	from := d.Status
	d.Plugin.Info("device status changed", "from", from, "to", status)
	d.Status = status
	switch status {
	case PushProxyStatusOnline:
		if from == PushProxyStatusOffline {
			if d.PushOnStart {
				d.Push()
			} else {
				d.Plugin.Server.CallOnStreamTask(func() {
					if d.Plugin.Server.Streams.Has(d.GetStreamPath()) {
						d.Push()
					}
				})
			}
		}
	}
}

func (d *BasePushProxy) Dispose() {
	d.ChangeStatus(PushProxyStatusOffline)
	pushJob, ok := d.Plugin.Server.Pushs.Find(func(job *PushJob) bool {
		return job.StreamPath == d.GetStreamPath()
	})
	if ok {
		pushJob.Stop(task.ErrStopByUser)
	}
}

func (d *BasePushProxy) Push() {
	var subConf = d.Plugin.config.Subscribe
	subConf.SubAudio = d.Audio
	d.Plugin.handler.Push(d.GetStreamPath(), d.PushProxyConfig.Push, &subConf)
}

func (d *TCPPushProxy) GetTickInterval() time.Duration {
	return time.Second * 10
}

func (d *TCPPushProxy) Tick(any) {
	startTime := time.Now()
	conn, err := net.DialTCP("tcp", nil, d.TCPAddr)
	if err != nil {
		d.ChangeStatus(PushProxyStatusOffline)
		return
	}
	conn.Close()
	d.RTT = time.Since(startTime)
	if d.Status == PushProxyStatusOffline {
		d.ChangeStatus(PushProxyStatusOnline)
	}
}

func (d *HTTPPushProxy) Start() (err error) {
	d.URL, err = url.Parse(d.PushProxyConfig.URL)
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
	return d.TCPPushProxy.Start()
}

func (d *PushProxyConfig) InitializeWithServer(s *Server) {
	if d.Type == "" {
		u, err := url.Parse(d.URL)
		if err != nil {
			s.Error("parse push url failed", "error", err)
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

func (s *Server) GetPushProxyList(ctx context.Context, req *emptypb.Empty) (res *pb.PushProxyListResponse, err error) {
	res = &pb.PushProxyListResponse{}

	if s.DB == nil {
		err = pkg.ErrNoDB
		return
	}

	var pushProxyConfigs []PushProxyConfig
	err = s.DB.Find(&pushProxyConfigs).Error
	if err != nil {
		return
	}

	for _, conf := range pushProxyConfigs {
		// 获取运行时状态信息（如果需要的话）
		info := &pb.PushProxyInfo{
			Name:        conf.Name,
			CreateTime:  timestamppb.New(conf.CreatedAt),
			UpdateTime:  timestamppb.New(conf.UpdatedAt),
			Type:        conf.Type,
			PushURL:     conf.URL,
			ParentID:    uint32(conf.ParentID),
			Status:      uint32(conf.Status),
			ID:          uint32(conf.ID),
			PushOnStart: conf.PushOnStart,
			Audio:       conf.Audio,
			Description: conf.Description,
			StreamPath:  conf.GetStreamPath(),
		}
		// 如果内存中有对应的设备，获取实时状态
		if device, ok := s.PushProxies.Get(conf.ID); ok {
			runtimeConf := device.GetConfig()
			info.Rtt = uint32(runtimeConf.RTT.Milliseconds())
			info.Status = uint32(runtimeConf.Status)
		}

		res.Data = append(res.Data, info)
	}
	return
}

func (s *Server) AddPushProxy(ctx context.Context, req *pb.PushProxyInfo) (res *pb.SuccessResponse, err error) {
	device := &PushProxyConfig{
		Name:        req.Name,
		Type:        req.Type,
		ParentID:    uint(req.ParentID),
		PushOnStart: req.PushOnStart,
		Description: req.Description,
		StreamPath:  req.StreamPath,
	}

	if device.Type == "" {
		var u *url.URL
		u, err = url.Parse(req.PushURL)
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

	defaults.SetDefaults(&device.Push)
	if device.PushOnStart {
		device.Push.MaxRetry = -1
	}
	device.URL = req.PushURL
	device.Audio = req.Audio
	if s.DB == nil {
		err = pkg.ErrNoDB
		return
	}
	s.DB.Create(device)
	_, err = s.createPushProxy(device)
	res = &pb.SuccessResponse{}
	return
}

func (s *Server) UpdatePushProxy(ctx context.Context, req *pb.UpdatePushProxyRequest) (res *pb.SuccessResponse, err error) {
	if s.DB == nil {
		err = pkg.ErrNoDB
		return
	}
	target := &PushProxyConfig{}
	err = s.DB.First(target, req.ID).Error
	if err != nil {
		return
	}

	// Update only if the field is provided (optional fields)
	if req.Name != nil {
		target.Name = *req.Name
	}
	if req.PushURL != nil {
		target.URL = *req.PushURL
	}
	if req.ParentID != nil {
		target.ParentID = uint(*req.ParentID)
	}
	if req.Type != nil {
		target.Type = *req.Type
	}
	if target.Type == "" {
		var u *url.URL
		if req.PushURL != nil {
			u, err = url.Parse(*req.PushURL)
			if err != nil {
				s.Error("parse push url failed", "error", err)
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
	}
	if req.PushOnStart != nil {
		target.PushOnStart = *req.PushOnStart
	}
	if req.Audio != nil {
		target.Audio = *req.Audio
	}
	if req.Description != nil {
		target.Description = *req.Description
	}
	if req.StreamPath != nil {
		target.StreamPath = *req.StreamPath
	}
	// 如果设置状态为非 disable，需要检查是否有相同 streamPath 的其他非 disable 代理
	if req.Status != nil && *req.Status != uint32(PushProxyStatusDisabled) {
		var existingCount int64
		streamPath := target.StreamPath
		if streamPath == "" {
			streamPath = target.GetStreamPath()
		}
		s.DB.Model(&PushProxyConfig{}).Where("stream_path = ? AND id != ? AND status != ?", streamPath, req.ID, PushProxyStatusDisabled).Count(&existingCount)

		// 如果存在相同 streamPath 且状态不是 disabled 的其他记录，更新失败
		if existingCount > 0 {
			err = fmt.Errorf("已存在相同 streamPath [%s] 的非禁用代理，更新失败", streamPath)
			return
		}
		target.Status = byte(*req.Status)
	} else if req.Status != nil {
		target.Status = PushProxyStatusDisabled
	}

	s.DB.Save(target)

	// 检查是否从 disable 状态变为非 disable 状态
	isNowDisabled := target.Status == PushProxyStatusDisabled

	// 检查是否需要停止和重新创建代理

	if device, ok := s.PushProxies.Get(uint(req.ID)); ok {
		conf := device.GetConfig()
		originalStatus := conf.Status
		wasEnabled := originalStatus != PushProxyStatusDisabled

		// 如果现在变为 disable 状态，需要停止并移除代理
		if wasEnabled && isNowDisabled {
			device.Stop(task.ErrStopByUser)
			return
		}

		// 如果URL或音频设置或流路径发生变化，需要重新创建代理
		if target.URL != conf.URL || conf.Audio != target.Audio || conf.StreamPath != target.StreamPath {
			device.Stop(task.ErrStopByUser)
			device, err = s.createPushProxy(target)
			if err != nil {
				s.Error("create push proxy failed", "error", err)
				return
			}
			// 如果原来状态是推送中，并且PushOnStart为true，则重新开始推送
			if originalStatus == PushProxyStatusPushing && target.PushOnStart {
				device.Push()
			}
		} else {
			// 只更新配置，不重新创建代理
			conf.Name = target.Name
			conf.PushOnStart = target.PushOnStart
			conf.Description = target.Description

			// 如果PushOnStart为true且当前状态为在线，则开始推送
			if conf.PushOnStart && conf.Status == PushProxyStatusOnline {
				device.Push()
			}
		}
	} else {
		// 如果代理不存在，则创建新代理
		_, err = s.createPushProxy(target)
		if err != nil {
			s.Error("create push proxy failed", "error", err)
		}
	}

	res = &pb.SuccessResponse{}
	return
}

func (s *Server) RemovePushProxy(ctx context.Context, req *pb.RequestWithId) (res *pb.SuccessResponse, err error) {
	if s.DB == nil {
		err = pkg.ErrNoDB
		return
	}
	res = &pb.SuccessResponse{}
	if req.Id > 0 {
		tx := s.DB.Delete(&PushProxyConfig{
			ID: uint(req.Id),
		})
		err = tx.Error
		if device, ok := s.PushProxies.Get(uint(req.Id)); ok {
			device.Stop(task.ErrStopByUser)
		}
		return
	} else if req.StreamPath != "" {
		var deviceList []*PushProxyConfig
		s.DB.Find(&deviceList, "stream_path=?", req.StreamPath)
		if len(deviceList) > 0 {
			for _, device := range deviceList {
				tx := s.DB.Delete(device)
				err = tx.Error
				s.PushProxies.Call(func() {
					if device, ok := s.PushProxies.Get(uint(device.ID)); ok {
						device.Stop(task.ErrStopByUser)
					}
				})
			}
		}
		return
	} else {
		res.Message = "parameter wrong"
		return
	}
}
