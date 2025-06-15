package plugin_mp4

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"
	"m7s.live/v5"
	"m7s.live/v5/pkg/task"
	mp4 "m7s.live/v5/plugin/mp4/pkg"
	"m7s.live/v5/plugin/mp4/pkg/box"
)

// RecordRecoveryTask 从录像文件中恢复数据库记录的任务
type RecordRecoveryTask struct {
	task.Task
	DB     *gorm.DB
	plugin *MP4Plugin
}

// RecoveryStats 恢复统计信息
type RecoveryStats struct {
	TotalFiles   int
	SuccessCount int
	FailureCount int
	SkippedCount int
	Errors       []error
}

// Start 从文件系统中恢复录像记录
func (t *RecordRecoveryTask) Start() error {
	// 获取所有录像目录
	var recordDirs []string
	if len(t.plugin.GetCommonConf().OnPub.Record) > 0 {
		for _, conf := range t.plugin.GetCommonConf().OnPub.Record {
			dirPath := filepath.Dir(conf.FilePath)
			recordDirs = append(recordDirs, dirPath)
		}
	}
	if t.plugin.EventRecordFilePath != "" {
		dirPath := filepath.Dir(t.plugin.EventRecordFilePath)
		recordDirs = append(recordDirs, dirPath)
	}

	if len(recordDirs) == 0 {
		t.Info("No record directories configured, skipping recovery")
		return nil
	}

	stats := &RecoveryStats{}

	// 遍历所有录像目录，收集所有错误而不是在第一个错误时停止
	for _, dir := range recordDirs {
		dirStats, err := t.scanDirectory(dir)
		if dirStats != nil {
			stats.TotalFiles += dirStats.TotalFiles
			stats.SuccessCount += dirStats.SuccessCount
			stats.FailureCount += dirStats.FailureCount
			stats.SkippedCount += dirStats.SkippedCount
			stats.Errors = append(stats.Errors, dirStats.Errors...)
		}

		if err != nil {
			stats.Errors = append(stats.Errors, fmt.Errorf("failed to scan directory %s: %w", dir, err))
		}
	}

	// 记录统计信息
	t.Info("Recovery completed",
		"totalFiles", stats.TotalFiles,
		"success", stats.SuccessCount,
		"failed", stats.FailureCount,
		"skipped", stats.SkippedCount,
		"errors", len(stats.Errors))

	// 如果有错误，返回一个汇总错误
	if len(stats.Errors) > 0 {
		var errorMsgs []string
		for _, err := range stats.Errors {
			errorMsgs = append(errorMsgs, err.Error())
		}
		return fmt.Errorf("recovery completed with %d errors: %s", len(stats.Errors), strings.Join(errorMsgs, "; "))
	}

	return nil
}

// scanDirectory 扫描目录中的MP4文件
func (t *RecordRecoveryTask) scanDirectory(dir string) (*RecoveryStats, error) {
	t.Info("Scanning directory for MP4 files", "directory", dir)

	stats := &RecoveryStats{}

	// 递归遍历目录
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			t.Error("Error accessing path", "path", path, "error", err)
			stats.Errors = append(stats.Errors, fmt.Errorf("failed to access path %s: %w", path, err))
			return nil // 继续遍历
		}

		// 跳过目录
		if info.IsDir() {
			return nil
		}

		// 只处理MP4文件
		if !strings.HasSuffix(strings.ToLower(path), ".mp4") {
			return nil
		}

		stats.TotalFiles++

		// 检查文件是否已经有记录
		var count int64
		if err := t.DB.Model(&m7s.RecordStream{}).Where("file_path = ?", path).Count(&count).Error; err != nil {
			t.Error("Failed to check existing record", "file", path, "error", err)
			stats.FailureCount++
			stats.Errors = append(stats.Errors, fmt.Errorf("failed to check existing record for %s: %w", path, err))
			return nil
		}

		if count > 0 {
			// 已有记录，跳过
			stats.SkippedCount++
			return nil
		}

		// 解析MP4文件并创建记录
		if err := t.recoverRecordFromFile(path); err != nil {
			stats.FailureCount++
			stats.Errors = append(stats.Errors, fmt.Errorf("failed to recover record from %s: %w", path, err))
		} else {
			stats.SuccessCount++
		}
		return nil
	})

	if err != nil {
		t.Error("Error walking directory", "directory", dir, "error", err)
		return stats, fmt.Errorf("failed to walk directory %s: %w", dir, err)
	}

	return stats, nil
}

// recoverRecordFromFile 从MP4文件中恢复记录
func (t *RecordRecoveryTask) recoverRecordFromFile(filePath string) error {
	t.Info("Recovering record from file", "file", filePath)

	// 打开文件
	file, err := os.Open(filePath)
	if err != nil {
		t.Error("Failed to open MP4 file", "file", filePath, "error", err)
		return fmt.Errorf("failed to open MP4 file %s: %w", filePath, err)
	}
	defer file.Close()

	// 创建解析器
	demuxer := mp4.NewDemuxer(file)
	err = demuxer.Demux()
	if err != nil {
		t.Error("Failed to demux MP4 file", "file", filePath, "error", err)
		return fmt.Errorf("failed to demux MP4 file %s: %w", filePath, err)
	}

	// 提取文件信息
	fileInfo, err := file.Stat()
	if err != nil {
		t.Error("Failed to get file info", "file", filePath, "error", err)
		return fmt.Errorf("failed to get file info for %s: %w", filePath, err)
	}

	// 尝试从MP4文件中提取流路径，如果没有则从文件名和路径推断
	streamPath := extractStreamPathFromMP4(demuxer)
	if streamPath == "" {
		streamPath = inferStreamPathFromFilePath(filePath)
	}

	// 创建记录
	record := m7s.RecordStream{
		FilePath:   filePath,
		StreamPath: streamPath,
		Type:       "mp4",
	}

	// 设置开始和结束时间
	record.StartTime = fileInfo.ModTime().Add(-estimateDurationFromFile(demuxer))
	record.EndTime = fileInfo.ModTime()

	// 提取编解码器信息
	for _, track := range demuxer.Tracks {
		forcc := box.GetCodecNameWithCodecId(track.Cid)
		if track.Cid.IsAudio() {
			record.AudioCodec = string(forcc[:])
		} else if track.Cid.IsVideo() {
			record.VideoCodec = string(forcc[:])
		}
	}

	// 保存记录到数据库
	err = t.DB.Create(&record).Error
	if err != nil {
		t.Error("Failed to save record to database", "file", filePath, "error", err)
		return fmt.Errorf("failed to save record to database for %s: %w", filePath, err)
	}

	t.Info("Successfully recovered record", "file", filePath, "streamPath", streamPath)
	return nil
}

// extractStreamPathFromMP4 从MP4文件中提取流路径
func extractStreamPathFromMP4(demuxer *mp4.Demuxer) string {
	// 尝试从MP4文件的用户数据中提取流路径
	moov := demuxer.GetMoovBox()
	if moov != nil && moov.UDTA != nil {
		for _, entry := range moov.UDTA.Entries {
			if entry.Type() == box.TypeALB {
				return entry.(*box.TextDataBox).Text
			}
		}
	}
	return ""
}

// inferStreamPathFromFilePath 从文件路径推断流路径
func inferStreamPathFromFilePath(filePath string) string {
	// 从文件路径中提取可能的流路径
	// 这里使用简单的启发式方法，实际应用中可能需要更复杂的逻辑
	base := filepath.Base(filePath)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)

	// 如果文件名是时间戳，尝试从父目录获取流名称
	if _, err := time.Parse("20060102150405", name); err == nil || isNumeric(name) {
		dir := filepath.Base(filepath.Dir(filePath))
		if dir != "" && dir != "." {
			return dir
		}
	}

	return name
}

// isNumeric 检查字符串是否为数字
func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

// estimateDurationFromFile 估计文件的持续时间
func estimateDurationFromFile(demuxer *mp4.Demuxer) time.Duration {
	var maxDuration uint32

	for _, track := range demuxer.Tracks {
		if len(track.Samplelist) > 0 {
			lastSample := track.Samplelist[len(track.Samplelist)-1]
			durationMs := lastSample.Timestamp * 1000 / uint32(track.Timescale)
			if durationMs > maxDuration {
				maxDuration = durationMs
			}
		}
	}

	return time.Duration(maxDuration) * time.Millisecond
}
