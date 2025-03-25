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

	"m7s.live/v5/pkg/util"

	sipgo "github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	m7s "m7s.live/v5"
	"m7s.live/v5/pkg/task"
	gb28181 "m7s.live/v5/plugin/gb28181/pkg"
)

type Dialog struct {
	task.Job
	Channel *Channel
	gb28181.InviteOptions
	gb         *GB28181Plugin
	session    *sipgo.DialogClientSession
	pullCtx    m7s.PullJob
	start      string
	end        string
	StreamMode string // 数据流传输模式（UDP:udp传输/TCP-ACTIVE：tcp主动模式/TCP-PASSIVE：tcp被动模式）
	targetIP   string // 目标设备的IP地址
	targetPort int    // 目标设备的端口
}

func (d *Dialog) GetCallID() string {
	return d.session.InviteRequest.CallID().Value()
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
	// 处理时间范围
	d.InviteOptions.Start = d.start
	d.InviteOptions.End = d.End
	if d.IsLive() {
		d.pullCtx.PublishConfig.PubType = m7s.PublishTypeVod
	}
	err = d.pullCtx.Publish()
	if err != nil {
		return
	}
	sss := strings.Split(d.pullCtx.RemoteURL, "/")
	if len(sss) < 2 {
		d.Info("remote url is invalid", d.pullCtx.RemoteURL)
		return
	}
	deviceId, channelId := sss[len(sss)-2], sss[len(sss)-1]
	var device *Device
	if deviceTmp, ok := d.gb.devices.Get(deviceId); ok {
		device = deviceTmp
		if channel, ok := deviceTmp.channels.Get(channelId); ok {
			d.Channel = channel
			d.StreamMode = device.StreamMode
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
	deviceIPParsed := net.ParseIP(device.IP)
	if !deviceIPParsed.IsPrivate() {
		sdpIP = d.gb.GetPublicIP(sdpIP)
	}
	d.Info("sdpIP is ", sdpIP)

	// 构建 SDP 内容
	sdpInfo := []string{
		"v=0",
		fmt.Sprintf("o=%s 0 0 IN IP4 %s", channelId, sdpIP),
		fmt.Sprintf("s=%s", util.Conditional(d.IsLive(), "Play", "Playback")), // 根据是否有时间参数决定
	}

	// 非直播模式下添加u行，保持在s=和c=之间
	//if !d.IsLive() {
	sdpInfo = append(sdpInfo, fmt.Sprintf("u=%s:0", channelId))
	//}

	// 添加c行
	sdpInfo = append(sdpInfo, "c=IN IP4 "+sdpIP)

	// 将字符串时间转换为 Unix 时间戳
	if !d.IsLive() {
		startTime, endTime, err := util.TimeRangeQueryParse(url.Values{"start": []string{d.start}, "end": []string{d.end}})
		if err != nil {
			d.Stop(errors.New("parse end time error"))
		}
		sdpInfo = append(sdpInfo, fmt.Sprintf("t=%d %d", startTime.Unix(), endTime.Unix()))
	} else {
		sdpInfo = append(sdpInfo, "t=0 0")
	}

	// 添加媒体行和相关属性
	var mediaLine string
	switch strings.ToUpper(device.StreamMode) {
	case "TCP-PASSIVE", "TCP-ACTIVE":
		mediaLine = fmt.Sprintf("m=video %d TCP/RTP/AVP 96", d.MediaPort)
	case "UDP":
		mediaLine = fmt.Sprintf("m=video %d RTP/AVP 96", d.MediaPort)
	default:
		mediaLine = fmt.Sprintf("m=video %d TCP/RTP/AVP 96", d.MediaPort)
	}

	sdpInfo = append(sdpInfo, mediaLine)
	sdpInfo = append(sdpInfo, "a=recvonly")

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
	case "UDP":
		d.Stop(errors.New("do not support udp mode"))
	default:
		sdpInfo = append(sdpInfo,
			"a=setup:passive",
			"a=connection:new",
		)
	}
	sdpInfo = append(sdpInfo, "a=rtpmap:96 PS/90000")

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
		Address: sip.Uri{User: channelId, Host: device.HostAddress},
	}
	userAgentHeader := sip.NewHeader("User-Agent", "M7S/"+m7s.Version)

	//customCallID := fmt.Sprintf("%s-%s-%d@%s", device.DeviceID, channelId, time.Now().Unix(), device.LocalIP)
	customCallID := fmt.Sprintf("%s@%s", GenerateCallID(32), device.LocalIP)
	callID := sip.CallIDHeader(customCallID)
	//viaHeader := sip.ViaHeader{
	//	ProtocolName:    "SIP",
	//	ProtocolVersion: "2.0",
	//	Transport:       "UDP",
	//	Host:            device.LocalIP,
	//	Port:            device.LocalPort,
	//	Params:          sip.HeaderParams(sip.NewParams()),
	//}
	//viaHeader.Params.Add("branch", sip.GenerateBranchN(10)).Add("rport", "")
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
			Host: device.LocalIP,
			Port: device.LocalPort,
		},
	}

	fromHDR := sip.FromHeader{
		Address: sip.Uri{
			User: d.gb.Serial,
			Host: d.gb.Realm,
		},
		Params: sip.NewParams(),
	}
	fromHDR.Params.Add("tag", sip.GenerateTagN(32))
	// 创建会话
	d.session, err = device.dialogClient.Invite(d.gb, recipient, []byte(strings.Join(sdpInfo, "\r\n")+"\r\n"), &callID, &csqHeader, &fromHDR, &toHeader, &maxforward, userAgentHeader, &contactHDR, subjectHeader, &contentTypeHeader)
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
	err = d.session.Ack(d.gb)
	if err != nil {
		d.gb.Error("ack session err", err)
	}
	pub := gb28181.NewPSPublisher(d.pullCtx.Publisher)
	if d.StreamMode == "TCP-ACTIVE" {
		pub.Receiver.ListenAddr = fmt.Sprintf("%s:%d", d.targetIP, d.targetPort)
	} else {
		pub.Receiver.ListenAddr = fmt.Sprintf(":%d", d.MediaPort)
	}
	pub.Receiver.StreamMode = d.StreamMode
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
		d.Error("dialog bye bye err", err)
	}
	err = d.session.Close()
	if err != nil {
		d.Error("dialog close session err", err)
	}
}
