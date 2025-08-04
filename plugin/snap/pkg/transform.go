package snap

import (
	"bytes"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/format"

	m7s "m7s.live/v5"
	"m7s.live/v5/pkg/task"
)

const (
	SnapModeTimeInterval = iota
	SnapModeIFrameInterval
	SnapModeManual
)

// parseRGBA 解析rgba格式的颜色字符串
func parseRGBA(rgbaStr string) (color.RGBA, error) {
	rgba := strings.TrimPrefix(rgbaStr, "rgba(")
	rgba = strings.TrimSuffix(rgba, ")")
	parts := strings.Split(rgba, ",")
	if len(parts) != 4 {
		return color.RGBA{}, fmt.Errorf("invalid rgba format")
	}

	r, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return color.RGBA{}, err
	}

	g, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return color.RGBA{}, err
	}

	b, err := strconv.Atoi(strings.TrimSpace(parts[2]))
	if err != nil {
		return color.RGBA{}, err
	}

	a, err := strconv.ParseFloat(strings.TrimSpace(parts[3]), 64)
	if err != nil {
		return color.RGBA{}, err
	}

	return color.RGBA{
		R: uint8(r),
		G: uint8(g),
		B: uint8(b),
		A: uint8(a * 255),
	}, nil
}

// 保存截图到文件
func saveSnapshot(annexb []*format.AnnexB, savePath string, plugin *m7s.Plugin, streamPath string, snapMode int, watermarkConfig *WatermarkConfig) error {
	var buf bytes.Buffer
	if err := ProcessWithFFmpeg(annexb, &buf); err != nil {
		return fmt.Errorf("process with ffmpeg error: %w", err)
	}

	// 如果配置了水印，添加水印
	if watermarkConfig != nil && watermarkConfig.Text != "" {
		imgData, err := AddWatermark(buf.Bytes(), *watermarkConfig)
		if err != nil {
			return fmt.Errorf("add watermark error: %w", err)
		}
		err = os.WriteFile(savePath, imgData, 0644)
		if err != nil {
			return err
		}
	} else {
		err := os.WriteFile(savePath, buf.Bytes(), 0644)
		if err != nil {
			return err
		}
	}

	// 保存记录到数据库
	if plugin != nil && plugin.DB != nil {
		record := SnapRecord{
			StreamName: streamPath,
			SnapMode:   snapMode,
			SnapTime:   time.Now(),
			SnapPath:   savePath,
		}
		if err := plugin.DB.Create(&record).Error; err != nil {
			return fmt.Errorf("save snapshot record failed: %w", err)
		}
	}

	return nil
}

// SnapConfig 截图配置
type SnapConfig struct {
	TimeInterval   time.Duration `json:"timeInterval" desc:"截图时间间隔，大于0时使用时间间隔模式"`
	IFrameInterval int           `json:"iFrameInterval" desc:"间隔多少帧截图，大于0时使用关键帧间隔模式"`
	SavePath       string        `json:"savePath" desc:"截图保存路径"`
	Watermark      struct {
		Text        string  `json:"text" default:"" desc:"水印文字内容"`
		FontPath    string  `json:"fontPath" default:"" desc:"水印字体文件路径"`
		FontColor   string  `json:"fontColor" default:"rgba(255,165,0,1)" desc:"水印字体颜色，支持rgba格式"`
		FontSize    float64 `json:"fontSize" default:"36" desc:"水印字体大小"`
		FontSpacing float64 `json:"fontSpacing" default:"2" desc:"水印字体间距"`
		OffsetX     int     `json:"offsetX" default:"0" desc:"水印位置X"`
		OffsetY     int     `json:"offsetY" default:"0" desc:"水印位置Y"`
	} `json:"watermark" desc:"水印配置"`
}

// SnapTask 基础截图任务结构
type SnapTask struct {
	config          SnapConfig
	job             *m7s.TransformJob
	watermarkConfig *WatermarkConfig
}

// saveSnap 保存截图
func (t *SnapTask) saveSnap(annexb []*format.AnnexB, snapMode int) error {
	// 生成文件名
	now := time.Now()
	filename := fmt.Sprintf("%s_%s.jpg", t.job.StreamPath, now.Format("20060102150405.000"))
	filename = strings.ReplaceAll(filename, "/", "_")
	savePath := filepath.Join(t.config.SavePath, filename)

	// 处理视频帧
	var buf bytes.Buffer
	if err := ProcessWithFFmpeg(annexb, &buf); err != nil {
		return fmt.Errorf("process with ffmpeg error: %w", err)
	}

	// 如果配置了水印，添加水印
	if t.watermarkConfig != nil && t.watermarkConfig.Text != "" {
		imgData, err := AddWatermark(buf.Bytes(), *t.watermarkConfig)
		if err != nil {
			return fmt.Errorf("add watermark error: %w", err)
		}
		err = os.WriteFile(savePath, imgData, 0644)
		if err != nil {
			return err
		}
	} else {
		err := os.WriteFile(savePath, buf.Bytes(), 0644)
		if err != nil {
			return err
		}
	}

	// 保存记录到数据库
	if t.job.Plugin != nil && t.job.Plugin.DB != nil {
		record := SnapRecord{
			StreamName: t.job.StreamPath,
			SnapMode:   snapMode,
			SnapTime:   time.Now(),
			SnapPath:   savePath,
		}
		if err := t.job.Plugin.DB.Create(&record).Error; err != nil {
			return fmt.Errorf("save snapshot record failed: %w", err)
		}
	}

	return nil
}

// TimeSnapTask 定时截图任务
type TimeSnapTask struct {
	task.TickTask
	SnapTask
}

func (t *TimeSnapTask) GetTickInterval() time.Duration {
	return t.config.TimeInterval
}

// Tick 执行定时截图操作
func (t *TimeSnapTask) Tick(any) {
	// 获取视频帧
	annexb, err := GetVideoFrame(t.job.OriginPublisher, t.job.Plugin.Server)
	if err != nil {
		t.Error("get video frame failed", "error", err.Error())
		return
	}

	if err := t.saveSnap(annexb, SnapModeTimeInterval); err != nil {
		t.Error("save snapshot failed", "error", err.Error())
	}
}

// IFrameSnapTask 关键帧截图任务
type IFrameSnapTask struct {
	task.Task
	SnapTask
	subscriber *m7s.Subscriber
}

func (t *IFrameSnapTask) Start() (err error) {
	subConfig := t.job.Plugin.GetCommonConf().Subscribe
	subConfig.SubType = m7s.SubscribeTypeTransform
	subConfig.IFrameOnly = true
	t.subscriber, err = t.job.Plugin.SubscribeWithConfig(t, t.job.StreamPath, subConfig)
	return
}

func (t *IFrameSnapTask) Go() (err error) {
	iframeCount := 0
	err = m7s.PlayBlock(t.subscriber, (func(audio *pkg.AVFrame) error)(nil), func(video *format.AnnexB) error {
		iframeCount++
		if iframeCount%t.config.IFrameInterval == 0 {
			if err := t.saveSnap([]*format.AnnexB{video}, SnapModeIFrameInterval); err != nil {
				t.Error("save snapshot failed", "error", err.Error())
			}
		}
		return nil
	})
	if err != nil {
		t.Error("iframe interval snap error", "error", err.Error())
	}
	return
}

type Transformer struct {
	task.Job
	TransformJob m7s.TransformJob
}

func (r *Transformer) GetTransformJob() *m7s.TransformJob {
	return &r.TransformJob
}

func NewTransform() m7s.ITransformer {
	return &Transformer{}
}

func (t *Transformer) Start() (err error) {
	// 为每个输出配置创建一个截图任务
	for _, output := range t.TransformJob.Config.Output {
		var task task.ITask
		var snapConfig SnapConfig
		if output.Conf != nil {
			switch v := output.Conf.(type) {
			case SnapConfig:
				snapConfig = v
			case map[string]any:
				var conf config.Config
				conf.Parse(&snapConfig)
				conf.ParseModifyFile(v)
			}
		}

		// 初始化水印配置
		var watermarkConfig *WatermarkConfig
		if snapConfig.Watermark.Text != "" {
			watermarkConfig = &WatermarkConfig{
				Text:        snapConfig.Watermark.Text,
				FontPath:    snapConfig.Watermark.FontPath,
				FontSize:    snapConfig.Watermark.FontSize,
				FontSpacing: snapConfig.Watermark.FontSpacing,
				OffsetX:     snapConfig.Watermark.OffsetX,
				OffsetY:     snapConfig.Watermark.OffsetY,
			}

			// 判断字体是否存在
			if _, err := os.Stat(watermarkConfig.FontPath); os.IsNotExist(err) {
				return fmt.Errorf("watermark font file not found: %w", err)
			}
			// 解析颜色
			if snapConfig.Watermark.FontColor != "" {
				fontColor, err := parseRGBA(snapConfig.Watermark.FontColor)
				if err == nil {
					watermarkConfig.FontColor = fontColor
				} else {
					t.Error("parse color failed", "error", err.Error())
					watermarkConfig.FontColor = color.RGBA{uint8(255), uint8(255), uint8(255), uint8(255)}
				}
			}

			// 预加载字体
			if err := watermarkConfig.LoadFont(); err != nil {
				return fmt.Errorf("load watermark font failed: %w", err)
			}
			t.Info("watermark config loaded",
				"text", watermarkConfig.Text,
				"font", watermarkConfig.FontPath,
				"size", watermarkConfig.FontSize,
			)
		}
		// 创建保存目录
		if err := os.MkdirAll(snapConfig.SavePath, 0755); err != nil {
			return fmt.Errorf("create save directory failed: %w", err)
		}
		// 根据配置创建对应的任务
		if snapConfig.TimeInterval > 0 {
			timeTask := &TimeSnapTask{
				SnapTask: SnapTask{
					config:          snapConfig,
					job:             &t.TransformJob,
					watermarkConfig: watermarkConfig,
				},
			}
			task = timeTask
		} else if snapConfig.IFrameInterval > 0 {
			iframeTask := &IFrameSnapTask{
				SnapTask: SnapTask{
					config:          snapConfig,
					job:             &t.TransformJob,
					watermarkConfig: watermarkConfig,
				},
			}
			task = iframeTask
		}

		if task != nil {
			t.AddTask(task)
		}
	}
	return nil
}
