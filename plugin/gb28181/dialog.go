package plugin_gb28181pro

import (
	"errors"
	"fmt"
	"math/rand"
	"net/url"
	"strconv"
	"strings"
	"time"

	sipgo "github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	task "github.com/langhuihui/gotask"
	m7s "m7s.live/v5"
	pkg "m7s.live/v5/pkg"
	"m7s.live/v5/pkg/util"
	gb28181 "m7s.live/v5/plugin/gb28181/pkg"
	mrtp "m7s.live/v5/plugin/rtp/pkg"
)

// Plugin-specific progress steps for GB28181
const (
	StepDeviceLookup pkg.StepName = "device_lookup"
	StepSIPPrepare   pkg.StepName = "sip_prepare"
	StepSDPBuild     pkg.StepName = "sdp_build"
	StepInviteSend   pkg.StepName = "invite_send"
	StepResponseWait pkg.StepName = "response_wait"
)

var gbPullSteps = []pkg.StepDef{
	{Name: pkg.StepPublish, Description: "Publishing stream"},
	{Name: StepDeviceLookup, Description: "Looking up device and channel"},
	{Name: StepSIPPrepare, Description: "Preparing SIP invitation"},
	{Name: StepSDPBuild, Description: "Building SDP content"},
	{Name: StepInviteSend, Description: "Sending SIP INVITE"},
	{Name: StepResponseWait, Description: "Waiting for response"},
	{Name: pkg.StepStreaming, Description: "Receiving media stream"},
}

type Dialog struct {
	task.Task
	Channel *Channel
	gb28181.InviteOptions
	gb         *GB28181Plugin
	session    *sipgo.DialogClientSession
	pullCtx    m7s.PullJob
	start      string
	end        string
	StreamMode mrtp.StreamMode // 数据流传输模式（UDP:udp传输/TCP-ACTIVE：tcp主动模式/TCP-PASSIVE：tcp被动模式）
	targetIP   string          // 目标设备的IP地址
	targetPort int             // 目标设备的端口
	/**
	子码流的配置,默认格式为:
	stream=stream:0;stream=stream:1
	GB28181-2022:
	stream=streanumber:0;stream=streamnumber:1
	大华为:
	stream=streamprofile:0;stream=streamprofile:1
	水星,tp-link:
	stream=streamMode:main;stream=streamMode:sub
	*/
	stream string
}

func (d *Dialog) GetCallID() string {
	if d.session != nil && d.session.InviteRequest != nil && d.session.InviteRequest.CallID() != nil {
		return d.session.InviteRequest.CallID().Value()
	} else {
		return ""
	}
}

func (d *Dialog) GetPullJob() *m7s.PullJob {
	return &d.pullCtx
}

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

var seededRand *rand.Rand = rand.New(rand.NewSource(time.Now().UnixNano()))

func GenerateCallID(length int) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}

func (d *Dialog) Start() (err error) {
	// Initialize progress tracking for pull operations
	d.pullCtx.SetProgressStepsDefs(gbPullSteps)

	// 处理时间范围
	d.InviteOptions.Start = d.start
	d.InviteOptions.End = d.End
	if d.IsLive() {
		d.pullCtx.PublishConfig.PubType = m7s.PublishTypeVod
	}
	err = d.pullCtx.Publish()
	if err != nil {
		d.pullCtx.Fail(err.Error())
		return
	}

	d.pullCtx.GoToStepConst(StepDeviceLookup)

	sss := strings.Split(d.pullCtx.RemoteURL, "/")
	if len(sss) < 2 {
		d.Info("remote url is invalid", d.pullCtx.RemoteURL)
		d.pullCtx.Fail("remote url is invalid")
		return
	}
	deviceId, channelId := sss[len(sss)-2], sss[len(sss)-1]
	var device *Device
	if deviceTmp, ok := d.gb.devices.Get(deviceId); ok {
		device = deviceTmp
		d.StreamMode = device.StreamMode
		if channel, ok := deviceTmp.channels.Get(deviceId + "_" + channelId); ok {
			d.Channel = channel
		} else if channel, ok := deviceTmp.channels.Find(func(c *Channel) bool {
			return c.CustomChannelId == channelId
		}); ok {
			channelId = channel.ChannelId
			d.Channel = channel
		} else {
			d.pullCtx.Fail(fmt.Sprintf("channel %s not found", channelId))
			return errors.Join(fmt.Errorf("channel %s not found", channelId))
		}
	} else {
		d.pullCtx.Fail(fmt.Sprintf("device %s not found", deviceId))
		return errors.Join(fmt.Errorf("device %s not found", deviceId))
	}

	d.pullCtx.GoToStepConst(StepSIPPrepare)

	//defer d.gb.dialogs.Remove(d)
	switch d.StreamMode {
	case mrtp.StreamModeTCPPassive:
		if d.gb.tcpPort > 0 {
			d.MediaPort = d.gb.tcpPort
		} else {
			if d.gb.MediaPort.Valid() {
				var ok bool
				d.MediaPort, ok = d.gb.tcpPB.Allocate()
				if !ok {
					d.pullCtx.Fail("no available tcp port")
					return errors.Join(fmt.Errorf("no available tcp port"))
				}
			} else {
				d.MediaPort = d.gb.MediaPort[0]
			}
		}
	case mrtp.StreamModeUDP:
		if d.gb.udpPort > 0 {
			d.MediaPort = d.gb.udpPort
		} else {
			if d.gb.MediaPort.Valid() {
				var ok bool
				d.MediaPort, ok = d.gb.udpPB.Allocate()
				if !ok {
					d.pullCtx.Fail("no available udp port")
					return fmt.Errorf("no available udp port")
				}
			} else {
				d.MediaPort = d.gb.MediaPort[0]
			}
		}
	}

	d.pullCtx.GoToStepConst(StepSDPBuild)

	ssrc := d.CreateSSRC(d.gb.Serial)

	// 构建 SDP 内容
	sdpInfo := []string{
		"v=0",
		fmt.Sprintf("o=%s 0 0 IN IP4 %s", channelId, device.SipIp),
		fmt.Sprintf("s=%s", util.Conditional(d.IsLive(), "Play", "Playback")), // 根据是否有时间参数决定
	}

	// 非直播模式下添加u行，保持在s=和c=之间
	if !d.IsLive() {
		sdpInfo = append(sdpInfo, fmt.Sprintf("u=%s:0", channelId))
	}

	// 添加c行
	sdpInfo = append(sdpInfo, "c=IN IP4 "+device.MediaIp)

	// 将字符串时间转换为 Unix 时间戳
	if !d.IsLive() {
		startTime, endTime, err := util.TimeRangeQueryParse(url.Values{"start": []string{d.start}, "end": []string{d.end}})
		if err != nil {
			return errors.New("parse end time error")
		}
		sdpInfo = append(sdpInfo, fmt.Sprintf("t=%d %d", startTime.Unix(), endTime.Unix()))
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
	if d.stream != "" {
		sdpInfo = append(sdpInfo, "a="+d.stream)
	}
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
	}

	// 添加 SSRC
	sdpInfo = append(sdpInfo, fmt.Sprintf("y=%s", ssrc))

	// 创建 INVITE 请求
	recipient := sip.Uri{
		Host: device.IP,
		Port: device.Port,
		User: channelId,
	}
	// 设置必需的头部
	contentTypeHeader := sip.ContentTypeHeader("APPLICATION/SDP")
	subjectHeader := sip.NewHeader("Subject", fmt.Sprintf("%s:%s,%s:0", channelId, ssrc, d.gb.Serial))
	toHeader := sip.ToHeader{
		Address: sip.Uri{User: channelId, Host: channelId[0:10]},
	}
	userAgentHeader := sip.NewHeader("User-Agent", "M7S/"+m7s.Version)

	customCallID := fmt.Sprintf("%s@%s", GenerateCallID(32), device.MediaIp)
	callID := sip.CallIDHeader(customCallID)
	maxforward := sip.MaxForwardsHeader(70)
	csqHeader := sip.CSeqHeader{
		SeqNo:      uint32(device.SN),
		MethodName: "INVITE",
	}
	contactHDR := sip.ContactHeader{
		Address: sip.Uri{
			User: d.gb.Serial,
			Host: device.MediaIp,
			Port: device.LocalPort,
		},
	}

	fromHDR := sip.FromHeader{
		Address: sip.Uri{
			User: d.gb.Serial,
			Host: device.MediaIp,
			Port: device.LocalPort,
		},
		Params: sip.NewParams(),
	}
	fromHDR.Params.Add("tag", sip.GenerateTagN(32))

	dialogClientCache := sipgo.NewDialogClientCache(device.client, contactHDR)

	d.pullCtx.GoToStepConst(StepInviteSend)

	// 创建Via头部，使用设备的Transport协议
	// Via头部必须放在第一个位置，这样AppendHeader时Via会在最前面
	viaHeader := &sip.ViaHeader{
		ProtocolName:    "SIP",
		ProtocolVersion: "2.0",
		Transport:       device.Transport, // 使用设备注册时的Transport
		Host:            device.SipIp,
		Port:            device.LocalPort,
		Params:          sip.HeaderParams(sip.NewParams()),
	}
	viaHeader.Params.Add("branch", sip.GenerateBranchN(16))

	d.Info("start to invite", "recipient:", recipient, " fromHDR:", fromHDR, " toHeader:", toHeader, " device.contactHDR:",
		device.contactHDR, "contactHDR:", contactHDR, "sdpInfo:", strings.Join(sdpInfo, "|||"), "viaHeader:", viaHeader, "transport", device.Transport)
	// Via头部必须是第一个参数！这样即使用AppendHeader，Via也会在最前面
	// 这样Client检查req.Via()时就能找到我们的Via头部，不会再创建默认的UDP Via
	d.session, err = dialogClientCache.Invite(d, recipient, []byte(strings.Join(sdpInfo, "\r\n")+"\r\n"), viaHeader, &callID, &csqHeader, &fromHDR, &toHeader, &maxforward, userAgentHeader, subjectHeader, &contentTypeHeader)
	// 最后添加Content-Length头部
	if err != nil {
		d.pullCtx.Fail("dialog invite error: " + err.Error())
		return errors.Join(fmt.Errorf("dialog invite error:%s", err.Error()))
	}

	d.SetDescriptions(task.Description{
		"streamPath":            d.StreamPath,
		"streamMode":            d.StreamMode,
		"mediaPort":             d.MediaPort,
		"mediaIP":               device.MediaIp,
		"sipIP":                 device.SipIp,
		"transport":             device.Transport,
		"ssrc":                  ssrc,
		"callID":                customCallID,
		"deviceID":              device.DeviceId,
		"channelID":             channelId,
		"deviceIP":              device.IP,
		"devicePort":            device.Port,
		"localPort":             device.LocalPort,
		"startTime":             time.Now(),
		"from":                  fromHDR.Address.String(),
		"to":                    toHeader.Address.String(),
		"contact":               contactHDR.Address.String(),
		"subject":               fmt.Sprintf("%s:%s,%s:0", channelId, ssrc, d.gb.Serial),
		"recipient":             recipient.String(),
		"sdp":                   strings.Join(sdpInfo, "\r\n"),
		"via":                   viaHeader.String(),
		"viaBranch":             func() string { v, _ := viaHeader.Params.Get("branch"); return v }(),
		"broadcastPushAfterAck": device.BroadcastPushAfterAck,
	})
	d.pullCtx.GoToStepConst(StepResponseWait)
	return
}

// setupReceiver 配置 PSReceiver 的网络参数（单端口模式、监听地址等）
func (d *Dialog) setupReceiver(pub *mrtp.PSReceiver) {
	switch d.StreamMode {
	case mrtp.StreamModeTCPPassive:
		if d.gb.tcpPort > 0 {
			d.Info("into single port mode,use gb.tcpPort", d.gb.tcpPort)
			reader := &gb28181.SinglePortReader{
				SSRC:    d.SSRC,
				Mouth:   make(chan []byte, 1),
				Context: d,
			}
			var loaded bool
			reader, loaded = d.gb.singlePorts.LoadOrStore(reader)
			if loaded {
				reader.Context = d
			}
			pub.SinglePort = reader
			d.OnStop(func() {
				reader.Close()
				d.gb.singlePorts.Remove(reader)
			})
		} else {
			// 多端口模式：根据SSRCCheck配置决定是否启用SSRC过滤
			if d.Channel.Device.SSRCCheck {
				pub.ExpectedSSRC = d.SSRC
				d.Info("multi-port mode, SSRC filtering enabled", "expectedSSRC", d.SSRC)
			} else {
				d.Info("multi-port mode, SSRC filtering disabled")
			}
		}
		pub.ListenAddr = fmt.Sprintf(":%d", d.MediaPort)
	case mrtp.StreamModeUDP:
		if d.gb.udpPort > 0 {
			d.Info("into single port mode, use gb.udpPort", d.gb.udpPort)
			reader := &gb28181.SinglePortReader{
				SSRC:    d.SSRC,
				Mouth:   make(chan []byte, 100),
				Context: d,
			}
			var loaded bool
			reader, loaded = d.gb.singlePorts.LoadOrStore(reader)
			if loaded {
				reader.Context = d
			}
			pub.SinglePort = reader
			d.OnStop(func() {
				reader.Close()
				d.gb.singlePorts.Remove(reader)
			})
		}
		pub.ListenAddr = fmt.Sprintf(":%d", d.MediaPort)
	}
	pub.StreamMode = d.StreamMode
}

func (d *Dialog) Run() (err error) {
	var pub mrtp.PSReceiver
	pub.Publisher = d.pullCtx.Publisher
	pub.Logger = d.gb.Logger.With("streamPath", d.StreamPath)

	// 如果不是 BroadcastPushAfterAck 模式，提前创建监听器（多端口模式需要）
	if !d.Channel.Device.BroadcastPushAfterAck {
		d.Info("creating listener before WaitAnswer", "broadcastPushAfterAck", false, "addr", d.MediaPort)
		d.setupReceiver(&pub)

		// 提前启动监听器
		err = pub.Receiver.Start()
		if err != nil {
			d.Error("start listener before WaitAnswer failed", "err", err)
			return err
		}
	}

	d.Info("before WaitAnswer")
	err = d.session.WaitAnswer(d, sipgo.AnswerOptions{})
	d.Info("after WaitAnswer")
	if err != nil {
		d.pullCtx.Fail("等待响应错误: " + err.Error())
		return errors.Join(errors.New("wait answer error"), err)
	}
	inviteResponseBody := string(d.session.InviteResponse.Body())
	d.Info("inviteResponse", "body", inviteResponseBody)

	// 添加响应信息到 Description
	d.SetDescriptions(task.Description{
		"responseStatus": d.session.InviteResponse.StatusCode,
		"responseReason": d.session.InviteResponse.Reason,
		"responseSDP":    inviteResponseBody,
		"responseContact": func() string {
			if c := d.session.InviteResponse.Contact(); c != nil {
				return c.Address.String()
			}
			return ""
		}(),
	})
	ds := strings.Split(inviteResponseBody, "\r\n")
	for _, l := range ds {
		if ls := strings.Split(l, "="); len(ls) > 1 {
			switch ls[0] {
			case "y":
				if len(ls[1]) > 0 {
					if _ssrc, err := strconv.ParseInt(ls[1], 10, 0); err == nil {
						d.SSRC = uint32(_ssrc)
					} else {
						d.pullCtx.Fail("解析邀请响应y字段错误: " + err.Error())
						return errors.New("read invite respose y error" + err.Error())
					}
				}
			case "c":
				// 解析 c=IN IP4 xxx.xxx.xxx.xxx 格式
				parts := strings.Split(ls[1], " ")
				if len(parts) >= 3 {
					d.targetIP = parts[len(parts)-1]
				}
			case "m":
				// 解析 m=video port xxx 格式
				parts := strings.Split(ls[1], " ")
				if len(parts) >= 2 {
					if port, err := strconv.Atoi(parts[1]); err == nil {
						d.targetPort = port
					}
				}
			}
		}
	}
	// 修复 Contact 地址：某些设备响应的 Contact 包含错误的域名，导致 ACK 发送失败
	// 强制使用原始的 Recipient 地址确保 ACK 能正确发送到设备
	if d.session.InviteResponse.Contact() != nil {
		d.session.InviteResponse.Contact().Address = d.session.InviteRequest.Recipient
	}

	// 添加解析后的响应参数到 Description
	d.SetDescriptions(task.Description{
		"responseSSRC":       d.SSRC,
		"responseTargetIP":   d.targetIP,
		"responseTargetPort": d.targetPort,
	})

	// 移动到流数据接收步骤
	d.pullCtx.GoToStepConst(pkg.StepStreaming)

	// TCP-ACTIVE 模式需要在解析 targetIP 后设置连接地址
	if d.StreamMode == mrtp.StreamModeTCPActive {
		pub.ListenAddr = fmt.Sprintf("%s:%d", d.targetIP, d.targetPort)
		d.Info("set TCP-ACTIVE connect address", "addr", pub.ListenAddr)
	}

	// 如果是 BroadcastPushAfterAck 模式，在 Ack 后创建监听器配置
	if d.Channel.Device.BroadcastPushAfterAck {
		d.Info("setup receiver after Ack", "broadcastPushAfterAck", true)
		d.setupReceiver(&pub)
	}

	err = d.session.Ack(d)
	if err != nil {
		d.Error("ack session err", err)
	}

	return d.RunTask(&pub)
}

func (d *Dialog) GetKey() string {
	return d.GetCallID()
}

func (d *Dialog) Dispose() {
	go func() {
		time.Sleep(90 * time.Second)
		switch d.StreamMode {
		case mrtp.StreamModeUDP:
			if d.gb.udpPort == 0 { //多端口模式
				// 回收端口，防止重复回收
				if !d.gb.udpPB.Release(d.MediaPort) {
					d.Warn("port already released or not allocated", "port", d.MediaPort, "type", "udp")
				}
			}
		case mrtp.StreamModeTCPPassive:
			if d.gb.tcpPort == 0 { //多端口模式
				// 回收端口，防止重复回收
				if !d.gb.tcpPB.Release(d.MediaPort) {
					d.Warn("port already released or not allocated", "port", d.MediaPort, "type", "tcp")
				}
			}
		}
		d.Info("listener port release", "port", d.MediaPort)
	}()
	d.Info("dialog dispose", "ssrc", d.SSRC, "listener", d.MediaPort, "streamMode", d.StreamMode, "deviceId", d.Channel.DeviceId, "channelId", d.Channel.ChannelId)
	if d.session != nil && d.session.InviteResponse != nil {
		err := d.session.Bye(d)
		if err != nil {
			d.Error("listener dialog bye bye", " err", err)
		}
	}
}
