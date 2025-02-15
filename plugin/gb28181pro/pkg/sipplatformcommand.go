package gb28181

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	myip "github.com/husanpao/ip"
	"github.com/icholy/digest"
	"m7s.live/v5/pkg/task"
)

// InitializeSIPClient 初始化SIP客户端
func (p *Platform) InitializeSIPClient(ua *sipgo.UserAgent) error {
	localIP := myip.InternalIPv4()
	var err error
	p.Client, err = sipgo.NewClient(ua, sipgo.WithClientHostname(localIP))
	if err != nil {
		return fmt.Errorf("failed to create sip client: %v", err)
	}

	// 设置联系人头部，使用本地平台的信息
	p.ContactHDR = sip.ContactHeader{
		Address: sip.Uri{
			User: p.DeviceGBID,
			Host: p.DeviceIP,
			Port: p.DevicePort,
		},
	}

	// 设置From头部，使用本地平台的信息
	p.FromHDR = sip.FromHeader{
		Address: sip.Uri{
			User: p.DeviceGBID,
			Host: p.ServerGBDomain,
		},
		Params: sip.NewParams(),
	}
	p.FromHDR.Params.Add("tag", sip.GenerateTagN(16))

	// 创建对话客户端
	p.DialogClient = sipgo.NewDialogClient(p.Client, p.ContactHDR)

	return nil
}

// Register 发送注册请求到上级平台
func (p *Platform) Register(ctx context.Context) (*sipgo.DialogClientSession, error) {
	// 创建注册请求的目标URI，使用上级平台的信息
	recipient := sip.Uri{
		User: p.ServerGBID,
		Host: p.ServerIP,
		Port: p.ServerPort,
	}

	// 创建基本的REGISTER请求
	req := sip.NewRequest(sip.REGISTER, recipient)

	// 添加Contact头部
	contactStr := fmt.Sprintf("<sip:%s@%s:%d>", p.DeviceGBID, p.DeviceIP, p.DevicePort)
	req.AppendHeader(sip.NewHeader("Contact", contactStr))

	// 添加From头部
	fromHeader := sip.FromHeader{
		Address: sip.Uri{
			User: p.DeviceGBID,
			Host: p.ServerGBDomain,
		},
		Params: sip.NewParams(),
	}
	fromHeader.Params.Add("tag", sip.GenerateTagN(16))
	req.AppendHeader(&fromHeader)

	// 添加To头部
	toHeader := sip.ToHeader{
		Address: sip.Uri{
			User: p.DeviceGBID,
			Host: p.ServerGBDomain,
		},
	}
	req.AppendHeader(&toHeader)

	// 添加Expires头部
	req.AppendHeader(sip.NewHeader("Expires", fmt.Sprintf("%d", p.Expires)))

	// 设置传输协议
	req.SetTransport(strings.ToUpper(p.Transport))

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
			Username: p.Username,
			Password: p.Password,
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
		User: p.ServerGBID,
		Host: p.ServerIP,
		Port: p.ServerPort,
	}

	req := sip.NewRequest("MESSAGE", recipient)
	req.SetTransport(strings.ToUpper(p.Transport))

	// 添加From头部
	fromHeader := sip.FromHeader{
		Address: sip.Uri{
			User: p.DeviceGBID,
			Host: p.ServerGBDomain,
		},
		Params: sip.NewParams(),
	}
	fromHeader.Params.Add("tag", sip.GenerateTagN(16))
	req.AppendHeader(&fromHeader)

	// 添加To头部
	toHeader := sip.ToHeader{
		Address: sip.Uri{
			User: p.ServerGBID,
			Host: p.ServerGBDomain,
		},
	}
	req.AppendHeader(&toHeader)

	// 添加Contact头部
	contactStr := fmt.Sprintf("<sip:%s@%s:%d>", p.DeviceGBID, p.DeviceIP, p.DevicePort)
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
		User: p.ServerGBID,
		Host: p.ServerIP,
		Port: p.ServerPort,
	}

	// 创建基本的REGISTER请求
	req := sip.NewRequest(sip.REGISTER, recipient)

	// 添加Contact头部
	contactStr := fmt.Sprintf("<sip:%s@%s:%d>", p.DeviceGBID, p.DeviceIP, p.DevicePort)
	req.AppendHeader(sip.NewHeader("Contact", contactStr))

	// 添加From头部
	fromHeader := sip.FromHeader{
		Address: sip.Uri{
			User: p.DeviceGBID,
			Host: p.ServerGBDomain,
		},
		Params: sip.NewParams(),
	}
	fromHeader.Params.Add("tag", sip.GenerateTagN(16))
	req.AppendHeader(&fromHeader)

	// 添加To头部
	toHeader := sip.ToHeader{
		Address: sip.Uri{
			User: p.DeviceGBID,
			Host: p.ServerGBDomain,
		},
	}
	req.AppendHeader(&toHeader)

	// 添加Expires头部，设置为0表示注销
	req.AppendHeader(sip.NewHeader("Expires", "0"))

	// 设置传输协议
	req.SetTransport(strings.ToUpper(p.Transport))

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

// RegisterTask 处理定时注册
type RegisterTask struct {
	task.TickTask
	platform *Platform
}

func (r *RegisterTask) GetTickInterval() time.Duration {
	return time.Second * time.Duration(r.platform.Expires)
}

func (r *RegisterTask) Tick(any) {
	if !r.platform.Enable {
		r.platform.Status = false
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
		r.platform.IncrementRegisterAliveReply()
		r.Error("register", "error", err.Error(), "retries", r.platform.RegisterAliveReply)
		if r.platform.RegisterAliveReply >= 3 {
			r.platform.Status = false
			r.platform.CurrentSession = nil
			r.Stop(fmt.Errorf("max retries reached: %d", r.platform.RegisterAliveReply))
		}
		return
	}

	r.Info("register", "status", "success")
	r.platform.Status = true
	r.platform.CurrentSession = session
	r.platform.ResetRegisterAliveReply()
}

// StartRegisterTask 启动注册任务
func (p *Platform) StartRegisterTask() {
	ctx := context.Background()

	// 首次注册
	session, err := p.Register(ctx)
	if err != nil {
		p.Status = false
		p.IncrementRegisterAliveReply()
		// 注册失败，启动定时注册任务
		var rt RegisterTask
		rt.platform = p
		p.AddTask(&rt)
		return
	}

	// 注册成功，更新状态
	p.Status = true
	p.CurrentSession = session
	p.ResetRegisterAliveReply()
}
