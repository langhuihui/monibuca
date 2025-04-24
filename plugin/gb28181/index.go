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
	m7s "m7s.live/v5"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/pkg/util"
	"m7s.live/v5/plugin/gb28181/pb"
	gb28181 "m7s.live/v5/plugin/gb28181/pkg"
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

type GB28181Plugin struct {
	pb.UnimplementedApiServer
	m7s.Plugin
	Serial         string `default:"34020000002000000001" desc:"sip 服务 id"` //sip 服务器 id, 默认 34020000002000000001
	Realm          string `default:"3402000000" desc:"sip 服务域"`             //sip 服务器域，默认 3402000000
	Password       string
	Sip            SipConfig
	MediaPort      util.Range[uint16] `default:"10001-20000" desc:"媒体端口范围"` //媒体端口范围
	Position       PositionConfig
	Parent         string `desc:"父级设备"`
	AutoMigrate    bool   `default:"true" desc:"自动迁移数据库结构并初始化根组织"`
	ua             *sipgo.UserAgent
	server         *sipgo.Server
	devices        util.Collection[string, *Device]
	dialogs        util.Collection[uint32, *Dialog]
	forwardDialogs util.Collection[uint32, *ForwardDialog]
	platforms      util.Collection[string, *Platform]
	tcpPorts       chan uint16
	sipPorts       []int
	SipIP          string `desc:"sip发送命令的IP，一般是本地IP，多网卡时需要配置正确的IP"`
	MediaIP        string `desc:"流媒体IP，用于接收流"`
}

var _ = m7s.InstallPlugin[GB28181Plugin](m7s.PluginMeta{
	RegisterGRPCHandler: pb.RegisterApiHandler,
	ServiceDesc:         &pb.Api_ServiceDesc,
	NewPuller: func(conf config.Pull) m7s.IPuller {
		if util.Exist(conf.URL) {
			return &gb28181.DumpPuller{}
		}
		return new(Dialog)
	},
	NewPullProxy: NewPullProxy,
})

func init() {
	sip.SIPDebug = true
}

// initDatabase 初始化数据库，进行所有表结构迁移和初始化操作
func (gb *GB28181Plugin) initDatabase() error {
	if gb.DB == nil {
		return errors.New("database not initialized")
	}

	// 根据配置决定是否执行自动迁移和初始化
	if gb.AutoMigrate {
		// 迁移设备、通道和平台相关表
		if err := gb.DB.AutoMigrate(
			&Device{},
			&gb28181.DeviceChannel{},
			&gb28181.PlatformModel{},
			&gb28181.DeviceAlarm{},
			&gb28181.PlatformChannel{},
			&gb28181.GroupsModel{},
			&gb28181.GroupsChannelModel{},
		); err != nil {
			return fmt.Errorf("auto migrate tables error: %v", err)
		}
		gb.Info("数据库表结构迁移成功")

		// 查询是否存在根组织
		var count int64
		if err := gb.DB.Model(&gb28181.GroupsModel{}).Where("pid = ? AND level = ?", 0, 0).Count(&count).Error; err != nil {
			return fmt.Errorf("查询根组织失败: %v", err)
		}

		// 如果不存在根组织，创建一个
		if count == 0 {
			rootGroup := gb28181.NewRootGroup()
			if err := gb.DB.Create(rootGroup).Error; err != nil {
				return fmt.Errorf("创建根组织失败: %v", err)
			}
			gb.Info("已创建根组织")
		} else {
			// 获取根组织信息
			root := &gb28181.GroupsModel{}
			if err := gb.DB.Where("pid = ? AND level = ?", 0, 0).First(root).Error; err != nil {
				gb.Warn("根组织已存在但获取详情失败: %v", err)
			} else {
				gb.Info("根组织已存在，ID:", root.ID)
			}
		}
	} else {
		gb.Info("自动迁移已禁用，跳过表结构迁移和根组织初始化")
	}

	return nil
}

func (gb *GB28181Plugin) OnInit() (err error) {
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
			if port, err := strconv.Atoi(strings.TrimPrefix(addr, ":")); err == nil {
				gb.sipPorts = append(gb.sipPorts, port)
			}
			go gb.server.ListenAndServe(gb, netWork, addr)
		}
		if len(gb.Sip.ListenTLSAddr) > 0 {
			if tslConfig, err := config.GetTLSConfig(gb.Sip.CertFile, gb.Sip.KeyFile); err == nil {
				for _, addr := range gb.Sip.ListenTLSAddr {
					netWork, addr, _ := strings.Cut(addr, ":")
					gb.SetDescription(netWork+"TLS", strings.TrimPrefix(addr, ":"))
					if port, err := strconv.Atoi(strings.TrimPrefix(addr, ":")); err == nil {
						gb.sipPorts = append(gb.sipPorts, port)
					}
					go gb.server.ListenAndServeTLS(gb, netWork, addr, tslConfig)
				}
			} else {
				return err
			}
		}
		if gb.DB != nil {
			err = gb.initDatabase()
			if err != nil {
				gb.Error("initDatabase error: %v", err)
			}

			// 检查设备过期状态
			if err := gb.checkDeviceExpire(); err != nil {
				gb.Error("检查设备过期状态失败", "error", err)
			}

			// 检查并初始化平台
			gb.checkPlatform()
		}
	} else {
		gb.Error("GB28181 init failed,please set Sip.ListenAddr in GB28181 configuration like this   \nsip:\n  listenaddr:\n    - udp::5060\n")
	}
	return
}

func (gb *GB28181Plugin) checkDeviceExpire() (err error) {
	// 从数据库中查询所有设备
	var devices []*Device
	if err := gb.DB.Find(&devices).Error; err != nil {
		gb.Error("查询设备列表失败", "error", err)
		return err
	}

	now := time.Now()
	for _, device := range devices {
		if device.Online {
			// 检查设备是否过期
			expireTime := device.RegisterTime.Add(time.Duration(device.Expires) * time.Second)
			isExpired := now.After(expireTime)

			// 设置设备基本属性
			device.Status = DeviceOfflineStatus
			if !isExpired {
				device.Status = DeviceOnlineStatus
			}
			device.Online = !isExpired

			// 设置事件通道
			device.eventChan = make(chan any, 10)

			// 设置Logger
			device.Logger = gb.With("deviceid", device.DeviceID)

			// 初始化通道集合
			device.channels.L = new(sync.RWMutex)

			// 设置plugin引用
			device.plugin = gb

			// 设置联系人头信息
			device.contactHDR = sip.ContactHeader{
				Address: sip.Uri{
					User: gb.Serial,
					Host: device.SipIP,
					Port: device.localPort,
				},
			}

			// 设置来源头信息
			device.fromHDR = sip.FromHeader{
				Address: sip.Uri{
					User: gb.Serial,
					Host: device.SipIP,
					Port: device.localPort,
				},
				Params: sip.NewParams(),
			}
			device.fromHDR.Params.Add("tag", sip.GenerateTagN(16))

			// 设置接收者
			device.Recipient = sip.Uri{
				Host: device.IP,
				Port: device.Port,
				User: device.DeviceID,
			}

			// 创建SIP客户端
			device.client, _ = sipgo.NewClient(gb.ua, sipgo.WithClientLogger(zerolog.New(os.Stdout)), sipgo.WithClientHostname(device.SipIP))
			device.Info("checkDeviceExpire", "d.SipIP", device.SipIP, "d.localPort", device.localPort, "d.contactHDR", device.contactHDR)

			// 设置设备ID的hash值作为任务ID
			var hash uint32
			for i := 0; i < len(device.DeviceID); i++ {
				ch := device.DeviceID[i]
				hash = hash*31 + uint32(ch)
			}
			device.Task.ID = hash
			// 设置启动和销毁回调
			device.OnStart(func() {
				gb.devices.Set(device)
			})
			device.channels.OnAdd(func(c *Channel) {
				if absDevice, ok := gb.Server.PullProxies.Find(func(absDevice m7s.IPullProxy) bool {
					conf := absDevice.GetConfig()
					return conf.Type == "gb28181" && conf.URL == fmt.Sprintf("%s/%s", device.DeviceID, c.ChannelID)
				}); ok {
					c.PullProxyTask = absDevice.(*PullProxy)
					absDevice.ChangeStatus(m7s.PullProxyStatusOnline)
				}
			})
			device.OnDispose(func() {
				device.Status = DeviceOfflineStatus
				if gb.devices.RemoveByKey(device.DeviceID) {
					for c := range device.channels.Range {
						if c.PullProxyTask != nil {
							c.PullProxyTask.ChangeStatus(m7s.PullProxyStatusOffline)
						}
					}
				}
			})

			// 加载设备的通道
			var channels []gb28181.DeviceChannel
			if err := gb.DB.Where(&gb28181.DeviceChannel{DeviceID: device.DeviceID}).Find(&channels).Error; err != nil {
				gb.Error("加载通道失败", "error", err, "deviceId", device.DeviceID)
				continue
			}

			// 更新设备状态到数据库
			if err := gb.DB.Model(&Device{}).Where(&Device{DeviceID: device.DeviceID}).Updates(map[string]interface{}{
				"online": device.Online,
				"status": device.Status,
			}).Error; err != nil {
				gb.Error("更新设备状态到数据库失败", "error", err, "deviceId", device.DeviceID)
			}

			// 初始化设备通道并更新到数据库
			for _, channel := range channels {
				if isExpired {
					channel.Status = "OFF"
				} else {
					channel.Status = "ON"
				}
				// 更新通道状态到数据库
				if err := gb.DB.Model(&gb28181.DeviceChannel{}).Where(&gb28181.DeviceChannel{ID: channel.ID}).Update("status", channel.Status).Error; err != nil {
					gb.Error("更新通道状态到数据库失败", "error", err, "channelId", channel.DeviceID)
				}
				device.addOrUpdateChannel(channel)
			}

			// 添加设备任务
			if !isExpired {
				gb.AddTask(device)
			} else {
				//gb.devices.Set(device)
				//_, err := device.queryDeviceInfo()
				//if err != nil {
				//	device.Error("queryDeviceInfo when checkDeviceExpire", "err", err)
				//}
			}

			if isExpired {
				gb.Info("设备已过期", "deviceId", device.DeviceID, "registerTime", device.RegisterTime, "expireTime", expireTime)
			} else {
				gb.Info("设备有效", "deviceId", device.DeviceID, "registerTime", device.RegisterTime, "expireTime", expireTime)
			}
		}
	}
	return nil
}

// checkPlatform 从数据库中查找启用状态的平台，初始化它们，并进行注册和定时任务设置
func (gb *GB28181Plugin) checkPlatform() {
	// 检查数据库是否初始化
	if gb.DB == nil {
		gb.Error("数据库未初始化，无法检查平台")
		return
	}

	// 查询所有启用状态的平台
	var platformModels []*gb28181.PlatformModel
	platformModel := gb28181.PlatformModel{Enable: true}
	if err := gb.DB.Where(&platformModel).Find(&platformModels).Error; err != nil {
		gb.Error("查询平台失败", "error", err.Error())
		return
	}

	gb.Info("找到启用状态的平台", "count", len(platformModels))

	// 遍历所有平台进行初始化和注册
	for _, platformModel := range platformModels {
		// 创建Platform实例
		platform := NewPlatform(platformModel, gb, true)

		//go platform.Unregister()
		//if err != nil {
		//	 gb.Error("unregister err ", err)
		//}
		// 添加到任务系统
		gb.AddTask(platform)
		gb.Info("平台初始化完成", "ID", platformModel.ServerGBID, "Name", platformModel.Name)
	}
}

func (gb *GB28181Plugin) RegisterHandler() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"/api/ps/replay/{streamPath...}": gb.api_ps_replay,
	}
}

func (gb *GB28181Plugin) OnRegister(req *sip.Request, tx sip.ServerTransaction) {
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

	// 需要密码认证的情况
	if gb.Password != "" {
		h := req.GetHeader("Authorization")
		if h == nil {
			// 生成认证挑战
			nonce := fmt.Sprintf("%d", time.Now().UnixMicro())
			chal := digest.Challenge{
				Realm:     gb.Realm,
				Nonce:     nonce,
				Opaque:    "monibuca",
				Algorithm: "MD5",
				QOP:       []string{"auth"},
			}

			res := sip.NewResponseFromRequest(req, sip.StatusUnauthorized, "Unauthorized", nil)
			res.AppendHeader(sip.NewHeader("WWW-Authenticate", chal.String()))
			gb.Debug("sending auth challenge", "nonce", nonce, "realm", gb.Realm)

			if err = tx.Respond(res); err != nil {
				gb.Error("respond Unauthorized", "error", err.Error())
			}
			return
		}

		// 解析认证信息
		cred, err := digest.ParseCredentials(h.Value())
		if err != nil {
			gb.Error("parsing credentials failed", "error", err.Error())
			if err = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusUnauthorized, "Bad credentials", nil)); err != nil {
				gb.Error("respond Bad credentials", "error", err.Error())
			}
			return
		}

		gb.Debug("received auth info",
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
			gb.Error("username mismatch", "expected", deviceid, "got", cred.Username)
			if err = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusForbidden, "Invalid username", nil)); err != nil {
				gb.Error("respond Invalid username", "error", err.Error())
			}
			return
		}

		// 计算期望的响应
		opts := digest.Options{
			Method:   "REGISTER",
			URI:      cred.URI,
			Username: deviceid,
			Password: gb.Password,
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
			gb.Error("calculating digest failed", "error", err.Error())
			if err = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusUnauthorized, "Bad credentials", nil)); err != nil {
				gb.Error("respond Bad credentials", "error", err.Error())
			}
			return
		}

		gb.Debug("calculated response info",
			"username", opts.Username,
			"uri", opts.URI,
			"qop", cred.QOP,
			"nc", cred.Nc,
			"cnonce", opts.Cnonce,
			"count", opts.Count,
			"response", digCred.Response)

		// 比对响应
		if cred.Response != digCred.Response {
			gb.Error("response mismatch",
				"expected", digCred.Response,
				"got", cred.Response,
				"method", opts.Method,
				"uri", opts.URI,
				"username", opts.Username)
			if err = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusUnauthorized, "Invalid credentials", nil)); err != nil {
				gb.Error("respond Invalid credentials", "error", err.Error())
			}
			return
		}

		gb.Debug("auth successful", "username", deviceid)
	}
	response := sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil)
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
				d.channels.Range(func(channel *Channel) bool {
					channel.Status = "OFF"
					return true
				})
			}
			d.Stop(errors.New("unregister"))
		}
	} else {
		if d, ok := gb.devices.Get(deviceid); ok && d.Online {
			gb.Info("into recoverdevice ,deviceid is ", d.DeviceID)
			d.Status = DeviceOnlineStatus
			gb.RecoverDevice(d, req)
		} else {
			gb.Info("into StoreDevice ,deviceid is ", from)
			gb.StoreDevice(deviceid, req)
		}
	}
}

func (gb *GB28181Plugin) OnMessage(req *sip.Request, tx sip.ServerTransaction) {
	// 解析消息内容
	temp := &gb28181.Message{}
	err := gb28181.DecodeXML(temp, req.Body())
	gb.Debug("onmessage debug, message is ", temp)
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
		if err := gb.DB.First(&platform, gb28181.PlatformModel{ServerGBID: id, Enable: true}).Error; err == nil {
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
		var response *sip.Response
		gb.Error("OnMessage", "error", "device/platform not found", "id", id)
		response = sip.NewResponseFromRequest(req, sip.StatusNotFound, "Not Found", nil)
		if err := tx.Respond(response); err != nil {
			gb.Error("respond NotFound", "error", err.Error())
		}
		gb.Debug("after on message respond")
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
		if platformtmp, ok := gb.platforms.Get(p.ServerGBID); !ok {
			// 创建 Platform 实例
			platform = NewPlatform(p, gb, false)
		} else {
			platform = platformtmp
		}
		if err = platform.OnMessage(req, tx, temp); err != nil {
			gb.Error("onMessage", "error", err.Error(), "type", "platform")
		}
	}
}

func (gb *GB28181Plugin) RecoverDevice(d *Device, req *sip.Request) {
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

	gb.Info("Start StoreDevice", "source", source, "desc", desc, "myLanIP", myLanIP, "myWanIP", myWanIP)

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

	if gb.MediaIP != "" {
		myWanIP = gb.MediaIP
	}
	if gb.SipIP != "" {
		myLanIP = gb.SipIP
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
			User: gb.Serial,
			Host: myIP,
			Port: myPort,
		},
	}

	d.SipIP = myLanIP
	d.StartTime = time.Now()
	d.IP = sourceIP
	d.Port = sourcePort
	d.HostAddress = d.IP + ":" + sourcePortStr
	d.Status = DeviceOnlineStatus
	d.UpdateTime = time.Now()
	d.RegisterTime = time.Now()
	d.Online = true
	d.client, _ = sipgo.NewClient(gb.ua, sipgo.WithClientLogger(zerolog.New(os.Stdout)), sipgo.WithClientHostname(d.SipIP))
	d.channels.L = new(sync.RWMutex)
	d.Info("StoreDevice", "source", source, "desc", desc, "device.SipIP", myLanIP, "device.WanIP", myWanIP, "recipient", req.Recipient, "myPort", myPort)

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

func (gb *GB28181Plugin) StoreDevice(deviceid string, req *sip.Request) (d *Device) {
	source := req.Source()
	sourceIP, sourcePortStr, _ := net.SplitHostPort(source)
	sourcePort, _ := strconv.Atoi(sourcePortStr)
	desc := req.Destination()
	myIP, myPortStr, _ := net.SplitHostPort(desc)
	myPort, _ := strconv.Atoi(myPortStr)

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

	// 检查myPort是否在sipPorts中，如果不在则使用sipPorts[0]
	if len(gb.sipPorts) > 0 {
		portFound := false
		for _, port := range gb.sipPorts {
			if port == myPort {
				portFound = true
				break
			}
		}
		if !portFound {
			myPort = gb.sipPorts[0]
			gb.Debug("StoreDevice", "使用默认端口替换", myPort)
		}
	}

	// 如果设备IP是内网IP，则使用内网IP
	myIPParse := net.ParseIP(myIP)
	sourceIPParse := net.ParseIP(sourceIP)

	// 优先使用内网IP
	myLanIP := myip.InternalIPv4()
	myWanIP := myip.ExternalIPv4()

	gb.Info("Start StoreDevice", "source", source, "desc", desc, "myLanIP", myLanIP, "myWanIP", myWanIP)

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

	if gb.MediaIP != "" {
		myWanIP = gb.MediaIP
	}
	if gb.SipIP != "" {
		myLanIP = gb.SipIP
	}

	now := time.Now()
	d = &Device{
		DeviceID:      deviceid,
		CreateTime:    now,
		UpdateTime:    now,
		RegisterTime:  now,
		KeepaliveTime: now,
		Status:        DeviceOnlineStatus,
		Online:        true,
		StreamMode:    "TCP-PASSIVE",   // 默认UDP传输
		Charset:       "GB2312",        // 默认GB2312字符集
		GeoCoordSys:   "WGS84",         // 默认WGS84坐标系
		Transport:     req.Transport(), // 传输协议
		IP:            sourceIP,
		Port:          sourcePort,
		HostAddress:   sourceIP + ":" + sourcePortStr,
		SipIP:         myLanIP,
		MediaIP:       myWanIP,
		Expires:       int(expSec),
		eventChan:     make(chan any, 10),
		Recipient: sip.Uri{
			Host: sourceIP,
			Port: sourcePort,
			User: deviceid,
		},
		contactHDR: sip.ContactHeader{
			Address: sip.Uri{
				User: gb.Serial,
				Host: myWanIP,
				Port: myPort,
			},
		},
		fromHDR: sip.FromHeader{
			Address: sip.Uri{
				User: gb.Serial,
				Host: myWanIP,
				Port: myPort,
			},
			Params: sip.NewParams(),
		},
		plugin:    gb,
		localPort: myPort,
	}

	d.Logger = gb.With("deviceid", deviceid)
	d.fromHDR.Params.Add("tag", sip.GenerateTagN(16))
	d.client, _ = sipgo.NewClient(gb.ua, sipgo.WithClientLogger(zerolog.New(os.Stdout)), sipgo.WithClientHostname(d.SipIP))
	d.channels.L = new(sync.RWMutex)
	d.Info("StoreDevice", "source", source, "desc", desc, "device.SipIP", myLanIP, "device.WanIP", myWanIP, "req.Recipient", req.Recipient, "myPort", myPort, "d.Recipient", d.Recipient)

	// 使用简单的 hash 函数将设备 ID 转换为 uint32
	var hash uint32
	for i := 0; i < len(d.DeviceID); i++ {
		ch := d.DeviceID[i]
		hash = hash*31 + uint32(ch)
	}
	d.Task.ID = hash

	d.OnStart(func() {
		gb.devices.Set(d)
		d.channels.OnAdd(func(c *Channel) {
			if absDevice, ok := gb.Server.PullProxies.Find(func(absDevice m7s.IPullProxy) bool {
				conf := absDevice.GetConfig()
				return conf.Type == "gb28181" && conf.URL == fmt.Sprintf("%s/%s", d.DeviceID, c.DeviceID)
			}); ok {
				c.PullProxyTask = absDevice.(*PullProxy)
				absDevice.ChangeStatus(m7s.PullProxyStatusOnline)
			}
		})
	})
	d.OnDispose(func() {
		d.Status = DeviceOfflineStatus
		if gb.devices.RemoveByKey(d.DeviceID) {
			for c := range d.channels.Range {
				if c.PullProxyTask != nil {
					c.PullProxyTask.ChangeStatus(m7s.PullProxyStatusOffline)
				}
			}
		}
	})
	gb.AddTask(d)

	if gb.DB != nil {
		var existing Device
		if err := gb.DB.First(&existing, Device{DeviceID: d.DeviceID}).Error; err == nil {
			d.ID = existing.ID // 保持原有的自增ID
			gb.DB.Save(d).Omit("create_time")
			gb.Info("StoreDevice", "type", "更新设备", "deviceId", d.DeviceID)
		} else {
			gb.DB.Save(d)
			gb.Info("StoreDevice", "type", "新增设备", "deviceId", d.DeviceID)
		}
	}
	return
}

func (gb *GB28181Plugin) Pull(streamPath string, conf config.Pull, pubConf *config.Publish) {
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

func (gb *GB28181Plugin) GetPullableList() []string {
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
//	gb        *GB28181Plugin
//}
//
//func (gb *GB28181Plugin) OnTCPConnect(conn *net.TCPConn) task.ITask {
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

func (gb *GB28181Plugin) OnBye(req *sip.Request, tx sip.ServerTransaction) {
	if dialog, ok := gb.dialogs.Find(func(d *Dialog) bool {
		return d.GetCallID() == req.CallID().Value()
	}); ok {
		gb.Warn("OnBye", "devicedialog", dialog.GetCallID())
		dialog.Stop(task.ErrTaskComplete)
	}
	if forwardDialog, ok := gb.forwardDialogs.Find(func(d *ForwardDialog) bool {
		return d.platformCallId == req.CallID().Value()
	}); ok {
		err := tx.Respond(sip.NewResponseFromRequest(req, http.StatusOK, "OK", req.Body()))
		if err != nil {
			gb.Error("forwarddialog bye err", err)
		}
		gb.Warn("OnBye", "forwardDialog.platformCallId", req.CallID().Value())
		forwardDialog.Stop(task.ErrTaskComplete)
	}
}

func (gb *GB28181Plugin) GetSerial() string {
	return gb.Serial
}

func (gb *GB28181Plugin) OnInvite(req *sip.Request, tx sip.ServerTransaction) {
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
			if platformTmp, platformFound := gb.platforms.Get(platformModel.ServerGBID); !platformFound {
				gb.Error("OnInvite", "error", "platform found in DB but not in runtime", "platformId", inviteInfo.RequesterId)
				_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusNotFound, "Platform Not Found In Runtime", nil))
				return
			} else {
				platform = platformTmp
			}

			gb.Info("OnInvite", "action", "platform found", "platformId", inviteInfo.RequesterId, "platformName", platform.PlatformModel.Name)

			// 使用GORM的模型查询方式，更加符合GORM的使用习惯
			// 默认情况下GORM会自动处理软删除，只查询未删除的记录
			var deviceChannels []gb28181.DeviceChannel
			channelResult := gb.DB.Model(&gb28181.DeviceChannel{}).
				Joins("LEFT JOIN gb28181_platform_channel ON gb28181_channel.id = gb28181_platform_channel.channel_db_id").
				Where("gb28181_platform_channel.platform_server_gb_id = ? AND gb28181_channel.channel_id = ?",
					platform.PlatformModel.ServerGBID, inviteInfo.TargetChannelId).
				Order("gb28181_channel.id").
				Find(&deviceChannels)

			if channelResult.Error != nil || len(deviceChannels) == 0 {
				gb.Error("OnInvite", "error", "channel not found", "channelId", inviteInfo.TargetChannelId, "err", channelResult.Error)
				_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusNotFound, "Channel Not Found", nil))
				return
			}

			// 找到了通道
			channel := deviceChannels[len(deviceChannels)-1]
			gb.Info("OnInvite", "action", "channel found", "channelId", channel.ChannelID, "channelName", channel.Name)

			var channelTmp *Channel
			if deviceFound, ok := gb.devices.Find(func(device *Device) bool {
				return device.DeviceID == channel.DeviceID
			}); ok {
				if channelFound, ok := deviceFound.channels.Get(channel.ChannelID); ok {
					channelTmp = channelFound
				} else {
					gb.Error("OnInvite", "error", "channel not found memory,ChannelID is ", channel.ChannelID)
					_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusBadRequest, "SSRC Not Found", nil))
					return
				}
			} else {
				gb.Error("OnInvite", "error", "device not found memory,deviceID is ", channel.DeviceID)
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
				fmt.Sprintf("o=%s 0 0 IN IP4 %s", channel.ChannelID, sdpIP),
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
				upIP:           sdpIP,
				upPort:         mediaPort,
			}
			forwardDialog.forwarder = gb28181.NewRTPForwarder()
			forwardDialog.forwarder.TCP = forwardDialog.TCP
			forwardDialog.forwarder.TCPActive = forwardDialog.TCPActive
			forwardDialog.forwarder.StreamMode = forwardDialog.channel.Device.StreamMode

			if forwardDialog.TCPActive {
				forwardDialog.forwarder.UpListenAddr = fmt.Sprintf(":%d", forwardDialog.upPort)
			} else {
				forwardDialog.forwarder.UpListenAddr = fmt.Sprintf("%s:%d", forwardDialog.upIP, forwardDialog.platformPort)
			}

			// 设置监听地址和端口
			if strings.ToUpper(forwardDialog.channel.Device.StreamMode) == "TCP-ACTIVE" {
				forwardDialog.forwarder.DownListenAddr = fmt.Sprintf("%s:%d", forwardDialog.downIP, forwardDialog.downPort)
			} else {
				forwardDialog.forwarder.DownListenAddr = fmt.Sprintf(":%d", forwardDialog.MediaPort)
			}

			// 设置转发目标
			if inviteInfo.IP != "" && forwardDialog.platformPort > 0 {
				err = forwardDialog.forwarder.SetTarget(forwardDialog.platformIP, forwardDialog.platformPort)
				if err != nil {
					gb.Error("set target error", "err", err)
					return
				}
			} else {
				gb.Error("no target set, will only receive but not forward")
				return
			}

			// 设置目标SSRC
			if forwardDialog.platformSSRC != "" {
				forwardDialog.forwarder.TargetSSRC = forwardDialog.platformSSRC
				gb.Info("set target ssrc", "ssrc", forwardDialog.platformSSRC)
			}
			// 保存到集合中
			gb.forwardDialogs.Set(forwardDialog)
			gb.Info("OnInvite", "action", "sendRtpInfo created", "callId", req.CallID().Value())

			if err := tx.Respond(response); err != nil {
				gb.Error("OnInvite", "error", "send response failed", "err", err.Error())
				return
			}

			gb.Info("OnInvite", "action", "complete", "platformId", inviteInfo.RequesterId, "channelId", channel.ChannelID,
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

func (gb *GB28181Plugin) OnAck(req *sip.Request, tx sip.ServerTransaction) {
	callID := req.CallID().Value()
	if callID == "" {
		gb.Error("OnAck", "error", "callid header not found")
		return
	}
	// 构建streamPath
	if forwardDialog, ok := gb.forwardDialogs.Find(func(dialog *ForwardDialog) bool {
		return dialog.platformCallId == callID
	}); ok {
		pullUrl := fmt.Sprintf("%s/%s", forwardDialog.channel.Device.DeviceID, forwardDialog.channel.ChannelID)
		streamPath := fmt.Sprintf("platform_%d/%s/%s", time.Now().UnixMilli(), forwardDialog.channel.Device.DeviceID, forwardDialog.channel.ChannelID)

		// 创建配置
		pullConf := config.Pull{
			URL: pullUrl,
		}
		// 初始化拉流任务
		forwardDialog.GetPullJob().Init(forwardDialog, &gb.Plugin, streamPath, pullConf, nil)
	} else {
		gb.Error("OnAck", "error", "forwardDialog not found", "callID", callID)
		return
	}
}
