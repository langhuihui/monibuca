package plugin_gb28181pro

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"gorm.io/gorm"
	"m7s.live/v5/pkg/util"

	"github.com/rs/zerolog"
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
			if !strings.Contains(device.DeviceId, req.Query) && !strings.Contains(device.Name, req.Query) {
				return true // 继续遍历
			}
		}

		// 如果需要筛选在线设备
		if req.Status && !device.Online {
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
				Id:           channel.ID,
				DeviceId:     channel.CustomChannelId,
				ChannelId:    channel.CustomChannelId,
				ParentId:     d.DeviceId,
				Name:         channel.CustomName,
				Manufacturer: channel.Manufacturer,
				Model:        channel.Model,
				Owner:        channel.Owner,
				CivilCode:    channel.CivilCode,
				Address:      channel.Address,
				Port:         int32(channel.Port),
				Parental:     int32(channel.Parental),
				SafetyWay:    int32(channel.SafetyWay),
				RegisterWay:  int32(channel.RegisterWay),
				Secrecy:      int32(channel.Secrecy),
				Status:       string(channel.Status),
				Longitude:    fmt.Sprintf("%f", channel.GbLongitude),
				Latitude:     fmt.Sprintf("%f", channel.GbLatitude),
				GpsTime:      timestamppb.New(time.Now()),
			})
			return true
		})

		pbDevices = append(pbDevices, &pb.Device{
			DeviceId:      d.DeviceId,
			Name:          d.CustomName,
			Manufacturer:  d.Manufacturer,
			Model:         d.Model,
			Status:        string(d.Status),
			Online:        d.Online,
			Longitude:     d.Longitude,
			Latitude:      d.Latitude,
			RegisterTime:  timestamppb.New(d.RegisterTime),
			UpdateTime:    timestamppb.New(d.UpdateTime),
			KeepAliveTime: timestamppb.New(d.KeepaliveTime),
			ChannelCount:  int32(d.ChannelCount),
			Channels:      pbChannels,
			MediaIp:       d.MediaIp,
			SipIp:         d.SipIp,
			Password:      d.Password,
			StreamMode:    string(d.StreamMode),
		})
	}

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
				DeviceId:     c.ChannelId,
				ChannelId:    c.ChannelId,
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
		resp.Data = &pb.Device{
			DeviceId:     d.DeviceId,
			Name:         d.Name,
			Manufacturer: d.Manufacturer,
			Model:        d.Model,
			Status:       string(d.Status),
			Online:       d.Online,
			Longitude:    d.Longitude,
			Latitude:     d.Latitude,
			RegisterTime: timestamppb.New(d.RegisterTime),
			UpdateTime:   timestamppb.New(d.UpdateTime),
			Channels:     channels,
			MediaIp:      d.MediaIp,
			SipIp:        d.SipIp,
			Password:     d.Password,
			StreamMode:   string(d.StreamMode),
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
		if req.Status && !d.Online {
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
		resp.List = channels
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

	// 先从内存中获取设备
	d, ok := gb.devices.Get(req.DeviceId)
	if !ok && gb.DB != nil {
		// 如果内存中没有且数据库存在，则从数据库查询
		var device Device
		if err := gb.DB.Where("device_id = ?", req.DeviceId).First(&device).Error; err == nil {
			d = &device
			// 恢复设备的必要字段
			d.Logger = gb.Logger.With("deviceid", req.DeviceId)
			d.channels.L = new(sync.RWMutex)
			d.plugin = gb

			// 初始化 Task
			var hash uint32
			for i := 0; i < len(d.DeviceId); i++ {
				ch := d.DeviceId[i]
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
					Host: d.SipIp,
					Port: d.Port,
				},
			}

			d.Recipient = sip.Uri{
				Host: d.IP,
				Port: d.Port,
				User: d.DeviceId,
			}

			// 初始化 SIP 客户端
			d.client, _ = sipgo.NewClient(gb.ua, sipgo.WithClientLogger(zerolog.New(os.Stdout)), sipgo.WithClientHostname(d.SipIp))

			// 将设备添加到内存中
			gb.devices.AddTask(d)
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
						d.CatalogSubscribeTask.Tick(nil)
					} else {
						catalogSubTask := NewCatalogSubscribeTask(d)
						d.AddTask(catalogSubTask)
						d.CatalogSubscribeTask.Tick(nil)
					}
				} else {
					if d.CatalogSubscribeTask != nil {
						d.CatalogSubscribeTask.Stop(fmt.Errorf("catalog subscription disabled"))
					}
				}
				if d.SubscribePosition > 0 {
					if d.PositionSubscribeTask != nil {
						d.PositionSubscribeTask.Ticker.Reset(time.Second * time.Duration(d.SubscribePosition))
						d.PositionSubscribeTask.Tick(nil)
					} else {
						positionSubTask := NewPositionSubscribeTask(d)
						d.AddTask(positionSubTask)
						d.PositionSubscribeTask.Tick(nil)
					}
				} else {
					if d.PositionSubscribeTask != nil {
						d.PositionSubscribeTask.Stop(fmt.Errorf("position subscription disabled"))
					}
				}
				if d.SubscribeAlarm > 0 {
					if d.AlarmSubscribeTask != nil {
						d.AlarmSubscribeTask.Ticker.Reset(time.Second * time.Duration(d.SubscribeAlarm))
						d.AlarmSubscribeTask.Tick(nil)
					} else {
						alarmSubTask := NewAlarmSubscribeTask(d)
						d.AddTask(alarmSubTask)
						d.AlarmSubscribeTask.Tick(nil)
					}
				} else {
					if d.AlarmSubscribeTask != nil {
						d.AlarmSubscribeTask.Stop(fmt.Errorf("alarm subscription disabled"))
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
	var platform gb28181.PlatformModel
	if err := gb.DB.First(&platform, req.Id).Error; err != nil {
		resp.Code = 404
		resp.Message = "platform not found"
		return resp, nil
	}

	// 从请求中创建一个新的平台模型
	updatedPlatform := gb28181.PlatformModel{
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
	}

	// 使用 GORM 的 Updates 方法更新非零值字段
	if err := gb.DB.Model(&platform).Updates(updatedPlatform).Error; err != nil {
		resp.Code = 500
		resp.Message = fmt.Sprintf("failed to update platform: %v", err)
		return resp, nil
	}
	gb.DB.Model(&platform).Find(&platform)
	// 处理平台启用状态变化
	if platform.Enable {
		// 如果存在旧的platform实例，先停止并移除
		if oldPlatform, ok := gb.platforms.Get(platform.ServerGBID); ok {
			oldPlatform.Unregister()
			oldPlatform.Stop(fmt.Errorf("platform updated"))
			oldPlatform.WaitStopped()
		}
		// 创建新的Platform实例
		platformInstance := NewPlatform(&platform, gb, false)
		// 添加到任务系统
		gb.platforms.AddTask(platformInstance)
	} else {
		// 如果平台被禁用，停止并移除旧的platform实例
		if oldPlatform, ok := gb.platforms.Get(platform.ServerGBID); ok {
			oldPlatform.Unregister()
			oldPlatform.Stop(fmt.Errorf("platform disabled"))
		}
	}

	resp.Code = 0
	resp.Message = "success"
	return resp, nil
}

// DeletePlatform 实现删除平台信息
func (gb *GB28181Plugin) DeletePlatform(ctx context.Context, req *pb.DeletePlatformRequest) (*pb.BaseResponse, error) {
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
func (gb *GB28181Plugin) ListPlatforms(ctx context.Context, req *pb.ListPlatformsRequest) (*pb.PlatformsPageInfo, error) {
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

	// 查询平台列表
	// 当Page和Count都为0时，不做分页，返回所有数据
	if req.Page == 0 && req.Count == 0 {
		// 不分页，查询所有数据
		if err := query.Find(&platforms).Error; err != nil {
			resp.Code = 500
			resp.Message = fmt.Sprintf("failed to list platforms: %v", err)
			return resp, nil
		}
	} else {
		// 分页查询
		if err := query.Offset(int(req.Page-1) * int(req.Count)).
			Limit(int(req.Count)).
			Find(&platforms).Error; err != nil {
			resp.Code = 500
			resp.Message = fmt.Sprintf("failed to list platforms: %v", err)
			return resp, nil
		}
	}

	// 转换为proto消息
	var pbPlatforms []*pb.Platform
	for _, p := range platforms {
		pbPlatforms = append(pbPlatforms, &pb.Platform{
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
			SendStreamIp:            p.SendStreamIp,
			AutoPushChannel:         p.AutoPushChannel,
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
func (gb *GB28181Plugin) QueryRecord(ctx context.Context, req *pb.QueryRecordRequest) (*pb.QueryRecordResponse, error) {
	resp := &pb.QueryRecordResponse{
		Code:    0,
		Message: "",
	}
	startTime, endTime, err := util.TimeRangeQueryParse(url.Values{"range": []string{req.Range}, "start": []string{req.Start}, "end": []string{req.End}})
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
		resp.Count = int32(recordReq.ReceivedNum)
		if !firstResponse.LastTime.IsZero() {
			resp.LastTime = timestamppb.New(firstResponse.LastTime)
		}
	}

	for _, record := range recordReq.Response {
		for _, item := range record.RecordList.Item {
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

	resp.Code = 0
	resp.Message = fmt.Sprintf("success, received %d/%d records", recordReq.ReceivedNum, recordReq.SumNum)

	// 排序录像列表，按StartTime升序排序
	sort.Slice(resp.Data, func(i, j int) bool {
		return resp.Data[i].StartTime < resp.Data[j].StartTime
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
	// 初始化SIP客户端
	device.client, _ = sipgo.NewClient(gb.ua, sipgo.WithClientLogger(zerolog.New(os.Stdout)), sipgo.WithClientHostname("192.168.1.106"))
	if device.client == nil {
		resp.Code = 500
		resp.Message = "failed to create sip client"
		return resp, nil
	}

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
	//Via: SIP/2.0/UDP 192.168.1.106:5060;branch=z9hG4bK9279674404;rport
	viaHeader := sip.ViaHeader{
		ProtocolName:    "SIP",
		ProtocolVersion: "2.0",
		Transport:       "UDP",
		Host:            "192.168.1.106",
		Port:            5060,
		Params:          sip.HeaderParams(sip.NewParams()),
	}
	viaHeader.Params.Add("branch", "z9hG4bK9279674404").Add("rport", "")

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
	request.AppendHeader(&viaHeader)

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
	}

	resp.Code = 0
	resp.Message = "success"
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

	// 从请求中获取自定义通道ID
	customChannelId := req.Channel.ChannelId
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

		// 更新自定义通道ID
		channel.DeviceChannel.CustomChannelId = customChannelId
	}

	// 从请求中获取自定义名称
	if req.Channel.Name != "" {
		channel.DeviceChannel.CustomName = req.Channel.Name
	}

	// 记录日志
	gb.Info("通道信息已更新",
		"通道ID", req.Id,
		"自定义通道ID", channel.DeviceChannel.CustomChannelId,
		"自定义名称", channel.DeviceChannel.CustomName)

	// 返回成功响应
	resp.Code = 0
	resp.Message = "通道信息更新成功"
	return resp, nil
}
