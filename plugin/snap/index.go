package plugin_snap

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"image/color"

	m7s "m7s.live/v5"
	snap "m7s.live/v5/plugin/snap/pkg"
)

var _ = m7s.InstallPlugin[SnapPlugin](snap.NewTransform)

type SnapPlugin struct {
	m7s.Plugin
	SnapWatermark struct {
		Text      string  `default:"" desc:"水印文字内容"`
		FontPath  string  `default:"" desc:"水印字体文件路径"`
		FontColor string  `default:"rgba(255,165,0,1)" desc:"水印字体颜色，支持rgba格式"`
		FontSize  float64 `default:"36" desc:"水印字体大小"`
		OffsetX   int     `default:"0" desc:"水印位置X"`
		OffsetY   int     `default:"0" desc:"水印位置Y"`
	} `desc:"水印配置"`
	// 定时任务相关配置
	SnapInterval time.Duration `default:"1m" desc:"截图间隔"`
	SnapSavePath string        `default:"snaps" desc:"截图保存路径"`
	Filter       string        `default:".*" desc:"截图流过滤器，支持正则表达式"`
	filterRegex  *regexp.Regexp
}

// OnInit 在插件初始化时添加定时任务
func (p *SnapPlugin) OnInit() (err error) {
	// 创建保存目录
	if err = os.MkdirAll(p.SnapSavePath, 0755); err != nil {
		return
	}

	// 编译正则表达式
	if p.filterRegex, err = regexp.Compile(p.Filter); err != nil {
		p.Error("invalid filter regex", "error", err.Error())
		return
	}

	// 初始化全局水印配置
	snap.GlobalWatermarkConfig = snap.WatermarkConfig{
		Text:      p.SnapWatermark.Text,
		FontPath:  p.SnapWatermark.FontPath,
		FontSize:  p.SnapWatermark.FontSize,
		FontColor: color.RGBA{}, // 将在下面解析
		OffsetX:   p.SnapWatermark.OffsetX,
		OffsetY:   p.SnapWatermark.OffsetY,
	}

	if p.SnapWatermark.Text != "" {
		// 判断字体是否存在
		if _, err := os.Stat(p.SnapWatermark.FontPath); os.IsNotExist(err) {
			p.Error("watermark font file not found", "path", p.SnapWatermark.FontPath)
			return fmt.Errorf("watermark font file not found: %w", err)
		}
		// 解析颜色
		if p.SnapWatermark.FontColor != "" {
			rgba := p.SnapWatermark.FontColor
			rgba = strings.TrimPrefix(rgba, "rgba(")
			rgba = strings.TrimSuffix(rgba, ")")
			parts := strings.Split(rgba, ",")
			if len(parts) == 4 {
				r, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
				g, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
				b, _ := strconv.Atoi(strings.TrimSpace(parts[2]))
				a, _ := strconv.ParseFloat(strings.TrimSpace(parts[3]), 64)
				snap.GlobalWatermarkConfig.FontColor = color.RGBA{uint8(r), uint8(g), uint8(b), uint8(a * 255)}
			}
		}
	}

	// 预加载字体
	if snap.GlobalWatermarkConfig.Text != "" && snap.GlobalWatermarkConfig.FontPath != "" {
		if err := snap.GlobalWatermarkConfig.LoadFont(); err != nil {
			p.Error("load watermark font failed",
				"error", err.Error(),
				"path", snap.GlobalWatermarkConfig.FontPath,
			)
			return fmt.Errorf("load watermark font failed: %w", err)
		}
		p.Info("watermark config loaded",
			"text", snap.GlobalWatermarkConfig.Text,
			"font", snap.GlobalWatermarkConfig.FontPath,
			"size", snap.GlobalWatermarkConfig.FontSize,
		)
	}

	// 如果间隔时间为0，则不添加定时任务，走onpub的transform
	if p.SnapInterval == 0 {
		return
	}
	// 添加定时任务
	p.AddTask(&SnapTimerTask{
		Interval: p.SnapInterval,
		SavePath: p.SnapSavePath,
		Plugin:   p,
	})

	return
}
