package plugin_gb28181pro

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/langhuihui/gomem"
	"github.com/pion/rtp"
	"m7s.live/v5/pkg"
	mpegps "m7s.live/v5/pkg/format/ps"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	task "github.com/langhuihui/gotask"
	m7s "m7s.live/v5"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/util"
	"m7s.live/v5/plugin/gb28181/pb"
	gb28181 "m7s.live/v5/plugin/gb28181/pkg"
	mrtp "m7s.live/v5/plugin/rtp/pkg"
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
	clients        util.Collection[string, *ClientWrapper] // Client池，key为"IP:Port"
	defaultSipIP   string                                  // 默认SIP IP
	defaultSipPort int                                     // 默认SIP Port
	devices        task.WorkCollection[string, *Device]
	dialogs        util.Collection[string, *Dialog]
	forwardDialogs util.Collection[uint32, *ForwardDialog]
	platforms      task.WorkCollection[string, *Platform]
	tcpPort        uint16 // 单端口模式下的 TCP 端口
	udpPort        uint16 // 单端口模式下的 UDP 端口
	// 端口位图管理（多端口模式）
	tcpPB                 PortBitmap
	udpPB                 PortBitmap
	sipPorts              []int
	SipIP                 string `desc:"sip发送命令的IP，一般是本地IP，多网卡时需要配置正确的IP"`
	MediaIP               string `desc:"流媒体IP，用于接收流"`
	deviceRegisterManager task.WorkCollection[string, *DeviceRegisterQueueTask]
	Platforms             []*gb28181.PlatformModel
	channels              util.Collection[string, *Channel]
	singlePorts           util.Collection[uint32, *gb28181.SinglePortReader]
	downloadDialogs       task.WorkCollection[string, *DownloadDialog]
}

// ClientWrapper 包装sipgo.Client以实现GetKey接口
type ClientWrapper struct {
	*sipgo.Client
	key string
}

func (c *ClientWrapper) GetKey() string {
	return c.key
}

var _ = m7s.InstallPlugin[GB28181Plugin](m7s.PluginMeta{
	RegisterGRPCHandler: pb.RegisterApiHandler,
	ServiceDesc:         &pb.Api_ServiceDesc,
	NewPuller: func(conf config.Pull) m7s.IPuller {
		if util.Exist(conf.URL) {
			return &mrtp.DumpPuller{}
		}
		return new(Dialog)
	},
	NewPullProxy: NewPullProxy,
})

// RegisterHandler 注册自定义 HTTP 路由
func (gb *GB28181Plugin) RegisterHandler() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"/download/{deviceId}/{channelId}/{filename}": gb.handleDownloadFile,
	}
}

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
			&gb28181.DevicePosition{},
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
				gb.Warn("根组织已存在但获取详情失败", "error", err)
			} else {
				gb.Info("根组织已存在", "id", root.ID)
			}
		}
	} else {
		gb.Info("自动迁移已禁用，跳过表结构迁移和根组织初始化")
	}

	return nil
}

func (gb *GB28181Plugin) Start() (err error) {
	if gb.DB == nil {
		return pkg.ErrNoDB
	}
	gb.Info("GB28181 initing", gb.Platforms)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level:     slog.LevelDebug,
		AddSource: false,
	}))
	slog.SetDefault(logger) // 设置为默认logger，确保所有日志都使用这个配置
	// 设置 TCP 传输模式
	tcpOption := sip.WithTransportLayerConnectionReuse(true) // 启用连接重用
	gb.ua, err = sipgo.NewUA(
		sipgo.WithUserAgent("M7S/"+m7s.Version),
		sipgo.WithUserAgentTransportLayerOptions(tcpOption), // 使用 TCP 选项
	) // Build user agent
	// Creating client handle for ua
	if len(gb.Sip.ListenAddr) > 0 {
		gb.AddTask(&catalogHandlerQueueTask)
		gb.AddTask(&gb.devices)
		gb.AddTask(&gb.platforms)
		gb.AddTask(&gb.deviceRegisterManager)
		gb.AddTask(&gb.downloadDialogs)
		gb.dialogs.L = new(sync.RWMutex)
		gb.forwardDialogs.L = new(sync.RWMutex)
		gb.singlePorts.L = new(sync.RWMutex)
		gb.clients.L = new(sync.RWMutex)
		gb.server, _ = sipgo.NewServer(gb.ua, sipgo.WithServerLogger(logger)) // Creating server handle for ua
		gb.server.OnMessage(gb.OnMessage)
		gb.server.OnRegister(gb.OnRegister)
		gb.server.OnBye(gb.OnBye)
		gb.server.OnInvite(gb.OnInvite)
		gb.server.OnAck(gb.OnAck)
		gb.server.OnNotify(gb.OnNotify)

		if gb.MediaPort.Valid() {
			gb.SetDescription("media port", fmt.Sprintf("%d-%d", gb.MediaPort[0], gb.MediaPort[1]))
			if gb.MediaPort.Size() == 0 {
				gb.tcpPort = gb.MediaPort[0]
				gb.AddTask(&gb28181.SinglePortTCP{
					Port:       gb.tcpPort,
					Collection: &gb.singlePorts,
				})
				gb.udpPort = gb.MediaPort[0]
				gb.AddTask(&gb28181.SinglePortUDP{
					Port:       gb.udpPort,
					Collection: &gb.singlePorts,
				})
			} else {
				// 初始化位图
				gb.tcpPB.Init(gb.MediaPort[0], uint16(gb.MediaPort.Size()))
				gb.udpPB.Init(gb.MediaPort[0], uint16(gb.MediaPort.Size()))
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

		// 初始化默认SIP配置
		// 用于在无法从设备请求中确定本地IP时使用
		gb.defaultSipIP = gb.SipIP
		gb.defaultSipPort = 5060 // 默认端口

		if gb.defaultSipIP == "" {
			// 从第一个监听地址提取默认IP
			if len(gb.Sip.ListenAddr) > 0 {
				_, addr, _ := strings.Cut(gb.Sip.ListenAddr[0], ":")
				if strings.HasPrefix(addr, ":") {
					// 如果是 ":5060" 格式，提取端口
					gb.defaultSipIP = "0.0.0.0"
					if port, err := strconv.Atoi(strings.TrimPrefix(addr, ":")); err == nil {
						gb.defaultSipPort = port
					}
				} else {
					// 如果是 "192.168.1.106:5060" 格式，提取IP和端口
					host, portStr, _ := net.SplitHostPort(addr)
					if host != "" {
						gb.defaultSipIP = host
					} else {
						gb.defaultSipIP = addr
					}
					if port, err := strconv.Atoi(portStr); err == nil {
						gb.defaultSipPort = port
					}
				}
			}
		}
		gb.Info("默认SIP配置已初始化", "defaultSipIP", gb.defaultSipIP, "defaultSipPort", gb.defaultSipPort)

		if gb.DB != nil {
			err = gb.initDatabase()
			if err != nil {
				gb.Error("initDatabase", "error", err)
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

// getOrCreateClient 根据IP:Port:Transport获取或创建Client
// hostname: SIP IP地址
// port: SIP端口
// transport: 传输协议（TCP/UDP）
func (gb *GB28181Plugin) getOrCreateClient(hostname string, port int, transport string) (*sipgo.Client, error) {
	// Key包含transport，因为同一个IP:Port可能同时有TCP和UDP
	key := fmt.Sprintf("%s:%d:%s", hostname, port, strings.ToUpper(transport))
	
	// 尝试从缓存获取
	if wrapper, ok := gb.clients.Get(key); ok {
		return wrapper.Client, nil
	}
	
	// 创建新Client
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level:     slog.LevelDebug,
		AddSource: false,
	}))
	
	client, err := sipgo.NewClient(gb.ua,
		sipgo.WithClientLogger(logger),
		sipgo.WithClientHostname(hostname),
		sipgo.WithClientPort(port),
	)
	if err != nil {
		return nil, fmt.Errorf("创建Client失败 %s: %v", key, err)
	}
	
	// 存入缓存（需要包装成实现GetKey的类型）
	wrapper := &ClientWrapper{
		Client: client,
		key:    key,
	}
	gb.clients.Set(wrapper)
	gb.Info("创建新Client", "hostname", hostname, "port", port, "key", key)
	
	return client, nil
}

func (gb *GB28181Plugin) deleteDevice(device *Device, reason string) bool {
	gb.Info(fmt.Sprintf("准备删除设备: %s", reason), "deviceId", device.DeviceId)

	// 开启数据库事务
	tx := gb.DB.Begin()
	if tx.Error != nil {
		gb.Error("开启事务失败", "error", tx.Error)
		return false
	}

	// 删除设备
	if err := tx.Delete(&Device{DeviceId: device.DeviceId}).Error; err != nil {
		tx.Rollback()
		gb.Error(fmt.Sprintf("删除设备失败: %s", reason), "error", err, "deviceId", device.DeviceId)
		return false
	}

	// 删除设备关联的通道
	if err := tx.Where("device_id = ?", device.DeviceId).Delete(&gb28181.DeviceChannel{}).Error; err != nil {
		tx.Rollback()
		gb.Error(fmt.Sprintf("删除设备通道失败: %s", reason), "error", err, "deviceId", device.DeviceId)
		return false
	}

	// 提交事务
	if err := tx.Commit().Error; err != nil {
		tx.Rollback()
		gb.Error("提交事务失败", "error", err, "deviceId", device.DeviceId)
		return false
	}

	gb.Info(fmt.Sprintf("已删除设备: %s", reason), "deviceId", device.DeviceId)
	return true
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
		// 检查设备是否过期
		expireTime := device.RegisterTime.Add(time.Duration(device.Expires) * time.Second)
		isExpired := now.After(expireTime)
		if device.CustomName == "" {
			device.CustomName = device.Name
		}
		// 设置设备基本属性
		device.Status = DeviceOfflineStatus
		if !isExpired {
			device.Status = DeviceOnlineStatus
		}
		device.Online = !isExpired

		// 设置事件通道
		device.eventChan = make(chan any, 10)

		// 设置Logger
		device.Logger = gb.Logger.With("deviceid", device.DeviceId)

		// 初始化通道集合
		device.channels.L = new(sync.RWMutex)

		// 初始化目录请求集合
		device.catalogReqs.L = new(sync.RWMutex)

		// 设置plugin引用
		device.plugin = gb

		// 设置联系人头信息
		device.contactHDR = sip.ContactHeader{
			Address: sip.Uri{
				User: gb.Serial,
				Host: device.SipIp,
				Port: device.LocalPort,
			},
		}

		// 设置来源头信息
		device.fromHDR = sip.FromHeader{
			Address: sip.Uri{
				User: gb.Serial,
				Host: device.SipIp,
				Port: device.LocalPort,
			},
			Params: sip.NewParams(),
		}
		device.fromHDR.Params.Add("tag", sip.GenerateTagN(16))

		// 设置接收者
		device.Recipient = sip.Uri{
			Host: device.IP,
			Port: device.Port,
			User: device.DeviceId,
		}

		// 根据设备的SipIp、LocalPort和Transport获取或创建对应的Client
		transport := device.Transport
		if transport == "" {
			transport = "UDP" // 默认UDP
		}
		client, err := gb.getOrCreateClient(device.SipIp, device.LocalPort, transport)
		if err != nil {
			gb.Error("创建Device Client失败", "error", err, "deviceId", device.DeviceId, "sipIp", device.SipIp, "localPort", device.LocalPort, "transport", transport)
			continue
		}
		device.client = client
		device.Info("checkDeviceExpire", "d.SipIp", device.SipIp, "d.LocalPort", device.LocalPort, "d.contactHDR", device.contactHDR)
		device.channels.OnAdd(func(c *Channel) {
			if absDevice, ok := gb.Server.PullProxies.Find(func(absDevice m7s.IPullProxy) bool {
				conf := absDevice.GetConfig()
				return conf.Type == "gb28181" && conf.URL == fmt.Sprintf("%s/%s", device.DeviceId, c.ChannelId)
			}); ok {
				c.PullProxyTask = absDevice.(*PullProxy)
				absDevice.ChangeStatus(m7s.PullProxyStatusOnline)
			}
		})
		//device.OnDispose(func() {
		//	device.Online = false
		//	device.Status = DeviceOfflineStatus
		//	if gb.devices.RemoveByKey(device.DeviceId) {
		//		for c := range device.channels.Range {
		//			c.DeviceChannel.Status = "OFF"
		//			if c.PullProxyTask != nil {
		//				c.PullProxyTask.ChangeStatus(m7s.PullProxyStatusOffline)
		//			}
		//		}
		//	}
		//})

		// 加载设备的通道（包括deviceId或parentId等于device.DeviceId的通道）
		var channels []gb28181.DeviceChannel
		if err := gb.DB.Where("device_id = ? OR parent_id = ?", device.DeviceId, device.DeviceId).Find(&channels).Error; err != nil {
			gb.Error("加载通道失败", "error", err, "deviceId", device.DeviceId)
			continue
		}

		if gb.SipIP != "" {
			device.SipIp = gb.SipIP
		}
		if gb.MediaIP != "" {
			device.MediaIp = gb.MediaIP
		}

		// 更新设备状态到数据库
		if err := gb.DB.Model(&Device{}).Where(&Device{DeviceId: device.DeviceId}).Updates(map[string]interface{}{
			"online": device.Online,
			"status": device.Status,
		}).Error; err != nil {
			gb.Error("更新设备状态到数据库失败", "error", err, "deviceId", device.DeviceId)
		}

		// 初始化设备通道并更新到数据库
		for _, channel := range channels {
			if channel.CustomName == "" {
				channel.CustomName = channel.Name
			}
			if channel.CustomChannelId == "" {
				channel.CustomChannelId = channel.ChannelId
			}
			if isExpired {
				channel.Status = gb28181.ChannelOffStatus
			} else {
				channel.Status = gb28181.ChannelOnStatus
			}
			// 更新通道状态到数据库
			if err := gb.DB.Model(&gb28181.DeviceChannel{}).Where(&gb28181.DeviceChannel{ID: channel.ID}).Update("status", channel.Status).Error; err != nil {
				gb.Error("更新通道状态到数据库失败", "error", err, "channelId", channel.ChannelId)
			}
			device.addOrUpdateChannel(channel)
		}

		// 添加设备任务
		gb.devices.AddTask(device)
		gb.Info("设备有效", "deviceId", device.DeviceId, "registerTime", device.RegisterTime, "expireTime", expireTime, "isExpired", isExpired, "device.Online", device.Online, "device.Status", device.Status)

	}

	// 查询streamPath不为空的拉流代理通道
	var proxyChannels []gb28181.DeviceChannel
	if err := gb.DB.Where("stream_path != ? AND stream_path IS NOT NULL", "").Find(&proxyChannels).Error; err != nil {
		gb.Error("查询拉流代理通道失败", "error", err)
	} else if len(proxyChannels) > 0 {
		gb.Info("找到拉流代理通道", "count", len(proxyChannels))
		for _, c := range proxyChannels {
			// 创建Channel实例
			channel := &Channel{
				DeviceChannel: &c,
				Device:        nil, // 拉流代理通道不关联真实GB设备
				Logger:        gb.Logger.With("channel", c.ID, "streamPath", c.StreamPath),
			}
			// 添加到内存集合
			gb.channels.Add(channel)
			gb.Info("加载拉流代理通道", "channelId", c.ChannelId, "id", c.ID, "streamPath", c.StreamPath)
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
	if gb.Platforms != nil && len(gb.Platforms) > 0 {
		platformModels = append(platformModels, gb.Platforms...)
	}
	gb.Info("找到启用状态的平台", "count", len(platformModels))
	// 遍历所有平台进行初始化和注册
	for _, platformModel := range platformModels {
		// 创建Platform实例
		platform := NewPlatform(platformModel, gb, true)

		if platformModel.PlatformChannels != nil && len(platformModel.PlatformChannels) > 0 {
			for i := range platformModel.PlatformChannels {
				channelDbId := platformModel.PlatformChannels[i].ChannelDBID
				if channelDbId != "" {
					if channel, ok := gb.channels.Get(channelDbId); ok {
						platform.channels.Set(channel)
					}
				}
			}
		} else {
			// 查询通道列表
			var channels []gb28181.DeviceChannel
			if gb.DB != nil {
				if err := gb.DB.Table("gb28181_channel gc").
					Select(`gc.*`).
					Joins("left join gb28181_platform_channel gpc on gc.id=gpc.channel_db_id").
					Where("gpc.platform_server_gb_id = ? and gc.status='ON'", platformModel.ServerGBID).
					Find(&channels).Error; err != nil {
					gb.Error("<UNK>", "error", err.Error())
				}
				if channels != nil && len(channels) > 0 {
					for i := range channels {
						if channel, ok := gb.channels.Get(channels[i].ID); ok {
							platform.channels.Set(channel)
						}
					}
				}
			}
		}
		// 添加到任务系统
		gb.platforms.AddTask(platform)
		gb.Info("平台初始化完成", "ID", platformModel.ServerGBID, "Name", platformModel.Name)
	}
}

func (gb *GB28181Plugin) OnRegister(req *sip.Request, tx sip.ServerTransaction) {
	from := req.From()
	if from == nil || from.Address.User == "" {
		gb.Error("OnRegister", "invaliad from", from)
		return
	}
	// 验证设备ID是否为20位数字
	matched, err := regexp.MatchString("^\\d{20}$", from.Address.User)
	if err != nil || !matched {
		gb.Error("OnRegister", "invalid deviceId", from.Address.User)
		return
	}
	deviceId := from.Address.User
	registerHandlerTask := registerHandlerTask{
		gb:  gb,
		req: req,
		tx:  tx,
	}
	gb.Debug("onregister start", "deviceId", deviceId)

	gb.Debug("get gb.deviceRegisterManager.length", "length", gb.deviceRegisterManager.Length())
	if deviceRegisterQueueTask, ok := gb.deviceRegisterManager.Get(deviceId); ok {
		gb.Debug("gb.deviceRegisterManager.Get", "deviceId", deviceId)
		gb.Debug("gb.deviceRegisterManager.Get", "deviceRegisterQueueTask", deviceRegisterQueueTask)
		deviceRegisterQueueTask.AddTask(&registerHandlerTask)
	} else {
		deviceRegisterQueueTask := &DeviceRegisterQueueTask{
			deviceId: deviceId,
		}
		gb.Debug("do not safeget deviceRegisterQueueTask", "deviceId", deviceId)
		gb.deviceRegisterManager.AddTask(deviceRegisterQueueTask)
		deviceRegisterQueueTask.AddTask(&registerHandlerTask)
	}
}

func (gb *GB28181Plugin) OnMessage(req *sip.Request, tx sip.ServerTransaction) {
	from := req.From()
	if from == nil || from.Address.User == "" {
		gb.Error("OnMessage", "error", "no user")
		return
	}
	id := from.Address.User

	// 检查消息来源，获取字符集配置
	var d *Device
	var p *gb28181.PlatformModel
	var charset string = "GB2312" // 默认字符集

	// 先从设备缓存中获取
	d, _ = gb.devices.Get(id)
	if d != nil && d.Charset != "" {
		charset = d.Charset
	}

	// 使用正确的字符集解析消息内容
	temp := &gb28181.Message{}
	err := gb28181.DecodeXML(temp, req.Body(), charset)
	gb.Debug("OnMessage debug", "message", temp.BasicParam.Expiration, "charset", charset)
	if err != nil {
		gb.Error("OnMessage", "error", err.Error(), "charset", charset)
		response := sip.NewResponseFromRequest(req, sip.StatusBadRequest, "Bad Request", nil)
		if err := tx.Respond(response); err != nil {
			gb.Error("respond BadRequest", "error", err.Error())
		}
		return
	}

	// 检查是否是平台
	//if gb.DB != nil {
	//	var platform gb28181.PlatformModel
	//	if err := gb.DB.First(&platform, gb28181.PlatformModel{ServerGBID: id, Enable: true}).Error; err == nil {
	//		p = &platform
	//	}
	//}
	if platformtmp, ok := gb.platforms.Get(id); ok {
		if platformtmp.PlatformModel.Enable {
			p = platformtmp.PlatformModel
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
	if (d == nil && p == nil) || (d != nil && !d.Online) {
		var response *sip.Response
		gb.Info("OnMessage", "error", "device/platform not found", "id", id)
		response = sip.NewResponseFromRequest(req, sip.StatusNotFound, "Not Found", nil)
		if err := tx.Respond(response); err != nil {
			gb.Error("respond NotFound", "error", err.Error())
		}
		gb.Debug("after on message respond")
		return
	}

	// 根据来源调用不同的处理方法
	if d != nil && d.Online {
		d.UpdateTime = time.Now()
		if err = d.onMessage(req, tx, temp); err != nil {
			gb.Error("onMessage", "error", err.Error(), "type", "device,deviceid is", d.DeviceId)
		}
	} else {
		if platform, ok := gb.platforms.Get(p.ServerGBID); !ok {
			gb.Error("OnMessage", "error", "platform not found", "id", p.ServerGBID)
			return
		} else if err = platform.OnMessage(req, tx, temp); err != nil {
			gb.Error("onMessage", "error", err.Error(), "type", "platform")
		}
	}
}

func (gb *GB28181Plugin) OnNotify(req *sip.Request, tx sip.ServerTransaction) {
	from := req.From()
	if from == nil || from.Address.User == "" {
		gb.Error("OnNotify", "error", "no user")
		return
	}
	id := from.Address.User

	// 检查消息来源，获取字符集配置
	var d *Device
	var p *gb28181.PlatformModel
	var charset string = "GB2312" // 默认字符集

	// 先从设备缓存中获取
	d, _ = gb.devices.Get(id)
	if d != nil && d.Charset != "" {
		charset = d.Charset
	}

	// 使用正确的字符集解析消息内容
	temp := &gb28181.Message{}
	err := gb28181.DecodeXML(temp, req.Body(), charset)
	gb.Debug("onnotify debug", "message", temp, "charset", charset)
	if err != nil {
		gb.Error("OnNotify", "error", err.Error(), "charset", charset)
		response := sip.NewResponseFromRequest(req, sip.StatusBadRequest, "Bad Request", nil)
		if err := tx.Respond(response); err != nil {
			gb.Error("respond BadRequest", "error", err.Error())
		}
		return
	}

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
		gb.Info("OnNotify", "error", "device/platform not found", "id", id)
		response = sip.NewResponseFromRequest(req, sip.StatusNotFound, "Not Found", nil)
		if err := tx.Respond(response); err != nil {
			gb.Error("respond NotFound", "error", err.Error())
		}
		gb.Debug("after on notify respond")
		return
	}

	// 根据来源调用不同的处理方法
	if d != nil {
		d.UpdateTime = time.Now()
		if err = d.onNotify(req, tx, temp); err != nil {
			gb.Error("onNotify", "error", err.Error(), "type", "device")
		}
	} else {
		//var platform *Platform
		//if platformtmp, ok := gb.platforms.Get(p.ServerGBID); !ok {
		//	// 创建 Platform 实例
		//	platform = NewPlatform(p, gb, false)
		//} else {
		//	platform = platformtmp
		//}
		//if err = platform.OnNotify(req, tx, temp); err != nil {
		//	gb.Error("onNotify", "error", err.Error(), "type", "platform")
		//}
	}

	// 发送200 OK响应
	response := sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil)
	if err := tx.Respond(response); err != nil {
		gb.Error("OnNotify", "error", "send response failed", "err", err.Error())
		return
	}
}

func (gb *GB28181Plugin) Pull(streamPath string, conf config.Pull, pubConf *config.Publish) (job *m7s.PullJob, err error) {
	if util.Exist(conf.URL) {
		var puller mrtp.DumpPuller
		job = puller.GetPullJob()
		job.Init(&puller, &gb.Plugin, streamPath, conf, pubConf)
		return
	}
	dialog := Dialog{
		gb: gb,
	}
	dialog.Logger = gb.Logger.With("streamPath", streamPath, "conf.URL", conf.URL)
	if conf.Args != nil {
		if conf.Args.Get(util.StartKey) != "" || conf.Args.Get(util.EndKey) != "" {
			dialog.start = conf.Args.Get(util.StartKey)
			dialog.end = conf.Args.Get(util.EndKey)
		}
		if conf.Args.Get("stream") != "" {
			dialog.stream = conf.Args.Get("stream")
		}
	}
	job = dialog.GetPullJob()
	job.Init(&dialog, &gb.Plugin, streamPath, conf, pubConf)
	return
}

func (gb *GB28181Plugin) GetPullableList() []string {
	return slices.Collect(func(yield func(string) bool) {
		for d := range gb.devices.Range {
			for c := range d.channels.Range {
				if c.Status == gb28181.ChannelOnStatus {
					yield(fmt.Sprintf("%s/%s", d.DeviceId, c.ChannelId))
				}
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
			gb.Error("forwarddialog bye", "error", err)
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
	if platformTmp, platformFound := gb.platforms.Get(inviteInfo.RequesterId); !platformFound {
		gb.Error("OnInvite", "error", "platform found in DB but not in runtime", "platformId", inviteInfo.RequesterId)
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusNotFound, "Platform Not Found In Runtime", nil))
		return
	} else {
		platform = platformTmp
	}

	gb.Debug("OnInvite", "action", "platform found", "platformId", inviteInfo.RequesterId, "platformName", platform.PlatformModel.Name)
	var channel *Channel
	platform.channels.Range(func(channelTmp *Channel) bool {
		if channelTmp.CustomChannelId == inviteInfo.TargetChannelId {
			channel = channelTmp
		}
		return true
	})

	gb.Info("OnInvite", "action", "channel found", "channel.ChannelId", channel.ChannelId, "channel.CustomChannelId", channel.CustomChannelId, "channelName", channel.Name)

	// 通道存在，发送100 Trying响应
	tryingResp := sip.NewResponseFromRequest(req, sip.StatusTrying, "Trying", nil)
	if err := tx.Respond(tryingResp); err != nil {
		gb.Error("OnInvite", "error", "send trying response failed", "err", err.Error())
		return
	}

	// 检查SSRC
	if inviteInfo.SSRC == 0 {
		gb.Error("OnInvite", "error", "ssrc not found in invite")
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusBadRequest, "SSRC Not Found", nil))
		return
	}

	// 获取媒体信息
	mediaPort := uint16(0)
	if inviteInfo.StreamMode != mrtp.StreamModeTCPPassive {
		if gb.MediaPort.Valid() {
			var ok bool
			mediaPort, ok = gb.tcpPB.Allocate()
			if !ok {
				gb.Error("OnInvite", "error", "no available port")
				_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusServiceUnavailable, "No Available Port", nil))
				return
			}
			gb.Debug("OnInvite", "action", "allocate port", "port", mediaPort)
		} else {
			mediaPort = gb.MediaPort[0]
			gb.Debug("OnInvite", "action", "use default port", "port", mediaPort)
		}
	}

	// 构建SDP响应
	// 使用平台和通道的信息构建响应
	sdpIP := platform.PlatformModel.DeviceIP
	// 如果平台配置了SendStreamIP，则使用此IP
	if platform.PlatformModel.SendStreamIp != "" {
		sdpIP = platform.PlatformModel.SendStreamIp
	}

	// 构建SDP内容，参考Java代码createSendSdp方法
	content := []string{
		"v=0",
		fmt.Sprintf("o=%s 0 0 IN IP4 %s", channel.ChannelId, sdpIP),
		fmt.Sprintf("s=%s", inviteInfo.SessionName),
		fmt.Sprintf("c=IN IP4 %s", sdpIP),
	}

	// 处理播放时间
	if strings.EqualFold("Playback", inviteInfo.SessionName) && inviteInfo.StartTime > 0 && inviteInfo.StopTime > 0 {
		content = append(content, fmt.Sprintf("t=%d %d", inviteInfo.StartTime, inviteInfo.StopTime))
	} else {
		content = append(content, "t=0 0")
	}

	switch inviteInfo.StreamMode {
	case mrtp.StreamModeTCPActive:
		content = append(content, fmt.Sprintf("m=video %d TCP/RTP/AVP 96", mediaPort))
		content = append(content, "a=setup:passive")
		content = append(content, "a=connection:new")
	case mrtp.StreamModeTCPPassive:
		content = append(content, fmt.Sprintf("m=video %d TCP/RTP/AVP 96", mediaPort))
		content = append(content, "a=setup:active")
		content = append(content, "a=connection:new")
	case mrtp.StreamModeUDP:
		content = append(content, fmt.Sprintf("m=video %d RTP/AVP 96", mediaPort))
	}

	// 添加其他属性，参考Java代码
	content = append(content,
		"a=sendonly",
		"a=rtpmap:96 PS/90000",
		fmt.Sprintf("y=%s", strconv.FormatUint(uint64(inviteInfo.SSRC), 10)),
		"f=",
	)

	// 发送200 OK响应
	response := sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil)
	contentType := sip.ContentTypeHeader("application/sdp")
	response.AppendHeader(&contentType)
	response.SetBody([]byte(strings.Join(content, "\r\n") + "\r\n"))
	var ip = ""
	var streamMode mrtp.StreamMode
	if channel.StreamPath == "" {
		ip = channel.Device.MediaIp
		streamMode = channel.Device.StreamMode
	}
	// 创建并保存SendRtpInfo，以供OnAck方法使用
	forwardDialog := &ForwardDialog{
		gb:             gb,
		platformCallId: req.CallID().Value(),
		platformSSRC:   inviteInfo.SSRC,
		start:          inviteInfo.StartTime,
		end:            inviteInfo.StopTime,
		channel:        channel,
		// 初始化 ForwardConfig
		ForwardConfig: mrtp.ForwardConfig{
			Source: mrtp.ConnectionConfig{
				//IP:   util.Conditional(channel.StreamPath != "", "", channel.Device.MediaIp),    // 将在 Run 方法中从 SDP 响应中获取
				IP:   ip, // 将在 Run 方法中从 SDP 响应中获取
				Port: 0,  // 将在 Run 方法中从 SDP 响应中获取
				//Mode: util.Conditional(channel.StreamPath != "", "", channel.Device.StreamMode), // 默认值，将在 Run 方法中根据 StreamMode 更新
				Mode: streamMode, // 默认值，将在 Run 方法中根据 StreamMode 更新
				SSRC: 0,          // 将在 Start 方法中设置
			},
			Target: mrtp.ConnectionConfig{
				IP:   inviteInfo.IP,
				Port: inviteInfo.Port,
				Mode: inviteInfo.StreamMode, // 默认值，将在 Run 方法中根据 StreamMode 更新
				SSRC: inviteInfo.SSRC,       // 将在 Run 方法中从 platformSSRC 解析
			},
			Relay: false,
		},
	}
	forwardDialog.Logger = gb.Logger.With("ssrc", inviteInfo.SSRC, "platformid", platform.PlatformModel.ServerGBID, "deviceid", channel.ID)
	gb.forwardDialogs.Set(forwardDialog)
	gb.Info("OnInvite", "action", "sendRtpInfo created", "callId", req.CallID().Value())

	if err := tx.Respond(response); err != nil {
		gb.Error("OnInvite", "error", "send response failed", "err", err.Error())
		return
	}

	gb.Info("OnInvite", "action", "complete", "platformId", inviteInfo.RequesterId, "channelId", channel.ChannelId,
		"ip", inviteInfo.IP, "port", inviteInfo.Port, "StreamMode", inviteInfo.StreamMode)
	return
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
		if forwardDialog.channel.StreamPath == "" { //为空表示是正常的GB设备
			pullUrl := fmt.Sprintf("%s/%s", util.Conditional(forwardDialog.channel.DeviceId == "", forwardDialog.channel.ParentId, forwardDialog.channel.DeviceId), forwardDialog.channel.ChannelId)
			streamPath := fmt.Sprintf("platform_%d/%s/%s", time.Now().UnixMilli(), forwardDialog.channel.DeviceId, forwardDialog.channel.ChannelId)

			// 创建配置
			pullConf := config.Pull{
				URL: pullUrl,
			}
			// 初始化拉流任务
			forwardDialog.GetPullJob().Init(forwardDialog, &gb.Plugin, streamPath, pullConf, nil)
		} else { //不为空表示是个拉流代理相关联的设备，直接推送已有的流
			// 异步推送PS流到上级平台
			go gb.sendPSToUpstream(forwardDialog)
		}
	} else {
		gb.Error("OnAck", "error", "forwardDialog not found", "callID", callID)
		return
	}
}

// sendPSToUpstream 将拉流代理的流转换为PS格式并推送到上级平台
func (gb *GB28181Plugin) sendPSToUpstream(forwardDialog *ForwardDialog) {
	streamPath := forwardDialog.channel.StreamPath
	targetIP := forwardDialog.ForwardConfig.Target.IP
	targetPort := forwardDialog.ForwardConfig.Target.Port
	isUDP := forwardDialog.ForwardConfig.Target.Mode == mrtp.StreamModeUDP
	ssrc := forwardDialog.ForwardConfig.Target.SSRC

	// 订阅流 - 使用gb作为context
	suber, err := gb.Subscribe(gb, streamPath)
	if err != nil {
		gb.Error("sendPSToUpstream", "error", "subscribe stream failed", "err", err, "streamPath", streamPath)
		return
	}

	var w io.WriteCloser
	var writeRTP func() error
	var mem gomem.RecyclableMemory
	allocator := gomem.NewScalableMemoryAllocator(1 << gomem.MinPowerOf2)
	mem.SetAllocator(allocator)
	defer allocator.Recycle()
	var headerBuf [14]byte
	writeBuffer := make(net.Buffers, 1)
	var totalBytesSent int
	var packet rtp.Packet
	packet.Version = 2
	packet.SSRC = ssrc
	packet.PayloadType = 96
	defer func() {
		gb.Info("sendPSToUpstream", "action", "complete", "total", packet.SequenceNumber, "totalBytesSent", totalBytesSent)
	}()

	if isUDP {
		// UDP模式
		conn, err := net.DialUDP("udp", nil, &net.UDPAddr{
			IP:   net.ParseIP(targetIP),
			Port: int(targetPort),
		})
		if err != nil {
			gb.Error("sendPSToUpstream", "error", "dial udp failed", "err", err)
			return
		}
		w = conn
		writeRTP = func() (err error) {
			defer mem.Recycle()
			r := mem.NewReader()
			packet.Timestamp = uint32(time.Now().UnixMilli()) * 90
			for r.Length > 0 {
				packet.SequenceNumber += 1
				buf := writeBuffer
				buf[0] = headerBuf[:12]
				_, err = packet.Header.MarshalTo(headerBuf[:12])
				if err != nil {
					return
				}
				r.RangeN(mrtp.MTUSize, func(b []byte) {
					buf = append(buf, b)
				})
				n, _ := buf.WriteTo(w)
				totalBytesSent += int(n)
			}
			return
		}
	} else {
		// TCP模式
		gb.Info("sendPSToUpstream", "action", "connect tcp", "ip", targetIP, "port", targetPort)
		conn, err := net.DialTCP("tcp", nil, &net.TCPAddr{
			IP:   net.ParseIP(targetIP),
			Port: int(targetPort),
		})
		if err != nil {
			gb.Error("sendPSToUpstream", "error", "dial tcp failed", "err", err)
			return
		}
		w = conn
		writeRTP = func() (err error) {
			defer mem.Recycle()
			r := mem.NewReader()
			packet.Timestamp = uint32(time.Now().UnixMilli()) * 90

			// 检查是否需要分割成多个RTP包
			const maxRTPSize = 65535 - 12 // uint16最大值减去RTP头部长度

			for r.Length > 0 {
				buf := writeBuffer
				buf[0] = headerBuf[:14]
				packet.SequenceNumber += 1

				// 计算当前包的有效载荷大小
				payloadSize := r.Length
				if payloadSize > maxRTPSize {
					payloadSize = maxRTPSize
				}

				// 设置TCP长度字段 (2字节) + RTP头部长度 (12字节) + 载荷长度
				rtpPacketSize := uint16(12 + payloadSize)
				binary.BigEndian.PutUint16(headerBuf[:2], rtpPacketSize)

				// 生成RTP头部
				_, err = packet.Header.MarshalTo(headerBuf[2:14])
				if err != nil {
					return
				}

				// 添加载荷数据
				r.RangeN(payloadSize, func(b []byte) {
					buf = append(buf, b)
				})

				// 发送RTP包
				n, writeErr := buf.WriteTo(w)
				if writeErr != nil {
					return writeErr
				}
				totalBytesSent += int(n)
			}
			return
		}
	}
	defer w.Close()

	// 创建PS封装器
	var muxer mpegps.MpegPSMuxer
	muxer.Subscriber = suber
	muxer.Packet = &mem
	muxer.Mux(writeRTP)

	gb.Info("sendPSToUpstream", "action", "stream ended", "streamPath", streamPath)
}
