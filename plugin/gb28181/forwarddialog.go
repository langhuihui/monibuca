package plugin_gb28181pro

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	sipgo "github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	m7s "m7s.live/v5"
	"m7s.live/v5/pkg/task"
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
	platformSSRC   string // 上级平台的SSRC
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

	if d.gb.MediaPort.Valid() {
		select {
		case d.MediaPort = <-d.gb.tcpPorts:
			defer func() {
				d.gb.tcpPorts <- d.MediaPort
			}()
		default:
			return fmt.Errorf("no available tcp port")
		}
	} else {
		d.MediaPort = d.gb.MediaPort[0]
	}

	// 使用上级平台的SSRC（如果有）或者设备的CreateSSRC方法
	var ssrcValue uint16
	if d.platformSSRC != "" {
		// 使用上级平台的SSRC
		if ssrcInt, err := strconv.ParseUint(d.platformSSRC, 10, 32); err == nil {
			d.SSRC = uint32(ssrcInt)
		} else {
			d.gb.Error("parse platform ssrc error", "err", err)
			// 使用设备的CreateSSRC方法作为备选
			ssrcValue = device.CreateSSRC(d.gb.Serial)
			d.SSRC = uint32(ssrcValue)
		}
	} else {
		// 使用设备的CreateSSRC方法
		ssrcValue = device.CreateSSRC(d.gb.Serial)
		d.SSRC = uint32(ssrcValue)
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

	sdpInfo = append(sdpInfo, fmt.Sprintf("m=video %d TCP/RTP/AVP 96", d.MediaPort))
	sdpInfo = append(sdpInfo, "a=recvonly")
	sdpInfo = append(sdpInfo, "a=rtpmap:96 PS/90000")
	//sdpInfo = append(sdpInfo, "a=rtpmap:98 H264/90000")
	//sdpInfo = append(sdpInfo, "a=rtpmap:97 MPEG4/90000")

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
		d.Stop(errors.New("do not support udp mode"))
	default:
		sdpInfo = append(sdpInfo,
			"a=setup:passive",
			"a=connection:new",
		)
	}
	if d.SSRC == 0 {
		d.SSRC = uint32(ssrcValue)
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

	viaHeader := sip.ViaHeader{
		ProtocolName:    "SIP",
		ProtocolVersion: "2.0",
		Transport:       "UDP",
		Host:            device.SipIp,
		Port:            device.LocalPort,
		Params:          sip.HeaderParams(sip.NewParams()),
	}
	viaHeader.Params.Add("branch", sip.GenerateBranchN(16)).Add("rport", "")
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
	// 创建会话 - 使用device的dialogClient创建
	dialogClientCache := sipgo.NewDialogClientCache(device.client, device.contactHDR)
	//d.session, err = dialogClientCache.Invite(d.gb, recipient, request.Body(), &fromHDR, &toHeader, &viaHeader, subjectHeader, &contentTypeHeader)
	d.session, err = dialogClientCache.Invite(d.gb, recipient, []byte(strings.Join(sdpInfo, "\r\n")+"\r\n"), &fromHDR, &toHeader, subjectHeader, &contentTypeHeader)
	return
}

// Run 运行会话
func (d *ForwardDialog) Run() (err error) {
	d.channel.Info("before WaitAnswer")
	err = d.session.WaitAnswer(d.gb, sipgo.AnswerOptions{})
	d.channel.Info("after WaitAnswer")
	if err != nil {
		return
	}
	inviteResponseBody := string(d.session.InviteResponse.Body())
	d.channel.Info("inviteResponse", "body", inviteResponseBody)
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
				parts := strings.Split(ls[1], " ")
				if len(parts) >= 2 {
					if port, err := strconv.Atoi(parts[1]); err == nil {
						d.ForwardConfig.Source.Port = uint32(port)
					}
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
		d.gb.Error("ack session err", err)
		d.Stop(errors.New("ack session err" + err.Error()))
	}

	// 更新 ForwardConfig 中的 SSRC
	d.ForwardConfig.Source.SSRC = d.SSRC

	// 设置源和目标配置
	// Source 模式由设备决定
	d.ForwardConfig.Source.Mode = d.channel.Device.StreamMode

	// Target 模式应该根据平台配置或默认设置
	// 这里可以根据实际需求设置，比如从平台配置中获取
	// 暂时使用默认的 TCP-PASSIVE 模式
	d.ForwardConfig.Target.Mode = mrtp.StreamModeTCPPassive

	// 解析目标SSRC
	if d.ForwardConfig.Target.SSRC == 0 && d.platformSSRC != "" {
		if ssrcInt, err := strconv.ParseUint(d.platformSSRC, 10, 32); err == nil {
			d.ForwardConfig.Target.SSRC = uint32(ssrcInt)
		} else {
			d.gb.Error("parse platform ssrc error", "err", err)
		}
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

// Dispose 释放会话资源
func (d *ForwardDialog) Dispose() {
	if d.session != nil {
		err := d.session.Bye(d)
		if err != nil {
			d.Error("forwarddialog bye bye err", err)
		}
		err = d.session.Close()
		if err != nil {
			d.Error("forwarddialog close session err", err)
		}
	}
}
