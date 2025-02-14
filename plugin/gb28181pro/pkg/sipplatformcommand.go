package gb28181

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/icholy/digest"
)

// InitializeSIPClient 初始化SIP客户端
func (p *Platform) InitializeSIPClient(ua *sipgo.UserAgent, localIP string) error {
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
func (p *Platform) Register(ctx context.Context, plugin interface{}) (*sipgo.DialogClientSession, error) {
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
		return nil, fmt.Errorf("创建事务失败: %v", err)
	}
	defer tx.Terminate()

	// 获取响应
	res, err := p.getResponse(tx)
	if err != nil {
		return nil, fmt.Errorf("获取响应失败: %v", err)
	}

	// 处理401未授权响应
	if res.StatusCode == 401 {
		// 获取WWW-Authenticate头部
		wwwAuth := res.GetHeader("WWW-Authenticate")
		if wwwAuth == nil {
			return nil, fmt.Errorf("未收到认证质询")
		}

		// 解析认证质询
		chal, err := digest.ParseChallenge(wwwAuth.Value())
		if err != nil {
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
		return nil, fmt.Errorf("注册失败，状态码: %d", res.StatusCode)
	}

	// 注册成功，不需要维护会话状态
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
func (p *Platform) Keepalive(ctx context.Context, plugin interface{}) (*sipgo.DialogClientSession, error) {
	// TODO: 实现心跳逻辑
	return nil, nil
}

// Unregister 发送注销请求到上级平台
func (p *Platform) Unregister(ctx context.Context, plugin interface{}) (*sipgo.DialogClientSession, error) {
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

// StartRegisterTask 启动注册任务
// 这个方法会在平台启用时被调用，负责处理注册和保活
func (p *Platform) StartRegisterTask(plugin interface{}) {
	ctx := context.Background()

	// 首次注册
	session, err := p.Register(ctx, plugin)
	if err != nil {
		// TODO: 处理注册失败
		return
	}

	// 保存当前会话
	p.CurrentSession = session

	// 启动保活协程
	go func() {
		ticker := time.NewTicker(time.Duration(p.KeepTimeout) * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			if !p.Enable {
				// 如果平台被禁用，发送注销请求并退出
				_, _ = p.Unregister(ctx, plugin)
				return
			}

			// 发送心跳
			_, err := p.Keepalive(ctx, plugin)
			if err != nil {
				// TODO: 处理心跳失败
			}
		}
	}()
}
