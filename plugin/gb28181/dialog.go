package plugin_gb28181pro

import (
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	sipgo "github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	m7s "m7s.live/v5"
	pkg "m7s.live/v5/pkg"
	"m7s.live/v5/pkg/task"
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
			return fmt.Errorf("channel %s not found", channelId)
		}
	} else {
		d.pullCtx.Fail(fmt.Sprintf("device %s not found", deviceId))
		return fmt.Errorf("device %s not found", deviceId)
	}

	d.pullCtx.GoToStepConst(StepSIPPrepare)

	//defer d.gb.dialogs.Remove(d)
	if d.gb.tcpPort > 0 {
		d.MediaPort = d.gb.tcpPort
	} else {
		if d.gb.MediaPort.Valid() {
			select {
			case d.MediaPort = <-d.gb.tcpPorts:
			default:
				d.pullCtx.Fail("no available tcp port")
				return fmt.Errorf("no available tcp port")
			}
		} else {
			d.MediaPort = d.gb.MediaPort[0]
		}
	}

	d.pullCtx.GoToStepConst(StepSDPBuild)

	ssrc := d.CreateSSRC(d.gb.Serial)
	d.Info("MediaIp is ", device.MediaIp)

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
	case mrtp.StreamModeUDP:
		return errors.New("do not support udp mode")
	default:
		sdpInfo = append(sdpInfo,
			"a=setup:passive",
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
	//allowHeader := sip.NewHeader("Allow", "INVITE, ACK, CANCEL, REGISTER, MESSAGE, NOTIFY, BYE")
	//Toheader里需要放入目录通道的id
	toHeader := sip.ToHeader{
		Address: sip.Uri{User: channelId, Host: channelId[0:10]},
	}
	userAgentHeader := sip.NewHeader("User-Agent", "M7S/"+m7s.Version)

	//customCallID := fmt.Sprintf("%s-%s-%d@%s", device.DeviceId, channelId, time.Now().Unix(), device.SipIp)
	customCallID := fmt.Sprintf("%s@%s", GenerateCallID(32), device.MediaIp)
	callID := sip.CallIDHeader(customCallID)
	viaHeader := sip.ViaHeader{
		ProtocolName:    "SIP",
		ProtocolVersion: "2.0",
		Transport:       "UDP",
		Host:            device.MediaIp,
		Port:            device.LocalPort,
		Params:          sip.NewParams(),
	}
	viaHeader.Params.Add("branch", sip.GenerateBranchN(10)).Add("rport", "")
	maxforward := sip.MaxForwardsHeader(70)
	//contentLengthHeader := sip.ContentLengthHeader(len(strings.Join(sdpInfo, "\r\n") + "\r\n"))
	csqHeader := sip.CSeqHeader{
		SeqNo:      uint32(device.SN),
		MethodName: "INVITE",
	}
	//request.AppendHeader(&contentLengthHeader)
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
	// 创建会话
	d.gb.Info("start to invite,recipient:", recipient, " viaHeader:", viaHeader, " fromHDR:", fromHDR, " toHeader:", toHeader, " device.contactHDR:", device.contactHDR, "contactHDR:", contactHDR)

	d.pullCtx.GoToStepConst(StepInviteSend)

	// 判断当前系统类型
	//if runtime.GOOS == "windows" {
	//	d.session, err = dialogClientCache.Invite(d.gb, recipient, []byte(strings.Join(sdpInfo, "\r\n")+"\r\n"), &callID, &csqHeader, &fromHDR, &toHeader, &maxforward, userAgentHeader, subjectHeader, &contentTypeHeader)
	//} else {
	d.session, err = dialogClientCache.Invite(d.gb, recipient, []byte(strings.Join(sdpInfo, "\r\n")+"\r\n"), &callID, &csqHeader, &fromHDR, &toHeader, &maxforward, userAgentHeader, subjectHeader, &contentTypeHeader)
	//}
	// 最后添加Content-Length头部
	if err != nil {
		d.pullCtx.Fail("dialog invite error: " + err.Error())
		return errors.New("dialog invite error" + err.Error())
	}

	d.pullCtx.GoToStepConst(StepResponseWait)
	return
}

func (d *Dialog) Run() (err error) {
	d.gb.Info("before WaitAnswer")
	err = d.session.WaitAnswer(d.gb, sipgo.AnswerOptions{})
	d.gb.Info("after WaitAnswer")
	if err != nil {
		d.pullCtx.Fail("wait answer error: " + err.Error())
		return errors.New("wait answer error" + err.Error())
	}
	inviteResponseBody := string(d.session.InviteResponse.Body())
	d.gb.Info("inviteResponse", "body", inviteResponseBody)
	ds := strings.Split(inviteResponseBody, "\r\n")
	for _, l := range ds {
		if ls := strings.Split(l, "="); len(ls) > 1 {
			switch ls[0] {
			case "y":
				if len(ls[1]) > 0 {
					if _ssrc, err := strconv.ParseInt(ls[1], 10, 0); err == nil {
						d.SSRC = uint32(_ssrc)
					} else {
						d.pullCtx.Fail("read invite response y error: " + err.Error())
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
	if d.session.InviteResponse.Contact() != nil {
		if &d.session.InviteRequest.Recipient != &d.session.InviteResponse.Contact().Address {
			d.session.InviteResponse.Contact().Address = d.session.InviteRequest.Recipient
		}
	}
	err = d.session.Ack(d.gb)
	if err != nil {
		d.gb.Error("ack session err", err)
	}

	d.pullCtx.GoToStepConst(pkg.StepStreaming)

	var pub mrtp.PSReceiver
	pub.Publisher = d.pullCtx.Publisher
	if d.StreamMode == mrtp.StreamModeTCPActive {
		pub.ListenAddr = fmt.Sprintf("%s:%d", d.targetIP, d.targetPort)
	} else {
		if d.gb.tcpPort > 0 {
			d.Info("into single port mode,use gb.tcpPort", d.gb.tcpPort)
			if d.gb.netListener != nil {
				d.Info("use gb.netListener", d.gb.netListener.Addr())
				pub.Listener = d.gb.netListener
			} else {
				d.Info("listen tcp4", fmt.Sprintf(":%d", d.gb.tcpPort))
				pub.Listener, _ = net.Listen("tcp4", fmt.Sprintf(":%d", d.gb.tcpPort))
				d.gb.netListener = pub.Listener
			}
			pub.SSRC = d.SSRC
		}
		pub.ListenAddr = fmt.Sprintf(":%d", d.MediaPort)
	}
	pub.StreamMode = d.StreamMode
	return d.RunTask(&pub)
}

func (d *Dialog) GetKey() string {
	return d.GetCallID()
}

func (d *Dialog) Dispose() {
	if d.gb.tcpPort == 0 {
		// 如果没有设置tcp端口，则将MediaPort设置为0，表示不再使用
		d.gb.tcpPorts <- d.MediaPort
	}
	d.Info("dialog dispose", "ssrc", d.SSRC, "mediaPort", d.MediaPort, "streamMode", d.StreamMode, "deviceId", d.Channel.DeviceId, "channelId", d.Channel.ChannelId)
	if d.session != nil {
		err := d.session.Bye(d)
		if err != nil {
			d.Error("dialog bye bye err", err)
		}
		err = d.session.Close()
		if err != nil {
			d.Error("dialog close session err", err)
		}
	}
}
