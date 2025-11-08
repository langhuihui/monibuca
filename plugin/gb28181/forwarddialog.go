package plugin_gb28181pro

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	sipgo "github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/langhuihui/gotask"
	m7s "m7s.live/v5"
	"m7s.live/v5/pkg/util"
	gb28181 "m7s.live/v5/plugin/gb28181/pkg"
	mrtp "m7s.live/v5/plugin/rtp/pkg"
)

// ForwardDialog 是用于转发RTP流的会话结构体
type ForwardDialog struct {
	task.Job
	channel *Channel
	gb28181.InviteOptions
	gb        *GB28181Plugin
	session   *sipgo.DialogClientSession
	pullCtx   m7s.PullJob
	forwarder *mrtp.Forwarder
	// 嵌入 ForwardConfig 来管理转发配置
	ForwardConfig  mrtp.ForwardConfig
	platformCallId string //上级平台发起invite的callid
	platformSSRC   uint32 // 上级平台的SSRC
	start          int64
	end            int64
}

// GetCallID 获取会话的CallID
func (d *ForwardDialog) GetCallID() string {

	return d.session.InviteRequest.CallID().Value()
}

// GetPullJob 获取拉流任务
func (d *ForwardDialog) GetPullJob() *m7s.PullJob {
	return &d.pullCtx
}

// GetKey 获取会话标识符
func (d *ForwardDialog) GetKey() uint32 {
	return d.SSRC
}

// Start 启动会话
func (d *ForwardDialog) Start() (err error) {
	// 处理时间范围
	isLive := true
	if d.start > 0 && d.end > 0 {
		isLive = false
		d.pullCtx.PublishConfig.PubType = m7s.PublishTypeVod
	}
	//err = d.pullCtx.Publish()
	if err != nil {
		return
	}
	sss := strings.Split(d.pullCtx.RemoteURL, "/")
	deviceId, channelId := sss[0], sss[1]
	var device *Device
	if deviceTmp, ok := d.gb.devices.Get(deviceId); ok {
		device = deviceTmp
		if channel, ok := deviceTmp.channels.Get(deviceId + "_" + channelId); ok {
			d.channel = channel
		} else {
			return fmt.Errorf("channel %s not found", channelId)
		}
	} else {
		return fmt.Errorf("device %s not found", deviceId)
	}

	// 注册对话到集合，使用类型转换
	d.MediaPort = uint16(0)

	d.Debug("ForwardDialog端口分配", "device.StreamMode", device.StreamMode, "StreamModeTCPActive", mrtp.StreamModeTCPActive)
	
	if device.StreamMode != mrtp.StreamModeTCPActive {
		if d.gb.MediaPort.Valid() {
			d.Debug("ForwardDialog端口分配路径", "path", "tcpPB.Allocate()", "MediaPort.Valid", true)
			var ok bool
			d.MediaPort, ok = d.gb.tcpPB.Allocate()
			if !ok {
				return fmt.Errorf("no available tcp port")
			}
			d.Debug("ForwardDialog端口分配成功", "allocatedPort", d.MediaPort)
		} else {
			d.Debug("ForwardDialog端口分配路径", "path", "MediaPort[0]", "MediaPort.Valid", false)
			d.MediaPort = d.gb.MediaPort[0]
			d.Debug("ForwardDialog端口分配成功", "defaultPort", d.MediaPort)
		}
	} else {
		d.Debug("ForwardDialog端口分配", "path", "StreamModeTCPActive，不分配端口", "MediaPort", d.MediaPort)
	}

	// 使用上级平台的SSRC（如果有）或者设备的CreateSSRC方法
	if d.platformSSRC != 0 {
		// 使用上级平台的SSRC
		d.SSRC = d.platformSSRC
	} else {
		// 使用设备的CreateSSRC方法
		d.SSRC = device.CreateSSRC(d.gb.Serial)
	}

	// 构建 SDP 内容
	sdpInfo := []string{
		"v=0",
		fmt.Sprintf("o=%s 0 0 IN IP4 %s", device.DeviceId, device.MediaIp),
		fmt.Sprintf("s=%s", util.Conditional(isLive, "Play", "Playback")), // 根据是否有时间参数决定
	}

	// 非直播模式下添加u行，保持在s=和c=之间
	if !isLive {
		sdpInfo = append(sdpInfo, fmt.Sprintf("u=%s:0", channelId))
	}

	// 添加c行
	sdpInfo = append(sdpInfo, "c=IN IP4 "+device.MediaIp)

	// 将字符串时间转换为 Unix 时间戳
	if !isLive {
		// 直接使用字符串格式的日期时间转换为秒级时间戳，不考虑时区问题
		sdpInfo = append(sdpInfo, fmt.Sprintf("t=%d %d", d.start, d.end))
	} else {
		sdpInfo = append(sdpInfo, "t=0 0")
	}

	// 添加媒体行和相关属性
	var mediaLine string
	switch device.StreamMode {
	case mrtp.StreamModeTCPPassive, mrtp.StreamModeTCPActive:
		mediaLine = fmt.Sprintf("m=video %d TCP/RTP/AVP 96", d.MediaPort)
	case mrtp.StreamModeUDP:
		mediaLine = fmt.Sprintf("m=video %d RTP/AVP 96", d.MediaPort)
	default:
		mediaLine = fmt.Sprintf("m=video %d TCP/RTP/AVP 96", d.MediaPort)
	}

	sdpInfo = append(sdpInfo, mediaLine)

	sdpInfo = append(sdpInfo, "a=recvonly")
	sdpInfo = append(sdpInfo, "a=rtpmap:96 PS/90000")

	//根据传输模式添加 setup 和 connection 属性
	switch device.StreamMode {
	case mrtp.StreamModeTCPPassive:
		sdpInfo = append(sdpInfo,
			"a=setup:passive",
			"a=connection:new",
		)
	case mrtp.StreamModeTCPActive:
		sdpInfo = append(sdpInfo,
			"a=setup:active",
			"a=connection:new",
		)
	case mrtp.StreamModeUDP:
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

	// 将SSRC转换为字符串格式
	ssrcStr := strconv.FormatUint(uint64(d.SSRC), 10)
	sdpInfo = append(sdpInfo, fmt.Sprintf("y=%s", ssrcStr))

	// 创建INVITE请求
	request := sip.NewRequest(sip.INVITE, sip.Uri{User: channelId, Host: device.IP})
	// 使用字符串格式的SSRC
	subject := fmt.Sprintf("%s:%s,%s:0", channelId, ssrcStr, deviceId)

	// 创建自定义头部
	contentTypeHeader := sip.ContentTypeHeader("APPLICATION/SDP")
	subjectHeader := sip.NewHeader("Subject", subject)

	// 设置请求体
	request.SetBody([]byte(strings.Join(sdpInfo, "\r\n") + "\r\n"))

	recipient := device.Recipient
	recipient.User = channelId

	fromHDR := sip.FromHeader{
		Address: sip.Uri{
			User: d.gb.Serial,
			Host: device.MediaIp,
			Port: device.LocalPort,
		},
		Params: sip.NewParams(),
	}
	toHeader := sip.ToHeader{
		Address: sip.Uri{User: channelId, Host: channelId[0:10]},
	}
	fromHDR.Params.Add("tag", sip.GenerateTagN(16))

	// 输出设备Transport信息用于调试
	d.Info("ForwardDialog准备发送INVITE", "deviceId", device.DeviceId, "device.Transport", device.Transport, "device.SipIp", device.SipIp, "device.LocalPort", device.LocalPort)

	// 创建会话 - 使用device的dialogClient创建
	dialogClientCache := sipgo.NewDialogClientCache(device.client, device.contactHDR)
	d.Info("start to invite", "recipient:", recipient, " fromHDR:", fromHDR, " toHeader:", toHeader, " device.contactHDR:", device.contactHDR, "contactHDR:", device.contactHDR)

	// 创建Via头部，使用设备的Transport协议
	// Via头部必须放在第一个位置
	viaHeader := &sip.ViaHeader{
		ProtocolName:    "SIP",
		ProtocolVersion: "2.0",
		Transport:       device.Transport, // 使用设备注册时的Transport
		Host:            device.SipIp,
		Port:            device.LocalPort,
		Params:          sip.HeaderParams(sip.NewParams()),
	}
	viaHeader.Params.Add("branch", sip.GenerateBranchN(16))

	d.Info("ForwardDialog发送INVITE使用Transport", "transport", device.Transport, "via", viaHeader)

	// Via头部必须是第一个参数！
	d.session, err = dialogClientCache.Invite(d.gb, recipient, []byte(strings.Join(sdpInfo, "\r\n")+"\r\n"), viaHeader, &fromHDR, &toHeader, subjectHeader, &contentTypeHeader)
	return
}

// Run 运行会话
func (d *ForwardDialog) Run() (err error) {
	err = d.session.WaitAnswer(d.gb, sipgo.AnswerOptions{})
	if err != nil {
		return
	}
	inviteResponseBody := string(d.session.InviteResponse.Body())
	d.Info("inviteResponse", "body", inviteResponseBody)
	ds := strings.Split(inviteResponseBody, "\r\n")
	for _, l := range ds {
		if ls := strings.Split(l, "="); len(ls) > 1 {
			switch ls[0] {
			case "y":
				if len(ls[1]) > 0 {
					if _ssrc, err := strconv.ParseInt(ls[1], 10, 0); err == nil {
						d.SSRC = uint32(_ssrc)
					} else {
						d.gb.Error("read invite response y ", "err", err)
					}
				}
			case "c":
				// 解析 c=IN IP4 xxx.xxx.xxx.xxx 格式
				parts := strings.Split(ls[1], " ")
				if len(parts) >= 3 {
					d.ForwardConfig.Source.IP = parts[len(parts)-1]
				}
			case "m":
				// 解析 m=video port xxx 格式
				if d.ForwardConfig.Source.Mode == mrtp.StreamModeTCPActive {
					parts := strings.Split(ls[1], " ")
					if len(parts) >= 2 {
						if port, err := strconv.Atoi(parts[1]); err == nil {
							d.ForwardConfig.Source.Port = uint16(port)
						}
					}
				} else {
					d.ForwardConfig.Source.Port = d.MediaPort
				}
			}
		}
	}
	if d.session.InviteResponse.Contact() != nil {
		if &d.session.InviteRequest.Recipient != &d.session.InviteResponse.Contact().Address {
			d.session.InviteResponse.Contact().Address = d.session.InviteRequest.Recipient
		}
	}
	err = d.session.Ack(d.gb)
	if err != nil {
		d.Error("ack session err", err)
		d.Stop(errors.New("ack session err" + err.Error()))
	}

	// 更新 ForwardConfig 中的 SSRC
	d.ForwardConfig.Source.SSRC = d.SSRC
	// 创建新的 Forwarder
	d.forwarder = mrtp.NewForwarder(&d.ForwardConfig)

	d.Info("forwarder started successfully",
		"source", fmt.Sprintf("%s:%d", d.ForwardConfig.Source.IP, d.ForwardConfig.Source.Port),
		"target", fmt.Sprintf("%s:%d", d.ForwardConfig.Target.IP, d.ForwardConfig.Target.Port),
		"sourceMode", d.ForwardConfig.Source.Mode,
		"targetMode", d.ForwardConfig.Target.Mode,
		"sourceSSRC", d.ForwardConfig.Source.SSRC,
		"targetSSRC", d.ForwardConfig.Target.SSRC)

	// 启动转发
	return d.forwarder.Forward(d)
}

// Dispose 释放会话资源
func (d *ForwardDialog) Dispose() {
	// 回收端口（如果是多端口模式）
	if d.MediaPort > 0 && d.gb.tcpPort == 0 {
		if !d.gb.tcpPB.Release(d.MediaPort) {
			d.Warn("port already released or not allocated", "port", d.MediaPort, "type", "tcp")
		}
	}
	if d.session != nil && d.session.InviteResponse != nil {
		err := d.session.Bye(d)
		if err != nil {
			d.Error("forwarddialog bye bye err", err)
		}
	}
}
