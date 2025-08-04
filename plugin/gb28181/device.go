package plugin_gb28181pro

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"m7s.live/v5"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/pkg/util"
	gb28181 "m7s.live/v5/plugin/gb28181/pkg"
	mrtp "m7s.live/v5/plugin/rtp/pkg"
)

type DeviceStatus string

const (
	DeviceRegisterStatus DeviceStatus = "REGISTER"
	DeviceRecoverStatus  DeviceStatus = "RECOVER"
	DeviceOnlineStatus   DeviceStatus = "ONLINE"
	DeviceOfflineStatus  DeviceStatus = "OFFLINE"
	DeviceAlarmedStatus  DeviceStatus = "ALARMED"
)

type DeviceKeepaliveTickTask struct {
	task.TickTask
	device  *Device
	seconds time.Duration
}

func (d *DeviceKeepaliveTickTask) GetTickInterval() time.Duration {
	return d.seconds
}

func (d *DeviceKeepaliveTickTask) Tick(any) {
	keepaliveSeconds := 60
	if d.device.KeepaliveInterval >= 5 {
		keepaliveSeconds = d.device.KeepaliveInterval
	}
	d.Debug("keepLiveTick", "deviceID", d.device.DeviceId,
		"keepaliveTime", d.device.KeepaliveTime,
		"interval", d.device.KeepaliveInterval,
		"count", d.device.KeepaliveCount)
	if timeDiff := time.Since(d.device.KeepaliveTime); timeDiff > time.Duration(d.device.KeepaliveCount*keepaliveSeconds)*time.Second {
		d.device.Online = false
		d.device.Status = DeviceOfflineStatus
		// 设置所有通道状态为off
		d.device.channels.Range(func(channel *Channel) bool {
			channel.Status = "OFF"
			return true
		})
	}
}

type Device struct {
	task.Job              `gorm:"-:all"`
	DeviceId              string          `gorm:"primaryKey"` // 设备国标编号
	Name                  string          // 设备名
	CustomName            string          // 自定义名称
	Manufacturer          string          // 生产厂商
	Model                 string          // 型号
	Firmware              string          // 固件版本
	Transport             string          // 传输协议（UDP/TCP）
	StreamMode            mrtp.StreamMode // 数据流传输模式（UDP:udp传输/TCP-ACTIVE：tcp主动模式/TCP-PASSIVE：tcp被动模式）
	IP                    string          // wan地址_ip
	Port                  int             // wan地址_port
	HostAddress           string          // wan地址
	Online                bool            // 是否在线，true为在线，false为离线
	RegisterTime          time.Time       // 注册时间
	KeepaliveTime         time.Time       // 心跳时间
	KeepaliveInterval     int             `gorm:"default:60" default:"60"` // 心跳间隔
	KeepaliveCount        int             `gorm:"default:3" default:"3"`   // 心跳次数
	ChannelCount          int             // 通道个数
	Expires               int             // 注册有效期
	CreateTime            time.Time       `gorm:"primaryKey"` // 创建时间
	UpdateTime            time.Time       // 更新时间
	Charset               string          // 字符集, 支持 UTF-8 与 GB2312
	SubscribeCatalog      int             `gorm:"default:0"` // 目录订阅周期，0为不订阅
	SubscribePosition     int             `gorm:"default:0"` // 移动设备位置订阅周期，0为不订阅
	PositionInterval      int             `gorm:"default:6"` // 移动设备位置信息上报时间间隔,单位:秒,默认值6
	SubscribeAlarm        int             `gorm:"default:0"` // 报警订阅周期，0为不订阅
	SSRCCheck             bool            // 是否开启ssrc校验，默认关闭，开启可以防止串流
	GeoCoordSys           string          // 地理坐标系， 目前支持 WGS84,GCJ02
	Password              string          // 密码
	SipIp                 string          // SIP交互IP（设备访问平台的IP）
	AsMessageChannel      bool            // 是否作为消息通道
	BroadcastPushAfterAck bool            // 控制语音对讲流程，释放收到ACK后发流
	DeletedAt             gorm.DeletedAt  `yaml:"-"`
	// 删除强关联字段
	// channels              []gb28181.DeviceChannel `gorm:"foreignKey:DeviceDBID;references:ID"` // 设备通道列表

	// 保留原有字段
	Status                DeviceStatus
	SN                    int
	Recipient             sip.Uri                               `gorm:"-:all"`
	channels              util.Collection[string, *Channel]     `gorm:"-:all"`
	catalogReqs           util.Collection[int, *CatalogRequest] `gorm:"-:all"`
	MediaIp               string                                `desc:"收流IP"`
	Longitude, Latitude   string                                // 经度,纬度
	eventChan             chan any                              `gorm:"-:all"`
	client                *sipgo.Client
	contactHDR            sip.ContactHeader
	fromHDR               sip.FromHeader
	toHDR                 sip.ToHeader
	plugin                *GB28181Plugin `gorm:"-:all"`
	LocalPort             int
	CatalogSubscribeTask  *CatalogSubscribeTask  `gorm:"-:all"`
	PositionSubscribeTask *PositionSubscribeTask `gorm:"-:all"`
	AlarmSubscribeTask    *AlarmSubscribeTask    `gorm:"-:all"`
}

func (d *Device) TableName() string {
	return "gb28181_device"
}

func (d *Device) Dispose() {
	//d.Online = false
	//d.Status = DeviceOfflineStatus
	if d.plugin.DB != nil {
		// 先删除该设备关联的所有channels
		if err := d.plugin.DB.Where("device_id = ?", d.DeviceId).Delete(&gb28181.DeviceChannel{}).Error; err != nil {
			d.Error("删除设备通道记录失败", "error", err)
		}

		// 保存当前内存中的channels
		if d.channels.Length > 0 {
			d.channels.Range(func(channel *Channel) bool {
				if err := d.plugin.DB.Create(channel.DeviceChannel).Error; err != nil {
					d.Error("保存设备通道记录失败", "error", err)
				}
				if channel.PullProxyTask != nil {
					channel.PullProxyTask.ChangeStatus(m7s.PullProxyStatusOffline)
				}
				d.plugin.channels.RemoveByKey(channel.ID)
				return true
			})
			d.channels.Clear()
		}
		// 保存设备信息
		d.plugin.DB.Save(d)
	}
}

func (d *Device) GetKey() string {
	return d.DeviceId
}

// CatalogRequest 目录请求结构体
type CatalogRequest struct {
	SN, SumNum, TotalCount int
	FirstResponse          bool // 是否为第一个响应
	*util.Promise
	sync.Mutex // 保护并发访问
}

func (r *CatalogRequest) GetKey() int {
	return r.SN
}

// AddResponse 处理响应并返回是否是第一个响应
func (r *CatalogRequest) AddResponse() bool {
	r.Lock()
	defer r.Unlock()
	fmt.Println("r.FirstResponse: " + fmt.Sprintf("%v", r.FirstResponse))
	wasFirst := r.FirstResponse
	r.FirstResponse = false
	fmt.Println("r.FirstResponse after: " + fmt.Sprintf("%v", r.FirstResponse))

	return wasFirst
}

// IsComplete 检查是否完成接收
func (r *CatalogRequest) IsComplete() bool {
	r.Lock()
	defer r.Unlock()
	return r.TotalCount >= r.SumNum
}

type CatalogHandlerQueueTask struct {
	task.Work
}

var catalogHandlerQueueTask CatalogHandlerQueueTask

type catalogHandlerTask struct {
	task.Task
	d   *Device
	msg *gb28181.Message
}

func (c *catalogHandlerTask) Run() (err error) {
	// 处理目录信息
	d := c.d
	msg := c.msg
	catalogReq, exists := d.catalogReqs.Get(msg.SN)
	d.Debug("into catalog", "msg.SN", msg.SN, "exists", exists)
	if !exists {
		// 创建新的目录请求
		catalogReq = &CatalogRequest{
			SN:            msg.SN,
			SumNum:        msg.SumNum,
			TotalCount:    0,
			FirstResponse: true,
			Promise:       util.NewPromise(context.Background()),
		}
		d.catalogReqs.Set(catalogReq)
		d.Debug("into catalog", "msg.SN", msg.SN, "d.catalogReqs", d.catalogReqs.Length)
	}

	// 添加响应并获取是否是第一个响应
	isFirst := catalogReq.AddResponse()

	// 更新设备信息到数据库
	// 如果是第一个响应，将所有通道状态标记为OFF
	if isFirst {
		d.Debug("将所有通道状态标记为OFF", "deviceId", d.DeviceId)
		// 标记所有通道为OFF状态
		d.channels.Range(func(channel *Channel) bool {
			if channel.DeviceChannel != nil {
				channel.DeviceChannel.Status = gb28181.ChannelOffStatus
			}
			return true
		})
	}

	// 更新通道信息
	for _, c := range msg.DeviceList.DeviceChannelList {
		// 设置关联的设备数据库ID
		c.ChannelId = c.DeviceId
		c.DeviceId = d.DeviceId
		c.ID = d.DeviceId + "_" + c.ChannelId
		if c.CustomChannelId == "" {
			c.CustomChannelId = c.ChannelId
		}
		d.Debug("msg.DeviceList.DeviceChannelList range", "c.ChannelId", c.ChannelId, "c.Status", c.Status)
		// 使用 Save 进行 upsert 操作
		d.addOrUpdateChannel(c)
		catalogReq.TotalCount++
	}

	// 更新当前设备的通道数
	d.ChannelCount = msg.SumNum
	d.UpdateTime = time.Now()
	d.Debug("save channel", "deviceid", d.DeviceId, " d.channels.Length", d.channels.Length, "d.ChannelCount", d.ChannelCount, "d.UpdateTime", d.UpdateTime)

	// 删除所有状态为OFF的通道
	// d.channels.Range(func(channel *Channel) bool {
	// 	if channel.DeviceChannel != nil && channel.DeviceChannel.Status == gb28181.ChannelOffStatus {
	// 		d.Debug("删除不存在的通道", "channelId", channel.ID)
	// 		d.channels.RemoveByKey(channel.ID)
	// 		d.plugin.channels.RemoveByKey(channel.ID)
	// 	}
	// 	return true
	// })

	// 在所有通道都添加完成后，检查是否完成接收
	if catalogReq.IsComplete() {
		d.Debug("IsComplete")
		catalogReq.Resolve()
		d.catalogReqs.RemoveByKey(msg.SN)
	}
	return
}

func (d *Device) onMessage(req *sip.Request, tx sip.ServerTransaction, msg *gb28181.Message) (err error) {
	d.plugin.Debug("into onMessage", "deviceid is ", d.DeviceId, "msg is", msg)
	source := req.Source()
	hostname, portStr, _ := net.SplitHostPort(source)
	port, _ := strconv.Atoi(portStr)
	if d.IP != hostname || d.Port != port {
		d.Recipient.Host = hostname
		d.Recipient.Port = port
	}
	d.IP = hostname
	d.Port = port
	d.HostAddress = hostname + ":" + portStr
	var body []byte
	//d.Online = true
	//if d.Status != DeviceOnlineStatus {
	//	d.Status = DeviceOnlineStatus
	//}
	//d.Debug("OnMessage", "cmdType", msg.CmdType, "body", string(req.Body()))
	switch msg.CmdType {
	case "Keepalive":
		d.KeepaliveInterval = int(time.Since(d.KeepaliveTime).Seconds())
		if d.KeepaliveInterval < 60 {
			d.KeepaliveInterval = 60
		}
		d.KeepaliveTime = time.Now()
		d.Debug("into keeplive,deviceid is ", d.DeviceId, "d.KeepaliveTime is", d.KeepaliveTime, "d.KeepaliveInterval is", d.KeepaliveInterval)
		if d.plugin.DB != nil {
			if err := d.plugin.DB.Model(d).Updates(map[string]interface{}{
				"keepalive_interval": d.KeepaliveInterval,
				"keepalive_time":     d.KeepaliveTime,
			}).Error; err != nil {
				d.Error("update keepalive info failed", "error", err)
			}
		}
	case "Catalog":
		catalogHandler := &catalogHandlerTask{
			d:   d,
			msg: msg,
		}
		catalogHandlerQueueTask.AddTask(catalogHandler)
	case "RecordInfo":
		if channel, ok := d.channels.Get(d.DeviceId + "_" + msg.DeviceID); ok {
			if req, ok := channel.RecordReqs.Get(msg.SN); ok {
				// 添加响应并检查是否完成
				if req.AddResponse(*msg) {
					req.Resolve()
				}
			}
		}
	case "PresetQuery":
		if channel, ok := d.channels.Get(d.DeviceId + "_" + msg.DeviceID); ok {
			if req, ok := channel.PresetReqs.Get(msg.SN); ok {
				// 添加预置位响应
				req.Response = msg.PresetList.Item
				req.Resolve()
			}
		}
		// 查询平台信息
		type Result struct {
			PlatformServerGBID string `gorm:"column:platform_server_gb_id"`
		}
		var result Result
		if d.plugin.DB != nil {
			if err := d.plugin.DB.Table("gb28181_platform_channel gpc").
				Select("gpc.platform_server_gb_id").
				Joins("LEFT JOIN gb28181_channel gc on gpc.channel_db_id= gc.id").
				Where("gc.channel_id = ?", msg.DeviceID).
				First(&result).Error; err != nil {
				d.Error("查询平台信息失败", "error", err)
				return err
			}
			// 从platforms集合中获取平台实例
			if platform, ok := d.plugin.platforms.Get(result.PlatformServerGBID); ok {
				// 创建并发送响应消息
				request := platform.CreateRequest("MESSAGE")
				fromTag, _ := req.From().Params.Get("tag")
				// 设置From头部
				fromHeader := sip.FromHeader{
					Address: sip.Uri{
						User: platform.PlatformModel.DeviceGBID,
						Host: platform.PlatformModel.ServerGBDomain,
					},
					Params: sip.NewParams(),
				}
				fromHeader.Params.Add("tag", fromTag)
				request.AppendHeader(&fromHeader)

				// 添加To头部
				toHeader := sip.ToHeader{
					Address: sip.Uri{
						User: platform.PlatformModel.ServerGBID,
						Host: platform.PlatformModel.ServerGBDomain,
					},
				}
				request.AppendHeader(&toHeader)

				// 添加Via头部
				viaHeader := sip.ViaHeader{
					ProtocolName:    "SIP",
					ProtocolVersion: "2.0",
					Transport:       platform.PlatformModel.Transport,
					Host:            platform.PlatformModel.DeviceIP,
					Port:            platform.PlatformModel.DevicePort,
					Params:          sip.NewParams(),
				}
				viaHeader.Params.Add("branch", sip.GenerateBranchN(16)).Add("rport", "")
				request.AppendHeader(&viaHeader)

				// 设置Content-Type
				contentTypeHeader := sip.ContentTypeHeader("Application/MANSCDP+xml")
				request.AppendHeader(&contentTypeHeader)

				// 直接使用原始消息体
				request.SetBody(req.Body())

				// 发送请求
				_, err = platform.Client.Do(platform, request)
				if err != nil {
					d.Error("发送预置位查询响应失败", "error", err)
					return err
				}
			}
		}
	case "DeviceStatus":
		d.UpdateTime = time.Now()
	case "DeviceInfo":
		// 主设备信息
		d.Info("DeviceInfo message", "body", req.Body(), "d.Name", d.Name, "d.DeviceId", d.DeviceId, "msg.DeviceName", msg.DeviceName)
		if msg.DeviceName != "" {
			d.Name = msg.DeviceName
			if d.CustomName == "" {
				d.CustomName = msg.DeviceName
			}
		}
		d.Manufacturer = msg.Manufacturer
		d.Model = msg.Model
		d.Firmware = msg.Firmware
		d.UpdateTime = time.Now()
		d.Latitude = msg.Latitude
		d.Longitude = msg.Longitude
	case "Alarm":
		// 创建报警记录
		alarm := &gb28181.DeviceAlarm{
			DeviceID:      d.DeviceId, // 使用当前设备的ID
			DeviceName:    d.Name,
			ChannelID:     msg.DeviceID, // 使用消息中的DeviceID作为通道ID
			AlarmPriority: msg.AlarmPriority,
			AlarmMethod:   msg.AlarmMethod,
			AlarmType:     msg.Info.AlarmType,
			CreateTime:    time.Now(),
		}

		// 尝试解析报警时间
		loc, _ := time.LoadLocation("Local")
		alarmTime, err := time.ParseInLocation("2006-1-2T15:4:5", msg.AlarmTime, loc)
		if err != nil {
			// 如果使用非标准格式解析失败，尝试使用标准格式
			alarmTime, err = time.ParseInLocation("2006-01-02T15:04:05", msg.AlarmTime, loc)
			if err != nil {
				d.Error("解析报警时间失败", "error", err)
				alarmTime = time.Now().UTC()
			}
		}
		// 将本地时间转换为 UTC
		alarm.AlarmTime = alarmTime.UTC()

		// 保存到数据库
		if d.plugin.DB != nil {
			if err := d.plugin.DB.Create(alarm).Error; err != nil {
				d.Error("保存报警信息失败", "error", err)
			} else {
				d.Info("保存报警信息成功",
					"deviceId", alarm.DeviceID,
					"channelId", alarm.ChannelID,
					"alarmType", alarm.GetAlarmTypeDescription(),
					"alarmMethod", alarm.GetAlarmMethodDescription(),
					"alarmPriority", alarm.GetAlarmPriorityDescription())
			}
		}
	case "Broadcast":
		d.Info("Broadcast message", "body", req.Body())
	case "DeviceControl":
		d.Info("DeviceControl message", "body", req.Body())
	default:
		d.Warn("Not supported CmdType", "CmdType", msg.CmdType, "body", req.Body())
		err = tx.Respond(sip.NewResponseFromRequest(req, http.StatusBadRequest, "", nil))
		return
	}
	err = tx.Respond(sip.NewResponseFromRequest(req, http.StatusOK, "OK", body))
	return
}

func (d *Device) send(req *sip.Request) (*sip.Response, error) {
	d.SN++
	d.Trace("send", "req", req.String())
	return d.client.Do(context.Background(), req)
}

func (d *Device) Go() (err error) {
	d.Trace("into device.Go,deviceid is ", d.DeviceId)
	var response *sip.Response

	// 初始化catalogReqs
	d.catalogReqs.L = new(sync.RWMutex)

	response, err = d.queryDeviceInfo()
	if err != nil {
		d.Error("queryDeviceInfo", "err", err)
	}
	response, err = d.queryDeviceStatus()
	if err != nil {
		d.Error("queryDeviceStatus", "err", err)
	}
	response, err = d.catalog()
	if err != nil {
		d.Error("catalog", "err", err)
	} else {
		d.Trace("catalog", "response", response.String())
	}

	// 创建并启动目录订阅任务
	if d.SubscribeCatalog > 0 {
		d.AddTask(NewCatalogSubscribeTask(d))
	}

	// 创建并启动位置订阅任务
	if d.SubscribePosition > 0 {
		d.AddTask(NewPositionSubscribeTask(d))
	}
	deviceKeepaliveTickTask := &DeviceKeepaliveTickTask{
		seconds: time.Second * 30,
		device:  d,
	}
	d.AddTask(deviceKeepaliveTickTask)
	return deviceKeepaliveTickTask.WaitStopped()
}

func (d *Device) CreateRequest(Method sip.RequestMethod, Recipient any) *sip.Request {
	var req *sip.Request
	if recipient, ok := Recipient.(sip.Uri); ok {
		req = sip.NewRequest(Method, recipient)
	} else {
		req = sip.NewRequest(Method, d.Recipient)
	}
	fromHDR := d.fromHDR
	fromHDR.Params.Add("tag", sip.GenerateTagN(32))
	req.AppendHeader(&fromHDR)
	contentType := sip.ContentTypeHeader("Application/MANSCDP+xml")
	req.AppendHeader(sip.NewHeader("User-Agent", "M7S/"+m7s.Version))
	req.AppendHeader(&contentType)
	toHeader := sip.ToHeader{
		Address: sip.Uri{User: d.DeviceId, Host: d.HostAddress},
	}
	req.AppendHeader(&toHeader)
	//viaHeader := sip.ViaHeader{
	//	ProtocolName:    "SIP",
	//	ProtocolVersion: "2.0",
	//	Transport:       "UDP",
	//	Host:            d.SipIp,
	//	Port:            d.LocalPort,
	//	Params:          sip.HeaderParams(sip.NewParams()),
	//}
	//viaHeader.Params.Add("branch", sip.GenerateBranchN(10)).Add("rport", "")
	//req.AppendHeader(&viaHeader)
	req.AppendHeader(&d.contactHDR)
	return req
}

func (d *Device) catalog() (*sip.Response, error) {
	request := d.CreateRequest(sip.MESSAGE, nil)
	//d.subscriber.Timeout = time.Now().Add(time.Second * time.Duration(expires))
	request.AppendHeader(sip.NewHeader("Expires", "3600"))
	request.SetBody(gb28181.BuildCatalogXML(d.Charset, d.SN, d.DeviceId))
	return d.send(request)
}

func (d *Device) subscribeCatalog() (*sip.Response, error) {
	request := d.CreateRequest(sip.SUBSCRIBE, nil)
	request.AppendHeader(sip.NewHeader("Expires", strconv.Itoa(d.SubscribeCatalog)))
	request.SetBody(gb28181.BuildCatalogXML(d.Charset, d.SN, d.DeviceId))
	return d.send(request)
}

func (d *Device) queryDeviceInfo() (*sip.Response, error) {
	request := d.CreateRequest(sip.MESSAGE, nil)
	request.SetBody(gb28181.BuildDeviceInfoXML(d.SN, d.DeviceId, d.Charset))
	return d.send(request)
}

func (d *Device) queryDeviceStatus() (*sip.Response, error) {
	request := d.CreateRequest(sip.MESSAGE, nil)
	request.SetBody(gb28181.BuildDeviceStatusXML(d.SN, d.DeviceId, d.Charset))
	return d.send(request)
}

func (d *Device) subscribePosition(interval int) (*sip.Response, error) {
	request := d.CreateRequest(sip.SUBSCRIBE, nil)
	request.AppendHeader(sip.NewHeader("Expires", strconv.Itoa(d.SubscribePosition)))
	request.SetBody(gb28181.BuildDevicePositionXML(d.SN, d.DeviceId, interval))
	return d.send(request)
}

// frontEndCmd 前端控制命令，包括PTZ指令、FI指令、预置位指令、巡航指令、扫描指令和辅助开关指令
func (d *Device) frontEndCmd(channelId string, cmdStr string) (*sip.Response, error) {
	// 构建前端控制指令字符串
	//cmdStr := d.frontEndCmdString(cmdCode, parameter1, parameter2, combineCode2)

	// 构建XML消息体
	ptzXml := strings.Builder{}
	ptzXml.WriteString(fmt.Sprintf("<?xml version=\"1.0\" encoding=\"%s\"?>\r\n", d.Charset))
	ptzXml.WriteString("<Control>\r\n")
	ptzXml.WriteString("<CmdType>DeviceControl</CmdType>\r\n")
	ptzXml.WriteString(fmt.Sprintf("<SN>%d</SN>\r\n", int(time.Now().UnixNano()/1e6%1000000)))
	ptzXml.WriteString(fmt.Sprintf("<DeviceID>%s</DeviceID>\r\n", channelId))
	ptzXml.WriteString(fmt.Sprintf("<PTZCmd>%s</PTZCmd>\r\n", cmdStr))
	ptzXml.WriteString("<Info>\r\n")
	ptzXml.WriteString("<ControlPriority>5</ControlPriority>\r\n")
	ptzXml.WriteString("</Info>\r\n")
	ptzXml.WriteString("</Control>\r\n")

	// 创建并发送请求
	request := d.CreateRequest(sip.MESSAGE, nil)
	request.SetBody([]byte(ptzXml.String()))
	return d.send(request)
}

// frontEndCmdString 生成前端控制指令字符串
func (d *Device) frontEndCmdString(cmdCode int32, parameter1 int32, parameter2 int32, combineCode2 int32) string {
	// 构建指令字符串
	var builder strings.Builder
	builder.WriteString("A50F01")

	// 添加指令码
	builder.WriteString(fmt.Sprintf("%02X", cmdCode))

	// 添加参数1
	builder.WriteString(fmt.Sprintf("%02X", parameter1))

	// 添加参数2
	builder.WriteString(fmt.Sprintf("%02X", parameter2))

	// 添加组合码2（左移4位）
	builder.WriteString(fmt.Sprintf("%02X", combineCode2<<4))

	// 计算校验码
	checkCode := (0xA5 + 0x0F + 0x01 + int(cmdCode) + int(parameter1) + int(parameter2) + int(combineCode2<<4)) % 0x100
	builder.WriteString(fmt.Sprintf("%02X", checkCode))

	return builder.String()
}

func (d *Device) addOrUpdateChannel(c gb28181.DeviceChannel) {
	// 设置通道状态为在线
	c.Status = gb28181.ChannelOnStatus

	if channel, ok := d.channels.Get(c.ID); ok {
		// 通道已存在，保留自定义字段
		if channel.DeviceChannel != nil {
			// 保存原有的自定义字段
			customName := channel.DeviceChannel.CustomName
			customChannelId := channel.DeviceChannel.CustomChannelId

			// 如果原有字段有值，则保留
			if customName != "" {
				c.CustomName = customName
			}
			if customChannelId != "" {
				c.CustomChannelId = customChannelId
			}
		}
		// 更新通道信息
		channel.DeviceChannel = &c
		d.channels.Range(func(channel *Channel) bool {
			d.Debug("range d.channels", "channel.ChannelId", channel.ChannelId, "channel.status", channel.Status)
			return true
		})
	} else {
		// 创建新通道
		channel = &Channel{
			Device:        d,
			Logger:        d.Logger.With("channel", c.ID),
			DeviceChannel: &c,
		}
		d.channels.Set(channel)
		d.plugin.channels.Set(channel)
	}
}

func (d *Device) GetID() string {
	return d.DeviceId
}

func (d *Device) GetIP() string {
	return d.IP
}

func (d *Device) GetStreamMode() mrtp.StreamMode {
	return d.StreamMode
}

func (d *Device) Send(req *sip.Request) (*sip.Response, error) {
	return d.send(req)
}

func (d *Device) CreateSSRC(serial string) uint16 {
	// 使用简单的 hash 函数将设备 ID 转换为 uint16
	var hash uint16
	for i := 0; i < len(d.DeviceId); i++ {
		hash = hash*31 + uint16(d.DeviceId[i])
	}
	return hash
}

// recordCmd 录制控制命令
func (d *Device) recordCmd(channelId string, cmdType string) (*sip.Response, error) {
	// 构建XML消息体
	recordXml := strings.Builder{}
	recordXml.WriteString(fmt.Sprintf("<?xml version=\"1.0\" encoding=\"%s\"?>\r\n", d.Charset))
	recordXml.WriteString("<Control>\r\n")
	recordXml.WriteString("<CmdType>DeviceControl</CmdType>\r\n")
	recordXml.WriteString(fmt.Sprintf("<SN>%d</SN>\r\n", int(time.Now().UnixNano()/1e6%1000000)))
	recordXml.WriteString(fmt.Sprintf("<DeviceID>%s</DeviceID>\r\n", channelId))
	recordXml.WriteString(fmt.Sprintf("<RecordCmd>%s</RecordCmd>\r\n", cmdType))
	recordXml.WriteString("</Control>\r\n")

	// 创建并发送请求
	request := d.CreateRequest(sip.MESSAGE, nil)
	request.SetBody([]byte(recordXml.String()))
	return d.send(request)
}

// SnapshotConfig 抓拍配置结构体
type SnapshotConfig struct {
	SnapNum   int    `json:"snapNum"`   // 连拍张数(1-10张)
	Interval  int    `json:"interval"`  // 单张抓拍间隔(单位:秒，最小1秒)
	UploadURL string `json:"uploadUrl"` // 抓拍图片上传路径
	SessionID string `json:"sessionId"` // 会话ID，用于标识抓拍会话
}

// BuildSnapshotConfigXML 生成抓拍配置XML
func (d *Device) BuildSnapshotConfigXML(config SnapshotConfig, channelID string) string {
	// 参数验证和限制
	if config.SnapNum < 1 {
		config.SnapNum = 1
	} else if config.SnapNum > 10 {
		config.SnapNum = 10
	}
	if config.Interval < 1 {
		config.Interval = 1
	}

	xml := strings.Builder{}
	xml.WriteString(fmt.Sprintf("<?xml version=\"1.0\" encoding=\"%s\"?>\r\n", d.Charset))
	xml.WriteString("<Control>\r\n")
	xml.WriteString("<CmdType>DeviceConfig</CmdType>\r\n")
	xml.WriteString(fmt.Sprintf("<SN>%d</SN>\r\n", d.SN))
	xml.WriteString(fmt.Sprintf("<DeviceID>%s</DeviceID>\r\n", channelID))
	xml.WriteString("<SnapShotConfig>\r\n")
	xml.WriteString(fmt.Sprintf("<SnapNum>%d</SnapNum>\r\n", config.SnapNum))
	xml.WriteString(fmt.Sprintf("<Interval>%d</Interval>\r\n", config.Interval))
	xml.WriteString(fmt.Sprintf("<UploadURL>%s</UploadURL>\r\n", config.UploadURL))
	xml.WriteString(fmt.Sprintf("<SessionID>%s</SessionID>\r\n", config.SessionID))
	xml.WriteString("</SnapShotConfig>\r\n")
	xml.WriteString("</Control>\r\n")

	return xml.String()
}

func (d *Device) onNotify(req *sip.Request, tx sip.ServerTransaction, msg *gb28181.Message) error {
	// 首先尝试解析为 Notify 消息
	notifyBody := req.Body()
	if strings.Contains(string(notifyBody), "<Notify>") {
		// 处理 Notify 通知
		notify := &gb28181.AlarmNotify{}
		if err := gb28181.DecodeXML(notify, notifyBody); err != nil {
			return fmt.Errorf("decode notify xml error: %v", err)
		}

		if notify.CmdType == "MobilePosition" {
			// 处理 MobilePosition 通知
			posNotify := &gb28181.MobilePositionNotify{}
			if err := gb28181.DecodeXML(posNotify, notifyBody); err != nil {
				return fmt.Errorf("decode mobile position notify xml error: %v", err)
			}

			// 解析GPS时间
			loc, _ := time.LoadLocation("Local")
			gpsTime, err := time.ParseInLocation("2006-1-2T15:4:5", posNotify.Time, loc)
			if err != nil {
				// 如果使用非标准格式解析失败，尝试使用标准格式
				gpsTime, err = time.ParseInLocation("2006-01-02T15:04:05", posNotify.Time, loc)
				if err != nil {
					d.Error("parse gps time error", "err", err)
					gpsTime = time.Now().UTC() // 如果解析失败，使用当前UTC时间
				}
			}
			// 将本地时间转换为 UTC
			gpsTime = gpsTime.UTC()

			// 更新设备的经纬度信息
			d.Longitude = fmt.Sprintf("%.6f", posNotify.Longitude)
			d.Latitude = fmt.Sprintf("%.6f", posNotify.Latitude)
			d.UpdateTime = time.Now()

			// 如果需要，可以将更新保存到数据库
			if d.plugin.DB != nil {
				// 更新设备表中的位置信息
				if err := d.plugin.DB.Model(&Device{}).
					Where("device_id = ?", d.DeviceId).
					Updates(map[string]interface{}{
						"longitude":   d.Longitude,
						"latitude":    d.Latitude,
						"update_time": d.UpdateTime,
					}).Error; err != nil {
					d.Error("update device position error", "err", err)
				}

				// 创建新的位置记录
				position := &gb28181.DevicePosition{
					DeviceID:   posNotify.DeviceID,
					GpsTime:    gpsTime,
					Longitude:  posNotify.Longitude,
					Latitude:   posNotify.Latitude,
					CreateTime: time.Now(),
				}

				// 保存位置记录到数据库
				if err := d.plugin.DB.Create(position).Error; err != nil {
					d.Error("save device position record error", "err", err)
				} else {
					d.Info("save device position record success",
						"deviceId", posNotify.DeviceID,
						"gpsTime", gpsTime,
						"longitude", posNotify.Longitude,
						"latitude", posNotify.Latitude)
				}
			}
			return nil
		} else if notify.CmdType == "Alarm" {
			// 创建报警记录
			alarm := &gb28181.DeviceAlarm{
				DeviceID:      d.DeviceId, // 使用当前设备的ID
				DeviceName:    d.Name,
				ChannelID:     notify.DeviceID, // 使用通知中的DeviceID作为通道ID
				AlarmPriority: notify.AlarmPriority,
				AlarmMethod:   notify.AlarmMethod,
				AlarmType:     notify.Info.AlarmType,
				CreateTime:    time.Now(),
			}

			// 解析报警时间
			loc, _ := time.LoadLocation("Local")
			alarmTime, err := time.ParseInLocation("2006-1-2T15:4:5", notify.AlarmTime, loc)
			if err != nil {
				// 如果使用非标准格式解析失败，尝试使用标准格式
				alarmTime, err = time.ParseInLocation("2006-01-02T15:04:05", notify.AlarmTime, loc)
				if err != nil {
					d.Error("解析报警时间失败", "error", err)
					alarmTime = time.Now().UTC()
				}
			}
			// 将本地时间转换为 UTC
			alarm.AlarmTime = alarmTime.UTC()

			// 保存到数据库
			if d.plugin.DB != nil {
				if err := d.plugin.DB.Create(alarm).Error; err != nil {
					d.Error("保存报警信息失败", "error", err)
				} else {
					d.Info("保存报警信息成功",
						"deviceId", alarm.DeviceID,
						"channelId", alarm.ChannelID,
						"alarmType", alarm.GetAlarmTypeDescription(),
						"alarmMethod", alarm.GetAlarmMethodDescription(),
						"alarmPriority", alarm.GetAlarmPriorityDescription())
				}
			}
			return nil
		}
		return fmt.Errorf("unknown notify cmdtype: %s", notify.CmdType)
	}

	// 如果不是 Notify 消息，尝试按 Response 消息处理
	if strings.Contains(string(notifyBody), "<Response>") {
		// 重新解析为 Response 消消息
		response := &gb28181.Message{}
		if err := gb28181.DecodeXML(response, notifyBody); err != nil {
			return fmt.Errorf("decode response xml error: %v", err)
		}

		// 按照 Message 处理（与 OnMessage 相同的逻辑）
		if response.CmdType == "Catalog" {
			return d.handleCatalog(response)
		}
		return fmt.Errorf("unknown response cmdtype: %s", response.CmdType)
	}

	return fmt.Errorf("unknown notify message type")
}

// handleCatalog 处理设备目录更新
func (d *Device) handleCatalog(msg *gb28181.Message) error {
	if msg.DeviceList.DeviceChannelList == nil || len(msg.DeviceList.DeviceChannelList) == 0 {
		return fmt.Errorf("no device items in catalog")
	}

	// 遍历并更新设备列表
	for _, item := range msg.DeviceList.DeviceChannelList {
		channel := &gb28181.DeviceChannel{
			DeviceId:     item.DeviceId,
			Name:         item.Name,
			Manufacturer: item.Manufacturer,
			Model:        item.Model,
			Owner:        item.Owner,
			CivilCode:    item.CivilCode,
			Address:      item.Address,
			Parental:     item.Parental,
			ParentId:     item.ParentId,
			SafetyWay:    item.SafetyWay,
			RegisterWay:  item.RegisterWay,
			Secrecy:      item.Secrecy,
			Status:       item.Status,
		}

		// 添加或更新通道
		d.addOrUpdateChannel(*channel)

		// 如果需要，保存到数据库
		if d.plugin.DB != nil {
			var existingChannel gb28181.DeviceChannel
			result := d.plugin.DB.Where("channel_id = ?", channel.DeviceId).First(&existingChannel)
			if result.Error != nil {
				// 通道不存在，创建新通道
				channel.DeviceId = d.DeviceId // 设置设备ID
				if err := d.plugin.DB.Create(channel).Error; err != nil {
					d.Error("create channel error", "err", err)
				}
			} else {
				// 通道存在，更新通道
				if err := d.plugin.DB.Model(&existingChannel).Updates(channel).Error; err != nil {
					d.Error("update channel error", "err", err)
				}
			}
		}
	}

	return nil
}

// AlarmXML 报警订阅xml样式
const AlarmXML = `<?xml version="1.0" encoding="GB2312"?>
<Query>
<CmdType>Alarm</CmdType>
<SN>%d</SN>
<DeviceID>%s</DeviceID>
<StartAlarmPriority>0</StartAlarmPriority>
<EndAlarmPriority>0</EndAlarmPriority>
<AlarmMethod>0</AlarmMethod>
</Query>
`

// subscribeAlarm 订阅报警信息
func (d *Device) subscribeAlarm() (*sip.Response, error) {
	request := d.CreateRequest(sip.SUBSCRIBE, nil)
	request.AppendHeader(sip.NewHeader("Event", "presence"))
	request.AppendHeader(sip.NewHeader("Expires", strconv.Itoa(d.SubscribeAlarm)))
	request.SetBody([]byte(fmt.Sprintf(AlarmXML, d.SN, d.DeviceId)))
	return d.send(request)
}
