package plugin_gb28181pro

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"m7s.live/v5"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/pkg/util"
	gb28181 "m7s.live/v5/plugin/gb28181/pkg"
)

type DeviceStatus string

const (
	DeviceRegisterStatus DeviceStatus = "REGISTER"
	DeviceRecoverStatus  DeviceStatus = "RECOVER"
	DeviceOnlineStatus   DeviceStatus = "ONLINE"
	DeviceOfflineStatus  DeviceStatus = "OFFLINE"
	DeviceAlarmedStatus  DeviceStatus = "ALARMED"
)

type Device struct {
	task.Task             `gorm:"-:all"`
	ID                    int64     `gorm:"primaryKey;autoIncrement"` // 数据库自增长ID
	DeviceID              string    // 设备国标编号
	Name                  string    // 设备名
	Manufacturer          string    // 生产厂商
	Model                 string    // 型号
	Firmware              string    // 固件版本
	Transport             string    // 传输协议（UDP/TCP）
	StreamMode            string    // 数据流传输模式（UDP:udp传输/TCP-ACTIVE：tcp主动模式/TCP-PASSIVE：tcp被动模式）
	IP                    string    // wan地址_ip
	Port                  int       // wan地址_port
	HostAddress           string    // wan地址
	Online                bool      // 是否在线，true为在线，false为离线
	RegisterTime          time.Time // 注册时间
	KeepaliveTime         time.Time // 心跳时间
	KeepaliveInterval     int       `gorm:"default:60"` // 心跳间隔
	ChannelCount          int       // 通道个数
	Expires               int       // 注册有效期
	CreateTime            time.Time // 创建时间
	UpdateTime            time.Time // 更新时间
	Charset               string    // 字符集, 支持 UTF-8 与 GB2312
	SubscribeCatalog      int       // 目录订阅周期，0为不订阅
	SubscribePosition     int       // 移动设备位置订阅周期，0为不订阅
	PositionInterval      int       // 移动设备位置信息上报时间间隔,单位:秒,默认值5
	SubscribeAlarm        int       // 报警订阅周期，0为不订阅
	SSRCCheck             bool      // 是否开启ssrc校验，默认关闭，开启可以防止串流
	GeoCoordSys           string    // 地理坐标系， 目前支持 WGS84,GCJ02
	Password              string    // 密码
	SdpIP                 string    // 收流IP
	LocalIP               string    // SIP交互IP（设备访问平台的IP）
	AsMessageChannel      bool      // 是否作为消息通道
	BroadcastPushAfterAck bool      // 控制语音对讲流程，释放收到ACK后发流
	// 删除强关联字段
	// Channels              []gb28181.DeviceChannel `gorm:"foreignKey:DeviceDBID;references:ID"` // 设备通道列表

	// 保留原有字段
	Status              DeviceStatus
	SN                  int
	Recipient           sip.Uri                           `gorm:"-:all"`
	channels            util.Collection[string, *Channel] `gorm:"-:all"`
	mediaIp             string
	Longitude, Latitude string // 经度,纬度
	eventChan           chan any
	client              *sipgo.Client
	contactHDR          sip.ContactHeader
	fromHDR             sip.FromHeader
	toHDR               sip.ToHeader
	plugin              *GB28181Plugin
	LocalPort           int
	WanIP               string
	WanPort             int
}

func (d *Device) TableName() string {
	return "device_gb28181pro"
}

func (d *Device) Dispose() {
	if d.plugin.DB != nil {
		d.plugin.DB.Save(d)
	}
}

func (d *Device) GetKey() string {
	return d.DeviceID
}

func (d *Device) onMessage(req *sip.Request, tx sip.ServerTransaction, msg *gb28181.Message) (err error) {
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
		d.KeepaliveTime = time.Now()
		if d.plugin.DB != nil {
			d.plugin.DB.Save(d)
		}
	case "Catalog":
		d.eventChan <- msg.DeviceChannelList
		// 更新设备信息到数据库
		if d.plugin.DB != nil {
			// 更新通道信息
			for _, c := range msg.DeviceChannelList {
				// 设置关联的设备数据库ID
				c.DeviceDBID = d.ID
				// 先查询是否存在
				var existing gb28181.DeviceChannel
				if err := d.plugin.DB.Where(&gb28181.DeviceChannel{
					DeviceDBID: d.ID,
					DeviceID:   c.DeviceID,
				}).First(&existing).Error; err == nil {
					c.ID = existing.ID // 保持原有的自增ID
					d.Debug("update channel", "channelId", c.DeviceID)
				} else {
					d.Debug("create channel", "channelId", c.DeviceID)
				}
				// 使用 Save 进行 upsert 操作
				if err := d.plugin.DB.Save(&c).Error; err != nil {
					d.Error("save channel failed", "error", err)
				}
			}
			// 更新当前设备的通道数
			d.ChannelCount = msg.SumNum
			d.UpdateTime = time.Now()
			d.Debug("save channel", "deviceid", d.DeviceID)
			if err := d.plugin.DB.Save(d).Error; err != nil {
				d.Error("save device failed", "error", err)
			}
		}
	case "RecordInfo":
		if channel, ok := d.channels.Get(msg.DeviceID); ok {
			if req, ok := channel.RecordReqs.Get(msg.SN); ok {
				// 添加响应并检查是否完成
				if req.AddResponse(*msg) {
					req.Resolve()
				}
			}
		}
	case "PresetQuery":
		if channel, ok := d.channels.Get(msg.DeviceID); ok {
			if req, ok := channel.PresetReqs.Get(msg.SN); ok {
				// 添加预置位响应
				req.Response = msg.PresetList.Item
				req.Resolve()
			}
		}
		// 查询平台信息
		type Result struct {
			PlatformID uint32 `gorm:"column:platform_id"`
		}
		var result Result
		if d.plugin.DB != nil {
			if err := d.plugin.DB.Table("platform_channel_gb28181pro pcg").
				Select("pcg.platform_id").
				Joins("LEFT JOIN channel_gb28181pro cg on pcg.device_channel_id= cg.id").
				Where("cg.device_id = ?", msg.DeviceID).
				First(&result).Error; err != nil {
				d.Error("查询平台信息失败", "error", err)
				return err
			}
			// 从platforms集合中获取平台实例
			if platform, ok := d.plugin.platforms.Get(result.PlatformID); ok {
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
				_, err = platform.Client.Do(platform.ctx, request)
				if err != nil {
					d.Error("发送预置位查询响应失败", "error", err)
					return err
				}
			}
		}
	case "DeviceStatus":
		if d.plugin.DB != nil {
			d.UpdateTime = time.Now()
			d.plugin.DB.Save(d)
		}
	case "DeviceInfo":
		// 主设备信息
		d.Name = msg.DeviceName
		d.Manufacturer = msg.Manufacturer
		d.Model = msg.Model
		d.Firmware = msg.Firmware
		// 更新设备信息到数据库
		if d.plugin.DB != nil {
			d.UpdateTime = time.Now()
			d.plugin.DB.Save(d)
		}
	case "Alarm":
		// 创建报警记录
		alarm := &gb28181.DeviceAlarm{
			DeviceID:      d.DeviceID, // 使用当前设备的ID
			DeviceName:    d.Name,
			ChannelID:     msg.DeviceID, // 使用消息中的DeviceID作为通道ID
			AlarmPriority: msg.AlarmPriority,
			AlarmMethod:   msg.AlarmMethod,
			AlarmType:     msg.Info.AlarmType,
			CreateTime:    time.Now(),
		}

		// 尝试解析报警时间
		if alarmTime, err := time.Parse("2006-01-02T15:04:05", msg.AlarmTime); err == nil {
			alarm.AlarmTime = alarmTime
		} else {
			alarm.AlarmTime = time.Now()
			d.Error("解析报警时间失败", "error", err)
		}

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
	d.Debug("send", "req", req.String())
	return d.client.Do(context.Background(), req)
}

func (d *Device) Go() (err error) {
	var response *sip.Response
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
		d.Debug("catalog", "response", response.String())
	}
	subTick := time.NewTicker(time.Second * 3600)
	defer subTick.Stop()
	catalogTick := time.NewTicker(time.Second * 60)
	defer catalogTick.Stop()
	for {
		select {
		case <-d.Done():
		case <-subTick.C:
			response, err = d.subscribeCatalog()
			if err != nil {
				d.Error("subCatalog", "err", err)
			} else {
				d.Debug("subCatalog", "response", response.String())
			}
			response, err = d.subscribePosition(int(6))
			if err != nil {
				d.Error("subPosition", "err", err)
			} else {
				d.Debug("subPosition", "response", response.String())
			}
		case <-catalogTick.C:
			if time.Since(d.KeepaliveTime) > time.Second*time.Duration(d.Expires) {
				d.Error("keepalive timeout", "keepaliveTime", d.KeepaliveTime)
				return
			}
			response, err = d.catalog()
			if err != nil {
				d.Error("catalog", "err", err)
			} else {
				d.Debug("catalogTick", "response", response.String())
			}
		case event := <-d.eventChan:
			d.Debug("eventChan", "event", event)
			switch v := event.(type) {
			case []gb28181.DeviceChannel:
				for _, c := range v {
					//当父设备非空且存在时、父设备节点增加通道
					if c.ParentID != "" {
						path := strings.Split(c.ParentID, "/")
						parentId := path[len(path)-1]
						//如果父ID并非本身所属设备，一般情况下这是因为下级设备上传了目录信息，该信息通常不需要处理。
						// 暂时不考虑级联目录的实现
						if d.DeviceID != parentId {
							if parent, ok := d.plugin.devices.Get(parentId); ok {
								parent.addOrUpdateChannel(c)
								continue
							} else {
								c.Model = "Directory " + c.Model
								c.Status = "NoParent"
							}
						}
					}
					d.addOrUpdateChannel(c)
				}
			}
		}
	}
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
		Address: sip.Uri{User: d.DeviceID, Host: d.HostAddress},
	}
	req.AppendHeader(&toHeader)
	//viaHeader := sip.ViaHeader{
	//	ProtocolName:    "SIP",
	//	ProtocolVersion: "2.0",
	//	Transport:       "UDP",
	//	Host:            d.LocalIP,
	//	Port:            d.LocalPort,
	//	Params:          sip.HeaderParams(sip.NewParams()),
	//}
	//viaHeader.Params.Add("branch", sip.GenerateBranchN(10)).Add("rport", "")
	//req.AppendHeader(&viaHeader)
	//req.AppendHeader(&d.contactHDR)
	return req
}

func (d *Device) catalog() (*sip.Response, error) {
	request := d.CreateRequest(sip.MESSAGE, nil)
	//d.subscriber.Timeout = time.Now().Add(time.Second * time.Duration(expires))
	request.AppendHeader(sip.NewHeader("Expires", "3600"))
	request.SetBody(gb28181.BuildCatalogXML(d.Charset, d.SN, d.DeviceID))
	return d.send(request)
}

func (d *Device) subscribeCatalog() (*sip.Response, error) {
	request := d.CreateRequest(sip.SUBSCRIBE, nil)
	request.AppendHeader(sip.NewHeader("Expires", "3600"))
	request.SetBody(gb28181.BuildCatalogXML(d.Charset, d.SN, d.DeviceID))
	return d.send(request)
}

func (d *Device) queryDeviceInfo() (*sip.Response, error) {
	request := d.CreateRequest(sip.MESSAGE, nil)
	request.SetBody(gb28181.BuildDeviceInfoXML(d.SN, d.DeviceID, d.Charset))
	return d.send(request)
}

func (d *Device) queryDeviceStatus() (*sip.Response, error) {
	request := d.CreateRequest(sip.MESSAGE, nil)
	request.SetBody(gb28181.BuildDeviceStatusXML(d.SN, d.DeviceID, d.Charset))
	return d.send(request)
}

func (d *Device) subscribePosition(interval int) (*sip.Response, error) {
	request := d.CreateRequest(sip.SUBSCRIBE, nil)
	request.AppendHeader(sip.NewHeader("Expires", "3600"))
	request.SetBody(gb28181.BuildDevicePositionXML(d.SN, d.DeviceID, interval))
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
	if channel, ok := d.channels.Get(c.DeviceID); ok {
		channel.DeviceChannel = c
	} else {
		channel = &Channel{
			Device:        d,
			Logger:        d.Logger.With("channel", c.DeviceID),
			DeviceChannel: c,
		}
		d.channels.Set(channel)
	}
}

func (d *Device) GetID() string {
	return d.DeviceID
}

func (d *Device) GetSdpIP() string {
	return d.SdpIP
}

func (d *Device) GetIP() string {
	return d.IP
}

func (d *Device) GetStreamMode() string {
	return d.StreamMode
}

func (d *Device) Send(req *sip.Request) (*sip.Response, error) {
	return d.send(req)
}

func (d *Device) CreateSSRC(serial string) uint16 {
	// 使用简单的 hash 函数将设备 ID 转换为 uint16
	var hash uint16
	for i := 0; i < len(d.DeviceID); i++ {
		hash = hash*31 + uint16(d.DeviceID[i])
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
