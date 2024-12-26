package plugin_snap

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"m7s.live/v5/pkg/task"
)

// SnapTimerTask 定时截图任务结构体
type SnapTimerTask struct {
	task.TickTask
	Interval time.Duration // 截图时间间隔
	SavePath string        // 截图保存路径
	Plugin   *SnapPlugin   // 插件实例引用
}

// GetTickInterval 设置定时间隔
func (t *SnapTimerTask) GetTickInterval() time.Duration {
	return t.Interval // 使用配置的间隔时间
}

// Tick 执行定时截图
func (t *SnapTimerTask) Tick(any) {
	for publisher := range t.Plugin.Server.Streams.Range {
		// 检查流是否匹配过滤器
		if !t.Plugin.filterRegex.MatchString(publisher.StreamPath) {
			continue
		}

		if publisher.HasVideoTrack() {
			streamPath := publisher.StreamPath
			go func() {
				buf, err := t.Plugin.snap(streamPath)
				if err != nil {
					t.Error("take snapshot failed", "error", err.Error())
					return
				}
				now := time.Now()
				filename := fmt.Sprintf("%s_%s.jpg", streamPath, now.Format("20060102150405"))
				filename = strings.ReplaceAll(filename, "/", "_")
				savePath := filepath.Join(t.SavePath, filename)
				// 保存到本地
				err = os.WriteFile(savePath, buf.Bytes(), 0644)
				if err != nil {
					t.Error("take snapshot failed", "error", err.Error())
					return
				}
				t.Info("take snapshot success", "path", savePath)

				// 保存记录到数据库
				if t.Plugin.DB != nil {
					record := SnapRecord{
						StreamName: streamPath,
						SnapMode:   t.Plugin.SnapMode,
						SnapTime:   now,
						SnapPath:   savePath,
					}
					if err := t.Plugin.DB.Create(&record).Error; err != nil {
						t.Error("save snapshot record failed",
							"error", err.Error(),
							"record", record,
						)
					} else {
						t.Info("save snapshot record success",
							"stream", streamPath,
							"path", savePath,
							"time", now,
						)
					}
				} else {
					t.Warn("database not initialized, skip saving record")
				}
			}()
		}
	}
}
