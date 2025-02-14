package plugin_gb28181pro

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/plugin/gb28181pro/pb"
	gb28181 "m7s.live/v5/plugin/gb28181pro/pkg"
)

func (gb *GB28181ProPlugin) List(context.Context, *emptypb.Empty) (ret *pb.ResponseList, err error) {
	ret = &pb.ResponseList{}
	for d := range gb.devices.Range {
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
				Longitude:    c.Longitude,
				Latitude:     c.Latitude,
				GpsTime:      timestamppb.New(c.GpsTime),
			})
		}
		ret.Data = append(ret.Data, &pb.Device{
			Id:           d.DeviceID,
			Name:         d.Name,
			Manufacturer: d.Manufacturer,
			Model:        d.Model,
			Owner:        d.Owner,
			Status:       string(d.Status),
			Longitude:    d.Longitude,
			Latitude:     d.Latitude,
			GpsTime:      timestamppb.New(d.GpsTime),
			RegisterTime: timestamppb.New(d.StartTime),
			UpdateTime:   timestamppb.New(d.UpdateTime),
			Channels:     channels,
		})
	}
	return
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
				Longitude:    c.Longitude,
				Latitude:     c.Latitude,
				GpsTime:      timestamppb.New(c.GpsTime),
			})
		}
		resp.Data = &pb.Device{
			Id:           d.DeviceID,
			Name:         d.Name,
			Manufacturer: d.Manufacturer,
			Model:        d.Model,
			Owner:        d.Owner,
			Status:       string(d.Status),
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
	var devices []*pb.Device
	total := 0
	for d := range gb.devices.Range {
		// TODO: 实现查询条件过滤
		if req.Query != "" && !strings.Contains(d.DeviceID, req.Query) && !strings.Contains(d.Name, req.Query) {
			continue
		}
		if req.Status && string(d.Status) != "ON" {
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
				Longitude:    c.Longitude,
				Latitude:     c.Latitude,
				GpsTime:      timestamppb.New(c.GpsTime),
			})
		}
		devices = append(devices, &pb.Device{
			Id:           d.DeviceID,
			Name:         d.Name,
			Manufacturer: d.Manufacturer,
			Model:        d.Model,
			Owner:        d.Owner,
			Status:       string(d.Status),
			Longitude:    d.Longitude,
			Latitude:     d.Latitude,
			GpsTime:      timestamppb.New(d.GpsTime),
			RegisterTime: timestamppb.New(d.StartTime),
			UpdateTime:   timestamppb.New(d.UpdateTime),
			Channels:     channels,
		})
	}
	resp.Total = int32(total)
	resp.List = devices
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
		if err := gb.DB.Where("id = ?", req.DeviceId).First(&device).Error; err == nil {
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
				Longitude:    c.Longitude,
				Latitude:     c.Latitude,
				GpsTime:      timestamppb.New(c.GpsTime),
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

	// 从设备列表中获取设备
	device, ok := gb.devices.Get(req.DeviceId)
	if !ok {
		gb.Error("StartPlay failed", "error", "device not found", "deviceId", req.DeviceId)
		resp.Code = 404
		resp.Message = "device not found"
		return resp, nil
	}

	// 从设备中获取通道
	channel, ok := device.channels.Get(req.ChannelId)
	if !ok {
		gb.Error("StartPlay failed", "error", "channel not found", "channelId", req.ChannelId)
		resp.Code = 404
		resp.Message = "channel not found"
		return resp, nil
	}

	// 获取可用的媒体端口
	var mediaPort uint16
	if gb.MediaPort.Valid() {
		select {
		case mediaPort = <-gb.tcpPorts:
			defer func() {
				gb.tcpPorts <- mediaPort
			}()
		default:
			resp.Code = 500
			resp.Message = "no available tcp port"
			return resp, nil
		}
	} else {
		mediaPort = gb.MediaPort[0]
	}

	// 调用 PlayStreamCmd
	session, err := gb.PlayStreamCmd(device, channel, mediaPort)
	if err != nil {
		gb.Error("StartPlay failed", "error", err, "step", "PlayStreamCmd")
		resp.Code = 500
		resp.Message = fmt.Sprintf("play stream failed: %v", err)
		return resp, nil
	}

	// 等待应答
	err = session.WaitAnswer(gb, sipgo.AnswerOptions{})
	if err != nil {
		gb.Error("StartPlay failed", "error", err, "step", "WaitAnswer")
		resp.Code = 500
		resp.Message = fmt.Sprintf("wait answer failed: %v", err)
		return resp, nil
	}

	// 解析应答中的 SSRC
	inviteResponseBody := string(session.InviteResponse.Body())
	gb.Debug("StartPlay response", "body", inviteResponseBody)
	lines := strings.Split(inviteResponseBody, "\r\n")
	var ssrc string
	for _, line := range lines {
		if parts := strings.Split(line, "="); len(parts) > 1 {
			if parts[0] == "y" && len(parts[1]) > 0 {
				ssrc = parts[1]
				break
			}
		}
	}

	// 发送 ACK
	err = session.Ack(gb)
	if err != nil {
		gb.Error("StartPlay failed", "error", err, "step", "Ack")
		resp.Code = 500
		resp.Message = fmt.Sprintf("send ack failed: %v", err)
		return resp, nil
	}

	// 设置响应信息
	resp.Code = 0
	resp.Message = "success"
	resp.StreamInfo = &pb.StreamInfo{
		Stream: fmt.Sprintf("%s/%s", req.DeviceId, req.ChannelId),
		App:    "gb28181",
		Ip:     device.IP,
		Port:   int32(mediaPort),
		Ssrc:   ssrc,
	}

	gb.Info("StartPlay success", "deviceId", req.DeviceId, "channelId", req.ChannelId, "ssrc", ssrc)
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
	var existingPlatform gb28181.Platform
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
	platform := &gb28181.Platform{
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
	if err := gb.DB.Create(platform).Error; err != nil {
		resp.Code = 500
		resp.Message = fmt.Sprintf("failed to create platform: %v", err)
		return resp, nil
	}

	// 如果平台启用，则启动注册任务
	if platform.Enable {
		// 获取本地SIP端口
		localPort := 5060 // 默认端口
		if len(gb.Sip.ListenAddr) > 0 {
			if port, err := strconv.Atoi(strings.Split(gb.Sip.ListenAddr[0], ":")[1]); err == nil {
				localPort = port
			}
		}
		// 创建平台命令器
		commander := gb28181.NewSIPPlatformCommander(platform, gb.GetPublicIP(platform.ServerIP), localPort)
		if err := commander.InitializeSIPClient(gb.ua); err != nil {
			gb.Error("初始化SIP客户端失败", "error", err)
			resp.Code = 500
			resp.Message = fmt.Sprintf("初始化SIP客户端失败: %v", err)
			return resp, nil
		}
		// 启动注册任务
		commander.StartRegisterTask(gb)
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

	var platform gb28181.Platform
	if err := gb.DB.First(&platform, req.Id).Error; err != nil {
		resp.Code = 404
		resp.Message = "platform not found"
		return resp, nil
	}

	// 将数据库模型转换为proto消息
	resp.Data = &pb.Platform{
		Id:                      int32(platform.ID),
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
	var platform gb28181.Platform
	if err := gb.DB.First(&platform, req.Id).Error; err != nil {
		resp.Code = 404
		resp.Message = "platform not found"
		return resp, nil
	}

	// 记录之前的启用状态
	wasEnabled := platform.Enable

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
	if !wasEnabled && platform.Enable {
		// 获取本地SIP端口
		localPort := 5060 // 默认端口
		if len(gb.Sip.ListenAddr) > 0 {
			if port, err := strconv.Atoi(strings.Split(gb.Sip.ListenAddr[0], ":")[1]); err == nil {
				localPort = port
			}
		}
		// 平台从禁用变为启用，启动注册任务
		commander := gb28181.NewSIPPlatformCommander(&platform, gb.GetPublicIP(platform.ServerIP), localPort)
		if err := commander.InitializeSIPClient(gb.ua); err != nil {
			gb.Error("初始化SIP客户端失败", "error", err)
			resp.Code = 500
			resp.Message = fmt.Sprintf("初始化SIP客户端失败: %v", err)
			return resp, nil
		}
		commander.StartRegisterTask(gb)
	} else if wasEnabled && !platform.Enable {
		// TODO: 平台从启用变为禁用，需要处理注销逻辑
		// 这里可以添加注销相关的代码
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
	if err := gb.DB.Delete(&gb28181.Platform{}, req.Id).Error; err != nil {
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

	var platforms []gb28181.Platform
	var total int64

	// 构建查询条件
	query := gb.DB.Model(&gb28181.Platform{})
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
			Id:                      int32(p.ID),
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
