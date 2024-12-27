package plugin_snap

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"image/color"
	snap_pkg "m7s.live/v5/plugin/snap/pkg"

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
	SnapTimeInterval   time.Duration `default:"1m" desc:"截图间隔"`
	SnapSavePath       string        `default:"snaps" desc:"截图保存路径"`
	Filter             string        `default:".*" desc:"截图流过滤器，支持正则表达式"`
	SnapIFrameInterval int           `default:"3" desc:"间隔多少帧截图"`
	SnapMode           int           `default:"1" desc:"截图模式 0:间隔时间 1:间隔关键帧"`
	SnapQueryTimeDelta int           `default:"3" desc:"查询截图时允许的最大时间差（秒）"`
	SnapSaveManual     bool          `default:"false" desc:"手动截图是否保存文件"`
	filterRegex        *regexp.Regexp
}

// OnInit 在插件初始化时添加定时任务
func (p *SnapPlugin) OnInit() (err error) {
	// 检查 SnapMode 的值范围
	if p.SnapMode < 0 || p.SnapMode > 1 {
		p.Error("invalid snap mode",
			"mode", p.SnapMode,
			"valid_range", "0-1",
		)
		return fmt.Errorf("invalid snap mode: %d, valid range is 0-1", p.SnapMode)
	}

	// 初始化数据库
	if p.DB != nil {
		err = p.DB.AutoMigrate(&snap_pkg.SnapRecord{})
		if err != nil {
			p.Error("failed to migrate database", "error", err.Error())
			return
		}
	}

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

	//如果截图模式不是时间模式，则不加定时任务
	if p.SnapMode != 0 {
		return
	}

	// 如果间隔时间小于0，则不添加定时任务;等于0则走onpub的transform
	if p.SnapTimeInterval <= 0 {
		return
	}
	// 添加定时任务
	p.AddTask(&SnapTimerTask{
		Interval: p.SnapTimeInterval,
		SavePath: p.SnapSavePath,
		Plugin:   p,
	})

	return
}
