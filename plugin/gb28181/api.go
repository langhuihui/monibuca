package plugin_gb28181pro

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/gobwas/ws"
	"gorm.io/gorm"
	"m7s.live/v5"
	"m7s.live/v5/pkg/util"

	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"m7s.live/v5/plugin/gb28181/pb"
	gb28181 "m7s.live/v5/plugin/gb28181/pkg"
	mrtp "m7s.live/v5/plugin/rtp/pkg"
)

func (gb *GB28181Plugin) List(ctx context.Context, req *pb.GetDevicesRequest) (*pb.DevicesPageInfo, error) {
	resp := &pb.DevicesPageInfo{}

	// 从内存中读取设备信息，而不是从数据库查询
	var pbDevices []*pb.Device
	var filteredDevices []*Device

	// 遍历内存中的设备集合
	gb.devices.Range(func(device *Device) bool {
		// 应用筛选条件
		if req.Query != "" {
			// 检查设备ID或名称是否包含查询字符串
			if !strings.Contains(device.DeviceId, req.Query) && !strings.Contains(device.Name, req.Query) && !strings.Contains(device.CustomName, req.Query) {
				return true // 继续遍历
			}
		}

		// 如果需要筛选在线设备
		if (req.Status == 1 && !device.Online) || (req.Status == 0 && device.Online) {
			return true // 继续遍历
		}

		// 添加到过滤后的设备列表
		filteredDevices = append(filteredDevices, device)
		return true
	})

	// 计算总数
	total := len(filteredDevices)
	resp.Total = int32(total)

	// 处理分页
	if req.Page > 0 && req.Count > 0 {
		// 计算起始和结束索引
		start := int(req.Page-1) * int(req.Count)
		end := start + int(req.Count)

		// 边界检查
		if start >= total {
			// 超出范围，返回空列表
			resp.Code = 0
			resp.Message = "success"
			resp.Data = pbDevices
			return resp, nil
		}

		if end > total {
			end = total
		}

		// 应用分页
		filteredDevices = filteredDevices[start:end]
	}

	// 转换为proto消息
	for _, d := range filteredDevices {
		var pbChannels []*pb.Channel

		// 从设备的内存通道集合中获取通道信息
		d.channels.Range(func(channel *Channel) bool {
			if channel.ID == "34020000001320000109_34020000001310000109" {
				gb.Info("channel", "id", channel.ID)
				d.Info("channel", "id", channel.ID)
			}
			pbChannels = append(pbChannels, &pb.Channel{
				Id:                channel.ID,
				DeviceId:          channel.ChannelId,
				ChannelId:         channel.ChannelId,
				ParentId:          d.DeviceId,
				Name:              util.Conditional(channel.CustomName == "", channel.Name, channel.CustomName),
				CustomChannelId:   channel.CustomChannelId,
				CustomChannelName: util.Conditional(channel.CustomName == "", channel.Name, channel.CustomName),
				StreamPath:        channel.StreamPath,
				Manufacturer:      channel.Manufacturer,
				Model:             channel.Model,
				Owner:             channel.Owner,
				CivilCode:         channel.CivilCode,
				Address:           channel.Address,
				Port:              int32(channel.Port),
				Parental:          int32(channel.Parental),
				SafetyWay:         int32(channel.SafetyWay),
				RegisterWay:       int32(channel.RegisterWay),
				Secrecy:           int32(channel.Secrecy),
				Status:            string(channel.Status),
				Longitude:         fmt.Sprintf("%f", channel.GbLongitude),
				Latitude:          fmt.Sprintf("%f", channel.GbLatitude),
				GpsTime:           timestamppb.New(time.Now()),
			})
			return true
		})
		sort.Slice(pbChannels, func(i, j int) bool {
			return pbChannels[i].DeviceId < pbChannels[j].DeviceId
		})

		pbDevices = append(pbDevices, &pb.Device{
			DeviceId:              d.DeviceId,
			Name:                  d.CustomName,
			Manufacturer:          d.Manufacturer,
			Model:                 d.Model,
			Status:                string(d.Status),
			Online:                d.Online,
			Longitude:             d.Longitude,
			Latitude:              d.Latitude,
			RegisterTime:          timestamppb.New(d.RegisterTime),
			UpdateTime:            timestamppb.New(d.UpdateTime),
			KeepAliveTime:         timestamppb.New(d.KeepaliveTime),
			ChannelCount:          int32(d.ChannelCount),
			Channels:              pbChannels,
			MediaIp:               d.MediaIp,
			SipIp:                 d.SipIp,
			Password:              d.Password,
			StreamMode:            string(d.StreamMode),
			Transport:             d.Transport,
			Ip:                    d.IP,
			Port:                  int32(d.Port),
			BroadcastPushAfterAck: d.BroadcastPushAfterAck,
			SubscribeCatalog:      util.Conditional(d.SubscribeCatalog == 0, false, true),
			SubscribePosition:     util.Conditional(d.SubscribePosition == 0, false, true),
			SubscribeAlarm:        util.Conditional(d.SubscribeAlarm == 0, false, true),
			SsrcCheck:             d.SSRCCheck,
			Charset:               d.Charset,
		})
	}

	// 按deviceId对设备列表进行排序
	sort.Slice(pbDevices, func(i, j int) bool {
		return pbDevices[i].DeviceId < pbDevices[j].DeviceId
	})

	resp.Code = 0
	resp.Message = "success"
	resp.Data = pbDevices

	return resp, nil
}

// GetDevice 实现获取单个设备信息
func (gb *GB28181Plugin) GetDevice(ctx context.Context, req *pb.GetDeviceRequest) (*pb.DeviceResponse, error) {
	resp := &pb.DeviceResponse{}

	// 先从内存中获取
	d, ok := gb.devices.Get(req.DeviceId)
	if !ok {
		resp.Code = 404
		resp.Message = "设备不存在"
	}

	if d != nil {
		var channels []*pb.Channel
		for c := range d.channels.Range {
			channels = append(channels, &pb.Channel{
				DeviceId:          c.ChannelId,
				ChannelId:         c.ChannelId,
				ParentId:          d.DeviceId,
				Name:              c.Name,
				Manufacturer:      c.Manufacturer,
				Model:             c.Model,
				Owner:             c.Owner,
				CivilCode:         c.CivilCode,
				Address:           c.Address,
				Port:              int32(c.Port),
				Parental:          int32(c.Parental),
				SafetyWay:         int32(c.SafetyWay),
				RegisterWay:       int32(c.RegisterWay),
				Secrecy:           int32(c.Secrecy),
				Status:            string(c.Status),
				Longitude:         fmt.Sprintf("%f", c.GbLongitude),
				Latitude:          fmt.Sprintf("%f", c.GbLatitude),
				GpsTime:           timestamppb.New(time.Now()),
				CustomChannelId:   c.CustomChannelId,
				CustomChannelName: util.Conditional(c.CustomName == "", c.Name, c.CustomName),
				StreamPath:        c.StreamPath,
			})
		}
		resp.Data = &pb.Device{
			DeviceId:         d.DeviceId,
			Name:             d.Name,
			Manufacturer:     d.Manufacturer,
			Model:            d.Model,
			Status:           string(d.Status),
			Online:           d.Online,
			Longitude:        d.Longitude,
			Latitude:         d.Latitude,
			RegisterTime:     timestamppb.New(d.RegisterTime),
			UpdateTime:       timestamppb.New(d.UpdateTime),
			Channels:         channels,
			MediaIp:          d.MediaIp,
			SipIp:            d.SipIp,
			Password:         d.Password,
			StreamMode:       string(d.StreamMode),
			SubscribeCatalog: util.Conditional(d.SubscribeCatalog == 0, false, true),
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
func (gb *GB28181Plugin) GetDevices(ctx context.Context, req *pb.GetDevicesRequest) (*pb.DevicesPageInfo, error) {
	resp := &pb.DevicesPageInfo{}

	// 从内存中获取设备列表
	var pbDevices []*pb.Device
	var total int64
	var filteredDevices []*Device

	// 遍历内存中的设备
	for d := range gb.devices.Range {
		// 应用查询条件过滤
		if req.Query != "" && !strings.Contains(d.DeviceId, req.Query) && !strings.Contains(d.Name, req.Query) {
			continue
		}
		if req.Status == 1 && !d.Online {
			continue
		}

		filteredDevices = append(filteredDevices, d)
		total++
	}

	// 处理分页
	startIdx := 0
	endIdx := len(filteredDevices)

	// 当Page和Count都不为0时，进行分页处理
	if req.Page > 0 && req.Count > 0 {
		startIdx = int((req.Page - 1) * req.Count)
		endIdx = int(req.Page * req.Count)
		if startIdx >= len(filteredDevices) {
			startIdx = len(filteredDevices)
		}
		if endIdx > len(filteredDevices) {
			endIdx = len(filteredDevices)
		}
	}

	// 处理分页后的设备列表
	for _, d := range filteredDevices[startIdx:endIdx] {
		// 从内存中获取通道
		var pbChannels []*pb.Channel
		for c := range d.channels.Range {
			pbChannels = append(pbChannels, &pb.Channel{
				ChannelId:    c.ChannelId,
				DeviceId:     c.ChannelId,
				ParentId:     d.DeviceId,
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
			DeviceId:      d.DeviceId,
			Name:          d.Name,
			Manufacturer:  d.Manufacturer,
			Model:         d.Model,
			Status:        string(d.Status),
			Online:        d.Online,
			Longitude:     d.Longitude,
			Latitude:      d.Latitude,
			RegisterTime:  timestamppb.New(d.RegisterTime),
			UpdateTime:    timestamppb.New(d.UpdateTime),
			KeepAliveTime: timestamppb.New(d.KeepaliveTime),
			Channels:      pbChannels,
			MediaIp:       d.MediaIp,
			SipIp:         d.SipIp,
			Password:      d.Password,
			StreamMode:    string(d.StreamMode),
		}
		pbDevices = append(pbDevices, pbDevice)
	}

	resp.Total = int32(total)
	resp.Data = pbDevices
	resp.Code = 0
	resp.Message = "success"
	return resp, nil
}

// GetChannels 实现分页查询通道
func (gb *GB28181Plugin) GetChannels(ctx context.Context, req *pb.GetChannelsRequest) (*pb.ChannelsPageInfo, error) {
	resp := &pb.ChannelsPageInfo{}

	// 直接从内存中获取设备
	d, ok := gb.devices.Get(req.DeviceId)
	if !ok {
		// 如果内存中没有设备，返回设备未找到的错误
		resp.Code = 404
		resp.Message = "设备未找到"
		return resp, nil
	}

	if d != nil {
		var channels []*pb.Channel
		total := 0
		for c := range d.channels.Range {
			// 实现查询条件过滤
			if req.Query != "" && !strings.Contains(c.DeviceId, req.Query) && !strings.Contains(c.Name, req.Query) {
				continue
			}
			if req.Online && string(c.Status) != "ON" {
				continue
			}
			if req.ChannelType && c.ParentId == "" {
				continue
			}
			total++

			// 当Page和Count都为0时，不做分页，返回所有数据
			if req.Page == 0 && req.Count == 0 {
				// 不分页，添加所有符合条件的通道
				channels = append(channels, &pb.Channel{
					DeviceId:          c.ChannelId,
					ChannelId:         c.ChannelId,
					ParentId:          c.ParentId,
					Name:              c.Name,
					Manufacturer:      c.Manufacturer,
					Model:             c.Model,
					Owner:             c.Owner,
					CivilCode:         c.CivilCode,
					Address:           c.Address,
					Port:              int32(c.Port),
					Parental:          int32(c.Parental),
					SafetyWay:         int32(c.SafetyWay),
					RegisterWay:       int32(c.RegisterWay),
					Secrecy:           int32(c.Secrecy),
					Status:            string(c.Status),
					Longitude:         fmt.Sprintf("%f", c.GbLongitude),
					Latitude:          fmt.Sprintf("%f", c.GbLatitude),
					GpsTime:           timestamppb.New(time.Now()),
					CustomChannelId:   c.CustomChannelId,
					CustomChannelName: util.Conditional(c.CustomName == "", c.Name, c.CustomName),
					StreamPath:        c.StreamPath,
				})
			} else {
				// 分页处理
				if total > int(req.Page*req.Count) {
					continue
				}
				if total <= int((req.Page-1)*req.Count) {
					continue
				}
				channels = append(channels, &pb.Channel{
					DeviceId:     c.ChannelId,
					ChannelId:    c.ChannelId,
					ParentId:     c.ParentId,
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
		}
		resp.Total = int32(total)
		resp.Data = channels
		resp.Code = 0
		resp.Message = "success"
	} else {
		resp.Code = 404
		resp.Message = "设备未找到"
	}
	return resp, nil
}

// SyncDevice 实现同步设备通道信息
func (gb *GB28181Plugin) SyncDevice(ctx context.Context, req *pb.SyncDeviceRequest) (*pb.SyncStatus, error) {
	resp := &pb.SyncStatus{
		Code:    404,
		Message: "device not found",
	}
	var device *Device

	// 先从内存中获取设备
	if d, ok := gb.devices.Get(req.DeviceId); ok {
		device = d
	}

	if device != nil {
		// 发送目录查询请求
		_, err := device.catalog()
		if err != nil {
			resp.Code = 500
			resp.Message = "catalog request failed"
			resp.ErrorMsg = err.Error()
		} else {
			resp.Code = 0
			resp.Message = "sync request sent"
			resp.Total = int32(device.ChannelCount)
			resp.Current = 0 // 初始化进度为0
		}
	}

	return resp, nil
}

// UpdateDevice 实现更新设备信息
func (gb *GB28181Plugin) UpdateDevice(ctx context.Context, req *pb.Device) (*pb.BaseResponse, error) {
	resp := &pb.BaseResponse{}

	// 检查数据库连接
	if gb.DB == nil {
		resp.Code = 500
		resp.Message = "数据库未初始化"
		return resp, nil
	}

	// 先从缓存中读取设备
	if d, ok := gb.devices.Get(req.DeviceId); ok {
		// 保存原始密码，用于后续检查是否修改了密码
		originalPassword := d.Password

		// 更新基本字段
		if req.Name != "" {
			d.CustomName = req.Name
		} else {
			d.CustomName = d.Name
		}
		if req.Manufacturer != "" {
			d.Manufacturer = req.Manufacturer
		}
		if req.Model != "" {
			d.Model = req.Model
		}
		if req.Longitude != "" {
			d.Longitude = req.Longitude
		}
		if req.Latitude != "" {
			d.Latitude = req.Latitude
		}

		// 更新新增字段
		if req.MediaIp != "" {
			d.MediaIp = req.MediaIp
		}
		if req.SipIp != "" {
			d.SipIp = req.SipIp

			// 更新SIP相关字段
			d.contactHDR = sip.ContactHeader{
				Address: sip.Uri{
					User: gb.Serial,
					Host: d.SipIp,
					Port: d.Port,
				},
			}
		}
		if req.StreamMode != "" {
			d.StreamMode = mrtp.StreamMode(req.StreamMode)
		}
		if req.Charset != "" {
			d.Charset = req.Charset
		}
		if req.Password != "" {
			d.Password = req.Password
		}

		// 更新订阅相关字段
		if req.SubscribeCatalog {
			d.SubscribeCatalog = 3600 // 默认订阅周期为60分钟
		} else {
			d.SubscribeCatalog = 0 // 不订阅
		}

		if req.SubscribePosition {
			d.SubscribePosition = 3600 // 默认订阅周期为60分钟
		} else {
			d.SubscribePosition = 0 // 不订阅
		}

		//更新订阅报警信息的字段
		if req.SubscribeAlarm {
			d.SubscribeAlarm = 3600 // 默认订阅周期为60分钟
		} else {
			d.SubscribeAlarm = 0 // 不订阅
		}
		if req.BroadcastPushAfterAck {
			d.BroadcastPushAfterAck = req.BroadcastPushAfterAck
		} else {
			d.BroadcastPushAfterAck = false
		}

		// 更新 SSRC 校验开关
		d.SSRCCheck = req.SsrcCheck

		d.UpdateTime = time.Now()

		// 先停止设备任务
		//d.Stop(fmt.Errorf("device updated"))
		// 更新数据库中的设备信息
		updates := map[string]interface{}{
			"name":               d.Name,
			"manufacturer":       d.Manufacturer,
			"model":              d.Model,
			"longitude":          d.Longitude,
			"latitude":           d.Latitude,
			"media_ip":           d.MediaIp,
			"sip_ip":             d.SipIp,
			"stream_mode":        d.StreamMode,
			"password":           d.Password,
			"subscribe_catalog":  d.SubscribeCatalog,
			"subscribe_position": d.SubscribePosition,
			"subscribe_alarm":    d.SubscribeAlarm,
			"ssrc_check":         d.SSRCCheck,
			"update_time":        d.UpdateTime,
		}

		if err := gb.DB.Model(&Device{}).Where("device_id = ?", req.DeviceId).Updates(updates).Error; err != nil {
			resp.Code = 500
			resp.Message = fmt.Sprintf("更新设备失败: %v", err)
			return resp, nil
		}

		// 检查密码是否被修改
		passwordChanged := req.Password != "" && req.Password != originalPassword

		// 如果密码没有被修改，则需要重新启动设备任务和订阅任务
		if !passwordChanged {
			// 重新启动设备任务
			//gb.AddTask(d)

			// 如果需要订阅目录，创建并启动目录订阅任务
			if d.Online {
				if d.SubscribeCatalog > 0 {
					if d.CatalogSubscribeTask != nil {
						d.CatalogSubscribeTask.Ticker.Reset(time.Second * time.Duration(d.SubscribeCatalog))
					} else {
						d.CatalogSubscribeTask = NewCatalogSubscribeTask(d)
						d.AddTask(d.CatalogSubscribeTask)
					}
					d.CatalogSubscribeTask.Tick(nil)
				} else {
					if d.CatalogSubscribeTask != nil {
						d.CatalogSubscribeTask.Tick(nil)
						d.CatalogSubscribeTask.Ticker.Reset(time.Hour * 999999)
					}
				}
				if d.SubscribePosition > 0 {
					if d.PositionSubscribeTask != nil {
						d.PositionSubscribeTask.Ticker.Reset(time.Second * time.Duration(d.SubscribePosition))
					} else {
						d.PositionSubscribeTask = NewPositionSubscribeTask(d)
						d.AddTask(d.PositionSubscribeTask)
					}
					d.PositionSubscribeTask.Tick(nil)
				} else {
					if d.PositionSubscribeTask != nil {
						d.PositionSubscribeTask.Tick(nil)
						d.PositionSubscribeTask.Ticker.Reset(time.Hour * 999999)
					}
				}
				if d.SubscribeAlarm > 0 {
					if d.AlarmSubscribeTask != nil {
						d.AlarmSubscribeTask.Ticker.Reset(time.Second * time.Duration(d.SubscribeAlarm))
					} else {
						d.AlarmSubscribeTask = NewAlarmSubscribeTask(d)
						d.AddTask(d.AlarmSubscribeTask)
					}
					d.AlarmSubscribeTask.Tick(nil)
				} else {
					if d.AlarmSubscribeTask != nil {
						d.AlarmSubscribeTask.Ticker.Reset(time.Hour * 999999)
					}
				}
			}
		} else {
			d.Status = DeviceOfflineStatus
			d.Online = false
			d.channels.Range(func(c *Channel) bool {
				c.Status = gb28181.ChannelOffStatus
				return true
			})
			//d.Stop(fmt.Errorf("password changed"))
		}

		resp.Code = 0
		resp.Message = "设备更新成功"
		return resp, nil
	}

	// 如果缓存中没有，则从数据库中查找设备
	var device Device
	if err := gb.DB.Where("device_id = ?", req.DeviceId).First(&device).Error; err != nil {
		// 如果数据库中也没有找到设备，返回错误
		resp.Code = 404
		resp.Message = fmt.Sprintf("设备不存在: %v", err)
		return resp, nil
	}

	// 如果数据库中找到了设备，直接更新数据库
	updates := map[string]interface{}{}

	// 更新基本字段
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Manufacturer != "" {
		updates["manufacturer"] = req.Manufacturer
	}
	if req.Model != "" {
		updates["model"] = req.Model
	}
	if req.Longitude != "" {
		updates["longitude"] = req.Longitude
	}

	if req.Latitude != "" {
		updates["latitude"] = req.Latitude
	}

	// 更新新增字段
	if req.MediaIp != "" {
		updates["media_ip"] = req.MediaIp
	}
	if req.SipIp != "" {
		updates["sip_ip"] = req.SipIp
	}
	if req.StreamMode != "" {
		updates["stream_mode"] = req.StreamMode
	}
	if req.Password != "" {
		updates["password"] = req.Password
	}

	// 更新订阅相关字段
	if req.SubscribeCatalog {
		updates["subscribe_catalog"] = 3600 // 默认订阅周期为3600秒
	} else {
		updates["subscribe_catalog"] = 0 // 不订阅
	}

	if req.SubscribePosition {
		updates["subscribe_position"] = 3600 // 默认订阅周期为3600秒
	} else {
		updates["subscribe_position"] = 0 // 不订阅
	}

	// 更新 SSRC 校验开关
	updates["ssrc_check"] = req.SsrcCheck

	updates["update_time"] = time.Now()

	// 保存到数据库
	if err := gb.DB.Model(&device).Updates(updates).Error; err != nil {
		resp.Code = 500
		resp.Message = fmt.Sprintf("更新设备失败: %v", err)
		return resp, nil
	}

	resp.Code = 0
	resp.Message = "设备更新成功"
	return resp, nil
}

// AddPlatform 实现添加平台信息
func (gb *GB28181Plugin) AddPlatform(ctx context.Context, req *pb.Platform) (*pb.BaseResponse, error) {
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
	if req.RegisterInterval <= 0 {
		req.RegisterInterval = 60 // 默认60秒
	}
	if req.MaxTimeoutCount <= 0 {
		req.MaxTimeoutCount = 3 // 默认3次
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
		DeviceGBDomain:          req.DeviceGBDomain,
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
		SendStreamIp:            req.SendStreamIp,
		AutoPushChannel:         req.AutoPushChannel,
		CatalogWithPlatform:     int(req.CatalogWithPlatform),
		CatalogWithGroup:        int(req.CatalogWithGroup),
		CatalogWithRegion:       int(req.CatalogWithRegion),
		CivilCode:               req.CivilCode,
		Manufacturer:            req.Manufacturer,
		Model:                   req.Model,
		Address:                 req.Address,
		RegisterWay:             int(req.RegisterWay),
		Secrecy:                 int(req.Secrecy),
		RegisterInterval:        int(req.RegisterInterval),
		MaxTimeoutCount:         int(req.MaxTimeoutCount),
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
		platform := NewPlatform(platformModel, gb, false)
		// 添加到任务系统
		platform.Logger = gb.Logger.With("platform_server_gb_id", platformModel.ServerGBID)
		gb.platforms.AddTask(platform)
	}

	resp.Code = 0
	resp.Message = "success"
	return resp, nil
}

// GetPlatform 实现获取平台信息
func (gb *GB28181Plugin) GetPlatform(ctx context.Context, req *pb.GetPlatformRequest) (*pb.PlatformResponse, error) {
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
		Enable:                  platform.Enable,
		Name:                    platform.Name,
		ServerGBId:              platform.ServerGBID,
		ServerGBDomain:          platform.ServerGBDomain,
		ServerIp:                platform.ServerIP,
		ServerPort:              int32(platform.ServerPort),
		DeviceGBId:              platform.DeviceGBID,
		DeviceGBDomain:          platform.DeviceGBDomain,
		DeviceIp:                platform.DeviceIP,
		DevicePort:              int32(platform.DevicePort),
		Username:                platform.Username,
		Password:                platform.Password,
		Expires:                 int32(platform.Expires),
		RegisterInterval:        int32(platform.RegisterInterval),
		KeepTimeout:             int32(platform.KeepTimeout),
		MaxTimeoutCount:         int32(platform.MaxTimeoutCount),
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
		SendStreamIp:            platform.SendStreamIp,
		AutoPushChannel:         platform.AutoPushChannel,
		CatalogWithPlatform:     int32(platform.CatalogWithPlatform),
		CatalogWithGroup:        int32(platform.CatalogWithGroup),
		CatalogWithRegion:       int32(platform.CatalogWithRegion),
		CivilCode:               platform.CivilCode,
		Manufacturer:            platform.Manufacturer,
		Model:                   platform.Model,
		Address:                 platform.Address,
		RegisterWay:             int32(platform.RegisterWay),
		Secrecy:                 int32(platform.Secrecy),
		ReadOnly:                platform.ReadOnly,
	}

	resp.Code = 0
	resp.Message = "success"
	return resp, nil
}

// UpdatePlatform 实现更新平台信息
func (gb *GB28181Plugin) UpdatePlatform(ctx context.Context, req *pb.Platform) (*pb.BaseResponse, error) {
	resp := &pb.BaseResponse{}

	if gb.DB == nil {
		resp.Code = 500
		resp.Message = "database not initialized"
		return resp, nil
	}

	// 检查平台是否存在
	if oldPlatform, ok := gb.platforms.Get(req.ServerGBId); !ok {
		resp.Code = 404
		resp.Message = "platform not found"
		return resp, nil
	} else {
		// 记录原始值，用于检查是否有变化
		oldEnable := oldPlatform.PlatformModel.Enable
		oldExpires := oldPlatform.PlatformModel.Expires
		oldKeepTimeout := oldPlatform.PlatformModel.KeepTimeout
		oldServerIp := oldPlatform.PlatformModel.ServerIP
		oldServerPort := int32(oldPlatform.PlatformModel.ServerPort)

		// 更新oldPlatform中的字段
		if req.Name != "" {
			oldPlatform.PlatformModel.Name = req.Name
		}
		if req.ServerGBDomain != "" {
			oldPlatform.PlatformModel.ServerGBDomain = req.ServerGBDomain
		}
		if req.ServerIp != "" {
			oldPlatform.PlatformModel.ServerIP = req.ServerIp
		}
		if req.ServerPort > 0 {
			oldPlatform.PlatformModel.ServerPort = int(req.ServerPort)
		}
		if req.DeviceGBId != "" {
			oldPlatform.PlatformModel.DeviceGBID = req.DeviceGBId
		}
		if req.DeviceGBDomain != "" {
			oldPlatform.PlatformModel.DeviceGBDomain = req.DeviceGBDomain
		}
		if req.DeviceIp != "" {
			oldPlatform.PlatformModel.DeviceIP = req.DeviceIp
		}
		if req.DevicePort > 0 {
			oldPlatform.PlatformModel.DevicePort = int(req.DevicePort)
		}
		if req.Username != "" {
			oldPlatform.PlatformModel.Username = req.Username
		}
		if req.Password != "" {
			oldPlatform.PlatformModel.Password = req.Password
		}
		if req.Expires > 0 {
			oldPlatform.PlatformModel.Expires = int(req.Expires)
		}
		if req.RegisterInterval > 0 {
			oldPlatform.PlatformModel.RegisterInterval = int(req.RegisterInterval)
		}
		if req.KeepTimeout > 0 {
			oldPlatform.PlatformModel.KeepTimeout = int(req.KeepTimeout)
		}
		if req.MaxTimeoutCount > 0 {
			oldPlatform.PlatformModel.MaxTimeoutCount = int(req.MaxTimeoutCount)
		}
		if req.Transport != "" {
			oldPlatform.PlatformModel.Transport = req.Transport
		}
		if req.CharacterSet != "" {
			oldPlatform.PlatformModel.CharacterSet = req.CharacterSet
		}

		oldPlatform.PlatformModel.Enable = req.Enable
		oldPlatform.PlatformModel.PTZ = req.Ptz
		oldPlatform.PlatformModel.RTCP = req.Rtcp
		oldPlatform.PlatformModel.CatalogSubscribe = req.CatalogSubscribe
		oldPlatform.PlatformModel.AlarmSubscribe = req.AlarmSubscribe
		oldPlatform.PlatformModel.MobilePositionSubscribe = req.MobilePositionSubscribe

		if req.CatalogGroup > 0 {
			oldPlatform.PlatformModel.CatalogGroup = int(req.CatalogGroup)
		}
		if req.SendStreamIp != "" {
			oldPlatform.PlatformModel.SendStreamIp = req.SendStreamIp
		}

		oldPlatform.PlatformModel.AsMessageChannel = req.AsMessageChannel
		oldPlatform.PlatformModel.AutoPushChannel = req.AutoPushChannel

		if req.CatalogWithPlatform > 0 {
			oldPlatform.PlatformModel.CatalogWithPlatform = int(req.CatalogWithPlatform)
		}
		if req.CatalogWithGroup > 0 {
			oldPlatform.PlatformModel.CatalogWithGroup = int(req.CatalogWithGroup)
		}
		if req.CatalogWithRegion > 0 {
			oldPlatform.PlatformModel.CatalogWithRegion = int(req.CatalogWithRegion)
		}
		if req.CivilCode != "" {
			oldPlatform.PlatformModel.CivilCode = req.CivilCode
		}
		if req.Manufacturer != "" {
			oldPlatform.PlatformModel.Manufacturer = req.Manufacturer
		}
		if req.Model != "" {
			oldPlatform.PlatformModel.Model = req.Model
		}
		if req.Address != "" {
			oldPlatform.PlatformModel.Address = req.Address
		}
		if req.RegisterWay > 0 {
			oldPlatform.PlatformModel.RegisterWay = int(req.RegisterWay)
		}
		if req.Secrecy > 0 {
			oldPlatform.PlatformModel.Secrecy = int(req.Secrecy)
		}

		// 更新时间，使用UTC时间
		oldPlatform.PlatformModel.UpdateTime = time.Now().UTC().Format("2006-01-02 15:04:05")

		// 使用 GORM 的 Updates 方法更新数据库
		if err := gb.DB.Model(&gb28181.PlatformModel{}).Where("server_gb_id = ?", req.ServerGBId).Updates(oldPlatform.PlatformModel).Error; err != nil {
			resp.Code = 500
			resp.Message = fmt.Sprintf("failed to update platform: %v", err)
			return resp, nil
		}

		// 检查关键字段是否有变化
		enableChanged := oldEnable != oldPlatform.PlatformModel.Enable
		expiresChanged := oldExpires != oldPlatform.PlatformModel.Expires
		keepTimeoutChanged := oldKeepTimeout != oldPlatform.PlatformModel.KeepTimeout
		serverIpChanged := oldServerIp != oldPlatform.PlatformModel.ServerIP
		serverPortChanged := oldServerPort != int32(oldPlatform.PlatformModel.ServerPort)

		// 处理平台启用状态变化
		if oldPlatform.PlatformModel.Enable {
			// 如果平台被启用或关键参数变化，需要更新注册和心跳任务
			if enableChanged || expiresChanged || keepTimeoutChanged || serverIpChanged || serverPortChanged {
				oldPlatform.Unregister()
				oldPlatform.register.Ticker.Reset(time.Second * time.Duration(oldPlatform.PlatformModel.Expires))
				oldPlatform.register.Tick(nil)
				if oldPlatform.register.platformKeepAliveTask != nil {
					oldPlatform.register.platformKeepAliveTask.Ticker.Reset(time.Second * time.Duration(oldPlatform.PlatformModel.KeepTimeout))
				}
			}
		} else if enableChanged {
			// 如果平台被禁用，停止并移除旧的platform实例
			oldPlatform.Unregister()
			oldPlatform.register.Ticker.Reset(time.Hour * 999999)
			if oldPlatform.register.platformKeepAliveTask != nil {
				oldPlatform.register.platformKeepAliveTask.Ticker.Reset(time.Hour * 999999)
			}
		}
	}

	resp.Code = 0
	resp.Message = "success"
	return resp, nil
}

// DeletePlatform 实现删除平台信息
func (gb *GB28181Plugin) DeletePlatform(ctx context.Context, req *pb.DeletePlatformRequest) (*pb.BaseResponse, error) {
	resp := &pb.BaseResponse{}
	if platform, ok := gb.platforms.Get(req.Id); ok {
		platform.PlatformModel.DeletedAt = gorm.DeletedAt{Time: time.Now(), Valid: true}
		platform.Stop(fmt.Errorf("device removed"))
		platform.WaitStopped()
		resp.Code = 0
		resp.Message = "success"
	} else {

	}
	return resp, nil
}

// ListPlatforms 实现获取平台列表
func (gb *GB28181Plugin) ListPlatforms(ctx context.Context, req *pb.ListPlatformsRequest) (*pb.PlatformsPageInfo, error) {
	resp := &pb.PlatformsPageInfo{}

	// 从内存中读取平台信息，而不是从数据库查询
	var pbPlatforms []*pb.Platform
	var filteredPlatforms []*Platform

	// 遍历内存中的平台集合
	gb.platforms.Range(func(platform *Platform) bool {
		gb.Info(platform.PlatformModel.Name)
		// 应用筛选条件（统一使用 req.Query，且不区分大小写）
		if req.Query != "" {
			q := strings.ToLower(strings.TrimSpace(req.Query))
			name := strings.ToLower(platform.PlatformModel.Name)
			server := strings.ToLower(platform.PlatformModel.ServerGBID)
			device := strings.ToLower(platform.PlatformModel.DeviceGBID)
			if !strings.Contains(name, q) && !strings.Contains(server, q) && !strings.Contains(device, q) {
				return true // 继续遍历
			}
		}

		// 如果需要筛选在线平台
		if req.Enable != -1 && req.Enable != int32(util.Conditional(platform.PlatformModel.Enable, 1, 0)) {
			return true // 继续遍历
		}

		// 添加到过滤后的平台列表
		filteredPlatforms = append(filteredPlatforms, platform)
		return true
	})

	// 计算总数
	total := len(filteredPlatforms)
	resp.Total = int32(total)

	// 按ServerGBID对平台列表进行排序
	sort.Slice(filteredPlatforms, func(i, j int) bool {
		return filteredPlatforms[i].PlatformModel.ServerGBID < filteredPlatforms[j].PlatformModel.ServerGBID
	})

	// 处理分页
	var pagePlatforms []*Platform
	if req.Page > 0 && req.Count > 0 {
		// 计算起始和结束索引
		start := int(req.Page-1) * int(req.Count)
		end := start + int(req.Count)

		// 边界检查
		if start >= total {
			// 超出范围，返回空列表
			resp.Code = 0
			resp.Message = "success"
			resp.Data = pbPlatforms
			return resp, nil
		}

		if end > total {
			end = total
		}

		// 应用分页
		pagePlatforms = filteredPlatforms[start:end]
	} else {
		// 不分页，返回所有数据
		pagePlatforms = filteredPlatforms
	}

	// 转换为proto消息
	for _, p := range pagePlatforms {
		pbPlatforms = append(pbPlatforms, &pb.Platform{
			Enable:                  p.PlatformModel.Enable,
			Name:                    p.PlatformModel.Name,
			ServerGBId:              p.PlatformModel.ServerGBID,
			ServerGBDomain:          p.PlatformModel.ServerGBDomain,
			ServerIp:                p.PlatformModel.ServerIP,
			ServerPort:              int32(p.PlatformModel.ServerPort),
			DeviceGBId:              p.PlatformModel.DeviceGBID,
			DeviceGBDomain:          p.PlatformModel.DeviceGBDomain,
			DeviceIp:                p.PlatformModel.DeviceIP,
			DevicePort:              int32(p.PlatformModel.DevicePort),
			Username:                p.PlatformModel.Username,
			Password:                p.PlatformModel.Password,
			Expires:                 int32(p.PlatformModel.Expires),
			RegisterInterval:        int32(p.PlatformModel.RegisterInterval),
			KeepTimeout:             int32(p.PlatformModel.KeepTimeout),
			MaxTimeoutCount:         int32(p.PlatformModel.MaxTimeoutCount),
			Transport:               p.PlatformModel.Transport,
			CharacterSet:            p.PlatformModel.CharacterSet,
			Ptz:                     p.PlatformModel.PTZ,
			Rtcp:                    p.PlatformModel.RTCP,
			Status:                  p.PlatformModel.Status,
			ChannelCount:            int32(p.PlatformModel.ChannelCount),
			CatalogSubscribe:        p.PlatformModel.CatalogSubscribe,
			AlarmSubscribe:          p.PlatformModel.AlarmSubscribe,
			MobilePositionSubscribe: p.PlatformModel.MobilePositionSubscribe,
			CatalogGroup:            int32(p.PlatformModel.CatalogGroup),
			UpdateTime:              p.PlatformModel.UpdateTime,
			CreateTime:              p.PlatformModel.CreateTime,
			AsMessageChannel:        p.PlatformModel.AsMessageChannel,
			SendStreamIp:            p.PlatformModel.SendStreamIp,
			AutoPushChannel:         p.PlatformModel.AutoPushChannel,
			CatalogWithPlatform:     int32(p.PlatformModel.CatalogWithPlatform),
			CatalogWithGroup:        int32(p.PlatformModel.CatalogWithGroup),
			CatalogWithRegion:       int32(p.PlatformModel.CatalogWithRegion),
			CivilCode:               p.PlatformModel.CivilCode,
			Manufacturer:            p.PlatformModel.Manufacturer,
			Model:                   p.PlatformModel.Model,
			Address:                 p.PlatformModel.Address,
			RegisterWay:             int32(p.PlatformModel.RegisterWay),
			Secrecy:                 int32(p.PlatformModel.Secrecy),
			ReadOnly:                p.PlatformModel.ReadOnly,
		})
	}

	resp.Data = pbPlatforms
	resp.Code = 0
	resp.Message = "success"
	return resp, nil
}

// QueryRecord 实现录像查询接口
func (gb *GB28181Plugin) QueryRecord(ctx context.Context, req *pb.QueryRecordRequest) (*pb.QueryRecordResponse, error) {
	resp := &pb.QueryRecordResponse{
		Code:    0,
		Message: "",
		Data:    []*pb.RecordItem{},
	}
	startTime, endTime, err := util.TimeRangeQueryParse(url.Values{"range": []string{req.Range}, "start": []string{req.Start}, "end": []string{req.End}})
	if err != nil {
		resp.Code = 400
		resp.Message = err.Error()
		return resp, nil
	}
	// 获取设备和通道
	device, ok := gb.devices.Get(req.DeviceId)
	if !ok {
		resp.Code = 404
		resp.Message = "device not found"
		return resp, nil
	}

	channel, ok := device.channels.Get(req.DeviceId + "_" + req.ChannelId)
	if !ok {
		resp.Code = 404
		resp.Message = "channel not found"
		return resp, nil
	}

	// 生成随机序列号
	sn := int(time.Now().UnixNano() / 1e6 % 1000000)

	// 发送录像查询请求
	promise, err := gb.RecordInfoQuery(req.DeviceId, req.ChannelId, startTime, endTime, sn)
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
		resp.SumNum = int32(recordReq.SumNum)
		if !firstResponse.LastTime.IsZero() {
			resp.LastTime = timestamppb.New(firstResponse.LastTime)
		}
	}

	for _, record := range recordReq.Response {
		if len(record.RecordList.Item) == 0 {
			continue
		}
		for _, item := range record.RecordList.Item {
			// 过滤无效的记录（所有字段都为空）
			if item.DeviceID == "" && item.StartTime == "" {
				continue
			}
			resp.Data = append(resp.Data, &pb.RecordItem{
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

	resp.Count = int32(recordReq.SumNum)
	resp.Code = 0
	resp.Message = fmt.Sprintf("success, received %d/%d records", recordReq.ReceivedNum, recordReq.SumNum)

	// 排序录像列表，按StartTime降序排序（最新的在前）
	sort.Slice(resp.Data, func(i, j int) bool {
		return resp.Data[i].StartTime > resp.Data[j].StartTime
	})

	// 清理请求
	channel.RecordReqs.Remove(recordReq)

	return resp, nil
}

// PtzControl 实现云台控制功能
func (gb *GB28181Plugin) PtzControl(ctx context.Context, req *pb.PtzControlRequest) (*pb.BaseResponse, error) {
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

	// 获取设备
	device, ok := gb.devices.Get(req.DeviceId)
	if !ok {
		resp.Code = 404
		resp.Message = "设备不存在"
		return resp, nil
	}

	// 调用设备的前端控制命令
	response, err := device.frontEndCmd(req.ChannelId, req.Ptzcmd)
	if err != nil {
		resp.Code = 500
		resp.Message = fmt.Sprintf("发送云台控制命令失败: %v", err)
		return resp, nil
	}

	gb.Info("云台控制",
		"deviceId", req.DeviceId,
		"channelId", req.ChannelId,
		"Ptzcmd", req.Ptzcmd,
		"response", response.String())

	resp.Code = 0
	resp.Message = "success"
	return resp, nil
}

// GetServerConfig 实现获取服务器基本配置功能
func (gb *GB28181Plugin) GetServerConfig(ctx context.Context, req *emptypb.Empty) (*pb.ServerConfigResponse, error) {
	resp := &pb.ServerConfigResponse{
		Code:    0,
		Message: "success",
		Data:    &pb.ServerConfigData{},
	}

	// 获取本地IP列表
	localIPs, err := gb.getLocalIPs()
	if err != nil {
		gb.Error("Failed to get local IPs", "error", err)
		// 不返回错误，继续返回其他配置
	} else {
		resp.Data.LocalIPs = localIPs
	}

	// 获取设备国标编号和域
	resp.Data.DeviceGBId = gb.Serial
	resp.Data.DeviceGBDomain = gb.Realm

	// 获取本地端口
	resp.Data.LocalPort = int32(gb.defaultSipPort)

	return resp, nil
}

// getLocalIPs 获取本地所有网络接口的IP地址
func (gb *GB28181Plugin) getLocalIPs() ([]string, error) {
	var ips []string

	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range interfaces {
		// 跳过down状态和loopback接口
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			// 只获取IPv4地址，且不是回环地址
			if ip != nil && ip.To4() != nil && !ip.IsLoopback() {
				ips = append(ips, ip.String())
			}
		}
	}

	return ips, nil
}

// TestSip 实现测试SIP连接功能
func (gb *GB28181Plugin) TestSip(ctx context.Context, req *pb.TestSipRequest) (*pb.TestSipResponse, error) {
	resp := &pb.TestSipResponse{
		Code:    0,
		Message: "success",
	}

	// 创建一个临时设备用于测试
	device := &Device{
		DeviceId:   "34020000002000000001",
		SipIp:      "192.168.1.106",
		LocalPort:  5060,
		Port:       5060,
		IP:         "192.168.1.102",
		StreamMode: "TCP-PASSIVE",
	}
	//From: <sip:41010500002000000001@4101050000>;tag=4183af2ecc934758ad393dfe588f2dfd
	// 初始化设备的SIP相关字段
	device.fromHDR = sip.FromHeader{
		Address: sip.Uri{
			User: "41010500002000000001",
			Host: "4101050000",
		},
		Params: sip.NewParams(),
	}
	device.fromHDR.Params.Add("tag", "4183af2ecc934758ad393dfe588f2dfd")
	//Contact: <sip:41010500002000000001@192.168.1.106:5060>
	device.contactHDR = sip.ContactHeader{
		Address: sip.Uri{
			User: "41010500002000000001",
			Host: "192.168.1.106",
			Port: 5060,
		},
	}

	//Request-Line: INVITE sip:34020000001320000006@192.168.1.102:5060 SIP/2.0
	//    Method: INVITE
	//    Request-URI: sip:34020000001320000006@192.168.1.102:5060
	//    [Resent Packet: False]
	// 根据设备的SipIp、LocalPort和Transport获取或创建对应的Client
	// 测试默认使用UDP
	client, err := gb.getOrCreateClient(device.SipIp, device.LocalPort, "UDP")
	if err != nil {
		resp.Code = 500
		resp.Message = fmt.Sprintf("创建Client失败: %v", err)
		return resp, nil
	}
	device.client = client

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
		fmt.Sprintf("o=%s 0 0 IN IP4 %s", "34020000001320000102", "192.168.1.106"),
		"s=Play",
		"c=IN IP4 192.168.1.106",
		"t=0 0",
		"m=video 40940 TCP/RTP/AVP 96 97 98 99",
		"a=recvonly",
		"a=rtpmap:96 PS/90000",
		"a=rtpmap:98 H264/90000",
		"a=rtpmap:97 MPEG4/90000",
		"a=rtpmap:99 H265/90000",
		"a=setup:passive",
		"a=connection:new",
		"y=0105006213",
	}

	// 设置必需的头部
	contentTypeHeader := sip.ContentTypeHeader("APPLICATION/SDP")
	//Subject: 34020000001320000006:0105006213,41010500002000000001:0
	subjectHeader := sip.NewHeader("Subject", "34020000001320000006:0105006213,41010500002000000001:0")

	//To: <sip:34020000001320000006@192.168.1.102:5060>
	toHeader := sip.ToHeader{
		Address: sip.Uri{
			User: "34020000001320000006",
			Host: "192.168.1.102",
			Port: 5060,
		},
	}
	userAgentHeader := sip.NewHeader("User-Agent", "WVP-Pro v2.7.3.20241218")
	// 不手动添加Via头部，让Client自动创建

	csqHeader := sip.CSeqHeader{
		SeqNo:      3,
		MethodName: "INVITE",
	}
	maxforward := sip.MaxForwardsHeader(70)
	//contentLengthHeader := sip.ContentLengthHeader(288)
	request.AppendHeader(&contentTypeHeader)
	request.AppendHeader(subjectHeader)
	request.AppendHeader(&toHeader)
	request.AppendHeader(userAgentHeader)

	// 设置消息体
	request.SetBody([]byte(strings.Join(sdpInfo, "\r\n") + "\r\n"))

	// 创建会话并发送请求
	dialogClientCache := sipgo.NewDialogClientCache(device.client, device.contactHDR)
	session, err := dialogClientCache.Invite(gb, recipient, request.Body(), &csqHeader, &device.fromHDR, &toHeader, &maxforward, userAgentHeader, &device.contactHDR, subjectHeader, &contentTypeHeader)
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
func (gb *GB28181Plugin) GetDeviceAlarm(ctx context.Context, req *pb.GetDeviceAlarmRequest) (*pb.DeviceAlarmResponse, error) {
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

	// 获取符合条件的总记录数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		resp.Code = 500
		resp.Message = fmt.Sprintf("查询总数失败: %v", err)
		return resp, nil
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
		resp.Data = append(resp.Data, alarmInfo)
	}

	// 在消息中添加总记录数信息
	resp.Message = fmt.Sprintf("success, total: %d", total)

	return resp, nil
}

// AddPlatformChannel 实现添加平台通道
func (gb *GB28181Plugin) AddPlatformChannel(ctx context.Context, req *pb.AddPlatformChannelRequest) (*pb.BaseResponse, error) {
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
			PlatformServerGBID: req.PlatformId,
			ChannelDBID:        channelId,
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
	if platform, ok := gb.platforms.Get(req.PlatformId); !ok {
		for _, channelId := range req.ChannelIds {
			if channel, ok := gb.channels.Get(channelId); ok {
				platform.channels.Set(channel)
			}
		}
		platform.PlatformModel.ChannelCount = platform.channels.Length
	}

	resp.Code = 0
	resp.Message = "success"
	return resp, nil
}

// GetPlatformChannels 根据平台ID分页查询平台下的通道列表（支持已/未共享与模糊查询）
func (gb *GB28181Plugin) GetPlatformChannels(ctx context.Context, req *pb.GetPlatformChannelsRequest) (*pb.PlatformChannelsResponse, error) {
	resp := &pb.PlatformChannelsResponse{
		Code:    0,
		Message: "success",
		Data:    []*pb.PlatformChannel{},
	}

	// 规范化查询参数
	q := strings.ToLower(strings.TrimSpace(req.Query))
	page := int(req.Page)
	count := int(req.Count)

	// 构建平台内已共享通道的集合（key 为通道的复合 ID）
	platformChannelIDs := make(map[string]struct{})
	var sharedList []*pb.PlatformChannel
	platform, platformExists := gb.platforms.Get(req.PlatformId)
	if platformExists {
		for ch := range platform.channels.Range {
			platformChannelIDs[ch.ID] = struct{}{}
			// 判断查询匹配
			if q != "" {
				if !strings.Contains(strings.ToLower(ch.ID), q) &&
					!strings.Contains(strings.ToLower(ch.CustomChannelId), q) &&
					!strings.Contains(strings.ToLower(ch.Name), q) &&
					!strings.Contains(strings.ToLower(ch.CustomName), q) {
					continue
				}
			}
			// 根据 status 过滤：-1 全部, 0 离线, 1 在线
			if req.Status != -1 {
				// 自定义通道：检查通道状态
				isOnline := ch.Status == gb28181.ChannelOnStatus
				// 如果需要在线但实际离线，或者需要离线但实际在线，则跳过
				if (req.Status == 1 && !isOnline) || (req.Status == 0 && isOnline) {
					continue
				}
			}
			// 获取设备名称（若有），channel 结构里含 Device 指针
			deviceName := ""
			if ch.Device != nil {
				deviceName = ch.Device.Name
			}
			sharedList = append(sharedList, &pb.PlatformChannel{
				Id: ch.ID,
				// ChannelId 使用通道自身的 channelId 字段（前端列表显示）
				ChannelId:         ch.ChannelId,
				CustomChannelId:   ch.CustomChannelId,
				ChannelName:       ch.Name,
				CustomChannelName: util.Conditional(ch.CustomName == "", ch.Name, ch.CustomName),
				DeviceId:          ch.DeviceId,
				DeviceName:        deviceName,
				StreamPath:        ch.StreamPath,
				InPlatform:        true,
				Status:            string(ch.Status),
				ChannelType: func() string {
					if ch.Device != nil {
						return "设备通道"
					}
					return "自定义通道"
				}(),
			})
		}
	}

	// 构建未共享通道列表（所有在 gb.channels 但不在平台集合中的通道）
	var unsharedList []*pb.PlatformChannel
	for ch := range gb.channels.Range {
		// 如果在平台集合内，则跳过（已共享）
		if _, ok := platformChannelIDs[ch.ID]; ok {
			continue
		}
		// 只显示状态为正常的通道（ChannelOnStatus）
		//if ch.Status != gb28181.ChannelOnStatus {
		//	continue
		//}
		// 根据 status 过滤：-1 全部, 0 离线, 1 在线
		if req.Status != -1 {
			isOnline := ch.Status == gb28181.ChannelOnStatus
			// 如果需要在线但实际离线，或者需要离线但实际在线，则跳过
			if (req.Status == 1 && !isOnline) || (req.Status == 0 && isOnline) {
				continue
			}
		}
		// 根据 query 过滤
		if q != "" {
			if !strings.Contains(strings.ToLower(ch.ChannelId), q) &&
				!strings.Contains(strings.ToLower(ch.CustomChannelId), q) &&
				!strings.Contains(strings.ToLower(ch.Name), q) &&
				!strings.Contains(strings.ToLower(ch.CustomName), q) {
				continue
			}
		}
		deviceName := ""
		if ch.Device != nil {
			deviceName = ch.Device.Name
		}
		unsharedList = append(unsharedList, &pb.PlatformChannel{
			Id: ch.ID,
			// ChannelId 使用通道自身的 channelId 字段（前端列表显示）
			ChannelId:         ch.ChannelId,
			CustomChannelId:   ch.CustomChannelId,
			ChannelName:       ch.Name,
			CustomChannelName: util.Conditional(ch.CustomName == "", ch.Name, ch.CustomName),
			DeviceId:          ch.DeviceId,
			DeviceName:        deviceName,
			StreamPath:        ch.StreamPath,
			InPlatform:        false,
			Status:            string(ch.Status),
			ChannelType: func() string {
				if ch.Device != nil {
					return "设备通道"
				}
				return "自定义通道"
			}(),
		})
	}

	// 根据 req.Shared 决定返回哪一部分： -1 全部, 1 已共享, 0 未共享
	var combined []*pb.PlatformChannel
	switch req.Shared {
	case 1:
		combined = sharedList
	case 0:
		combined = unsharedList
	default:
		// -1 or other: return all, but ensure InPlatform flag set correctly
		combined = append([]*pb.PlatformChannel{}, sharedList...)
		combined = append(combined, unsharedList...)
	}

	// 排序：按 channelId 升序
	sort.Slice(combined, func(i, j int) bool {
		return combined[i].ChannelId < combined[j].ChannelId
	})

	// 总数
	total := len(combined)
	resp.Total = int32(total)

	// 分页处理：当 page==0 && count==0 时视为不分页，返回全部
	if page > 0 && count > 0 {
		start := (page - 1) * count
		if start >= total {
			// 超出范围，返回空列表
			resp.Data = []*pb.PlatformChannel{}
			return resp, nil
		}
		end := start + count
		if end > total {
			end = total
		}
		resp.Data = combined[start:end]
	} else {
		resp.Data = combined
	}

	return resp, nil
}

// ChannelManageList 实现管理端的通道分页查询，数据来自内存 gb.channels，返回 ChannelsPageInfo（使用 pb.Channel）
func (gb *GB28181Plugin) ChannelManageList(ctx context.Context, req *pb.GetChannelManageListRequest) (*pb.ChannelsPageInfo, error) {
	resp := &pb.ChannelsPageInfo{
		Code:    0,
		Message: "success",
		Data:    []*pb.Channel{},
	}

	q := strings.ToLower(strings.TrimSpace(req.Query))
	var list []*pb.Channel

	// 遍历内存中的所有通道
	for ch := range gb.channels.Range {
		// 过滤条件：query 匹配 channelId/customChannelId/name/customName
		if q != "" {
			if !strings.Contains(strings.ToLower(ch.ChannelId), q) &&
				!strings.Contains(strings.ToLower(ch.CustomChannelId), q) &&
				!strings.Contains(strings.ToLower(ch.Name), q) &&
				!strings.Contains(strings.ToLower(ch.CustomName), q) &&
				!strings.Contains(strings.ToLower(ch.DeviceId), q) {
				continue
			}
		}

		// 类型过滤：-1 全部，0 设备通道（ch.Device != nil），1 自定义通道（ch.Device == nil）
		isDeviceChannel := ch.Device != nil
		if req.Type == 0 && !isDeviceChannel {
			continue
		}
		if req.Type == 1 && isDeviceChannel {
			continue
		}

		// 构建 pb.Channel（使用 proto 中的 Channel 结构）
		list = append(list, &pb.Channel{
			Id: ch.ID,
			DeviceId: func() string {
				if ch.Device == nil {
					return ch.DeviceId
				}
				return ch.Device.DeviceId
			}(),
			ChannelId:         ch.ChannelId,
			ParentId:          ch.ParentId,
			Name:              ch.Name,
			Manufacturer:      ch.Manufacturer,
			Model:             ch.Model,
			Owner:             ch.Owner,
			CivilCode:         ch.CivilCode,
			Address:           ch.Address,
			Port:              int32(ch.Port),
			Parental:          int32(ch.Parental),
			SafetyWay:         int32(ch.SafetyWay),
			RegisterWay:       int32(ch.RegisterWay),
			Secrecy:           int32(ch.Secrecy),
			Status:            string(ch.Status),
			Longitude:         ch.Longitude,
			Latitude:          ch.Latitude,
			StreamPath:        ch.StreamPath,
			CustomChannelId:   ch.CustomChannelId,
			CustomChannelName: util.Conditional(ch.CustomName == "", ch.Name, ch.CustomName),
		})
	}

	// 排序（按 ChannelId 升序）
	sort.Slice(list, func(i, j int) bool {
		return list[i].ChannelId < list[j].ChannelId
	})

	total := len(list)
	resp.Total = int32(total)

	// 分页处理：page/count 为 0 表示不分页
	if req.Page > 0 && req.Count > 0 {
		start := int((req.Page - 1) * req.Count)
		if start >= total {
			resp.Data = []*pb.Channel{}
			return resp, nil
		}
		end := start + int(req.Count)
		if end > total {
			end = total
		}
		resp.Data = list[start:end]
	} else {
		resp.Data = list
	}

	return resp, nil
}

// AddChannel 在内存和数据库中新增通道（管理端）
func (gb *GB28181Plugin) AddChannel(ctx context.Context, req *pb.AddChannelRequest) (*pb.BaseResponse, error) {
	resp := &pb.BaseResponse{}

	// 必填校验
	if req.ChannelId == "" || req.ChannelName == "" || req.StreamPath == "" {
		resp.Code = 400
		resp.Message = "channelId、channelName、streamPath 为必填项"
		return resp, nil
	}

	if gb.DB == nil {
		resp.Code = 500
		resp.Message = "database not initialized"
		return resp, nil
	}

	// 生成复合ID：deviceId_channelId 或 channelId_channelId（如果 deviceId 为空）
	baseDevice := req.DeviceId
	if baseDevice == "" {
		baseDevice = req.ChannelId
	}
	channelID := baseDevice + "_" + req.ChannelId

	// 先在内存中检查是否已存在（以复合ID为主键）
	if _, ok := gb.channels.Get(channelID); ok {
		resp.Code = 409
		resp.Message = "通道已存在"
		return resp, nil
	}

	// 计算最终的自定义通道ID（用于唯一性校验）
	finalCustomId := req.CustomChannelId
	if finalCustomId == "" {
		finalCustomId = req.ChannelId
	}

	// 在内存中检查 customChannelId 是否重复
	hasConflict := false
	gb.channels.Range(func(ch *Channel) bool {
		if ch == nil || ch.DeviceChannel == nil {
			return true
		}
		if ch.ID == channelID {
			return true
		}
		if ch.DeviceChannel.CustomChannelId == finalCustomId {
			hasConflict = true
			return false
		}
		return true
	})
	if hasConflict {
		resp.Code = 409
		resp.Message = "自定义通道ID已存在，请使用其他ID"
		return resp, nil
	}

	// 再在数据库中检查 id 与 custom_channel_id 是否重复（以防并发或 DB 中已有旧数据）
	var existingById gb28181.DeviceChannel
	if err := gb.DB.Where("id = ?", channelID).First(&existingById).Error; err == nil {
		resp.Code = 409
		resp.Message = "通道已存在"
		return resp, nil
	} else if err != gorm.ErrRecordNotFound {
		resp.Code = 500
		resp.Message = fmt.Sprintf("查询通道失败: %v", err)
		return resp, nil
	}
	var existingByCustom gb28181.DeviceChannel
	if err := gb.DB.Where("custom_channel_id = ?", finalCustomId).First(&existingByCustom).Error; err == nil {
		resp.Code = 409
		resp.Message = "自定义通道ID已存在，请使用其他ID"
		return resp, nil
	} else if err != gorm.ErrRecordNotFound {
		resp.Code = 500
		resp.Message = fmt.Sprintf("查询自定义通道ID失败: %v", err)
		return resp, nil
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	deviceChannel := &gb28181.DeviceChannel{
		ID:              channelID,
		DeviceId:        baseDevice,
		ChannelId:       req.ChannelId,
		CustomChannelId: finalCustomId,
		Name:            req.ChannelName,
		CustomName: func() string {
			if req.CustomChannelName != "" {
				return req.CustomChannelName
			}
			return req.ChannelName
		}(),
		StreamPath: req.StreamPath,
		CreateTime: now,
		Status:     gb28181.ChannelOnStatus,
	}

	// 写入数据库
	if err := gb.DB.Create(deviceChannel).Error; err != nil {
		resp.Code = 500
		resp.Message = fmt.Sprintf("保存通道失败: %v", err)
		return resp, nil
	}

	// 加入内存集合（以数据库为准）
	channel := &Channel{
		DeviceChannel: deviceChannel,
		Device:        nil,
		Logger:        gb.Logger.With("channel", channelID),
	}
	gb.channels.Set(channel)

	resp.Code = 0
	resp.Message = "通道添加成功"
	return resp, nil
}

// DeleteChannel 删除管理端通道（同时删除 DB 与内存，以及平台关联）
func (gb *GB28181Plugin) DeleteChannel(ctx context.Context, req *pb.DeleteChannelRequest) (*pb.BaseResponse, error) {
	resp := &pb.BaseResponse{}
	if req.Id == "" {
		resp.Code = 400
		resp.Message = "id 不能为空"
		return resp, nil
	}

	// 从数据库删除平台通道关联关系
	if gb.DB != nil {
		if err := gb.DB.Where("channel_db_id = ?", req.Id).Delete(&gb28181.PlatformChannel{}).Error; err != nil {
			resp.Code = 500
			resp.Message = fmt.Sprintf("删除平台通道关联失败: %v", err)
			return resp, nil
		}

		// 从数据库删除通道记录
		if err := gb.DB.Where("id = ?", req.Id).Delete(&gb28181.DeviceChannel{}).Error; err != nil {
			resp.Code = 500
			resp.Message = fmt.Sprintf("删除通道失败: %v", err)
			return resp, nil
		}
	}

	// 从所有平台的内存集合中移除该通道
	for p := range gb.platforms.Range {
		p.channels.RemoveByKey(req.Id)
		// 更新平台通道数量
		p.PlatformModel.ChannelCount = p.channels.Length
	}

	// 从全局通道内存集合中移除
	if ch, ok := gb.channels.Get(req.Id); ok {
		gb.channels.RemoveByKey(ch.ID)
	}

	resp.Code = 0
	resp.Message = "通道删除成功"
	return resp, nil
}

// RemovePlatformChannel 实现从平台移出共享通道
func (gb *GB28181Plugin) RemovePlatformChannel(ctx context.Context, req *pb.RemovePlatformChannelRequest) (*pb.BaseResponse, error) {
	resp := &pb.BaseResponse{}

	// 参数校验
	if req.PlatformId == "" || req.Id == "" {
		resp.Code = 400
		resp.Message = "platformId and id are required"
		return resp, nil
	}

	// 只更新内存：如果平台已加载，移除其 channels 集合中的指定通道（使用复合主键 id）
	if platform, ok := gb.platforms.Get(req.PlatformId); ok {
		platform.channels.RemoveByKey(req.Id)
		platform.PlatformModel.ChannelCount = platform.channels.Length
		platformChannel := &gb28181.PlatformChannel{PlatformServerGBID: platform.PlatformModel.ServerGBID, ChannelDBID: req.Id}
		gb.DB.Delete(platformChannel)
		resp.Code = 0
		resp.Message = "success"
		return resp, nil
	}

	// 平台未加载于内存时，无需操作也视为成功（调用方可根据需要决定是否报错）
	resp.Code = 0
	resp.Message = "platform not loaded, nothing to do"
	return resp, nil
}

// AddPlatformChannelShared 将已有通道加入平台的共享列表（仅更新内存）
func (gb *GB28181Plugin) AddPlatformChannelShared(ctx context.Context, req *pb.AddPlatformChannelSharedRequest) (*pb.BaseResponse, error) {
	resp := &pb.BaseResponse{}

	// 参数校验
	if req.PlatformId == "" || req.Id == "" {
		resp.Code = 400
		resp.Message = "platformId and id are required"
		return resp, nil
	}

	// 平台已加载于内存则添加通道引用（使用复合主键 id 查找通道）
	if platform, ok := gb.platforms.Get(req.PlatformId); ok {
		if ch, ok := gb.channels.Get(req.Id); ok {
			platform.channels.Set(ch)
			platform.PlatformModel.ChannelCount = platform.channels.Length
			platformChannel := &gb28181.PlatformChannel{PlatformServerGBID: platform.PlatformModel.ServerGBID, ChannelDBID: req.Id}
			gb.DB.Create(platformChannel)
			resp.Code = 0
			resp.Message = "success"
			return resp, nil
		}
		// 通道未在内存中找到
		resp.Code = 404
		resp.Message = "channel not found in memory"
		return resp, nil
	}

	// 平台未加载于内存时，不做 DB 操作，直接返回提示
	resp.Code = 0
	resp.Message = "platform not loaded, nothing to do"
	return resp, nil
}

// Recording 实现录制控制功能
func (gb *GB28181Plugin) Recording(ctx context.Context, req *pb.RecordingRequest) (*pb.BaseResponse, error) {
	resp := &pb.BaseResponse{}

	// 检查命令类型是否有效
	if req.CmdType != "Record" && req.CmdType != "RecordStop" {
		resp.Code = 400
		resp.Message = "无效的命令类型，只能是 Record 或 RecordStop"
		return resp, nil
	}

	// 1. 先在 platforms 中查找设备
	if platform, ok := gb.platforms.Get(req.DeviceId); ok {
		if gb.DB == nil {
			resp.Code = 500
			resp.Message = "数据库未初始化"
			return resp, nil
		}

		// 使用SQL查询获取实际的设备ID和通道ID
		var result struct {
			DeviceID  string
			ChannelID string
		}
		err := gb.DB.Raw(`
			SELECT gc.device_id, gc.channel_id as channelid
			FROM gb28181_platform gp
			LEFT JOIN gb28181_platform_channel gpc on gpc.platform_server_gb_id = pg.server_gb_id
			LEFT JOIN gb28181_channel gc on gc.id = gpc.channel_db_id
			WHERE gp.device_gb_id = ? AND gc.channel_id = ?`,
			req.DeviceId, req.ChannelId,
		).Scan(&result).Error

		if err != nil {
			resp.Code = 500
			resp.Message = fmt.Sprintf("查询设备通道关系失败: %v", err)
			return resp, nil
		}

		if result.DeviceID == "" || result.ChannelID == "" {
			resp.Code = 404
			resp.Message = "未找到对应的设备和通道信息"
			return resp, nil
		}

		// 从gb.devices中查找实际设备
		actualDevice, ok := gb.devices.Get(result.DeviceID)
		if !ok {
			resp.Code = 404
			resp.Message = "实际设备未找到"
			return resp, nil
		}

		// 从device.channels中查找实际通道
		_, ok = actualDevice.channels.Get(result.DeviceID + "_" + result.ChannelID)
		if !ok {
			resp.Code = 404
			resp.Message = "实际通道未找到"
			return resp, nil
		}

		// 发送录制控制命令
		response, err := actualDevice.recordCmd(result.ChannelID, req.CmdType)
		if err != nil {
			resp.Code = 500
			resp.Message = fmt.Sprintf("发送录制控制命令失败: %v", err)
			return resp, nil
		}

		gb.Info("通过平台控制录制",
			"command", req.CmdType,
			"platformDeviceId", req.DeviceId,
			"platformChannelId", req.ChannelId,
			"actualDeviceId", result.DeviceID,
			"actualChannelId", result.ChannelID,
			"ServerGBID", platform.PlatformModel.ServerGBID,
			"response", response.String())
	} else {
		// 2. 如果在平台中没找到，则在本地设备中查找
		device, ok := gb.devices.Get(req.DeviceId)
		if !ok {
			resp.Code = 404
			resp.Message = "设备未找到"
			return resp, nil
		}

		// 检查通道是否存在
		_, ok = device.channels.Get(req.DeviceId + "_" + req.ChannelId)
		if !ok {
			resp.Code = 404
			resp.Message = "通道未找到"
			return resp, nil
		}

		// 发送录制控制命令
		response, err := device.recordCmd(req.ChannelId, req.CmdType)
		if err != nil {
			resp.Code = 500
			resp.Message = fmt.Sprintf("发送录制控制命令失败: %v", err)
			return resp, nil
		}

		gb.Info("控制录制",
			"command", req.CmdType,
			"deviceId", req.DeviceId,
			"channelId", req.ChannelId,
			"response", response.String())
	}

	resp.Code = 0
	resp.Message = "success"
	return resp, nil
}

// GetSnap 实现抓拍功能
func (gb *GB28181Plugin) GetSnap(ctx context.Context, req *pb.GetSnapRequest) (*pb.SnapResponse, error) {
	resp := &pb.SnapResponse{}

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

	// 1. 先在 platforms 中查找设备
	if platform, ok := gb.platforms.Get(req.DeviceId); ok {
		if gb.DB == nil {
			resp.Code = 500
			resp.Message = "数据库未初始化"
			return resp, nil
		}

		// 使用SQL查询获取实际的设备ID和通道ID
		var result struct {
			DeviceID  string
			ChannelID string
		}
		err := gb.DB.Raw(`
			SELECT gc.device_id, gc.channel_id as channelid
			FROM gb28181_platform gp
			LEFT JOIN gb28181_platform_channel gpc on gpc.platform_server_gb_id = pg.server_gb_id
			LEFT JOIN gb28181_channel gc on gc.id = gpc.channel_db_id
			WHERE gp.device_gb_id = ? AND gc.channel_id = ?`,
			req.DeviceId, req.ChannelId,
		).Scan(&result).Error

		if err != nil {
			resp.Code = 500
			resp.Message = fmt.Sprintf("查询设备通道关系失败: %v", err)
			return resp, nil
		}

		if result.DeviceID == "" || result.ChannelID == "" {
			resp.Code = 404
			resp.Message = "未找到对应的设备和通道信息"
			return resp, nil
		}

		// 从gb.devices中查找实际设备
		actualDevice, ok := gb.devices.Get(result.DeviceID)
		if !ok {
			resp.Code = 404
			resp.Message = "实际设备未找到"
			return resp, nil
		}

		// 从device.channels中查找实际通道
		_, ok = actualDevice.channels.Get(result.DeviceID + "_" + result.ChannelID)
		if !ok {
			resp.Code = 404
			resp.Message = "实际通道未找到"
			return resp, nil
		}

		// 构建抓拍配置
		config := SnapshotConfig{
			SnapNum:   1, // 默认抓拍1张
			Interval:  1, // 默认间隔1秒
			UploadURL: fmt.Sprintf("http://%s%s/gb28181/api/snap/upload", actualDevice.SipIp, gb.GetCommonConf().HTTP.ListenAddr),
			SessionID: fmt.Sprintf("%d", time.Now().UnixNano()),
		}

		// 生成XML并发送请求
		xmlBody := actualDevice.BuildSnapshotConfigXML(config, result.ChannelID)
		request := actualDevice.CreateRequest(sip.MESSAGE, nil)
		request.SetBody([]byte(xmlBody))
		response, err := actualDevice.send(request)
		if err != nil {
			resp.Code = 500
			resp.Message = fmt.Sprintf("发送抓拍配置命令失败: %v", err)
			return resp, nil
		}

		gb.Info("通过平台配置抓拍",
			"platformDeviceId", req.DeviceId,
			"platformChannelId", req.ChannelId,
			"actualDeviceId", result.DeviceID,
			"actualChannelId", result.ChannelID,
			"ServerGBID", platform.PlatformModel.ServerGBID,
			"response", response.String())

	} else {
		// 2. 如果在平台中没找到，则在本地设备中查找
		device, ok := gb.devices.Get(req.DeviceId)
		if !ok {
			resp.Code = 404
			resp.Message = "设备未找到"
			return resp, nil
		}

		// 检查通道是否存在
		_, ok = device.channels.Get(req.DeviceId + "_" + req.ChannelId)
		if !ok {
			resp.Code = 404
			resp.Message = "通道未找到"
			return resp, nil
		}

		// 构建抓拍配置
		config := SnapshotConfig{
			SnapNum:   1, // 默认抓拍1张
			Interval:  1, // 默认间隔1秒
			UploadURL: fmt.Sprintf("http://%s%s/gb28181/api/snap/upload", device.SipIp, gb.GetCommonConf().HTTP.ListenAddr),
			SessionID: fmt.Sprintf("%d", time.Now().UnixNano()),
		}

		// 生成XML并发送请求
		xmlBody := device.BuildSnapshotConfigXML(config, req.ChannelId)
		request := device.CreateRequest(sip.MESSAGE, nil)
		request.SetBody([]byte(xmlBody))
		response, err := device.send(request)
		if err != nil {
			resp.Code = 500
			resp.Message = fmt.Sprintf("发送抓拍配置命令失败: %v", err)
			return resp, nil
		}

		gb.Info("配置抓拍",
			"deviceId", req.DeviceId,
			"channelId", req.ChannelId,
			"response", response.String())
	}

	resp.Code = 0
	resp.Message = "success"
	return resp, nil
}

// GetGroupChannels 获取分组下的通道列表
func (gb *GB28181Plugin) GetGroupChannels(ctx context.Context, req *pb.GetGroupChannelsRequest) (*pb.GroupChannelsResponse, error) {
	resp := &pb.GroupChannelsResponse{}
	groupChannelsData := &pb.GroupChannelsData{}
	// 检查数据库连接
	if gb.DB == nil {
		resp.Code = 500
		resp.Message = "数据库未初始化"
		return resp, nil
	}

	// 验证分组ID参数
	if req.GroupId <= 0 {
		resp.Code = 400
		resp.Message = "分组ID无效"
		return resp, nil
	}

	// 检查分组是否存在
	var group gb28181.GroupsModel
	if err := gb.DB.First(&group, req.GroupId).Error; err != nil {
		resp.Code = 404
		resp.Message = fmt.Sprintf("分组不存在: %v", err)
		return resp, nil
	}

	// 从数据库获取当前分组下的通道关联信息
	var groupChannels []gb28181.GroupsChannelModel
	if err := gb.DB.Where("group_id = ?", req.GroupId).Find(&groupChannels).Error; err != nil {
		resp.Code = 500
		resp.Message = fmt.Sprintf("查询分组通道关联失败: %v", err)
		return resp, nil
	}

	// 创建一个映射，用于快速查找通道是否在分组中
	inGroupMap := make(map[string]int64) // key: deviceId+channelId, value: 关联ID
	for _, gc := range groupChannels {
		key := gc.DeviceID + ":" + gc.ChannelID
		inGroupMap[key] = int64(gc.ID)
	}

	// 准备结果集
	var results []*pb.GroupChannel
	var filteredChannels []*Channel

	// 从内存中获取所有通道
	for channel := range gb.channels.Range {
		// 如果有设备ID过滤条件，则只处理匹配的设备
		if req.DeviceId != "" && channel.DeviceId != req.DeviceId {
			continue
		}
		filteredChannels = append(filteredChannels, channel)
	}

	// 计算总数
	total := int64(len(filteredChannels))

	// 应用排序（按设备ID和通道ID排序）
	sort.Slice(filteredChannels, func(i, j int) bool {
		if filteredChannels[i].DeviceId == filteredChannels[j].DeviceId {
			return filteredChannels[i].ChannelId < filteredChannels[j].ChannelId
		}
		return filteredChannels[i].DeviceId < filteredChannels[j].DeviceId
	})

	// 应用分页
	start, end := 0, len(filteredChannels)
	if req.Page > 0 && req.Count > 0 {
		start = int((req.Page - 1) * req.Count)
		end = int(req.Page * req.Count)
		if start >= len(filteredChannels) {
			start = len(filteredChannels)
		}
		if end > len(filteredChannels) {
			end = len(filteredChannels)
		}
	}

	// 处理分页后的通道数据
	for _, channel := range filteredChannels[start:end] {
		key := channel.DeviceId + ":" + channel.ChannelId
		id, inGroup := inGroupMap[key]

		// 获取设备名称
		deviceName := ""
		if device, ok := gb.devices.Get(channel.DeviceId); ok {
			deviceName = device.Name
		}

		channelInfo := &pb.GroupChannel{
			Id:          int32(id),
			GroupId:     req.GroupId,
			ChannelId:   channel.ChannelId,
			DeviceId:    channel.DeviceId,
			ChannelName: channel.Name,
			DeviceName:  deviceName,
			Status:      string(channel.Status),
			InGroup:     inGroup,
		}

		// 从内存中获取设备信息以获取传输协议
		if device, ok := gb.devices.Get(channel.DeviceId); ok {
			channelInfo.StreamMode = string(device.StreamMode)
		}

		results = append(results, channelInfo)
	}

	// 添加该分组下的所有通道列表（只包含channelId和deviceId）
	var allGroupChannels []*pb.GroupChannel
	for _, gc := range groupChannels {
		// 只添加确实在分组中的通道
		allGroupChannels = append(allGroupChannels, &pb.GroupChannel{
			ChannelId: gc.ChannelID,
			DeviceId:  gc.DeviceID,
			InGroup:   true,
		})
	}

	resp.Code = 0
	resp.Message = "获取通道列表成功"
	resp.Total = int32(total)
	groupChannelsData.List = results
	groupChannelsData.Channels = allGroupChannels
	resp.Data = groupChannelsData
	return resp, nil
}

// UploadJpeg 实现接收JPEG文件功能
func (gb *GB28181Plugin) UploadJpeg(ctx context.Context, req *pb.UploadJpegRequest) (*pb.BaseResponse, error) {
	gb.Info("UploadJpeg", "req", req.String())
	resp := &pb.BaseResponse{}

	// 检查图片数据是否为空
	if len(req.ImageData) == 0 {
		resp.Code = 400
		resp.Message = "图片数据不能为空"
		return resp, nil
	}

	// 生成文件名
	fileName := fmt.Sprintf("snap_%d.jpg", time.Now().UnixNano()/1e6)

	// 确保目录存在
	snapPath := "snaps"
	if err := os.MkdirAll(snapPath, 0755); err != nil {
		resp.Code = 500
		resp.Message = fmt.Sprintf("创建目录失败: %v", err)
		return resp, nil
	}

	// 保存文件
	filePath := fmt.Sprintf("%s/%s", snapPath, fileName)
	if err := os.WriteFile(filePath, req.ImageData, 0644); err != nil {
		resp.Code = 500
		resp.Message = fmt.Sprintf("保存文件失败: %v", err)
		return resp, nil
	}

	gb.Info("保存抓拍图片",
		"fileName", fileName,
		"size", len(req.ImageData))

	resp.Code = 0
	resp.Message = "success"
	return resp, nil
}

// GetGroups 实现获取子分组列表
// 根据传入的id作为父id(pid)查询其下的所有子分组
// 当pid为空或为-1时，返回所有分组
func (gb *GB28181Plugin) GetGroups(ctx context.Context, req *pb.GetGroupsRequest) (*pb.GroupsListResponse, error) {
	var groups []*pb.Group
	var dbGroups []gb28181.GroupsModel

	// 检查数据库连接
	if gb.DB == nil {
		return &pb.GroupsListResponse{
			Code:    500,
			Message: "数据库未初始化",
		}, nil
	}

	query := gb.DB
	// 如果pid为-1，查询顶层组织(pid=0)
	// 否则查询指定pid的子组织
	if req.Pid == -1 {
		query = query.Where("pid = ?", 0)
	} else {
		query = query.Where("pid = ?", req.Pid)
	}

	if err := query.Find(&dbGroups).Error; err != nil {
		return nil, err
	}

	for _, dbGroup := range dbGroups {
		// 创建组对象
		group := &pb.Group{
			Id:         int32(dbGroup.ID),
			Name:       dbGroup.Name,
			Pid:        int32(dbGroup.PID),
			Level:      int32(dbGroup.Level),
			CreateTime: timestamppb.New(dbGroup.CreateTime),
			UpdateTime: timestamppb.New(dbGroup.UpdateTime),
		}

		// 获取该组关联的通道
		channels, err := gb.getGroupChannels(int32(dbGroup.ID))
		if err != nil {
			gb.Error("获取组关联通道失败", "error", err, "groupId", dbGroup.ID)
		} else {
			// 设置该组的通道列表
			group.Channels = channels
		}

		// 递归获取子组织及其通道
		children, err := gb.getChildGroupsWithChannels(int32(dbGroup.ID))
		if err != nil {
			return nil, err
		}
		group.Children = children

		groups = append(groups, group)
	}

	// 创建响应
	resp := &pb.GroupsListResponse{
		Code:    0,
		Message: "success",
		Data:    groups,
	}

	return resp, nil
}

// getGroupChannels 获取指定组ID关联的通道列表
func (gb *GB28181Plugin) getGroupChannels(groupId int32) ([]*pb.GroupChannel, error) {
	// 从数据库中获取组-通道关联关系
	groupsChannel := &gb28181.GroupsChannelModel{}
	groupsChannelTable := groupsChannel.TableName()

	// 查询结果结构，只获取必要的ID信息
	type GroupChannelRelation struct {
		ID        int    `gorm:"column:id"`
		ChannelID string `gorm:"column:channel_id"`
		DeviceID  string `gorm:"column:device_id"`
	}

	// 从数据库中只查询关联关系
	query := gb.DB.Table(groupsChannelTable).
		Select("id, channel_id, device_id").
		Where("group_id = ?", groupId)

	var relations []GroupChannelRelation
	if err := query.Find(&relations).Error; err != nil {
		return nil, err
	}

	// 转换结果为响应格式
	var pbGroupChannels []*pb.GroupChannel
	for _, relation := range relations {
		// 构造通道ID
		channelId := relation.DeviceID + "_" + relation.ChannelID

		// 从内存中获取通道信息
		if channel, ok := gb.channels.Get(channelId); ok {
			// 从内存中的通道信息构建响应
			channelInfo := &pb.GroupChannel{
				Id:          int32(relation.ID),
				GroupId:     groupId,
				ChannelId:   relation.ChannelID,
				DeviceId:    relation.DeviceID,
				ChannelName: channel.Name,
				Status:      string(channel.Status),
				InGroup:     true,
			}

			// 从内存中获取设备信息
			if device, ok := gb.devices.Get(relation.DeviceID); ok {
				channelInfo.DeviceName = device.Name
				channelInfo.StreamMode = string(device.StreamMode)
			}

			pbGroupChannels = append(pbGroupChannels, channelInfo)
		} else {
			// 如果内存中没有找到，记录日志
			gb.Debug("通道在内存中未找到", "channelId", channelId, "groupId", groupId)
		}
	}

	return pbGroupChannels, nil
}

// 递归获取子组织及其通道
func (gb *GB28181Plugin) getChildGroupsWithChannels(parentId int32) ([]*pb.Group, error) {
	var children []*pb.Group
	var dbGroups []gb28181.GroupsModel

	// 从数据库中获取子组织结构
	if err := gb.DB.Where("pid = ?", parentId).Find(&dbGroups).Error; err != nil {
		return nil, err
	}

	for _, dbGroup := range dbGroups {
		group := &pb.Group{
			Id:         int32(dbGroup.ID),
			Name:       dbGroup.Name,
			Pid:        int32(dbGroup.PID),
			Level:      int32(dbGroup.Level),
			CreateTime: timestamppb.New(dbGroup.CreateTime),
			UpdateTime: timestamppb.New(dbGroup.UpdateTime),
		}

		// 获取该组关联的通道（使用修改后的getGroupChannels函数，从内存中获取通道信息）
		channels, err := gb.getGroupChannels(int32(dbGroup.ID))
		if err != nil {
			gb.Error("获取组关联通道失败", "error", err, "groupId", dbGroup.ID)
		} else {
			// 设置该组的通道列表
			group.Channels = channels
		}

		// 递归获取子组织及其通道
		subChildren, err := gb.getChildGroupsWithChannels(int32(dbGroup.ID))
		if err != nil {
			return nil, err
		}
		group.Children = subChildren

		children = append(children, group)
	}

	return children, nil
}

// 递归获取子组织（不包含通道信息，保留此方法以兼容其他可能的调用）
func (gb *GB28181Plugin) getChildGroups(parentId int32) ([]*pb.Group, error) {
	var children []*pb.Group
	var dbGroups []gb28181.GroupsModel

	if err := gb.DB.Where("pid = ?", parentId).Find(&dbGroups).Error; err != nil {
		return nil, err
	}

	for _, dbGroup := range dbGroups {
		group := &pb.Group{
			Id:         int32(dbGroup.ID),
			Name:       dbGroup.Name,
			Pid:        int32(dbGroup.PID),
			Level:      int32(dbGroup.Level),
			CreateTime: timestamppb.New(dbGroup.CreateTime),
			UpdateTime: timestamppb.New(dbGroup.UpdateTime),
		}

		// 递归获取子组织
		subChildren, err := gb.getChildGroups(int32(dbGroup.ID))
		if err != nil {
			return nil, err
		}
		group.Children = subChildren

		children = append(children, group)
	}

	return children, nil
}

// AddGroup 实现添加分组功能
func (gb *GB28181Plugin) AddGroup(ctx context.Context, req *pb.Group) (*pb.BaseResponse, error) {
	resp := &pb.BaseResponse{}

	// 检查数据库连接
	if gb.DB == nil {
		resp.Code = 500
		resp.Message = "数据库未初始化"
		return resp, nil
	}

	// 验证参数
	if req.Name == "" {
		resp.Code = 400
		resp.Message = "分组名称不能为空"
		return resp, nil
	}

	// 禁止添加pid为0的分组，保证只有一个根组织
	if req.Pid == 0 {
		resp.Code = 400
		resp.Message = "不能添加根级组织，系统已有一个根组织"
		return resp, nil
	}

	// 创建新的分组实例
	now := time.Now()
	group := &gb28181.GroupsModel{
		Name:       req.Name,
		PID:        int(req.Pid),
		CreateTime: now,
		UpdateTime: now,
	}

	// 检查父分组是否存在
	var parentGroup gb28181.GroupsModel
	if err := gb.DB.First(&parentGroup, req.Pid).Error; err != nil {
		resp.Code = 404
		resp.Message = fmt.Sprintf("父分组不存在: %v", err)
		return resp, nil
	}
	// 设置新分组的level为父分组level+1
	group.Level = parentGroup.Level + 1

	// 保存到数据库
	if err := gb.DB.Create(group).Error; err != nil {
		resp.Code = 500
		resp.Message = fmt.Sprintf("创建分组失败: %v", err)
		return resp, nil
	}

	// 返回成功响应
	resp.Code = 0
	resp.Message = "分组创建成功"
	return resp, nil
}

// UpdateGroup 实现更新分组功能
func (gb *GB28181Plugin) UpdateGroup(ctx context.Context, req *pb.Group) (*pb.BaseResponse, error) {
	resp := &pb.BaseResponse{}

	// 检查数据库连接
	if gb.DB == nil {
		resp.Code = 500
		resp.Message = "数据库未初始化"
		return resp, nil
	}

	// 验证参数
	if req.Id <= 0 {
		resp.Code = 400
		resp.Message = "分组ID不能为空"
		return resp, nil
	}

	if req.Name == "" {
		resp.Code = 400
		resp.Message = "分组名称不能为空"
		return resp, nil
	}

	// 禁止将分组改为根组织
	if req.Pid == 0 {
		resp.Code = 400
		resp.Message = "不能修改为根级组织，系统已有一个根组织"
		return resp, nil
	}

	// 查询现有分组
	var existingGroup gb28181.GroupsModel
	if err := gb.DB.First(&existingGroup, req.Id).Error; err != nil {
		resp.Code = 404
		resp.Message = fmt.Sprintf("分组不存在: %v", err)
		return resp, nil
	}

	// 检查是否为根组织，根组织的特殊处理
	if existingGroup.PID == 0 && existingGroup.Level == 0 {
		resp.Code = 400
		resp.Message = "根组织不能被修改"
		return resp, nil
	}

	// 如果父ID改变，需要检查新父分组是否存在
	var newLevel int
	if int(req.Pid) != existingGroup.PID {
		var parentGroup gb28181.GroupsModel
		if err := gb.DB.First(&parentGroup, req.Pid).Error; err != nil {
			resp.Code = 404
			resp.Message = fmt.Sprintf("父分组不存在: %v", err)
			return resp, nil
		}

		// 检查是否会导致循环引用（不能将一个分组的父级设置为其自身或其子级）
		if req.Id == req.Pid {
			resp.Code = 400
			resp.Message = "不能将分组的父级设置为其自身"
			return resp, nil
		}

		// 检查是否会导致循环引用（不能将一个分组的父级设置为其子级）
		var childGroups []gb28181.GroupsModel
		if err := gb.DB.Where("pid = ?", req.Id).Find(&childGroups).Error; err != nil {
			resp.Code = 500
			resp.Message = fmt.Sprintf("查询子分组失败: %v", err)
			return resp, nil
		}

		for _, child := range childGroups {
			if child.ID == int(req.Pid) {
				resp.Code = 400
				resp.Message = "不能将分组的父级设置为其子级"
				return resp, nil
			}
		}

		// 设置新的level值
		newLevel = parentGroup.Level + 1
	} else {
		// 如果父ID未改变，保持原有level
		newLevel = existingGroup.Level
	}

	// 更新分组信息
	updates := map[string]interface{}{
		"name":        req.Name,
		"pid":         req.Pid,
		"level":       newLevel,
		"update_time": time.Now(),
	}

	// 执行更新
	if err := gb.DB.Model(&existingGroup).Updates(updates).Error; err != nil {
		resp.Code = 500
		resp.Message = fmt.Sprintf("更新分组失败: %v", err)
		return resp, nil
	}

	// 如果分组的父ID发生变化且有子分组，需要递归更新所有子分组的level
	if int(req.Pid) != existingGroup.PID {
		// 更新所有子分组的level
		if err := gb.updateChildLevels(int(req.Id), newLevel); err != nil {
			gb.Error("更新子分组level失败", "error", err)
			// 这里不返回错误，因为主要更新已经成功
		}
	}

	resp.Code = 0
	resp.Message = "分组更新成功"
	return resp, nil
}

// updateChildLevels 递归更新子分组的level值
func (gb *GB28181Plugin) updateChildLevels(parentID int, parentLevel int) error {
	// 查询所有直接子分组
	var childGroups []gb28181.GroupsModel
	if err := gb.DB.Where("pid = ?", parentID).Find(&childGroups).Error; err != nil {
		return err
	}

	// 没有子分组，直接返回
	if len(childGroups) == 0 {
		return nil
	}

	// 更新每个子分组的level，并递归更新它们的子分组
	for _, child := range childGroups {
		newLevel := parentLevel + 1

		// 更新当前子分组的level
		if err := gb.DB.Model(&child).Update("level", newLevel).Error; err != nil {
			return err
		}

		// 递归更新其子分组
		if err := gb.updateChildLevels(child.ID, newLevel); err != nil {
			return err
		}
	}

	return nil
}

// AddGroupChannel 添加通道到分组
func (gb *GB28181Plugin) AddGroupChannel(ctx context.Context, req *pb.AddGroupChannelRequest) (*pb.BaseResponse, error) {
	if gb.DB == nil {
		return &pb.BaseResponse{Code: 500, Message: "数据库未初始化"}, nil
	}

	// 开始事务
	tx := gb.DB.Begin()

	// 先删除该分组下的所有通道关联
	if err := tx.Where("group_id = ?", req.GroupId).Delete(&gb28181.GroupsChannelModel{}).Error; err != nil {
		tx.Rollback()
		return &pb.BaseResponse{Code: 500, Message: fmt.Sprintf("删除分组下的所有通道关联失败: %v", err)}, nil
	}

	// 检查Channels是否为空数组
	if len(req.Channels) == 0 {
		// 如果是空数组，表示清空该分组下的所有通道关联
		// 由于前面已经删除了所有关联，这里直接提交事务即可
		if err := tx.Commit().Error; err != nil {
			return &pb.BaseResponse{Code: 500, Message: fmt.Sprintf("提交事务失败: %v", err)}, nil
		}
		return &pb.BaseResponse{Code: 0, Message: "清空分组下的所有通道关联成功"}, nil
	}

	// 遍历通道列表，为每个通道创建新的关联
	for _, channel := range req.Channels {
		newGroupChannel := &gb28181.GroupsChannelModel{
			GroupID:   int(req.GroupId),
			ChannelID: channel.ChannelId,
			DeviceID:  channel.DeviceId,
		}

		if err := tx.Create(newGroupChannel).Error; err != nil {
			tx.Rollback()
			return &pb.BaseResponse{Code: 500, Message: fmt.Sprintf("创建分组通道关联失败: %v", err)}, nil
		}
	}

	// 提交事务
	if err := tx.Commit().Error; err != nil {
		return &pb.BaseResponse{Code: 500, Message: fmt.Sprintf("提交事务失败: %v", err)}, nil
	}

	return &pb.BaseResponse{Code: 0, Message: "添加分组通道关联成功"}, nil
}

// PlaybackPause 实现回放暂停功能
func (gb *GB28181Plugin) PlaybackPause(ctx context.Context, req *pb.PlaybackPauseRequest) (*pb.BaseResponse, error) {
	resp := &pb.BaseResponse{}

	// 参数校验
	if req.StreamPath == "" {
		resp.Code = 400
		resp.Message = "流路径不能为空"
		return resp, nil
	}

	// 查找对应的dialog
	dialog, ok := gb.dialogs.Find(func(d *Dialog) bool {
		return d.pullCtx.StreamPath == req.StreamPath
	})
	if !ok {
		resp.Code = 404
		resp.Message = "未找到对应的回放会话"
		return resp, nil
	}

	// 构建RTSP PAUSE消息内容
	content := strings.Builder{}
	content.WriteString("PAUSE RTSP/1.0\r\n")
	content.WriteString(fmt.Sprintf("CSeq: %d\r\n", int(time.Now().UnixNano()/1e6%1000000)))
	content.WriteString("PauseTime: now\r\n")

	// 创建INFO请求
	request := sip.NewRequest(sip.INFO, dialog.session.InviteRequest.Recipient)
	request.SetBody([]byte(content.String()))
	contentType := sip.ContentTypeHeader("Application/MANSRTSP")
	request.AppendHeader(&contentType)

	// 发送请求
	_, err := dialog.session.TransactionRequest(ctx, request)
	if err != nil {
		resp.Code = 500
		resp.Message = fmt.Sprintf("发送暂停请求失败: %v", err)
		return resp, nil
	}
	if s, ok := gb.Server.Streams.SafeGet(req.StreamPath); ok {
		s.Pause()
	}
	gb.Info("暂停回放",
		"streampath", req.StreamPath)

	resp.Code = 0
	resp.Message = "success"
	return resp, nil
}

// PlaybackResume 实现回放恢复功能
func (gb *GB28181Plugin) PlaybackResume(ctx context.Context, req *pb.PlaybackResumeRequest) (*pb.BaseResponse, error) {
	resp := &pb.BaseResponse{}

	// 参数校验
	if req.StreamPath == "" {
		resp.Code = 400
		resp.Message = "流路径不能为空"
		return resp, nil
	}

	// 查找对应的dialog
	dialog, ok := gb.dialogs.Find(func(d *Dialog) bool {
		return d.pullCtx.StreamPath == req.StreamPath
	})
	if !ok {
		resp.Code = 404
		resp.Message = "未找到对应的回放会话"
		return resp, nil
	}

	// 构建RTSP PLAY消息内容
	content := strings.Builder{}
	content.WriteString("PLAY RTSP/1.0\r\n")
	content.WriteString(fmt.Sprintf("CSeq: %d\r\n", int(time.Now().UnixNano()/1e6%1000000)))
	content.WriteString("Range: npt=now-\r\n")

	// 创建INFO请求
	request := sip.NewRequest(sip.INFO, dialog.session.InviteRequest.Recipient)
	request.SetBody([]byte(content.String()))
	contentType := sip.ContentTypeHeader("Application/MANSRTSP")
	request.AppendHeader(&contentType)

	// 发送请求
	_, err := dialog.session.TransactionRequest(ctx, request)
	if err != nil {
		resp.Code = 500
		resp.Message = fmt.Sprintf("发送恢复请求失败: %v", err)
		return resp, nil
	}
	if s, ok := gb.Server.Streams.SafeGet(req.StreamPath); ok {
		s.Resume()
	}
	gb.Info("恢复回放",
		"streampath", req.StreamPath)

	resp.Code = 0
	resp.Message = "success"
	return resp, nil
}

// PlaybackSeek 实现回放拖动功能
func (gb *GB28181Plugin) PlaybackSeek(ctx context.Context, req *pb.PlaybackSeekRequest) (*pb.BaseResponse, error) {
	resp := &pb.BaseResponse{}

	// 参数校验
	if req.StreamPath == "" {
		resp.Code = 400
		resp.Message = "流路径不能为空"
		return resp, nil
	}

	// TODO: 实现拖动播放逻辑

	gb.Info("拖动回放",
		"streampath", req.StreamPath,
		"seekTime", req.SeekTime)

	resp.Code = 0
	resp.Message = "success"
	return resp, nil
}

// PlaybackSpeed 实现回放倍速功能
func (gb *GB28181Plugin) PlaybackSpeed(ctx context.Context, req *pb.PlaybackSpeedRequest) (*pb.BaseResponse, error) {
	resp := &pb.BaseResponse{}

	// 参数校验
	if req.StreamPath == "" {
		resp.Code = 400
		resp.Message = "流路径不能为空"
		return resp, nil
	}

	// 查找对应的dialog
	dialog, ok := gb.dialogs.Find(func(d *Dialog) bool {
		return d.pullCtx.StreamPath == req.StreamPath
	})
	if !ok {
		resp.Code = 404
		resp.Message = "未找到对应的回放会话"
		return resp, nil
	}

	// 构建RTSP SCALE消息内容
	content := strings.Builder{}
	content.WriteString("PLAY RTSP/1.0\r\n")
	content.WriteString(fmt.Sprintf("CSeq: %d\r\n", int(time.Now().UnixNano()/1e6%1000000)))
	content.WriteString(fmt.Sprintf("Scale: %f\r\n", req.Speed))
	content.WriteString("Range: npt=now-\r\n")

	// 创建INFO请求
	request := sip.NewRequest(sip.INFO, dialog.session.InviteRequest.Recipient)
	request.SetBody([]byte(content.String()))
	contentType := sip.ContentTypeHeader("Application/MANSRTSP")
	request.AppendHeader(&contentType)

	// 发送请求
	_, err := dialog.session.TransactionRequest(ctx, request)

	if s, ok := gb.Server.Streams.SafeGet(req.StreamPath); ok {
		s.Speed = float64(req.Speed)
		s.Scale = float64(req.Speed)
		s.Info("set stream speed", "speed", req.Speed)
	}
	if err != nil {
		resp.Code = 500
		resp.Message = fmt.Sprintf("发送倍速请求失败: %v", err)
		return resp, nil
	}

	gb.Info("倍速回放",
		"streampath", req.StreamPath,
		"speed", req.Speed)

	resp.Code = 0
	resp.Message = "success"
	return resp, nil
}

// DeleteGroup 实现删除分组功能
func (gb *GB28181Plugin) DeleteGroup(ctx context.Context, req *pb.DeleteGroupRequest) (*pb.BaseResponse, error) {
	resp := &pb.BaseResponse{}

	// 检查数据库连接
	if gb.DB == nil {
		resp.Code = 500
		resp.Message = "数据库未初始化"
		return resp, nil
	}

	// 验证参数
	if req.Id <= 0 {
		resp.Code = 400
		resp.Message = "分组ID不能为空"
		return resp, nil
	}

	// 查询分组是否存在
	var group gb28181.GroupsModel
	if err := gb.DB.First(&group, req.Id).Error; err != nil {
		resp.Code = 404
		resp.Message = fmt.Sprintf("分组不存在: %v", err)
		return resp, nil
	}

	// 检查是否为根组织，根组织不能删除
	if group.PID == 0 && group.Level == 0 {
		resp.Code = 400
		resp.Message = "根组织不能被删除"
		return resp, nil
	}

	// 查询所有子分组，用于递归删除
	var childGroups []gb28181.GroupsModel
	if err := gb.DB.Where("pid = ?", req.Id).Find(&childGroups).Error; err != nil {
		resp.Code = 500
		resp.Message = fmt.Sprintf("查询子分组失败: %v", err)
		return resp, nil
	}

	// 开始事务
	tx := gb.DB.Begin()

	// 定义递归删除函数，返回删除的分组数量和错误
	var deleteGroupAndChildren func(groupID int) (int, error)
	deleteGroupAndChildren = func(groupID int) (int, error) {
		// 查询所有子分组
		var children []gb28181.GroupsModel
		if err := tx.Where("pid = ?", groupID).Find(&children).Error; err != nil {
			return 0, fmt.Errorf("查询子分组失败: %v", err)
		}

		count := 1 // 当前分组

		// 递归删除每个子分组
		for _, child := range children {
			// 递归删除子分组及其子分组
			subCount, err := deleteGroupAndChildren(child.ID)
			if err != nil {
				return 0, err
			}
			count += subCount
		}

		// 删除当前分组的通道关联
		if err := tx.Where("group_id = ?", groupID).Delete(&gb28181.GroupsChannelModel{}).Error; err != nil {
			return 0, fmt.Errorf("删除分组的通道关联失败: %v", err)
		}

		// 删除当前分组
		if err := tx.Delete(&gb28181.GroupsModel{}, groupID).Error; err != nil {
			return 0, fmt.Errorf("删除分组失败: %v", err)
		}

		return count, nil
	}

	// 记录删除的分组数量
	deletedCount, err := deleteGroupAndChildren(int(req.Id))
	if err != nil {
		tx.Rollback()
		resp.Code = 500
		resp.Message = fmt.Sprintf("删除分组失败: %v", err)
		return resp, nil
	}

	// 提交事务
	if err := tx.Commit().Error; err != nil {
		resp.Code = 500
		resp.Message = fmt.Sprintf("提交事务失败: %v", err)
		return resp, nil
	}

	gb.Info("删除分组成功",
		"groupId", req.Id,
		"groupName", group.Name,
		"deletedCount", deletedCount)

	resp.Code = 0
	resp.Message = fmt.Sprintf("分组删除成功，共删除 %d 个分组", deletedCount)
	return resp, nil
}

// SearchAlarms 实现分页查询报警记录
func (gb *GB28181Plugin) SearchAlarms(ctx context.Context, req *pb.SearchAlarmsRequest) (*pb.SearchAlarmsResponse, error) {
	resp := &pb.SearchAlarmsResponse{
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
		"range": []string{req.Range},
		"start": []string{req.Start},
		"end":   []string{req.End},
	})
	if err != nil {
		resp.Code = 400
		resp.Message = fmt.Sprintf("时间格式错误: %v", err)
		return resp, nil
	}

	// 构建基础查询条件
	query := gb.DB.Model(&gb28181.DeviceAlarm{})

	// 如果指定了设备ID，添加设备ID过滤条件
	if req.DeviceId != "" {
		query = query.Where("device_id = ?", req.DeviceId)
	}

	// 添加时间范围条件
	if !startTime.IsZero() {
		query = query.Where("alarm_time > ?", startTime)
	}
	if !endTime.IsZero() {
		query = query.Where("alarm_time < ?", endTime)
	}

	// 获取符合条件的总记录数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		resp.Code = 500
		resp.Message = fmt.Sprintf("查询总数失败: %v", err)
		return resp, nil
	}

	// 查询报警记录，添加分页处理
	var alarms []gb28181.DeviceAlarm
	queryWithOrder := query.Order("alarm_time DESC")

	// 当Page和Count都大于0时，应用分页
	if req.Page > 0 && req.Count > 0 {
		offset := (req.Page - 1) * req.Count
		queryWithOrder = queryWithOrder.Offset(int(offset)).Limit(int(req.Count))
	}

	if err := queryWithOrder.Find(&alarms).Error; err != nil {
		resp.Code = 500
		resp.Message = fmt.Sprintf("查询报警记录失败: %v", err)
		return resp, nil
	}

	// 转换为proto消息
	for _, alarm := range alarms {
		alarmRecord := &pb.AlarmRecord{
			Id:                fmt.Sprintf("%d", alarm.ID),
			DeviceId:          alarm.DeviceID,
			DeviceName:        alarm.DeviceName,
			ChannelId:         alarm.ChannelID,
			AlarmPriority:     alarm.AlarmPriority,
			AlarmMethod:       alarm.AlarmMethod,
			AlarmTime:         timestamppb.New(alarm.AlarmTime),
			AlarmDescription:  alarm.AlarmDescription,
			Longitude:         alarm.Longitude,
			Latitude:          alarm.Latitude,
			AlarmType:         alarm.AlarmType,
			CreateTime:        timestamppb.New(alarm.CreateTime),
			AlarmPriorityDesc: alarm.GetAlarmPriorityDescription(),
			AlarmMethodDesc:   alarm.GetAlarmMethodDescription(),
			AlarmTypeDesc:     alarm.GetAlarmTypeDescription(),
		}
		resp.Data = append(resp.Data, alarmRecord)
	}

	// 添加总记录数到响应中
	resp.Total = int32(total)

	return resp, nil
}

// RemoveDevice 实现删除设备功能
func (gb *GB28181Plugin) RemoveDevice(ctx context.Context, req *pb.RemoveDeviceRequest) (*pb.BaseResponse, error) {
	resp := &pb.BaseResponse{}

	// 参数校验
	if req.Id == "" {
		resp.Code = 400
		resp.Message = "设备ID不能为空"
		return resp, nil
	}

	// 使用数据库中的 DeviceId 从内存中查找设备
	if device, ok := gb.devices.Get(req.Id); ok {
		//device.channels.Range(func(channel *Channel) bool {
		//	if err := device.plugin.DB.Where("device_id = ?", device.DeviceId).Delete(&gb28181.DeviceChannel{}).Error; err != nil {
		//		device.Error("删除设备通道记录失败", "error", err)
		//	}
		//	return true
		//})
		//if device.Online {
		// 停止设备相关任务
		device.DeletedAt = gorm.DeletedAt{Time: time.Now(), Valid: true}
		device.Stop(fmt.Errorf("device removed"))
		device.WaitStopped()
		//}

		resp.Code = 200
		resp.Message = "设备删除成功"
	}

	return resp, nil
}

// ReceiveAlarm 实现接收告警信息接口
func (gb *GB28181Plugin) ReceiveAlarm(ctx context.Context, req *pb.AlarmInfoRequest) (*pb.BaseResponse, error) {
	resp := &pb.BaseResponse{}

	// 如果CreateAt为空，则使用当前时间
	if req.CreateAt == nil {
		req.CreateAt = timestamppb.Now()
	}

	// 打印接收到的告警信息
	gb.Info("收到告警信息",
		"创建时间", req.CreateAt,
		"服务器信息", req.ServerInfo,
		"流名称", req.StreamName,
		"流路径", req.StreamPath,
		"告警描述", req.AlarmDesc,
		"告警类型", req.AlarmType,
	)

	// 转换为JSON格式并打印
	jsonData, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		gb.Error("告警信息转JSON失败", "error", err)
		resp.Code = 500
		resp.Message = "告警信息处理失败: " + err.Error()
		return resp, nil
	}

	gb.Info("告警信息JSON格式", "data", string(jsonData))

	// 返回成功响应
	resp.Code = 0
	resp.Message = "告警信息接收成功"
	return resp, nil
}

// UpdateChannel 实现更新通道信息
func (gb *GB28181Plugin) UpdateChannel(ctx context.Context, req *pb.UpdateChannelRequest) (*pb.BaseResponse, error) {
	resp := &pb.BaseResponse{}

	// 参数校验
	if req.Id == "" {
		resp.Code = 400
		resp.Message = "通道复合ID不能为空"
		return resp, nil
	}

	if req.Channel == nil {
		resp.Code = 400
		resp.Message = "通道信息不能为空"
		return resp, nil
	}

	// 打印接收到的 channel 用于调试前端传参问题
	gb.Debug("UpdateChannel request", "id", req.Id, "channel", req.Channel.String())

	// 直接使用 id 查找通道
	parts := strings.Split(req.Id, "_")
	if len(parts) != 2 {
		resp.Code = 400
		resp.Message = "通道复合ID格式错误，应为 deviceId_channelId"
		return resp, nil
	}

	// 查找通道
	channel, ok := gb.channels.Get(req.Id)
	if !ok {
		resp.Code = 404
		resp.Message = "通道不存在"
		return resp, nil
	}

	// 详细调试信息，帮助确认前端字段映射
	gb.Debug("UpdateChannel debug",
		"req.Id", req.Id,
		"req.Channel.CustomChannelId", req.Channel.CustomChannelId,
		"req.Channel.CustomChannelName", req.Channel.CustomChannelName,
		"req.Channel.StreamPath", req.Channel.StreamPath,
		"memory.Channel.DeviceNil", channel.Device == nil,
		"memory.Channel.DeviceChannel.StreamPath", channel.DeviceChannel.StreamPath,
	)

	// 从请求中获取自定义通道ID（使用 CustomChannelId 字段）
	customChannelId := req.Channel.CustomChannelId
	if customChannelId != "" && customChannelId != channel.DeviceChannel.CustomChannelId {
		// 检查自定义通道ID是否已存在（全局唯一性检查）
		hasConflict := false

		// 遍历所有通道检查是否有重复的 customChannelId
		gb.channels.Range(func(ch *Channel) bool {
			// 跳过当前正在更新的通道
			if ch.DeviceChannel.ID == req.Id {
				return true
			}

			// 检查其他通道是否已使用该自定义ID
			if ch.DeviceChannel.CustomChannelId == customChannelId {
				hasConflict = true
				return false
			}
			return true
		})

		// 如果有冲突，返回错误
		if hasConflict {
			resp.Code = 409
			resp.Message = "自定义通道ID已存在，请使用其他ID"
			return resp, nil
		}

		// 更新自定义通道ID（内存）
		channel.DeviceChannel.CustomChannelId = customChannelId
	}

	// 从请求中获取自定义名称（使用 CustomChannelName 字段）
	if req.Channel.CustomChannelName != "" {
		channel.DeviceChannel.CustomName = req.Channel.CustomChannelName
	} else {
		channel.DeviceChannel.CustomName = channel.DeviceChannel.Name
	}

	// 处理 streamPath 更新：仅允许自定义通道（channel.Device == nil）更新 streamPath，且不能为空
	if channel.Device == nil {
		// 自定义通道，streamPath 必须存在且不可为空
		if req.Channel.StreamPath == "" {
			resp.Code = 400
			resp.Message = "自定义通道的 streamPath 不能为空"
			return resp, nil
		}
		if req.Channel.StreamPath != "" && req.Channel.StreamPath != channel.DeviceChannel.StreamPath {
			channel.DeviceChannel.StreamPath = req.Channel.StreamPath
		}
	} else {
		// 设备通道不允许填写 streamPath（防止误改）
		if req.Channel.StreamPath != "" {
			resp.Code = 400
			resp.Message = "设备通道不允许修改 streamPath"
			return resp, nil
		}
	}

	// 持久化内存中的完整 DeviceChannel 到数据库（一次保存，避免部分覆盖问题）
	if gb.DB != nil {
		if err := gb.DB.Save(channel.DeviceChannel).Error; err != nil {
			// 若为唯一索引冲突，可返回 409；否则返回 500
			if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "重复") {
				resp.Code = 409
				resp.Message = "自定义通道ID已存在，请使用其他ID"
				return resp, nil
			}
			resp.Code = 500
			resp.Message = fmt.Sprintf("保存通道失败: %v", err)
			return resp, nil
		}
	}

	// 记录日志
	gb.Debug("通道信息已更新",
		"通道ID", req.Id,
		"自定义通道ID", channel.DeviceChannel.CustomChannelId,
		"自定义名称", channel.DeviceChannel.CustomName,
		"streamPath", channel.DeviceChannel.StreamPath)

	// 返回成功响应
	resp.Code = 0
	resp.Message = "通道信息更新成功"
	return resp, nil
}

// AddChannelWithProxy 添加通道并关联拉流代理
func (gb *GB28181Plugin) AddChannelWithProxy(ctx context.Context, req *pb.AddChannelWithProxyRequest) (*pb.BaseResponse, error) {
	resp := &pb.BaseResponse{}

	// 1. 参数验证
	if req.ChannelId == "" {
		resp.Code = 400
		resp.Message = "channelId不能为空"
		return resp, nil
	}
	if req.Name == "" {
		resp.Code = 400
		resp.Message = "name不能为空"
		return resp, nil
	}
	if req.StreamPath == "" {
		resp.Code = 400
		resp.Message = "streamPath不能为空"
		return resp, nil
	}

	// 2. 检查数据库连接
	if gb.DB == nil {
		resp.Code = 500
		resp.Message = "数据库未初始化"
		return resp, nil
	}

	// 3. 重复检查 - 检查customChannelId是否已存在
	var existingChannel gb28181.DeviceChannel
	if err := gb.DB.Where("custom_channel_id = ?", req.ChannelId).First(&existingChannel).Error; err == nil {
		resp.Code = 409
		resp.Message = "通道ID已存在，请使用其他ID"
		return resp, nil
	} else if err != gorm.ErrRecordNotFound {
		resp.Code = 500
		resp.Message = fmt.Sprintf("检查通道ID失败: %v", err)
		return resp, nil
	}

	// 4. 生成ID和相关字段
	channelID := req.ChannelId + "_" + req.ChannelId
	deviceID := req.ChannelId

	// 5. 创建DeviceChannel实例
	now := time.Now().Format("2006-01-02 15:04:05")
	deviceChannel := &gb28181.DeviceChannel{
		ID:                 channelID,
		DeviceId:           deviceID,
		ChannelId:          req.ChannelId,
		CustomChannelId:    req.ChannelId,
		Name:               req.Name,
		CustomName:         req.Name,
		Manufacturer:       req.Manufacturer,
		Model:              req.Model,
		Owner:              req.Owner,
		CivilCode:          req.CivilCode,
		Block:              req.Block,
		Address:            req.Address,
		Port:               int(req.Port),
		Parental:           int(req.Parental),
		ParentId:           req.ParentId,
		SafetyWay:          int(req.SafetyWay),
		RegisterWay:        int(req.RegisterWay),
		CertNum:            req.CertNum,
		Certifiable:        int(req.Certifiable),
		ErrCode:            int(req.ErrCode),
		EndTime:            req.EndTime,
		Secrecy:            int(req.Secrecy),
		IPAddress:          req.IpAddress,
		Password:           req.Password,
		PTZType:            int(req.PtzType),
		PositionType:       int(req.PositionType),
		RoomType:           int(req.RoomType),
		UseType:            int(req.UseType),
		SupplyLightType:    int(req.SupplyLightType),
		DirectionType:      int(req.DirectionType),
		Resolution:         req.Resolution,
		BusinessGroupID:    req.BusinessGroupId,
		DownloadSpeed:      req.DownloadSpeed,
		SVCSpaceSupportMod: int(req.SvcSpaceSupportMod),
		SVCTimeSupportMode: int(req.SvcTimeSupportMode),
		Status:             gb28181.ChannelStatus(req.Status),
		CreateTime:         now,
		StreamPath:         req.StreamPath, // 关联拉流代理的流路径
	}

	// 6. 处理经纬度 - 字符串转float64
	if req.Longitude != "" {
		// 使用fmt.Sscanf解析字符串为float64
		var lon float64
		if _, err := fmt.Sscanf(req.Longitude, "%f", &lon); err == nil {
			deviceChannel.Longitude = lon
			deviceChannel.GbLongitude = lon
		}
	}
	if req.Latitude != "" {
		var lat float64
		if _, err := fmt.Sscanf(req.Latitude, "%f", &lat); err == nil {
			deviceChannel.Latitude = lat
			deviceChannel.GbLatitude = lat
		}
	}

	// 7. 设置默认状态
	if deviceChannel.Status == "" {
		deviceChannel.Status = gb28181.ChannelOffStatus
	}

	// 8. 保存到数据库
	if err := gb.DB.Create(deviceChannel).Error; err != nil {
		resp.Code = 500
		resp.Message = fmt.Sprintf("保存通道失败: %v", err)
		return resp, nil
	}

	// 9. 添加到内存集合
	channel := &Channel{
		DeviceChannel: deviceChannel,
		Device:        nil, // 这是虚拟设备通道，不关联真实GB设备
		Logger:        gb.Logger.With("channel", channelID),
	}
	gb.channels.Set(channel)

	// 10. 记录日志
	gb.Info("添加通道成功",
		"channelId", req.ChannelId,
		"id", channelID,
		"deviceId", deviceID,
		"streamPath", req.StreamPath,
		"name", req.Name)

	resp.Code = 0
	resp.Message = "通道添加成功"
	return resp, nil
}

// UpdateChannelWithProxy 更新通道信息
func (gb *GB28181Plugin) UpdateChannelWithProxy(ctx context.Context, req *pb.UpdateChannelWithProxyRequest) (*pb.BaseResponse, error) {
	resp := &pb.BaseResponse{}

	// 1. 参数验证
	if req.ChannelId == "" {
		resp.Code = 400
		resp.Message = "channelId不能为空"
		return resp, nil
	}

	// 2. 检查数据库连接
	if gb.DB == nil {
		resp.Code = 500
		resp.Message = "数据库未初始化"
		return resp, nil
	}

	// 3. 生成ID并查找通道
	channelID := req.ChannelId + "_" + req.ChannelId
	var existingChannel gb28181.DeviceChannel
	if err := gb.DB.Where("id = ?", channelID).First(&existingChannel).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			resp.Code = 404
			resp.Message = "通道不存在"
			return resp, nil
		}
		resp.Code = 500
		resp.Message = fmt.Sprintf("查询通道失败: %v", err)
		return resp, nil
	}

	// 4. 构建更新字段（只更新非空字段）
	updates := make(map[string]interface{})

	if req.StreamPath != "" {
		updates["stream_path"] = req.StreamPath
	}
	if req.Name != "" {
		updates["name"] = req.Name
		updates["custom_name"] = req.Name
	}
	if req.Manufacturer != "" {
		updates["manufacturer"] = req.Manufacturer
	}
	if req.Model != "" {
		updates["model"] = req.Model
	}
	if req.Owner != "" {
		updates["owner"] = req.Owner
	}
	if req.CivilCode != "" {
		updates["civil_code"] = req.CivilCode
	}
	if req.Block != "" {
		updates["block"] = req.Block
	}
	if req.Address != "" {
		updates["address"] = req.Address
	}
	if req.Port > 0 {
		updates["port"] = req.Port
	}
	if req.Parental >= 0 {
		updates["parental"] = req.Parental
	}
	if req.ParentId != "" {
		updates["parent_id"] = req.ParentId
	}
	if req.SafetyWay >= 0 {
		updates["safety_way"] = req.SafetyWay
	}
	if req.RegisterWay >= 0 {
		updates["register_way"] = req.RegisterWay
	}
	if req.CertNum != "" {
		updates["cert_num"] = req.CertNum
	}
	if req.Certifiable >= 0 {
		updates["certifiable"] = req.Certifiable
	}
	if req.ErrCode >= 0 {
		updates["err_code"] = req.ErrCode
	}
	if req.EndTime != "" {
		updates["end_time"] = req.EndTime
	}
	if req.Secrecy >= 0 {
		updates["secrecy"] = req.Secrecy
	}
	if req.IpAddress != "" {
		updates["ip_address"] = req.IpAddress
	}
	if req.Password != "" {
		updates["password"] = req.Password
	}
	if req.PtzType >= 0 {
		updates["ptz_type"] = req.PtzType
	}
	if req.PositionType >= 0 {
		updates["position_type"] = req.PositionType
	}
	if req.RoomType >= 0 {
		updates["room_type"] = req.RoomType
	}
	if req.UseType >= 0 {
		updates["use_type"] = req.UseType
	}
	if req.SupplyLightType >= 0 {
		updates["supply_light_type"] = req.SupplyLightType
	}
	if req.DirectionType >= 0 {
		updates["direction_type"] = req.DirectionType
	}
	if req.Resolution != "" {
		updates["resolution"] = req.Resolution
	}
	if req.BusinessGroupId != "" {
		updates["business_group_id"] = req.BusinessGroupId
	}
	if req.DownloadSpeed != "" {
		updates["download_speed"] = req.DownloadSpeed
	}
	if req.SvcSpaceSupportMod >= 0 {
		updates["svc_space_support_mod"] = req.SvcSpaceSupportMod
	}
	if req.SvcTimeSupportMode >= 0 {
		updates["svc_time_support_mode"] = req.SvcTimeSupportMode
	}
	if req.Status != "" {
		updates["status"] = req.Status
	}
	if req.Longitude != "" {
		var lon float64
		if _, err := fmt.Sscanf(req.Longitude, "%f", &lon); err == nil {
			updates["longitude"] = lon
			updates["gb_longitude"] = lon
		}
	}
	if req.Latitude != "" {
		var lat float64
		if _, err := fmt.Sscanf(req.Latitude, "%f", &lat); err == nil {
			updates["latitude"] = lat
			updates["gb_latitude"] = lat
		}
	}

	// 5. 如果没有要更新的字段
	if len(updates) == 0 {
		resp.Code = 400
		resp.Message = "没有要更新的字段"
		return resp, nil
	}

	// 6. 更新数据库
	if err := gb.DB.Model(&gb28181.DeviceChannel{}).Where("id = ?", channelID).Updates(updates).Error; err != nil {
		resp.Code = 500
		resp.Message = fmt.Sprintf("更新通道失败: %v", err)
		return resp, nil
	}

	// 7. 更新内存中的通道（如果存在）
	if channel, ok := gb.channels.Get(channelID); ok {
		// 重新从数据库加载最新数据
		if err := gb.DB.Where("id = ?", channelID).First(channel.DeviceChannel).Error; err == nil {
			gb.Info("内存中的通道已更新", "channelId", req.ChannelId)
		}
	}

	// 8. 记录日志
	gb.Info("更新通道成功",
		"channelId", req.ChannelId,
		"id", channelID,
		"updatedFields", len(updates))

	resp.Code = 0
	resp.Message = "通道更新成功"
	return resp, nil
}

// DeleteChannelWithProxy 删除通道
func (gb *GB28181Plugin) DeleteChannelWithProxy(ctx context.Context, req *pb.DeleteChannelWithProxyRequest) (*pb.BaseResponse, error) {
	resp := &pb.BaseResponse{}

	// 1. 参数验证
	if req.ChannelId == "" {
		resp.Code = 400
		resp.Message = "channelId不能为空"
		return resp, nil
	}

	// 2. 检查数据库连接
	if gb.DB == nil {
		resp.Code = 500
		resp.Message = "数据库未初始化"
		return resp, nil
	}

	// 3. 生成ID
	channelID := req.ChannelId + "_" + req.ChannelId

	// 4. 检查通道是否存在
	var existingChannel gb28181.DeviceChannel
	if err := gb.DB.Where("id = ?", channelID).First(&existingChannel).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			resp.Code = 404
			resp.Message = "通道不存在"
			return resp, nil
		}
		resp.Code = 500
		resp.Message = fmt.Sprintf("查询通道失败: %v", err)
		return resp, nil
	}

	// 5. 从数据库删除
	if err := gb.DB.Where("id = ?", channelID).Delete(&gb28181.DeviceChannel{}).Error; err != nil {
		resp.Code = 500
		resp.Message = fmt.Sprintf("删除通道失败: %v", err)
		return resp, nil
	}

	// 6. 从内存中移除
	if channel, ok := gb.channels.Get(channelID); ok {
		gb.channels.RemoveByKey(channel.ID)
		gb.Info("从内存中移除通道", "channelId", req.ChannelId)
	}

	// 7. 记录日志
	gb.Info("删除通道成功",
		"channelId", req.ChannelId,
		"id", channelID,
		"streamPath", existingChannel.StreamPath)

	resp.Code = 0
	resp.Message = "通道删除成功"
	return resp, nil
}

// StartDownload 实现发起录像下载接口
func (gb *GB28181Plugin) StartDownload(ctx context.Context, req *pb.StartDownloadRequest) (*pb.StartDownloadResponse, error) {
	resp := &pb.StartDownloadResponse{}

	// 1. 参数验证
	if req.DeviceId == "" || req.ChannelId == "" {
		resp.Code = 400
		resp.Message = "deviceId 和 channelId 不能为空"
		return resp, nil
	}

	if req.Start == "" || req.End == "" {
		resp.Code = 400
		resp.Message = "start 和 end 时间不能为空"
		return resp, nil
	}

	// 2. 解析时间范围
	startTime, endTime, err := util.TimeRangeQueryParse(url.Values{
		"start": []string{req.Start},
		"end":   []string{req.End},
	})
	if err != nil {
		resp.Code = 400
		resp.Message = fmt.Sprintf("时间解析失败: %v", err)
		return resp, nil
	}

	// 3. 验证设备和通道是否存在
	device, ok := gb.devices.Get(req.DeviceId)
	if !ok {
		resp.Code = 404
		resp.Message = "设备不存在"
		return resp, nil
	}

	channelKey := req.DeviceId + "_" + req.ChannelId
	_, ok = device.channels.Get(channelKey)
	if !ok {
		resp.Code = 404
		resp.Message = "通道不存在"
		return resp, nil
	}

	// 4. 生成下载任务ID（复合键）
	downloadId := fmt.Sprintf("%s_%s_%d_%d", req.DeviceId, req.ChannelId, startTime.Unix(), endTime.Unix())

	// 5. 优先从缓存表查询已完成的下载
	if gb.DB != nil {
		var cachedRecord gb28181.GB28181Record
		if err := gb.DB.Where("download_id = ? AND status = ?", downloadId, "completed").First(&cachedRecord).Error; err == nil {
			// 检查文件是否存在
			if _, err := os.Stat(cachedRecord.FilePath); err == nil {
				// 生成下载 URL
				downloadUrl := fmt.Sprintf("/gb28181/download?downloadId=%s", downloadId)

				gb.Info("从缓存返回已下载的录像",
					"downloadId", downloadId,
					"filePath", cachedRecord.FilePath,
					"downloadUrl", downloadUrl)
				resp.Code = 0
				resp.Message = "录像已存在（来自缓存）"
				resp.Total = 0
				resp.Data = &pb.StartDownloadData{
					DownloadId:  downloadId,
					Status:      "completed",
					DownloadUrl: downloadUrl,
				}
				return resp, nil
			} else {
				// 文件不存在，删除缓存记录和RecordStream记录
				gb.DB.Delete(&cachedRecord)
				// 同时删除MP4插件的RecordStream记录（通过FilePath）
				gb.DB.Where("file_path = ?", cachedRecord.FilePath).Delete(&m7s.RecordStream{})
				gb.Warn("缓存记录的文件不存在，已删除缓存和RecordStream记录",
					"downloadId", downloadId,
					"filePath", cachedRecord.FilePath)
			}
		}
	}

	// 6. 检查正在进行的下载任务
	if existingDialog, exists := gb.downloadDialogs.Get(downloadId); exists {
		resp.Code = 200
		resp.Message = "下载任务正在进行中"
		resp.Total = 0
		resp.Data = &pb.StartDownloadData{
			DownloadId:  downloadId,
			Status:      existingDialog.Status,
			DownloadUrl: existingDialog.DownloadUrl,
		}
		return resp, nil
	}

	// 7. 检查已完成的下载任务（内存缓存）
	if completedDialog, exists := gb.completedDownloads.Get(downloadId); exists {
		resp.Code = 0
		resp.Message = "下载任务已完成"
		resp.Total = 0
		resp.Data = &pb.StartDownloadData{
			DownloadId:  downloadId,
			Status:      completedDialog.Status,
			DownloadUrl: completedDialog.DownloadUrl,
		}
		return resp, nil
	}

	// 8. 下载链接将在录制开始后动态生成
	// 初始为空，等进度更新时从数据库查询后填充
	downloadUrl := ""

	// 9. 创建下载对话
	downloadSpeed := int(req.DownloadSpeed)
	if downloadSpeed <= 0 || downloadSpeed > 4 {
		downloadSpeed = 4 // 默认4倍速，避免丢帧
	}

	dialog := &DownloadDialog{
		gb:            gb,
		DownloadId:    downloadId,
		DeviceId:      req.DeviceId,
		ChannelId:     req.ChannelId,
		StartTime:     startTime,
		EndTime:       endTime,
		DownloadSpeed: downloadSpeed,
		DownloadUrl:   downloadUrl,
		Status:        "pending",
		Progress:      0,
	}
	dialog.Logger = gb.Logger.With("streamPath", downloadId, "channelId", req.DeviceId+"_"+req.ChannelId)
	dialog.Task.Context = ctx

	// 10. 添加到下载对话集合（会自动调用 Start 方法）
	gb.downloadDialogs.AddTask(dialog)

	resp.Code = 0
	resp.Message = "下载任务已创建"
	resp.Total = 0
	resp.Data = &pb.StartDownloadData{
		DownloadId:  downloadId,
		Status:      "pending",
		DownloadUrl: downloadUrl,
	}
	return resp, nil
}

// GetDownloadProgress 实现查询下载进度接口
func (gb *GB28181Plugin) GetDownloadProgress(ctx context.Context, req *pb.GetDownloadProgressRequest) (*pb.DownloadProgressResponse, error) {
	resp := &pb.DownloadProgressResponse{}

	// 1. 参数验证
	if req.DownloadId == "" {
		resp.Code = 400
		resp.Message = "downloadId 不能为空"
		return resp, nil
	}

	// 2. 查询任务
	dialog, exists := gb.downloadDialogs.Get(req.DownloadId)
	if !exists {
		completedDialog, exists := gb.completedDownloads.Get(req.DownloadId)
		if exists {
			resp.Code = 0
			resp.Message = "success"
			resp.Total = 0
			resp.Data = &pb.DownloadProgressData{
				DownloadId:  completedDialog.DownloadId,
				Status:      completedDialog.Status,
				Progress:    int32(completedDialog.Progress),
				FilePath:    completedDialog.FilePath,
				DownloadUrl: completedDialog.DownloadUrl,
				Error:       completedDialog.Error,
				StartedAt:   timestamppb.New(completedDialog.StartedAt),
			}
			if !completedDialog.CompletedAt.IsZero() {
				resp.Data.CompletedAt = timestamppb.New(completedDialog.CompletedAt)
			}
			return resp, nil
		} else {
			resp.Code = 404
			resp.Message = "下载任务不存在"
			return resp, nil
		}
	}

	// 3. 构建响应
	resp.Code = 0
	resp.Message = "success"
	resp.Total = 0
	resp.Data = &pb.DownloadProgressData{
		DownloadId:  dialog.DownloadId,
		Status:      dialog.Status,
		Progress:    int32(dialog.Progress),
		FilePath:    dialog.FilePath,
		DownloadUrl: dialog.DownloadUrl,
		Error:       dialog.ErrorString,
		StartedAt:   timestamppb.New(dialog.StartedAt),
	}
	if !dialog.CompletedAt.IsZero() {
		resp.Data.CompletedAt = timestamppb.New(dialog.CompletedAt)
	}

	return resp, nil
}

// StartBroadcast 启动语音广播
func (gb *GB28181Plugin) StartBroadcast(ctx context.Context, req *pb.BroadcastRequest) (*pb.BaseResponse, error) {
	resp := &pb.BaseResponse{}

	// 1. 验证参数
	if req.DeviceId == "" || req.ChannelId == "" {
		resp.Code = 400
		resp.Message = "deviceId 和 channelId 不能为空"
		return resp, nil
	}

	// 2. 获取设备
	device, ok := gb.devices.Get(req.DeviceId)
	if !ok {
		resp.Code = 404
		resp.Message = "设备不存在"
		return resp, nil
	}

	// 3. 检查设备是否在线
	if !device.Online {
		resp.Code = 400
		resp.Message = "设备离线"
		return resp, nil
	}

	// 4. 检查会话是否已存在
	if _, exists := BroadcastSessions.Get(req.ChannelId); exists {
		resp.Code = 409
		resp.Message = "广播会话已存在"
		return resp, nil
	}

	// 5. 启动广播会话（会发送 SIP MESSAGE）
	broadcastSession, err := device.StartBroadcast(req.ChannelId)
	if err != nil {
		resp.Code = 500
		resp.Message = fmt.Sprintf("启动广播失败: %v", err)
		return resp, nil
	}

	// 6. 等待设备 INVITE（30秒超时）
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := broadcastSession.WaitInvite(ctx); err != nil {
		// 超时或失败，清理会话
		broadcastSession.StopBroadcast()
		if errors.Is(err, context.DeadlineExceeded) {
			resp.Code = 504
			resp.Message = "等待设备响应超时"
		} else {
			resp.Code = 500
			resp.Message = fmt.Sprintf("等待设备响应失败: %v", err)
		}
		return resp, nil
	}

	resp.Code = 0
	resp.Message = "广播启动成功"
	return resp, nil
}

// StopBroadcast 停止语音广播
func (gb *GB28181Plugin) StopBroadcast(ctx context.Context, req *pb.BroadcastRequest) (*pb.BaseResponse, error) {
	resp := &pb.BaseResponse{}

	// 1. 验证参数
	if req.DeviceId == "" || req.ChannelId == "" {
		resp.Code = 400
		resp.Message = "deviceId 和 channelId 不能为空"
		return resp, nil
	}

	// 2. 查找广播会话
	broadcastSession, exists := BroadcastSessions.Get(req.ChannelId)
	if !exists {
		resp.Code = 404
		resp.Message = "广播会话不存在"
		return resp, nil
	}

	// 3. 停止广播
	if err := broadcastSession.StopBroadcast(); err != nil {
		resp.Code = 500
		resp.Message = fmt.Sprintf("停止广播失败: %v", err)
		return resp, nil
	}

	resp.Code = 0
	resp.Message = "广播停止成功"
	return resp, nil
}

// API_talk_start WebSocket 接口，用于实时音频传输
// 路径: /gb28181/api/talk/start
func (gb *GB28181Plugin) API_talk_start(w http.ResponseWriter, r *http.Request) {
	// Upgrade HTTP connection to WebSocket
	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		gb.Error("WebSocket upgrade failed", "error", err)
		return
	}

	// Create a new TalkWebsocketTask for this connection
	talkTask := NewTalkWebsocketTask(gb, conn)

	if err := gb.AddTask(talkTask).WaitStarted(); err != nil {
		gb.Error("Failed to start talk websocket task", "error", err)
		_ = conn.Close()
		return
	}

	gb.Info("WebSocket talk session started", "taskId", talkTask.ID)
}
