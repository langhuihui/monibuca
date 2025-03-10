package plugin_gb28181pro

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	sipgo "github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	m7s "m7s.live/v5"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/pkg/util"
	gb28181 "m7s.live/v5/plugin/gb28181pro/pkg"
)

// ForwardDialog 是用于转发RTP流的会话结构体
type ForwardDialog struct {
	task.Job
	Channel *Channel
	gb28181.InviteOptions
	gb           *GB28181ProPlugin
	session      *sipgo.DialogClientSession
	pullCtx      m7s.PullJob
	start        string
	end          string
	forwarder    *gb28181.RTPForwarder
	targetIP     string
	targetPort   int
	listenProto  string // 监听协议：tcp或udp
	enableBuffer bool   // 是否启用缓冲（当false时直接转发，不缓存到FeedChan）
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

// SetTarget 设置转发目标地址和端口
func (d *ForwardDialog) SetTarget(ip string, port int) {
	d.targetIP = ip
	d.targetPort = port
}

// SetListenProtocol 设置监听协议类型
func (d *ForwardDialog) SetListenProtocol(proto string) {
	d.listenProto = strings.ToLower(proto)
}

// EnableBuffering 设置是否启用缓冲
func (d *ForwardDialog) EnableBuffering(enable bool) {
	d.enableBuffer = enable
}

// NewForwardDialog 创建一个新的转发会话
func NewForwardDialog(gb *GB28181ProPlugin) *ForwardDialog {
	return &ForwardDialog{
		gb:           gb,
		listenProto:  "tcp", // 默认TCP
		enableBuffer: true,  // 默认启用缓冲
	}
}

// Start 启动会话
func (d *ForwardDialog) Start() (err error) {
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

	// 注册对话到集合，使用类型转换
	d.gb.AddTask(d)

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

	// 使用设备的CreateSSRC方法
	ssrcValue := device.CreateSSRC(d.gb.Serial)
	d.SSRC = uint32(ssrcValue)

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

	sdpInfo = append(sdpInfo, fmt.Sprintf("m=video %d RTP/AVP 96 98 97", d.MediaPort))
	sdpInfo = append(sdpInfo, "a=recvonly")
	sdpInfo = append(sdpInfo, "a=rtpmap:96 PS/90000")
	sdpInfo = append(sdpInfo, "a=rtpmap:98 H264/90000")
	sdpInfo = append(sdpInfo, "a=rtpmap:97 MPEG4/90000")

	if d.SSRC == 0 {
		d.SSRC = uint32(ssrcValue)
	}
	sdpInfo = append(sdpInfo, fmt.Sprintf("y=%d", d.SSRC))

	// 创建INVITE请求
	request := sip.NewRequest(sip.INVITE, sip.Uri{User: channelId, Host: device.IP})
	subject := fmt.Sprintf("%s:0,%s:0", channelId, deviceId)

	// 创建自定义头部
	contentTypeHeader := sip.ContentTypeHeader("APPLICATION/SDP")
	subjectHeader := sip.NewHeader("Subject", subject)

	// 设置请求体
	request.SetBody([]byte(strings.Join(sdpInfo, "\r\n") + "\r\n"))

	// 创建会话 - 使用device的dialogClient创建
	d.session, err = device.dialogClient.Invite(d.gb, device.Recipient, request.Body(), &contentTypeHeader, subjectHeader)

	return
}

// Run 运行会话
func (d *ForwardDialog) Run() (err error) {
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
	d.forwarder.Protocol = d.listenProto // 使用设定的协议
	if d.listenProto == "" {
		d.forwarder.Protocol = "tcp" // 默认使用TCP
	}

	// 设置监听地址和端口
	d.forwarder.ListenAddr = fmt.Sprintf(":%d", d.MediaPort)
	d.forwarder.ListenPort = d.MediaPort

	// 设置转发目标
	if d.targetIP != "" && d.targetPort > 0 {
		err = d.forwarder.SetTarget(d.targetIP, d.targetPort)
		if err != nil {
			d.Error("set target error", "err", err)
			return err
		}
	} else {
		d.Warn("no target set, will only receive but not forward")
	}

	// 将forwarder添加到任务中
	d.AddTask(d.forwarder)

	d.Info("forwarder started successfully",
		"protocol", d.forwarder.Protocol,
		"listen", d.forwarder.ListenAddr,
		"target", fmt.Sprintf("%s:%d", d.targetIP, d.targetPort))

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

	// 清理RTPForwarder资源
	if d.forwarder != nil {
		d.forwarder.Dispose()
	}
}
