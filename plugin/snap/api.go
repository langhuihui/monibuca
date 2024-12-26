package plugin_snap

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"net/http"
	"os"
	"strconv"
	"strings"

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
func (t *SnapPlugin) snap(streamPath string) (*bytes.Buffer, error) {
	buf := new(bytes.Buffer)
	transformer := snap.NewTransform().(*snap.Transformer)
	transformer.TransformJob.Init(transformer, &t.Plugin, streamPath, config.Transform{
		Output: []config.TransfromOutput{
			{
				Target:     streamPath,
				StreamPath: streamPath,
				Conf:       buf,
			},
		},
	}).WaitStarted()

	transformer.TriggerSnap()
	if err := transformer.Run(); err != nil {
		return nil, err
	}

	// 如果设置了水印文字，添加水印
	if t.SnapWatermark.Text != "" {
		// 读取字体文件
		fontBytes, err := os.ReadFile(t.SnapWatermark.FontPath)
		if err != nil {
			t.Error("read font file error", err)
			return nil, err
		}

		// 解析字体
		font, err := truetype.Parse(fontBytes)
		if err != nil {
			t.Error("parse font error", err)
			return nil, err
		}

		// 解码图片
		img, _, err := image.Decode(bytes.NewReader(buf.Bytes()))
		if err != nil {
			t.Error("decode image error", err)
			return nil, err
		}

		// 解码颜色
		rgba, err := parseRGBA(t.SnapWatermark.FontColor)
		if err != nil {
			t.Error("parse color error", err)
			return nil, err
		}
		// 确保alpha通道正确
		if rgba.A == 0 {
			rgba.A = 255 // 如果完全透明，改为不透明
		}

		// 添加水印
		result, err := watermark.DrawWatermarkSingle(img, watermark.TextConfig{
			Text:       t.SnapWatermark.Text,
			Font:       font,
			FontSize:   t.SnapWatermark.FontSize,
			Spacing:    10,
			RowSpacing: 10,
			ColSpacing: 20,
			Rows:       1,
			Cols:       1,
			DPI:        72,
			Color:      rgba,
			IsGrid:     false,
			Angle:      0,
			OffsetX:    t.SnapWatermark.OffsetX,
			OffsetY:    t.SnapWatermark.OffsetY,
		}, false)
		if err != nil {
			t.Error("add watermark error", err)
			return nil, err
		}

		// 清空原buffer并写入新图片
		buf.Reset()
		if err := imaging.Encode(buf, result, imaging.JPEG); err != nil {
			t.Error("encode image error", err)
			return nil, err
		}
	}

	return buf, nil
}

func (t *SnapPlugin) doSnap(rw http.ResponseWriter, r *http.Request) {
	streamPath := r.PathValue("streamPath")

	if !t.Server.Streams.Has(streamPath) {
		http.Error(rw, pkg.ErrNotFound.Error(), http.StatusNotFound)
		return
	}

	buf, err := t.snap(streamPath)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	rw.Header().Set("Content-Type", "image/jpeg")
	rw.Header().Set("Content-Length", strconv.Itoa(buf.Len()))

	if _, err := buf.WriteTo(rw); err != nil {
		t.Error("write response error", err.Error())
		return
	}
}

func (config *SnapPlugin) RegisterHandler() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"/{streamPath...}": config.doSnap,
	}
}
