package plugin_gb28181pro

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	myip "github.com/husanpao/ip"
	"github.com/icholy/digest"
	"m7s.live/v5/pkg/task"
	gb28181 "m7s.live/v5/plugin/gb28181pro/pkg"
)

// Platform 表示GB28181平台的运行时实例
type Platform struct {
	task.Job      `gorm:"-:all"` // 使用 Task 而不是 Job，并且排除 gorm 序列化
	PlatformModel *gb28181.PlatformModel

	// SIP相关字段，不存储到数据库
	Client         *sipgo.Client              `gorm:"-" json:"-"` // SIP客户端
	DialogClient   *sipgo.DialogClient        `gorm:"-" json:"-"` // SIP对话客户端
	ContactHDR     *sip.ContactHeader         `gorm:"-" json:"-"` // 联系人头部
	FromHDR        *sip.FromHeader            `gorm:"-" json:"-"` // From头部
	CurrentSession *sipgo.DialogClientSession `gorm:"-" json:"-"` // 当前会话
	Recipient      sip.Uri                    `gorm:"-" json:"-"` // 接收者地址

	// 运行时字段
	KeepAliveReply     int    `gorm:"-" json:"keepAliveReply"`     // KeepAliveReply表示心跳未回复次数
	RegisterAliveReply int    `gorm:"-" json:"registerAliveReply"` // RegisterAliveReply表示注册未回复次数
	CallID             string `gorm:"-" json:"callId"`             // CallID表示SIP会话的标识符

	// 插件配置
	plugin *GB28181ProPlugin
}

// InitializeSIPClient 初始化SIP客户端
func (p *Platform) InitializeSIPClient(ua *sipgo.UserAgent) error {
	localIP := myip.InternalIPv4()
	var err error
	p.Client, err = sipgo.NewClient(ua, sipgo.WithClientHostname(localIP))
	if err != nil {
		return fmt.Errorf("failed to create sip client: %v", err)
	}

	// 设置联系人头部，使用本地平台的信息
	contactHdr := sip.ContactHeader{
		Address: sip.Uri{
			User: p.PlatformModel.DeviceGBID,
			Host: p.PlatformModel.DeviceIP,
			Port: p.PlatformModel.DevicePort,
		},
	}
	p.ContactHDR = &contactHdr

	// 设置From头部，使用本地平台的信息
	fromHdr := sip.FromHeader{
		Address: sip.Uri{
			User: p.PlatformModel.DeviceGBID,
			Host: p.PlatformModel.ServerGBDomain,
		},
		Params: sip.NewParams(),
	}
	fromHdr.Params.Add("tag", sip.GenerateTagN(16))
	p.FromHDR = &fromHdr

	// 创建对话客户端
	p.DialogClient = sipgo.NewDialogClient(p.Client, *p.ContactHDR)

	return nil
}

// Register 发送注册请求到上级平台
func (p *Platform) Register(ctx context.Context) (*sipgo.DialogClientSession, error) {
	// 创建注册请求的目标URI，使用上级平台的信息
	recipient := sip.Uri{
		User: p.PlatformModel.ServerGBID,
		Host: p.PlatformModel.ServerIP,
		Port: p.PlatformModel.ServerPort,
	}

	// 创建基本的REGISTER请求
	req := sip.NewRequest(sip.REGISTER, recipient)

	// 添加Contact头部
	contactStr := fmt.Sprintf("<sip:%s@%s:%d>", p.PlatformModel.DeviceGBID, p.PlatformModel.DeviceIP, p.PlatformModel.DevicePort)
	req.AppendHeader(sip.NewHeader("Contact", contactStr))

	// 添加From头部
	fromHeader := sip.FromHeader{
		Address: sip.Uri{
			User: p.PlatformModel.DeviceGBID,
			Host: p.PlatformModel.ServerGBDomain,
		},
		Params: sip.NewParams(),
	}
	fromHeader.Params.Add("tag", sip.GenerateTagN(16))
	req.AppendHeader(&fromHeader)

	// 添加To头部
	toHeader := sip.ToHeader{
		Address: sip.Uri{
			User: p.PlatformModel.DeviceGBID,
			Host: p.PlatformModel.ServerGBDomain,
		},
	}
	req.AppendHeader(&toHeader)

	// 添加Expires头部
	req.AppendHeader(sip.NewHeader("Expires", fmt.Sprintf("%d", p.PlatformModel.Expires)))

	// 设置传输协议
	req.SetTransport(strings.ToUpper(p.PlatformModel.Transport))

	// 发送请求并获取响应
	tx, err := p.Client.TransactionRequest(ctx, req, sipgo.ClientRequestAddVia)
	if err != nil {
		p.Error("register", "error", err.Error())
		return nil, fmt.Errorf("创建事务失败: %v", err)
	}
	defer tx.Terminate()

	// 获取响应
	res, err := p.getResponse(tx)
	if err != nil {
		p.Error("register", "error", err.Error())
		return nil, fmt.Errorf("获取响应失败: %v", err)
	}

	// 处理401未授权响应
	if res.StatusCode == 401 {
		// 获取WWW-Authenticate头部
		wwwAuth := res.GetHeader("WWW-Authenticate")
		if wwwAuth == nil {
			p.Error("register", "error", "no auth challenge")
			return nil, fmt.Errorf("未收到认证质询")
		}

		// 解析认证质询
		chal, err := digest.ParseChallenge(wwwAuth.Value())
		if err != nil {
			p.Error("register", "error", err.Error())
			return nil, fmt.Errorf("解析认证质询失败: %v", err)
		}

		// 生成认证响应
		cred, _ := digest.Digest(chal, digest.Options{
			Method:   req.Method.String(),
			URI:      recipient.Host,
			Username: p.PlatformModel.Username,
			Password: p.PlatformModel.Password,
		})

		// 创建新的带认证信息的请求
		newReq := req.Clone()
		newReq.RemoveHeader("Via") // 必须由传输层重新生成
		newReq.AppendHeader(sip.NewHeader("Authorization", cred.String()))

		// 发送认证请求
		tx, err = p.Client.TransactionRequest(ctx, newReq, sipgo.ClientRequestAddVia)
		if err != nil {
			return nil, fmt.Errorf("创建认证事务失败: %v", err)
		}
		defer tx.Terminate()

		// 获取认证响应
		res, err = p.getResponse(tx)
		if err != nil {
			return nil, fmt.Errorf("获取认证响应失败: %v", err)
		}
	}

	// 检查最终响应状态
	if res.StatusCode != 200 {
		p.Error("register", "status", res.StatusCode)
		return nil, fmt.Errorf("注册失败，状态码: %d", res.StatusCode)
	}

	p.Info("register", "response", res.String())
	return nil, nil
}

// getResponse 从事务中获取响应
func (p *Platform) getResponse(tx sip.ClientTransaction) (*sip.Response, error) {
	select {
	case <-tx.Done():
		return nil, fmt.Errorf("事务已终止")
	case res := <-tx.Responses():
		return res, nil
	}
}

// Keepalive 发送心跳请求到上级平台
func (p *Platform) Keepalive(ctx context.Context) (*sipgo.DialogClientSession, error) {
	recipient := sip.Uri{
		User: p.PlatformModel.ServerGBID,
		Host: p.PlatformModel.ServerIP,
		Port: p.PlatformModel.ServerPort,
	}

	req := sip.NewRequest("MESSAGE", recipient)
	req.SetTransport(strings.ToUpper(p.PlatformModel.Transport))

	// 添加From头部
	fromHeader := sip.FromHeader{
		Address: sip.Uri{
			User: p.PlatformModel.DeviceGBID,
			Host: p.PlatformModel.ServerGBDomain,
		},
		Params: sip.NewParams(),
	}
	fromHeader.Params.Add("tag", sip.GenerateTagN(16))
	req.AppendHeader(&fromHeader)

	// 添加To头部
	toHeader := sip.ToHeader{
		Address: sip.Uri{
			User: p.PlatformModel.ServerGBID,
			Host: p.PlatformModel.ServerGBDomain,
		},
	}
	req.AppendHeader(&toHeader)

	// 添加Contact头部
	contactStr := fmt.Sprintf("<sip:%s@%s:%d>", p.PlatformModel.DeviceGBID, p.PlatformModel.DeviceIP, p.PlatformModel.DevicePort)
	req.AppendHeader(sip.NewHeader("Contact", contactStr))

	tx, err := p.Client.TransactionRequest(ctx, req, sipgo.ClientRequestAddVia)
	if err != nil {
		p.Error("keepalive", "error", err.Error())
		return nil, fmt.Errorf("创建事务失败: %v", err)
	}
	defer tx.Terminate()

	res, err := p.getResponse(tx)
	if err != nil {
		p.Error("keepalive", "error", err.Error())
		return nil, err
	}

	if res.StatusCode != 200 {
		p.Error("keepalive", "status", res.StatusCode)
		return nil, fmt.Errorf("心跳失败，状态码: %d", res.StatusCode)
	}

	p.Info("keepalive", "response", res.String())
	return nil, nil
}

// Unregister 发送注销请求到上级平台
func (p *Platform) Unregister(ctx context.Context) (*sipgo.DialogClientSession, error) {
	// 创建注销请求的目标URI
	recipient := sip.Uri{
		User: p.PlatformModel.ServerGBID,
		Host: p.PlatformModel.ServerIP,
		Port: p.PlatformModel.ServerPort,
	}

	// 创建基本的REGISTER请求
	req := sip.NewRequest(sip.REGISTER, recipient)

	// 添加Contact头部
	contactStr := fmt.Sprintf("<sip:%s@%s:%d>", p.PlatformModel.DeviceGBID, p.PlatformModel.DeviceIP, p.PlatformModel.DevicePort)
	req.AppendHeader(sip.NewHeader("Contact", contactStr))

	// 添加From头部
	fromHeader := sip.FromHeader{
		Address: sip.Uri{
			User: p.PlatformModel.DeviceGBID,
			Host: p.PlatformModel.ServerGBDomain,
		},
		Params: sip.NewParams(),
	}
	fromHeader.Params.Add("tag", sip.GenerateTagN(16))
	req.AppendHeader(&fromHeader)

	// 添加To头部
	toHeader := sip.ToHeader{
		Address: sip.Uri{
			User: p.PlatformModel.DeviceGBID,
			Host: p.PlatformModel.ServerGBDomain,
		},
	}
	req.AppendHeader(&toHeader)

	// 添加Expires头部，设置为0表示注销
	req.AppendHeader(sip.NewHeader("Expires", "0"))

	// 设置传输协议
	req.SetTransport(strings.ToUpper(p.PlatformModel.Transport))

	// 发送请求并获取响应
	tx, err := p.Client.TransactionRequest(ctx, req, sipgo.ClientRequestAddVia)
	if err != nil {
		return nil, fmt.Errorf("创建事务失败: %v", err)
	}
	defer tx.Terminate()

	// 获取响应
	res, err := p.getResponse(tx)
	if err != nil {
		return nil, fmt.Errorf("获取响应失败: %v", err)
	}

	// 检查响应状态
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("注销失败，状态码: %d", res.StatusCode)
	}

	return nil, nil
}

// PlatformKeepAliveTask 任务
type PlatformKeepAliveTask struct {
	task.TickTask
	platform *Platform
}

func (k *PlatformKeepAliveTask) GetTickInterval() time.Duration {
	return time.Second * time.Duration(k.platform.PlatformModel.KeepTimeout)
}

func (k *PlatformKeepAliveTask) Tick(any) {
	if !k.platform.PlatformModel.Enable {
		return
	}

	ctx := context.Background()
	_, err := k.platform.Keepalive(ctx)
	if err != nil {
		k.platform.KeepAliveReply++
		k.Error("keepalive", "error", err.Error())
		if k.platform.KeepAliveReply >= 3 {
			k.platform.PlatformModel.Status = false
			k.platform.CurrentSession = nil
			k.Stop(fmt.Errorf("max keepalive retries reached"))
			// 重新启动注册任务
			var rt PlatformRegisterTask
			rt.platform = k.platform
			k.platform.AddTask(&rt)
		}
	} else {
		k.platform.KeepAliveReply = 0
	}
}

// PlatformRegisterTask 处理定时注册
type PlatformRegisterTask struct {
	task.TickTask
	platform *Platform
}

func (r *PlatformRegisterTask) GetTickInterval() time.Duration {
	return time.Second * time.Duration(r.platform.PlatformModel.Expires)
}

func (r *PlatformRegisterTask) Tick(any) {
	if !r.platform.PlatformModel.Enable {
		r.platform.PlatformModel.Status = false
		r.platform.CurrentSession = nil
		ctx := context.Background()
		_, _ = r.platform.Unregister(ctx)
		r.Error("register", "error", "platform disabled")
		r.Stop(fmt.Errorf("platform disabled"))
		return
	}

	ctx := context.Background()
	session, err := r.platform.Register(ctx)
	if err != nil {
		r.platform.RegisterAliveReply++
		r.Error("register", "error", err.Error(), "retries", r.platform.RegisterAliveReply)
		if r.platform.RegisterAliveReply >= 3 {
			r.platform.PlatformModel.Status = false
			r.platform.CurrentSession = nil
			r.Stop(fmt.Errorf("max retries reached: %d", r.platform.RegisterAliveReply))
		}
		return
	}

	r.Info("register", "status", "success")
	r.platform.PlatformModel.Status = true
	r.platform.CurrentSession = session
	r.platform.RegisterAliveReply = 0
}

// StartRegisterTask 启动注册任务
func (p *Platform) StartRegisterTask() {
	ctx := context.Background()

	// 首次注册
	session, err := p.Register(ctx)
	if err != nil {
		p.PlatformModel.Status = false
		p.RegisterAliveReply++
		// 注册失败，启动定时注册任务
		var rt PlatformRegisterTask
		rt.platform = p
		p.AddTask(&rt)
		return
	}

	// 注册成功，更新状态
	p.PlatformModel.Status = true
	p.CurrentSession = session
	p.RegisterAliveReply = 0
}

// OnMessage 处理来自平台的消息
func (p *Platform) OnMessage(req *sip.Request, tx sip.ServerTransaction, msg *gb28181.Message) error {
	// 更新平台状态
	p.PlatformModel.UpdateTime = time.Now().Format("2006-01-02 15:04:05")

	// 根据消息类型处理不同的消息
	switch msg.CmdType {
	case "Catalog":
		// 处理目录请求
		return p.handleCatalog(req, tx)
	case "DeviceControl":
		// 处理设备控制请求
		return p.handleDeviceControl(req, tx, msg)
	case "DeviceInfo":
		// 处理设备信息请求
		return p.handleDeviceInfo(req, tx, msg)
	case "Alarm":
		// 处理报警消息
		return p.handleAlarm(req, tx, msg)
	case "MobilePosition":
		// 处理移动位置信息
		return p.handleMobilePosition(req, tx, msg)
	default:
		// 不支持的消息类型，返回错误
		response := sip.NewResponseFromRequest(req, sip.StatusUnsupportedMediaType, "Unsupported message type", nil)
		if err := tx.Respond(response); err != nil {
			return fmt.Errorf("respond error: %v", err)
		}
		return fmt.Errorf("unsupported message type: %s", msg.CmdType)
	}
}

// handleCatalog 处理目录请求
func (p *Platform) handleCatalog(req *sip.Request, tx sip.ServerTransaction) error {
	// 回复 200 OK
	err := tx.Respond(sip.NewResponseFromRequest(req, http.StatusOK, "OK", nil))
	if err != nil {
		return err
	}

	// 获取 SN 和 FromTag
	sn, _ := req.From().Params.Get("tag")
	fromTag, _ := req.From().Params.Get("tag")

	// 查询通道列表
	var channels []gb28181.CommonGBChannel
	if p.plugin.DB != nil {
		if err := p.plugin.DB.Where("platform_id = ?", p.PlatformModel.ID).Find(&channels).Error; err != nil {
			return fmt.Errorf("query channels error: %v", err)
		}
	}

	// 发送目录响应
	if len(channels) > 0 {
		return p.sendCatalogResponse(req, sn, fromTag, channels)
	} else {
		return p.sendEmptyCatalogResponse(req, sn, fromTag)
	}
}

// CreateRequest 创建 SIP 请求
func (p *Platform) CreateRequest(method string) *sip.Request {
	request := sip.NewRequest(sip.RequestMethod(method), p.Recipient)
	request.SetDestination(p.Recipient.String())
	return request
}

// sendCatalogResponse 发送目录响应
func (p *Platform) sendCatalogResponse(req *sip.Request, sn string, fromTag string, channels []gb28181.CommonGBChannel) error {
	request := p.CreateRequest("MESSAGE")
	request.From().Params.Add("tag", fromTag)
	request.To().Params.Add("tag", fromTag)
	request.SetSource(req.Source())
	request.SetDestination(req.Destination())
	request.SetTransport(req.Transport())
	contentTypeHeader := sip.ContentTypeHeader("Application/MANSCDP+xml")
	request.AppendHeader(&contentTypeHeader)
	request.SetBody([]byte(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Response>
<CmdType>Catalog</CmdType>
<SN>%s</SN>
<DeviceChannelList Num="%d">
%s
</DeviceChannelList>
</Response>`, sn, len(channels), p.buildChannelList(channels))))
	_, err := p.Client.Do(p, request)
	return err
}

// sendEmptyCatalogResponse 发送空目录响应
func (p *Platform) sendEmptyCatalogResponse(req *sip.Request, sn string, fromTag string) error {
	request := p.CreateRequest("MESSAGE")
	request.From().Params.Add("tag", fromTag)
	request.To().Params.Add("tag", fromTag)
	request.SetSource(req.Source())
	request.SetDestination(req.Destination())
	request.SetTransport(req.Transport())
	contentTypeHeader := sip.ContentTypeHeader("Application/MANSCDP+xml")
	request.AppendHeader(&contentTypeHeader)
	request.SetBody([]byte(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Response>
<CmdType>Catalog</CmdType>
<SN>%s</SN>
<DeviceChannelList Num="0">
</DeviceChannelList>
</Response>`, sn)))
	_, err := p.Client.Do(p, request)
	return err
}

// handleDeviceControl 处理设备控制请求
func (p *Platform) handleDeviceControl(req *sip.Request, tx sip.ServerTransaction, msg *gb28181.Message) error {
	// TODO: 实现设备控制请求处理
	response := sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil)
	return tx.Respond(response)
}

// handleDeviceInfo 处理设备信息查询请求
func (p *Platform) handleDeviceInfo(req *sip.Request, tx sip.ServerTransaction, msg *gb28181.Message) error {
	// 先回复200 OK
	err := tx.Respond(sip.NewResponseFromRequest(req, http.StatusOK, "OK", nil))
	if err != nil {
		return fmt.Errorf("respond error: %v", err)
	}

	// 获取 SN 和 FromTag
	sn := strconv.Itoa(msg.SN)
	fromTag, _ := req.From().Params.Get("tag")

	// 获取请求的设备ID
	channelId := msg.DeviceID

	// 1. 判断是否是查询平台自身信息
	if p.PlatformModel.DeviceGBID == channelId {
		// 如果是查询平台信息，直接返回平台信息
		return p.sendDeviceInfoResponse(req, nil, sn, fromTag)
	}

	// 2. 查询通道信息
	var channel gb28181.CommonGBChannel
	if p.plugin.DB != nil {
		if err := p.plugin.DB.Where("platform_id = ? AND gb_device_id = ?", p.PlatformModel.ID, channelId).First(&channel).Error; err != nil {
			// 通道不存在，返回404
			response := sip.NewResponseFromRequest(req, sip.StatusNotFound, "channel not found or offline", nil)
			return tx.Respond(response)
		}
	}

	// 3. 判断通道类型
	if channel.GbDeviceDbID == 0 {
		// 非国标通道不支持设备信息查询
		response := sip.NewResponseFromRequest(req, sip.StatusForbidden, "non-gb channel not supported", nil)
		return tx.Respond(response)
	}

	// 4. 查询设备信息
	var device Device
	if p.plugin.DB != nil {
		if err := p.plugin.DB.First(&device, channel.GbDeviceDbID).Error; err != nil {
			// 设备不存在，返回404
			response := sip.NewResponseFromRequest(req, sip.StatusNotFound, "device not found", nil)
			return tx.Respond(response)
		}
	}

	// 5. 发送设备信息响应
	return p.sendDeviceInfoResponse(req, &device, sn, fromTag)
}

// sendDeviceInfoResponse 发送设备信息响应
func (p *Platform) sendDeviceInfoResponse(req *sip.Request, device *Device, sn string, fromTag string) error {
	request := p.CreateRequest("MESSAGE")
	request.From().Params.Add("tag", fromTag)
	request.To().Params.Add("tag", fromTag)
	request.SetSource(req.Source())
	request.SetDestination(req.Destination())
	request.SetTransport(req.Transport())
	contentTypeHeader := sip.ContentTypeHeader("Application/MANSCDP+xml")
	request.AppendHeader(&contentTypeHeader)

	// 构建响应XML
	var xmlContent string
	if device == nil {
		// 返回平台信息
		xmlContent = fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Response>
<CmdType>DeviceInfo</CmdType>
<SN>%s</SN>
<DeviceID>%s</DeviceID>
<Result>OK</Result>
<Manufacturer>%s</Manufacturer>
<Model>%s</Model>
<Firmware>%s</Firmware>
<Channel>%d</Channel>
</Response>`, sn, p.PlatformModel.DeviceGBID, p.PlatformModel.Manufacturer, p.PlatformModel.Model, "", p.PlatformModel.ChannelCount)
	} else {
		// 返回设备信息
		xmlContent = fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Response>
<CmdType>DeviceInfo</CmdType>
<SN>%s</SN>
<DeviceID>%s</DeviceID>
<Result>OK</Result>
<Manufacturer>%s</Manufacturer>
<Model>%s</Model>
<Firmware>%s</Firmware>
<Channel>%d</Channel>
</Response>`, sn, device.DeviceID, device.Manufacturer, device.Model, device.Firmware, device.ChannelCount)
	}

	request.SetBody([]byte(xmlContent))
	_, err := p.Client.Do(p, request)
	return err
}

// handleAlarm 处理报警消息
func (p *Platform) handleAlarm(req *sip.Request, tx sip.ServerTransaction, msg *gb28181.Message) error {
	// TODO: 实现报警消息处理
	response := sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil)
	return tx.Respond(response)
}

// handleMobilePosition 处理移动位置信息
func (p *Platform) handleMobilePosition(req *sip.Request, tx sip.ServerTransaction, msg *gb28181.Message) error {
	// TODO: 实现移动位置信息处理
	response := sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil)
	return tx.Respond(response)
}

func (p *Platform) buildChannelList(channels []gb28181.CommonGBChannel) string {
	var content string
	for _, channel := range channels {
		content += channel.GetFullContent(channel.StreamID, channel.StreamID, p.PlatformModel.DeviceGBID, "Catalog")
	}
	return content
}

// GetKey 返回平台的唯一标识符
func (p *Platform) GetKey() string {
	return p.PlatformModel.ServerGBID
}
