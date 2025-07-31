package plugin_snap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	m7s "m7s.live/v5"
	"m7s.live/v5/pkg"
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
	if err := snap_pkg.ProcessWithFFmpeg(annexb, buf); err != nil {
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
	now := time.Now()
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

	targetTime := time.Unix(snapTimeUnix+1, 0)
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
	imgData, err := os.ReadFile(record.SnapPath)
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
			videoTrack.RLock()
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
			videoTrack.RUnlock()
			
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
				SnapTime:   snapTime,
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

func (p *SnapPlugin) RegisterHandler() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"/{streamPath...}":       p.doSnap,
		"/query/{streamPath...}": p.querySnap,
		"/batch/{streamPath...}": p.batchSnap,
	}
}


