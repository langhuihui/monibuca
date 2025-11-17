package plugin_gb28181pro

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"m7s.live/v5"

	sipgo "github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	task "github.com/langhuihui/gotask"
	"m7s.live/v5/pkg/config"
	gb28181 "m7s.live/v5/plugin/gb28181/pkg"
	mrtp "m7s.live/v5/plugin/rtp/pkg"
)

// DownloadDialog 下载会话
type DownloadDialog struct {
	task.Task
	gb28181.InviteOptions
	gb         *GB28181Plugin
	session    *sipgo.DialogClientSession
	device     *Device
	channel    *Channel
	MediaPort  uint16
	targetIP   string
	targetPort uint16
	// 任务信息
	DownloadId    string
	DeviceId      string
	ChannelId     string
	StartTime     time.Time
	EndTime       time.Time
	DownloadSpeed int // 下载速度倍数（1-4倍，默认1倍）
	// 状态信息
	Status      string // pending/downloading/completed/failed
	Progress    int    // 0-100
	FilePath    string
	DownloadUrl string // 下载链接
	Error       string
	StartedAt   time.Time
	CompletedAt time.Time
}

// CompletedDownloadDialog 用于缓存已完成下载任务的最终结果
// 与 DownloadDialog 生命周期解耦，仅保留前端查询所需字段
type CompletedDownloadDialog struct {
	DownloadId  string
	DeviceId    string
	ChannelId   string
	Status      string
	Progress    int
	FilePath    string
	DownloadUrl string
	Error       string
	StartedAt   time.Time
	CompletedAt time.Time
}

func (d *CompletedDownloadDialog) GetKey() string {
	return d.DownloadId
}

// setupReceiver 配置 PSReceiver 的网络参数（单端口模式、监听地址等）
func (d *DownloadDialog) setupReceiver(ps *mrtp.PSReceiver) {
	switch d.device.StreamMode {
	case mrtp.StreamModeTCPPassive:
		if d.gb.tcpPort > 0 {
			// 单端口模式
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
			ps.SinglePort = reader
			d.OnStop(func() {
				reader.Close()
				d.gb.singlePorts.Remove(reader)
			})
		}
		ps.ListenAddr = fmt.Sprintf(":%d", d.MediaPort)
	case mrtp.StreamModeUDP:
		if d.gb.udpPort > 0 {
			// 单端口模式
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
			ps.SinglePort = reader
			d.OnStop(func() {
				reader.Close()
				d.gb.singlePorts.Remove(reader)
			})
		}
		ps.ListenAddr = fmt.Sprintf(":%d", d.MediaPort)
	}
	ps.StreamMode = d.device.StreamMode
}

// GetKey 返回下载任务的唯一标识
func (d *DownloadDialog) GetKey() string {
	return d.DownloadId
}

// Start 启动下载会话
func (d *DownloadDialog) Start() (err error) {
	// 更新状态
	d.Status = "downloading"
	d.StartedAt = time.Now()

	// 1. 获取设备和通道
	device, ok := d.gb.devices.Get(d.DeviceId)
	if !ok {
		d.Status = "failed"
		d.Error = fmt.Sprintf("设备不存在: %s", d.DeviceId)
		return errors.Join(fmt.Errorf("device not found"), errors.New(d.Error))
	}
	d.device = device

	channelKey := d.DeviceId + "_" + d.ChannelId
	channel, ok := device.channels.Get(channelKey)
	if !ok {
		d.Status = "failed"
		d.Error = fmt.Sprintf("通道不存在: %s", d.ChannelId)
		return errors.Join(fmt.Errorf("channel not found"), errors.New(d.Error))
	}
	d.channel = channel

	// 2. 分配媒体端口
	switch d.device.StreamMode {
	case mrtp.StreamModeTCPPassive:
		if d.gb.tcpPort > 0 {
			d.MediaPort = d.gb.tcpPort
		} else {
			if d.gb.MediaPort.Valid() {
				var ok bool
				d.MediaPort, ok = d.gb.tcpPB.Allocate()
				if !ok {
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
					return errors.Join(fmt.Errorf("no available udp port"))
				}
			} else {
				d.MediaPort = d.gb.MediaPort[0]
			}
		}
	}

	// 3. 生成 SSRC
	ssrc := d.CreateSSRC(d.gb.Serial)

	// 4. 构建 SDP
	startTimestamp := d.StartTime.Unix()
	endTimestamp := d.EndTime.Unix()

	sdpInfo := []string{
		"v=0",
		fmt.Sprintf("o=%s 0 0 IN IP4 %s", d.ChannelId, device.MediaIp),
		"s=Download", // 下载模式
		fmt.Sprintf("u=%s:0", d.ChannelId),
		"c=IN IP4 " + device.MediaIp,
		fmt.Sprintf("t=%d %d", startTimestamp, endTimestamp),
	}

	// 添加媒体行
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

	// 根据传输模式添加 setup 和 connection 属性
	switch device.StreamMode {
	case mrtp.StreamModeTCPPassive:
		sdpInfo = append(sdpInfo, "a=setup:passive", "a=connection:new")
	case mrtp.StreamModeTCPActive:
		sdpInfo = append(sdpInfo, "a=setup:active", "a=connection:new")
	case mrtp.StreamModeUDP:
		sdpInfo = append(sdpInfo, "a=setup:active", "a=connection:new")
	default:
		sdpInfo = append(sdpInfo, "a=setup:passive", "a=connection:new")
	}

	// 添加下载速度属性（默认1倍速，避免丢帧）
	downloadSpeed := d.DownloadSpeed
	if downloadSpeed <= 0 || downloadSpeed > 4 {
		downloadSpeed = 4 // 默认1倍速
	}
	sdpInfo = append(sdpInfo, fmt.Sprintf("a=downloadspeed:%d", downloadSpeed))

	// 添加 SSRC
	sdpInfo = append(sdpInfo, fmt.Sprintf("y=%s", ssrc))

	// 创建 INVITE 请求
	recipient := sip.Uri{
		Host: device.IP,
		Port: device.Port,
		User: d.ChannelId,
	}
	// 设置必需的头部
	contentTypeHeader := sip.ContentTypeHeader("APPLICATION/SDP")
	subjectHeader := sip.NewHeader("Subject", fmt.Sprintf("%s:%s,%s:0", d.ChannelId, ssrc, d.gb.Serial))
	//allowHeader := sip.NewHeader("Allow", "INVITE, ACK, CANCEL, REGISTER, MESSAGE, NOTIFY, BYE")
	//Toheader里需要放入目录通道的id
	toHeader := sip.ToHeader{
		Address: sip.Uri{User: d.ChannelId, Host: d.ChannelId[0:10]},
	}
	userAgentHeader := sip.NewHeader("User-Agent", "M7S/"+m7s.Version)

	//customCallID := fmt.Sprintf("%s-%s-%d@%s", device.DeviceId, channelId, time.Now().Unix(), device.SipIp)
	customCallID := fmt.Sprintf("%s@%s", GenerateCallID(32), device.MediaIp)
	callID := sip.CallIDHeader(customCallID)
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
	if err != nil {
		return fmt.Errorf("发送 INVITE 失败: %w", err)
	}
	d.SetDescriptions(task.Description{
		"streamPath":            d.StreamPath,
		"streamMode":            device.StreamMode,
		"mediaPort":             d.MediaPort,
		"mediaIP":               device.MediaIp,
		"sipIP":                 device.SipIp,
		"transport":             device.Transport,
		"ssrc":                  ssrc,
		"callID":                d.session.InviteRequest.CallID().Value(),
		"deviceID":              device.DeviceId,
		"channelID":             d.ChannelId,
		"deviceIP":              device.IP,
		"devicePort":            device.Port,
		"localPort":             device.LocalPort,
		"startTime":             time.Now(),
		"from":                  fromHDR.Address.String(),
		"to":                    toHeader.Address.String(),
		"subject":               fmt.Sprintf("%s:%s,%s:0", d.ChannelId, ssrc, d.gb.Serial),
		"recipient":             recipient.String(),
		"sdp":                   strings.Join(sdpInfo, "\r\n"),
		"viaBranch":             func() string { v, _ := viaHeader.Params.Get("branch"); return v }(),
		"broadcastPushAfterAck": device.BroadcastPushAfterAck,
	})

	return nil
}

// Go 运行下载会话（异步执行，支持并发）
func (d *DownloadDialog) Go() error {
	var psReceiver mrtp.PSReceiver

	// 如果不是 BroadcastPushAfterAck 模式，提前创建监听器（多端口模式需要）
	if !d.device.BroadcastPushAfterAck {
		d.device.Info("creating listener before WaitAnswer", "broadcastPushAfterAck", false, "addr", d.MediaPort)
		d.setupReceiver(&psReceiver)

		// 提前启动监听器
		if err := psReceiver.Receiver.Start(); err != nil {
			d.device.Error("start listener before WaitAnswer failed", "err", err)
			return err
		}
	}

	d.device.Info("before WaitAnswer")
	err := d.session.WaitAnswer(d, sipgo.AnswerOptions{})
	d.device.Info("after WaitAnswer")
	if err != nil {
		d.Status = "failed"
		d.Error = fmt.Sprintf("等待响应失败: %v", err)
		return errors.Join(errors.New("wait answer error"), err)
	}

	// 解析响应
	inviteResponseBody := string(d.session.InviteResponse.Body())
	d.device.Info("收到 INVITE 响应", "body", inviteResponseBody)
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
					}
				}
			case "c":
				parts := strings.Split(ls[1], " ")
				if len(parts) >= 3 {
					d.targetIP = parts[len(parts)-1]
				}
			case "m":
				if d.device.StreamMode == mrtp.StreamModeTCPActive {
					parts := strings.Split(ls[1], " ")
					if len(parts) >= 2 {
						if port, err := strconv.Atoi(parts[1]); err == nil {
							d.targetPort = uint16(port)
						}
					}
				} else {
					d.targetPort = d.MediaPort
				}
			}
		}
	}
	// invite响应里的contact是域名的话，sip尝试去解析，可能失败，这时候用invite请求里的recipient
	if d.session.InviteResponse.Contact() != nil {
		if &d.session.InviteRequest.Recipient != &d.session.InviteResponse.Contact().Address {
			d.session.InviteResponse.Contact().Address = d.session.InviteRequest.Recipient
		}
	}

	// 如果是 BroadcastPushAfterAck 模式，在 Ack 后创建监听器配置
	if d.device.BroadcastPushAfterAck {
		d.device.Info("setup receiver after Ack", "broadcastPushAfterAck", true)
		d.setupReceiver(&psReceiver)
	}

	// 发送 ACK
	err = d.session.Ack(d)
	if err != nil {
		// 与 dialog.Run 保持一致，仅记录错误，不直接 panic
		d.device.Error("ack session", "err", err)
	}

	d.gb.Info("下载会话已建立",
		"ssrc", d.SSRC,
		"targetIP", d.targetIP,
		"targetPort", d.targetPort)

	// 使用简洁的流路径格式
	// 格式：{设备ID}/{通道ID}
	streamPath := fmt.Sprintf("%s%s/%s/%s", "gbdownload_", time.Now().Local().Format("20060102150405"), d.DeviceId, d.ChannelId)

	// 5. 创建临时 Publisher 用于下载

	// 配置更大的缓冲区以支持高速下载，避免丢帧
	pubConf := d.gb.GetCommonConf().Publish
	pubConf.RingSize[0] = 1024 // 增大最小缓冲区
	pubConf.RingSize[1] = 4096 // 增大最大缓冲区
	pubConf.MaxFPS = 0         // 禁用FPS限制，避免丢帧
	pubConf.PubType = m7s.PublishTypeVod

	publisher, err := d.gb.PublishWithConfig(d, streamPath, pubConf)
	if err != nil {
		d.Status = "failed"
		d.Error = fmt.Sprintf("创建 Publisher 失败: %v", err)
		return fmt.Errorf("创建 Publisher 失败: %w", err)
	}

	// 6. 绑定 Publisher 到 PSReceiver，并监听 Publisher 停止事件
	psReceiver.Publisher = publisher

	// 监听 Publisher 停止事件，主动停止 PSReceiver
	// 避免 Publisher timeout 后 PSReceiver 仍在阻塞等待数据
	publisher.OnStop(func() {
		d.gb.Info("Publisher 已停止，主动停止 PSReceiver",
			"downloadId", d.DownloadId,
			"progress", d.Progress)
		psReceiver.Stop(io.EOF)
	})

	// 7. 创建 Recorder 订阅 Publisher 并录制
	// 使用 MP4 插件的标准录制配置
	if mp4Plugin, ok := d.gb.Server.Plugins.Get("MP4"); ok && mp4Plugin.Meta.NewRecorder != nil {
		// 生成文件路径：record/{deviceId}/{channelId}/{timestamp}
		// Fragment=0 表示不分片，FilePath 是完整路径（不含 .mp4 扩展名）
		filePath := filepath.Join("record", streamPath)

		recConf := config.Record{
			Fragment: 0,        // 不分片，单个文件
			FilePath: filePath, // 完整路径（不含扩展名）
		}

		// 使用 Plugin.Record 方法创建录制任务
		mp4Plugin.Record(publisher, recConf, nil)

		// 保存存储路径前缀（用于后续模糊匹配查找完整路径）
		d.FilePath = filePath
		// 生成下载 URL
		d.DownloadUrl = fmt.Sprintf("/gb28181/download?downloadId=%s", d.DownloadId)

		d.gb.Info("MP4 录制器已创建",
			"streamPath", streamPath,
			"storagePathPrefix", filePath,
			"downloadUrl", d.DownloadUrl)
	} else {
		d.gb.Warn("MP4 插件未加载，无法录制")
	}

	d.gb.Info("开始接收 RTP 数据并录制", "streamPath", streamPath)

	// 8. 设置进度更新回调（在 RTP 读取循环中触发，无需单独协程）
	totalDuration := d.EndTime.Sub(d.StartTime).Seconds()
	psReceiver.OnProgressUpdate = func() {
		d.updateProgress(&psReceiver, totalDuration)
	}

	// 9. 使用 RunTask 运行 PSReceiver（会阻塞直到完成）
	err = d.RunTask(&psReceiver)

	// 10. 任务完成，更新状态
	if err != nil {
		// 判断是否为正常结束：EOF/timeout 且视频PTS已稳定（说明流真的结束了）
		errStr := err.Error()
		isNormalEnd := err == io.EOF ||
			strings.Contains(errStr, "EOF") ||
			strings.Contains(errStr, "timeout")

		// PTS稳定说明设备已经停止发送数据，流真正结束了
		ptsStable := psReceiver.IsPtsStable()

		if isNormalEnd && ptsStable {
			d.gb.Info("下载完成：视频 PTS 已稳定，视为成功",
				"downloadId", d.DownloadId,
				"progress", d.Progress,
				"error", errStr)
			d.Status = "completed"
			d.Progress = 100
			d.Error = "" // 清除错误信息
		} else {
			d.Status = "failed"
			d.Error = err.Error()
			d.gb.Warn("下载失败",
				"downloadId", d.DownloadId,
				"progress", d.Progress,
				"ptsStable", ptsStable,
				"error", errStr)
		}
	} else {
		d.Status = "completed"
		d.Progress = 100
	}
	d.CompletedAt = time.Now()

	// 11. 延迟 5 秒后再返回，确保前端能轮询到 100% 状态
	if d.Status == "completed" {
		d.gb.Info("下载任务已完成，延迟 5 秒后释放资源（确保前端获取到 100% 状态）",
			"downloadId", d.DownloadId,
			"progress", d.Progress)

		// 12. 从 RecordStream 表查询完整的文件路径（通过 LIKE 模糊匹配）
		var actualFilePath string
		if d.gb.DB != nil && d.FilePath != "" {
			var record m7s.RecordStream
			// 使用 LIKE 查询匹配存储路径前缀的记录
			if err := d.gb.DB.Where("file_path LIKE ?", d.FilePath+"%").
				Order("start_time DESC").First(&record).Error; err == nil {
				actualFilePath = record.FilePath
				d.FilePath = actualFilePath // 更新为完整路径
				d.gb.Info("找到完整文件路径",
					"downloadId", d.DownloadId,
					"filePath", actualFilePath)
			} else {
				d.gb.Warn("未找到匹配的录制文件",
					"downloadId", d.DownloadId,
					"searchPath", d.FilePath,
					"error", err)
			}
		}

		completed := &CompletedDownloadDialog{
			DownloadId:  d.DownloadId,
			DeviceId:    d.DeviceId,
			ChannelId:   d.ChannelId,
			Status:      d.Status,
			Progress:    d.Progress,
			FilePath:    d.FilePath,
			DownloadUrl: d.DownloadUrl,
			Error:       d.Error,
			StartedAt:   d.StartedAt,
			CompletedAt: d.CompletedAt,
		}
		d.gb.completedDownloads.Set(completed)

		// 13. 保存到 GB28181Record 缓存表并清理RecordStream记录
		if d.gb.DB != nil && actualFilePath != "" {
			record := &gb28181.GB28181Record{
				DownloadId: d.DownloadId,
				FilePath:   actualFilePath,
				Status:     "completed",
			}
			// 使用 Save 方法，如果存在则更新，不存在则插入
			if err := d.gb.DB.Save(record).Error; err != nil {
				d.gb.Error("保存下载记录到缓存表失败",
					"downloadId", d.DownloadId,
					"error", err)
			} else {
				d.gb.Info("下载记录已保存到缓存表",
					"downloadId", d.DownloadId,
					"filePath", actualFilePath)

				// 清理MP4插件的RecordStream记录（通过完整路径）
				if err := d.gb.DB.Where("file_path = ?", actualFilePath).Delete(&m7s.RecordStream{}).Error; err != nil {
					d.gb.Error("删除RecordStream记录失败",
						"filePath", actualFilePath,
						"error", err)
				} else {
					d.gb.Info("已清理RecordStream记录",
						"filePath", actualFilePath)
				}
			}
		}
	} else if d.Status == "failed" {
		// 14. 下载失败时也需要清理RecordStream记录（通过 LIKE 模糊匹配）
		if d.gb.DB != nil && d.FilePath != "" {
			if err := d.gb.DB.Where("file_path LIKE ?", d.FilePath+"%").Delete(&m7s.RecordStream{}).Error; err != nil {
				d.gb.Error("删除失败任务的RecordStream记录失败",
					"searchPath", d.FilePath,
					"error", err)
			} else {
				d.gb.Info("已清理失败任务的RecordStream记录",
					"searchPath", d.FilePath)
			}
		}
	}

	return err
}

// updateProgress 更新下载进度（在 PS 流解析过程中通过回调触发）
func (d *DownloadDialog) updateProgress(psReceiver *mrtp.PSReceiver, totalDuration float64) {
	// 基于视频 PTS 的进度计算（与倍速无关，反映真实媒体时长）
	elapsedSeconds := psReceiver.GetElapsedSeconds()
	progress := int(elapsedSeconds / totalDuration * 100)

	if progress > 100 {
		progress = 100
	}
	if progress < 0 {
		progress = 0
	}
	d.Progress = progress

	d.gb.Info("下载进度更新",
		"downloadId", d.DownloadId,
		"elapsedSeconds", elapsedSeconds,
		"totalDuration", totalDuration,
		"progress", progress,
		"filePath", d.FilePath)
}

// Dispose 释放资源
func (d *DownloadDialog) Dispose() {
	go func() {
		time.Sleep(60 * time.Second)
		switch d.device.StreamMode {
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
	}()

	// 2. 记录日志
	d.gb.Info("download dialog dispose",
		"downloadId", d.DownloadId,
		"ssrc", d.SSRC,
		"mediaPort", d.MediaPort,
		"deviceId", d.DeviceId,
		"channelId", d.ChannelId,
		"status", d.Status)

	// 3. 发送 BYE 结束会话
	if d.session != nil && d.session.InviteResponse != nil {
		err := d.session.Bye(d)
		if err != nil {
			d.gb.Error("发送 BYE 失败", "error", err)
		}
	}
}
