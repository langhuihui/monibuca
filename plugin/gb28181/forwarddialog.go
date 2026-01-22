package plugin_gb28181pro

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	sipgo "github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	task "github.com/langhuihui/gotask"
	m7s "m7s.live/v5"
	"m7s.live/v5/pkg/util"
	mrtp "m7s.live/v5/plugin/rtp/pkg"
)

// ForwardDialog 是用于转发RTP流的会话结构体
type ForwardDialog struct {
	task.Job
	channel   *Channel
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
	return d.platformSSRC
}

// Start 启动会话
func (d *ForwardDialog) Start() (err error) {
	// 处理时间范围
	isLive := true
	if d.start > 0 && d.end > 0 {
		isLive = false
		d.pullCtx.PublishConfig.PubType = m7s.PublishTypeVod
	}
	err = d.pullCtx.Publish()
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

	streamMode := d.ForwardConfig.Source.Mode // ForwardDialog使用Target的Mode作为StreamMode

	switch streamMode {
	case mrtp.StreamModeTCPPassive:
		if d.gb.tcpPort > 0 {
			d.ForwardConfig.Source.Port = d.gb.tcpPort
		} else {
			if d.gb.MediaPort.Valid() {
				var ok bool
				d.ForwardConfig.Source.Port, ok = d.gb.tcpPB.Allocate()
				if !ok {
					d.Error("[PORT_ALLOCATE_FAILED] TCP端口分配失败 - 无可用端口 (ForwardDialog)", "platformId", d.platformCallId, "channelId", d.channel.ChannelId)
					return fmt.Errorf("no available tcp port")
				}
				d.Info("[PORT_ALLOCATE_SUCCESS] TCP端口分配成功 (ForwardDialog)", "port", d.ForwardConfig.Source.Port, "platformId", d.platformCallId, "channelId", d.channel.ChannelId, "streamMode", streamMode)
				d.gb.updatePortStats()
			} else {
				d.ForwardConfig.Source.Port = d.gb.MediaPort[0]
			}
		}
	case mrtp.StreamModeUDP:
		if d.gb.udpPort > 0 {
			d.ForwardConfig.Source.Port = d.gb.udpPort
		} else {
			if d.gb.MediaPort.Valid() {
				var ok bool
				d.ForwardConfig.Source.Port, ok = d.gb.udpPB.Allocate()
				if !ok {
					d.Error("[PORT_ALLOCATE_FAILED] UDP端口分配失败 - 无可用端口 (ForwardDialog)", "platformId", d.platformCallId, "channelId", d.channel.ChannelId)
					return fmt.Errorf("no available udp port")
				}
				d.Info("[PORT_ALLOCATE_SUCCESS] UDP端口分配成功 (ForwardDialog)", "port", d.ForwardConfig.Source.Port, "platformId", d.platformCallId, "channelId", d.channel.ChannelId, "streamMode", streamMode)
				d.gb.updatePortStats()
			} else {
				d.ForwardConfig.Source.Port = d.gb.MediaPort[0]
			}
		}
	case mrtp.StreamModeTCPActive:
		// TCP Active 模式不需要分配端口
		d.Debug("ForwardDialog端口分配", "path", "StreamModeTCPActive，不分配端口", "streamMode", streamMode)
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
	switch streamMode {
	case mrtp.StreamModeTCPPassive, mrtp.StreamModeTCPActive:
		mediaLine = fmt.Sprintf("m=video %d TCP/RTP/AVP 96", d.ForwardConfig.Source.Port)
	case mrtp.StreamModeUDP:
		mediaLine = fmt.Sprintf("m=video %d RTP/AVP 96", d.ForwardConfig.Source.Port)
	default:
		mediaLine = fmt.Sprintf("m=video %d TCP/RTP/AVP 96", d.ForwardConfig.Source.Port)
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
	ssrcStr := strconv.FormatUint(uint64(d.platformSSRC), 10)
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
	d.Info("start to invite", "recipient:", recipient, " fromHDR:", fromHDR, " toHeader:", toHeader, " device.contactHDR:", device.contactHDR, "contactHDR:",
		device.contactHDR, "sdpInfo:", strings.Join(sdpInfo, "|||"), "viaHeader:", viaHeader, "transport", device.Transport)
	// Via头部必须是第一个参数！
	d.session, err = dialogClientCache.Invite(d, recipient, []byte(strings.Join(sdpInfo, "\r\n")+"\r\n"), viaHeader, &fromHDR, &toHeader, subjectHeader, &contentTypeHeader)

	if err != nil {
		d.Error("ForwardDialog invite error: " + err.Error())
		// SIP邀请失败时释放已分配的端口
		d.releaseAllocatedSourcePort()
		return err
	}

	d.SetDescriptions(task.Description{
		"targetStreamMode": d.ForwardConfig.Target.Mode,
		"targetMediaPort":  d.ForwardConfig.Target.Port,
		"sourceStreamMode": d.ForwardConfig.Source.Mode,
		"sourceMediaPort":  d.ForwardConfig.Source.Port,
		"mediaIP":          device.MediaIp,
		"sipIP":            device.SipIp,
		"transport":        device.Transport,
		"ssrc":             d.platformSSRC,
		"callID":           d.platformCallId,
		"deviceID":         device.DeviceId,
		"channelID":        d.channel.ChannelId,
		"deviceIP":         device.IP,
		"devicePort":       device.Port,
		"localPort":        device.LocalPort,
		"startTime":        time.Now(),
		"from":             fromHDR.Address.String(),
		"to":               toHeader.Address.String(),
		"contact":          device.contactHDR.Address.String(),
		"subject":          subject,
		"recipient":        recipient.String(),
		"sdp":              strings.Join(sdpInfo, "\r\n"),
		"via":              viaHeader.String(),
		"viaBranch":        func() string { v, _ := viaHeader.Params.Get("branch"); return v }(),
	})
	return
}

// Run 运行会话
func (d *ForwardDialog) Run() (err error) {
	err = d.session.WaitAnswer(d, sipgo.AnswerOptions{})
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
						d.ForwardConfig.Source.SSRC = uint32(_ssrc)
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
				}
			}
		}
	}
	if d.session.InviteResponse.Contact() != nil {
		d.session.InviteResponse.Contact().Address = d.session.InviteRequest.Recipient
	}
	err = d.session.Ack(d)
	if err != nil {
		d.Error("ack session err", err)
		d.Stop(errors.New("ack session err" + err.Error()))
	}
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

// releaseAllocatedSourcePort 释放已分配的Source端口（用于启动失败时的清理）
func (d *ForwardDialog) releaseAllocatedSourcePort() {
	streamMode := d.ForwardConfig.Source.Mode // 使用Source.Mode，因为这是我们在OnInvite中设置的模式
	sourcePort := d.ForwardConfig.Source.Port // 使用分配的端口

	switch streamMode {
	case mrtp.StreamModeTCPPassive:
		if d.gb.tcpPort == 0 && sourcePort > 0 { // 多端口模式且分配了端口
			// 回收端口，防止重复回收
			if !d.gb.tcpPB.Release(sourcePort) {
				d.Warn("[PORT_RELEASE_FAILED] Source TCP端口回收失败 - 端口已被释放或未分配 (ForwardDialog)", "port", sourcePort, "platformId", d.platformCallId, "channelId", d.channel.ChannelId, "streamMode", streamMode)
			} else {
				d.Info("[PORT_RELEASE_SUCCESS] Source TCP端口回收成功 (ForwardDialog)", "port", sourcePort, "platformId", d.platformCallId, "channelId", d.channel.ChannelId, "streamMode", streamMode)
				d.gb.updatePortStats()
			}
		}
	case mrtp.StreamModeUDP:
		if d.gb.udpPort == 0 && sourcePort > 0 { // 多端口模式且分配了端口
			// 回收端口，防止重复回收
			if !d.gb.udpPB.Release(sourcePort) {
				d.Warn("[PORT_RELEASE_FAILED] Source UDP端口回收失败 - 端口已被释放或未分配 (ForwardDialog)", "port", sourcePort, "platformId", d.platformCallId, "channelId", d.channel.ChannelId, "streamMode", streamMode)
			} else {
				d.Info("[PORT_RELEASE_SUCCESS] Source UDP端口回收成功 (ForwardDialog)", "port", sourcePort, "platformId", d.platformCallId, "channelId", d.channel.ChannelId, "streamMode", streamMode)
				d.gb.updatePortStats()
			}
		}
	case mrtp.StreamModeTCPActive:
		// TCP Active 模式：设备会连接我们，我们没有分配端口，无需回收
		d.Debug("ForwardDialog Dispose", "Source TCP Active模式，无需回收端口", "streamMode", streamMode)
	}

	d.Info("ForwardDialog source port release", "sourcePort", sourcePort)
}

// Dispose 释放会话资源
func (d *ForwardDialog) Dispose() {
	go func() {
		time.Sleep(time.Second * 90) // 延迟90秒回收端口
		d.releaseAllocatedSourcePort()

		// 回收 Target 端口（上级平台，如果是在 OnInvite 中分配的）
		targetMode := d.ForwardConfig.Target.Mode
		targetPort := d.ForwardConfig.Target.Port

		switch targetMode {
		case mrtp.StreamModeTCPActive:
			if d.gb.tcpPort == 0 && targetPort > 0 { // 多端口模式且分配了端口
				// 回收端口，防止重复回收
				if !d.gb.tcpPB.Release(targetPort) {
					d.Warn("[PORT_RELEASE_FAILED] Target TCP端口回收失败 - 端口已被释放或未分配 (ForwardDialog)", "port", targetPort, "platformId", d.platformCallId, "channelId", d.channel.ChannelId, "streamMode", targetMode)
				} else {
					d.Info("[PORT_RELEASE_SUCCESS] Target TCP端口回收成功 (ForwardDialog)", "port", targetPort, "platformId", d.platformCallId, "channelId", d.channel.ChannelId, "streamMode", targetMode)
					d.gb.updatePortStats()
				}
			}
		case mrtp.StreamModeTCPPassive:
			// TCP Passive 模式：在 OnInvite 中没有分配端口，无需回收
			d.Debug("ForwardDialog Dispose", "Target TCP Passive模式，无需回收端口", "streamMode", targetMode)
		case mrtp.StreamModeUDP:
			// UDP 模式：在 OnInvite 中没有分配端口，无需回收
			d.Debug("ForwardDialog Dispose", "Target UDP模式，无需回收端口", "streamMode", targetMode)
		}

		d.Info("ForwardDialog target port release", "targetPort", targetPort)
	}()

	if d.session != nil && d.session.InviteResponse != nil {
		err := d.session.Bye(d)
		if err != nil {
			d.Error("forwarddialog bye bye err", err)
		}
	}
}
