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

// PlayStreamCmd 请求预览视频流
func (gb *GB28181ProPlugin) PlayStreamCmd(device *Device, channel *Channel, mediaPort uint16) (*sipgo.DialogClientSession, error) {
	if channel == nil {
		return nil, fmt.Errorf("channel is nil")
	}
	if device == nil {
		return nil, fmt.Errorf("device is nil")
	}

	// 创建 SSRC
	ssrc := device.CreateSSRC(gb.Serial)

	// 获取 SDP IP
	sdpIP := device.LocalIP
	if sdpIP == "" {
		sdpIP = device.mediaIp
	}

	// 构建 SDP 内容
	sdpInfo := []string{
		"v=0",
		fmt.Sprintf("o=%s 0 0 IN IP4 %s", device.ID, sdpIP),
		"s=Play",
		//"u=" + device.ID + ":0",
		"c=IN IP4 " + sdpIP,
		"t=0 0",
	}

	// 添加媒体行和相关属性
	var mediaLine string
	switch strings.ToUpper(device.StreamMode) {
	case "TCP-PASSIVE", "TCP-ACTIVE":
		mediaLine = fmt.Sprintf("m=video %d TCP/RTP/AVP 96 126 125 99 34 98 97", mediaPort)
	case "UDP":
		mediaLine = fmt.Sprintf("m=video %d TCP/RTP/AVP 96", mediaPort)
	default:
		mediaLine = fmt.Sprintf("m=video %d RTP/AVP 96 126 125 99 34 98 97", mediaPort)
	}

	sdpInfo = append(sdpInfo, mediaLine)
	sdpInfo = append(sdpInfo,
		"a=recvonly",
		"a=rtpmap:96 PS/90000",
		//"a=fmtp:126 profile-level-id=42e01e",
		//"a=rtpmap:126 H264/90000",
		//"a=rtpmap:125 H264S/90000",
		//"a=fmtp:125 profile-level-id=42e01e",
		//"a=rtpmap:99 H265/90000",
		//"a=rtpmap:98 H264/90000",
		//"a=rtpmap:97 MPEG4/90000",
	)

	//根据传输模式添加 setup 和 connection 属性
	switch strings.ToUpper(device.StreamMode) {
	case "TCP-PASSIVE":
		sdpInfo = append(sdpInfo,
			"a=setup:passive",
			"a=connection:new",
		)
	case "TCP-ACTIVE":
		sdpInfo = append(sdpInfo,
			"a=setup:active",
			"a=connection:new",
		)
	default:
		sdpInfo = append(sdpInfo,
			"a=setup:passive",
			"a=connection:new",
		)
	}

	// 添加 SSRC
	sdpInfo = append(sdpInfo, fmt.Sprintf("y=%010d", ssrc))

	// 创建 INVITE 请求
	request := device.CreateRequest(sip.INVITE)
	if request == nil {
		return nil, fmt.Errorf("create request failed")
	}

	// 设置必需的头部
	contentTypeHeader := sip.ContentTypeHeader("application/sdp")
	subjectHeader := sip.NewHeader("Subject", fmt.Sprintf("%s:%d,%s:0", channel.DeviceID, ssrc, gb.Serial))
	allowHeader := sip.NewHeader("Allow", "INVITE, ACK, CANCEL, REGISTER, MESSAGE, NOTIFY, BYE")
	//Toheader里需要放入目录通道的id
	toHeader := sip.ToHeader{
		Address: sip.Uri{User: channel.DeviceID, Host: device.HostAddress},
	}

	request.AppendHeader(&contentTypeHeader)
	request.AppendHeader(subjectHeader)
	request.AppendHeader(allowHeader)
	request.AppendHeader(&toHeader)
	request.SetBody([]byte(strings.Join(sdpInfo, "\r\n") + "\r\n"))

	// 创建会话
	return device.dialogClient.Invite(gb, device.Recipient, request.Body(), &contentTypeHeader, subjectHeader, &device.fromHDR, allowHeader, &toHeader)
}

type GB28181ProPlugin struct {
	pb.UnimplementedApiServer
	m7s.Plugin
	AutoInvite bool   `default:"true" desc:"自动邀请"`
	Serial     string `default:"34020000002000000001" desc:"sip 服务 id"` //sip 服务器 id, 默认 34020000002000000001
	Realm      string `default:"3402000000" desc:"sip 服务域"`            //sip 服务器域，默认 3402000000
	Username   string
	Password   string
	Sip        SipConfig
	MediaPort  util.Range[uint16] `default:"10000-20000" desc:"媒体端口范围"` //媒体端口范围
	Position   PositionConfig
	Parent     string `desc:"父级设备"`
	ua         *sipgo.UserAgent
	server     *sipgo.Server
	devices    util.Collection[string, *Device]
	dialogs    util.Collection[uint32, *Dialog]
	tcpPorts   chan uint16
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
		gb.server.OnRegister(gb.OnRegister)
		gb.server.OnMessage(gb.OnMessage)
		gb.server.OnBye(gb.OnBye)
		gb.devices.L = new(sync.RWMutex)
		gb.server.OnInvite(gb.OnInvite)
		//gb.server.OnAck(gb.OnAck)

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
			gb.DB.AutoMigrate(&Device{})
			gb.DB.AutoMigrate(&gb28181.DeviceChannel{})
			gb.DB.AutoMigrate(&gb28181.PlatformModel{})
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

	// 检查设备是否存在（通过deviceId判断）
	if d, ok := gb.devices.Get(deviceid); ok {
		gb.Info("OnRegister", "type", "续订", "deviceId", deviceid)
		d.UpdateTime = time.Now()
		d.RegisterTime = time.Now()
		d.Expires = int(expSec)
		d.Online = true
		if gb.DB != nil {
			gb.DB.Save(d) // 这里需要保留，因为这是设备续订时的状态更新
		}
		response := sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil)
		response.AppendHeader(sip.NewHeader("Expires", fmt.Sprintf("%d", expSec)))
		response.AppendHeader(sip.NewHeader("Date", time.Now().Local().Format(util.LocalTimeFormat)))
		response.AppendHeader(sip.NewHeader("Server", "M7S/"+m7s.Version))
		response.AppendHeader(sip.NewHeader("Allow", "INVITE,ACK,CANCEL,BYE,NOTIFY,OPTIONS,PRACK,UPDATE,REFER"))
		if err = tx.Respond(response); err != nil {
			gb.Error("respond OK", "error", err.Error())
		}
		return
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
	if err = tx.Respond(response); err != nil {
		gb.Error("respond OK", "error", err.Error())
	}
	if isUnregister {
		if d, ok := gb.devices.Get(deviceid); ok {
			d.Online = false
			if gb.DB != nil {
				gb.DB.Save(d)
			}
			d.Stop(errors.New("unregister"))
		}
	} else {
		if d, ok := gb.devices.Get(deviceid); ok {
			gb.RecoverDevice(d, req)
			d.Online = true
		} else {
			gb.StoreDevice(deviceid, req)
		}
	}
}

func (gb *GB28181ProPlugin) OnMessage(req *sip.Request, tx sip.ServerTransaction) {
	from := req.From()
	if from == nil || from.Address.User == "" {
		gb.Error("OnMessage", "error", "no user")
		return
	}
	id := from.Address.User

	// 检查消息来源
	var d *Device
	var p *gb28181.PlatformModel
	var ok bool

	// 先从设备缓存中获取
	if d, ok = gb.devices.Get(id); !ok {
		// 如果内存中没有，尝试从数据库查找
		if gb.DB != nil {
			var device Device
			if err := gb.DB.First(&device, Device{DeviceID: id}).Error; err == nil {
				d = &device
			}
		}
		gb.RecoverDevice(d, req)
		// 如果设备不存在，直接返回404
		if d == nil {
			gb.Error("OnMessage", "error", "device not found", "DeviceID", id)
			response := sip.NewResponseFromRequest(req, sip.StatusNotFound, "Not Found", nil)
			if err := tx.Respond(response); err != nil {
				gb.Error("respond NotFound", "error", err.Error())
			}
			return
		}
	}

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

	// 根据来源调用不同的处理方法
	if d != nil {
		d.UpdateTime = time.Now()
		if err = d.onMessage(req, tx, temp); err != nil {
			gb.Error("onMessage", "error", err.Error(), "type", "device")
		}
	} else {
		// 创建 Platform 实例
		platform := &Platform{
			PlatformModel: p,
			plugin:        gb,
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

	// 设置 fromHDR
	d.fromHDR = sip.FromHeader{
		Address: sip.Uri{
			User: gb.Serial,
			Host: gb.Realm,
		},
		Params: sip.NewParams(),
	}
	d.fromHDR.Params.Add("tag", sip.GenerateTagN(16))

	// 初始化日志
	d.Logger = gb.With("id", d.DeviceID)

	// 初始化 SIP 客户端
	d.client, _ = sipgo.NewClient(gb.ua, sipgo.WithClientLogger(zerolog.New(os.Stdout)), sipgo.WithClientHostname(host))
	d.dialogClient = sipgo.NewDialogClient(d.client, d.contactHDR)

	d.StartTime = time.Now()
	d.Status = DeviceRecoverStatus
	d.UpdateTime = time.Now()
	d.plugin = gb
	gb.devices.Add(d)
}

func (gb *GB28181ProPlugin) StoreDevice(deviceid string, req *sip.Request) (d *Device) {
	from := req.From()
	source := req.Source()
	desc := req.Destination()
	servIp, sPortStr, _ := net.SplitHostPort(desc)
	deviceIP, _, _ := net.SplitHostPort(source)

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

	hostname, portStr, _ := net.SplitHostPort(source)
	port, _ := strconv.Atoi(portStr)
	serverPort, _ := strconv.Atoi(sPortStr)

	now := time.Now()
	d = &Device{
		DeviceID:      deviceid,
		CreateTime:    now,
		UpdateTime:    now,
		RegisterTime:  now,
		Status:        DeviceRegisterStatus,
		Online:        true,
		StreamMode:    "UDP",           // 默认UDP传输
		Charset:       "GB2312",        // 默认GB2312字符集
		GeoCoordSys:   "WGS84",         // 默认WGS84坐标系
		MediaServerID: "auto",          // 默认auto
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
			User: from.Address.User,
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
		plugin: gb,
	}

	d.Logger = gb.With("deviceid", deviceid)
	d.fromHDR.Params.Add("tag", sip.GenerateTagN(16))
	d.client, _ = sipgo.NewClient(gb.ua, sipgo.WithClientLogger(zerolog.New(os.Stdout)), sipgo.WithClientHostname(host))
	d.dialogClient = sipgo.NewDialogClient(d.client, d.contactHDR)
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
					URL: fmt.Sprintf("%s/%s", d.DeviceID, c.DeviceID),
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

	// 检查设备是否存在
	d, ok := gb.devices.Get(inviteInfo.RequesterId)
	if !ok {
		gb.Error("OnInvite", "error", "device not found", "deviceId", inviteInfo.RequesterId)
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusNotFound, "Device Not Found", nil))
		return
	}

	// 检查通道是否存在
	_, ok = d.channels.Get(inviteInfo.TargetChannelId)
	if !ok {
		gb.Error("OnInvite", "error", "channel not found", "channelId", inviteInfo.TargetChannelId)
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusNotFound, "Channel Not Found", nil))
		return
	}

	gb.Info("OnInvite", "action", "start", "deviceId", inviteInfo.RequesterId, "channelId", inviteInfo.TargetChannelId)

	// 发送100 Trying响应
	_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusTrying, "Trying", nil))

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

	// 设置SSRC
	ssrc := d.CreateSSRC(gb.Serial)
	gb.Debug("OnInvite", "action", "create ssrc", "ssrc", ssrc)

	// 构建SDP响应
	sdpIP := d.LocalIP
	if sdpIP == "" {
		sdpIP = d.mediaIp
	}

	responseContent := []string{
		"v=0",
		fmt.Sprintf("o=%s 0 0 IN IP4 %s", gb.Serial, sdpIP),
		"s=Play",
		fmt.Sprintf("c=IN IP4 %s", sdpIP),
		"t=0 0",
	}

	// 根据传输模式添加媒体行
	var mediaLine string
	switch strings.ToUpper(d.StreamMode) {
	case "TCP-PASSIVE", "TCP-ACTIVE":
		mediaLine = fmt.Sprintf("m=video %d TCP/RTP/AVP 96", mediaPort)
		responseContent = append(responseContent, mediaLine)
		if d.StreamMode == "TCP-PASSIVE" {
			responseContent = append(responseContent, "a=setup:passive")
		} else {
			responseContent = append(responseContent, "a=setup:active")
		}
		responseContent = append(responseContent, "a=connection:new")
		gb.Debug("OnInvite", "action", "create sdp", "mode", d.StreamMode)
	default:
		mediaLine = fmt.Sprintf("m=video %d RTP/AVP 96", mediaPort)
		responseContent = append(responseContent, mediaLine)
		gb.Debug("OnInvite", "action", "create sdp", "mode", "UDP")
	}

	responseContent = append(responseContent,
		"a=recvonly",
		"a=rtpmap:96 PS/90000",
		fmt.Sprintf("y=%010d", ssrc),
	)

	// 发送200 OK响应
	response := sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil)
	contentType := sip.ContentTypeHeader("application/sdp")
	response.AppendHeader(&contentType)
	response.SetBody([]byte(strings.Join(responseContent, "\r\n") + "\r\n"))

	if err := tx.Respond(response); err != nil {
		gb.Error("OnInvite", "error", "send response failed", "err", err.Error())
		return
	}

	gb.Info("OnInvite", "action", "complete", "deviceId", inviteInfo.RequesterId, "channelId", inviteInfo.TargetChannelId,
		"ip", inviteInfo.IP, "port", inviteInfo.Port, "tcp", inviteInfo.TCP, "tcpActive", inviteInfo.TCPActive)

	// TODO: 实现媒体流处理
	// 1. 创建合适的Publisher
	// 2. 创建PS流接收器
	// 3. 配置接收参数：
	//    - 使用解析出的 inviteInfo.IP 和 inviteInfo.Port 作为目标地址
	//    - 使用 inviteInfo.TCP 和 inviteInfo.TCPActive 确定传输模式
	//    - 使用本地分配的 mediaPort 作为监听端口
	// 4. 启动接收任务
	// 5. 处理媒体流解复用
	// 6. 根据 inviteInfo.SessionName 判断是实时点播还是历史回放
	//    - 如果是回放，使用 inviteInfo.StartTime 和 inviteInfo.StopTime
	// 7. 使用 inviteInfo.SSRC 标识流
	// 8. 如果指定了 inviteInfo.DownloadSpeed，控制下载速度
}
