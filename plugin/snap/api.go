package plugin_snap

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"m7s.live/v5/pkg"
	snap_pkg "m7s.live/v5/plugin/snap/pkg"
	"m7s.live/v5/plugin/snap/pkg/watermark"
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
func (p *SnapPlugin) snap(streamPath string) (*bytes.Buffer, error) {
	// 获取视频帧
	annexb, _, err := snap_pkg.GetVideoFrame(streamPath, p.Server)
	if err != nil {
		return nil, err
	}

	// 处理视频帧生成图片
	buf := new(bytes.Buffer)
	if err := snap_pkg.ProcessWithFFmpeg(annexb, buf); err != nil {
		return nil, err
	}

	// 如果设置了水印文字，添加水印
	if p.Watermark.Text != "" && snap_pkg.GlobalWatermarkConfig.Font != nil {
		// 解码图片
		img, _, err := image.Decode(bytes.NewReader(buf.Bytes()))
		if err != nil {
			return nil, fmt.Errorf("decode image failed: %w", err)
		}

		// 添加水印
		result, err := watermark.DrawWatermarkSingle(img, watermark.TextConfig{
			Text:       snap_pkg.GlobalWatermarkConfig.Text,
			Font:       snap_pkg.GlobalWatermarkConfig.Font,
			FontSize:   snap_pkg.GlobalWatermarkConfig.FontSize,
			Spacing:    snap_pkg.GlobalWatermarkConfig.FontSpacing,
			RowSpacing: 10,
			ColSpacing: 20,
			Rows:       1,
			Cols:       1,
			DPI:        72,
			Color:      snap_pkg.GlobalWatermarkConfig.FontColor,
			IsGrid:     false,
			Angle:      0,
			OffsetX:    snap_pkg.GlobalWatermarkConfig.OffsetX,
			OffsetY:    snap_pkg.GlobalWatermarkConfig.OffsetY,
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

	if !p.Server.Streams.Has(streamPath) {
		http.Error(rw, pkg.ErrNotFound.Error(), http.StatusNotFound)
		return
	}

	// 调用 snap 进行截图
	buf, err := p.snap(streamPath)
	if err != nil {
		p.Error("snap failed", "error", err.Error())
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	// 处理保存逻辑
	var savePath string
	if p.SavePath != "" && p.IsManualModeSave {
		now := time.Now()
		filename := fmt.Sprintf("%s_%s.jpg", streamPath, now.Format("20060102150405.000"))
		filename = strings.ReplaceAll(filename, "/", "_")
		savePath = filepath.Join(p.SavePath, filename)

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

	// 返回图片
	rw.Header().Set("Content-Type", "image/jpeg")
	rw.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	if _, err := buf.WriteTo(rw); err != nil {
		p.Error("write response failed", "error", err.Error())
	}
}

func (p *SnapPlugin) querySnap(rw http.ResponseWriter, r *http.Request) {
	if p.DB == nil {
		http.Error(rw, "database not initialized", http.StatusInternalServerError)
		return
	}

	streamPath := r.URL.Query().Get("streamPath")
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

func (p *SnapPlugin) RegisterHandler() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"/{streamPath...}": p.doSnap,
		"/query":           p.querySnap,
	}
}
