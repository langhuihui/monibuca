package plugin_gb28181pro

import (
	"net/http"
	"strings"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"m7s.live/v5"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/pkg/util"
	gb28181 "m7s.live/v5/plugin/gb28181pro/pkg"
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
	ID                    int64                   `gorm:"primaryKey;autoIncrement"` // 数据库自增长ID
	DeviceID              string                  // 设备国标编号
	Name                  string                  // 设备名
	Manufacturer          string                  // 生产厂商
	Model                 string                  // 型号
	Owner                 string                  // 所有者
	Firmware              string                  // 固件版本
	Transport             string                  // 传输协议（UDP/TCP）
	StreamMode            string                  // 数据流传输模式（UDP:udp传输/TCP-ACTIVE：tcp主动模式/TCP-PASSIVE：tcp被动模式）
	IP                    string                  // wan地址_ip
	Port                  int                     // wan地址_port
	HostAddress           string                  // wan地址
	Online                bool                    // 是否在线，true为在线，false为离线
	RegisterTime          time.Time               // 注册时间
	KeepaliveTime         time.Time               // 心跳时间
	KeepaliveInterval     int                     `gorm:"default:60"` // 心跳间隔
	ChannelCount          int                     // 通道个数
	Expires               int                     // 注册有效期
	CreateTime            time.Time               // 创建时间
	UpdateTime            time.Time               // 更新时间
	MediaServerID         string                  // 设备使用的媒体id, 默认为null
	Charset               string                  // 字符集, 支持 UTF-8 与 GB2312
	SubscribeCatalog      int                     // 目录订阅周期，0为不订阅
	SubscribePosition     int                     // 移动设备位置订阅周期，0为不订阅
	PositionInterval      int                     // 移动设备位置信息上报时间间隔,单位:秒,默认值5
	SubscribeAlarm        int                     // 报警订阅周期，0为不订阅
	SSRCCheck             bool                    // 是否开启ssrc校验，默认关闭，开启可以防止串流
	GeoCoordSys           string                  // 地理坐标系， 目前支持 WGS84,GCJ02
	Password              string                  // 密码
	SdpIP                 string                  // 收流IP
	LocalIP               string                  // SIP交互IP（设备访问平台的IP）
	AsMessageChannel      bool                    // 是否作为消息通道
	BroadcastPushAfterAck bool                    // 控制语音对讲流程，释放收到ACK后发流
	Channels              []gb28181.DeviceChannel `gorm:"foreignKey:DeviceDBID;references:ID"` // 设备通道列表

	// 保留原有字段
	Status              DeviceStatus
	SN                  int
	Recipient           sip.Uri                           `gorm:"-:all"`
	channels            util.Collection[string, *Channel] `gorm:"-:all"`
	mediaIp             string
	GpsTime             time.Time // gps时间
	Longitude, Latitude string    // 经度,纬度
	eventChan           chan any
	client              *sipgo.Client
	dialogClient        *sipgo.DialogClient
	contactHDR          sip.ContactHeader
	fromHDR             sip.FromHeader
	toHDR               sip.ToHeader
	plugin              *GB28181ProPlugin
	abc                 *sip.ClientTransaction
}

func (d *Device) TableName() string {
	return "device_gb28181pro"
}

func (d *Device) GetKey() string {
	return d.DeviceID
}

func (d *Device) onMessage(req *sip.Request, tx sip.ServerTransaction, msg *gb28181.Message) (err error) {
	var body []byte
	if d.Status == DeviceRecoverStatus {
		d.Status = DeviceOnlineStatus
	}
	d.Debug("OnMessage", "cmdType", msg.CmdType, "body", string(req.Body()))
	switch msg.CmdType {
	case "Keepalive":
		d.KeepaliveInterval = int(time.Since(d.KeepaliveTime).Seconds())
		d.KeepaliveTime = time.Now()
		if d.plugin.DB != nil {
			d.plugin.DB.Save(d)
		}
	case "Catalog":
		d.eventChan <- msg.DeviceList
		// 更新设备信息到数据库
		if d.plugin.DB != nil {
			// 更新通道信息
			for _, c := range msg.DeviceList {
				// 设置关联的设备数据库ID
				c.DeviceDBID = d.ID
				// 先查询是否存在
				var existing gb28181.DeviceChannel
				if err := d.plugin.DB.Where("device_id = ?", c.DeviceID).First(&existing).Error; err == nil {
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
			d.ChannelCount = len(msg.DeviceList)
			d.UpdateTime = time.Now()
			if err := d.plugin.DB.Save(d).Error; err != nil {
				d.Error("save device failed", "error", err)
			}
		}
	case "RecordInfo":
		if channel, ok := d.channels.Get(msg.DeviceID); ok {
			if req, ok := channel.RecordReqs.Get(msg.SN); ok {
				req.Response = msg.RecordList
				req.Resolve()
			}
		}
	case "DeviceInfo":
		// 主设备信息
		d.Name = msg.DeviceName
		d.Manufacturer = msg.Manufacturer
		d.Model = msg.Model
		// 更新设备信息到数据库
		if d.plugin.DB != nil {
			d.UpdateTime = time.Now()
			d.plugin.DB.Save(d)
		}
	case "Alarm":
		d.Status = DeviceAlarmedStatus
		body = []byte(gb28181.BuildAlarmResponseXML(d.DeviceID))
	case "Broadcast":
		d.Info("broadcast message", "body", req.Body())
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
	return d.client.Do(d, req)
}

func (d *Device) Go() (err error) {
	var response *sip.Response
	response, err = d.catalog()
	if err != nil {
		d.Error("catalog", "err", err)
	} else {
		d.Debug("catalog", "response", response.String())
	}
	response, err = d.queryDeviceInfo()
	if err != nil {
		d.Error("deviceInfo", "err", err)
	} else {
		d.Debug("deviceInfo", "response", response.String())
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
			if time.Since(d.KeepaliveTime) > time.Second*3600 {
				d.Error("keepalive timeout", "keepaliveTime", d.KeepaliveTime)
				return
			}
			response, err = d.catalog()
			if err != nil {
				d.Error("catalog", "err", err)
			} else {
				d.Debug("catalog", "response", response.String())
			}
		case event := <-d.eventChan:
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

func (d *Device) CreateRequest(Method sip.RequestMethod) *sip.Request {
	req := sip.NewRequest(Method, d.Recipient)
	req.AppendHeader(&d.fromHDR)
	contentType := sip.ContentTypeHeader("Application/MANSCDP+xml")
	req.AppendHeader(sip.NewHeader("User-Agent", "M7S/"+m7s.Version))
	req.AppendHeader(&contentType)
	req.AppendHeader(&d.contactHDR)
	return req
}

func (d *Device) catalog() (*sip.Response, error) {
	request := d.CreateRequest(sip.MESSAGE)
	//d.subscriber.Timeout = time.Now().Add(time.Second * time.Duration(expires))
	request.AppendHeader(sip.NewHeader("Expires", "3600"))
	request.SetBody(gb28181.BuildCatalogXML(d.SN, d.DeviceID))
	return d.send(request)
}

func (d *Device) subscribeCatalog() (*sip.Response, error) {
	request := d.CreateRequest(sip.SUBSCRIBE)
	request.AppendHeader(sip.NewHeader("Expires", "3600"))
	request.SetBody(gb28181.BuildCatalogXML(d.SN, d.DeviceID))
	return d.send(request)
}

func (d *Device) queryDeviceInfo() (*sip.Response, error) {
	request := d.CreateRequest(sip.MESSAGE)
	request.SetBody(gb28181.BuildDeviceInfoXML(d.SN, d.DeviceID))
	return d.send(request)
}

func (d *Device) subscribePosition(interval int) (*sip.Response, error) {
	request := d.CreateRequest(sip.SUBSCRIBE)
	request.AppendHeader(sip.NewHeader("Expires", "3600"))
	request.SetBody(gb28181.BuildDevicePositionXML(d.SN, d.DeviceID, interval))
	return d.send(request)
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

func (d *Device) CreateDialogSession(req *sip.Request) (*sipgo.DialogClientSession, error) {
	return d.dialogClient.Invite(d.plugin, d.Recipient, req.Body(), req.GetHeader("Content-Type"), req.GetHeader("Subject"), &d.fromHDR, req.GetHeader("Allow"))
}

func (d *Device) CreateSSRC(serial string) uint16 {
	// 使用简单的 hash 函数将设备 ID 转换为 uint16
	var hash uint16
	for i := 0; i < len(d.DeviceID); i++ {
		hash = hash*31 + uint16(d.DeviceID[i])
	}
	return hash
}
