package m7s

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"regexp"
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
		task.ITask
		GetBase() *BasePullProxy
		GetStreamPath() string
		GetConfig() *PullProxyConfig
		ChangeStatus(status byte)
		GetPullJob() *PullJob
		Pull()
		GetKey() uint
	}
	PullProxyConfig struct {
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
	}
	PullProxyFactory = func() IPullProxy
	PullProxyManager struct {
		task.WorkCollection[uint, IPullProxy]
	}
	BasePullProxy struct {
		*PullProxyConfig
		Plugin  *Plugin
		PullJob *PullJob
	}
	HTTPPullProxy struct {
		TCPPullProxy
	}
	TCPPullProxy struct {
		task.AsyncTickTask
		BasePullProxy
		TCPAddr *net.TCPAddr
		URL     *url.URL
	}
)

func (b *BasePullProxy) GetBase() *BasePullProxy {
	return b
}

func NewHTTPPullPorxy() IPullProxy {
	return &HTTPPullProxy{}
}

func (d *PullProxyConfig) GetKey() uint {
	return d.ID
}

func (d *PullProxyConfig) GetConfig() *PullProxyConfig {
	return d
}

func (d *PullProxyConfig) GetStreamPath() string {
	if d.StreamPath == "" {
		return fmt.Sprintf("pull/%s/%d", d.Type, d.ID)
	}
	return d.StreamPath
}

func (d *BasePullProxy) ChangeStatus(status byte) {
	if d.Status == status {
		return
	}
	from := d.Status
	d.Plugin.Info("device status changed", "from", from, "to", status)
	d.Status = status
	switch status {
	case PullProxyStatusOnline:
		if d.PullOnStart && (from == PullProxyStatusOffline) {
			d.Pull()
		}
	}
}

func (d *BasePullProxy) Dispose() {
	d.ChangeStatus(PullProxyStatusOffline)
	if d.PullJob != nil {
		d.PullJob.Stop(task.ErrStopByUser)
	}
}

func (d *PullProxyConfig) InitializeWithServer(s *Server) {
	if d.Type == "" {
		u, err := url.Parse(d.URL)
		if err != nil {
			s.Error("parse pull url failed", "error", err)
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

func (d *BasePullProxy) GetPullJob() *PullJob {
	return d.PullJob
}

func (d *BasePullProxy) Pull() {
	var pubConf = d.Plugin.config.Publish
	pubConf.PubAudio = d.Audio
	pubConf.DelayCloseTimeout = util.Conditional(d.StopOnIdle, time.Second*5, 0)
	d.PullJob, _ = d.Plugin.handler.Pull(d.GetStreamPath(), d.PullProxyConfig.Pull, &pubConf)
}

func (d *HTTPPullProxy) Start() (err error) {
	d.URL, err = url.Parse(d.PullProxyConfig.URL)
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
	return d.TCPPullProxy.Start()
}

func (d *TCPPullProxy) GetTickInterval() time.Duration {
	return time.Second * 10
}

func (d *TCPPullProxy) Tick(any) {
	switch d.Status {
	case PullProxyStatusOffline:
		startTime := time.Now()
		conn, err := net.DialTCP("tcp", nil, d.TCPAddr)
		if err != nil {
			d.ChangeStatus(PullProxyStatusOffline)
			return
		}
		conn.Close()
		d.RTT = time.Since(startTime)
		d.ChangeStatus(PullProxyStatusOnline)
	}
}

func (p *Publisher) processPullProxyOnStart() {
	s := p.Plugin.Server
	if pullProxy, ok := s.PullProxies.Find(func(pullProxy IPullProxy) bool {
		return pullProxy.GetStreamPath() == p.StreamPath
	}); ok {
		p.PullProxyConfig = pullProxy.GetConfig()
		if p.PullProxyConfig.Status == PullProxyStatusOnline {
			pullProxy.ChangeStatus(PullProxyStatusPulling)
			if mp4Plugin, ok := s.Plugins.Get("MP4"); ok && p.PullProxyConfig.FilePath != "" {
				mp4Plugin.Record(p, p.PullProxyConfig.Record, nil)
			}
		}
	}
}

func (p *Publisher) processPullProxyOnDispose() {
	s := p.Plugin.Server
	if p.PullProxyConfig != nil && p.PullProxyConfig.Status == PullProxyStatusPulling {
		if pullproxy, ok := s.PullProxies.Get(p.PullProxyConfig.GetKey()); ok {
			pullproxy.ChangeStatus(PullProxyStatusOnline)
		}
	}
}

func (s *Server) createPullProxy(conf *PullProxyConfig) (pullProxy IPullProxy, err error) {
	var plugin *Plugin
	switch conf.Type {
	case "h265", "h264":
		if s.Meta.NewPullProxy != nil {
			plugin = &s.Plugin
		}
	default:
		for p := range s.Plugins.Range {
			if p.Meta.NewPullProxy != nil && strings.EqualFold(conf.Type, p.Meta.Name) {
				plugin = p
				break
			}
		}
	}
	if plugin == nil {
		return
	}
	pullProxy = plugin.Meta.NewPullProxy()
	base := pullProxy.GetBase()
	base.PullProxyConfig = conf
	base.Plugin = plugin
	s.PullProxies.AddTask(pullProxy, plugin.Logger.With("pullProxyId", conf.ID, "pullProxyType", conf.Type, "pullProxyName", conf.Name))
	return
}

func (s *Server) GetPullProxyList(ctx context.Context, req *emptypb.Empty) (res *pb.PullProxyListResponse, err error) {
	res = &pb.PullProxyListResponse{}

	if s.DB == nil {
		err = pkg.ErrNoDB
		return
	}

	var pullProxyConfigs []PullProxyConfig
	err = s.DB.Find(&pullProxyConfigs).Error
	if err != nil {
		return
	}

	for _, conf := range pullProxyConfigs {
		// 获取运行时状态信息（如果需要的话）
		info := &pb.PullProxyInfo{
			Name:           conf.Name,
			CreateTime:     timestamppb.New(conf.CreatedAt),
			UpdateTime:     timestamppb.New(conf.UpdatedAt),
			Type:           conf.Type,
			PullURL:        conf.URL,
			ParentID:       uint32(conf.ParentID),
			Status:         uint32(conf.Status),
			ID:             uint32(conf.ID),
			PullOnStart:    conf.PullOnStart,
			StopOnIdle:     conf.StopOnIdle,
			Audio:          conf.Audio,
			RecordPath:     conf.Record.FilePath,
			RecordFragment: durationpb.New(conf.Record.Fragment),
			Description:    conf.Description,
			StreamPath:     conf.GetStreamPath(),
		}
		// 如果内存中有对应的设备，获取实时状态
		if device, ok := s.PullProxies.Get(conf.ID); ok {
			runtimeConf := device.GetConfig()
			info.Rtt = uint32(runtimeConf.RTT.Milliseconds())
			info.Status = uint32(runtimeConf.Status)
		}

		res.Data = append(res.Data, info)
	}
	return
}

func (s *Server) AddPullProxy(ctx context.Context, req *pb.PullProxyInfo) (res *pb.SuccessResponse, err error) {
	pullProxyConfig := &PullProxyConfig{
		Name:        req.Name,
		Type:        req.Type,
		ParentID:    uint(req.ParentID),
		PullOnStart: req.PullOnStart,
		Description: req.Description,
		StreamPath:  req.StreamPath,
	}
	if pullProxyConfig.Type == "" {
		var u *url.URL
		u, err = url.Parse(req.PullURL)
		if err != nil {
			s.Error("parse pull url failed", "error", err)
			return
		}
		switch u.Scheme {
		case "srt", "rtsp", "rtmp":
			pullProxyConfig.Type = u.Scheme
		default:
			ext := filepath.Ext(u.Path)
			switch ext {
			case ".m3u8":
				pullProxyConfig.Type = "hls"
			case ".flv":
				pullProxyConfig.Type = "flv"
			case ".mp4":
				pullProxyConfig.Type = "mp4"
			default:
				pattern := `^\d{20}/\d{20}$`
				re := regexp.MustCompile(pattern)
				if re.MatchString(u.Path) {
					pullProxyConfig.Type = "gb28181"
				}
			}
		}
	}
	defaults.SetDefaults(&pullProxyConfig.Pull)
	defaults.SetDefaults(&pullProxyConfig.Record)
	if pullProxyConfig.PullOnStart {
		pullProxyConfig.Pull.MaxRetry = -1
	}
	pullProxyConfig.URL = req.PullURL
	pullProxyConfig.Audio = req.Audio
	pullProxyConfig.StopOnIdle = req.StopOnIdle
	pullProxyConfig.Record.FilePath = req.RecordPath
	pullProxyConfig.Record.Fragment = req.RecordFragment.AsDuration()
	if s.DB == nil {
		err = pkg.ErrNoDB
		return
	}

	// 检查数据库中是否有相同的 streamPath 且状态不是 disabled 的记录
	var existingCount int64
	streamPath := pullProxyConfig.StreamPath
	if streamPath == "" {
		streamPath = pullProxyConfig.GetStreamPath()
	}
	s.DB.Model(&PullProxyConfig{}).Where("stream_path = ? AND status != ?", streamPath, PullProxyStatusDisabled).Count(&existingCount)

	// 如果存在相同 streamPath 且状态不是 disabled 的记录，将当前记录状态设置为 disabled
	if existingCount > 0 {
		pullProxyConfig.Status = PullProxyStatusDisabled
	}

	s.DB.Create(pullProxyConfig)
	if req.StreamPath == "" {
		pullProxyConfig.StreamPath = pullProxyConfig.GetStreamPath()
	}
	_, err = s.createPullProxy(pullProxyConfig)

	res = &pb.SuccessResponse{}
	return
}

func (s *Server) UpdatePullProxy(ctx context.Context, req *pb.UpdatePullProxyRequest) (res *pb.SuccessResponse, err error) {
	if s.DB == nil {
		err = pkg.ErrNoDB
		return
	}
	target := &PullProxyConfig{}
	err = s.DB.First(target, req.ID).Error
	if err != nil {
		return
	}

	// 只有当字段有值时才进行赋值
	if req.Name != nil {
		target.Name = *req.Name
	}
	if req.PullURL != nil {
		target.URL = *req.PullURL
	}
	if req.ParentID != nil {
		target.ParentID = uint(*req.ParentID)
	}
	if req.Type != nil {
		target.Type = *req.Type
	}

	// 如果Type字段被更新或者原来为空，则尝试从URL推导类型
	if (req.Type != nil && *req.Type == "") || (req.PullURL != nil && target.Type == "") {
		var urlToParse string
		if req.PullURL != nil {
			urlToParse = *req.PullURL
		} else {
			urlToParse = target.URL
		}

		if urlToParse != "" {
			var u *url.URL
			u, err = url.Parse(urlToParse)
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
	}

	if req.PullOnStart != nil {
		target.PullOnStart = *req.PullOnStart
	}
	if target.PullOnStart {
		target.Pull.MaxRetry = -1
	} else {
		target.Pull.MaxRetry = 0
	}
	if req.StopOnIdle != nil {
		target.StopOnIdle = *req.StopOnIdle
	}
	if req.Audio != nil {
		target.Audio = *req.Audio
	}
	if req.Description != nil {
		target.Description = *req.Description
	}
	if req.RecordPath != nil {
		target.Record.FilePath = *req.RecordPath
	}
	if req.RecordFragment != nil {
		target.Record.Fragment = req.RecordFragment.AsDuration()
	}
	if req.StreamPath != nil {
		target.StreamPath = *req.StreamPath
	}
	// 如果设置状态为非 disable，需要检查是否有相同 streamPath 的其他非 disable 代理
	if req.Status != nil && *req.Status != uint32(PullProxyStatusDisabled) {
		var existingCount int64
		streamPath := target.StreamPath
		if streamPath == "" {
			streamPath = target.GetStreamPath()
		}
		s.DB.Model(&PullProxyConfig{}).Where("stream_path = ? AND id != ? AND status != ?", streamPath, req.ID, PullProxyStatusDisabled).Count(&existingCount)

		// 如果存在相同 streamPath 且状态不是 disabled 的其他记录，更新失败
		if existingCount > 0 {
			err = fmt.Errorf("已存在相同 streamPath [%s] 的非禁用代理，更新失败", streamPath)
			return
		}
		target.Status = byte(*req.Status)
	} else if req.Status != nil {
		target.Status = PullProxyStatusDisabled
	}

	s.DB.Save(target)

	// 检查是否从 disable 状态变为非 disable 状态
	isNowDisabled := target.Status == PullProxyStatusDisabled

	if device, ok := s.PullProxies.Get(uint(req.ID)); ok {
		conf := device.GetConfig()
		originalStatus := conf.Status
		wasEnabled := originalStatus != PullProxyStatusDisabled
		// 如果现在变为 disable 状态，需要停止并移除代理
		if wasEnabled && isNowDisabled {
			device.Stop(task.ErrStopByUser)
			return
		}

		if target.URL != conf.URL || conf.Audio != target.Audio || conf.StreamPath != target.StreamPath || conf.Record.FilePath != target.Record.FilePath || conf.Record.Fragment != target.Record.Fragment {
			device.Stop(task.ErrStopByUser)
			device.WaitStopped()
			device, err = s.createPullProxy(target)
			device.WaitStarted()
			if originalStatus == PullProxyStatusPulling {
				device.Pull()
			}
		} else {
			conf.Name = target.Name
			conf.PullOnStart = target.PullOnStart
			conf.StopOnIdle = target.StopOnIdle
			conf.Description = target.Description
			if conf.PullOnStart && conf.Status == PullProxyStatusOnline {
				device.Pull()
			} else if originalStatus == PullProxyStatusPulling {
				if pullJob := device.GetPullJob(); pullJob != nil {
					pullJob.PublishConfig.DelayCloseTimeout = util.Conditional(target.StopOnIdle, time.Second*5, 0)
					if pullJob.Publisher != nil {
						pullJob.Publisher.Publish.DelayCloseTimeout = pullJob.PublishConfig.DelayCloseTimeout
					}
				}
			}
		}
	} else if !isNowDisabled {
		// 尝试再次激活
		_, err = s.createPullProxy(target)
		if err != nil {
			s.Error("create pull proxy failed", "error", err)
		}
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
		tx := s.DB.Delete(&PullProxyConfig{
			ID: uint(req.Id),
		})
		err = tx.Error
		if device, ok := s.PullProxies.Get(uint(req.Id)); ok {
			device.Stop(task.ErrStopByUser)
		}
		return
	} else if req.StreamPath != "" {
		var deviceList []*PullProxyConfig
		s.DB.Find(&deviceList, "stream_path=?", req.StreamPath)
		if len(deviceList) > 0 {
			for _, device := range deviceList {
				tx := s.DB.Delete(&PullProxyConfig{}, device.ID)
				err = tx.Error
				if device, ok := s.PullProxies.Get(uint(device.ID)); ok {
					device.Stop(task.ErrStopByUser)
				}
			}
		}
		return
	} else {
		res.Message = "parameter wrong"
		return
	}
}
