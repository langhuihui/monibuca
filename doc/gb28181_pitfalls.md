# GB28181 插件 v5 分支坑点总结

> 本文档基于 v5 分支的历史 commit 记录（约 198 条与 gb28181 相关），梳理了所有已发现并修复的问题，供后续开发和接入参考。

---

## 一、SIP 协议层问题

### 1.1 Via Header 不应手动添加

**问题**：早期代码在 `CreateRequest` 和 `Invite` 时手动构造并附加 `Via` 头，导致 SIP 栈（sipgo）再次自动追加，产生重复头部，引起部分设备协议解析失败或请求被拒绝。

**修复**：移除所有手动添加 `Via` 的代码，改由 sipgo 库自动管理。Linux 和 Windows 的行为差异也因此消除，无需再根据操作系统区分分支逻辑。

> 相关 commit：`6583bc2`、`3f69866`、`7e64183`

---

### 1.2 FromHeader 并发竞争

**问题**：`Device.fromHDR` 的 `Params` 字段是 `map` 类型，多个协程并发调用 `CreateRequest` 时直接对其执行 `Add("tag", …)`，因共享同一个 map 引用导致数据竞争（data race），表现为 SIP 请求中 tag 相互覆盖或 panic。

**修复**：在 `CreateRequest` 中构建请求时，先调用 `d.fromHDR.Params.Clone()` 克隆 map，基于副本追加 tag，而非修改原始对象。同时删除了初始化阶段预先向 `fromHDR.Params` 添加 tag 的操作（改为在请求发送时动态生成）。

> 相关 commit：`5826d68`

---

### 1.3 SIP 客户端 IP 绑定错误

**问题**：`sipgo.NewClient` 使用 `WithClientHostname` 绑定 IP 时，早期代码传入的是公网 IP（WAN IP），导致：
- 内网设备的 SIP 请求源地址错误；
- SDP 中 Contact 地址不可达；
- 在 NAT 后面的场景下，设备无法正常建立会话。

**修复**：`sipgo.NewClient` 统一使用本地局域网 IP（LAN IP），公网 IP 仅用于外网场景下的 SDP 中 `c=` 行。

> 相关 commit：`3f69866`、`470cab36`

---

### 1.4 SIP 客户端重复创建与复用

**问题**：早期每个 Dialog 和每次 TestSip 都独立调用 `sipgo.NewClient` 和 `sipgo.NewDialogClientCache` 创建新客户端，占用资源且在恢复设备时可能使用旧的、失效的客户端。

**修复**：引入 `getOrCreateClient` 机制，对相同 `(sipIP, localPort, transport)` 三元组复用同一 `sipgo.Client` 实例；`NewDialogClientCache` 改为按需临时创建，不再长期保存在设备结构体上。

> 相关 commit：`7e64183`、`6f51a15`

---

### 1.5 SIP 注册鉴权（密码认证）

**问题**：最初实现不支持带密码的 SIP 注册，设备注册时服务端不做鉴权，向上级平台注册时密码处理流程也不完整（CSeq 未正确递增、用户名字段传错）。

**修复**：
- 新增 `Password` 配置字段；
- 注册需要密码时，正确处理 `401 Unauthorized` 响应，递增 CSeq 重新发送含 `Authorization` 头的注册请求；
- 向上级平台注销时，同样需要走密码验证流程（`Expires: 0`）。

> 相关 commit：`6fdc855`、`940a220`、`0914fb8`、`0470f78`

---

### 1.6 SIP Transport 字段错误

**问题**：早期对 TCP 被动模式设备的处理中，Transport 字段硬编码为 `"UDP"`，导致设备使用 TCP 接入时媒体流协商失败。

**修复**：根据设备实际传输模式（`StreamMode`）动态填写 Transport，支持 `TCP-PASSIVE`、`TCP-ACTIVE`、`UDP` 三种模式。

> 相关 commit：`f5fe7c7`、`2b462b4`

---

### 1.7 SIP Contact Header 获取错误

**问题**：设备发起请求时从错误的位置提取 Contact 头，导致后续 BYE 或 INFO 消息无法正确送达目标。

**修复**：修正 Contact Header 的解析来源。

> 相关 commit：`f475419`

---

## 二、设备注册与生命周期

### 2.1 设备重复快速注册导致启动过多 Task

**问题**：设备短时间内多次发送 REGISTER 请求（如网络抖动），`OnRegister` 会多次走 `StoreDevice` 流程，为同一设备启动多个并发任务（catalog 查询、keepalive 等），造成资源浪费甚至死锁。

**修复**：在 `OnRegister` 中增加判断——若设备已存在（无论在线与否），优先走 `StoreDevice(deviceid, req, d)` 复用已有设备对象并恢复，而不是无条件新建。

> 相关 commit：`80ad104`

---

### 2.2 设备注销时 Online 状态未正确更新

**问题**：设备发送注销（Expires: 0）请求后，内存中的设备状态未置为 offline，下次请求时仍视为在线设备处理。

**修复**：收到注销请求时，明确将 `device.Status`、`device.Online` 及所有子通道状态设置为 offline。

> 相关 commit：`1a8e2bc`

---

### 2.3 设备注销导致死锁

**问题**：设备登出处理流程中，先在 `channels.Range` 回调里调用 `d.channels.RemoveByKey`，对同一集合在遍历中修改，导致死锁。

**修复**：遍历时不再在回调中删除元素，遍历结束后统一调用 `d.channels.Clear()`。

> 相关 commit：`2034f06`

---

### 2.4 NAT 环境下设备 IP/Port 不更新

**问题**：设备经 NAT 重新上线（如路由器重启后 IP/Port 变化），`RecoverDevice` 流程未更新设备的 `IP`、`Port`、`HostAddress` 字段，导致后续 Invite 请求发往旧地址失败。

**修复**：`RecoverDevice` 时从 SIP 请求 Source 中提取最新 IP 和 Port 并覆盖到设备记录。

> 相关 commit：`470cab36`、`6f51a15`

---

### 2.5 程序启动时设备过期检查逻辑错误

**问题**：`checkDeviceExpire` 在程序启动时对数据库中所有设备（包括历史离线设备）执行过期计算，导致本来就是 offline 的设备被重新标记为 online/offline 状态相互覆盖，引发不必要的任务启动。

**修复**：只对数据库中 `online = true` 的设备才做过期时间判断，历史 offline 设备直接跳过。

> 相关 commit：`34397235`

---

### 2.6 `from.Address.User` 校验缺失

**问题**：`OnRegister` 时未校验 `from.Address.User` 是否为有效的设备 ID，导致部分非标准 SIP 消息被当成合法注册处理。

**修复**：添加对 `from.Address.User` 的非空校验，无效时直接忽略。

> 相关 commit：`23f2ed3`

---

### 2.7 SIP 注册响应缺少必要头部

**问题**：SIP 注册成功响应（200 OK）中缺少部分必要的 SIP 头（如 `Contact`、`Date` 等），导致严格实现的设备端无法正确处理注册结果，出现重复注册循环。

**修复**：在 SIP 注册响应中补全标准头部。

> 相关 commit：`4ac22f1`

---

## 三、Keepalive 心跳

### 3.1 Keepalive 超时判断逻辑不合理

**问题**：`DeviceKeepaliveTickTask.Tick` 中对设备心跳超时的判断直接调用 `d.Stop()`，停止整个设备任务，但会触发级联清理逻辑（如通道下线、数据库保存），导致在短暂网络中断后设备无法快速恢复，体验较差。

**修复**：超时后将 `d.seconds` 设为极大值（`time.Minute * 1440`）挂起心跳检查，不立即停止设备，等待下次注册重新激活。同时在任务描述中记录 deviceid 和 tick 秒数，便于调试。

> 相关 commit：`4543dec`

---

## 四、目录（Catalog）查询

### 4.1 Catalog XML 节点路径解析错误

**问题**：Catalog 响应 XML 中，通道列表节点路径实际为 `DeviceChannelList>Item`，但早期代码中解析路径为 `DeviceList>Item`，导致通道数量始终为 0。

**修复**：修正 XML 解析 tag 为 `DeviceList struct { DeviceChannelList []ChannelInfo \`xml:"DeviceChannelList>Item"\` }`。

> 相关 commit：`7343e24`

---

### 4.2 Catalog 响应 SumNum 不一致时处理缺失

**问题**：不同厂商设备可能分多次返回 Catalog 响应，每次 `SumNum`（总数）可能与实际 XML 中解析到的通道数不一致；同一 SN 的多次响应中 SumNum 也可能发生变化，导致完成判断失误（过早认为获取完成或永远等待）。

**修复**：累加实际解析到的通道数（`actualChannelCount`）而非依赖 `DeviceNum` 字段；对 SumNum 不一致的情况输出 Warn 日志；新增超时检测（10 秒），避免永久挂起。

> 相关 commit：`da57ddc`

---

### 4.3 CatalogRequest 并发锁误用

**问题**：`CatalogRequest` 结构体上加了 `sync.Mutex` 保护字段访问，但实际上所有 catalog 处理都在 `catalogHandlerTask.Run()` 的 `Work` 串行协程中执行，加锁不仅多余，还可能因错误的锁顺序导致死锁。

**修复**：移除 `sync.Mutex`，依靠 Work 队列的串行执行天然保证线程安全；同时在 `CatalogRequest` 上增加 `CreateTime` 字段用于超时检测。

> 相关 commit：`da57ddc`

---

### 4.4 设备恢复注册后 Catalog 更新时机混乱

**问题**：设备重新注册（RecoverDevice）后，代码中对是否触发 catalog 查询反复变动——曾存在「恢复时自动 catalog」「注释掉不查」「再次恢复」三个状态，导致通道列表未能及时刷新。

**修复**：最终确定在 `RecoverDevice` 完成后明确调用一次 `d.catalog()`，保证设备重连后通道列表更新。

> 相关 commit：`7a3543e`（注释掉）→ `29e2142`（恢复）

---

## 五、SDP 与媒体 IP

### 5.1 SDP 中 `o=` 行 IP 混用 MediaIP 和 SipIP

**问题**：`Invite` 时构建 SDP，`o=` 行（Origin）使用 `device.MediaIp`（媒体收流 IP），但标准规定此处应使用信令 IP（SipIP），错误的 IP 会导致部分设备因 SDP 校验不通过而拒绝 Invite。

**修复**：`o=` 行改为使用 `device.SipIp`。

> 相关 commit：`acf9f0c`

---

### 5.2 内网设备在局域网内播放时 IP 选择错误

**问题**：当设备 IP 为私有地址（内网），SDP `c=` 行（Connection）仍使用公网 IP，导致内网设备无法正常回流媒体流。

**修复**：根据设备 IP 是否为私有地址（`net.ParseIP(device.IP).IsPrivate()`）决定 SDP 中 `c=` 行使用内网还是公网 IP。

> 相关 commit：`262d24d`

---

### 5.3 SipIP 和 MediaIP 配置未正确下发到设备

**问题**：全局 `gb.SipIP` / `gb.MediaIP` 配置在程序重启恢复设备时未重新赋值给设备的 `SipIp` / `MediaIp` 字段，导致 Invite SDP 中 IP 不正确。

**修复**：在 `checkDeviceExpire` 加载历史设备时，若全局配置中有 `SipIP` / `MediaIP`，强制覆盖设备字段。

> 相关 commit：`acf9f0c`

---

## 六、端口管理

### 6.1 TCP 端口类型溢出

**问题**：Port 字段类型为 `uint32`，在将其传给端口通道（`chan uint16`）时发生隐式截断，可能导致端口值错误，进而绑定失败或端口冲突。

**修复**：将所有端口字段统一为 `uint16`，所有赋值处添加显式类型转换。

> 相关 commit：`8fb9ba4`

---

### 6.2 TCP 端口回收条件不正确

**问题**：`Dialog.Dispose()` 中端口回收逻辑写为 `} else {`，即当 UDP 端口回收分支没有命中时都会把端口放回 TCP 端口池，导致实际用 TCP-ACTIVE 模式的 dialog 也错误地回收了 TCP Passive 的端口槽位，造成端口计数混乱。

**修复**：改为 `} else if d.StreamMode == mrtp.StreamModeTCPPassive {`，明确只在 TCP Passive 模式才回收 TCP 端口。

> 相关 commit：`f5fe7c7`

---

### 6.3 端口范围管理和回收机制不完善

**问题**：早期端口管理使用简单的 channel 传递端口号，在高并发下存在端口泄漏（dialog 异常退出不回收）和重复分配问题；端口回收的代码路径不统一。

**修复**：引入 `port_bitmap.go`，基于 bitmap 管理端口分配和回收；优化端口回收逻辑，确保在 dialog 生命周期结束（`Dispose`）时统一回收；新增接口查看当前已用端口列表。

> 相关 commit：`4e55524`、`6a99a6d`、`78b91a4`

---

### 6.4 单端口模式（Single MediaPort）支持

**问题**：部分网络环境（如严格防火墙）只允许开放一个媒体端口，多端口模式下每路流需要独立端口，无法适配。

**修复**：新增 `tcpPort` 单端口配置，当配置了单一 TCP 端口时，复用同一个 `net.Listener`，所有 dialog 共享该监听器；每次 Invite 时不从端口池取端口，直接使用全局固定端口。

> 相关 commit：`42acf47`、`ea512e1`、`69ff04a`

---

## 七、Dialog 管理

### 7.1 Dialog Key 使用 SSRC 而非 CallID

**问题**：`gb.dialogs` 最初以 SSRC（`uint32`）为 key 管理 dialog，但 SSRC 由设备分配，不同设备可能发送相同 SSRC，导致 dialog 互相覆盖，Pause/Resume 等操作找不到正确的 dialog。

**修复**：改为以 CallID（`string`）为 key，`dialogs` 类型从 `task.Manager[uint32, *Dialog]` 改为 `task.Manager[string, *Dialog]`。

> 相关 commit：`966153f`、`584c2e9`

---

### 7.2 录像 Pause/Resume 时 Dialog 未注册

**问题**：`Dialog.Start()` 成功后未将 dialog 保存到 `gb.dialogs`，导致外部调用 Pause/Resume API 时无法通过 CallID 找到 dialog，操作失效。

**修复**：在 `Dialog.Start()` 末尾调用 `d.gb.dialogs.Set(d)`；在 `Dialog.Dispose()` 末尾调用 `d.gb.dialogs.Remove(d)`。

> 相关 commit：`35df83b`

---

## 八、通道管理

### 8.1 Channel.DeviceID 与 Channel.ChannelID 混用

**问题**：多处代码在需要使用通道 ID（ChannelID）的地方错误地使用了设备 ID（DeviceID），包括：
- `GetPullableList` 返回的可拉流路径；
- `OnInvite` 时查找 device 的逻辑；
- API 返回的 Channel 列表中 DeviceID 字段。

**修复**：统一区分 `DeviceID`（设备编号）和 `ChannelID`（通道编号），所有通道相关操作使用正确字段。

> 相关 commit：`8ca001e`、`192a846`

---

### 8.2 设备离线时子通道状态未更新

**问题**：设备下线（注销或 keepalive 超时）时，仅更新设备本身状态，子通道的 `Status` 字段未一并置为 `"OFF"`，导致 API 返回的通道状态与实际不符。

**修复**：设备 Dispose/下线时遍历所有子通道，将其状态设置为 `ChannelOffStatus`。

> 相关 commit：`1a8e2bc`、`4543dec`

---

### 8.3 通道更新时新增/删除/改 ID 未能正确同步

**问题**：设备重新 catalog 后，若某些通道 ChannelID 变化、新增或删除，仅简单追加或覆盖，未处理旧通道的清理，导致内存中保留"幽灵通道"。

**修复**：引入 `catalogReqs` 集合管理当次 catalog 请求，catalog 完成后对比新旧通道列表，删除不再存在的通道，更新变化的通道。

> 相关 commit：`4de4a832`

---

## 九、级联平台（上级平台）

### 9.1 FromHeader.Host 字段错误

**问题**：向上级平台发送 REGISTER 时，`fromHDR.Address.Host` 使用了 `ServerGBDomain`（上级平台的 ID/域），应使用 `DeviceGBDomain`（本设备的域）。

**修复**：将 `Host` 字段改为使用 `p.PlatformModel.DeviceGBDomain`。

> 相关 commit：`b2122bc`

---

### 9.2 平台过早注册到 platforms 集合

**问题**：`NewPlatform` 中调用 `p.plugin.platforms.Set(p)` 在平台任务实际启动之前就注册，若启动失败平台仍残留在集合中，后续管理逻辑混乱。

**修复**：注释掉 `NewPlatform` 中的 `p.plugin.platforms.Set(p)`，改为在任务真正 start 成功后再注册。

> 相关 commit：`b6ee284`

---

### 9.3 级联只支持单一传输模式

**问题**：早期级联（`ForwardDialog`）的 SDP 硬编码为 `a=setup:passive`，无法应对上级平台要求 TCP Active 或 UDP 的场景。

**修复**：`ForwardDialog.Start()` 根据设备 `StreamMode` 动态生成 SDP 中的 `a=setup` 和 `a=connection` 字段，支持 `TCP-PASSIVE`、`TCP-ACTIVE`、`UDP` 三种模式。

> 相关 commit：`2b462b4`

---

### 9.4 SIP 客户端 XML 解码编码问题

**问题**：部分设备返回的 XML 消息使用 GB2312 编码，Go 标准 XML 解析器默认 UTF-8，直接解析会乱码或报错。

**修复**：在 XML 解码前检测编码声明，若为 GB2312 / GBK 则先转换为 UTF-8，再交给标准 XML 解析器处理。

> 相关 commit：`7e64183`、`b8c1fa5`

---

## 十、历史录像查询与下载

### 10.1 录像查询结果包含空白记录

**问题**：设备返回的录像查询响应中，`RecordList.Item` 可能包含全空字段的"幽灵记录"（DeviceID 和 StartTime 均为空），直接加入结果集导致前端显示异常。

**修复**：遍历录像列表时过滤掉 `DeviceID == "" && StartTime == ""` 的无效条目；同时跳过 `RecordList.Item` 为空的响应包。

> 相关 commit：`7e64183`

---

### 10.2 录像下载文件路径错误

**问题**：历史录像下载时，文件保存路径拼接逻辑有误，实际写出的文件路径与预期不符，导致下载后找不到文件或覆盖错误。

**修复**：修正 `download_handler.go` 中的路径拼接逻辑。

> 相关 commit：`e65d7b1`

---

### 10.3 下载快进时时间戳突变检测误判

**问题**：历史录像支持 1x/2x/4x 速度下载，快速下载时接收码率远大于真实帧率，`TsTamer` 的突变检测（连续帧间距超过 `10 * frameDur`）会频繁触发，错误地修正时间戳，导致播放卡顿或时间轴错乱。

**修复**：在 `TsTamer.Tame()` 中增加判断，当 fps > 100 时（表示快速下载场景）跳过突变检测。

> 相关 commit：`9d5c01d`

---

### 10.4 下载 dialog 端口未正确回收

**问题**：历史录像下载（`downloaddialog.go`）结束后，占用的 RTP 端口未归还端口池，长时间运行后端口耗尽。

**修复**：在 `DownloadDialog.Dispose()` 中补充端口回收逻辑。

> 相关 commit：`f0666f4`

---

## 十一、数据库与数据一致性

### 11.1 设备主键使用自增 ID 导致重复插入

**问题**：Device 表原以自增整数 ID 为主键，同一设备（相同 DeviceID）断线重连后再次注册时，GORM 的 `FirstOrCreate` 逻辑不稳定，可能创建多条记录，导致查询到重复设备。

**修复**：将 `device_id`（国标编号）设为主键，移除自增 `id` 列；关联 channel 的外键也从 `device_db_id` 改为 `device_id`。

> 相关 commit：`192a846`

---

### 11.2 设备删除时通道未级联删除

**问题**：删除设备时只删除了 `devices` 表中的记录，`device_channels` 表中该设备的所有通道记录未同步删除，造成孤立数据累积。

**修复**：删除设备改用数据库事务，先删 Device 再删对应的 DeviceChannel（用 `WHERE device_id = ?`），提交失败时回滚。

> 相关 commit：`23f2ed3`

---

### 11.3 设备列表 API 数据源不一致

**问题**：`/api/device/list` 从数据库查询设备列表，而 OnMessage、OnInvite 等处理逻辑使用内存中的 `gb.devices`，两者可能不一致（内存中的实时状态未及时落库），导致 API 返回的在线状态与实际不符。

**修复**：设备列表 API 改为直接遍历内存中的 `gb.devices`，不再走数据库查询。

> 相关 commit：`966153f`

---

### 11.4 设备经纬度被 0 值覆盖

**问题**：收到 DeviceStatus 或 PositionNotify 消息时，若字段值为 0（设备未上报位置），直接赋值给 `device.Latitude/Longitude`，覆盖了之前保存的真实坐标。

**修复**：只在值非空/非零时才更新经纬度字段。

> 相关 commit：`ae698c7`

---

### 11.5 数据库连接池参数缺失

**问题**：GORM 连接未配置连接池参数，高并发场景下连接数超限导致请求超时。

**修复**：初始化时调用 `SetMaxIdleConns(25)`、`SetMaxOpenConns(100)`、`SetConnMaxLifetime(5 * time.Minute)`。

> 相关 commit：`ae698c7`

---

### 11.6 DeviceName 重复更新问题

**问题**：每次收到 DeviceInfo 响应都无条件覆盖 `device.Name`，若用户在管理界面手动修改了设备名，设备重新上报后会被恢复为原始名称。

**修复**：只在 `d.Name == ""` 时才从 DeviceInfo 响应中赋值，用户自定义名称优先。

> 相关 commit：`0470f78`

---

## 十二、流路径匹配

### 12.1 OnSub 正则表达式过于宽泛

**问题**：`config.yaml` 中 `gb28181.onsub.pull` 的正则配置为 `.*`，匹配所有订阅路径，导致与其他插件（HLS、RTMP 等）的流路径产生冲突，非 GB28181 的流被误路由到 GB28181 设备触发 Invite。

**修复**：将正则改为精确匹配 GB28181 流路径格式：
- `^\d{20}/\d{20}$`（设备 ID/通道 ID）
- `^gb_\d+/(.+)$`（特殊前缀格式）

> 相关 commit：`91ddd03`

---

### 12.2 PullProxy 从 StreamPath 提取 DeviceID 错误

**问题**：GB28181 类型的 PullProxy 在 `Start()` 中调用 `p.GetStreamPath()` 获取设备 ID 和通道 ID，但 `GetStreamPath()` 返回的是处理后的路径（可能包含前缀），与 `conf.url` 中配置的原始路径不同，导致设备查找失败。

**修复**：改为直接从 `p.PullProxyConfig.Pull.URL` 中分割获取 `deviceId` 和 `channelId`。

> 相关 commit：`20a1e95`

---

## 十三、第三方设备兼容性

### 13.1 DJI 等设备发送 DataTransfer 命令

**问题**：大疆等设备在 SIP MESSAGE 消息中发送 `CmdType = DataTransfer` 命令，早期代码走到 `default` 分支返回 `400 Bad Request`，导致设备端认为信令失败，可能影响后续通信。

**修复**：增加 `case "DataTransfer":` 分支，暂时静默处理（`/* todo */`），不返回错误。

> 相关 commit：`b1cb41a`

---

### 13.2 时间戳字符串格式兼容 UTC（`Z` 后缀）

**问题**：录像查询时间范围解析时，正则和 `time.ParseInLocation` 不支持带 `Z` 的 ISO 8601 UTC 时间格式（如 `2024-01-01T00:00:00Z`），解析失败返回零值，查询结果为空。

**修复**：时间格式正则增加 `Z?` 可选匹配；解析逻辑中判断若包含 `Z` 则用 `time.Parse(time.RFC3339, …)` 解析。

> 相关 commit：`6c8c444`

---

## 十四、配置与初始化

### 14.1 未配置 SIP 监听地址时无明确报错

**问题**：`GB28181Plugin.OnInit()` 中若未配置 `Sip.ListenAddr`，后续启动监听会静默失败，没有明确错误提示，排查困难。

**修复**：在 `OnInit` 中检测 `Sip.ListenAddr` 为空时，打印明确错误日志，提示用户如何配置：
```
GB28181 init failed,please set Sip.ListenAddr in GB28181 configuration like this
sip:
  listenaddr:
    - udp::5060
```

> 相关 commit：`6fdc855`

---

### 14.2 从 config.yaml 加载平台配置的问题

**问题**：通过 `config.yaml` 中 `platforms` 字段静态配置上级平台时，InitPlugin 执行顺序导致平台配置未能正确合并到数据库中查到的平台列表，部分静态平台启动时被跳过。

**修复**：调整平台初始化顺序，先查数据库中的启用平台，再追加 `config.yaml` 中的静态平台，最后统一处理；为平台自动填充 `Serial`、`Realm`、`DeviceIP`、`DevicePort` 等字段（从当前 SIP 配置派生）。

> 相关 commit：`75791fe`、`7282f1f`、`127c063`

---

### 14.3 DB 为 nil 时 OnInit 未返回错误

**问题**：部分操作依赖数据库（如保存设备、查询平台），若 DB 未初始化（未启用任何数据库插件）时直接 panic 或空指针，且 `OnInit` 未提前检测和返回错误。

**修复**：`OnInit` 中检测 `gb.DB == nil` 时返回明确错误。

> 相关 commit：`0470f78`

---

## 十五、其他问题

### 15.1 循环依赖（Circular Dependency）

**问题**：早期 gb28181 插件与其他包之间存在循环 import，导致编译失败。

**修复**：重构包结构，将共用的类型和接口抽离到 `plugin/gb28181/pkg`，消除循环依赖。

> 相关 commit：`eef8892`

---

### 15.2 字段命名不规范

**问题**：代码中 `deviceId` 和 `deviceID` 混用（Go 规范应为 `deviceID`），以及 JSON/API 字段名使用 UpperCamelCase 而非 lowerCamelCase，导致前后端字段对不上。

**修复**：统一字段命名为 lowerCamelCase（如 `device_id`、`gb_device_id`），JSON tag 规范化。

> 相关 commit：`987cd4f`、`3c5becd`

---

### 15.3 UpdateDevice API 订阅任务条件判断反转

**问题**：`UpdateDevice` 中更新设备的 `SubscribeCatalog`/`SubscribePosition`/`SubscribeAlarm` 时，先判断 `Task != nil`，再判断订阅间隔 `> 0`，逻辑顺序反转——若任务不存在但配置了订阅间隔，不会创建新任务；若任务存在但间隔为 0，仍会错误触发。

**修复**：改为先判断订阅间隔 `> 0`，再判断任务是否已存在：存在则更新间隔并触发，不存在则新建任务。

> 相关 commit：`0470f78`

---

*文档最后更新：基于 v5 分支截至 2026-04-15 的提交记录分析。*
