package plugin_gb28181pro

import (
	"fmt"
	sipgo "github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	m7s "m7s.live/v5"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/pkg/util"
	gb28181 "m7s.live/v5/plugin/gb28181pro/pkg"
	"strconv"
	"strings"
)

// ForwardDialog 是用于转发RTP流的会话结构体
type ForwardDialog struct {
	task.Job
	channel *Channel
	gb28181.InviteOptions
	gb             *GB28181ProPlugin
	session        *sipgo.DialogClientSession
	pullCtx        m7s.PullJob
	forwarder      *gb28181.RTPForwarder
	platformIP     string
	platformPort   int
	platformSSRC   string // 上级平台的SSRC
	platformCallId string //上级平台发起invite的callid
	// 是否为TCP传输
	TCP bool
	// 是否为TCP主动模式
	TCPActive bool
	start     int64
	end       int64
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
		if channel, ok := deviceTmp.channels.Get(channelId); ok {
			d.channel = channel
		} else {
			return fmt.Errorf("channel %s not found", channelId)
		}
	} else {
		return fmt.Errorf("device %s not found", deviceId)
	}

	// 注册对话到集合，使用类型转换
	d.gb.forwardDialogs.Set(d)
	//defer d.gb.forwardDialogs.Remove(d)

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

	// 获取 SDP IP
	sdpIP := device.LocalIP
	if sdpIP == "" {
		sdpIP = device.mediaIp
	}

	// 构建 SDP 内容
	sdpInfo := []string{
		"v=0",
		fmt.Sprintf("o=%s 0 0 IN IP4 %s", device.DeviceID, sdpIP),
		fmt.Sprintf("s=%s", util.Conditional(isLive, "Play", "Playback")), // 根据是否有时间参数决定
	}

	// 非直播模式下添加u行，保持在s=和c=之间
	if !isLive {
		sdpInfo = append(sdpInfo, fmt.Sprintf("u=%s:0", channelId))
	}

	// 添加c行
	sdpInfo = append(sdpInfo, "c=IN IP4 "+sdpIP)

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
	sdpInfo = append(sdpInfo, "a=setup:passive")
	sdpInfo = append(sdpInfo, "a=connection:new")

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
		Host:            device.LocalIP,
		Port:            device.LocalPort,
		Params:          sip.HeaderParams(sip.NewParams()),
	}
	viaHeader.Params.Add("branch", sip.GenerateBranchN(16)).Add("rport", "")

	// 创建会话 - 使用device的dialogClient创建
	d.session, err = device.dialogClient.Invite(d.gb, recipient, request.Body(), &device.fromHDR, &viaHeader, &device.contactHDR, subjectHeader, &contentTypeHeader)

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
			if ls[0] == "y" && len(ls[1]) > 0 {
				if _ssrc, err := strconv.ParseInt(ls[1], 10, 0); err == nil {
					d.SSRC = uint32(_ssrc)
				} else {
					d.gb.Error("read invite response y ", "err", err)
				}
			}
			if ls[0] == "m" && len(ls[1]) > 0 {
				netinfo := strings.Split(ls[1], " ")
				if strings.ToUpper(netinfo[2]) == "TCP/RTP/AVP" {
					d.gb.Debug("device support tcp")
				} else {
					d.gb.Debug("device using udp")
				}
			}
		}
	}
	err = d.session.Ack(d.gb)

	// 创建并初始化RTPForwarder
	d.forwarder = gb28181.NewRTPForwarder()
	d.forwarder.TCP = d.TCP
	d.forwarder.TCPActive = d.TCPActive

	// 设置监听地址和端口
	d.forwarder.ListenAddr = fmt.Sprintf(":%d", d.MediaPort)
	d.forwarder.ListenPort = d.MediaPort

	// 设置转发目标
	if d.platformIP != "" && d.platformPort > 0 {
		err = d.forwarder.SetTarget(d.platformIP, d.platformPort)
		if err != nil {
			d.Error("set target error", "err", err)
			return err
		}
	} else {
		d.Error("no target set, will only receive but not forward")
		return
	}

	// 设置目标SSRC
	if d.platformSSRC != "" {
		d.forwarder.TargetSSRC = d.platformSSRC
		d.Info("set target ssrc", "ssrc", d.platformSSRC)
	}

	// 将forwarder添加到任务中
	d.AddTask(d.forwarder)

	d.Info("forwarder started successfully",
		"TCP", d.forwarder.TCP,
		"TCPActive", d.forwarder.TCPActive,
		"listen", d.forwarder.ListenAddr,
		"target", fmt.Sprintf("%s:%d", d.platformIP, d.platformPort),
		"ssrc", d.platformSSRC)

	// 使用goroutine启动Demux，避免阻塞
	d.forwarder.Demux()

	return
}

// Dispose 释放会话资源
func (d *ForwardDialog) Dispose() {
	if d.session != nil {
		err := d.session.Bye(d)
		if err != nil {
			d.Error("bye bye err", err)
		}
		err = d.session.Close()
		if err != nil {
			d.Error("close session err", err)
		}
	}
}
