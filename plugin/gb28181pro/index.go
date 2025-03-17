package plugin_gb28181pro

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	myip "github.com/husanpao/ip"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/icholy/digest"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	m7s "m7s.live/v5"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/pkg/util"
	"m7s.live/v5/plugin/gb28181pro/pb"
	gb28181 "m7s.live/v5/plugin/gb28181pro/pkg"
)

type SipConfig struct {
	ListenAddr    []string
	ListenTLSAddr []string
	CertFile      string `desc:"证书文件"`
	KeyFile       string `desc:"私钥文件"`
}

type PositionConfig struct {
	Expires  time.Duration `default:"3600s" desc:"订阅周期"` //订阅周期
	Interval time.Duration `default:"6s" desc:"订阅间隔"`    //订阅间隔
}

type GB28181ProPlugin struct {
	pb.UnimplementedApiServer
	m7s.Plugin
	AutoInvite     bool   `default:"true" desc:"自动邀请"`
	Serial         string `default:"34020000002000000001" desc:"sip 服务 id"` //sip 服务器 id, 默认 34020000002000000001
	Realm          string `default:"3402000000" desc:"sip 服务域"`            //sip 服务器域，默认 3402000000
	Username       string
	Password       string
	Sip            SipConfig
	MediaPort      util.Range[uint16] `default:"10001-20000" desc:"媒体端口范围"` //媒体端口范围
	Position       PositionConfig
	Parent         string `desc:"父级设备"`
	ua             *sipgo.UserAgent
	server         *sipgo.Server
	devices        util.Collection[string, *Device]
	dialogs        util.Collection[uint32, *Dialog]
	forwardDialogs util.Collection[uint32, *ForwardDialog]
	platforms      util.Collection[uint32, *Platform]
	tcpPorts       chan uint16
}

var _ = m7s.InstallPlugin[GB28181ProPlugin](pb.RegisterApiHandler, &pb.Api_ServiceDesc, func(conf config.Pull) m7s.IPuller {
	if util.Exist(conf.URL) {
		return &gb28181.DumpPuller{}
	}
	return new(Dialog)
})

func init() {
	sip.SIPDebug = true
}
func (gb *GB28181ProPlugin) OnInit() (err error) {
	logger := zerolog.New(os.Stdout)
	gb.ua, err = sipgo.NewUA(sipgo.WithUserAgent("M7S/" + m7s.Version)) // Build user agent
	// Creating client handle for ua
	if len(gb.Sip.ListenAddr) > 0 {
		gb.server, _ = sipgo.NewServer(gb.ua, sipgo.WithServerLogger(logger)) // Creating server handle for ua
		gb.server.OnMessage(gb.OnMessage)
		gb.server.OnRegister(gb.OnRegister)
		gb.server.OnBye(gb.OnBye)
		gb.devices.L = new(sync.RWMutex)
		gb.server.OnInvite(gb.OnInvite)
		gb.server.OnAck(gb.OnAck)

		if gb.MediaPort.Valid() {
			gb.SetDescription("tcp", fmt.Sprintf("%d-%d", gb.MediaPort[0], gb.MediaPort[1]))
			gb.tcpPorts = make(chan uint16, gb.MediaPort.Size())
			for i := range gb.MediaPort.Size() {
				gb.tcpPorts <- gb.MediaPort[0] + i
			}
		} else {
			gb.SetDescription("tcp", fmt.Sprintf("%d", gb.MediaPort[0]))
			tcpConfig := &gb.GetCommonConf().TCP
			tcpConfig.ListenAddr = fmt.Sprintf(":%d", gb.MediaPort[0])
		}
		for _, addr := range gb.Sip.ListenAddr {
			netWork, addr, _ := strings.Cut(addr, ":")
			gb.SetDescription(netWork, strings.TrimPrefix(addr, ":"))
			go gb.server.ListenAndServe(gb, netWork, addr)
		}
		if len(gb.Sip.ListenTLSAddr) > 0 {
			if tslConfig, err := config.GetTLSConfig(gb.Sip.CertFile, gb.Sip.KeyFile); err == nil {
				for _, addr := range gb.Sip.ListenTLSAddr {
					netWork, addr, _ := strings.Cut(addr, ":")
					gb.SetDescription(netWork+"TLS", strings.TrimPrefix(addr, ":"))
					go gb.server.ListenAndServeTLS(gb, netWork, addr, tslConfig)
				}
			} else {
				return err
			}
		}
		if gb.DB != nil {
			gb.DB.AutoMigrate(&Device{}, &gb28181.DeviceChannel{}, &gb28181.PlatformModel{}, &gb28181.DeviceAlarm{}, &gb28181.PlatformChannel{})
			// 检查设备过期状态
			if err := gb.checkDeviceExpire(); err != nil {
				gb.Error("检查设备过期状态失败", "error", err)
			}

			// 初始化数据库中的设备
			if err := gb.checkDevices(); err != nil {
				gb.Error("检查设备有效性失败", "error", err)
			}

			// 检查并初始化平台
			gb.checkPlatform()
		}
	}
	if gb.Parent != "" {
		var client Client
		client.conf = gb
		client.SetRetry(-1, time.Second*5)
		gb.AddTask(&client)
	}
	return
}

// InitDevicesFromDB 从数据库中加载并初始化在线设备
func (gb *GB28181ProPlugin) checkDevices() error {
	// 检查数据库是否已初始化
	if gb.DB == nil {
		gb.Warn("InitDevicesFromDB", "warning", "数据库未初始化")
		return nil
	}

	// 查询所有有效设备：在线且注册未过期
	var devices []*Device
	now := time.Now()

	// 先查询所有在线设备
	if err := gb.DB.Where("online = ?", true).Find(&devices).Error; err != nil {
		gb.Error("InitDevicesFromDB", "error", err.Error())
		return err
	}

	// 过滤出未过期的设备
	validDevices := make([]*Device, 0, len(devices))
	for _, d := range devices {
		expireTime := d.RegisterTime.Add(time.Duration(d.Expires) * time.Second)
		if !now.After(expireTime) {
			validDevices = append(validDevices, d)
		} else {
			gb.Debug("InitDevicesFromDB", "跳过过期设备", d.DeviceID, "注册时间", d.RegisterTime, "过期时间", expireTime)
		}
	}

	gb.Info("InitDevicesFromDB", "找到有效设备数量", len(validDevices), "总在线设备数量", len(devices))

	// 初始化每个设备
	for _, device := range validDevices {
		d := device // 创建副本以避免循环变量问题

		// 设置设备基本属性
		d.Status = DeviceRecoverStatus

		// 设置事件通道
		d.eventChan = make(chan any, 10)

		// 设置Logger
		d.Logger = gb.With("deviceid", d.DeviceID)

		// 初始化通道集合
		d.channels.L = new(sync.RWMutex)

		// 设置plugin引用
		d.plugin = gb

		// 配置SIP相关参数
		host := d.LocalIP
		if host == "" {
			host = myip.InternalIPv4()
			d.LocalIP = host
			d.mediaIp = host
		}

		// 获取公网或内网IP配置
		if !net.ParseIP(d.IP).IsPrivate() {
			host = gb.GetPublicIP(host)
		}

		// 设置联系人头信息
		d.contactHDR = sip.ContactHeader{
			Address: sip.Uri{
				User: gb.Serial,
				Host: host,
				Port: d.LocalPort,
			},
		}

		// 设置来源头信息
		d.fromHDR = sip.FromHeader{
			Address: sip.Uri{
				User: gb.Serial,
				Host: gb.Realm,
			},
			Params: sip.NewParams(),
		}
		d.fromHDR.Params.Add("tag", sip.GenerateTagN(16))

		// 设置接收者
		d.Recipient = sip.Uri{
			Host: d.IP,
			Port: d.Port,
			User: d.DeviceID,
		}

		// 创建SIP客户端
		d.client, _ = sipgo.NewClient(gb.ua, sipgo.WithClientLogger(zerolog.New(os.Stdout)), sipgo.WithClientHostname(host), sipgo.WithClientPort(d.LocalPort))
		d.dialogClient = sipgo.NewDialogClientCache(d.client, d.contactHDR)

		// 设置设备ID的hash值作为任务ID
		var hash uint32
		for i := 0; i < len(d.DeviceID); i++ {
			ch := d.DeviceID[i]
			hash = hash*31 + uint32(ch)
		}
		d.Task.ID = hash

		// 设置启动和销毁回调
		d.OnStart(func() {
			gb.devices.Add(d)
			d.channels.OnAdd(func(c *Channel) {
				if absDevice, ok := gb.Server.PullProxies.Find(func(absDevice *m7s.PullProxy) bool {
					return absDevice.Type == "gb28181" && absDevice.URL == fmt.Sprintf("%s/%s", d.DeviceID, c.DeviceID)
				}); ok {
					c.AbstractDevice = absDevice
					absDevice.Handler = c
					absDevice.ChangeStatus(m7s.PullProxyStatusOnline)
				}
				if gb.AutoInvite {
					gb.Pull(fmt.Sprintf("%s/%s", d.DeviceID, c.DeviceID), config.Pull{
						MaxRetry: 0,
						URL:      fmt.Sprintf("%s/%s", d.DeviceID, c.DeviceID),
					}, nil)
				}
			})
		})
		d.OnDispose(func() {
			d.Status = DeviceOfflineStatus
			if gb.devices.RemoveByKey(d.DeviceID) {
				for c := range d.channels.Range {
					if c.AbstractDevice != nil {
						c.AbstractDevice.ChangeStatus(m7s.PullProxyStatusOffline)
					}
				}
			}
		})

		// 添加设备任务
		gb.AddTask(d)
		expireTime := d.RegisterTime.Add(time.Duration(d.Expires) * time.Second)
		gb.Info("InitDevicesFromDB", "已初始化设备", d.DeviceID, "注册时间", d.RegisterTime, "过期时间", expireTime)

		// 加载设备的通道
		var channels []gb28181.DeviceChannel
		if err := gb.DB.Where(&gb28181.DeviceChannel{DeviceDBID: d.ID}).Find(&channels).Error; err != nil {
			gb.Error("InitDevicesFromDB", "加载通道失败", d.DeviceID, "error", err.Error())
		} else {
			// 初始化设备通道
			for _, channel := range channels {
				d.addOrUpdateChannel(channel)
			}
			gb.Info("InitDevicesFromDB", "已加载通道数量", len(channels), "设备ID", d.DeviceID)
		}
	}

	return nil
}

// checkPlatform 从数据库中查找启用状态的平台，初始化它们，并进行注册和定时任务设置
func (gb *GB28181ProPlugin) checkPlatform() {
	// 检查数据库是否初始化
	if gb.DB == nil {
		gb.Error("数据库未初始化，无法检查平台")
		return
	}

	// 查询所有启用状态的平台
	var platformModels []*gb28181.PlatformModel
	enableTrue := true
	platformModel := gb28181.PlatformModel{Enable: &enableTrue}
	if err := gb.DB.Where(&platformModel).Find(&platformModels).Error; err != nil {
		gb.Error("查询平台失败", "error", err.Error())
		return
	}

	gb.Info("找到启用状态的平台", "count", len(platformModels))

	// 遍历所有平台进行初始化和注册
	for _, platformModel := range platformModels {
		// 创建Platform实例
		platform := NewPlatform(platformModel, gb)

		_, err := platform.Unregister()
		if err != nil {
			gb.Error("unregister err ", err)
		} else {
			// 添加到任务系统
			gb.AddTask(platform)
			gb.Info("平台初始化完成", "ID", platformModel.ID, "Name", platformModel.Name)
		}
	}
}

func (gb *GB28181ProPlugin) checkDeviceExpire() (err error) {
	// 从数据库中查询所有设备
	var devices []*Device
	if err := gb.DB.Find(&devices).Error; err != nil {
		gb.Error("查询设备列表失败", "error", err)
		return err
	}

	now := time.Now()
	for _, device := range devices {
		// 检查设备是否过期
		expireTime := device.RegisterTime.Add(time.Duration(device.Expires) * time.Second)
		if now.After(expireTime) {
			// 设备已过期，更新设备状态
			device.Online = false
			device.Status = "OFFLINE"
			if err := gb.DB.Model(&Device{}).Where(&Device{ID: device.ID}).Updates(device).Error; err != nil {
				gb.Error("更新设备状态失败", "error", err, "deviceId", device.DeviceID)
				continue
			}

			// 更新关联的通道状态
			var channels []gb28181.DeviceChannel
			if err := gb.DB.Where(&gb28181.DeviceChannel{DeviceDBID: device.ID}).Find(&channels).Error; err != nil {
				gb.Error("查询通道列表失败", "error", err, "deviceId", device.DeviceID)
				continue
			}

			// 更新所有关联通道的状态
			for _, channel := range channels {
				channel.Status = "OFF"
				if err := gb.DB.Model(&gb28181.DeviceChannel{}).Where(&gb28181.DeviceChannel{ID: channel.ID}).Updates(channel).Error; err != nil {
					gb.Error("更新通道状态失败", "error", err, "channelId", channel.DeviceID)
				}
			}

			gb.Info("设备已过期", "deviceId", device.DeviceID, "registerTime", device.RegisterTime, "expireTime", expireTime)
		}
	}

	return nil
}

func (p *GB28181ProPlugin) OnPullProxyAdd(pullProxy *m7s.PullProxy) any {
	deviceID, channelID, _ := strings.Cut(pullProxy.URL, "/")
	if d, ok := p.devices.Get(deviceID); ok {
		if channel, ok := d.channels.Get(channelID); ok {
			channel.AbstractDevice = pullProxy
			return channel
		}
	}
	return nil
}

func (gb *GB28181ProPlugin) RegisterHandler() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"/api/ps/replay/{streamPath...}": gb.api_ps_replay,
	}
}

func (gb *GB28181ProPlugin) OnRegister(req *sip.Request, tx sip.ServerTransaction) {
	from := req.From()
	if from == nil || from.Address.User == "" {
		gb.Error("OnRegister", "error", "no user")
		return
	}
	isUnregister := false
	deviceid := from.Address.User
	exp := req.GetHeader("Expires")
	if exp == nil {
		gb.Error("OnRegister", "error", "no expires")
		return
	}
	expSec, err := strconv.ParseInt(exp.Value(), 10, 32)
	if err != nil {
		gb.Error("OnRegister", "error", err.Error())
		return
	}
	if expSec == 0 {
		isUnregister = true
	}

	// 不需要密码情况
	if gb.Username != "" && gb.Password != "" {
		h := req.GetHeader("Authorization")
		var chal digest.Challenge
		var cred *digest.Credentials
		var digCred *digest.Credentials
		if h == nil {
			chal = digest.Challenge{
				Realm:     gb.Realm,
				Nonce:     fmt.Sprintf("%d", time.Now().UnixMicro()),
				Opaque:    "monibuca",
				Algorithm: "MD5",
			}

			res := sip.NewResponseFromRequest(req, sip.StatusUnauthorized, "Unathorized", nil)
			res.AppendHeader(sip.NewHeader("WWW-Authenticate", chal.String()))

			if err = tx.Respond(res); err != nil {
				gb.Error("respond Unathorized", "error", err.Error())
			}
			return
		}

		cred, err = digest.ParseCredentials(h.Value())
		if err != nil {
			log.Error().Err(err).Msg("parsing creds failed")
			if err = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusUnauthorized, "Bad credentials", nil)); err != nil {
				gb.Error("respond Bad credentials", "error", err.Error())
			}
			return
		}

		// Check registry
		if cred.Username != gb.Username {
			if err = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusNotFound, "Bad authorization header", nil)); err != nil {
				gb.Error("respond Bad authorization header", "error", err.Error())
			}
			return
		}

		// Make digest and compare response
		digCred, err = digest.Digest(&chal, digest.Options{
			Method:   "REGISTER",
			URI:      cred.URI,
			Username: gb.Username,
			Password: gb.Password,
		})

		if err != nil {
			gb.Error("Calc digest failed")
			if err = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusUnauthorized, "Bad credentials", nil)); err != nil {
				gb.Error("respond Bad credentials", "error", err.Error())
			}
			return
		}

		if cred.Response != digCred.Response {
			if err = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusUnauthorized, "Unathorized", nil)); err != nil {
				gb.Error("respond Unathorized", "error", err.Error())
			}
			return
		}
	}
	response := sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil)
	response.AppendHeader(sip.NewHeader("Expires", fmt.Sprintf("%d", expSec)))
	response.AppendHeader(sip.NewHeader("Date", time.Now().Local().Format(util.LocalTimeFormat)))
	response.AppendHeader(sip.NewHeader("Server", "M7S/"+m7s.Version))
	response.AppendHeader(sip.NewHeader("Allow", "INVITE,ACK,CANCEL,BYE,NOTIFY,OPTIONS,PRACK,UPDATE,REFER"))
	hostname, portStr, _ := net.SplitHostPort(req.Source())
	port, _ := strconv.Atoi(portStr)
	response.AppendHeader(&sip.ContactHeader{
		Address: sip.Uri{
			User: deviceid,
			Host: hostname,
			Port: port,
		},
	})
	if err = tx.Respond(response); err != nil {
		gb.Error("respond OK", "error", err.Error())
	}
	if isUnregister { //取消绑定操作
		if d, ok := gb.devices.Get(deviceid); ok {
			d.Online = false
			d.Status = DeviceOfflineStatus
			if gb.DB != nil {
				// 更新设备状态
				var dbDevice Device
				if err := gb.DB.First(&dbDevice, Device{DeviceID: deviceid}).Error; err == nil {
					d.ID = dbDevice.ID
				}
				gb.DB.Save(d)

				// 批量更新关联的通道状态
				if err := gb.DB.Model(&gb28181.DeviceChannel{}).Where(&gb28181.DeviceChannel{DeviceDBID: d.ID}).Update("status", "OFF").Error; err != nil {
					gb.Error("更新通道状态失败", "error", err, "deviceId", d.DeviceID)
				} else {
					gb.Info("更新通道状态成功", "deviceId", d.DeviceID)
				}
			}
			d.Stop(errors.New("unregister"))
		}
	} else {
		if d, ok := gb.devices.Get(deviceid); ok {
			d.Online = true
			gb.RecoverDevice(d, req)
		} else {
			gb.StoreDevice(deviceid, req)
		}
	}
}

func (gb *GB28181ProPlugin) OnMessage(req *sip.Request, tx sip.ServerTransaction) {
	// 解析消息内容
	temp := &gb28181.Message{}
	err := gb28181.DecodeXML(temp, req.Body())
	if err != nil {
		gb.Error("OnMessage", "error", err.Error())
		response := sip.NewResponseFromRequest(req, sip.StatusBadRequest, "Bad Request", nil)
		if err := tx.Respond(response); err != nil {
			gb.Error("respond BadRequest", "error", err.Error())
		}
		return
	}
	from := req.From()
	if from == nil || from.Address.User == "" {
		gb.Error("OnMessage", "error", "no user")
		return
	}
	id := from.Address.User

	// 检查消息来源
	var d *Device
	var p *gb28181.PlatformModel

	// 先从设备缓存中获取
	d, _ = gb.devices.Get(id)

	// 检查是否是平台
	if gb.DB != nil {
		var platform gb28181.PlatformModel
		if err := gb.DB.First(&platform, gb28181.PlatformModel{ServerGBID: id}).Error; err == nil {
			p = &platform
		}
	}

	// 如果设备和平台都存在，通过源地址判断真实来源
	if d != nil && p != nil {
		source := req.Source()
		if d.HostAddress == source {
			// 如果源地址匹配设备地址，则确认是设备消息
			p = nil
		} else {
			// 否则认为是平台消息
			d = nil
		}
	}

	// 如果既不是设备也不是平台，返回404
	if d == nil && p == nil {
		gb.Error("OnMessage", "error", "device/platform not found", "id", id)
		response := sip.NewResponseFromRequest(req, sip.StatusNotFound, "Not Found", nil)
		if err := tx.Respond(response); err != nil {
			gb.Error("respond NotFound", "error", err.Error())
		}
		return
	}

	// 根据来源调用不同的处理方法
	if d != nil {
		d.UpdateTime = time.Now()
		if err = d.onMessage(req, tx, temp); err != nil {
			gb.Error("onMessage", "error", err.Error(), "type", "device")
		}
	} else {
		var platform *Platform
		if platformtmp, ok := gb.platforms.Get(p.ID); !ok {
			// 创建 Platform 实例
			platform = NewPlatform(p, gb)
		} else {
			platform = platformtmp
		}
		if err = platform.OnMessage(req, tx, temp); err != nil {
			gb.Error("onMessage", "error", err.Error(), "type", "platform")
		}
	}
}

func (gb *GB28181ProPlugin) RecoverDevice(d *Device, req *sip.Request) {
	from := req.From()
	source := req.Source()
	desc := req.Destination()
	servIp, sPortStr, _ := net.SplitHostPort(desc)
	deviceIP, _, _ := net.SplitHostPort(source)
	hostname, portStr, _ := net.SplitHostPort(source)
	port, _ := strconv.Atoi(portStr)
	serverPort, _ := strconv.Atoi(sPortStr)

	// 优先使用内网IP
	host := myip.InternalIPv4()
	// 如果设备IP是内网IP，则使用内网IP
	deviceIPParsed := net.ParseIP(deviceIP)
	if deviceIPParsed != nil {
		if deviceIPParsed.IsPrivate() {
			// 设备是内网IP，优先使用内网IP
			servIPParsed := net.ParseIP(servIp)
			if servIPParsed != nil && servIPParsed.IsPrivate() {
				// 如果服务器配置的也是内网IP，则使用配置的IP
				host = servIp
			}
		} else {
			// 设备是公网IP，使用公网IP
			host = gb.GetPublicIP(servIp)
		}
	}

	// 设置 Recipient
	d.Recipient = sip.Uri{
		Host: hostname,
		Port: port,
		User: from.Address.User,
	}
	// 设置 contactHDR
	d.contactHDR = sip.ContactHeader{
		Address: sip.Uri{
			User: gb.Serial,
			Host: host,
			Port: serverPort,
		},
	}

	d.StartTime = time.Now()
	d.Status = DeviceRecoverStatus
	d.UpdateTime = time.Now()
	d.Online = true

	if gb.DB != nil {
		var existing Device
		if err := gb.DB.First(&existing, Device{DeviceID: d.DeviceID}).Error; err == nil {
			d.ID = existing.ID // 保持原有的自增ID
			gb.Info("RecoverDevice", "type", "更新设备", "deviceId", d.DeviceID)
		} else {
			gb.Info("RecoverDevice", "type", "新增设备", "deviceId", d.DeviceID)
		}
		gb.DB.Save(d)
	}
	return
}

func (gb *GB28181ProPlugin) StoreDevice(deviceid string, req *sip.Request) (d *Device) {
	source := req.Source()
	desc := req.Destination()
	servIp, sPortStr, _ := net.SplitHostPort(desc)

	exp := req.GetHeader("Expires")
	if exp == nil {
		gb.Error("OnRegister", "error", "no expires")
		return
	}
	expSec, err := strconv.ParseInt(exp.Value(), 10, 32)
	if err != nil {
		gb.Error("OnRegister", "error", err.Error())
		return
	}
	// 优先使用内网IP
	host := myip.InternalIPv4()
	// 如果设备IP是内网IP，则使用内网IP
	deviceIPParsed := net.ParseIP(servIp)
	if deviceIPParsed != nil {
		if !deviceIPParsed.IsPrivate() { //公网情况就去获取本地网卡的公网IP
			// 设备是公网IP，使用公网IP
			host = gb.GetPublicIP(host)
		}
	}

	hostname, portStr, _ := net.SplitHostPort(source)
	port, _ := strconv.Atoi(portStr)
	serverPort, _ := strconv.Atoi(sPortStr)

	now := time.Now()
	d = &Device{
		DeviceID:      deviceid,
		CreateTime:    now,
		UpdateTime:    now,
		RegisterTime:  now,
		KeepaliveTime: now,
		Status:        DeviceRegisterStatus,
		Online:        true,
		StreamMode:    "UDP",           // 默认UDP传输
		Charset:       "GB2312",        // 默认GB2312字符集
		GeoCoordSys:   "WGS84",         // 默认WGS84坐标系
		Transport:     req.Transport(), // 传输协议
		IP:            hostname,
		Port:          port,
		HostAddress:   hostname + ":" + portStr,
		LocalIP:       host,
		mediaIp:       host,
		Expires:       int(expSec),
		eventChan:     make(chan any, 10),
		Recipient: sip.Uri{
			Host: hostname,
			Port: port,
			User: deviceid,
		},
		contactHDR: sip.ContactHeader{
			Address: sip.Uri{
				User: gb.Serial,
				Host: host,
				Port: serverPort,
			},
		},
		fromHDR: sip.FromHeader{
			Address: sip.Uri{
				User: gb.Serial,
				Host: gb.Realm,
			},
			Params: sip.NewParams(),
		},
		plugin:    gb,
		LocalPort: serverPort,
	}

	d.Logger = gb.With("deviceid", deviceid)
	d.fromHDR.Params.Add("tag", sip.GenerateTagN(16))
	d.client, _ = sipgo.NewClient(gb.ua, sipgo.WithClientLogger(zerolog.New(os.Stdout)), sipgo.WithClientHostname(host), sipgo.WithClientPort(serverPort))
	gb.Info("get serverport is ", serverPort)
	d.dialogClient = sipgo.NewDialogClientCache(d.client, d.contactHDR)
	d.channels.L = new(sync.RWMutex)
	d.Info("StoreDevice", "source", source, "desc", desc, "servIp", servIp, "publicIP", host, "recipient", req.Recipient)

	// 使用简单的 hash 函数将设备 ID 转换为 uint32
	var hash uint32
	for i := 0; i < len(d.DeviceID); i++ {
		ch := d.DeviceID[i]
		hash = hash*31 + uint32(ch)
	}
	d.Task.ID = hash

	d.OnStart(func() {
		gb.devices.Add(d)
		d.channels.OnAdd(func(c *Channel) {
			if absDevice, ok := gb.Server.PullProxies.Find(func(absDevice *m7s.PullProxy) bool {
				return absDevice.Type == "gb28181" && absDevice.URL == fmt.Sprintf("%s/%s", d.DeviceID, c.DeviceID)
			}); ok {
				c.AbstractDevice = absDevice
				absDevice.Handler = c
				absDevice.ChangeStatus(m7s.PullProxyStatusOnline)
			}
			if gb.AutoInvite {
				gb.Pull(fmt.Sprintf("%s/%s", d.DeviceID, c.DeviceID), config.Pull{
					MaxRetry: 0,
					URL:      fmt.Sprintf("%s/%s", d.DeviceID, c.DeviceID),
				}, nil)
			}
		})
	})
	d.OnDispose(func() {
		d.Status = DeviceOfflineStatus
		if gb.devices.RemoveByKey(d.DeviceID) {
			for c := range d.channels.Range {
				if c.AbstractDevice != nil {
					c.AbstractDevice.ChangeStatus(m7s.PullProxyStatusOffline)
				}
			}
		}
	})
	gb.AddTask(d)

	if gb.DB != nil {
		var existing Device
		if err := gb.DB.First(&existing, Device{DeviceID: d.DeviceID}).Error; err == nil {
			d.ID = existing.ID // 保持原有的自增ID
			gb.Info("StoreDevice", "type", "更新设备", "deviceId", d.DeviceID)
		} else {
			gb.Info("StoreDevice", "type", "新增设备", "deviceId", d.DeviceID)
		}
		gb.DB.Save(d)
	}
	return
}

func (gb *GB28181ProPlugin) Pull(streamPath string, conf config.Pull, pubConf *config.Publish) {
	if util.Exist(conf.URL) {
		var puller gb28181.DumpPuller
		puller.GetPullJob().Init(&puller, &gb.Plugin, streamPath, conf, pubConf)
		return
	}
	dialog := Dialog{
		gb: gb,
	}
	if conf.Args != nil {
		if conf.Args.Get(util.StartKey) != "" || conf.Args.Get(util.EndKey) != "" {
			dialog.start = conf.Args.Get(util.StartKey)
			dialog.end = conf.Args.Get(util.EndKey)
		}
	}
	dialog.GetPullJob().Init(&dialog, &gb.Plugin, streamPath, conf, pubConf)
}

func (gb *GB28181ProPlugin) GetPullableList() []string {
	return slices.Collect(func(yield func(string) bool) {
		for d := range gb.devices.Range {
			for c := range d.channels.Range {
				yield(fmt.Sprintf("%s/%s", d.DeviceID, c.DeviceID))
			}
		}
	})
}

//type PSServer struct {
//	task.Task
//	*rtp2.TCP
//	theDialog *Dialog
//	gb        *GB28181ProPlugin
//}
//
//func (gb *GB28181ProPlugin) OnTCPConnect(conn *net.TCPConn) task.ITask {
//	ret := &PSServer{gb: gb, TCP: (*rtp2.TCP)(conn)}
//	ret.Task.Logger = gb.With("remote", conn.RemoteAddr().String())
//	return ret
//}
//
//func (task *PSServer) Dispose() {
//	_ = task.TCP.Close()
//	if task.theDialog != nil {
//		close(task.theDialog.FeedChan)
//	}
//}
//
//func (task *PSServer) Go() (err error) {
//	return task.Read(func(data util.Buffer) (err error) {
//		if task.theDialog != nil {
//			return task.theDialog.ReadRTP(data)
//		}
//		var rtpPacket rtp.Packet
//		if err = rtpPacket.Unmarshal(data); err != nil {
//			task.Error("decode rtp", "err", err)
//		}
//		ssrc := rtpPacket.SSRC
//		if dialog, ok := task.gb.dialogs.Get(ssrc); ok {
//			task.theDialog = dialog
//			return dialog.ReadRTP(data)
//		}
//		task.Warn("dialog not found", "ssrc", ssrc)
//		return
//	})
//}

func (gb *GB28181ProPlugin) OnBye(req *sip.Request, tx sip.ServerTransaction) {
	if dialog, ok := gb.dialogs.Find(func(d *Dialog) bool {
		return d.GetCallID() == req.CallID().Value()
	}); ok {
		gb.Warn("OnBye", "dialog", dialog.GetCallID())
		dialog.Stop(task.ErrTaskComplete)
	}
	if forwardDialog, ok := gb.forwardDialogs.Find(func(d *ForwardDialog) bool {
		return d.platformCallId == req.CallID().Value()
	}); ok {
		gb.Warn("OnBye", "dialog", forwardDialog.GetCallID())
		forwardDialog.Stop(task.ErrTaskComplete)
	}
}

func (gb *GB28181ProPlugin) GetSerial() string {
	return gb.Serial
}

func (gb *GB28181ProPlugin) OnInvite(req *sip.Request, tx sip.ServerTransaction) {
	// 解析 INVITE 请求
	inviteInfo, err := gb28181.DecodeSDP(req)
	if err != nil {
		gb.Error("OnInvite", "error", "decode sdp failed", "err", err.Error())
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusBadRequest, err.Error(), nil))
		return
	}

	// 首先从数据库中查询平台
	var platform *Platform
	var platformModel = &gb28181.PlatformModel{}
	if gb.DB != nil {
		// 使用requesterId查询平台，类似于Java代码中的queryPlatformByServerGBId
		result := gb.DB.Where("server_gb_id = ?", inviteInfo.RequesterId).First(&platformModel)
		if result.Error == nil {
			// 数据库中找到平台，根据平台ID从运行时实例中查找
			if platformTmp, platformFound := gb.platforms.Get(platformModel.ID); !platformFound {
				gb.Error("OnInvite", "error", "platform found in DB but not in runtime", "platformId", inviteInfo.RequesterId)
				_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusNotFound, "Platform Not Found In Runtime", nil))
				return
			} else {
				platform = platformTmp
			}

			gb.Info("OnInvite", "action", "platform found", "platformId", inviteInfo.RequesterId, "platformName", platform.PlatformModel.Name)

			// 查询平台下是否有该通道
			// 根据Java代码中的 channelService.queryOneWithPlatform 逻辑
			var commonGBChannels []gb28181.CommonGBChannel

			// 使用类似Java代码中queryOneWithPlatform的SQL查询
			// 进行JOIN查询，查找平台ID和通道ID匹配的记录
			query := `
				SELECT 
					wdc.id as gb_id,
					wdc.device_db_id as gb_device_db_id,
					wdc.stream_push_id,
					wdc.stream_proxy_id,
					wdc.create_time,
					wdc.update_time,
					COALESCE(NULLIF(wpgc.custom_device_id, ''), NULLIF(wdc.gb_device_id, ''), NULLIF(wdc.device_id, '')) as gb_device_id,
					COALESCE(NULLIF(wpgc.custom_name, ''), NULLIF(wdc.gb_name, ''), NULLIF(wdc.name, '')) as gb_name,
					COALESCE(NULLIF(wpgc.custom_manufacturer, ''), NULLIF(wdc.gb_manufacturer, ''), NULLIF(wdc.manufacturer, '')) as gb_manufacturer,
					COALESCE(NULLIF(wpgc.custom_model, ''), NULLIF(wdc.gb_model, ''), NULLIF(wdc.model, '')) as gb_model,
					COALESCE(NULLIF(wpgc.custom_owner, ''), NULLIF(wdc.gb_owner, ''), NULLIF(wdc.owner, '')) as gb_owner,
					COALESCE(NULLIF(wpgc.custom_civil_code, ''), NULLIF(wdc.gb_civil_code, ''), NULLIF(wdc.civil_code, '')) as gb_civil_code,
					COALESCE(NULLIF(wpgc.custom_block, ''), NULLIF(wdc.gb_block, ''), NULLIF(wdc.block, '')) as gb_block,
					COALESCE(NULLIF(wpgc.custom_address, ''), NULLIF(wdc.gb_address, ''), NULLIF(wdc.address, '')) as gb_address,
					COALESCE(NULLIF(wpgc.custom_parental, ''), NULLIF(wdc.gb_parental, ''), NULLIF(wdc.parental, '')) as gb_parental,
					COALESCE(NULLIF(wpgc.custom_parent_id, ''), NULLIF(wdc.gb_parent_id, ''), NULLIF(wdc.parent_id, '')) as gb_parent_id,
					COALESCE(NULLIF(wpgc.custom_safety_way, ''), NULLIF(wdc.gb_safety_way, ''), NULLIF(wdc.safety_way, '')) as gb_safety_way,
					COALESCE(NULLIF(wpgc.custom_register_way, ''), NULLIF(wdc.gb_register_way, ''), NULLIF(wdc.register_way, '')) as gb_register_way,
					COALESCE(NULLIF(wpgc.custom_cert_num, ''), NULLIF(wdc.gb_cert_num, ''), NULLIF(wdc.cert_num, '')) as gb_cert_num,
					COALESCE(NULLIF(wpgc.custom_certifiable, ''), NULLIF(wdc.gb_certifiable, ''), NULLIF(wdc.certifiable, '')) as gb_certifiable,
					COALESCE(NULLIF(wpgc.custom_err_code, ''), NULLIF(wdc.gb_err_code, ''), NULLIF(wdc.err_code, '')) as gb_err_code,
					COALESCE(NULLIF(wpgc.custom_end_time, ''), NULLIF(wdc.gb_end_time, ''), NULLIF(wdc.end_time, '')) as gb_end_time,
					COALESCE(NULLIF(wpgc.custom_secrecy, ''), NULLIF(wdc.gb_secrecy, ''), NULLIF(wdc.secrecy, '')) as gb_secrecy,
					COALESCE(NULLIF(wpgc.custom_ip_address, ''), NULLIF(wdc.gb_ip_address, ''), NULLIF(wdc.ip_address, '')) as gb_ip_address,
					COALESCE(NULLIF(wpgc.custom_port, ''), NULLIF(wdc.gb_port, ''), NULLIF(wdc.port, '')) as gb_port,
					COALESCE(NULLIF(wpgc.custom_password, ''), NULLIF(wdc.gb_password, ''), NULLIF(wdc.password, '')) as gb_password,
					COALESCE(NULLIF(wpgc.custom_status, ''), NULLIF(wdc.gb_status, ''), NULLIF(wdc.status, '')) as gb_status,
					COALESCE(NULLIF(wpgc.custom_longitude, ''), NULLIF(wdc.gb_longitude, ''), NULLIF(wdc.longitude, '')) as gb_longitude,
					COALESCE(NULLIF(wpgc.custom_latitude, ''), NULLIF(wdc.gb_latitude, ''), NULLIF(wdc.latitude, '')) as gb_latitude,
					COALESCE(NULLIF(wpgc.custom_ptz_type, ''), NULLIF(wdc.gb_ptz_type, ''), NULLIF(wdc.ptz_type, '')) as gb_ptz_type,
					COALESCE(NULLIF(wpgc.custom_position_type, ''), NULLIF(wdc.gb_position_type, ''), NULLIF(wdc.position_type, '')) as gb_position_type,
					COALESCE(NULLIF(wpgc.custom_room_type, ''), NULLIF(wdc.gb_room_type, ''), NULLIF(wdc.room_type, '')) as gb_room_type,
					COALESCE(NULLIF(wpgc.custom_use_type, ''), NULLIF(wdc.gb_use_type, ''), NULLIF(wdc.use_type, '')) as gb_use_type,
					COALESCE(NULLIF(wpgc.custom_supply_light_type, ''), NULLIF(wdc.gb_supply_light_type, ''), NULLIF(wdc.supply_light_type, '')) as gb_supply_light_type,
					COALESCE(NULLIF(wpgc.custom_direction_type, ''), NULLIF(wdc.gb_direction_type, ''), NULLIF(wdc.direction_type, '')) as gb_direction_type,
					COALESCE(NULLIF(wpgc.custom_resolution, ''), NULLIF(wdc.gb_resolution, ''), NULLIF(wdc.resolution, '')) as gb_resolution,
					COALESCE(NULLIF(wpgc.custom_business_group_id, ''), NULLIF(wdc.gb_business_group_id, ''), NULLIF(wdc.business_group_id, '')) as gb_business_group_id,
					COALESCE(NULLIF(wpgc.custom_download_speed, ''), NULLIF(wdc.gb_download_speed, ''), NULLIF(wdc.download_speed, '')) as gb_download_speed,
					COALESCE(NULLIF(wpgc.custom_svc_space_support_mod, ''), NULLIF(wdc.gb_svc_space_support_mod, ''), NULLIF(wdc.svc_space_support_mod, '')) as gb_svc_space_support_mod,
					COALESCE(NULLIF(wpgc.custom_svc_time_support_mode, ''), NULLIF(wdc.gb_svc_time_support_mode, ''), NULLIF(wdc.svc_time_support_mode, '')) as gb_svc_time_support_mode
				FROM 
					channel_gb28181pro wdc
				LEFT JOIN 
					platform_channel_gb28181pro wpgc ON wdc.id = wpgc.device_channel_id
				WHERE 
					wpgc.platform_id = ? AND 
					COALESCE(NULLIF(wpgc.custom_device_id,''), NULLIF(wdc.gb_device_id,''), NULLIF(wdc.device_id,'')) = ?
				ORDER BY 
					wdc.id
			`

			// 执行查询
			channelResult := gb.DB.Raw(query, platform.PlatformModel.ID, inviteInfo.TargetChannelId).Scan(&commonGBChannels)
			if channelResult.Error != nil || len(commonGBChannels) == 0 {
				gb.Error("OnInvite", "error", "channel not found", "channelId", inviteInfo.TargetChannelId, "err", channelResult.Error)
				_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusNotFound, "Channel Not Found", nil))
				return
			}

			// 找到了通道
			channel := commonGBChannels[len(commonGBChannels)-1]
			gb.Info("OnInvite", "action", "channel found", "channelId", channel.GbDeviceID, "channelName", channel.GbName)

			var channelTmp *Channel
			if deviceFound, ok := gb.devices.Find(func(device *Device) bool {
				return device.ID == int64(channel.GbDeviceDbID)
			}); ok {
				if channelFound, ok := deviceFound.channels.Get(channel.GbDeviceID); ok {
					channelTmp = channelFound
				} else {
					gb.Error("OnInvite", "error", "channel not found memory,channel deviceid is ", channel.GbDeviceID)
					_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusBadRequest, "SSRC Not Found", nil))
					return
				}
			} else {
				gb.Error("OnInvite", "error", "device not found memory,device dbid is ", channel.GbDeviceDbID)
				_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusBadRequest, "SSRC Not Found", nil))
				return
			}

			// 通道存在，发送100 Trying响应
			tryingResp := sip.NewResponseFromRequest(req, sip.StatusTrying, "Trying", nil)
			if err := tx.Respond(tryingResp); err != nil {
				gb.Error("OnInvite", "error", "send trying response failed", "err", err.Error())
				return
			}

			// 检查SSRC
			if inviteInfo.SSRC == "" {
				gb.Error("OnInvite", "error", "ssrc not found in invite")
				_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusBadRequest, "SSRC Not Found", nil))
				return
			}

			// 获取媒体信息
			mediaPort := uint16(0)
			if gb.MediaPort.Valid() {
				select {
				case port := <-gb.tcpPorts:
					mediaPort = port
					gb.Debug("OnInvite", "action", "allocate port", "port", port)
				default:
					gb.Error("OnInvite", "error", "no available port")
					_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusServiceUnavailable, "No Available Port", nil))
					return
				}
			} else {
				mediaPort = gb.MediaPort[0]
				gb.Debug("OnInvite", "action", "use default port", "port", mediaPort)
			}

			// 构建SDP响应
			// 使用平台和通道的信息构建响应
			sdpIP := platform.PlatformModel.DeviceIP
			// 如果平台配置了SendStreamIP，则使用此IP
			if platform.PlatformModel.SendStreamIP != "" {
				sdpIP = platform.PlatformModel.SendStreamIP
			}

			// 构建SDP内容，参考Java代码createSendSdp方法
			content := []string{
				"v=0",
				fmt.Sprintf("o=%s 0 0 IN IP4 %s", channel.GbDeviceID, sdpIP),
				fmt.Sprintf("s=%s", inviteInfo.SessionName),
				fmt.Sprintf("c=IN IP4 %s", sdpIP),
			}

			// 处理播放时间
			if strings.EqualFold("Playback", inviteInfo.SessionName) && inviteInfo.StartTime > 0 && inviteInfo.StopTime > 0 {
				content = append(content, fmt.Sprintf("t=%d %d", inviteInfo.StartTime, inviteInfo.StopTime))
			} else {
				content = append(content, "t=0 0")
			}

			// 处理传输模式
			if inviteInfo.TCP {
				content = append(content, fmt.Sprintf("m=video %d TCP/RTP/AVP 96", mediaPort))
				if inviteInfo.TCPActive {
					content = append(content, "a=setup:passive")
				} else {
					content = append(content, "a=setup:active")
				}
				if inviteInfo.TCP {
					content = append(content, "a=connection:new")
				}
			} else {
				content = append(content, fmt.Sprintf("m=video %d RTP/AVP 96", mediaPort))
			}

			// 添加其他属性，参考Java代码
			content = append(content,
				"a=sendonly",
				"a=rtpmap:96 PS/90000",
				fmt.Sprintf("y=%s", inviteInfo.SSRC),
				"f=",
			)

			// 发送200 OK响应
			response := sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil)
			contentType := sip.ContentTypeHeader("application/sdp")
			response.AppendHeader(&contentType)
			response.SetBody([]byte(strings.Join(content, "\r\n") + "\r\n"))

			// 创建并保存SendRtpInfo，以供OnAck方法使用
			forwardDialog := &ForwardDialog{
				gb:             gb,
				platformIP:     inviteInfo.IP,
				platformPort:   inviteInfo.Port,
				platformSSRC:   inviteInfo.SSRC,
				TCP:            inviteInfo.TCP,
				TCPActive:      inviteInfo.TCPActive,
				platformCallId: req.CallID().Value(),
				start:          inviteInfo.StartTime,
				end:            inviteInfo.StopTime,
				channel:        channelTmp,
			}

			// 保存到集合中
			gb.forwardDialogs.Set(forwardDialog)
			gb.Info("OnInvite", "action", "sendRtpInfo created", "callId", req.CallID().Value())

			if err := tx.Respond(response); err != nil {
				gb.Error("OnInvite", "error", "send response failed", "err", err.Error())
				return
			}

			gb.Info("OnInvite", "action", "complete", "platformId", inviteInfo.RequesterId, "channelId", channel.GbDeviceID,
				"ip", inviteInfo.IP, "port", inviteInfo.Port, "tcp", inviteInfo.TCP, "tcpActive", inviteInfo.TCPActive)
			return
		} else {
			// 数据库中未找到平台，响应not found
			gb.Error("OnInvite", "error", "platform not found in database", "platformId", inviteInfo.RequesterId)
			_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusNotFound, "Platform Not Found", nil))
			return
		}
	} else {
		// 数据库未初始化，响应服务不可用
		gb.Error("OnInvite", "error", "database not initialized")
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusServiceUnavailable, "Database Not Initialized", nil))
		return
	}
}

func (gb *GB28181ProPlugin) OnAck(req *sip.Request, tx sip.ServerTransaction) {
	callID := req.CallID().Value()
	if callID == "" {
		gb.Error("OnAck", "error", "callid header not found")
		return
	}
	// 构建streamPath
	if forwardDialog, ok := gb.forwardDialogs.Find(func(dialog *ForwardDialog) bool {
		return dialog.platformCallId == callID
	}); ok {
		streamPath := fmt.Sprintf("%s/%s", forwardDialog.channel.Device.DeviceID, forwardDialog.channel.DeviceID)

		// 创建配置
		pullConf := config.Pull{
			URL: streamPath,
		}
		// 初始化拉流任务
		forwardDialog.GetPullJob().Init(forwardDialog, &gb.Plugin, streamPath, pullConf, nil)
	} else {
		gb.Error("OnAck", "error", "forwardDialog not found", "callID", callID)
		return
	}
}
