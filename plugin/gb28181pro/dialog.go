package plugin_gb28181pro

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"m7s.live/v5/pkg/util"

	sipgo "github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	m7s "m7s.live/v5"
	"m7s.live/v5/pkg/task"
	gb28181 "m7s.live/v5/plugin/gb28181pro/pkg"
)

type Dialog struct {
	task.Job
	Channel *Channel
	gb28181.InviteOptions
	gb      *GB28181ProPlugin
	session *sipgo.DialogClientSession
	pullCtx m7s.PullJob
	start   string
	end     string
}

func (d *Dialog) GetCallID() string {
	return d.session.InviteRequest.CallID().Value()
}

func (d *Dialog) GetPullJob() *m7s.PullJob {
	return &d.pullCtx
}

func (d *Dialog) Start() (err error) {
	// 处理时间范围
	isLive := true
	if d.start != "" && d.end != "" {
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
		if channel, ok := deviceTmp.channels.Get(channelId); ok {
			d.Channel = channel
		} else {
			return fmt.Errorf("channel %s not found", channelId)
		}
	} else {
		return fmt.Errorf("device %s not found", deviceId)
	}

	d.gb.dialogs.Set(d)
	defer d.gb.dialogs.Remove(d)
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
	ssrc := d.CreateSSRC(d.gb.Serial)
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
		startime, err := time.ParseInLocation("2006-01-02T15:04:05", d.start, time.Local)
		if err != nil {
			d.Stop(errors.New("parse start time error"))
		}
		endtime, err := time.ParseInLocation("2006-01-02T15:04:05", d.end, time.Local)
		if err != nil {
			d.Stop(errors.New("parse end time error"))
		}
		sdpInfo = append(sdpInfo, fmt.Sprintf("t=%d %d", startime.Unix(), endtime.Unix()))
	} else {
		sdpInfo = append(sdpInfo, "t=0 0")
	}

	// 添加媒体行和相关属性
	var mediaLine string
	//switch strings.ToUpper(device.StreamMode) {
	//case "TCP-PASSIVE", "TCP-ACTIVE":
	mediaLine = fmt.Sprintf("m=video %d TCP/RTP/AVP 96", d.MediaPort)
	//case "UDP":
	//	mediaLine = fmt.Sprintf("m=video %d TCP/RTP/AVP 96", d.MediaPort)
	//default:
	//	mediaLine = fmt.Sprintf("m=video %d RTP/AVP 96 126 125 99 34 98 97", d.MediaPort)
	//}

	sdpInfo = append(sdpInfo, mediaLine)
	sdpInfo = append(sdpInfo,
		"a=recvonly",
		"a=rtpmap:96 PS/90000",
		//"a=fmtp:126 profile-level-id=42e01e",
		//"a=rtpmap:126 H264/90000",
		//"a=rtpmap:125 H264S/90000",
		//"a=fmtp:125 profile-level-id=42e01e",
		//"a=rtpmap:98 H264/90000",
		//"a=rtpmap:97 MPEG4/90000",
		//"a=rtpmap:99 H265/90000",
	)

	//根据传输模式添加 setup 和 connection 属性
	switch strings.ToUpper(device.StreamMode) {
	case "TCP-PASSIVE":
		sdpInfo = append(sdpInfo,
			"a=setup:passive",
			"a=connection:new",
		)
	case "TCP-ACTIVE":
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

	// 添加 SSRC
	sdpInfo = append(sdpInfo, fmt.Sprintf("y=%s", ssrc))

	// 创建 INVITE 请求
	recipient := sip.Uri{
		Host: device.IP,
		Port: device.Port,
		User: channelId,
	}
	request := device.CreateRequest(sip.INVITE, recipient)
	if request == nil {
		return nil
	}

	// 设置必需的头部
	contentTypeHeader := sip.ContentTypeHeader("APPLICATION/SDP")
	subjectHeader := sip.NewHeader("Subject", fmt.Sprintf("%s:%s,%s:0", channelId, ssrc, d.gb.Serial))
	//allowHeader := sip.NewHeader("Allow", "INVITE, ACK, CANCEL, REGISTER, MESSAGE, NOTIFY, BYE")
	//Toheader里需要放入目录通道的id
	toHeader := sip.ToHeader{
		Address: sip.Uri{User: channelId, Host: device.HostAddress},
	}
	userAgentHeader := sip.NewHeader("User-Agent", "M7S/"+m7s.Version)

	request.AppendHeader(&contentTypeHeader)
	request.AppendHeader(subjectHeader)
	//request.AppendHeader(allowHeader)
	request.AppendHeader(&toHeader)
	customCallID := fmt.Sprintf("%s-%s-%d@%s", device.DeviceID, channelId, time.Now().Unix(), device.LocalIP)
	callID := sip.CallIDHeader(customCallID)
	//request.AppendHeaderAfter(&callID, "User-Agent")
	viaHeader := sip.ViaHeader{
		ProtocolName:    "SIP",
		ProtocolVersion: "2.0",
		Transport:       "UDP",
		Host:            device.LocalIP,
		Port:            device.LocalPort,
		Params:          sip.HeaderParams(sip.NewParams()),
	}
	viaHeader.Params.Add("branch", sip.GenerateBranchN(16)).Add("rport", "")
	maxforward := sip.MaxForwardsHeader(70)
	//contentLengthHeader := sip.ContentLengthHeader(len(strings.Join(sdpInfo, "\r\n") + "\r\n"))
	csqHeader := sip.CSeqHeader{
		SeqNo:      uint32(device.SN),
		MethodName: "INVITE",
	}
	request.AppendHeader(&viaHeader)
	request.SetBody([]byte(strings.Join(sdpInfo, "\r\n") + "\r\n"))
	//request.AppendHeader(&contentLengthHeader)

	// 创建会话
	d.session, err = device.dialogClient.Invite(d.gb, recipient, request.Body(), &callID, &csqHeader, &device.fromHDR, &toHeader, &viaHeader, &maxforward, userAgentHeader, &device.contactHDR, subjectHeader, &contentTypeHeader)
	// 最后添加Content-Length头部
	return
}

func (d *Dialog) Run() (err error) {
	d.Channel.Info("before WaitAnswer")
	err = d.session.WaitAnswer(d.gb, sipgo.AnswerOptions{})
	d.Channel.Info("after WaitAnswer")
	if err != nil {
		return
	}
	inviteResponseBody := string(d.session.InviteResponse.Body())
	d.Channel.Info("inviteResponse", "body", inviteResponseBody)
	ds := strings.Split(inviteResponseBody, "\r\n")
	for _, l := range ds {
		if ls := strings.Split(l, "="); len(ls) > 1 {
			if ls[0] == "y" && len(ls[1]) > 0 {
				if _ssrc, err := strconv.ParseInt(ls[1], 10, 0); err == nil {
					d.SSRC = uint32(_ssrc)
				} else {
					d.gb.Error("read invite response y ", "err", err)
				}
				//	break
			}
			if ls[0] == "m" && len(ls[1]) > 0 {
				netinfo := strings.Split(ls[1], " ")
				if strings.ToUpper(netinfo[2]) == "TCP/RTP/AVP" {
					d.gb.Debug("device support tcp")
				} else {
					d.gb.Error("device not support tcp")
					return
				}
			}
		}
	}
	err = d.session.Ack(d.gb)
	pub := gb28181.NewPSPublisher(d.pullCtx.Publisher)
	pub.Receiver.ListenAddr = fmt.Sprintf(":%d", d.MediaPort)
	pub.Receiver.ListenPort = d.MediaPort
	d.AddTask(&pub.Receiver)
	pub.Demux()
	return
}

func (d *Dialog) GetKey() uint32 {
	return d.SSRC
}

func (d *Dialog) Dispose() {
	err := d.session.Bye(d)
	if err != nil {
		d.Error("bye bye err", err)
	}
	err = d.session.Close()
	if err != nil {
		d.Error("close session err", err)
	}
}
