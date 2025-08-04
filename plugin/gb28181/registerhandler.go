package plugin_gb28181pro

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	myip "github.com/husanpao/ip"
	"github.com/icholy/digest"
	"github.com/rs/zerolog"
	"gorm.io/gorm"
	"m7s.live/v5"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/pkg/util"
	mrtp "m7s.live/v5/plugin/rtp/pkg"
)

type DeviceRegisterQueueTask struct {
	task.Work
	deviceId string
}

func (queueTask *DeviceRegisterQueueTask) GetKey() string {
	return queueTask.deviceId
}

type registerHandlerTask struct {
	task.Task
	gb  *GB28181Plugin
	req *sip.Request
	tx  sip.ServerTransaction
}

// getDevicePassword 获取设备密码
func (task *registerHandlerTask) getDevicePassword(device *Device) string {
	if device != nil && device.Password != "" {
		return device.Password
	}
	return task.gb.Password
}

func (task *registerHandlerTask) Run() (err error) {
	var password string
	var device *Device
	var recover = false
	from := task.req.From()
	if from == nil || from.Address.User == "" {
		task.gb.Error("OnRegister", "error", "no user")
		return
	}
	isUnregister := false
	deviceid := from.Address.User

	if existingDevice, exists := task.gb.devices.Get(deviceid); exists && existingDevice != nil {
		device = existingDevice
		recover = true
	} else {
		// 尝试从数据库加载设备信息
		device = &Device{DeviceId: deviceid}
		if task.gb.DB != nil {
			if err := task.gb.DB.First(device, Device{DeviceId: deviceid}).Error; err != nil {
				if !errors.Is(err, gorm.ErrRecordNotFound) {
					task.gb.Error("OnRegister", "error", err)
				}
			}
		}
	}

	// 获取设备密码
	password = task.getDevicePassword(device)

	exp := task.req.GetHeader("Expires")
	if exp == nil {
		task.gb.Error("OnRegister", "error", "no expires")
		return
	}
	expSec, err := strconv.ParseInt(exp.Value(), 10, 32)
	if err != nil {
		task.gb.Error("OnRegister", "error", err.Error())
		return
	}
	if expSec == 0 {
		isUnregister = true
	}

	// 需要密码认证的情况
	if password != "" {
		h := task.req.GetHeader("Authorization")
		if h == nil {
			// 生成认证挑战
			nonce := fmt.Sprintf("%d", time.Now().UnixMicro())
			chal := digest.Challenge{
				Realm:     task.gb.Realm,
				Nonce:     nonce,
				Opaque:    "monibuca",
				Algorithm: "MD5",
				QOP:       []string{"auth"},
			}

			res := sip.NewResponseFromRequest(task.req, sip.StatusUnauthorized, "Unauthorized", nil)
			res.AppendHeader(sip.NewHeader("WWW-Authenticate", chal.String()))
			task.gb.Debug("sending auth challenge", "nonce", nonce, "realm", task.gb.Realm)

			if err = task.tx.Respond(res); err != nil {
				task.gb.Error("respond Unauthorized", "error", err.Error())
			}
			return
		}

		// 解析认证信息
		cred, err := digest.ParseCredentials(h.Value())
		if err != nil {
			task.gb.Error("parsing credentials failed", "error", err.Error())
			if err = task.tx.Respond(sip.NewResponseFromRequest(task.req, sip.StatusUnauthorized, "Bad credentials", nil)); err != nil {
				task.gb.Error("respond Bad credentials", "error", err.Error())
			}
			return err
		}

		task.gb.Debug("received auth info",
			"username", cred.Username,
			"realm", cred.Realm,
			"nonce", cred.Nonce,
			"uri", cred.URI,
			"qop", cred.QOP,
			"nc", cred.Nc,
			"cnonce", cred.Cnonce,
			"response", cred.Response)

		// 使用设备ID作为用户名
		if cred.Username != deviceid {
			task.gb.Error("username mismatch", "expected", deviceid, "got", cred.Username)
			if err = task.tx.Respond(sip.NewResponseFromRequest(task.req, sip.StatusForbidden, "Invalid username", nil)); err != nil {
				task.gb.Error("respond Invalid username", "error", err.Error())
			}
			return err
		}

		// 计算期望的响应
		opts := digest.Options{
			Method:   "REGISTER",
			URI:      cred.URI,
			Username: deviceid,
			Password: password,
			Cnonce:   cred.Cnonce,
			Count:    int(cred.Nc),
		}

		digCred, err := digest.Digest(&digest.Challenge{
			Realm:     cred.Realm,
			Nonce:     cred.Nonce,
			Opaque:    cred.Opaque,
			Algorithm: cred.Algorithm,
			QOP:       []string{cred.QOP},
		}, opts)

		if err != nil {
			task.gb.Error("calculating digest failed", "error", err.Error())
			if err = task.tx.Respond(sip.NewResponseFromRequest(task.req, sip.StatusUnauthorized, "Bad credentials", nil)); err != nil {
				task.gb.Error("respond Bad credentials", "error", err.Error())
			}
			return err
		}

		task.gb.Debug("calculated response info",
			"username", opts.Username,
			"uri", opts.URI,
			"qop", cred.QOP,
			"nc", cred.Nc,
			"cnonce", opts.Cnonce,
			"count", opts.Count,
			"response", digCred.Response)

		// 比对响应
		if cred.Response != digCred.Response {
			task.gb.Error("response mismatch",
				"expected", digCred.Response,
				"got", cred.Response,
				"method", opts.Method,
				"uri", opts.URI,
				"username", opts.Username)
			if err = task.tx.Respond(sip.NewResponseFromRequest(task.req, sip.StatusUnauthorized, "Invalid credentials", nil)); err != nil {
				task.gb.Error("respond Invalid credentials", "error", err.Error())
			}
			return err
		}

		task.gb.Debug("auth successful", "username", deviceid)
	}
	response := sip.NewResponseFromRequest(task.req, sip.StatusOK, "OK", nil)
	response.AppendHeader(sip.NewHeader("Expires", fmt.Sprintf("%d", expSec)))
	response.AppendHeader(sip.NewHeader("Date", time.Now().Local().Format(util.LocalTimeFormat)))
	response.AppendHeader(sip.NewHeader("Server", "M7S/"+m7s.Version))
	response.AppendHeader(sip.NewHeader("Allow", "INVITE,ACK,CANCEL,BYE,NOTIFY,OPTIONS,PRACK,UPDATE,REFER"))
	//hostname, portStr, _ := net.SplitHostPort(req.Source())
	//port, _ := strconv.Atoi(portStr)
	//response.AppendHeader(&sip.ContactHeader{
	//	Address: sip.Uri{
	//		User: deviceid,
	//		Host: hostname,
	//		Port: port,
	//	},
	//})
	if err = task.tx.Respond(response); err != nil {
		task.gb.Error("respond OK", "error", err.Error())
	}
	if isUnregister { //取消绑定操作
		if d, ok := task.gb.devices.Get(deviceid); ok {
			d.Online = false
			d.Status = DeviceOfflineStatus
			d.channels.Range(func(channel *Channel) bool {
				channel.Status = "OFF"
				return true
			})
			//d.Stop(errors.New("unregister"))
		}
	} else {
		if recover {
			task.gb.Info("into recoverdevice", "deviceId", device.DeviceId)
			device.Status = DeviceOnlineStatus
			task.RecoverDevice(device, task.req)
		} else {
			var newDevice *Device
			if device == nil {
				newDevice = &Device{DeviceId: deviceid}
			} else {
				newDevice = device
			}
			task.gb.Info("into StoreDevice", "deviceId", from)
			task.StoreDevice(deviceid, task.req, newDevice)
		}
	}
	task.gb.Info("registerHandlerTask start end", "deviceid", deviceid, "expires", expSec, "isUnregister", isUnregister)
	return nil
}

func (task *registerHandlerTask) RecoverDevice(d *Device, req *sip.Request) {
	from := req.From()
	source := req.Source()
	desc := req.Destination()
	myIP, myPortStr, _ := net.SplitHostPort(desc)
	sourceIP, sourcePortStr, _ := net.SplitHostPort(source)
	sourcePort, _ := strconv.Atoi(sourcePortStr)
	myPort, _ := strconv.Atoi(myPortStr)

	// 如果设备IP是内网IP，则使用内网IP
	myIPParse := net.ParseIP(myIP)
	sourceIPParse := net.ParseIP(sourceIP)

	// 优先使用内网IP
	myLanIP := myip.InternalIPv4()
	myWanIP := myip.ExternalIPv4()

	task.gb.Info("Start RecoverDevice", "source", source, "desc", desc, "myLanIP", myLanIP, "myWanIP", myWanIP, "deviceid", d.DeviceId)

	// 处理目标地址和源地址的IP映射关系
	if sourceIPParse != nil { // 源IP有效时才进行处理
		if myIPParse == nil { // 目标地址是域名
			if sourceIPParse.IsPrivate() { // 源IP是内网IP
				myWanIP = myLanIP // 使用内网IP作为外网IP
			}
		} else { // 目标地址是IP
			if sourceIPParse.IsPrivate() { // 源IP是内网IP
				myLanIP, myWanIP = myIP, myIP // 使用目标IP作为内外网IP
			}
		}
	}

	if task.gb.MediaIP != "" {
		myWanIP = task.gb.MediaIP
	}
	if task.gb.SipIP != "" {
		myLanIP = task.gb.SipIP
	}
	// 设置 Recipient
	d.Recipient = sip.Uri{
		Host: sourceIP,
		Port: sourcePort,
		User: from.Address.User,
	}
	// 设置 contactHDR
	d.contactHDR = sip.ContactHeader{
		Address: sip.Uri{
			User: task.gb.Serial,
			Host: myIP,
			Port: myPort,
		},
	}

	d.SipIp = myLanIP
	d.StartTime = time.Now()
	d.IP = sourceIP
	d.Port = sourcePort
	d.HostAddress = d.IP + ":" + sourcePortStr
	d.Status = DeviceOnlineStatus
	d.UpdateTime = time.Now()
	d.KeepaliveTime = time.Now()
	d.RegisterTime = time.Now()
	d.Online = true
	d.client, _ = sipgo.NewClient(task.gb.ua, sipgo.WithClientLogger(zerolog.New(os.Stdout)), sipgo.WithClientHostname(d.SipIp))
	d.channels.L = new(sync.RWMutex)
	d.catalogReqs.L = new(sync.RWMutex)
	d.plugin = task.gb
	d.plugin.Info("RecoverDevice", "source", source, "desc", desc, "device.SipIp", myLanIP, "device.WanIP", myWanIP, "recipient", req.Recipient, "myPort", myPort, "deviceid", d.DeviceId)

	if task.gb.DB != nil {
		//var existing Device
		//if err := gb.DB.First(&existing, Device{DeviceId: d.DeviceId}).Error; err == nil {
		//	d.ID = existing.ID // 保持原有的自增ID
		//	gb.Info("RecoverDevice", "type", "更新设备", "deviceId", d.DeviceId)
		//} else {
		//	gb.Info("RecoverDevice", "type", "新增设备", "deviceId", d.DeviceId)
		//}
		task.gb.DB.Save(d)
	}
	return
}

func (task *registerHandlerTask) StoreDevice(deviceid string, req *sip.Request, d *Device) {
	task.gb.Debug("device info", "deviceid", deviceid, "via", req.Via(), "source", req.Source())
	source := req.Source()
	sourceIP, sourcePortStr, _ := net.SplitHostPort(source)
	sourcePort, _ := strconv.Atoi(sourcePortStr)
	desc := req.Destination()
	myIP, myPortStr, _ := net.SplitHostPort(desc)
	myPort, _ := strconv.Atoi(myPortStr)

	exp := req.GetHeader("Expires")
	if exp == nil {
		task.gb.Error("OnRegister", "error", "no expires")
		return
	}
	expSec, err := strconv.ParseInt(exp.Value(), 10, 32)
	if err != nil {
		task.gb.Error("OnRegister", "error", err.Error())
		return
	}

	// 检查myPort是否在sipPorts中，如果不在则使用sipPorts[0]
	if len(task.gb.sipPorts) > 0 {
		portFound := false
		for _, port := range task.gb.sipPorts {
			if port == myPort {
				portFound = true
				break
			}
		}
		if !portFound {
			myPort = task.gb.sipPorts[0]
			task.gb.Debug("StoreDevice", "使用默认端口替换", myPort)
		}
	}

	// 如果设备IP是内网IP，则使用内网IP
	myIPParse := net.ParseIP(myIP)
	sourceIPParse := net.ParseIP(sourceIP)

	// 优先使用内网IP
	myLanIP := myip.InternalIPv4()
	myWanIP := myip.ExternalIPv4()

	task.gb.Info("Start StoreDevice", "source", source, "desc", desc, "myLanIP", myLanIP, "myWanIP", myWanIP)

	// 处理目标地址和源地址的IP映射关系
	if sourceIPParse != nil { // 源IP有效时才进行处理
		if myIPParse == nil { // 目标地址是域名
			if sourceIPParse.IsPrivate() { // 源IP是内网IP
				myWanIP = myLanIP // 使用内网IP作为外网IP
			}
		} else { // 目标地址是IP
			if sourceIPParse.IsPrivate() { // 源IP是内网IP
				myLanIP, myWanIP = myIP, myIP // 使用目标IP作为内外网IP
			}
		}
	}

	if task.gb.MediaIP != "" {
		myWanIP = task.gb.MediaIP
	}
	if task.gb.SipIP != "" {
		myLanIP = task.gb.SipIP
	}

	now := time.Now()
	if d.CreateTime.IsZero() {
		d.CreateTime = now
	}
	d.UpdateTime = now
	d.RegisterTime = now
	d.KeepaliveTime = now
	d.Status = DeviceOnlineStatus
	d.Online = true
	d.StreamMode = mrtp.StreamModeTCPPassive // 默认TCP-PASSIVE传输
	d.Charset = "GB2312"                     // 默认GB2312字符集
	d.GeoCoordSys = "WGS84"                  // 默认WGS84坐标系
	d.Transport = req.Transport()            // 传输协议
	d.IP = sourceIP
	d.Port = sourcePort
	d.HostAddress = sourceIP + ":" + sourcePortStr
	d.SipIp = myLanIP
	d.MediaIp = myWanIP
	d.Expires = int(expSec)
	d.eventChan = make(chan any, 10)
	d.Recipient = sip.Uri{
		Host: sourceIP,
		Port: sourcePort,
		User: deviceid,
	}
	d.contactHDR = sip.ContactHeader{
		Address: sip.Uri{
			User: task.gb.Serial,
			Host: myWanIP,
			Port: myPort,
		},
	}
	d.fromHDR = sip.FromHeader{
		Address: sip.Uri{
			User: task.gb.Serial,
			Host: myWanIP,
			Port: myPort,
		},
		Params: sip.NewParams(),
	}
	d.plugin = task.gb
	d.LocalPort = myPort

	d.Logger = task.gb.Logger.With("deviceid", deviceid)
	d.fromHDR.Params.Add("tag", sip.GenerateTagN(16))
	d.client, _ = sipgo.NewClient(task.gb.ua, sipgo.WithClientLogger(zerolog.New(os.Stdout)), sipgo.WithClientHostname(d.SipIp))
	d.channels.L = new(sync.RWMutex)
	d.catalogReqs.L = new(sync.RWMutex)
	d.Info("StoreDevice", "source", source, "desc", desc, "device.SipIp", myLanIP, "device.WanIP", myWanIP, "req.Recipient", req.Recipient, "myPort", myPort, "d.Recipient", d.Recipient)

	// 使用简单的 hash 函数将设备 ID 转换为 uint32
	var hash uint32
	for i := 0; i < len(d.DeviceId); i++ {
		ch := d.DeviceId[i]
		hash = hash*31 + uint32(ch)
	}
	d.Task.ID = hash

	d.channels.OnAdd(func(c *Channel) {
		if absDevice, ok := task.gb.Server.PullProxies.Find(func(absDevice m7s.IPullProxy) bool {
			conf := absDevice.GetConfig()
			return conf.Type == "gb28181" && conf.URL == fmt.Sprintf("%s/%s", d.DeviceId, c.ChannelId)
		}); ok {
			c.PullProxyTask = absDevice.(*PullProxy)
			absDevice.ChangeStatus(m7s.PullProxyStatusOnline)
		}
	})
	task.gb.devices.AddTask(d).WaitStarted()

	if task.gb.DB != nil {
		//var existing Device
		//if err := task.gb.DB.First(&existing, Device{DeviceId: d.DeviceId}).Error; err == nil {
		//	d.ID = existing.ID // 保持原有的自增ID
		//	task.gb.DB.Save(d).Omit("create_time")
		//	task.gb.Info("StoreDevice", "type", "更新设备", "deviceId", d.DeviceId)
		//} else {
		task.gb.DB.Save(d)
		task.gb.Info("StoreDevice", "type", "新增设备", "deviceId", d.DeviceId)
		//}
	}
	return
}
