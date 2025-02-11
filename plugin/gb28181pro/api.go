package plugin_gb28181pro

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"

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
			Id:           d.ID,
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
			Id:           d.ID,
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
		if req.Query != "" && !strings.Contains(d.ID, req.Query) && !strings.Contains(d.Name, req.Query) {
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
			Id:           d.ID,
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
			for i := 0; i < len(d.ID); i++ {
				hash = hash*31 + uint32(d.ID[i])
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
				User: d.ID,
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
