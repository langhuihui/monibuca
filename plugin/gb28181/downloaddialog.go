package plugin_gb28181pro

import (
	"fmt"
	"io"
	"os"
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
	SSRC       uint32
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
	Status          string // pending/downloading/completed/failed
	Progress        int    // 0-100
	FilePath        string
	DownloadUrl     string // 下载链接
	Error           string
	DownloadedBytes int64
	TotalBytes      int64
	StartedAt       time.Time
	CompletedAt     time.Time
}

// GetKey 返回下载任务的唯一标识
func (d *DownloadDialog) GetKey() string {
	return d.DownloadId
}

// Start 启动下载会话
func (d *DownloadDialog) Start() error {
	// 更新状态
	d.Status = "downloading"
	d.StartedAt = time.Now()

	// 1. 获取设备和通道
	device, ok := d.gb.devices.Get(d.DeviceId)
	if !ok {
		d.Status = "failed"
		d.Error = fmt.Sprintf("设备不存在: %s", d.DeviceId)
		return fmt.Errorf(d.Error)
	}
	d.device = device

	channelKey := d.DeviceId + "_" + d.ChannelId
	channel, ok := device.channels.Get(channelKey)
	if !ok {
		d.Status = "failed"
		d.Error = fmt.Sprintf("通道不存在: %s", d.ChannelId)
		return fmt.Errorf(d.Error)
	}
	d.channel = channel

	// 2. 分配媒体端口
	if device.StreamMode != mrtp.StreamModeTCPActive {
		if d.gb.MediaPort.Valid() {
			select {
			case d.MediaPort = <-d.gb.tcpPorts:
			default:
				return fmt.Errorf("没有可用的 TCP 端口")
			}
		} else {
			d.MediaPort = d.gb.MediaPort[0]
		}
	}

	// 3. 生成 SSRC
	d.SSRC = device.CreateSSRC(d.gb.Serial)

	// 4. 构建 SDP
	startTimestamp := d.StartTime.Unix()
	endTimestamp := d.EndTime.Unix()

	sdpInfo := []string{
		"v=0",
		fmt.Sprintf("o=%s 0 0 IN IP4 %s", device.DeviceId, device.MediaIp),
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
	ssrcStr := strconv.FormatUint(uint64(d.SSRC), 10)
	sdpInfo = append(sdpInfo, fmt.Sprintf("y=%s", ssrcStr))

	// 5. 创建 INVITE 请求
	request := sip.NewRequest(sip.INVITE, sip.Uri{User: d.ChannelId, Host: device.IP})
	subject := fmt.Sprintf("%s:%s,%s:0", d.ChannelId, ssrcStr, device.DeviceId)

	contentTypeHeader := sip.ContentTypeHeader("APPLICATION/SDP")
	subjectHeader := sip.NewHeader("Subject", subject)
	request.SetBody([]byte(strings.Join(sdpInfo, "\r\n") + "\r\n"))

	recipient := device.Recipient
	recipient.User = d.ChannelId

	fromHDR := sip.FromHeader{
		Address: sip.Uri{
			User: d.gb.Serial,
			Host: device.MediaIp,
			Port: device.LocalPort,
		},
		Params: sip.NewParams(),
	}
	toHeader := sip.ToHeader{
		Address: sip.Uri{User: d.ChannelId, Host: d.ChannelId[0:10]},
	}
	fromHDR.Params.Add("tag", sip.GenerateTagN(16))

	// 6. 创建会话并发送 INVITE
	dialogClientCache := sipgo.NewDialogClientCache(device.client, device.contactHDR)
	d.gb.Info("发送 INVITE 请求",
		"deviceId", d.DeviceId,
		"channelId", d.ChannelId,
		"startTime", d.StartTime,
		"endTime", d.EndTime,
		"ssrc", ssrcStr)

	session, err := dialogClientCache.Invite(d.gb, recipient, request.Body(), &fromHDR, &toHeader, subjectHeader, &contentTypeHeader)
	if err != nil {
		return fmt.Errorf("发送 INVITE 失败: %w", err)
	}
	d.session = session

	return nil
}

// Go 运行下载会话（异步执行，支持并发）
func (d *DownloadDialog) Go() error {
	// 1. 等待 200 OK 响应
	err := d.session.WaitAnswer(d.gb, sipgo.AnswerOptions{})
	if err != nil {
		d.Status = "failed"
		d.Error = fmt.Sprintf("等待响应失败: %v", err)
		return fmt.Errorf("等待响应失败: %w", err)
	}

	// 2. 解析响应
	inviteResponseBody := string(d.session.InviteResponse.Body())
	d.gb.Info("收到 INVITE 响应", "body", inviteResponseBody)

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

	// 3. 发送 ACK
	err = d.session.Ack(d.gb)
	if err != nil {
		d.Status = "failed"
		d.Error = fmt.Sprintf("发送 ACK 失败: %v", err)
		return fmt.Errorf("发送 ACK 失败: %w", err)
	}

	d.gb.Info("下载会话已建立",
		"ssrc", d.SSRC,
		"targetIP", d.targetIP,
		"targetPort", d.targetPort)

	// 4. 使用简洁的流路径格式
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

	// 6. 创建 PSReceiver 接收 RTP 并解析 PS 流
	var psReceiver mrtp.PSReceiver
	psReceiver.Publisher = publisher

	// 监听 Publisher 停止事件，主动停止 PSReceiver
	// 避免 Publisher timeout 后 PSReceiver 仍在阻塞等待数据
	publisher.OnStop(func() {
		d.gb.Info("Publisher 已停止，主动停止 PSReceiver",
			"downloadId", d.DownloadId,
			"progress", d.Progress)
		psReceiver.Stop(io.EOF)
	})

	// 配置接收器
	switch d.device.StreamMode {
	case mrtp.StreamModeTCPActive:
		psReceiver.ListenAddr = fmt.Sprintf("%s:%d", d.targetIP, d.targetPort)
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
			psReceiver.SinglePort = reader
			d.OnStop(func() {
				reader.Close()
				d.gb.singlePorts.Remove(reader)
			})
		}
		psReceiver.ListenAddr = fmt.Sprintf(":%d", d.MediaPort)
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
			psReceiver.SinglePort = reader
			d.OnStop(func() {
				reader.Close()
				d.gb.singlePorts.Remove(reader)
			})
		}
		psReceiver.ListenAddr = fmt.Sprintf(":%d", d.MediaPort)
	}
	psReceiver.StreamMode = d.device.StreamMode

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

		d.gb.Info("MP4 录制器已创建", "streamPath", streamPath, "filePath", filePath)
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
		// 判断是否为正常结束：EOF/timeout 且 RTP 时间戳已稳定（说明流真的结束了）
		errStr := err.Error()
		isNormalEnd := err == io.EOF ||
			strings.Contains(errStr, "EOF") ||
			strings.Contains(errStr, "timeout")
		
		// 时间戳稳定说明设备已经停止发送数据，流真正结束了
		timestampStable := psReceiver.IsTimestampStable()

		if isNormalEnd && timestampStable {
			d.gb.Info("下载完成：RTP 时间戳已稳定，视为成功",
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
				"timestampStable", timestampStable,
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
		time.Sleep(5 * time.Second)
		d.gb.Info("延迟时间到，准备释放资源", "downloadId", d.DownloadId)
	}

	return err
}

// updateProgress 更新下载进度（在 RTP 读取循环中通过回调触发）
func (d *DownloadDialog) updateProgress(psReceiver *mrtp.PSReceiver, totalDuration float64) {
	// 基于 RTP 时间戳的进度计算（与倍速无关）
	elapsedSeconds := psReceiver.GetElapsedSeconds()
	progress := int(elapsedSeconds / totalDuration * 100)

	if progress > 100 {
		progress = 100
	}
	if progress < 0 {
		progress = 0
	}
	d.Progress = progress

	// 尝试从 MP4 插件的数据库中获取文件信息
	if mp4Plugin, ok := d.gb.Server.Plugins.Get("MP4"); ok {
		if mp4Plugin.DB != nil {
			var record m7s.RecordStream
			// 查询最新的录制记录
			if err := mp4Plugin.DB.Where("stream_path = ? AND type = ?", psReceiver.Publisher.StreamPath, "mp4").
				Order("start_time DESC").First(&record).Error; err == nil {
				d.FilePath = record.FilePath

				// 使用 record.ID 生成下载链接（单文件下载）
				// 这样无论录制是否完成，都能正确下载
				d.DownloadUrl = fmt.Sprintf("/mp4/download/%s.mp4?id=%d",
					psReceiver.Publisher.StreamPath,
					record.ID)

				// 获取文件大小
				if fileInfo, err := os.Stat(record.FilePath); err == nil {
					d.DownloadedBytes = fileInfo.Size()
					// 根据当前进度估算总大小
					if progress > 0 && progress < 100 {
						d.TotalBytes = d.DownloadedBytes * 100 / int64(progress)
					} else if progress >= 100 {
						d.TotalBytes = d.DownloadedBytes
					}
				}
			}
		}
	}

	d.gb.Info("下载进度更新",
		"downloadId", d.DownloadId,
		"elapsedSeconds", elapsedSeconds,
		"totalDuration", totalDuration,
		"progress", progress,
		"downloadedBytes", d.DownloadedBytes,
		"totalBytes", d.TotalBytes,
		"filePath", d.FilePath)
}

// Dispose 释放资源
func (d *DownloadDialog) Dispose() {
	d.gb.Info("download dialog dispose", "downloadId", d.DownloadId, "deviceId", d.DeviceId, "channelId", d.ChannelId)
	// 1. 回收端口
	if d.device != nil {
		if d.device.StreamMode == mrtp.StreamModeUDP {
			if d.gb.udpPort == 0 { // 多端口模式
				select {
				case d.gb.udpPorts <- d.MediaPort:
					d.gb.Info("udp port returned", "port", d.MediaPort)
				default:
					d.gb.Warn("udpPorts channel full, port not returned", "port", d.MediaPort)
				}
			}
		} else if d.device.StreamMode == mrtp.StreamModeTCPPassive {
			if d.gb.tcpPort == 0 { // 多端口模式
				select {
				case d.gb.tcpPorts <- d.MediaPort:
					d.gb.Info("tcp port returned", "port", d.MediaPort)
				default:
					d.gb.Warn("tcpPorts channel full, port not returned", "port", d.MediaPort)
				}
			}
		}
	}

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
