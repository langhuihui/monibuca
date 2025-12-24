package plugin_snap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	m7s "m7s.live/v5"
	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/storage"
	"m7s.live/v5/pkg/util"
	snap_pkg "m7s.live/v5/plugin/snap/pkg"
	"m7s.live/v5/plugin/snap/pkg/watermark"
)

const (
	MacFont   = "/System/Library/Fonts/STHeiti Light.ttc"                // mac字体路径
	LinuxFont = "/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc" // linux字体路径 思源黑体
	WinFont   = "C:/Windows/Fonts/msyh.ttf"                              // windows字体路径 微软雅黑
)

func parseRGBA(rgba string) (color.RGBA, error) {
	rgba = strings.TrimPrefix(rgba, "rgba(")
	rgba = strings.TrimSuffix(rgba, ")")
	parts := strings.Split(rgba, ",")
	if len(parts) != 4 {
		return color.RGBA{}, fmt.Errorf("invalid rgba format")
	}
	r, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
	g, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
	b, _ := strconv.Atoi(strings.TrimSpace(parts[2]))
	a, _ := strconv.ParseFloat(strings.TrimSpace(parts[3]), 64)
	return color.RGBA{uint8(r), uint8(g), uint8(b), uint8(a * 255)}, nil
}

// snap 方法负责实际的截图操作
func (p *SnapPlugin) snap(publisher *m7s.Publisher, watermarkConfig *snap_pkg.WatermarkConfig) (*bytes.Buffer, error) {

	// 获取视频帧
	annexb, err := snap_pkg.GetVideoFrame(publisher, p.Server)
	if err != nil {
		return nil, err
	}

	// 处理视频帧生成图片
	buf := new(bytes.Buffer)
	if err := snap_pkg.ProcessWithFFmpeg(annexb, buf, p.FFMPEGPath); err != nil {
		return nil, err
	}

	// 如果设置了水印文字，添加水印
	if watermarkConfig != nil && watermarkConfig.Text != "" {
		// 加载字体
		if err := watermarkConfig.LoadFont(); err != nil {
			return nil, fmt.Errorf("load watermark font failed: %w", err)
		}

		// 解码图片
		img, _, err := image.Decode(bytes.NewReader(buf.Bytes()))
		if err != nil {
			return nil, fmt.Errorf("decode image failed: %w", err)
		}

		// 添加水印
		result, err := watermark.DrawWatermarkSingle(img, watermark.TextConfig{
			Text:       watermarkConfig.Text,
			Font:       watermarkConfig.Font,
			FontSize:   watermarkConfig.FontSize,
			Spacing:    watermarkConfig.FontSpacing,
			RowSpacing: 10,
			ColSpacing: 20,
			Rows:       1,
			Cols:       1,
			DPI:        72,
			Color:      watermarkConfig.FontColor,
			IsGrid:     false,
			Angle:      0,
			OffsetX:    watermarkConfig.OffsetX,
			OffsetY:    watermarkConfig.OffsetY,
		}, false)
		if err != nil {
			return nil, fmt.Errorf("add watermark failed: %w", err)
		}

		// 清空原buffer并写入新图片
		buf.Reset()
		if err := imaging.Encode(buf, result, imaging.JPEG); err != nil {
			return nil, fmt.Errorf("encode image failed: %w", err)
		}
	}

	return buf, nil
}

func (p *SnapPlugin) doSnap(rw http.ResponseWriter, r *http.Request) {
	streamPath := r.PathValue("streamPath")
	isUrl := r.URL.Query().Get("isUrl")

	// 获取发布者
	publisher, err := p.Server.GetPublisher(streamPath)
	if err != nil {
		http.Error(rw, pkg.ErrNotFound.Error(), http.StatusNotFound)
		return
	}

	// 获取查询参数
	query := r.URL.Query()

	// 从查询参数中获取水印配置
	var watermarkConfig *snap_pkg.WatermarkConfig
	watermarkText := query.Get("watermark")
	if watermarkText != "" {
		fontPath := query.Get("fontPath")
		if fontPath == "" {
			switch {
			case strings.Contains(runtime.GOOS, "darwin"):
				fontPath = MacFont
			case strings.Contains(runtime.GOOS, "linux"):
				fontPath = LinuxFont
			case strings.Contains(runtime.GOOS, "windows"):
				fontPath = WinFont
			}
		}
		watermarkConfig = &snap_pkg.WatermarkConfig{
			Text:        watermarkText,
			FontPath:    fontPath,
			FontSize:    parseFloat64(query.Get("fontSize"), 36),
			FontSpacing: parseFloat64(query.Get("fontSpacing"), 2),
			OffsetX:     parseInt(query.Get("offsetX"), 0),
			OffsetY:     parseInt(query.Get("offsetY"), 0),
		}

		// 解析颜色
		if fontColor := query.Get("fontColor"); fontColor != "" {
			if color, err := parseRGBA(fontColor); err == nil {
				watermarkConfig.FontColor = color
			}
		}
	}

	// 调用 snap 进行截图
	buf, err := p.snap(publisher, watermarkConfig)
	if err != nil {
		p.Error("snap failed", "error", err.Error())
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	// 处理保存逻辑
	savePath := query.Get("savePath")
	now := time.Now().UTC()
	if savePath != "" {
		os.Mkdir(savePath, 0755)
		filename := fmt.Sprintf("%s_%s.jpg", streamPath, now.Format("20060102150405.000"))
		filename = strings.ReplaceAll(filename, "/", "_")
		savePath = filepath.Join(savePath, filename)

		// 保存到本地
		if err := os.WriteFile(savePath, buf.Bytes(), 0644); err != nil {
			p.Error("save snapshot failed", "error", err.Error())
			savePath = ""
		}

		// 保存截图记录到数据库
		if p.DB != nil && savePath != "" {
			record := snap_pkg.SnapRecord{
				StreamName: streamPath,
				SnapMode:   2, // HTTP请求截图模式
				SnapTime:   now,
				SnapPath:   savePath,
			}
			if err := p.DB.Create(&record).Error; err != nil {
				p.Error("save snapshot record failed", "error", err.Error())
			}
		}
	}

	if isUrl == "1" && savePath != "" {

		url := fmt.Sprintf("http://%s/snap/query/%s?snapTime=%d", "localhost:8080", streamPath, now.Unix())
		data := map[string]string{
			"url":      url,
			"markdown": fmt.Sprintf("![%s](%s)", streamPath, url),
		}
		rw.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(rw).Encode(data); err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
		}
	} else {
		// 返回图片
		rw.Header().Set("Content-Type", "image/jpeg")
		rw.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
		if _, err := buf.WriteTo(rw); err != nil {
			p.Error("write response failed", "error", err.Error())
		}
	}

}

// 辅助函数：解析浮点数
func parseFloat64(s string, defaultValue float64) float64 {
	if s == "" {
		return defaultValue
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return defaultValue
	}
	return v
}

// 辅助函数：解析整数
func parseInt(s string, defaultValue int) int {
	if s == "" {
		return defaultValue
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return defaultValue
	}
	return v
}

func (p *SnapPlugin) querySnap(rw http.ResponseWriter, r *http.Request) {
	if p.DB == nil {
		http.Error(rw, "database not initialized", http.StatusInternalServerError)
		return
	}
	var err error

	streamPath := r.PathValue("streamPath")
	if streamPath == "" {
		http.Error(rw, "streamPath is required", http.StatusBadRequest)
		return
	}

	snapTimeStr := r.URL.Query().Get("snapTime")
	if snapTimeStr == "" {
		http.Error(rw, "snapTime is required", http.StatusBadRequest)
		return
	}

	snapTimeUnix, err := strconv.ParseInt(snapTimeStr, 10, 64)
	if err != nil {
		http.Error(rw, "invalid snapTime format, should be unix timestamp", http.StatusBadRequest)
		return
	}

	// 将时间戳转换为UTC时间，确保与数据库中存储的UTC时间一致
	targetTime := time.Unix(snapTimeUnix+1, 0).UTC()
	var record snap_pkg.SnapRecord

	// 查询小于等于目标时间的最近一条记录
	if err := p.DB.Where("stream_name = ? AND snap_time <= ?", streamPath, targetTime).
		Order("id DESC").
		First(&record).Error; err != nil {
		http.Error(rw, "snapshot not found", http.StatusNotFound)
		return
	}

	// 计算时间差（秒）
	timeDiff := targetTime.Sub(record.SnapTime).Seconds()
	if timeDiff > float64(time.Duration(p.QueryTimeDelta)*time.Second) {
		http.Error(rw, "no snapshot found within time delta", http.StatusNotFound)
		return
	}

	// 读取图片文件
	var imgData []byte
	if st := p.Server.Storage; st != nil {
		// 优先通过全局存储读取
		if localStorage, ok := st.(*storage.LocalStorage); ok {
			path := record.SnapPath
			if !filepath.IsAbs(path) {
				path = localStorage.GetFullPath(path, 1)
			}
			imgData, err = os.ReadFile(path)
		} else {
			var f storage.File
			f, err = st.OpenFile(context.Background(), record.SnapPath)
			if err == nil {
				defer f.Close()
				imgData, err = io.ReadAll(f)
			}
		}
	} else {
		imgData, err = os.ReadFile(record.SnapPath)
	}
	if err != nil {
		http.Error(rw, "failed to read snapshot file", http.StatusNotFound)
		return
	}

	rw.Header().Set("Content-Type", "image/jpeg")
	rw.Header().Set("Content-Length", strconv.Itoa(len(imgData)))
	rw.Write(imgData)
}

// BatchSnapRequest 批量截图请求结构体
type BatchSnapRequest struct {
	StartTime   string `json:"startTime"`   // 开始时间 UTC格式
	EndTime     string `json:"endTime"`     // 结束时间 UTC格式
	Granularity int    `json:"granularity"` // 颗粒度(间隔秒数)，0表示按关键帧
}

// BatchSnapResponse 批量截图响应结构体
type BatchSnapResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// batchSnap 处理批量截图请求
func (p *SnapPlugin) batchSnap(rw http.ResponseWriter, r *http.Request) {
	// 只接受GET请求
	if r.Method != http.MethodGet {
		http.Error(rw, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 获取streamPath
	streamPath := r.PathValue("streamPath")
	if streamPath == "" {
		responseWithError(rw, "streamPath is required")
		return
	}

	// 获取查询参数
	query := r.URL.Query()

	// 解析时间范围
	startTime, endTime, err := util.TimeRangeQueryParse(query)
	if err != nil {
		responseWithError(rw, "Invalid time range: "+err.Error())
		return
	}

	// 验证时间范围
	if endTime.Before(startTime) {
		responseWithError(rw, "endTime must be after startTime")
		return
	}

	// 获取granularity参数
	granularity := 0
	granularityStr := query.Get("granularity")
	if granularityStr != "" {
		granularityVal, err := strconv.Atoi(granularityStr)
		if err != nil {
			responseWithError(rw, "Invalid granularity format: "+err.Error())
			return
		}
		if granularityVal < 0 {
			responseWithError(rw, "granularity must be non-negative")
			return
		}
		granularity = granularityVal
	}

	// 获取发布者
	publisher, err := p.Server.GetPublisher(streamPath)
	if err != nil {
		responseWithError(rw, "Stream not found")
		return
	}

	// 创建保存目录
	savePath := filepath.Join("snap", streamPath)
	os.MkdirAll(savePath, 0755)
	savePath = strings.ReplaceAll(savePath, "/", "_")
	os.MkdirAll(savePath, 0755)

	// 计算截图时间点
	snapTimes := p.calculateSnapTimes(publisher, startTime, endTime, granularity)

	// 检查截图时间点是否为空
	if len(snapTimes) == 0 {
		p.Warn("no valid snapshot times available",
			"streamPath", streamPath,
			"startTime", startTime.Format(time.RFC3339),
			"endTime", endTime.Format(time.RFC3339),
			"granularity", granularity)

		response := BatchSnapResponse{
			Success: false,
			Message: "No valid snapshot times available. Please check your time range and try again.",
		}

		rw.Header().Set("Content-Type", "application/json")
		json.NewEncoder(rw).Encode(response)
		return
	}

	// 立即返回成功响应，表示任务已接收
	response := BatchSnapResponse{
		Success: true,
		Message: fmt.Sprintf("Batch snap task started. Total snapshots to take: %d. Processing in background.", len(snapTimes)),
	}

	rw.Header().Set("Content-Type", "application/json")
	json.NewEncoder(rw).Encode(response)

	// 在后台异步执行截图任务
	go p.executeBatchSnapTask(publisher, streamPath, snapTimes, savePath, granularity)
}

// responseWithError 返回错误响应
func responseWithError(rw http.ResponseWriter, message string) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(http.StatusBadRequest)
	response := BatchSnapResponse{
		Success: false,
		Message: message,
	}
	json.NewEncoder(rw).Encode(response)
}

// calculateSnapTimes 计算截图时间点
func (p *SnapPlugin) calculateSnapTimes(publisher *m7s.Publisher, startTime, endTime time.Time, granularity int) []time.Time {
	// 检查开始时间是否早于当前时间，如果是，则从当前时间开始
	now := time.Now()
	if startTime.Before(now) {
		p.Info("adjusting start time from past to current time",
			"originalStartTime", startTime.Format(time.RFC3339),
			"adjustedStartTime", now.Format(time.RFC3339))
		startTime = now
	}

	// 检查结束时间是否晚于开始时间
	if endTime.Before(startTime) || endTime.Equal(startTime) {
		p.Warn("invalid time range: end time is not after start time",
			"startTime", startTime.Format(time.RFC3339),
			"endTime", endTime.Format(time.RFC3339))
		return nil
	}

	var snapTimes []time.Time

	// 根据颗粒度确定截图时间点
	if granularity == 0 {
		// 按关键帧截图的逻辑
		// 获取视频轨道的IDRingList（关键帧列表）
		videoTrack := publisher.VideoTrack.AVTrack
		if videoTrack != nil {
			// 获取GOP信息
			gopDuration := time.Duration(0)
			if publisher.GOP > 0 {
				// 如果有GOP信息，估算关键帧间隔
				// 注意：这里的计算只是估算，实际应该使用帧的真实时间戳
				frameRate := float64(30) // 默认帧率，实际应该从视频流中获取
				if videoTrack.ICodecCtx != nil {
					// 获取帧率信息
					if videoTrack.FPS > 0 {
						frameRate = float64(videoTrack.FPS)
					}
				}
				gopDuration = time.Duration(float64(publisher.GOP) / frameRate * float64(time.Second))
				p.Info("estimated GOP duration", "gopFrames", publisher.GOP, "frameRate", frameRate, "duration", gopDuration)
			}

			// 遍历IDRingList获取关键帧
			if videoTrack.IDRingList.Len() > 0 {
				// 从头开始遍历所有关键帧
				for idrElem := videoTrack.IDRingList.Front(); idrElem != nil; idrElem = idrElem.Next() {
					idrRing := idrElem.Value
					if idrRing != nil {
						// 将时间戳转换为time.Time（从纳秒转为秒）
						keyframeTime := time.Unix(0, int64(idrRing.Value.Timestamp))

						// 检查是否在指定时间范围内
						if (keyframeTime.Equal(startTime) || keyframeTime.After(startTime)) &&
							(keyframeTime.Equal(endTime) || keyframeTime.Before(endTime)) {
							snapTimes = append(snapTimes, keyframeTime)
						}
					}
				}
			}

			// 如果没有找到关键帧，但有GOP信息，则使用估算的GOP间隔生成时间点
			if len(snapTimes) == 0 && gopDuration > 0 {
				p.Info("no keyframes found in range, using estimated GOP interval")
				for t := startTime; t.Before(endTime); t = t.Add(gopDuration) {
					snapTimes = append(snapTimes, t)
				}
			} else if len(snapTimes) == 0 {
				// 如果既没有关键帧也没有GOP信息，则默认每2秒一帧
				p.Info("no keyframes or GOP info found, using default 2s interval")
				for t := startTime; t.Before(endTime); t = t.Add(2 * time.Second) {
					snapTimes = append(snapTimes, t)
				}
			}
		} else {
			// 如果没有视频轨道，使用默认间隔
			p.Info("no video track found, using default 2s interval")
			for t := startTime; t.Before(endTime); t = t.Add(2 * time.Second) {
				snapTimes = append(snapTimes, t)
			}
		}
	} else {
		// 按指定间隔截图
		for t := startTime; t.Before(endTime); t = t.Add(time.Duration(granularity) * time.Second) {
			snapTimes = append(snapTimes, t)
		}
	}

	return snapTimes
}

// executeBatchSnapTask 在后台执行批量截图任务
func (p *SnapPlugin) executeBatchSnapTask(publisher *m7s.Publisher, streamPath string, snapTimes []time.Time, savePath string, granularity int) {
	// 检查是否有截图时间点
	if len(snapTimes) == 0 {
		p.Info("batch snap task aborted: no snapshot times")
		return
	}

	// 检查开始时间是否在未来
	firstSnapTime := snapTimes[0]
	now := time.Now()
	if firstSnapTime.After(now) {
		waitDuration := firstSnapTime.Sub(now)
		p.Info("batch snap task scheduled for future",
			"streamPath", streamPath,
			"totalSnapshots", len(snapTimes),
			"startTime", firstSnapTime.Format(time.RFC3339),
			"waitDuration", waitDuration.String())

		// 等待到开始时间
		time.Sleep(waitDuration)
	}

	// 记录任务开始时间
	taskStartTime := time.Now()
	p.Info("batch snap task executing", "streamPath", streamPath, "totalSnapshots", len(snapTimes), "startTime", taskStartTime)

	// 批量截图处理
	var successCount int
	var failCount int

	// 执行批量截图
	for i, snapTime := range snapTimes {
		// 打印日志，记录当前截图时间和进度
		p.Debug("taking snapshot", "progress", fmt.Sprintf("%d/%d", i+1, len(snapTimes)), "time", snapTime.Format(time.RFC3339))

		// 当前实现不支持指定时间截图，所以这里只能截取当前帧
		// 注意：这里每次都会重新创建一个读取器，确保获取到最新的帧
		buf, err := p.snap(publisher, nil)
		if err != nil {
			p.Error("batch snap failed", "error", err.Error(), "time", snapTime.Format(time.RFC3339))
			failCount++
			continue
		}

		// 如果是按间隔截图，每次截图后等待指定时间
		if granularity > 0 && i < len(snapTimes)-1 { // 不是最后一帧才需要等待
			// 等待granularity秒，确保下一次截图与当前截图有足够的时间差
			// 这里直接使用用户指定的granularity参数作为等待时间
			p.Debug("waiting for next snapshot", "granularity", granularity)
			time.Sleep(time.Duration(granularity) * time.Second)
		}

		// 保存截图
		filename := fmt.Sprintf("%s_%s.jpg", streamPath, snapTime.Format("20060102150405.000"))
		filename = strings.ReplaceAll(filename, "/", "_")
		filePath := filepath.Join(savePath, filename)

		if err := os.WriteFile(filePath, buf.Bytes(), 0644); err != nil {
			p.Error("save batch snapshot failed", "error", err.Error())
			failCount++
			continue
		}

		// 保存截图记录到数据库
		if p.DB != nil {
			record := snap_pkg.SnapRecord{
				StreamName: streamPath,
				SnapMode:   3, // 批量截图模式
				SnapTime:   snapTime.UTC(),
				SnapPath:   filePath,
			}
			if err := p.DB.Create(&record).Error; err != nil {
				p.Error("save batch snapshot record failed", "error", err.Error())
			}
		}

		successCount++
	}

	// 记录任务完成时间和结果
	taskEndTime := time.Now()
	taskDuration := taskEndTime.Sub(taskStartTime)
	p.Info("batch snap task completed",
		"streamPath", streamPath,
		"total", len(snapTimes),
		"success", successCount,
		"failed", failCount,
		"duration", taskDuration.String())
}

// batchPlayBack 处理从MP4录像文件中按时间范围和颗粒度进行截图的请求
func (p *SnapPlugin) batchPlayBack(rw http.ResponseWriter, r *http.Request) {
	// 只接受GET请求
	if r.Method != http.MethodGet {
		http.Error(rw, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 检查数据库连接
	if p.DB == nil {
		responseWithError(rw, "数据库未初始化")
		return
	}

	// 获取streamPath
	streamPath := r.PathValue("streamPath")
	if streamPath == "" {
		responseWithError(rw, "streamPath参数必须提供")
		return
	}

	// 获取查询参数
	query := r.URL.Query()

	// 解析时间范围
	startTime, endTime, err := util.TimeRangeQueryParse(query)
	if err != nil {
		responseWithError(rw, "无效的时间范围: "+err.Error())
		return
	}

	// 验证时间范围
	if endTime.Before(startTime) {
		responseWithError(rw, "结束时间必须晚于开始时间")
		return
	}

	// 获取granularity参数
	granularity := 0
	granularityStr := query.Get("granularity")
	if granularityStr != "" {
		granularityVal, err := strconv.Atoi(granularityStr)
		if err != nil {
			responseWithError(rw, "无效的颗粒度格式: "+err.Error())
			return
		}
		if granularityVal < 0 {
			responseWithError(rw, "颗粒度必须为非负数")
			return
		}
		granularity = granularityVal
	}

	// 保存路径前缀（相对路径），实际写入时使用全局存储解析
	savePath := filepath.Join("snap", "playback", streamPath)
	savePath = strings.ReplaceAll(savePath, "/", "_")

	// 立即返回成功响应，表示任务已接收
	response := BatchSnapResponse{
		Success: true,
		Message: fmt.Sprintf("回放截图任务已开始。正在后台处理。时间范围: %s 到 %s (使用参数 start 和 end)", startTime.Format(time.RFC3339), endTime.Format(time.RFC3339)),
	}

	rw.Header().Set("Content-Type", "application/json")
	json.NewEncoder(rw).Encode(response)

	// 在后台异步执行截图任务
	go p.executePlayBackSnapTask(streamPath, startTime, endTime, savePath, granularity)
}

// executePlayBackSnapTask 在后台执行从MP4录像文件中截图的任务
func (p *SnapPlugin) executePlayBackSnapTask(streamPath string, startTime, endTime time.Time, savePath string, granularity int) {
	// 记录任务开始时间
	taskStartTime := time.Now()
	p.Info("playback snap task started", "streamPath", streamPath, "startTime", startTime, "endTime", endTime)

	// 必须有全局存储且为本地存储，便于 ffmpeg 输出到文件系统
	st := p.Server.Storage
	localStorage, ok := st.(*storage.LocalStorage)
	if !ok || st == nil {
		p.Error("playback snap aborted: storage is not LocalStorage")
		return
	}

	// 从数据库中查询指定时间范围内的MP4录像文件
	var streams []m7s.RecordStream
	queryRecord := m7s.RecordStream{
		Type: "mp4",
	}

	// 查询条件：结束时间大于请求的开始时间，开始时间小于请求的结束时间，流路径匹配
	p.DB.Where(&queryRecord).Find(&streams, "end_time>? AND start_time<? AND stream_path=?", startTime, endTime, streamPath)

	// 检查是否找到录像文件
	if len(streams) == 0 {
		p.Warn("no mp4 records found for playback snap", "streamPath", streamPath, "startTime", startTime, "endTime", endTime)
		return
	}

	p.Info("found mp4 records for playback snap", "streamPath", streamPath, "count", len(streams))

	// 按开始时间排序录像文件，确保时间连续性
	sort.Slice(streams, func(i, j int) bool {
		return streams[i].StartTime.Before(streams[j].StartTime)
	})

	// 检查全局存储是否可用
	if p.Server.Storage == nil {
		p.Error("global storage not initialized")
		return
	}

	// 全局截图时间点列表
	var allSnapTimes []time.Time
	// 如果颜粒度小于等于0，则对每个文件提取关键帧
	if granularity <= 0 {
		// 对每个文件分别提取关键帧
		for _, stream := range streams {
			// 检查文件是否存在
			if _, err := os.Stat(stream.FilePath); os.IsNotExist(err) {
				// 如果文件不存在，尝试从全局存储获取完整路径
				var absFilePath string
				if localStorage, ok := p.Server.Storage.(*storage.LocalStorage); ok {
					// 使用本地存储的方法根据存储级别获取完整路径
					absFilePath = localStorage.GetFullPath(stream.FilePath, stream.StorageLevel)
				} else {
					// 非本地存储，尝试使用 GetURL 获取路径
					url, err := p.Server.Storage.GetURL(context.Background(), stream.FilePath)
					if err != nil {
						p.Warn("failed to get file URL from storage", "path", stream.FilePath, "err", err)
						continue
					}
					absFilePath = url
				}
				stream.FilePath = absFilePath
				if _, err := os.Stat(stream.FilePath); os.IsNotExist(err) {
					p.Warn("mp4 file not found", "path", stream.FilePath)
					continue
				}
			}

			// 计算此文件的有效时间范围（与请求时间范围的交集）
			fileStartTime := stream.StartTime
			if fileStartTime.Before(startTime) {
				fileStartTime = startTime
			}

			fileEndTime := stream.EndTime
			if fileEndTime.After(endTime) {
				fileEndTime = endTime
			}

			// 提取关键帧
			keyFrameTimes, err := p.extractKeyFrameTimes(stream.FilePath, fileStartTime, fileEndTime)
			if err != nil {
				p.Error("extract key frames failed", "error", err.Error())
				// 如果提取失败，使用默认的每2秒截图
				defaultGranularity := 2 * time.Second
				for t := fileStartTime; t.Before(fileEndTime); t = t.Add(defaultGranularity) {
					allSnapTimes = append(allSnapTimes, t)
				}
			} else {
				// 将关键帧时间点添加到全局列表
				allSnapTimes = append(allSnapTimes, keyFrameTimes...)
			}
		}
	} else {
		// 当指定颜粒度时，基于整个时间范围生成均匀的截图时间点
		// 这样可以确保在不同文件之间保持一致的颜粒度
		for t := startTime; t.Before(endTime); t = t.Add(time.Duration(granularity) * time.Second) {
			allSnapTimes = append(allSnapTimes, t)
		}
	}

	// 按时间排序并去重
	sort.Slice(allSnapTimes, func(i, j int) bool {
		return allSnapTimes[i].Before(allSnapTimes[j])
	})

	// 去除重复的时间点（如果有）
	var uniqueSnapTimes []time.Time
	if len(allSnapTimes) > 0 {
		uniqueSnapTimes = append(uniqueSnapTimes, allSnapTimes[0])
		for i := 1; i < len(allSnapTimes); i++ {
			// 如果与前一个时间点不同，则添加
			if !allSnapTimes[i].Equal(allSnapTimes[i-1]) {
				uniqueSnapTimes = append(uniqueSnapTimes, allSnapTimes[i])
			}
		}
	}

	p.Info("generated snapshot times", "count", len(uniqueSnapTimes))

	// 处理每个截图时间点
	var successCount, failCount int
	for _, snapTime := range uniqueSnapTimes {
		// 找到包含该时间点的录像文件
		var targetStream *m7s.RecordStream
		for j := range streams {
			if (snapTime.Equal(streams[j].StartTime) || snapTime.After(streams[j].StartTime)) &&
				(snapTime.Equal(streams[j].EndTime) || snapTime.Before(streams[j].EndTime)) {
				targetStream = &streams[j]
				break
			}
		}

		// 如果找不到对应的文件，跳过该时间点
		if targetStream == nil {
			p.Warn("no mp4 file found for time point", "time", snapTime.Format(time.RFC3339))
			failCount++
			continue
		}

		// 检查文件是否存在
		if _, err := os.Stat(targetStream.FilePath); os.IsNotExist(err) {
			// 如果文件不存在，尝试从全局存储获取完整路径
			if localStorage, ok := p.Server.Storage.(*storage.LocalStorage); ok {
				// 使用本地存储的方法根据存储级别获取完整路径
				targetStream.FilePath = localStorage.GetFullPath(targetStream.FilePath, targetStream.StorageLevel)
			} else {
				// 非本地存储，尝试使用 GetURL 获取路径
				url, err := p.Server.Storage.GetURL(context.Background(), targetStream.FilePath)
				if err != nil {
					p.Warn("mp4 file not found and failed to get URL", "path", targetStream.FilePath, "err", err)
					failCount++
					continue
				}
				targetStream.FilePath = url
			}
			// 再次检查文件是否存在
			if _, err := os.Stat(targetStream.FilePath); os.IsNotExist(err) {
				p.Warn("mp4 file not found", "path", targetStream.FilePath)
				failCount++
				continue
			}
		}

		// 计算在文件中的时间偏移（毫秒）
		// 使用文件的duration字段来计算时间偏移
		// 首先计算截图时间点在整个文件时间范围内的相对位置
		fileStartTime := targetStream.StartTime
		fileEndTime := targetStream.EndTime
		fileDuration := targetStream.Duration

		// 如果数据库中的duration字段有效，则使用它来计算时间偏移
		var timeOffset int64
		if fileDuration > 0 {
			// 注意：duration字段存储的是毫秒值，如 69792 表示 69.792 秒
			// 计算截图时间点在整个文件时间范围内的相对位置（百分比）
			totalDuration := fileEndTime.Sub(fileStartTime).Milliseconds()
			if totalDuration > 0 {
				position := float64(snapTime.Sub(fileStartTime).Milliseconds()) / float64(totalDuration)
				// 根据百分比位置和实际duration计算出时间偏移
				// duration已经是毫秒值，直接使用
				timeOffset = int64(position * float64(fileDuration))
				p.Debug("using duration for time offset calculation", "position", position, "duration_ms", fileDuration, "timeOffset_ms", timeOffset)
			} else {
				// 如果计算出问题，回退到直接使用时间差
				timeOffset = snapTime.Sub(fileStartTime).Milliseconds()
				p.Debug("fallback to direct time difference", "timeOffset", timeOffset)
			}
		} else {
			// 如果duration无效，则使用时间差
			timeOffset = snapTime.Sub(fileStartTime).Milliseconds()
			p.Debug("invalid duration, using time difference", "timeOffset", timeOffset)
		}

		// 使用FFmpeg从MP4文件中截取指定时间点的图片
		// 文件名包含截图时间点和颜粒度信息，避免不同颜粒度的截图相互覆盖
		var granularityInfo string
		if granularity <= 0 {
			granularityInfo = "keyframe"
		} else {
			granularityInfo = fmt.Sprintf("%ds", granularity)
		}

		filename := fmt.Sprintf("%s_%s_%s.jpg",
			streamPath,
			snapTime.Format("20060102150405"),
			granularityInfo)
		filename = strings.ReplaceAll(filename, "/", "_")
		relPath := filepath.Join(savePath, filename)
		fullPath := localStorage.GetFullPath(relPath, 1)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			p.Error("create snapshot directory failed", "err", err, "path", fullPath)
			failCount++
			continue
		}

		// 调用截图函数输出到本地完整路径
		err := p.snapFromMP4(targetStream.FilePath, fullPath, timeOffset)
		if err != nil {
			p.Error("playback snap failed", "error", err.Error(), "time", snapTime.Format(time.RFC3339))
			failCount++
			continue
		}

		// 保存截图记录到数据库
		if p.DB != nil {
			record := snap_pkg.SnapRecord{
				StreamName: streamPath,
				SnapMode:   4, // 回放截图模式
				SnapTime:   snapTime,
				SnapPath:   relPath,
			}
			if err := p.DB.Create(&record).Error; err != nil {
				p.Error("save playback snapshot record failed", "error", err.Error())
			}
		}

		successCount++
	}

	// 记录任务完成时间和结果
	taskEndTime := time.Now()
	taskDuration := taskEndTime.Sub(taskStartTime)
	p.Info("playback snap task completed",
		"streamPath", streamPath,
		"success", successCount,
		"failed", failCount,
		"duration", taskDuration.String())
}

// snapFromMP4 从MP4文件中截取指定时间点的图片
func (p *SnapPlugin) snapFromMP4(mp4FilePath, outputPath string, timeOffsetMs int64) error {
	// 将时间偏移转换为秒
	timeOffsetSec := float64(timeOffsetMs) / 1000.0

	// 构建ffmpeg命令
	cmd := exec.Command(
		p.FFMPEGPath,
		"-hide_banner",
		"-ss", fmt.Sprintf("%f", timeOffsetSec), // 设置时间偏移
		"-i", mp4FilePath, // 输入文件
		"-vframes", "1", // 只截取一帧
		"-q:v", "2", // 设置图片质量
		"-y",       // 覆盖输出文件
		outputPath, // 输出文件路径
	)

	// 执行命令
	output, err := cmd.CombinedOutput()
	if err != nil {
		p.Error("ffmpeg command failed", "error", err.Error(), "output", string(output))
		return fmt.Errorf("ffmpeg error: %s, output: %s", err.Error(), string(output))
	}

	return nil
}

// extractKeyFrameTimes 从MP4文件中提取关键帧时间点
func (p *SnapPlugin) extractKeyFrameTimes(mp4FilePath string, startTime, endTime time.Time) ([]time.Time, error) {
	// 使用FFmpeg的-skip_frame nokey参数和-show_entries frame=pkt_pts_time参数提取关键帧时间
	cmd := exec.Command(
		"ffprobe",
		"-v", "quiet",
		"-select_streams", "v",
		"-skip_frame", "nokey", // 只处理关键帧
		"-show_entries", "frame=pkt_pts_time", // 显示帧的时间戳
		"-of", "csv=p=0", // 输出为CSV格式
		"-i", mp4FilePath,
	)

	// 执行命令
	output, err := cmd.CombinedOutput()
	if err != nil {
		p.Error("ffprobe command failed", "error", err.Error(), "output", string(output))
		return nil, fmt.Errorf("ffprobe error: %s", err.Error())
	}

	// 解析输出结果，提取时间戳
	lines := strings.Split(string(output), "\n")

	// 获取MP4文件的开始时间信息
	// 注意：ffprobe返回的时间戳是相对于文件开始的秒数
	// 我们需要将其转换为绝对时间
	var fileStartTime time.Time
	// 使用数据库中记录的文件开始时间
	// 查询数据库获取文件信息
	var fileInfo m7s.RecordStream
	if err := p.DB.Where("file_path = ?", mp4FilePath).First(&fileInfo).Error; err == nil {
		fileStartTime = fileInfo.StartTime
	} else {
		p.Warn("failed to get file start time from database, using request start time", "error", err.Error())
		fileStartTime = startTime
	}

	p.Info("file start time", "time", fileStartTime.Format(time.RFC3339))

	// 存储关键帧时间点
	var keyFrameTimes []time.Time

	// 处理每一行输出
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 将时间戳转换为浮点数（秒）
		timeOffsetSec, err := strconv.ParseFloat(line, 64)
		if err != nil {
			p.Warn("invalid time format in ffprobe output", "line", line)
			continue
		}

		// 计算实际时间：文件开始时间 + 偏移秒数
		frameTime := fileStartTime.Add(time.Duration(timeOffsetSec * float64(time.Second)))

		// 只保留在请求时间范围内的关键帧
		if (frameTime.Equal(startTime) || frameTime.After(startTime)) &&
			(frameTime.Equal(endTime) || frameTime.Before(endTime)) {
			keyFrameTimes = append(keyFrameTimes, frameTime)
		}
	}

	// 如果没有找到关键帧，返回错误
	if len(keyFrameTimes) == 0 {
		return nil, fmt.Errorf("no key frames found in the specified time range")
	}

	return keyFrameTimes, nil
}

func (p *SnapPlugin) RegisterHandler() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"/{streamPath...}":               p.doSnap,
		"/query/{streamPath...}":         p.querySnap,
		"/batch/{streamPath...}":         p.batchSnap,
		"/batchplayback/{streamPath...}": p.batchPlayBack,
	}
}
