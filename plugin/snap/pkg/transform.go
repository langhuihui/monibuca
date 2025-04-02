package snap

import (
	"bytes"
	"fmt"
	"os"
	"time"

	"m7s.live/v5/pkg"

	m7s "m7s.live/v5"
	"m7s.live/v5/pkg/task"
)

const (
	SnapModeTimeInterval = iota
	SnapModeIFrameInterval
	SnapModeManual
)

// 保存截图到文件
func saveSnapshot(annexb []*pkg.AnnexB, savePath string, plugin *m7s.Plugin, streamPath string, snapMode int) error {
	var buf bytes.Buffer
	if err := ProcessWithFFmpeg(annexb, &buf); err != nil {
		return fmt.Errorf("process with ffmpeg error: %w", err)
	}

	// 如果配置了水印，添加水印
	if GlobalWatermarkConfig.Text != "" {
		imgData, err := AddWatermark(buf.Bytes(), GlobalWatermarkConfig)
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

var _ task.TaskGo = (*Transformer)(nil)

func NewTransform() m7s.ITransformer {
	ret := &Transformer{}
	return ret
}

type Transformer struct {
	task.Job
	TransformJob m7s.TransformJob
}

func (r *Transformer) GetTransformJob() *m7s.TransformJob {
	return &r.TransformJob
}

func (t *Transformer) Start() (err error) {

}

func (t *Transformer) Go() error {
	return nil
}

func (t *Transformer) Dispose() {
}
