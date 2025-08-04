package plugin_gb28181pro

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
	"m7s.live/v5"
	"m7s.live/v5/pkg/util"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/icholy/digest"
	"m7s.live/v5/pkg/task"
	gb28181 "m7s.live/v5/plugin/gb28181/pkg"
)

// Platform 表示GB28181平台的运行时实例
type Platform struct {
	task.Job      `gorm:"-:all"` // 使用TickTask，并且排除 gorm 序列化
	PlatformModel *gb28181.PlatformModel

	// SIP相关字段，不存储到数据库
	Client         *sipgo.Client            `gorm:"-" json:"-"` // SIP客户端
	DialogClient   *sipgo.DialogClientCache `gorm:"-" json:"-"` // SIP对话客户端
	Recipient      sip.Uri                  `gorm:"-" json:"-"` // 接收者地址
	ContactHDR     *sip.ContactHeader       `gorm:"-" json:"-"` // 联系人头部
	UserAgentHDR   sip.Header               `gorm:"-" json:"-"` //
	MaxForwardsHDR sip.MaxForwardsHeader    `gorm:"-" json:"-"`

	// 运行时字段
	KeepAliveReply int    `gorm:"-" json:"keepAliveReply"` // KeepAliveReply表示心跳未回复次数
	RegisterCallID string `gorm:"-" json:"registerCallID"` // CallID表示SIP会话的标识符
	SN             int

	// 插件配置
	plugin     *GB28181Plugin
	unRegister bool
	channels   util.Collection[string, *Channel] `gorm:"-:all"`
	register   *Register
}

// UTF8ToGB2312 将UTF-8编码的字符串转换为GB2312编码
func UTF8ToGB2312(s string) (string, error) {
	reader := transform.NewReader(bytes.NewReader([]byte(s)), simplifiedchinese.GB18030.NewEncoder())
	d, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return string(d), nil
}

func NewPlatform(pm *gb28181.PlatformModel, plugin *GB28181Plugin, unRegister bool) *Platform {
	p := &Platform{
		PlatformModel: pm,
		plugin:        plugin,
		unRegister:    unRegister,
	}
	client, err := sipgo.NewClient(p.plugin.ua, sipgo.WithClientHostname(p.PlatformModel.DeviceIP), sipgo.WithClientPort(p.PlatformModel.DevicePort))
	if err != nil {
		p.Error("failed to create sip client", "err", err)
	}
	p.Client = client
	userAgentHeader := sip.NewHeader("User-Agent", "M7S/"+m7s.Version)
	p.UserAgentHDR = userAgentHeader

	// 创建注册请求的目标URI，使用上级平台的信息
	recipient := sip.Uri{
		User: p.PlatformModel.ServerGBID,
		Host: p.PlatformModel.ServerIP,
		Port: p.PlatformModel.ServerPort,
	}

	p.Recipient = recipient

	// 设置联系人头部，使用本地平台的信息
	contactHdr := sip.ContactHeader{
		Address: sip.Uri{
			User: p.PlatformModel.DeviceGBID,
			Host: p.PlatformModel.DeviceIP,
			Port: p.PlatformModel.DevicePort,
		},
	}
	p.ContactHDR = &contactHdr

	// 创建对话客户端
	p.DialogClient = sipgo.NewDialogClientCache(p.Client, *p.ContactHDR)

	p.MaxForwardsHDR = sip.MaxForwardsHeader(70)
	return p
}

func (p *Platform) Start() error {
	if p.unRegister {
		err := p.Unregister()
		if err != nil {
			p.Error("failed to unregister", "err", err)
		}
		p.unRegister = false
	}
	register := NewRegister(p, "firstRegister")
	register.OnStart(func() {
		register.Tick(nil)
	})
	p.register = register
	p.AddTask(register)
	return nil
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
func (p *Platform) Keepalive() (*sipgo.DialogClientSession, error) {

	req := sip.NewRequest("MESSAGE", p.Recipient)
	req.SetTransport(strings.ToUpper(p.PlatformModel.Transport))
	customCallID := fmt.Sprintf("%s-%d@%s", p.PlatformModel.DeviceGBID, time.Now().Unix(), p.PlatformModel.ServerIP)
	callID := sip.CallIDHeader(customCallID)
	req.AppendHeader(&callID)

	csqHeader := sip.CSeqHeader{
		SeqNo:      uint32(p.SN),
		MethodName: "REGISTER",
	}
	p.SN++
	req.AppendHeader(&csqHeader)

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

	//viaHeader := sip.ViaHeader{
	//	ProtocolName:    "SIP",
	//	ProtocolVersion: "2.0",
	//	Transport:       p.PlatformModel.Transport,
	//	Host:            p.PlatformModel.DeviceIP,
	//	Port:            p.PlatformModel.DevicePort,
	//	Params:          sip.NewParams(),
	//}
	//viaHeader.Params.Add("branch", sip.GenerateBranchN(16)).Add("rport", "")
	//req.AppendHeader(&viaHeader)

	req.AppendHeader(&p.MaxForwardsHDR)

	// 添加Contact头部
	req.AppendHeader(p.ContactHDR)

	req.AppendHeader(p.UserAgentHDR)

	// 添加Expires头部，根据是否注销设置不同值
	req.AppendHeader(sip.NewHeader("Expires", fmt.Sprintf("%d", p.PlatformModel.Expires)))

	contentLengthHeader := sip.ContentLengthHeader(0)
	req.AppendHeader(&contentLengthHeader)
	req.SetBody(gb28181.BuildKeepAliveXML(p.SN, p.PlatformModel.DeviceGBID))
	p.SN++
	tx, err := p.Client.TransactionRequest(p, req)
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

// Register 执行注册或注销流程
func (p *Platform) Register(isUnregister bool) error {
	// 创建基本的REGISTER请求
	req := sip.NewRequest(sip.REGISTER, p.Recipient)

	// 设置日志标签
	logTag := "register"
	if isUnregister {
		logTag = "unregister"
	}

	//callid
	if p.RegisterCallID != "" {
		callID := sip.CallIDHeader(p.RegisterCallID)
		req.AppendHeader(&callID)
	} else {
		customCallID := fmt.Sprintf("%d@%s", time.Now().Unix(), p.PlatformModel.DeviceIP)
		callID := sip.CallIDHeader(customCallID)
		req.AppendHeader(&callID)
	}

	//cseqheader
	csqHeader := sip.CSeqHeader{
		SeqNo:      uint32(p.SN),
		MethodName: "REGISTER",
	}
	p.SN++
	req.AppendHeader(&csqHeader)

	// 设置From头部，使用本地平台的信息
	fromHdr := sip.FromHeader{
		Address: sip.Uri{
			User: p.PlatformModel.DeviceGBID,
			Host: p.PlatformModel.ServerGBDomain,
		},
		Params: sip.NewParams(),
	}
	fromHdr.Params.Add("tag", sip.GenerateTagN(16))
	req.AppendHeader(&fromHdr)

	// 添加To头部
	toHeader := sip.ToHeader{
		Address: sip.Uri{
			User: p.PlatformModel.DeviceGBID,
			Host: p.PlatformModel.ServerGBDomain,
		},
	}
	req.AppendHeader(&toHeader)

	// 添加Via头部
	//viaHeader := sip.ViaHeader{
	//	ProtocolName:    "SIP",
	//	ProtocolVersion: "2.0",
	//	Transport:       p.PlatformModel.Transport,
	//	Host:            p.PlatformModel.DeviceIP,
	//	Port:            p.PlatformModel.DevicePort,
	//	Params:          sip.NewParams(),
	//}
	//viaHeader.Params.Add("branch", sip.GenerateBranchN(16)).Add("rport", "")
	//req.AppendHeader(&viaHeader)

	req.AppendHeader(&p.MaxForwardsHDR)

	// 添加Contact头部
	req.AppendHeader(p.ContactHDR)

	req.AppendHeader(p.UserAgentHDR)

	// 添加Expires头部，根据是否注销设置不同值
	if isUnregister {
		req.AppendHeader(sip.NewHeader("Expires", "0"))
	} else {
		req.AppendHeader(sip.NewHeader("Expires", fmt.Sprintf("%d", p.PlatformModel.Expires)))
	}

	contentLengthHeader := sip.ContentLengthHeader(0)
	req.AppendHeader(&contentLengthHeader)
	// 设置传输协议
	req.SetTransport(strings.ToUpper(p.PlatformModel.Transport))

	tx, err := p.Client.TransactionRequest(p, req)
	if err != nil {
		p.plugin.Error(logTag, "error", err.Error())
		return fmt.Errorf("创建事务失败: %v", err)
	}
	defer tx.Terminate()

	// 获取响应
	res, err := p.getResponse(tx)
	if err != nil {
		p.plugin.Error(logTag, "error", err.Error())
		return err
	}

	// 处理401未授权响应
	if res.StatusCode == 401 {
		// 获取WWW-Authenticate头部
		wwwAuth := res.GetHeader("WWW-Authenticate")
		if wwwAuth == nil {
			p.plugin.Error(logTag, "error", "no auth challenge")
			return fmt.Errorf("no auth challenge")
		}

		// 解析认证质询
		chal, err := digest.ParseChallenge(wwwAuth.Value())
		if err != nil {
			p.plugin.Error(logTag, "error", err.Error())
			return err
		}

		p.plugin.Debug("received auth challenge",
			"realm", chal.Realm,
			"nonce", chal.Nonce,
			"algorithm", chal.Algorithm,
			"qop", chal.QOP)

		// 生成认证响应
		opts := digest.Options{
			Method:   req.Method.String(),
			URI:      "sip:" + p.PlatformModel.ServerGBDomain,
			Username: p.PlatformModel.Username,
			Password: p.PlatformModel.Password,
			Cnonce:   sip.GenerateTagN(16),
			Count:    1,
		}

		cred, err := digest.Digest(chal, opts)
		if err != nil {
			p.plugin.Error("calculating digest failed", "error", err.Error())
			return err
		}

		p.plugin.Debug("calculated response info",
			"username", opts.Username,
			"uri", opts.URI,
			"cnonce", opts.Cnonce,
			"count", opts.Count,
			"response", cred.Response)

		// 创建新的带认证信息的请求
		newReq := req.Clone()
		newReq.RemoveHeader("Via") // 必须由传输层重新生成
		newReq.AppendHeader(sip.NewHeader("Authorization", cred.String()))
		newReq.CSeq().SeqNo = uint32(p.SN) // 更新CSeq序号
		p.SN++

		// 发送认证请求
		tx, err = p.Client.TransactionRequest(p, newReq, sipgo.ClientRequestAddVia)
		if err != nil {
			p.plugin.Error(logTag, "error", err.Error())
			return err
		}
		defer tx.Terminate()

		// 获取认证响应
		res, err = p.getResponse(tx)
		if err != nil {
			p.plugin.Error(logTag, "error", err.Error())
			return err
		}
	}

	// 检查最终响应状态
	if res.StatusCode != 200 {
		p.plugin.Error(logTag, "status", res.StatusCode)
		return fmt.Errorf("%s失败，状态码: %d", logTag, res.StatusCode)
	}

	p.plugin.Info(logTag, "status", "success")
	// 根据操作类型设置状态
	p.PlatformModel.Status = !isUnregister
	return nil
}

// Unregister 发送注销请求到上级平台
func (p *Platform) Unregister() error {
	return p.Register(true)
}

// DoRegister 执行注册流程
func (p *Platform) DoRegister() error {
	return p.Register(false)
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
	_, err := k.platform.Keepalive()
	if err != nil {
		k.platform.KeepAliveReply++
		k.Error("keepalive", "error", err.Error())
		if k.platform.KeepAliveReply >= 3 {
			k.platform.PlatformModel.Status = false
			// 重新启动注册任务
			//k.platform.Start()
			platform := k.platform
			platform.KeepAliveReply = 0
			k.Stop(fmt.Errorf("max keepalive retries reached"))
			platform.register.registerType = "firstRegister"
			platform.register.Ticker.Reset(time.Second * time.Duration(platform.PlatformModel.Expires))
			platform.register.Tick(nil)
		}
	} else {
		k.platform.KeepAliveReply = 0
	}
}

// OnMessage 处理来自平台的消息
func (p *Platform) OnMessage(req *sip.Request, tx sip.ServerTransaction, msg *gb28181.Message) error {
	// 更新平台状态
	p.PlatformModel.UpdateTime = time.Now().Format("2006-01-02 15:04:05")

	// 根据消息类型处理不同的消息
	switch msg.CmdType {
	case "Catalog":
		// 处理目录请求
		return p.handleCatalog(req, tx, msg)
	case "DeviceControl":
		// 处理设备控制请求
		return p.handleDeviceControl(req, tx, msg)
	case "DeviceInfo":
		// 处理设备信息请求
		return p.handleDeviceInfo(req, tx, msg)
	case "DeviceStatus":
		// 处理设备信息请求
		return p.handleDeviceStatus(req, tx, msg)
	case "PresetQuery":
		// 处理预置位查询请求
		return p.handlePresetQuery(req, tx, msg)
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
func (p *Platform) handleCatalog(req *sip.Request, tx sip.ServerTransaction, msg *gb28181.Message) error {
	// 回复 200 OK
	err := tx.Respond(sip.NewResponseFromRequest(req, http.StatusOK, "OK", nil))
	if err != nil {
		return err
	}

	// 获取 SN 和 FromTag
	sn := strconv.Itoa(msg.SN)
	fromTag, _ := req.From().Params.Get("tag")
	p.plugin.Info("catalog", "sn", sn, "fromTag", fromTag)

	// 打印平台ID
	p.plugin.Info("catalog query platform_id", "platform_id", p.PlatformModel.ServerGBID)

	// 查询通道列表
	var channels []gb28181.DeviceChannel
	//if p.plugin.DB != nil {
	//	if err := p.plugin.DB.Table("gb28181_channel gc").
	//		Select(`gc.*`).
	//		Joins("left join gb28181_platform_channel gpc on gc.id=gpc.channel_db_id").
	//		Where("gpc.platform_server_gb_id = ? and gc.status='ON'", p.PlatformModel.ServerGBID).
	//		Find(&channels).Error; err != nil {
	//		return fmt.Errorf("query channels error: %v", err)
	//	}
	//}
	for channel := range p.channels.Range {
		channels = append(channels, *channel.DeviceChannel)
	}

	// 发送目录响应，无论是否有通道
	p.plugin.Info("get channels success", "channels", channels)
	return p.sendCatalogResponse(req, sn, fromTag, channels)
}

// CreateRequest 创建 SIP 请求
func (p *Platform) CreateRequest(method string) *sip.Request {
	request := sip.NewRequest(sip.RequestMethod(method), p.Recipient)
	//request.SetDestination(p.Recipient.String())
	return request
}

// sendCatalogResponse 发送目录响应
func (p *Platform) sendCatalogResponse(req *sip.Request, sn string, fromTag string, channels []gb28181.DeviceChannel) error {
	// 如果没有通道，发送一个空的目录列表
	if len(channels) == 0 {
		request := p.CreateRequest("MESSAGE")

		// 设置From头部
		fromHeader := sip.FromHeader{
			Address: sip.Uri{
				User: p.PlatformModel.DeviceGBID,
				Host: p.PlatformModel.ServerGBDomain,
			},
			Params: sip.NewParams(),
		}
		fromHeader.Params.Add("tag", fromTag)
		request.AppendHeader(&fromHeader)

		// 添加To头部
		toHeader := sip.ToHeader{
			Address: sip.Uri{
				User: p.PlatformModel.ServerGBID,
				Host: p.PlatformModel.ServerGBDomain,
			},
		}
		request.AppendHeader(&toHeader)

		// 添加Via头部
		//viaHeader := sip.ViaHeader{
		//	ProtocolName:    "SIP",
		//	ProtocolVersion: "2.0",
		//	Transport:       p.PlatformModel.Transport,
		//	Host:            p.PlatformModel.DeviceIP,
		//	Port:            p.PlatformModel.DevicePort,
		//	Params:          sip.NewParams(),
		//}
		//viaHeader.Params.Add("branch", sip.GenerateBranchN(16)).Add("rport", "")
		//request.AppendHeader(&viaHeader)

		request.SetTransport(req.Transport())
		contentTypeHeader := sip.ContentTypeHeader("Application/MANSCDP+xml")
		request.AppendHeader(&contentTypeHeader)

		// 空目录列表XML
		xmlContent := fmt.Sprintf(`<?xml version="1.0" encoding="GB2312"?>
<Response>
<CmdType>Catalog</CmdType>
<SN>%s</SN>
<DeviceID>%s</DeviceID>
<SumNum>0</SumNum>
<DeviceList Num="0">
</DeviceList>
</Response>`, sn, p.PlatformModel.DeviceGBID)
		request.SetBody([]byte(xmlContent))

		// 修正：使用TransactionRequest替代Do
		tx, err := p.Client.TransactionRequest(p, request)
		if err != nil {
			p.Error("sendCatalogResponse", "error", err.Error())
			return fmt.Errorf("创建事务失败: %v", err)
		}
		defer tx.Terminate()

		// 获取响应
		res, err := p.getResponse(tx)
		if err != nil {
			p.Error("sendCatalogResponse", "error", err.Error())
			return err
		}

		// 处理401未授权响应
		if res.StatusCode == 401 {
			// 获取WWW-Authenticate头部
			wwwAuth := res.GetHeader("WWW-Authenticate")
			if wwwAuth == nil {
				p.Error("sendCatalogResponse", "error", "no auth challenge")
				return fmt.Errorf("no auth challenge")
			}

			// 解析认证质询
			chal, err := digest.ParseChallenge(wwwAuth.Value())
			if err != nil {
				p.Error("sendCatalogResponse", "error", err.Error())
				return err
			}

			p.plugin.Debug("received auth challenge",
				"realm", chal.Realm,
				"nonce", chal.Nonce,
				"algorithm", chal.Algorithm,
				"qop", chal.QOP)

			// 生成认证响应
			opts := digest.Options{
				Method:   request.Method.String(),
				URI:      "sip:" + p.PlatformModel.ServerGBDomain,
				Username: p.PlatformModel.Username,
				Password: p.PlatformModel.Password,
				Cnonce:   sip.GenerateTagN(16),
				Count:    1,
			}

			cred, err := digest.Digest(chal, opts)
			if err != nil {
				p.Error("calculating digest failed", "error", err.Error())
				return err
			}

			p.plugin.Debug("calculated response info",
				"username", opts.Username,
				"uri", opts.URI,
				"cnonce", opts.Cnonce,
				"count", opts.Count,
				"response", cred.Response)

			// 创建新的带认证信息的请求
			newReq := request.Clone()
			newReq.RemoveHeader("Via") // 必须由传输层重新生成
			newReq.AppendHeader(sip.NewHeader("Authorization", cred.String()))

			// 发送认证请求
			tx, err = p.Client.TransactionRequest(p, newReq, sipgo.ClientRequestAddVia)
			if err != nil {
				p.Error("sendCatalogResponse", "error", err.Error())
				return err
			}
			defer tx.Terminate()

			// 获取认证响应
			res, err = p.getResponse(tx)
			if err != nil {
				p.Error("sendCatalogResponse", "error", err.Error())
				return err
			}
		}

		// 检查最终响应状态
		if res.StatusCode != 200 {
			p.Error("sendCatalogResponse", "status", res.StatusCode)
			return fmt.Errorf("发送目录响应失败，状态码: %d", res.StatusCode)
		}

		return nil
	}

	// 有通道时，为每个通道单独发送一个XML
	for i, channel := range channels {
		request := p.CreateRequest("MESSAGE")

		// 设置From头部
		fromHeader := sip.FromHeader{
			Address: sip.Uri{
				User: p.PlatformModel.DeviceGBID,
				Host: p.PlatformModel.ServerGBDomain,
			},
			Params: sip.NewParams(),
		}
		fromHeader.Params.Add("tag", fromTag)
		request.AppendHeader(&fromHeader)

		// 添加To头部
		toHeader := sip.ToHeader{
			Address: sip.Uri{
				User: p.PlatformModel.ServerGBID,
				Host: p.PlatformModel.ServerGBDomain,
			},
		}
		request.AppendHeader(&toHeader)

		// 添加Via头部
		//viaHeader := sip.ViaHeader{
		//	ProtocolName:    "SIP",
		//	ProtocolVersion: "2.0",
		//	Transport:       p.PlatformModel.Transport,
		//	Host:            p.PlatformModel.DeviceIP,
		//	Port:            p.PlatformModel.DevicePort,
		//	Params:          sip.NewParams(),
		//}
		//viaHeader.Params.Add("branch", sip.GenerateBranchN(16)).Add("rport", "")
		//request.AppendHeader(&viaHeader)

		request.SetTransport(req.Transport())
		contentTypeHeader := sip.ContentTypeHeader("Application/MANSCDP+xml")
		request.AppendHeader(&contentTypeHeader)

		// 为单个通道创建XML
		channelXML := p.buildChannelItem(channel)
		xmlContent := fmt.Sprintf(`<?xml version="1.0" encoding="GB2312"?>
<Response>
<CmdType>Catalog</CmdType>
<SN>%s</SN>
<DeviceID>%s</DeviceID>
<SumNum>%d</SumNum>
<DeviceList Num="1">
%s
</DeviceList>
</Response>`, sn, p.PlatformModel.DeviceGBID, len(channels), channelXML)

		request.SetBody([]byte(xmlContent))

		// 修正：使用TransactionRequest替代Do
		tx, err := p.Client.TransactionRequest(p, request)
		if err != nil {
			p.Error("sendCatalogResponse", "error", err.Error(), "channel_index", i)
			return fmt.Errorf("创建事务失败: %v", err)
		}
		defer tx.Terminate()

		// 获取响应
		res, err := p.getResponse(tx)
		if err != nil {
			p.Error("sendCatalogResponse", "error", err.Error(), "channel_index", i)
			return err
		}

		// 处理401未授权响应
		if res.StatusCode == 401 {
			// 获取WWW-Authenticate头部
			wwwAuth := res.GetHeader("WWW-Authenticate")
			if wwwAuth == nil {
				p.Error("sendCatalogResponse", "error", "no auth challenge", "channel_index", i)
				return fmt.Errorf("no auth challenge")
			}

			// 解析认证质询
			chal, err := digest.ParseChallenge(wwwAuth.Value())
			if err != nil {
				p.Error("sendCatalogResponse", "error", err.Error(), "channel_index", i)
				return err
			}

			p.plugin.Debug("received auth challenge",
				"realm", chal.Realm,
				"nonce", chal.Nonce,
				"algorithm", chal.Algorithm,
				"qop", chal.QOP,
				"channel_index", i)

			// 生成认证响应
			opts := digest.Options{
				Method:   request.Method.String(),
				URI:      "sip:" + p.PlatformModel.ServerGBDomain,
				Username: p.PlatformModel.Username,
				Password: p.PlatformModel.Password,
				Cnonce:   sip.GenerateTagN(16),
				Count:    1,
			}

			cred, err := digest.Digest(chal, opts)
			if err != nil {
				p.Error("calculating digest failed", "error", err.Error(), "channel_index", i)
				return err
			}

			p.plugin.Debug("calculated response info",
				"username", opts.Username,
				"uri", opts.URI,
				"cnonce", opts.Cnonce,
				"count", opts.Count,
				"response", cred.Response,
				"channel_index", i)

			// 创建新的带认证信息的请求
			newReq := request.Clone()
			newReq.RemoveHeader("Via") // 必须由传输层重新生成
			newReq.AppendHeader(sip.NewHeader("Authorization", cred.String()))

			// 发送认证请求
			tx, err = p.Client.TransactionRequest(p, newReq, sipgo.ClientRequestAddVia)
			if err != nil {
				p.Error("sendCatalogResponse", "error", err.Error(), "channel_index", i)
				return err
			}
			defer tx.Terminate()

			// 获取认证响应
			res, err = p.getResponse(tx)
			if err != nil {
				p.Error("sendCatalogResponse", "error", err.Error(), "channel_index", i)
				return err
			}
		}

		// 检查最终响应状态
		if res.StatusCode != 200 {
			p.Error("sendCatalogResponse", "status", res.StatusCode, "channel_index", i)
			return fmt.Errorf("发送目录响应失败，状态码: %d", res.StatusCode)
		}

		// 添加短暂延迟以防止发送过快
		time.Sleep(time.Millisecond * 50)
	}

	return nil
}

// buildChannelItem 构建单个通道的XML项
func (p *Platform) buildChannelItem(channel gb28181.DeviceChannel) string {
	// 确保字符串字段不为空
	deviceID := channel.ChannelId
	if deviceID == "" {
		deviceID = "unknown_device" // 如果没有设备ID，使用默认值
	}
	name := channel.Name
	if name == "" {
		name = "未命名设备"
	}
	manufacturer := channel.Manufacturer
	if manufacturer == "" {
		manufacturer = "未知厂商"
	}
	model := channel.Model
	if model == "" {
		model = "未知型号"
	}
	owner := channel.Owner
	if owner == "" {
		owner = "未知所有者"
	}
	address := channel.Address
	if address == "" {
		address = "未知地址"
	}
	parentID := channel.ParentId
	if parentID == "" {
		parentID = p.PlatformModel.DeviceGBID // 使用平台ID作为父ID
	}

	return fmt.Sprintf(`<Item>
<DeviceID>%s</DeviceID>
<Name>%s</Name>
<Manufacturer>%s</Manufacturer>
<Model>%s</Model>
<Owner>%s</Owner>
<Address>%s</Address>
<RegisterWay>%d</RegisterWay>
<Secrecy>%d</Secrecy>
<ParentID>%s</ParentID>
<Parental>%d</Parental>
<SafetyWay>%d</SafetyWay>
<Status>ON</Status>
<Info>
</Info>
</Item>`, deviceID, name, manufacturer, model,
		owner, address,
		channel.RegisterWay, // 直接使用整数值
		channel.Secrecy,     // 直接使用整数值
		parentID,
		channel.Parental,  // 直接使用整数值
		channel.SafetyWay) // 直接使用整数值
}

// handleDeviceControl 处理设备控制请求
func (p *Platform) handleDeviceControl(req *sip.Request, tx sip.ServerTransaction, msg *gb28181.Message) error {
	// 首先回复200 OK给上级平台
	err := tx.Respond(sip.NewResponseFromRequest(req, http.StatusOK, "OK", nil))
	if err != nil {
		return fmt.Errorf("respond error: %v", err)
	}

	// 获取通道ID
	channelId := msg.DeviceID
	var deviceId string

	if tmpChannel, ok := p.plugin.channels.Find(func(c *Channel) bool {
		return c.ChannelId == channelId
	}); ok {
		deviceId = tmpChannel.DeviceId
	} else {
		p.Error("设备不存在或未注册", "device_id", msg.DeviceID)
		return fmt.Errorf("device not found or not registered: %v", msg.DeviceID)
	}

	// 从devices集合中获取设备实例
	device, ok := p.plugin.devices.Get(deviceId)
	if !ok {
		p.Error("设备不存在或未注册", "device_id", deviceId)
		return fmt.Errorf("device not found or not registered: %v", deviceId)
	}

	// 创建转发请求
	request := sip.NewRequest(sip.MESSAGE, device.Recipient)

	// 设置From头部，使用平台信息
	fromHeader := device.fromHDR
	fromTag, _ := req.From().Params.Get("tag")
	fromHeader.Params.Add("tag", fromTag)
	request.AppendHeader(&fromHeader)

	// 添加To头部，使用设备信息
	toHeader := sip.ToHeader{
		Address: device.Recipient,
	}
	request.AppendHeader(&toHeader)

	// 添加Via头部
	//viaHeader := sip.ViaHeader{
	//	ProtocolName:    "SIP",
	//	ProtocolVersion: "2.0",
	//	Transport:       device.Transport,
	//	Host:            device.SipIp,
	//	Port:            device.LocalPort,
	//	Params:          sip.NewParams(),
	//}
	//viaHeader.Params.Add("branch", sip.GenerateBranchN(16)).Add("rport", "")
	//request.AppendHeader(&viaHeader)

	// 设置Content-Type
	contentTypeHeader := sip.ContentTypeHeader("Application/MANSCDP+xml")
	request.AppendHeader(&contentTypeHeader)

	// 直接使用原始消息体
	request.SetBody(req.Body())

	// 设置传输协议
	request.SetTransport(strings.ToUpper(device.Transport))

	// 发送请求
	_, err = device.client.Do(p, request)
	if err != nil {
		p.Error("发送控制命令失败", "error", err.Error())
		return fmt.Errorf("send control command failed: %v", err)
	}

	return nil
}

// handleDeviceStatus 处理设备状态查询请求
func (p *Platform) handleDeviceStatus(req *sip.Request, tx sip.ServerTransaction, msg *gb28181.Message) error {
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
		// 如果是查询平台信息，直接返回平台状态
		return p.sendDeviceStatusResponse(req, nil, sn, fromTag)
	}

	// 2. 查询通道和设备信息
	type Result struct {
		DeviceID string `gorm:"column:device_id"`
	}
	var result Result

	if p.plugin.DB != nil {
		// 多表联查: channel_gb28181pro -> device_gb28181pro -> devices
		if err := p.plugin.DB.Table("gb28181_channel gc").
			Select("gd.device_id").
			Joins("LEFT JOIN gb28181_device gd ON gc.device_id = gd.device_id").
			Where("gc.channel_id = ?", channelId).
			First(&result).Error; err != nil {
			p.Error("查询通道和设备信息失败", "error", err.Error())
			return fmt.Errorf("channel or device not found: %v", err)
		}
	}

	// 3. 从devices集合中获取设备实例
	device, ok := p.plugin.devices.Get(result.DeviceID)
	if !ok {
		p.Error("设备不存在或未注册", "device_id", result.DeviceID)
		return fmt.Errorf("device not found or not registered: %v", result.DeviceID)
	}

	// 4. 发送设备状态响应
	return p.sendDeviceStatusResponse(req, device, sn, fromTag)
}

// sendDeviceStatusResponse 发送设备状态响应
func (p *Platform) sendDeviceStatusResponse(req *sip.Request, device *Device, sn string, fromTag string) error {
	request := p.CreateRequest("MESSAGE")

	// 设置From头部
	fromHeader := sip.FromHeader{
		Address: sip.Uri{
			User: p.PlatformModel.DeviceGBID,
			Host: p.PlatformModel.ServerGBDomain,
		},
		Params: sip.NewParams(),
	}
	fromHeader.Params.Add("tag", fromTag)
	request.AppendHeader(&fromHeader)

	// 添加To头部
	toHeader := sip.ToHeader{
		Address: sip.Uri{
			User: p.PlatformModel.ServerGBID,
			Host: p.PlatformModel.ServerGBDomain,
		},
	}
	request.AppendHeader(&toHeader)

	// 添加Via头部
	//viaHeader := sip.ViaHeader{
	//	ProtocolName:    "SIP",
	//	ProtocolVersion: "2.0",
	//	Transport:       p.PlatformModel.Transport,
	//	Host:            p.PlatformModel.DeviceIP,
	//	Port:            p.PlatformModel.DevicePort,
	//	Params:          sip.NewParams(),
	//}
	//viaHeader.Params.Add("branch", sip.GenerateBranchN(16)).Add("rport", "")
	//request.AppendHeader(&viaHeader)

	// 设置Content-Type
	contentTypeHeader := sip.ContentTypeHeader("Application/MANSCDP+xml")
	request.AppendHeader(&contentTypeHeader)

	// 获取当前时间，格式化为设备时间
	currentTime := time.Now().Format("2006-01-02T15:04:05")

	// 根据设备状态构建响应
	var deviceID, online, status, encode, record string
	if device == nil {
		// 平台自身状态
		deviceID = p.PlatformModel.DeviceGBID
		online = "ONLINE"
		status = "OK"
		encode = "ON"
		record = "OFF"
	} else {
		// 设备状态
		deviceID = device.DeviceId
		// 将布尔值转换为对应的状态字符串
		if device.Online {
			online = "ONLINE"
			status = "OK"
			encode = "ON"  // 在线时默认编码开启
			record = "OFF" // 默认不录制
		} else {
			online = "OFFLINE"
			status = "ERROR"
			encode = "OFF" // 离线时编码关闭
			record = "OFF" // 离线时不录制
		}
	}

	// 构建响应XML
	xmlContent := fmt.Sprintf(`<?xml version="1.0" encoding="GB2312" standalone="yes" ?>
<Response>
<CmdType>DeviceStatus</CmdType>
<SN>%s</SN>
<DeviceID>%s</DeviceID>
<Result>OK</Result>
<Online>%s</Online>
<Status>%s</Status>
<DeviceTime>%s</DeviceTime>
<Encode>%s</Encode>
<Record>%s</Record>
<Alarmstatus Num="0"/>
</Response>`, sn, deviceID, online, status, currentTime, encode, record)

	request.SetBody([]byte(xmlContent))

	// 设置传输协议
	request.SetTransport(strings.ToUpper(p.PlatformModel.Transport))

	// 发送响应
	_, err := p.Client.Do(p, request)
	if err != nil {
		p.Error("发送设备状态响应失败", "error", err.Error())
		return fmt.Errorf("send device status response failed: %v", err)
	}

	return nil
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
	var channel gb28181.DeviceChannel
	if p.plugin.DB != nil {
		if err := p.plugin.DB.Where("device_id = ? AND channel_id = ?", p.PlatformModel.ServerGBID, channelId).First(&channel).Error; err != nil {
			// 通道不存在，返回404
			response := sip.NewResponseFromRequest(req, sip.StatusNotFound, "channel not found or offline", nil)
			return tx.Respond(response)
		}
	}

	// 3. 判断通道类型
	if channel.DeviceId == "" {
		// 非国标通道不支持设备信息查询
		response := sip.NewResponseFromRequest(req, sip.StatusForbidden, "non-gb channel not supported", nil)
		return tx.Respond(response)
	}

	// 4. 查询设备信息
	var device Device
	if p.plugin.DB != nil {
		if err := p.plugin.DB.First(&device, channel.DeviceId).Error; err != nil {
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
	// 设置From头部
	fromHeader := sip.FromHeader{
		Address: sip.Uri{
			User: p.PlatformModel.DeviceGBID,
			Host: p.PlatformModel.ServerGBDomain,
		},
		Params: sip.NewParams(),
	}
	fromHeader.Params.Add("tag", fromTag)
	request.AppendHeader(&fromHeader)
	// 添加To头部
	toHeader := sip.ToHeader{
		Address: sip.Uri{
			User: p.PlatformModel.ServerGBID,
			Host: p.PlatformModel.ServerGBDomain,
		},
	}
	request.AppendHeader(&toHeader)
	// 添加Via头部
	//viaHeader := sip.ViaHeader{
	//	ProtocolName:    "SIP",
	//	ProtocolVersion: "2.0",
	//	Transport:       p.PlatformModel.Transport,
	//	Host:            p.PlatformModel.DeviceIP,
	//	Port:            p.PlatformModel.DevicePort,
	//	Params:          sip.NewParams(),
	//}
	//viaHeader.Params.Add("branch", sip.GenerateBranchN(16)).Add("rport", "")
	//request.AppendHeader(&viaHeader)
	contentTypeHeader := sip.ContentTypeHeader("Application/MANSCDP+xml")
	request.AppendHeader(&contentTypeHeader)

	// 构建响应XML
	var xmlContent string
	if device == nil {
		// 返回平台信息
		xmlContent = fmt.Sprintf(`<?xml version="1.0" encoding="GB2312"?>
<Response>
<CmdType>DeviceInfo</CmdType>
<SN>%s</SN>
<DeviceID>%s</DeviceID>
<Result>OK</Result>
<DeviceName>%s</DeviceName>
<Manufacturer>%s</Manufacturer>
<Model>%s</Model>
<Firmware>%s</Firmware>
<Channel>%d</Channel>
</Response>`, sn, p.PlatformModel.DeviceGBID, p.PlatformModel.Name, p.PlatformModel.Manufacturer, p.PlatformModel.Model, "", p.PlatformModel.ChannelCount)
	} else {
		// 返回设备信息
		xmlContent = fmt.Sprintf(`<?xml version="1.0" encoding="GB2312"?>
<Response>
<CmdType>DeviceInfo</CmdType>
<SN>%s</SN>
<DeviceID>%s</DeviceID>
<Result>OK</Result>
<DeviceName>%s</DeviceName>
<Manufacturer>%s</Manufacturer>
<Model>%s</Model>
<Firmware>%s</Firmware>
<Channel>%d</Channel>
</Response>`, sn, device.DeviceId, device.Name, device.Manufacturer, device.Model, device.Firmware, device.ChannelCount)
	}

	// 将UTF-8编码的XML内容转换为GB2312编码
	gb2312Content, err := UTF8ToGB2312(xmlContent)
	if err != nil {
		p.Error("sendDeviceInfoResponse", "encoding error", err.Error())
		// 如果转换失败，仍然使用原始内容，避免完全失败
		request.SetBody([]byte(xmlContent))
	} else {
		// 使用转换后的GB2312编码内容
		request.SetBody([]byte(gb2312Content))
	}

	// 修正：使用正确的上下文参数
	tx, err := p.Client.TransactionRequest(p, request)
	if err != nil {
		p.Error("sendDeviceInfoResponse", "error", err.Error())
		return fmt.Errorf("创建事务失败: %v", err)
	}
	defer tx.Terminate()

	// 获取响应
	res, err := p.getResponse(tx)
	if err != nil {
		p.Error("sendDeviceInfoResponse", "error", err.Error())
		return err
	}

	// 处理401未授权响应
	if res.StatusCode == 401 {
		// 获取WWW-Authenticate头部
		wwwAuth := res.GetHeader("WWW-Authenticate")
		if wwwAuth == nil {
			p.Error("sendDeviceInfoResponse", "error", "no auth challenge")
			return fmt.Errorf("no auth challenge")
		}

		// 解析认证质询
		chal, err := digest.ParseChallenge(wwwAuth.Value())
		if err != nil {
			p.Error("sendDeviceInfoResponse", "error", err.Error())
			return err
		}

		p.Debug("received auth challenge",
			"realm", chal.Realm,
			"nonce", chal.Nonce,
			"algorithm", chal.Algorithm,
			"qop", chal.QOP)

		// 生成认证响应
		opts := digest.Options{
			Method:   request.Method.String(),
			URI:      "sip:" + p.PlatformModel.ServerGBDomain,
			Username: p.PlatformModel.Username,
			Password: p.PlatformModel.Password,
			Cnonce:   sip.GenerateTagN(16),
			Count:    1,
		}

		cred, err := digest.Digest(chal, opts)
		if err != nil {
			p.Error("calculating digest failed", "error", err.Error())
			return err
		}

		p.Debug("calculated response info",
			"username", opts.Username,
			"uri", opts.URI,
			"cnonce", opts.Cnonce,
			"count", opts.Count,
			"response", cred.Response)

		// 创建新的带认证信息的请求
		newReq := request.Clone()
		newReq.RemoveHeader("Via") // 必须由传输层重新生成
		newReq.AppendHeader(sip.NewHeader("Authorization", cred.String()))

		// 发送认证请求
		tx, err = p.Client.TransactionRequest(p, newReq, sipgo.ClientRequestAddVia)
		if err != nil {
			p.Error("sendDeviceInfoResponse", "error", err.Error())
			return err
		}
		defer tx.Terminate()

		// 获取认证响应
		res, err = p.getResponse(tx)
		if err != nil {
			p.Error("sendDeviceInfoResponse", "error", err.Error())
			return err
		}
	}

	// 检查最终响应状态
	if res.StatusCode != 200 {
		p.Error("sendDeviceInfoResponse", "status", res.StatusCode)
		return fmt.Errorf("发送设备信息响应失败，状态码: %d", res.StatusCode)
	}

	p.Info("sendDeviceInfoResponse", "status", "success")
	return nil
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

// handlePresetQuery 处理预置位查询请求
func (p *Platform) handlePresetQuery(req *sip.Request, tx sip.ServerTransaction, msg *gb28181.Message) error {
	// 首先回复200 OK给上级平台
	err := tx.Respond(sip.NewResponseFromRequest(req, http.StatusOK, "OK", nil))
	if err != nil {
		return fmt.Errorf("respond error: %v", err)
	}

	// 获取通道ID
	channelID := msg.DeviceID

	// 查询通道和设备信息
	type Result struct {
		DeviceID string `gorm:"column:device_id"`
	}
	var result Result

	if p.plugin.DB != nil {
		// 多表联查: channel_gb28181pro -> device_gb28181pro -> devices
		if err := p.plugin.DB.Table("gb28181_channel gc").
			Select("gc.device_id").
			Joins("LEFT JOIN gb28181_device gd ON gd.device_id = gc.device_id").
			Where("gc.channel_id = ?", channelID).
			First(&result).Error; err != nil {
			p.Error("查询通道和设备信息失败", "error", err.Error())
			return fmt.Errorf("channel or device not found: %v", err)
		}
	}

	// 从devices集合中获取设备实例
	device, ok := p.plugin.devices.Get(result.DeviceID)
	if !ok {
		p.Error("设备不存在或未注册", "device_id", result.DeviceID)
		return fmt.Errorf("device not found or not registered: %v", result.DeviceID)
	}

	// 创建转发请求
	request := sip.NewRequest(sip.MESSAGE, device.Recipient)

	// 设置From头部，使用平台信息
	fromHeader := device.fromHDR
	fromTag, _ := req.From().Params.Get("tag")
	fromHeader.Params.Add("tag", fromTag)
	request.AppendHeader(&fromHeader)

	// 添加To头部，使用设备信息
	toHeader := sip.ToHeader{
		Address: device.Recipient,
	}
	request.AppendHeader(&toHeader)

	// 添加Via头部
	//viaHeader := sip.ViaHeader{
	//	 ProtocolName:    "SIP",
	//	 ProtocolVersion: "2.0",
	//	 Transport:       device.Transport,
	//	 Host:            device.SipIp,
	//	 Port:            device.LocalPort,
	//	 Params:          sip.NewParams(),
	//}
	//viaHeader.Params.Add("branch", sip.GenerateBranchN(16)).Add("rport", "")
	//request.AppendHeader(&viaHeader)

	// 设置Content-Type
	contentTypeHeader := sip.ContentTypeHeader("Application/MANSCDP+xml")
	request.AppendHeader(&contentTypeHeader)

	// 直接使用原始消息体
	request.SetBody(req.Body())

	// 设置传输协议
	request.SetTransport(strings.ToUpper(device.Transport))

	// 发送请求
	_, err = device.client.Do(p, request)
	if err != nil {
		p.Error("发送预置位查询命令失败", "error", err.Error())
		return fmt.Errorf("send preset query command failed: %v", err)
	}

	return nil
}

// GetKey 返回平台的唯一标识符
func (p *Platform) GetKey() string {
	return p.PlatformModel.ServerGBID
}
