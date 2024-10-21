package plugin_gb28181

import (
	"net/http"
	"strings"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"m7s.live/v5"
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
	task.Task           `gorm:"-:all"`
	ID                  string `gorm:"primaryKey"`
	Name                string
	Manufacturer        string
	Model               string
	Owner               string
	UpdateTime          time.Time
	LastKeepaliveAt     time.Time
	Status              DeviceStatus
	SN                  int
	Recipient           sip.Uri `gorm:"-:all"`
	Transport           string
	channels            util.Collection[string, *Channel]
	mediaIp             string
	GpsTime             time.Time //gps时间
	Longitude, Latitude string    //经度,纬度
	eventChan           chan any
	client              *sipgo.Client
	dialogClient        *sipgo.DialogClient
	contactHDR          sip.ContactHeader
	fromHDR             sip.FromHeader
	plugin              *GB28181Plugin
}

func (d *Device) TableName() string {
	return "device_gb28181"
}

func (d *Device) GetKey() string {
	return d.ID
}

func (d *Device) onMessage(req *sip.Request, tx sip.ServerTransaction, msg *gb28181.Message) (err error) {
	var body []byte
	if d.Status == DeviceRecoverStatus {
		d.Status = DeviceOnlineStatus
	}
	d.Debug("OnMessage", "cmdType", msg.CmdType, "body", string(req.Body()))
	switch msg.CmdType {
	case "Keepalive":
		d.LastKeepaliveAt = time.Now()
	case "Catalog":
		d.eventChan <- msg.DeviceList
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
	case "Alarm":
		d.Status = DeviceAlarmedStatus
		body = []byte(gb28181.BuildAlarmResponseXML(d.ID))
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
			if time.Since(d.LastKeepaliveAt) > time.Second*3600 {
				d.Error("keepalive timeout", "lastKeepaliveAt", d.LastKeepaliveAt)
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
			case []gb28181.ChannelInfo:
				for _, c := range v {
					//当父设备非空且存在时、父设备节点增加通道
					if c.ParentID != "" {
						path := strings.Split(c.ParentID, "/")
						parentId := path[len(path)-1]
						//如果父ID并非本身所属设备，一般情况下这是因为下级设备上传了目录信息，该信息通常不需要处理。
						// 暂时不考虑级联目录的实现
						if d.ID != parentId {
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

func (d *Device) createRequest(Method sip.RequestMethod) (req *sip.Request) {
	req = sip.NewRequest(Method, d.Recipient)
	req.AppendHeader(&d.fromHDR)
	contentType := sip.ContentTypeHeader("Application/MANSCDP+xml")
	req.AppendHeader(sip.NewHeader("User-Agent", "M7S/"+m7s.Version))
	req.AppendHeader(&contentType)
	req.AppendHeader(&d.contactHDR)
	return
}

func (d *Device) catalog() (*sip.Response, error) {
	request := d.createRequest(sip.MESSAGE)
	//d.subscriber.Timeout = time.Now().Add(time.Second * time.Duration(expires))
	request.AppendHeader(sip.NewHeader("Expires", "3600"))
	request.SetBody(gb28181.BuildCatalogXML(d.SN, d.ID))
	return d.send(request)
}

func (d *Device) subscribeCatalog() (*sip.Response, error) {
	request := d.createRequest(sip.SUBSCRIBE)
	request.AppendHeader(sip.NewHeader("Expires", "3600"))
	request.SetBody(gb28181.BuildCatalogXML(d.SN, d.ID))
	return d.send(request)
}

func (d *Device) queryDeviceInfo() (*sip.Response, error) {
	request := d.createRequest(sip.MESSAGE)
	request.SetBody(gb28181.BuildDeviceInfoXML(d.SN, d.ID))
	return d.send(request)
}

func (d *Device) subscribePosition(interval int) (*sip.Response, error) {
	request := d.createRequest(sip.SUBSCRIBE)
	request.AppendHeader(sip.NewHeader("Expires", "3600"))
	request.SetBody(gb28181.BuildDevicePositionXML(d.SN, d.ID, interval))
	return d.send(request)
}

func (d *Device) addOrUpdateChannel(c gb28181.ChannelInfo) {
	if channel, ok := d.channels.Get(c.DeviceID); ok {
		channel.ChannelInfo = c
	} else {
		channel = &Channel{
			Device:      d,
			Logger:      d.Logger.With("channel", c.DeviceID),
			ChannelInfo: c,
		}
		d.channels.Set(channel)
	}
}
