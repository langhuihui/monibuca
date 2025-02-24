package plugin_gb28181pro

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"net/url"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/types/known/timestamppb"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/util"
	"m7s.live/v5/plugin/gb28181pro/pb"
	gb28181 "m7s.live/v5/plugin/gb28181pro/pkg"
)

func (gb *GB28181ProPlugin) List(ctx context.Context, req *pb.GetDevicesRequest) (*pb.DevicesPageInfo, error) {
	resp := &pb.DevicesPageInfo{}

	if gb.DB == nil {
		resp.Code = 500
		resp.Message = "数据库未初始化"
		return resp, nil
	}

	var devices []Device
	var total int64

	// 构建查询条件
	query := gb.DB.Model(&Device{})
	if req.Query != "" {
		query = query.Where("device_id LIKE ? OR name LIKE ?",
			"%"+req.Query+"%", "%"+req.Query+"%")
	}
	if req.Status {
		query = query.Where("online = ?", true)
	}

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		resp.Code = 500
		resp.Message = fmt.Sprintf("查询总数失败: %v", err)
		return resp, nil
	}

	// 分页查询设备列表
	if err := query.
		Offset(int(req.Page-1) * int(req.Count)).
		Limit(int(req.Count)).
		Find(&devices).Error; err != nil {
		resp.Code = 500
		resp.Message = fmt.Sprintf("查询设备列表失败: %v", err)
		return resp, nil
	}

	// 转换为proto消息
	var pbDevices []*pb.Device
	for _, d := range devices {
		// 查询设备对应的通道
		var channels []gb28181.DeviceChannel
		if err := gb.DB.Where(&gb28181.DeviceChannel{DeviceDBID: d.ID}).Find(&channels).Error; err != nil {
			gb.Error("查询通道失败", "error", err)
			continue
		}

		var pbChannels []*pb.Channel
		for _, c := range channels {
			pbChannels = append(pbChannels, &pb.Channel{
				DeviceID:     c.DeviceID,
				ParentID:     c.ParentID,
				Name:         c.Name,
				Manufacturer: c.Manufacturer,
				Model:        c.Model,
				Owner:        c.Owner,
				CivilCode:    c.CivilCode,
				Address:      c.Address,
				Port:         int32(c.Port),
				Parental:     int32(c.Parental),
				SafetyWay:    int32(c.SafetyWay),
				RegisterWay:  int32(c.RegisterWay),
				Secrecy:      int32(c.Secrecy),
				Status:       string(c.Status),
				Longitude:    fmt.Sprintf("%f", c.GbLongitude),
				Latitude:     fmt.Sprintf("%f", c.GbLatitude),
				GpsTime:      timestamppb.New(time.Now()),
			})
		}

		pbDevices = append(pbDevices, &pb.Device{
			Id:           d.DeviceID,
			Name:         d.Name,
			Manufacturer: d.Manufacturer,
			Model:        d.Model,
			Owner:        d.Owner,
			Status:       string(d.Status),
			Online:       d.Online,
			Longitude:    d.Longitude,
			Latitude:     d.Latitude,
			GpsTime:      timestamppb.New(d.GpsTime),
			RegisterTime: timestamppb.New(d.RegisterTime),
			UpdateTime:   timestamppb.New(d.UpdateTime),
			Channels:     pbChannels,
		})
	}

	resp.Code = 0
	resp.Message = "success"
	resp.Total = int32(total)
	resp.List = pbDevices

	return resp, nil
}

func (gb *GB28181ProPlugin) api_ps_replay(w http.ResponseWriter, r *http.Request) {
	dump := r.URL.Query().Get("dump")
	streamPath := r.PathValue("streamPath")
	if dump == "" {
		dump = "dump/ps"
	}
	if streamPath == "" {
		if strings.HasPrefix(dump, "/") {
			streamPath = "replay" + dump
		} else {
			streamPath = "replay/" + dump
		}
	}
	var puller gb28181.DumpPuller
	puller.GetPullJob().Init(&puller, &gb.Plugin, streamPath, config.Pull{
		URL: dump,
	}, nil)
}

// GetDevice 实现获取单个设备信息
func (gb *GB28181ProPlugin) GetDevice(ctx context.Context, req *pb.GetDeviceRequest) (*pb.DeviceResponse, error) {
	resp := &pb.DeviceResponse{}

	// 先从内存中获取
	d, ok := gb.devices.Get(req.DeviceId)
	if !ok && gb.DB != nil {
		// 如果内存中没有且数据库存在，则从数据库查询
		var device Device
		if err := gb.DB.Where("id = ?", req.DeviceId).First(&device).Error; err == nil {
			d = &device
		}
	}

	if d != nil {
		var channels []*pb.Channel
		for c := range d.channels.Range {
			channels = append(channels, &pb.Channel{
				DeviceID:     c.DeviceID,
				ParentID:     c.ParentID,
				Name:         c.Name,
				Manufacturer: c.Manufacturer,
				Model:        c.Model,
				Owner:        c.Owner,
				CivilCode:    c.CivilCode,
				Address:      c.Address,
				Port:         int32(c.Port),
				Parental:     int32(c.Parental),
				SafetyWay:    int32(c.SafetyWay),
				RegisterWay:  int32(c.RegisterWay),
				Secrecy:      int32(c.Secrecy),
				Status:       string(c.Status),
				Longitude:    fmt.Sprintf("%f", c.GbLongitude),
				Latitude:     fmt.Sprintf("%f", c.GbLatitude),
				GpsTime:      timestamppb.New(time.Now()),
			})
		}
		resp.Data = &pb.Device{
			Id:           d.DeviceID,
			Name:         d.Name,
			Manufacturer: d.Manufacturer,
			Model:        d.Model,
			Owner:        d.Owner,
			Status:       string(d.Status),
			Online:       d.Online,
			Longitude:    d.Longitude,
			Latitude:     d.Latitude,
			GpsTime:      timestamppb.New(d.GpsTime),
			RegisterTime: timestamppb.New(d.RegisterTime),
			UpdateTime:   timestamppb.New(d.UpdateTime),
			Channels:     channels,
		}
		resp.Code = 0
		resp.Message = "success"
	} else {
		resp.Code = 404
		resp.Message = "device not found"
	}
	return resp, nil
}

// GetDevices 实现分页查询设备列表
func (gb *GB28181ProPlugin) GetDevices(ctx context.Context, req *pb.GetDevicesRequest) (*pb.DevicesPageInfo, error) {
	resp := &pb.DevicesPageInfo{}

	if gb.DB == nil {
		resp.Code = 500
		resp.Message = "数据库未初始化"
		return resp, nil
	}

	var devices []Device
	var total int64

	// 构建查询条件
	query := gb.DB.Model(&Device{})
	if req.Query != "" {
		query = query.Where("device_id LIKE ? OR name LIKE ?",
			"%"+req.Query+"%", "%"+req.Query+"%")
	}
	if req.Status {
		query = query.Where("online = ?", true)
	}

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		resp.Code = 500
		resp.Message = fmt.Sprintf("查询总数失败: %v", err)
		return resp, nil
	}

	// 分页查询设备，并预加载通道数据
	if err := query.
		Offset(int(req.Page-1) * int(req.Count)).
		Limit(int(req.Count)).
		Find(&devices).Error; err != nil {
		resp.Code = 500
		resp.Message = fmt.Sprintf("查询设备列表失败: %v", err)
		return resp, nil
	}

	// 转换为proto消息
	var pbDevices []*pb.Device
	for _, d := range devices {
		// 查询设备对应的通道
		var channels []gb28181.DeviceChannel
		if err := gb.DB.Where(&gb28181.DeviceChannel{DeviceDBID: d.ID}).Find(&channels).Error; err != nil {
			gb.Error("查询通道失败", "error", err)
			continue
		}

		var pbChannels []*pb.Channel
		for _, c := range channels {
			pbChannels = append(pbChannels, &pb.Channel{
				DeviceID:     c.DeviceID,
				ParentID:     c.ParentID,
				Name:         c.Name,
				Manufacturer: c.Manufacturer,
				Model:        c.Model,
				Owner:        c.Owner,
				CivilCode:    c.CivilCode,
				Address:      c.Address,
				Port:         int32(c.Port),
				Parental:     int32(c.Parental),
				SafetyWay:    int32(c.SafetyWay),
				RegisterWay:  int32(c.RegisterWay),
				Secrecy:      int32(c.Secrecy),
				Status:       string(c.Status),
				Longitude:    fmt.Sprintf("%f", c.GbLongitude),
				Latitude:     fmt.Sprintf("%f", c.GbLatitude),
				GpsTime:      timestamppb.New(time.Now()),
			})
		}

		pbDevice := &pb.Device{
			Id:           d.DeviceID,
			Name:         d.Name,
			Manufacturer: d.Manufacturer,
			Model:        d.Model,
			Owner:        d.Owner,
			Status:       string(d.Status),
			Online:       d.Online,
			Longitude:    d.Longitude,
			Latitude:     d.Latitude,
			GpsTime:      timestamppb.New(d.GpsTime),
			RegisterTime: timestamppb.New(d.RegisterTime),
			UpdateTime:   timestamppb.New(d.UpdateTime),
			Channels:     pbChannels,
		}
		pbDevices = append(pbDevices, pbDevice)
	}

	resp.Total = int32(total)
	resp.List = pbDevices
	resp.Code = 0
	resp.Message = "success"
	return resp, nil
}

// GetChannels 实现分页查询通道
func (gb *GB28181ProPlugin) GetChannels(ctx context.Context, req *pb.GetChannelsRequest) (*pb.ChannelsPageInfo, error) {
	resp := &pb.ChannelsPageInfo{}

	// 先从内存中获取
	d, ok := gb.devices.Get(req.DeviceId)
	if !ok && gb.DB != nil {
		// 如果内存中没有且数据库存在，则从数据库查询
		var device Device
		if err := gb.DB.Where(Device{DeviceID: req.DeviceId}).First(&device).Error; err == nil {
			d = &device
		}
	}

	if d != nil {
		var channels []*pb.Channel
		total := 0
		for c := range d.channels.Range {
			// TODO: 实现查询条件过滤
			if req.Query != "" && !strings.Contains(c.DeviceID, req.Query) && !strings.Contains(c.Name, req.Query) {
				continue
			}
			if req.Online && string(c.Status) != "ON" {
				continue
			}
			if req.ChannelType && c.ParentID == "" {
				continue
			}
			total++
			// 分页处理
			if total > int(req.Page*req.Count) {
				continue
			}
			if total <= int((req.Page-1)*req.Count) {
				continue
			}
			channels = append(channels, &pb.Channel{
				DeviceID:     c.DeviceID,
				ParentID:     c.ParentID,
				Name:         c.Name,
				Manufacturer: c.Manufacturer,
				Model:        c.Model,
				Owner:        c.Owner,
				CivilCode:    c.CivilCode,
				Address:      c.Address,
				Port:         int32(c.Port),
				Parental:     int32(c.Parental),
				SafetyWay:    int32(c.SafetyWay),
				RegisterWay:  int32(c.RegisterWay),
				Secrecy:      int32(c.Secrecy),
				Status:       string(c.Status),
				Longitude:    fmt.Sprintf("%f", c.GbLongitude),
				Latitude:     fmt.Sprintf("%f", c.GbLatitude),
				GpsTime:      timestamppb.New(time.Now()),
			})
		}
		resp.Total = int32(total)
		resp.List = channels
		resp.Code = 0
		resp.Message = "success"
	} else {
		resp.Code = 404
		resp.Message = "device not found"
	}
	return resp, nil
}

// SyncDevice 实现同步设备通道信息
func (gb *GB28181ProPlugin) SyncDevice(ctx context.Context, req *pb.SyncDeviceRequest) (*pb.SyncStatus, error) {
	resp := &pb.SyncStatus{
		Code:    404,
		Message: "device not found",
	}

	// 先从内存中获取设备
	d, ok := gb.devices.Get(req.DeviceId)
	if !ok && gb.DB != nil {
		// 如果内存中没有且数据库存在，则从数据库查询
		var device Device
		if err := gb.DB.Where("id = ?", req.DeviceId).First(&device).Error; err == nil {
			d = &device
			// 恢复设备的必要字段
			d.Logger = gb.With("id", req.DeviceId)
			d.channels.L = new(sync.RWMutex)
			d.plugin = gb

			// 初始化 Task
			var hash uint32
			for i := 0; i < len(d.DeviceID); i++ {
				ch := d.DeviceID[i]
				hash = hash*31 + uint32(ch)
			}
			d.Task.ID = hash
			d.Task.Logger = d.Logger
			d.Task.Context, d.Task.CancelCauseFunc = context.WithCancelCause(context.Background())

			// 初始化 SIP 相关字段
			d.fromHDR = sip.FromHeader{
				Address: sip.Uri{
					User: gb.Serial,
					Host: gb.Realm,
				},
				Params: sip.NewParams(),
			}
			d.fromHDR.Params.Add("tag", sip.GenerateTagN(16))

			d.contactHDR = sip.ContactHeader{
				Address: sip.Uri{
					User: gb.Serial,
					Host: d.LocalIP,
					Port: d.Port,
				},
			}

			d.Recipient = sip.Uri{
				Host: d.IP,
				Port: d.Port,
				User: d.DeviceID,
			}

			// 初始化 SIP 客户端
			d.client, _ = sipgo.NewClient(gb.ua, sipgo.WithClientLogger(zerolog.New(os.Stdout)), sipgo.WithClientHostname(d.LocalIP))
			if d.client != nil {
				d.dialogClient = sipgo.NewDialogClient(d.client, d.contactHDR)
			} else {
				return resp, fmt.Errorf("failed to create sip client")
			}

			// 将设备添加到内存中
			gb.devices.Add(d)
		}
	}

	if d != nil {
		// 发送目录查询请求
		_, err := d.catalog()
		if err != nil {
			resp.Code = 500
			resp.Message = "catalog request failed"
			resp.ErrorMsg = err.Error()
		} else {
			resp.Code = 0
			resp.Message = "sync request sent"
			resp.Total = int32(d.ChannelCount)
			resp.Current = 0 // 初始化进度为0
		}
	}

	return resp, nil
}

// StartPlay 处理 StartPlay 请求，用于播放视频流
func (gb *GB28181ProPlugin) StartPlay(ctx context.Context, req *pb.PlayRequest) (*pb.PlayResponse, error) {
	resp := &pb.PlayResponse{}
	gb.Info("StartPlay request", "deviceId", req.DeviceId, "channelId", req.ChannelId)

	// 先从内存中获取设备
	device, ok := gb.devices.Get(req.DeviceId)
	if !ok && gb.DB != nil {
		// 如果内存中没有且数据库存在，则从数据库查询
		var dbDevice Device
		if err := gb.DB.Where("device_id = ?", req.DeviceId).First(&dbDevice).Error; err == nil {
			// 恢复设备的必要字段
			dbDevice.Logger = gb.With("id", req.DeviceId)
			dbDevice.channels.L = new(sync.RWMutex)
			dbDevice.plugin = gb
			device = &dbDevice
		} else {
			gb.Error("StartPlay failed", "error", "device not found", "deviceId", req.DeviceId)
			resp.Code = 404
			resp.Message = "device not found"
			return resp, nil
		}
	}

	// 先从内存中获取通道
	channel, ok := device.channels.Get(req.ChannelId)
	if !ok && gb.DB != nil {
		// 如果内存中没有且数据库存在，则从数据库查询
		var dbChannel gb28181.DeviceChannel
		if err := gb.DB.Where("device_id = ? AND device_db_id = ?", req.ChannelId, device.ID).First(&dbChannel).Error; err == nil {
			channel = &Channel{
				Device:        device,
				Logger:        device.Logger.With("channel", req.ChannelId),
				DeviceChannel: dbChannel,
			}
			device.channels.Set(channel)
		} else {
			gb.Error("StartPlay failed", "error", "channel not found", "channelId", req.ChannelId)
			resp.Code = 404
			resp.Message = "channel not found"
			return resp, nil
		}
	}

	// 构建流路径
	streamPath := fmt.Sprintf("%s/%s", req.DeviceId, req.ChannelId)

	// 调用 Pull 方法开始拉流
	gb.Pull(streamPath, config.Pull{
		MaxRetry: 0,
		URL:      streamPath,
	}, nil)

	// 设置响应信息
	resp.Code = 0
	resp.Message = "success"
	resp.StreamInfo = &pb.StreamInfo{
		Stream: streamPath,
		App:    "gb28181",
		Ip:     device.IP,
	}

	gb.Info("StartPlay success", "deviceId", req.DeviceId, "channelId", req.ChannelId)
	return resp, nil
}

// StartPlayback 处理回放请求
func (gb *GB28181ProPlugin) StartPlayback(ctx context.Context, req *pb.PlaybackRequest) (*pb.PlayResponse, error) {
	resp := &pb.PlayResponse{}
	gb.Info("StartPlayback request", "deviceId", req.DeviceId, "channelId", req.ChannelId, "start", req.Start, "end", req.End, "range", req.Range)

	// 先从内存中获取设备
	device, ok := gb.devices.Get(req.DeviceId)
	if !ok && gb.DB != nil {
		// 如果内存中没有且数据库存在，则从数据库查询
		var dbDevice Device
		if err := gb.DB.Where("device_id = ?", req.DeviceId).First(&dbDevice).Error; err == nil {
			// 恢复设备的必要字段
			dbDevice.Logger = gb.With("id", req.DeviceId)
			dbDevice.channels.L = new(sync.RWMutex)
			dbDevice.plugin = gb
			device = &dbDevice
		} else {
			gb.Error("StartPlayback failed", "error", "device not found", "deviceId", req.DeviceId)
			resp.Code = 404
			resp.Message = "device not found"
			return resp, nil
		}
	}

	// 先从内存中获取通道
	channel, ok := device.channels.Get(req.ChannelId)
	if !ok && gb.DB != nil {
		// 如果内存中没有且数据库存在，则从数据库查询
		var dbChannel gb28181.DeviceChannel
		if err := gb.DB.Where("device_id = ? AND device_db_id = ?", req.ChannelId, device.ID).First(&dbChannel).Error; err == nil {
			channel = &Channel{
				Device:        device,
				Logger:        device.Logger.With("channel", req.ChannelId),
				DeviceChannel: dbChannel,
			}
			device.channels.Set(channel)
		} else {
			gb.Error("StartPlayback failed", "error", "channel not found", "channelId", req.ChannelId)
			resp.Code = 404
			resp.Message = "channel not found"
			return resp, nil
		}
	}

	// 处理时间范围
	startTime, endTime, err := util.TimeRangeQueryParse(url.Values{
		"range": []string{req.Range},
		"start": []string{req.Start},
		"end":   []string{req.End},
	})
	if err != nil {
		gb.Error("StartPlayback failed", "error", "invalid time format", "err", err)
		resp.Code = 400
		resp.Message = fmt.Sprintf("invalid time format: %v", err)
		return resp, nil
	}

	// 构建流路径，加入时间信息以区分不同的回放请求
	streamPath := fmt.Sprintf("%s/%s/playback_%s_%s", req.DeviceId, req.ChannelId,
		startTime.Format("20060102150405"), endTime.Format("20060102150405"))

	// 调用 Pull 方法开始拉流
	gb.Pull(streamPath, config.Pull{
		URL: streamPath,
		Args: config.HTTPValues{
			"start": []string{startTime.Format(time.RFC3339)},
			"end":   []string{endTime.Format(time.RFC3339)},
		},
	}, nil)

	// 设置响应信息
	resp.Code = 0
	resp.Message = "success"
	resp.StreamInfo = &pb.StreamInfo{
		Stream: streamPath,
		App:    "gb28181",
		Ip:     device.IP,
	}

	gb.Info("StartPlayback success", "deviceId", req.DeviceId, "channelId", req.ChannelId,
		"start", startTime.Format(time.RFC3339), "end", endTime.Format(time.RFC3339))
	return resp, nil
}

// AddPlatform 实现添加平台信息
func (gb *GB28181ProPlugin) AddPlatform(ctx context.Context, req *pb.Platform) (*pb.BaseResponse, error) {
	resp := &pb.BaseResponse{}

	if gb.DB == nil {
		resp.Code = 500
		resp.Message = "database not initialized"
		return resp, nil
	}

	// 必填字段校验
	if req.Name == "" {
		resp.Code = 400
		resp.Message = "平台名称不可为空"
		return resp, nil
	}
	if req.ServerGBId == "" {
		resp.Code = 400
		resp.Message = "上级平台国标编号不可为空"
		return resp, nil
	}
	if req.ServerIp == "" {
		resp.Code = 400
		resp.Message = "上级平台IP不可为空"
		return resp, nil
	}
	if req.ServerPort <= 0 || req.ServerPort > 65535 {
		resp.Code = 400
		resp.Message = "上级平台端口异常"
		return resp, nil
	}
	if req.DeviceGBId == "" {
		resp.Code = 400
		resp.Message = "本平台国标编号不可为空"
		return resp, nil
	}

	// 检查平台是否已存在
	var existingPlatform gb28181.PlatformModel
	if err := gb.DB.Where("server_gb_id = ?", req.ServerGBId).First(&existingPlatform).Error; err == nil {
		resp.Code = 400
		resp.Message = fmt.Sprintf("平台 %s 已存在", req.ServerGBId)
		return resp, nil
	}

	// 设置默认值
	if req.ServerGBDomain == "" {
		req.ServerGBDomain = req.ServerGBId[:6] // 取前6位作为域
	}
	if req.Expires <= 0 {
		req.Expires = 3600 // 默认3600秒
	}
	if req.KeepTimeout <= 0 {
		req.KeepTimeout = 60 // 默认60秒
	}
	if req.Transport == "" {
		req.Transport = "UDP" // 默认UDP
	}
	if req.CharacterSet == "" {
		req.CharacterSet = "GB2312" // 默认GB2312
	}

	// 设置创建时间和更新时间
	currentTime := time.Now().Format("2006-01-02 15:04:05")
	req.CreateTime = currentTime
	req.UpdateTime = currentTime

	// 将proto消息转换为数据库模型
	platformModel := &gb28181.PlatformModel{
		Enable:                  req.Enable,
		Name:                    req.Name,
		ServerGBID:              req.ServerGBId,
		ServerGBDomain:          req.ServerGBDomain,
		ServerIP:                req.ServerIp,
		ServerPort:              int(req.ServerPort),
		DeviceGBID:              req.DeviceGBId,
		DeviceIP:                req.DeviceIp,
		DevicePort:              int(req.DevicePort),
		Username:                req.Username,
		Password:                req.Password,
		Expires:                 int(req.Expires),
		KeepTimeout:             int(req.KeepTimeout),
		Transport:               req.Transport,
		CharacterSet:            req.CharacterSet,
		PTZ:                     req.Ptz,
		RTCP:                    req.Rtcp,
		Status:                  req.Status,
		ChannelCount:            int(req.ChannelCount),
		CatalogSubscribe:        req.CatalogSubscribe,
		AlarmSubscribe:          req.AlarmSubscribe,
		MobilePositionSubscribe: req.MobilePositionSubscribe,
		CatalogGroup:            int(req.CatalogGroup),
		UpdateTime:              req.UpdateTime,
		CreateTime:              req.CreateTime,
		AsMessageChannel:        req.AsMessageChannel,
		SendStreamIP:            req.SendStreamIp,
		AutoPushChannel:         &req.AutoPushChannel,
		CatalogWithPlatform:     int(req.CatalogWithPlatform),
		CatalogWithGroup:        int(req.CatalogWithGroup),
		CatalogWithRegion:       int(req.CatalogWithRegion),
		CivilCode:               req.CivilCode,
		Manufacturer:            req.Manufacturer,
		Model:                   req.Model,
		Address:                 req.Address,
		RegisterWay:             int(req.RegisterWay),
		Secrecy:                 int(req.Secrecy),
	}

	// 保存到数据库
	if err := gb.DB.Create(platformModel).Error; err != nil {
		resp.Code = 500
		resp.Message = fmt.Sprintf("failed to create platform: %v", err)
		return resp, nil
	}

	// 如果平台启用，则创建Platform实例并启动任务
	if platformModel.Enable {
		// 创建Platform实例
		platform := &Platform{
			PlatformModel: platformModel,
			plugin:        gb,
		}

		// 添加到任务系统
		gb.AddTask(platform)
		gb.platforms.Set(platform)
	}

	resp.Code = 0
	resp.Message = "success"
	return resp, nil
}

// GetPlatform 实现获取平台信息
func (gb *GB28181ProPlugin) GetPlatform(ctx context.Context, req *pb.GetPlatformRequest) (*pb.PlatformResponse, error) {
	resp := &pb.PlatformResponse{}

	if gb.DB == nil {
		resp.Code = 500
		resp.Message = "database not initialized"
		return resp, nil
	}

	var platform gb28181.PlatformModel
	if err := gb.DB.First(&platform, req.Id).Error; err != nil {
		resp.Code = 404
		resp.Message = "platform not found"
		return resp, nil
	}

	// 将数据库模型转换为proto消息
	resp.Data = &pb.Platform{
		ID:                      platform.ID,
		Enable:                  platform.Enable,
		Name:                    platform.Name,
		ServerGBId:              platform.ServerGBID,
		ServerGBDomain:          platform.ServerGBDomain,
		ServerIp:                platform.ServerIP,
		ServerPort:              int32(platform.ServerPort),
		DeviceGBId:              platform.DeviceGBID,
		DeviceIp:                platform.DeviceIP,
		DevicePort:              int32(platform.DevicePort),
		Username:                platform.Username,
		Password:                platform.Password,
		Expires:                 int32(platform.Expires),
		KeepTimeout:             int32(platform.KeepTimeout),
		Transport:               platform.Transport,
		CharacterSet:            platform.CharacterSet,
		Ptz:                     platform.PTZ,
		Rtcp:                    platform.RTCP,
		Status:                  platform.Status,
		ChannelCount:            int32(platform.ChannelCount),
		CatalogSubscribe:        platform.CatalogSubscribe,
		AlarmSubscribe:          platform.AlarmSubscribe,
		MobilePositionSubscribe: platform.MobilePositionSubscribe,
		CatalogGroup:            int32(platform.CatalogGroup),
		UpdateTime:              platform.UpdateTime,
		CreateTime:              platform.CreateTime,
		AsMessageChannel:        platform.AsMessageChannel,
		SendStreamIp:            platform.SendStreamIP,
		AutoPushChannel:         platform.AutoPushChannel != nil && *platform.AutoPushChannel,
		CatalogWithPlatform:     int32(platform.CatalogWithPlatform),
		CatalogWithGroup:        int32(platform.CatalogWithGroup),
		CatalogWithRegion:       int32(platform.CatalogWithRegion),
		CivilCode:               platform.CivilCode,
		Manufacturer:            platform.Manufacturer,
		Model:                   platform.Model,
		Address:                 platform.Address,
		RegisterWay:             int32(platform.RegisterWay),
		Secrecy:                 int32(platform.Secrecy),
	}

	resp.Code = 0
	resp.Message = "success"
	return resp, nil
}

// UpdatePlatform 实现更新平台信息
func (gb *GB28181ProPlugin) UpdatePlatform(ctx context.Context, req *pb.Platform) (*pb.BaseResponse, error) {
	resp := &pb.BaseResponse{}

	if gb.DB == nil {
		resp.Code = 500
		resp.Message = "database not initialized"
		return resp, nil
	}

	// 检查平台是否存在
	var platform gb28181.PlatformModel
	if err := gb.DB.First(&platform, req.ID).Error; err != nil {
		resp.Code = 404
		resp.Message = "platform not found"
		return resp, nil
	}
	if runningPlatform, ok := gb.platforms.Get(req.ID); ok {
		runningPlatform.Stop(errors.New("stop running platform,platform.ServerGBID is " + platform.ServerGBID))
	}

	// 更新平台信息
	platform.Enable = req.Enable
	platform.Name = req.Name
	platform.ServerGBID = req.ServerGBId
	platform.ServerGBDomain = req.ServerGBDomain
	platform.ServerIP = req.ServerIp
	platform.ServerPort = int(req.ServerPort)
	platform.DeviceGBID = req.DeviceGBId
	platform.DeviceIP = req.DeviceIp
	platform.DevicePort = int(req.DevicePort)
	platform.Username = req.Username
	platform.Password = req.Password
	platform.Expires = int(req.Expires)
	platform.KeepTimeout = int(req.KeepTimeout)
	platform.Transport = req.Transport
	platform.CharacterSet = req.CharacterSet
	platform.PTZ = req.Ptz
	platform.RTCP = req.Rtcp
	platform.Status = req.Status
	platform.ChannelCount = int(req.ChannelCount)
	platform.CatalogSubscribe = req.CatalogSubscribe
	platform.AlarmSubscribe = req.AlarmSubscribe
	platform.MobilePositionSubscribe = req.MobilePositionSubscribe
	platform.CatalogGroup = int(req.CatalogGroup)
	platform.UpdateTime = req.UpdateTime
	platform.AsMessageChannel = req.AsMessageChannel
	platform.SendStreamIP = req.SendStreamIp
	platform.AutoPushChannel = &req.AutoPushChannel
	platform.CatalogWithPlatform = int(req.CatalogWithPlatform)
	platform.CatalogWithGroup = int(req.CatalogWithGroup)
	platform.CatalogWithRegion = int(req.CatalogWithRegion)
	platform.CivilCode = req.CivilCode
	platform.Manufacturer = req.Manufacturer
	platform.Model = req.Model
	platform.Address = req.Address
	platform.RegisterWay = int(req.RegisterWay)
	platform.Secrecy = int(req.Secrecy)

	if err := gb.DB.Save(&platform).Error; err != nil {
		resp.Code = 500
		resp.Message = fmt.Sprintf("failed to update platform: %v", err)
		return resp, nil
	}

	// 处理平台启用状态变化
	if platform.Enable {
		// 如果存在旧的platform实例，先停止并移除
		if oldPlatform, ok := gb.platforms.Get(platform.ID); ok {
			oldPlatform.Stop(fmt.Errorf("platform updated"))
			gb.platforms.Remove(oldPlatform)
		}

		// 创建新的Platform实例
		platformInstance := &Platform{
			PlatformModel: &platform,
			plugin:        gb,
		}

		// 添加到任务系统
		gb.AddTask(platformInstance)

		// 添加到platforms集合中
		gb.platforms.Add(platformInstance)
	} else {
		// 如果平台被禁用，停止并移除旧的platform实例
		if oldPlatform, ok := gb.platforms.Get(platform.ID); ok {
			oldPlatform.Stop(fmt.Errorf("platform disabled"))
			gb.platforms.Remove(oldPlatform)
		}
	}

	resp.Code = 0
	resp.Message = "success"
	return resp, nil
}

// DeletePlatform 实现删除平台信息
func (gb *GB28181ProPlugin) DeletePlatform(ctx context.Context, req *pb.DeletePlatformRequest) (*pb.BaseResponse, error) {
	resp := &pb.BaseResponse{}

	if gb.DB == nil {
		resp.Code = 500
		resp.Message = "database not initialized"
		return resp, nil
	}

	// 删除平台
	if err := gb.DB.Delete(&gb28181.PlatformModel{}, req.Id).Error; err != nil {
		resp.Code = 500
		resp.Message = fmt.Sprintf("failed to delete platform: %v", err)
		return resp, nil
	}

	resp.Code = 0
	resp.Message = "success"
	return resp, nil
}

// ListPlatforms 实现获取平台列表
func (gb *GB28181ProPlugin) ListPlatforms(ctx context.Context, req *pb.ListPlatformsRequest) (*pb.PlatformsPageInfo, error) {
	resp := &pb.PlatformsPageInfo{}

	if gb.DB == nil {
		resp.Code = 500
		resp.Message = "database not initialized"
		return resp, nil
	}

	var platforms []gb28181.PlatformModel
	var total int64

	// 构建查询条件
	query := gb.DB.Model(&gb28181.PlatformModel{})
	if req.Query != "" {
		query = query.Where("name LIKE ? OR server_gb_id LIKE ? OR device_gb_id LIKE ?",
			"%"+req.Query+"%", "%"+req.Query+"%", "%"+req.Query+"%")
	}
	if req.Status {
		query = query.Where("status = ?", true)
	}

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		resp.Code = 500
		resp.Message = fmt.Sprintf("failed to count platforms: %v", err)
		return resp, nil
	}

	// 分页查询
	if err := query.Offset(int(req.Page-1) * int(req.Count)).
		Limit(int(req.Count)).
		Find(&platforms).Error; err != nil {
		resp.Code = 500
		resp.Message = fmt.Sprintf("failed to list platforms: %v", err)
		return resp, nil
	}

	// 转换为proto消息
	var pbPlatforms []*pb.Platform
	for _, p := range platforms {
		pbPlatforms = append(pbPlatforms, &pb.Platform{
			ID:                      p.ID,
			Enable:                  p.Enable,
			Name:                    p.Name,
			ServerGBId:              p.ServerGBID,
			ServerGBDomain:          p.ServerGBDomain,
			ServerIp:                p.ServerIP,
			ServerPort:              int32(p.ServerPort),
			DeviceGBId:              p.DeviceGBID,
			DeviceIp:                p.DeviceIP,
			DevicePort:              int32(p.DevicePort),
			Username:                p.Username,
			Password:                p.Password,
			Expires:                 int32(p.Expires),
			KeepTimeout:             int32(p.KeepTimeout),
			Transport:               p.Transport,
			CharacterSet:            p.CharacterSet,
			Ptz:                     p.PTZ,
			Rtcp:                    p.RTCP,
			Status:                  p.Status,
			ChannelCount:            int32(p.ChannelCount),
			CatalogSubscribe:        p.CatalogSubscribe,
			AlarmSubscribe:          p.AlarmSubscribe,
			MobilePositionSubscribe: p.MobilePositionSubscribe,
			CatalogGroup:            int32(p.CatalogGroup),
			UpdateTime:              p.UpdateTime,
			CreateTime:              p.CreateTime,
			AsMessageChannel:        p.AsMessageChannel,
			SendStreamIp:            p.SendStreamIP,
			AutoPushChannel:         p.AutoPushChannel != nil && *p.AutoPushChannel,
			CatalogWithPlatform:     int32(p.CatalogWithPlatform),
			CatalogWithGroup:        int32(p.CatalogWithGroup),
			CatalogWithRegion:       int32(p.CatalogWithRegion),
			CivilCode:               p.CivilCode,
			Manufacturer:            p.Manufacturer,
			Model:                   p.Model,
			Address:                 p.Address,
			RegisterWay:             int32(p.RegisterWay),
			Secrecy:                 int32(p.Secrecy),
		})
	}

	resp.Total = int32(total)
	resp.List = pbPlatforms
	resp.Code = 0
	resp.Message = "success"
	return resp, nil
}

// QueryRecord 实现录像查询接口
func (gb *GB28181ProPlugin) QueryRecord(ctx context.Context, req *pb.QueryRecordRequest) (*pb.QueryRecordResponse, error) {
	resp := &pb.QueryRecordResponse{
		Code:    0,
		Message: "",
	}

	// 获取设备和通道
	device, ok := gb.devices.Get(req.DeviceId)
	if !ok {
		resp.Code = 404
		resp.Message = "device not found"
		return resp, nil
	}

	channel, ok := device.channels.Get(req.ChannelId)
	if !ok {
		resp.Code = 404
		resp.Message = "channel not found"
		return resp, nil
	}

	// 生成随机序列号
	sn := int(time.Now().UnixNano() / 1e6 % 1000000)

	// 发送录像查询请求
	promise, err := gb.RecordInfoQuery(req.DeviceId, req.ChannelId, req.Start, req.End, sn)
	if err != nil {
		resp.Code = 500
		resp.Message = err.Error()
		return resp, nil
	}

	// 等待响应
	err = promise.Await()
	if err != nil {
		resp.Code = 500
		resp.Message = fmt.Sprintf("query failed: %v", err)
		return resp, nil
	}

	// 获取录像请求
	recordReq, ok := channel.RecordReqs.Get(sn)
	if !ok {
		resp.Code = 500
		resp.Message = "record request not found"
		return resp, nil
	}

	// 转换结果
	if len(recordReq.Response) > 0 {
		firstResponse := recordReq.Response[0]
		resp.DeviceId = req.DeviceId
		resp.ChannelId = req.ChannelId
		resp.Name = firstResponse.Name
		resp.Count = int32(recordReq.ReceivedNum)
		if !firstResponse.LastTime.IsZero() {
			resp.LastTime = timestamppb.New(firstResponse.LastTime)
		}
	}

	for _, record := range recordReq.Response {
		for _, item := range record.RecordList.Item {
			resp.Records = append(resp.Records, &pb.RecordItem{
				DeviceId:   item.DeviceID,
				Name:       item.Name,
				FilePath:   item.FilePath,
				Address:    item.Address,
				StartTime:  item.StartTime,
				EndTime:    item.EndTime,
				Secrecy:    int32(item.Secrecy),
				Type:       item.Type,
				RecorderId: item.RecorderID,
			})
		}
	}

	resp.Code = 0
	resp.Message = fmt.Sprintf("success, received %d/%d records", recordReq.ReceivedNum, recordReq.SumNum)

	// 清理请求
	channel.RecordReqs.Remove(recordReq)

	return resp, nil
}

// PtzControl 实现云台控制功能
func (gb *GB28181ProPlugin) PtzControl(ctx context.Context, req *pb.PtzControlRequest) (*pb.BaseResponse, error) {
	resp := &pb.BaseResponse{}

	// 参数校验
	if req.DeviceId == "" {
		resp.Code = 400
		resp.Message = "设备ID不能为空"
		return resp, nil
	}
	if req.ChannelId == "" {
		resp.Code = 400
		resp.Message = "通道ID不能为空"
		return resp, nil
	}

	// 设置默认值
	if req.HorizonSpeed == 0 {
		req.HorizonSpeed = 100
	}
	if req.VerticalSpeed == 0 {
		req.VerticalSpeed = 100
	}
	if req.ZoomSpeed == 0 {
		req.ZoomSpeed = 16
	}

	// 参数范围验证
	if req.HorizonSpeed < 0 || req.HorizonSpeed > 255 {
		resp.Code = 400
		resp.Message = "水平速度必须在0-255范围内"
		return resp, nil
	}
	if req.VerticalSpeed < 0 || req.VerticalSpeed > 255 {
		resp.Code = 400
		resp.Message = "垂直速度必须在0-255范围内"
		return resp, nil
	}
	if req.ZoomSpeed < 0 || req.ZoomSpeed > 16 {
		resp.Code = 400
		resp.Message = "缩放速度必须在0-16范围内"
		return resp, nil
	}

	// 获取设备
	device, ok := gb.devices.Get(req.DeviceId)
	if !ok {
		resp.Code = 404
		resp.Message = "设备不存在"
		return resp, nil
	}

	// 根据命令设置对应的指令码
	var cmdCode int32
	switch req.Command {
	case "left":
		cmdCode = 2
	case "right":
		cmdCode = 1
	case "up":
		cmdCode = 8
	case "down":
		cmdCode = 4
	case "upleft":
		cmdCode = 10
	case "upright":
		cmdCode = 9
	case "downleft":
		cmdCode = 6
	case "downright":
		cmdCode = 5
	case "zoomin":
		cmdCode = 16
	case "zoomout":
		cmdCode = 32
	case "stop":
		cmdCode = 0
		req.HorizonSpeed = 0
		req.VerticalSpeed = 0
		req.ZoomSpeed = 0
	default:
		resp.Code = 400
		resp.Message = "不支持的控制命令"
		return resp, nil
	}

	// 调用设备的前端控制命令
	response, err := device.frontEndCmd(req.ChannelId, cmdCode, req.HorizonSpeed, req.VerticalSpeed, req.ZoomSpeed)
	if err != nil {
		resp.Code = 500
		resp.Message = fmt.Sprintf("发送云台控制命令失败: %v", err)
		return resp, nil
	}

	gb.Info("云台控制",
		"deviceId", req.DeviceId,
		"channelId", req.ChannelId,
		"command", req.Command,
		"horizonSpeed", req.HorizonSpeed,
		"verticalSpeed", req.VerticalSpeed,
		"zoomSpeed", req.ZoomSpeed,
		"response", response.String())

	resp.Code = 0
	resp.Message = "success"
	return resp, nil
}

// TestSip 实现测试SIP连接功能
func (gb *GB28181ProPlugin) TestSip(ctx context.Context, req *pb.TestSipRequest) (*pb.TestSipResponse, error) {
	resp := &pb.TestSipResponse{
		Code:    0,
		Message: "success",
	}

	// 创建一个临时设备用于测试
	device := &Device{
		DeviceID:   "34020000002000000001",
		LocalIP:    "192.168.1.17",
		Port:       5060,
		IP:         "192.168.1.102",
		StreamMode: "TCP-PASSIVE",
	}

	// 初始化设备的SIP相关字段
	device.fromHDR = sip.FromHeader{
		Address: sip.Uri{
			User: gb.Serial,
			Host: gb.Realm,
		},
		Params: sip.NewParams(),
	}
	device.fromHDR.Params.Add("tag", sip.GenerateTagN(16))

	device.contactHDR = sip.ContactHeader{
		Address: sip.Uri{
			User: gb.Serial,
			Host: device.LocalIP,
			Port: device.Port,
		},
	}

	// 初始化SIP客户端
	device.client, _ = sipgo.NewClient(gb.ua, sipgo.WithClientLogger(zerolog.New(os.Stdout)), sipgo.WithClientHostname(device.LocalIP))
	if device.client == nil {
		resp.Code = 500
		resp.Message = "failed to create sip client"
		return resp, nil
	}
	device.dialogClient = sipgo.NewDialogClient(device.client, device.contactHDR)

	// 构建目标URI
	recipient := sip.Uri{
		User: "34020000001320000006",
		Host: "192.168.1.102",
		Port: 5060,
	}

	// 创建INVITE请求
	request := device.CreateRequest(sip.INVITE, recipient)
	if request == nil {
		resp.Code = 500
		resp.Message = "failed to create request"
		return resp, nil
	}

	// 构建SDP消息体
	sdpInfo := []string{
		"v=0",
		fmt.Sprintf("o=%s 0 0 IN IP4 %s", "34020000001320000004", device.LocalIP),
		"s=Play",
		"c=IN IP4 " + device.LocalIP,
		"t=0 0",
		"m=video 43970 TCP/RTP/AVP 96 97 98 99",
		"a=recvonly",
		"a=rtpmap:96 PS/90000",
		"a=rtpmap:98 H264/90000",
		"a=rtpmap:97 MPEG4/90000",
		"a=rtpmap:99 H265/90000",
		"a=setup:passive",
		"a=connection:new",
		"y=0200005507",
	}

	// 设置必需的头部
	contentTypeHeader := sip.ContentTypeHeader("APPLICATION/SDP")
	subjectHeader := sip.NewHeader("Subject", "34020000001320000006:0200005507,34020000002000000001:0")
	toHeader := sip.ToHeader{
		Address: sip.Uri{
			User: "34020000001320000006",
			Host: device.IP,
			Port: device.Port,
		},
	}
	userAgentHeader := sip.NewHeader("User-Agent", "WVP-Pro v2.7.3.20241218")
	viaHeader := sip.ViaHeader{
		ProtocolName:    "SIP",
		ProtocolVersion: "2.0",
		Transport:       "UDP",
		Host:            device.LocalIP,
		Port:            device.Port,
		Params:          sip.HeaderParams(sip.NewParams()),
	}
	viaHeader.Params.Add("branch", sip.GenerateBranchN(16)).Add("rport", "")

	csqHeader := sip.CSeqHeader{
		SeqNo:      13,
		MethodName: "INVITE",
	}
	maxforward := sip.MaxForwardsHeader(70)
	contentLengthHeader := sip.ContentLengthHeader(286)
	request.AppendHeader(&contentTypeHeader)
	request.AppendHeader(subjectHeader)
	request.AppendHeader(&toHeader)
	request.AppendHeader(userAgentHeader)
	request.AppendHeader(&viaHeader)

	// 设置消息体
	request.SetBody([]byte(strings.Join(sdpInfo, "\r\n") + "\r\n"))

	// 创建会话并发送请求
	session, err := device.dialogClient.Invite(gb, recipient, request.Body(), &csqHeader, &device.fromHDR, &toHeader, &viaHeader, &maxforward, userAgentHeader, &device.contactHDR, subjectHeader, &contentTypeHeader, &contentLengthHeader)
	if err != nil {
		resp.Code = 500
		resp.Message = fmt.Sprintf("发送INVITE请求失败: %v", err)
		resp.TestResult = "failed"
		return resp, nil
	}

	// 等待响应
	err = session.WaitAnswer(gb, sipgo.AnswerOptions{})
	if err != nil {
		resp.Code = 500
		resp.Message = fmt.Sprintf("等待响应失败: %v", err)
		resp.TestResult = "failed"
		return resp, nil
	}

	// 发送ACK
	err = session.Ack(gb)
	if err != nil {
		resp.Code = 500
		resp.Message = fmt.Sprintf("发送ACK失败: %v", err)
		resp.TestResult = "failed"
		return resp, nil
	}

	resp.TestResult = "success"
	return resp, nil
}

// GetDeviceAlarm 实现设备报警查询
func (gb *GB28181ProPlugin) GetDeviceAlarm(ctx context.Context, req *pb.GetDeviceAlarmRequest) (*pb.DeviceAlarmResponse, error) {
	resp := &pb.DeviceAlarmResponse{
		Code:    0,
		Message: "success",
	}

	if gb.DB == nil {
		resp.Code = 500
		resp.Message = "数据库未初始化"
		return resp, nil
	}

	// 处理时间范围
	startTime, endTime, err := util.TimeRangeQueryParse(url.Values{
		"start": []string{req.StartTime},
		"end":   []string{req.EndTime},
	})
	if err != nil {
		resp.Code = 400
		resp.Message = fmt.Sprintf("时间格式错误: %v", err)
		return resp, nil
	}

	// 构建基础查询条件
	baseCondition := gb28181.DeviceAlarm{
		DeviceID: req.DeviceId,
	}

	// 构建查询
	query := gb.DB.Model(&gb28181.DeviceAlarm{}).Where(&baseCondition)

	// 添加时间范围条件
	if !startTime.IsZero() {
		query = query.Where("alarm_time >= ?", startTime)
	}
	if !endTime.IsZero() {
		query = query.Where("alarm_time <= ?", endTime)
	}

	// 添加报警方式条件
	if req.AlarmMethod != "" {
		query = query.Where(&gb28181.DeviceAlarm{AlarmMethod: req.AlarmMethod})
	}

	// 添加报警类型条件
	if req.AlarmType != "" {
		query = query.Where(&gb28181.DeviceAlarm{AlarmType: req.AlarmType})
	}

	// 添加报警级别范围条件
	if req.StartPriority != "" {
		query = query.Where("alarm_priority >= ?", req.StartPriority)
	}
	if req.EndPriority != "" {
		query = query.Where("alarm_priority <= ?", req.EndPriority)
	}

	// 查询报警记录
	var alarms []gb28181.DeviceAlarm
	if err := query.Order("alarm_time DESC").Find(&alarms).Error; err != nil {
		resp.Code = 500
		resp.Message = fmt.Sprintf("查询报警记录失败: %v", err)
		return resp, nil
	}

	// 转换为proto消息
	for _, alarm := range alarms {
		alarmInfo := &pb.AlarmInfo{
			DeviceId:         alarm.DeviceID,
			AlarmPriority:    alarm.AlarmPriority,
			AlarmMethod:      alarm.AlarmMethod,
			AlarmTime:        alarm.AlarmTime.Format("2006-01-02T15:04:05"),
			AlarmDescription: alarm.AlarmDescription,
		}
		resp.Alarms = append(resp.Alarms, alarmInfo)
	}

	return resp, nil
}

// AddPlatformChannel 实现添加平台通道
func (gb *GB28181ProPlugin) AddPlatformChannel(ctx context.Context, req *pb.AddPlatformChannelRequest) (*pb.BaseResponse, error) {
	resp := &pb.BaseResponse{}

	if gb.DB == nil {
		resp.Code = 500
		resp.Message = "数据库未初始化"
		return resp, nil
	}

	// 开始事务
	tx := gb.DB.Begin()

	// 遍历通道ID列表，为每个通道ID创建一条记录
	for _, channelId := range req.ChannelIds {
		// 创建新的平台通道记录
		platformChannel := &gb28181.PlatformChannel{
			PlatformId:      int(req.PlatformId),
			DeviceChannelId: int(channelId),
		}

		// 插入记录
		if err := tx.Create(platformChannel).Error; err != nil {
			tx.Rollback()
			resp.Code = 500
			resp.Message = fmt.Sprintf("添加平台通道失败: %v", err)
			return resp, nil
		}
	}

	// 提交事务
	if err := tx.Commit().Error; err != nil {
		resp.Code = 500
		resp.Message = fmt.Sprintf("提交事务失败: %v", err)
		return resp, nil
	}

	resp.Code = 0
	resp.Message = "success"
	return resp, nil
}
