package plugin_cascade

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	"gorm.io/gorm"

	m7s "m7s.live/v5"
	cascadepkg "m7s.live/v5/plugin/cascade/pkg"

	task "github.com/langhuihui/gotask"
)

// {{ AURA-X: PullProxy 拉流代理数据结构 }}
type PullProxy struct {
	ID             uint   `json:"ID"`
	Name           string `json:"name"`
	Type           string `json:"type"`
	Status         int    `json:"status"`
	PullURL        string `json:"pullURL"`
	PullOnStart    bool   `json:"pullOnStart"`
	StopOnIdle     bool   `json:"stopOnIdle"`
	Audio          bool   `json:"audio"`
	Description    string `json:"description"`
	RecordPath     string `json:"recordPath"`
	RecordFragment string `json:"recordFragment"`
	StreamPath     string `json:"streamPath"`
}

// {{ AURA-X: PullProxyListResp 拉流代理列表响应 }}
type PullProxyListResp struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    []PullProxy `json:"data"`
}

// {{ AURA-X: RecordCatalog 录像目录响应 }}
type RecordCatalog struct {
	StreamPath string `json:"streamPath"`
	Count      int    `json:"count"`
	StartTime  string `json:"startTime"`
	EndTime    string `json:"endTime"`
}

type RecordCatalogResp struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    []RecordCatalog `json:"data"`
}

// {{ AURA-X: RecordFile 录像文件信息 }}
type RecordFile struct {
	Id         uint   `json:"id"`
	FilePath   string `json:"filePath"`
	StreamPath string `json:"streamPath"`
	StartTime  string `json:"startTime"`
	EndTime    string `json:"endTime"`
	Filename   string `json:"filename"`
	Type       string `json:"type"`
	Duration   uint32 `json:"duration"`
	AudioCodec string `json:"audioCodec"`
	VideoCodec string `json:"videoCodec"`
	CreatedAt  string `json:"createdAt"`
}

type RecordListResp struct {
	Code     int          `json:"code"`
	Message  string       `json:"message"`
	Total    int          `json:"total"`
	PageNum  int          `json:"pageNum"`
	PageSize int          `json:"pageSize"`
	Data     []RecordFile `json:"data"`
}

// {{ AURA-X: ClientManager 客户端管理器，用于监控多服务器连接状态 }}
// 嵌入 task.Job，使其成为一个后台任务
// 直接通过 clients 集合判断连接状态：存在=已连接，不存在=未连接
type ClientManager struct {
	task.Job
	plugin          *CascadeClientPlugin     // 级联客户端插件引用
	serverRunner    *cascadepkg.ServerRunner // 服务器运行状态追踪器
	checkInterval   time.Duration            // 状态检查间隔
	retryInterval   time.Duration            // 重试间隔
	clientStoppedCh chan *ServerConfig       // 客户端停止事件通道

	// {{ AURA-X: 当前运行状态 }}
	RunningServer    string // 当前运行的服务器地址（主机或备机）
	RunningPriority  int    // 当前运行服务器的优先级 (1=主机, 2+=备机)
	IsPrimaryRunning bool   // 主机是否在运行

	// {{ AURA-X: M3.3 - 拉流代理同步 }}
	syncInterval      time.Duration // 拉流代理同步间隔
	cachedProxies     []PullProxy   // 缓存的拉流代理列表（从当前运行服务器同步）
	httpClient        *http.Client  // HTTP客户端
	wasPrimaryRunning bool          // 主机之前是否在运行

	// {{ AURA-X: M4 - 录像同步 }}
	recordSyncInterval time.Duration // 录像同步间隔
	db                 *gorm.DB      // 数据库连接
}

// {{ AURA-X: NewClientManager 创建客户端管理器 }}
func NewClientManager(plugin *CascadeClientPlugin, serverRunner *cascadepkg.ServerRunner, db *gorm.DB) *ClientManager {
	cm := &ClientManager{
		plugin:        plugin,
		serverRunner:  serverRunner,
		db:            db,
		checkInterval: 5 * time.Second, // 默认5秒检查一次
		retryInterval: 3 * time.Second, // 默认3秒重试一次
	}

	// {{ AURA-X: M3.3 - 初始化拉流代理同步 }}
	// 从插件配置获取同步间隔，默认30秒
	syncInterval := plugin.PullSyncInterval
	if syncInterval <= 0 {
		syncInterval = 30 // 默认30秒
	}
	cm.syncInterval = time.Duration(syncInterval) * time.Second

	// {{ AURA-X: M4 - 初始化录像同步 }}
	// 从插件配置获取同步间隔，默认60秒
	recordSyncInterval := plugin.RecordSyncInterval
	if recordSyncInterval <= 0 {
		recordSyncInterval = 60 // 默认60秒
	}
	cm.recordSyncInterval = time.Duration(recordSyncInterval) * time.Second

	// 初始化HTTP客户端
	cm.httpClient = &http.Client{
		Timeout: 10 * time.Second,
	}

	// {{ AURA-X: 创建客户端停止事件通道 }}
	cm.clientStoppedCh = make(chan *ServerConfig, 10)

	// {{ AURA-X: 将 channel 传给 plugin，供 CascadeClient.Dispose 使用 }}
	plugin.clientStoppedCh = cm.clientStoppedCh

	return cm
}

// {{ AURA-X: Start 在插件启动时被调用，用于初始化和连接服务器 }}
func (cm *ClientManager) Start() (err error) {
	cm.plugin.Info("ClientManager starting")

	// 添加调试信息
	cm.SetDescription("checkInterval", cm.checkInterval)
	cm.SetDescription("retryInterval", cm.retryInterval)
	cm.SetDescription("syncInterval", cm.syncInterval)

	// {{ AURA-X: 尝试连接所有服务器 }}
	cm.connectAllServers()

	return nil
}

// {{ AURA-X: Go 实现了 task.Job 的 Go 方法，作为主循环 }}
func (cm *ClientManager) Go() (err error) {
	cm.plugin.Info("ClientManager started")

	// 创建定时器
	ticker := time.NewTicker(cm.checkInterval)
	syncTicker := time.NewTicker(cm.syncInterval)             // 拉流代理同步定时器
	recordSyncTicker := time.NewTicker(cm.recordSyncInterval) // 录像同步定时器
	defer ticker.Stop()
	defer syncTicker.Stop()
	defer recordSyncTicker.Stop()

	for {
		select {
		case <-cm.Job.Context.Done():
			// 收到停止信号
			cm.plugin.Info("ClientManager stopped")
			return nil
		case <-ticker.C:
			// 定时检查客户端状态，并处理重连
			cm.checkAndRetry()
		case <-syncTicker.C:
			// {{ AURA-X: M3.3 - 定时同步拉流代理 }}
			cm.syncPullProxies()
		case <-recordSyncTicker.C:
			// {{ AURA-X: M4 - 定时同步录像 }}
			cm.syncRecords()
		case event := <-cm.clientStoppedCh:
			// {{ AURA-X: 收到客户端停止事件 }}
			cm.onClientStopped(event)
		}
	}
}

// {{ AURA-X: onClientStopped 处理客户端停止事件 }}
func (cm *ClientManager) onClientStopped(serverCfg *ServerConfig) {
	cm.plugin.Warn("client stopped", "name", serverCfg.Name, "addr", serverCfg.Addr, "priority", serverCfg.Priority)

	// {{ AURA-X: 记录服务器离线到 ServerRunner }}
	if cm.serverRunner != nil {
		cm.serverRunner.RecordStop(serverCfg.GetQuicAddr())
	}

	// {{ AURA-X: M4 - 服务器离线时设置 DeleteAt }}
	cm.markRecordsAsDeleted(serverCfg.Addr, serverCfg.HttpPort, true)

	// {{ AURA-X: 判断是否是主服务器，触发故障转移 }}
	if serverCfg.Priority == 1 {
		// {{ AURA-X: 转换为 []ServerConfig }}
		servers := make([]ServerConfig, len(cm.plugin.Servers))
		for i, sc := range cm.plugin.Servers {
			servers[i] = *sc
		}
		cm.handlePrimaryFailure(servers)
	}
}

// {{ AURA-X: handlePrimaryRecovery 处理主机恢复 }}
// 当主机重新连接成功时，调用此方法将任务从备机切回主机
func (cm *ClientManager) handlePrimaryRecovery() {
	// 检查主机是否在运行
	if !cm.IsPrimaryRunning {
		return
	}

	// 找到主机地址
	var primaryAddr string
	for _, sc := range cm.plugin.Servers {
		if sc.Priority == 1 && sc.Enabled {
			primaryAddr = sc.GetQuicAddr()
			break
		}
	}

	if primaryAddr == "" {
		cm.plugin.Warn("handlePrimaryRecovery: primary server config not found")
		return
	}

	cm.plugin.Info("primary server recovered, pushing proxies back", "primary", primaryAddr)

	// {{ AURA-X: M3.3 - 将备机拉流代理推送到主机 }}
	cm.pushProxiesToPrimary(primaryAddr)
}

// {{ AURA-X: connectAllServers 尝试连接所有配置的服务器 }}
func (cm *ClientManager) connectAllServers() {
	// {{ AURA-X: 从 collection 中获取所有服务器并排序 }}
	var sortedServers []ServerConfig
	cm.plugin.servers.Range(func(cfg *ServerConfig) bool {
		sortedServers = append(sortedServers, *cfg)
		return true
	})

	if len(sortedServers) == 0 {
		cm.plugin.Warn("no servers configured")
		return
	}

	// 按优先级排序
	sort.Slice(sortedServers, func(i, j int) bool {
		return sortedServers[i].Priority < sortedServers[j].Priority
	})

	// {{ AURA-X: 遍历所有服务器并尝试连接 }}
	for i, serverCfg := range sortedServers {
		if !serverCfg.Enabled {
			cm.plugin.Info("server disabled, skip", "name", serverCfg.Name, "addr", serverCfg.Addr)
			continue
		}

		if serverCfg.Addr == "" {
			cm.plugin.Warn("server addr is empty, skip", "name", serverCfg.Name)
			continue
		}

		// 创建客户端
		client := &CascadeClient{
			cfg:        cm.plugin,
			serverID:   uint32(i + 1),
			serverAddr: serverCfg.GetQuicAddr(),
			secret:     serverCfg.Secret,
		}

		// 添加调试信息
		client.SetDescription("name", serverCfg.Name)
		client.SetDescription("addr", serverCfg.Addr)
		client.SetDescription("quicAddr", serverCfg.GetQuicAddr())
		client.SetDescription("priority", serverCfg.Priority)
		client.SetDescription("secret", serverCfg.Secret)

		// 添加到 WorkCollection
		task := cm.plugin.clients.AddTask(client, cm.plugin.Logger)
		cm.plugin.Info("connecting to server", "name", serverCfg.Name, "addr", serverCfg.Addr, "priority", serverCfg.Priority, "id", client.serverID, "serverAddr", client.serverAddr, "secret", client.secret)

		// {{ AURA-X: 等待连接结果 }}
		if err := task.WaitStarted(); err != nil {
			cm.plugin.Error("failed to connect to server", "name", serverCfg.Name, "addr", serverCfg.Addr, "error", err)
			cm.updateClientStatus(client, false)
			// 不需要手动 Stop，task 会自动停止
		} else {
			cm.plugin.Info("successfully connected to server", "name", serverCfg.Name, "addr", serverCfg.Addr)
			// {{ AURA-X: 更新客户端状态 }}
			cm.updateClientStatus(client, true)
			// {{ AURA-X: 连接成功，记录到 ServerRunner }}
			if cm.serverRunner != nil {
				cm.serverRunner.RecordStart(serverCfg.GetQuicAddr(), serverCfg.Priority)
			}
			// {{ AURA-X: 更新当前运行状态 }}
			cm.updateRunningStatus(serverCfg.GetQuicAddr(), serverCfg.Priority)
		}
	}

	// 输出主备服务器信息
	if len(sortedServers) > 0 {
		primary := sortedServers[0]
		cm.plugin.Info("primary server", "name", primary.Name, "addr", primary.Addr)
		if len(sortedServers) > 1 {
			backup := sortedServers[1]
			cm.plugin.Info("backup server", "name", backup.Name, "addr", backup.Addr)
		}
	}
}

// {{ AURA-X: checkAndRetry 检查所有客户端的连接状态，失败则重试 }}
// 直接通过 clients 集合判断：存在=已连接，不存在=未连接
func (cm *ClientManager) checkAndRetry() {
	// {{ AURA-X: 从 collection 中获取所有服务器并排序 }}
	var sortedServers []ServerConfig
	cm.plugin.servers.Range(func(cfg *ServerConfig) bool {
		sortedServers = append(sortedServers, *cfg)
		return true
	})

	if len(sortedServers) == 0 {
		return
	}

	// 按优先级排序
	sort.Slice(sortedServers, func(i, j int) bool {
		return sortedServers[i].Priority < sortedServers[j].Priority
	})

	// 遍历所有配置的服务器，检查是否需要重连
	for i, serverCfg := range sortedServers {
		if !serverCfg.Enabled || serverCfg.Addr == "" {
			continue
		}

		// 检查该服务器是否已连接（通过 clients 集合判断）
		if client, ok := cm.plugin.clients.Get(serverCfg.GetQuicAddr()); ok {
			// 存在，检查连接是否有效
			isConnected := client.Conn != nil && client.Task.Context.Err() == nil
			if isConnected {
				// 已连接，跳过
				continue
			}
		}

		// 不存在或连接无效，需要重试
		cm.plugin.Warn("server not connected, retrying", "name", serverCfg.Name, "addr", serverCfg.Addr, "serverAddr", serverCfg.GetQuicAddr())
		cm.retryConnect(serverCfg, uint32(i+1))
	}
}

// {{ AURA-X: retryConnect 重试连接指定服务器 }}
func (cm *ClientManager) retryConnect(serverCfg ServerConfig, serverID uint32) {
	// 检查是否已经在重试中（通过检查 clients 中是否存在且未断开）
	if client, ok := cm.plugin.clients.Get(serverCfg.GetQuicAddr()); ok {
		if client.Conn != nil && client.Task.Context.Err() == nil {
			// 已经连接上了
			return
		}
		// 任务存在但未连接，不需要手动 Stop，会自动清理
	}

	// 创建新客户端
	client := &CascadeClient{
		cfg:        cm.plugin,
		serverID:   serverID,
		serverAddr: serverCfg.GetQuicAddr(),
		secret:     serverCfg.Secret,
	}

	// 添加调试信息
	client.SetDescription("name", serverCfg.Name)
	client.SetDescription("addr", serverCfg.Addr)
	client.SetDescription("quicAddr", serverCfg.GetQuicAddr())
	client.SetDescription("priority", serverCfg.Priority)
	client.SetDescription("secret", serverCfg.Secret)
	client.SetDescription("connected", "false")

	// 添加到 WorkCollection
	task := cm.plugin.clients.AddTask(client, cm.plugin.Logger)
	cm.plugin.Info("retrying connection to server", "name", serverCfg.Name, "addr", serverCfg.Addr, "serverAddr", client.serverAddr)

	// 等待连接结果
	if err := task.WaitStarted(); err != nil {
		cm.plugin.Error("failed to retry connect to server", "name", serverCfg.Name, "addr", serverCfg.Addr, "error", err)
		cm.updateClientStatus(client, false)
		// 连接失败，任务会自动停止
	} else {
		cm.plugin.Info("successfully reconnected to server", "name", serverCfg.Name, "addr", serverCfg.Addr)
		// {{ AURA-X: 更新客户端状态 }}
		cm.updateClientStatus(client, true)
		// {{ AURA-X: 连接成功，记录到 ServerRunner }}
		if cm.serverRunner != nil {
			cm.serverRunner.RecordStart(serverCfg.GetQuicAddr(), serverCfg.Priority)
		}
		// {{ AURA-X: M4 - 服务器恢复时清除 deleted_at }}
		cm.markRecordsAsDeleted(serverCfg.Addr, serverCfg.HttpPort, false)
		// {{ AURA-X: 更新当前运行状态 }}
		cm.updateRunningStatus(serverCfg.GetQuicAddr(), serverCfg.Priority)

		// {{ AURA-X: M3.3 - 检查是否是主机恢复 }}
		// 如果主机之前不在线，现在连接成功了，需要把备机的拉流代理推送到主机
		cm.plugin.Info("successfully connected to server", "serverCfg.Priority", serverCfg.Priority, "cm.wasPrimaryRunning", cm.wasPrimaryRunning, "serverCfg.GetQuicAddr()", serverCfg.GetQuicAddr())
		if serverCfg.Priority == 1 && !cm.wasPrimaryRunning {
			cm.handlePrimaryRecovery()
		}
		// 更新主机运行状态
		cm.wasPrimaryRunning = cm.IsPrimaryRunning
	}
}

// {{ AURA-X: handlePrimaryFailure 处理主服务器故障 }}
func (cm *ClientManager) handlePrimaryFailure(sortedServers []ServerConfig) {
	// 查找可用的备份服务器
	backupAddr := cm.findAvailableBackup(sortedServers)
	if backupAddr == "" {
		cm.plugin.Warn("no available backup server")
		return
	}

	cm.plugin.Info("triggering failover to backup", "backup", backupAddr)

	// {{ AURA-X: 标记主机已故障，需要在主机恢复时执行恢复逻辑 }}
	cm.wasPrimaryRunning = false

	// {{ AURA-X: 更新 RunningServer 为备机地址 }}
	cm.RunningServer = backupAddr
	cm.RunningPriority = 2 // 备机优先级为2
	cm.IsPrimaryRunning = false
	cm.SetDescription("runningServer", cm.RunningServer)
	cm.SetDescription("runningPriority", cm.RunningPriority)
	cm.SetDescription("isPrimaryRunning", cm.IsPrimaryRunning)

	// {{ AURA-X: M3.3 - 执行拉流代理迁移 }}
	cm.transferProxiesToBackup(backupAddr)
}

// {{ AURA-X: findAvailableBackup 查找可用的备份服务器 }}
func (cm *ClientManager) findAvailableBackup(sortedServers []ServerConfig) string {
	for _, cfg := range sortedServers {
		if cfg.Priority == 1 {
			continue // 跳过主服务器
		}
		// 检查是否连接
		if client, ok := cm.plugin.clients.Get(cfg.GetQuicAddr()); ok {
			if client.Conn != nil && client.Task.Context.Err() == nil {
				return cfg.GetQuicAddr()
			}
		}
	}
	return ""
}

// {{ AURA-X: updateClientStatus 更新客户端状态信息 }}
func (cm *ClientManager) updateClientStatus(client *CascadeClient, connected bool) {
	if client == nil {
		return
	}
	client.SetDescription("connected", connected)
}

// {{ AURA-X: updateRunningStatus 更新当前运行状态 }}
func (cm *ClientManager) updateRunningStatus(serverAddr string, priority int) {
	// 如果当前没有运行中的服务器，或者新连接的服务器优先级更高（主>备），则更新
	if cm.RunningServer == "" || priority < cm.RunningPriority {
		// {{ AURA-X: M4 - 服务器恢复时清除 DeleteAt（先清除旧的，再设置新的）}}
		if cm.RunningServer != "" {
			// 获取旧的服务器配置
			oldCfg := cm.getServerConfigByAddr(cm.RunningServer)
			if oldCfg != nil {
				cm.markRecordsAsDeleted(oldCfg.Addr, oldCfg.HttpPort, false)
			}
		}

		cm.RunningServer = serverAddr
		cm.RunningPriority = priority
		cm.IsPrimaryRunning = (priority == 1)
		// 更新调试信息
		cm.SetDescription("runningServer", cm.RunningServer)
		cm.SetDescription("runningPriority", cm.RunningPriority)
		cm.SetDescription("isPrimaryRunning", cm.IsPrimaryRunning)
		// 添加更详细的服务器信息
		if cfg, ok := cm.plugin.servers.Get(serverAddr); ok {
			cm.SetDescription("runningServerName", cfg.Name)
			cm.SetDescription("runningServerQuicPort", cfg.QuicPort)
			cm.SetDescription("runningServerHttpPort", cfg.HttpPort)
			cm.SetDescription("runningServerSecret", cfg.Secret)
		}
		cm.plugin.Info("running status updated", "server", cm.RunningServer, "priority", cm.RunningPriority, "isPrimary", cm.IsPrimaryRunning)
	}
}

// {{ AURA-X: clearRunningStatus 清除运行状态 }}
func (cm *ClientManager) clearRunningStatus(serverAddr string) {
	if cm.RunningServer == serverAddr {
		cm.RunningServer = ""
		cm.RunningPriority = 0
		cm.IsPrimaryRunning = false

		// 尝试切换到其他可用的服务器
		cm.trySwitchToAvailableServer()
	}
}

// {{ AURA-X: trySwitchToAvailableServer 尝试切换到其他可用服务器 }}
func (cm *ClientManager) trySwitchToAvailableServer() {
	// 从 clients 中找到任意一个在线的服务器
	cm.plugin.clients.Range(func(client *CascadeClient) bool {
		if client.Conn != nil && client.Task.Context.Err() == nil {
			// 获取该服务器的优先级
			if cfg, ok := cm.plugin.servers.Get(client.serverAddr); ok {
				cm.RunningServer = client.serverAddr
				cm.RunningPriority = cfg.Priority
				cm.IsPrimaryRunning = (cfg.Priority == 1)
				// 更新调试信息
				cm.SetDescription("runningServer", cm.RunningServer)
				cm.SetDescription("runningPriority", cm.RunningPriority)
				cm.SetDescription("isPrimaryRunning", cm.IsPrimaryRunning)
				// 添加更详细的服务器信息
				cm.SetDescription("runningServerName", cfg.Name)
				cm.SetDescription("runningServerQuicPort", cfg.QuicPort)
				cm.SetDescription("runningServerHttpPort", cfg.HttpPort)
				cm.SetDescription("runningServerSecret", cfg.Secret)
				cm.plugin.Info("switched to available server", "server", cm.RunningServer, "priority", cm.RunningPriority, "isPrimary", cm.IsPrimaryRunning)
				return false // 找到第一个在线的服务器就退出
			}
		}
		return true
	})
}

// {{ AURA-X: GetRunningServer 获取当前运行的服务器地址 }}
func (cm *ClientManager) GetRunningServer() string {
	return cm.RunningServer
}

// {{ AURA-X: GetRunningPriority 获取当前运行服务器的优先级 }}
func (cm *ClientManager) GetRunningPriority() int {
	return cm.RunningPriority
}

// {{ AURA-X: IsPrimaryRunning 检查主机是否在运行 }}
func (cm *ClientManager) IsPrimaryRunningCheck() bool {
	return cm.IsPrimaryRunning
}

// ============================================
// M3.3 拉流代理同步功能
// ============================================

// {{ AURA-X: syncPullProxies 定时同步拉流代理列表 }}
func (cm *ClientManager) syncPullProxies() {
	// 检查是否有运行中的服务器
	if cm.RunningServer == "" {
		return
	}

	// 获取当前运行服务器的HTTP地址
	serverCfg := cm.getServerConfigByAddr(cm.RunningServer)
	if serverCfg == nil {
		cm.plugin.Warn("syncPullProxies: server config not found", "addr", cm.RunningServer)
		return
	}

	httpAddr := serverCfg.GetHttpAddr()
	url := httpAddr + "/api/proxy/pull/list"

	// 发送HTTP请求
	resp, err := cm.httpClient.Get(url)
	if err != nil {
		cm.plugin.Error("syncPullProxies: failed to get pull proxy list", "url", url, "error", err)
		return
	}
	defer resp.Body.Close()

	// 解析响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		cm.plugin.Error("syncPullProxies: failed to read response", "error", err)
		return
	}

	var result PullProxyListResp
	if err := json.Unmarshal(body, &result); err != nil {
		cm.plugin.Error("syncPullProxies: failed to unmarshal response", "error", err)
		return
	}

	if result.Code != 0 {
		cm.plugin.Error("syncPullProxies: API returned error", "code", result.Code, "message", result.Message)
		return
	}

	// 缓存拉流代理列表
	cm.cachedProxies = result.Data
	cm.plugin.Info("synced pull proxies", "count", len(result.Data), "from", cm.RunningServer)
}

// {{ AURA-X: getServerConfigByAddr 根据地址获取服务器配置 }}
func (cm *ClientManager) getServerConfigByAddr(addr string) *ServerConfig {
	// 从 servers 集合中查找
	if cfg, ok := cm.plugin.servers.Get(addr); ok {
		return cfg
	}

	// 尝试用完整地址查找（Addr:Port）
	for _, sc := range cm.plugin.Servers {
		if sc.GetQuicAddr() == addr {
			return sc
		}
	}

	return nil
}

// {{ AURA-X: transferProxiesToBackup 将拉流代理迁移到备机 }}
// 当主机失效时，用缓存的拉流代理列表在备机上添加
func (cm *ClientManager) transferProxiesToBackup(backupAddr string) {
	if len(cm.cachedProxies) == 0 {
		cm.plugin.Info("transferProxiesToBackup: no cached proxies to transfer")
		return
	}

	// 获取备机配置
	backupCfg := cm.getServerConfigByAddr(backupAddr)
	if backupCfg == nil {
		cm.plugin.Error("transferProxiesToBackup: backup server config not found", "addr", backupAddr)
		return
	}

	httpAddr := backupCfg.GetHttpAddr()

	// 1. 清空备机所有拉流代理
	cm.clearAllProxies(httpAddr)

	// 2. 逐个添加拉流代理
	for _, proxy := range cm.cachedProxies {
		if err := cm.addPullProxy(httpAddr, &proxy); err != nil {
			cm.plugin.Error("transferProxiesToBackup: failed to add proxy", "streamPath", proxy.StreamPath, "error", err)
		}
	}

	cm.plugin.Info("transferred proxies to backup", "count", len(cm.cachedProxies), "backup", backupAddr)
}

// {{ AURA-X: pushProxiesToPrimary 将拉流代理推送到主机 }}
// 当主机恢复时，获取备机拉流代理列表推送到主机，并清空备机
func (cm *ClientManager) pushProxiesToPrimary(primaryAddr string) {
	// 获取备机地址（当前运行的不是主机的那个）
	backupAddr := cm.getBackupServerAddr(primaryAddr)
	if backupAddr == "" {
		cm.plugin.Warn("pushProxiesToPrimary: no backup server found")
		return
	}

	backupCfg := cm.getServerConfigByAddr(backupAddr)
	if backupCfg == nil {
		cm.plugin.Error("pushProxiesToPrimary: backup server config not found")
		return
	}

	// 1. 获取备机拉流代理列表
	backupProxies := cm.getPullProxyList(backupCfg.GetHttpAddr())
	if len(backupProxies) == 0 {
		cm.plugin.Info("pushProxiesToPrimary: no proxies on backup server")
		return
	}

	// 2. 清空主机所有拉流代理
	primaryCfg := cm.getServerConfigByAddr(primaryAddr)
	if primaryCfg == nil {
		cm.plugin.Error("pushProxiesToPrimary: primary server config not found")
		return
	}
	cm.clearAllProxies(primaryCfg.GetHttpAddr())

	// 3. 推送到主机
	for _, proxy := range backupProxies {
		if err := cm.addPullProxy(primaryCfg.GetHttpAddr(), &proxy); err != nil {
			cm.plugin.Error("pushProxiesToPrimary: failed to add proxy", "streamPath", proxy.StreamPath, "error", err)
		}
	}

	// 4. 清空备机
	cm.clearAllProxies(backupCfg.GetHttpAddr())

	cm.plugin.Info("pushed proxies to primary", "count", len(backupProxies), "primary", primaryAddr)
}

// {{ AURA-X: getBackupServerAddr 获取备机地址（完整的QUIC地址） }}
func (cm *ClientManager) getBackupServerAddr(primaryAddr string) string {
	for _, sc := range cm.plugin.Servers {
		if sc.GetQuicAddr() != primaryAddr && sc.Enabled {
			// 检查是否连接
			if client, ok := cm.plugin.clients.Get(sc.GetQuicAddr()); ok {
				if client.Conn != nil && client.Task.Context.Err() == nil {
					return sc.GetQuicAddr()
				}
			}
		}
	}
	return ""
}

// {{ AURA-X: clearAllProxies 清空指定服务器所有拉流代理 }}
func (cm *ClientManager) clearAllProxies(httpAddr string) {
	// 先获取列表
	cm.Debug("clearAllProxies", "httpAddr", httpAddr)
	proxies := cm.getPullProxyList(httpAddr)
	if len(proxies) == 0 {
		return
	}

	// 逐个删除
	for _, proxy := range proxies {
		if err := cm.removePullProxy(httpAddr, proxy.StreamPath); err != nil {
			cm.plugin.Error("clearAllProxies: failed to remove proxy", "streamPath", proxy.StreamPath, "error", err)
		}
	}

	cm.plugin.Info("cleared all proxies", "count", len(proxies), "server", httpAddr)
}

// {{ AURA-X: getPullProxyList 获取拉流代理列表 }}
func (cm *ClientManager) getPullProxyList(httpAddr string) []PullProxy {
	url := httpAddr + "/api/proxy/pull/list"

	resp, err := cm.httpClient.Get(url)
	if err != nil {
		cm.plugin.Error("getPullProxyList: failed to get list", "url", url, "error", err)
		return nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		cm.plugin.Error("getPullProxyList: failed to read response", "error", err)
		return nil
	}

	var result PullProxyListResp
	if err := json.Unmarshal(body, &result); err != nil {
		cm.plugin.Error("getPullProxyList: failed to unmarshal", "error", err)
		return nil
	}

	return result.Data
}

// {{ AURA-X: addPullProxy 添加拉流代理 }}
func (cm *ClientManager) addPullProxy(httpAddr string, proxy *PullProxy) error {
	url := httpAddr + "/api/proxy/pull/add"

	// 构建请求体（只传必要字段）
	data := map[string]interface{}{
		"name":           proxy.Name,
		"type":           proxy.Type,
		"pullURL":        proxy.PullURL,
		"pullOnStart":    proxy.PullOnStart,
		"stopOnIdle":     proxy.StopOnIdle,
		"audio":          proxy.Audio,
		"description":    proxy.Description,
		"recordPath":     proxy.RecordPath,
		"recordFragment": proxy.RecordFragment,
		"streamPath":     proxy.StreamPath,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	resp, err := cm.httpClient.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		cm.plugin.Error("addPullProxy: failed", "status", resp.StatusCode, "body", string(body))
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	return nil
}

// {{ AURA-X: removePullProxy 删除拉流代理 }}
func (cm *ClientManager) removePullProxy(httpAddr string, streamPath string) error {
	// id=0 时，在 body 中传 streamPath
	url := httpAddr + "/api/proxy/pull/remove/0"

	data := map[string]string{
		"streamPath": streamPath,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := cm.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

// ============================================
// M4 录像同步功能
// ============================================

// {{ AURA-X: syncRecords 定时同步录像列表 }}
func (cm *ClientManager) syncRecords() {
	// 检查数据库是否可用
	if cm.db == nil {
		return
	}

	// 检查是否有运行中的服务器
	if cm.RunningServer == "" {
		return
	}

	// 获取当前运行服务器的HTTP地址
	serverCfg := cm.getServerConfigByAddr(cm.RunningServer)
	if serverCfg == nil {
		cm.plugin.Warn("syncRecords: server config not found", "addr", cm.RunningServer)
		return
	}

	httpAddr := serverCfg.GetHttpAddr()

	// 1. 获取录像目录
	catalog, err := cm.getRecordCatalog(httpAddr)
	if err != nil {
		cm.plugin.Error("syncRecords: failed to get record catalog", "error", err)
		return
	}

	if len(catalog) == 0 {
		cm.plugin.Info("syncRecords: no records found")
		return
	}

	// 2. 对每个 streamPath 获取录像列表并插入数据库
	for _, cat := range catalog {
		// 查询本地数据库该 streamPath 的最大时间
		var maxEndTime time.Time
		var record m7s.RecordStream
		if err := cm.db.Where("stream_path = ?", cat.StreamPath).Order("end_time DESC").First(&record).Error; err == nil {
			maxEndTime = record.EndTime
		}

		// 确定开始时间
		var startTime time.Time
		if !maxEndTime.IsZero() {
			// 有本地数据，从最大时间开始
			startTime = maxEndTime
		} else {
			// 无本地数据，从 catalog 返回的开始时间
			startTime, _ = time.Parse(time.RFC3339, cat.StartTime)
		}

		// 结束时间为当前时间
		endTime := time.Now()

		// 调用 API 获取录像列表
		records, err := cm.getRecordList(httpAddr, cat.StreamPath, startTime, endTime)
		if err != nil {
			cm.plugin.Error("syncRecords: failed to get record list", "streamPath", cat.StreamPath, "error", err)
			continue
		}

		// 3. 插入到本地数据库
		for _, rec := range records {
			// 转换时间
			startTimeRec, _ := time.Parse(time.RFC3339, rec.StartTime)
			endTimeRec, _ := time.Parse(time.RFC3339, rec.EndTime)
			createdAtRec, _ := time.Parse(time.RFC3339, rec.CreatedAt)

			// 转换 FilePath 为 HTTP URL
			// 格式：http://{服务器IP}:{httpPort}/mp4/download/{streamPath}.mp4?id={id}
			filePath := fmt.Sprintf("http://%s:%d/mp4/download/%s.mp4?id=%d",
				serverCfg.Addr, serverCfg.HttpPort, rec.StreamPath, rec.Id)

			// 判重：检查是否已存在
			var existingRecord m7s.RecordStream
			if err := cm.db.Where("stream_path = ? AND start_time = ? AND end_time = ?",
				rec.StreamPath, startTimeRec, endTimeRec).First(&existingRecord).Error; err == nil {
				// 已存在，跳过
				continue
			}

			// 插入新记录
			newRecord := m7s.RecordStream{
				StreamPath: rec.StreamPath,
				FilePath:   filePath,
				Filename:   rec.Filename,
				Type:       rec.Type,
				StartTime:  startTimeRec,
				EndTime:    endTimeRec,
				Duration:   rec.Duration,
				AudioCodec: rec.AudioCodec,
				VideoCodec: rec.VideoCodec,
				CreatedAt:  createdAtRec,
			}

			if err := cm.db.Create(&newRecord).Error; err != nil {
				cm.plugin.Error("syncRecords: failed to insert record", "streamPath", rec.StreamPath, "error", err)
			}
		}

		cm.plugin.Info("synced records", "streamPath", cat.StreamPath, "count", len(records), "from", cm.RunningServer)
	}
}

// {{ AURA-X: getRecordCatalog 获取录像目录 }}
func (cm *ClientManager) getRecordCatalog(httpAddr string) ([]RecordCatalog, error) {
	url := httpAddr + "/api/record/mp4/catalog"

	resp, err := cm.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result RecordCatalogResp
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("API error: %s", result.Message)
	}

	return result.Data, nil
}

// {{ AURA-X: getRecordList 获取录像列表 }}
// startTime 和 endTime 是 Go 的 time.Time，会转换为时间戳
func (cm *ClientManager) getRecordList(httpAddr string, streamPath string, startTime time.Time, endTime time.Time) ([]RecordFile, error) {
	// 将时间转换为时间戳
	startTs := startTime.Unix()
	endTs := endTime.Unix()

	url := fmt.Sprintf("%s/api/record/mp4/list/%s?range=%d~%d", httpAddr, streamPath, startTs, endTs)

	resp, err := cm.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result RecordListResp
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("API error: %s", result.Message)
	}

	return result.Data, nil
}

// {{ AURA-X: markRecordsAsDeleted 设置/清除记录的 DeleteAt }}
// ip: 服务器 IP
// httpPort: 服务器 HTTP 端口
// deleted: true=设置 DeleteAt 为当前时间, false=清除 DeleteAt
func (cm *ClientManager) markRecordsAsDeleted(ip string, httpPort int, deleted bool) {
	if cm.db == nil {
		return
	}

	// 构建搜索条件：FilePath LIKE 'http://{IP}:{httpPort}/%'
	prefix := fmt.Sprintf("http://%s:%d/", ip, httpPort)

	if deleted {
		// 设置 DeleteAt 为当前时间
		result := cm.db.Unscoped().Model(&m7s.RecordStream{}).
			Where("file_path LIKE ?", prefix+"%").
			Where("deleted_at IS NULL").
			Update("deleted_at", time.Now())
		if result.Error != nil {
			cm.plugin.Error("markRecordsAsDeleted: failed to set deleted_at", "error", result.Error)
		} else if result.RowsAffected > 0 {
			cm.plugin.Info("marked records as deleted", "ip", ip, "port", httpPort, "count", result.RowsAffected)
		}
	} else {
		// 清除 DeleteAt（设置为 NULL）
		result := cm.db.Unscoped().Model(&m7s.RecordStream{}).
			Where("file_path LIKE ?", prefix+"%").
			Where("deleted_at IS NOT NULL").
			Updates(map[string]interface{}{
				"deleted_at": nil,
			})
		if result.Error != nil {
			cm.plugin.Error("markRecordsAsDeleted: failed to clear deleted_at", "error", result.Error)
		} else if result.RowsAffected > 0 {
			cm.plugin.Info("cleared deleted_at for records", "ip", ip, "port", httpPort, "count", result.RowsAffected)
		}
	}
}
