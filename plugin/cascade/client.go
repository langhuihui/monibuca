package plugin_cascade

import (
	"crypto/tls"
	"fmt"
	"sort"
	"time"

	"m7s.live/v5"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/util"
	cascade "m7s.live/v5/plugin/cascade/pkg"

	task "github.com/langhuihui/gotask"
	"github.com/quic-go/quic-go"
)

// {{ AURA-X: M1.1 - 多服务器配置 }}
// ServerConfig 单个服务器配置（主备模式）
type ServerConfig struct {
	Name     string `json:"name" desc:"服务器名称"`
	Addr     string `json:"addr" desc:"服务器地址（仅IP）"`
	QuicPort int    `json:"quicPort" default:"44944" desc:"QUIC端口"`
	HttpPort int    `json:"httpPort" default:"8080" desc:"HTTP端口"`
	Secret   string `json:"secret" desc:"连接密钥"`
	Priority int    `json:"priority" desc:"优先级 (1=最高)"`
	Enabled  bool   `json:"enabled" default:"true" desc:"是否启用"`
}

// {{ AURA-X: GetKey 实现 util.Collection 的接口 }}
func (sc *ServerConfig) GetKey() string {
	return sc.GetQuicAddr()
}

// {{ AURA-X: GetQuicAddr 获取 QUIC 连接地址 }}
func (sc *ServerConfig) GetQuicAddr() string {
	return fmt.Sprintf("%s:%d", sc.Addr, sc.QuicPort)
}

// {{ AURA-X: GetHttpAddr 获取 HTTP 地址 }}
func (sc *ServerConfig) GetHttpAddr() string {
	return fmt.Sprintf("http://%s:%d", sc.Addr, sc.HttpPort)
}

// {{ AURA-X: M1.1 - 扩展插件配置，保留单服务器兼容 }}
type CascadeClientPlugin struct {
	m7s.Plugin
	RelayAPI cascade.RelayAPIConfig `desc:"访问控制"`
	AutoPush bool                   `desc:"自动推流到上级"` //自动推流到上级

	// {{ AURA-X: 原有单服务器配置（保持兼容） }}
	Server string `desc:"上级服务器"` // 单服务器模式
	Secret string `desc:"连接密钥"`  // 单服务器模式密钥

	// {{ AURA-X: 新增多服务器配置（主备模式）- 用于接收YAML配置 }}
	Servers []*ServerConfig `desc:"服务器配置列表"` // 主备模式配置

	// {{ AURA-X: M3.3 - 拉流代理同步配置 }}
	PullSyncInterval int `desc:"拉流代理同步间隔(秒)" default:"30"` // 定期同步拉流代理列表

	// {{ AURA-X: M4 - 录像同步配置 }}
	RecordSyncInterval int `desc:"录像同步间隔(秒)" default:"60"` // 定期同步录像列表

	// {{ AURA-X: 内部运行时集合 }}
	servers util.Collection[string, *ServerConfig] // 服务器配置集合（运行时）

	// {{ AURA-X: 内部组件 }}
	client  *CascadeClient                              // 单服务器模式使用
	clients task.WorkCollection[string, *CascadeClient] // 多服务器模式使用: Addr -> CascadeClient

	// {{ AURA-X: 客户端管理器（指针） }}
	clientManager *ClientManager // ClientManager，用于多服务器的状态管理

	// {{ AURA-X: 客户端停止事件通道，用于 CascadeClient.Dispose 通知 ClientManager }}
	clientStoppedCh chan<- *ServerConfig
}

var _ = m7s.InstallPlugin[CascadeClientPlugin](m7s.PluginMeta{
	NewPuller: cascade.NewCascadePuller,
})

// {{ AURA-X: M1.2 - 扩展客户端配置，添加服务器标识和密钥 }}
type CascadeClient struct {
	task.Work
	cfg        *CascadeClientPlugin
	serverID   uint32 // 服务器标识（1=主, 2=备）
	serverAddr string // QUIC连接地址（如 localhost:44944）
	secret     string // 连接密钥
	*quic.Conn
}

// {{ AURA-X: M1.2 - 实现 GetKey 接口，供 util.Collection 使用 }}
func (c *CascadeClient) GetKey() string {
	return c.serverAddr
}

// {{ AURA-X: Dispose 实现 task.Work 的 Dispose，在连接断开时被调用 }}
func (c *CascadeClient) Dispose() {
	// {{ AURA-X: 向 ClientManager 发送服务器停止的消息 }}
	if c.cfg.clientStoppedCh != nil {
		// {{ AURA-X: 从 servers collection 获取对应的 ServerConfig，使用 serverAddr (QuicAddr) 作为 key }}
		if serverCfg, ok := c.cfg.servers.Get(c.serverAddr); ok {
			c.cfg.clientStoppedCh <- serverCfg
		}
	}
}

// {{ AURA-X: onClientStopped 处理客户端停止事件，由 CascadeClient.Dispose 调用 }}
func (c *CascadeClientPlugin) onClientStopped(serverCfg *ServerConfig) {
	// {{ AURA-X: 预留接口，实际处理在 ClientManager 中 }}
}

func (task *CascadeClient) Start() (err error) {
	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"monibuca"},
	}
	// {{ AURA-X: 复用 QUIC KeepAlive (10秒) }}
	task.Conn, err = quic.DialAddr(task.cfg.Context, task.serverAddr, tlsConf, &quic.Config{
		KeepAlivePeriod: time.Second * 10,
		EnableDatagrams: true,
	})
	if err != nil {
		task.Error("quic.DialAddr", "connect error", err.Error(), "server", task.serverAddr)
		return
	}
	var stream *quic.Stream
	if stream, err = task.OpenStreamSync(task.cfg); err == nil {
		res := []byte{0}
		// {{ AURA-X: 获取对应服务器的 Secret }}
		secret := task.getSecret()
		fmt.Fprintf(stream, "%s", secret)
		stream.Write([]byte{0})
		_, err = stream.Read(res)
		if err == nil && res[0] == 0 {
			task.Info("connected to cascade server", "server", task.serverAddr, "id", task.serverID)
			stream.Close()
		} else {
			var zapErr any = err
			if err == nil {
				zapErr = res[0]
			}
			task.Error("connect to cascade server", "server", task.serverAddr, "err", zapErr)
			return err
		}
	}
	return
}

// {{ AURA-X: M1.3 - 获取 Secret（简化版） }}
func (task *CascadeClient) getSecret() string {
	return task.secret
}

func (task *CascadeClient) Go() (err error) {
	for err == nil {
		var s *quic.Stream
		if s, err = task.AcceptStream(task.Task.Context); err == nil {
			task.AddTask(&cascade.ReceiveRequestTask{
				Conn:    task.Conn,
				Stream:  s,
				Handler: task.cfg.GetGlobalCommonConf().GetHandler(task.Logger),
				Plugin:  &task.cfg.Plugin,
			})
		}
	}
	return
}

// {{ AURA-X: M1.3 - 双模式启动逻辑 }}
func (c *CascadeClientPlugin) Start() (err error) {
	c.AddTask(&c.clients)
	// {{ AURA-X: 模式1 - 单服务器模式（原有逻辑） }}
	if c.Server != "" && len(c.Servers) == 0 {
		if c.Secret == "" {
			c.Warn("secret is empty, skip connecting")
			return nil
		}
		c.Info("running in single server mode", "server", c.Server)
		return c.startSingleServer()
	}

	// {{ AURA-X: 模式2 - 主备模式（新增逻辑） }}
	if len(c.Servers) > 0 {
		c.Info("running in multi-server failover mode")
		return c.startMultiServer()
	}

	// {{ AURA-X: 无配置，不启动 }}
	c.Warn("no server configured, skip starting")
	return nil
}

// {{ AURA-X: M1.3 - 单服务器模式（复用原有逻辑） }}
func (c *CascadeClientPlugin) startSingleServer() (err error) {
	connectTask := CascadeClient{
		cfg:        c,
		serverAddr: c.Server,
		secret:     c.Secret,
	}
	connectTask.SetRetry(-1, time.Second)
	connectTask.SetDescription("serverAddr", c.Server)
	connectTask.SetDescription("secret", c.Secret)
	c.AddTask(&connectTask)
	c.client = &connectTask
	return
}

// {{ AURA-X: M1.4 - 多服务器模式（新增逻辑） }}
// 注意：这里只做配置检查，不启动 client，启动由 ClientManager 负责
func (c *CascadeClientPlugin) startMultiServer() (err error) {
	// {{ AURA-X: 检查 Secret 是否重复 }}
	secretMap := make(map[string]int) // secret -> 出现次数
	for i := range c.Servers {
		if c.Servers[i].Secret != "" {
			secretMap[c.Servers[i].Secret]++
		}
	}
	for secret, count := range secretMap {
		if count > 1 {
			c.Error("duplicate secret detected, failover requires unique secrets", "secret", secret)
			return fmt.Errorf("duplicate secret detected: %s (count: %d), please use unique secrets for each server", secret, count)
		}
	}

	// {{ AURA-X: 按优先级排序 }}
	sortedServers := make([]*ServerConfig, len(c.Servers))
	copy(sortedServers, c.Servers)
	sort.Slice(sortedServers, func(i, j int) bool {
		return sortedServers[i].Priority < sortedServers[j].Priority
	})

	// {{ AURA-X: 输出主备服务器信息 }}
	if len(sortedServers) > 0 {
		primary := sortedServers[0]
		c.Info("primary server", "name", primary.Name, "addr", primary.Addr)
		if len(sortedServers) > 1 {
			backup := sortedServers[1]
			c.Info("backup server", "name", backup.Name, "addr", backup.Addr)
		}
	}

	// {{ AURA-X: 转换为 util.Collection 供运行时使用 }}
	c.servers = util.Collection[string, *ServerConfig]{}
	for i := range sortedServers {
		c.servers.Set(sortedServers[i])
	}

	// {{ AURA-X: 创建 ServerRunner 用于追踪服务器在线状态 }}
	// {{ AURA-X: 使用插件自带的数据库连接 c.DB }}
	var serverRunner *cascade.ServerRunner
	if c.DB != nil {
		serverRunner = cascade.NewServerRunner(c.DB)
	}

	// {{ AURA-X: 启动 ClientManager，由它负责连接管理 }}
	c.clientManager = NewClientManager(c, serverRunner, c.DB)
	c.AddTask(c.clientManager)

	return nil
}

// {{ AURA-X: M1.5 - 支持多服务器拉流 }}
func (c *CascadeClientPlugin) Pull(streamPath string, conf config.Pull, pub *config.Publish) (job *m7s.PullJob, err error) {
	var conn *quic.Conn

	// {{ AURA-X: 多服务器模式：从当前运行的服务器获取连接 }}
	if c.clients.Length() > 0 {
		// {{ AURA-X: 从 ClientManager 获取当前运行的服务器地址 }}
		runningAddr := c.clientManager.GetRunningServer()
		if runningAddr != "" {
			if client, ok := c.clients.Get(runningAddr); ok {
				conn = client.Conn
			}
		}
	} else if c.client != nil {
		// {{ AURA-X: 单服务器模式：使用原有 client }}
		conn = c.client.Conn
	}

	if conn == nil {
		return nil, fmt.Errorf("no available server connection")
	}

	puller := &cascade.Puller{
		Conn: conn,
	}
	job = puller.GetPullJob()
	job.Init(puller, &c.Plugin, streamPath, conf, pub)
	return
}

//func (c *CascadeClientPlugin) Start() {
//	retryDelay := [...]int{2, 3, 5, 8, 13}
//	for i := 0; c.Err() == nil; i++ {
//		connected, err := c.Remote()
//		if err == nil {
//			//不需要重试了，服务器返回了错误
//			return
//		}
//		c.Error("connect to cascade server ", "server", c.Server, "err", err)
//		if connected {
//			i = 0
//		} else if i >= 5 {
//			i = 4
//		}
//		time.Sleep(time.Second * time.Duration(retryDelay[i]))
//	}
//}

//func (c *CascadeClientPlugin) Remote() (wasConnected bool, err error) {
//	tlsConf := &tls.Config{
//		InsecureSkipVerify: true,
//		NextProtos:         []string{"monibuca"},
//	}
//	c.conn, err = quic.DialAddr(c, c.Server, tlsConf, &quic.Config{
//		KeepAlivePeriod: time.Second * 10,
//		EnableDatagrams: true,
//	})
//	wasConnected = err == nil
//	if stream := quic.Stream(nil); err == nil {
//		if stream, err = c.conn.OpenStreamSync(c); err == nil {
//			res := []byte{0}
//			fmt.Fprintf(stream, "%s", c.Secret)
//			stream.Write([]byte{0})
//			_, err = stream.Read(res)
//			if err == nil && res[0] == 0 {
//				c.Info("connected to cascade server", "server", c.Server)
//				stream.Close()
//			} else {
//				var zapErr any = err
//				if err == nil {
//					zapErr = res[0]
//				}
//				c.Error("connect to cascade server", "server", c.Server, "err", zapErr)
//				return false, nil
//			}
//		}
//	}
//
//	for err == nil {
//		var quicHttp cascade.QuicHTTP
//		//quicHttp.RelayAPIConfig = &c.RelayAPI
//		err = quicHttp.Accept(c.conn, &c.Plugin)
//	}
//	return wasConnected, err
//}
