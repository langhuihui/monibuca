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
	"github.com/golang/freetype/truetype"
	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/config"
	snap "m7s.live/v5/plugin/snap/pkg"
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
	buf := new(bytes.Buffer)
	transformer := snap.NewTransform().(*snap.Transformer)
	transformer.TransformJob.Init(transformer, &p.Plugin, streamPath, config.Transform{
		Output: []config.TransfromOutput{
			{
				Target:     streamPath,
				StreamPath: streamPath,
				Conf:       buf,
			},
		},
	}).WaitStarted()

	if err := transformer.Run(); err != nil {
		return nil, err
	}

	// 如果设置了水印文字，添加水印
	if p.SnapWatermark.Text != "" {
		// 读取字体文件
		fontBytes, err := os.ReadFile(p.SnapWatermark.FontPath)
		if err != nil {
			p.Error("read font file failed", "error", err.Error())
			return nil, err
		}

		// 解析字体
		font, err := truetype.Parse(fontBytes)
		if err != nil {
			p.Error("parse font failed", "error", err.Error())
			return nil, err
		}

		// 解码图片
		img, _, err := image.Decode(bytes.NewReader(buf.Bytes()))
		if err != nil {
			p.Error("decode image failed", "error", err.Error())
			return nil, err
		}

		// 解码颜色
		rgba, err := parseRGBA(p.SnapWatermark.FontColor)
		if err != nil {
			p.Error("parse color failed", "error", err.Error())
			return nil, err
		}
		// 确保alpha通道正确
		if rgba.A == 0 {
			rgba.A = 255 // 如果完全透明，改为不透明
		}

		// 添加水印
		result, err := watermark.DrawWatermarkSingle(img, watermark.TextConfig{
			Text:       p.SnapWatermark.Text,
			Font:       font,
			FontSize:   p.SnapWatermark.FontSize,
			Spacing:    10,
			RowSpacing: 10,
			ColSpacing: 20,
			Rows:       1,
			Cols:       1,
			DPI:        72,
			Color:      rgba,
			IsGrid:     false,
			Angle:      0,
			OffsetX:    p.SnapWatermark.OffsetX,
			OffsetY:    p.SnapWatermark.OffsetY,
		}, false)
		if err != nil {
			p.Error("add watermark failed", "error", err.Error())
			return nil, err
		}

		// 清空原buffer并写入新图片
		buf.Reset()
		if err := imaging.Encode(buf, result, imaging.JPEG); err != nil {
			p.Error("encode image failed", "error", err.Error())
			return nil, err
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

	buf, err := p.snap(streamPath)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	// 保存截图并记录到数据库
	if p.DB != nil {
		now := time.Now()
		filename := fmt.Sprintf("%s_%s.jpg", streamPath, now.Format("20060102150405"))
		filename = strings.ReplaceAll(filename, "/", "_")
		savePath := filepath.Join(p.SnapSavePath, filename)

		// 保存到本地
		err = os.WriteFile(savePath, buf.Bytes(), 0644)
		if err != nil {
			p.Error("save snapshot failed", "error", err.Error())
		} else {
			// 保存记录到数据库
			record := SnapRecord{
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

	rw.Header().Set("Content-Type", "image/jpeg")
	rw.Header().Set("Content-Length", strconv.Itoa(buf.Len()))

	if _, err := buf.WriteTo(rw); err != nil {
		p.Error("write response failed", "error", err.Error())
		return
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

	targetTime := time.Unix(snapTimeUnix, 0)
	var record SnapRecord

	// 查询小于等于目标时间的最近一条记录
	if err := p.DB.Where("stream_name = ? AND snap_time <= ?", streamPath, targetTime).
		Order("snap_time DESC").
		First(&record).Error; err != nil {
		http.Error(rw, "snapshot not found", http.StatusNotFound)
		return
	}

	// 计算时间差（秒）
	timeDiff := targetTime.Sub(record.SnapTime).Seconds()
	if timeDiff > float64(p.SnapQueryTimeDelta) {
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
