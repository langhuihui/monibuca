package plugin_mp4

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	task "github.com/langhuihui/gotask"
	"github.com/shirou/gopsutil/v4/disk"
	"gorm.io/gorm"
	"m7s.live/v5"
)

// mysql数据库里Exception 定义异常结构体
type Exception struct {
	CreateTime string `json:"createTime" gorm:"type:varchar(50)"`
	AlarmType  string `json:"alarmType" gorm:"type:varchar(50)"`
	AlarmDesc  string `json:"alarmDesc" gorm:"type:varchar(50)"`
	ServerIP   string `json:"serverIP" gorm:"type:varchar(50)"`
	StreamPath string `json:"streamPath" gorm:"type:varchar(50)"`
}

// // 向第三方发送异常报警
// func (p *MP4Plugin) SendToThirdPartyAPI(exception *Exception) {
// 	exception.CreateTime = time.Now().Format("2006-01-02 15:04:05")
// 	exception.ServerIP = p.GetCommonConf().PublicIP
// 	data, err := json.Marshal(exception)
// 	if err != nil {
// 		p.Error("SendToThirdPartyAPI", " marshalling exception error", err.Error())
// 		return
// 	}
// 	err = p.DB.Create(&exception).Error
// 	if err != nil {
// 		p.Error("SendToThirdPartyAPI", "insert into db error", err.Error())
// 		return
// 	}
// 	resp, err := http.Post(p.ExceptionPostUrl, "application/json", bytes.NewBuffer(data))
// 	if err != nil {
// 		p.Error("SendToThirdPartyAPI", "Error sending exception to third party API error", err.Error())
// 		return
// 	}
// 	defer resp.Body.Close()

// 	if resp.StatusCode != http.StatusOK {
// 		p.Error("SendToThirdPartyAPI", "Failed to send exception, status code:", resp.StatusCode)
// 	} else {
// 		p.Info("SendToThirdPartyAPI", "Exception sent successfully!")
// 	}
// }

// // 磁盘超上限报警
// func (p *DeleteRecordTask) getDisckException(streamPath string) bool {
// 	if p.getDiskOutOfSpace(p.DiskMaxPercent) {
// 		exceptionChannel <- &Exception{AlarmType: "disk alarm", AlarmDesc: "disk is full", StreamPath: streamPath}
// 		return true
// 	}
// 	return false
// }

// 判断磁盘使用量是否中超限
func (p *DeleteRecordTask) getDiskOutOfSpace(filePath string) bool {
	exePath := filepath.Dir(filePath)
	d, err := disk.Usage(exePath)
	if err != nil || d == nil {
		p.Error("getDiskOutOfSpace", "error", err)
		return false
	}
	p.plugin.Debug("getDiskOutOfSpace", "current path", exePath, "disk UsedPercent", d.UsedPercent, "total disk space", d.Total,
		"disk free", d.Free, "disk usage", d.Used, "AutoOverWriteDiskPercent", p.AutoOverWriteDiskPercent, "DiskMaxPercent", p.DiskMaxPercent)
	return d.UsedPercent >= p.AutoOverWriteDiskPercent
}

func (p *DeleteRecordTask) deleteOldestFile() {
	//当当前磁盘使用量大于AutoOverWriteDiskPercent自动覆盖磁盘使用量配置时，自动删除最旧的文件
	//连续录像删除最旧的文件
	// 使用 map 去重，存储所有的 conf.FilePath
	pathMap := make(map[string]bool)
	if p.plugin.Server.Plugins.Length > 0 {
		p.plugin.Server.Plugins.Range(func(plugin *m7s.Plugin) bool {
			if len(plugin.GetCommonConf().OnPub.Record) > 0 {
				for _, conf := range plugin.GetCommonConf().OnPub.Record {
					// 处理路径，去掉最后的/$0部分，只保留目录部分
					dirPath := filepath.Dir(conf.FilePath)
					if _, exists := pathMap[dirPath]; !exists {
						pathMap[dirPath] = true
						p.Info("deleteOldestFile", "original filepath", conf.FilePath, "processed filepath", dirPath)
					} else {
						p.Debug("deleteOldestFile", "duplicate path ignored", "path", dirPath)
					}
				}
			}
			return true
		})
	}

	// 将 map 转换为 slice
	var filePaths []string
	for path := range pathMap {
		filePaths = append(filePaths, path)
	}
	p.Debug("deleteOldestFile", "after get onpub.record,filePaths.length", len(filePaths))
	if p.plugin.EventRecordFilePath != "" {
		// 同样处理EventRecordFilePath
		dirPath := filepath.Dir(p.plugin.EventRecordFilePath)
		filePaths = append(filePaths, dirPath)
	}
	p.Debug("deleteOldestFile", "after get eventrecordfilepath,filePaths.length", len(filePaths))
	for _, filePath := range filePaths {
		for p.getDiskOutOfSpace(filePath) {
			var recordStreams []m7s.RecordStream
			// 使用不同的方法进行路径匹配，避免ESCAPE语法问题
			// 解决方案：用MySQL能理解的简单方式匹配路径前缀
			basePath := filePath
			// 直接替换所有反斜杠，不需要判断是否包含
			basePath = strings.Replace(basePath, "\\", "\\\\", -1)
			searchPattern := basePath + "%"
			p.Info("deleteOldestFile", "searching with path pattern", searchPattern)

			err := p.DB.Where(" record_level!='high' AND end_time IS NOT NULL").
				Where("file_path LIKE ?", searchPattern).
				Order("end_time ASC").Limit(1).Find(&recordStreams).Error
			if err == nil {
				if len(recordStreams) > 0 {
					p.Info("deleteOldestFile", "found %d records", len(recordStreams))
					for _, record := range recordStreams {
						p.Info("deleteOldestFile", "ready to delete oldestfile,ID", record.ID, "create time", record.EndTime, "filepath", record.FilePath)
						err = os.Remove(record.FilePath)
						if err != nil {
							// 检查是否为文件不存在的错误
							if os.IsNotExist(err) {
								// 文件不存在，记录日志但视为删除成功
								p.Warn("deleteOldestFile", "file does not exist, continuing with database deletion", record.FilePath)
								// 继续删除数据库记录
								err = p.DB.Delete(&record).Error
								if err != nil {
									p.Error("deleteOldestFile", "delete record from db error", err)
								}
							} else {
								// 其他错误，记录并跳过此记录
								p.Error("deleteOldestFile", "delete file from disk error", err)
								continue
							}
						} else {
							// 文件删除成功，继续删除数据库记录
							err = p.DB.Delete(&record).Error
							if err != nil {
								p.Error("deleteOldestFile", "delete record from db error", err)
							}
						}
					}
				}
			} else {
				p.Error("deleteOldestFile", "search record from db error", err)
			}
			time.Sleep(time.Second * 3)
		}
	}
}

type StorageManagementTask struct {
	task.TickTask
	DiskMaxPercent            float64
	AutoOverWriteDiskPercent  float64
	MigrationThresholdPercent float64
	RecordFileExpireDays      int
	DB                        *gorm.DB
	plugin                    *MP4Plugin
}

// 为了兼容性，保留 DeleteRecordTask 作为别名
type DeleteRecordTask = StorageManagementTask

func (t *DeleteRecordTask) GetTickInterval() time.Duration {
	return 1 * time.Minute
}

func (t *StorageManagementTask) Tick(any) {
	t.Debug("StorageManagementTask", "tick started")

	// 阶段1：文件迁移（优先级最高，释放主存储空间）
	t.Debug("StorageManagementTask", "phase 1: file migration")
	t.migrateFiles()

	// 阶段2：删除过期文件
	t.Debug("StorageManagementTask", "phase 2: delete expired files")
	t.deleteExpiredFiles()

	// 阶段3：删除最旧文件（兜底机制）
	t.Debug("StorageManagementTask", "phase 3: delete oldest files")
	t.deleteOldestFile()

	t.Debug("StorageManagementTask", "tick completed")
}

// migrateFiles 将主存储中的文件迁移到次级存储
func (t *StorageManagementTask) migrateFiles() {
	// 只有配置了迁移阈值才执行迁移
	if t.MigrationThresholdPercent <= 0 {
		t.Debug("migrateFiles", "migration disabled", "threshold not configured or set to 0")
		return
	}

	t.Debug("migrateFiles", "starting migration check,threshold", t.MigrationThresholdPercent)

	// 收集所有需要检查的路径（使用 map 去重）
	pathMap := make(map[string]string) // primary path -> secondary path
	if t.plugin.Server.Plugins.Length > 0 {
		t.plugin.Server.Plugins.Range(func(plugin *m7s.Plugin) bool {
			if len(plugin.GetCommonConf().OnPub.Record) > 0 {
				for _, conf := range plugin.GetCommonConf().OnPub.Record {
					// 只处理配置了次级路径的录像配置
					if conf.SecondaryFilePath == "" {
						t.Debug("migrateFiles", "skipping path without secondary storage,path", conf.FilePath)
						continue
					}
					primaryPath := filepath.Dir(conf.FilePath)
					secondaryPath := filepath.Dir(conf.SecondaryFilePath)

					// 检查是否已存在
					if existingSecondary, exists := pathMap[primaryPath]; exists {
						if existingSecondary != secondaryPath {
							t.Warn("migrateFiles", "duplicate primary path with different secondary paths",
								"primary", primaryPath,
								"existing secondary", existingSecondary,
								"new secondary", secondaryPath)
						} else {
							t.Debug("migrateFiles", "duplicate path ignored,primary", primaryPath)
						}
						continue
					}

					pathMap[primaryPath] = secondaryPath
					t.Debug("migrateFiles", "added path for migration check,primary", primaryPath, "secondary", secondaryPath)
				}
			}
			return true
		})
	}

	if len(pathMap) == 0 {
		t.Debug("migrateFiles", "no secondary paths configured", "skipping migration")
		return
	}

	t.Debug("migrateFiles", "checking paths count", len(pathMap))

	// 遍历每个主存储路径
	for primaryPath, secondaryPath := range pathMap {
		usage := t.getDiskUsagePercent(primaryPath)
		t.Debug("migrateFiles", "checking disk usage,path", primaryPath, "usage", usage, "threshold", t.MigrationThresholdPercent)

		if usage < t.MigrationThresholdPercent {
			t.Debug("migrateFiles", "usage below threshold,path", primaryPath, "skipping")
			continue // 未达到迁移阈值，跳过
		}

		t.Info("migrateFiles", "migration triggered", "primary path", primaryPath, "secondary path", secondaryPath, "usage", usage, "threshold", t.MigrationThresholdPercent)

		// 查找主存储中最旧的已完成录像（storage_level=1）
		var recordStreams []m7s.RecordStream
		basePath := strings.Replace(primaryPath, "\\", "\\\\", -1)
		searchPattern := basePath + "%"

		// 每次迁移多个文件，提高效率
		err := t.DB.Where("record_level!='high' AND end_time IS NOT NULL AND storage_level=1").
			Where("file_path LIKE ?", searchPattern).
			Order("end_time ASC").
			Limit(10). // 批量迁移10个文件
			Find(&recordStreams).Error

		if err != nil {
			t.Error("migrateFiles", "query records error", err)
			continue
		}

		if len(recordStreams) == 0 {
			t.Debug("migrateFiles", "no files to migrate", "path", primaryPath)
			continue
		}

		t.Info("migrateFiles", "found files to migrate", "count", len(recordStreams), "path", primaryPath)

		for _, record := range recordStreams {
			t.Debug("migrateFiles", "migrating file", "ID", record.ID, "filepath", record.FilePath, "endTime", record.EndTime)
			if err := t.migrateFile(&record, primaryPath); err != nil {
				t.Error("migrateFiles", "migrate file error", err, "ID", record.ID, "filepath", record.FilePath)
			} else {
				t.Info("migrateFiles", "file migrated successfully", "ID", record.ID, "from", record.FilePath, "to", record.FilePath)
			}
		}
	}

	t.Debug("migrateFiles", "migration check completed")
}

// migrateFile 迁移单个文件到次级存储
func (t *StorageManagementTask) migrateFile(record *m7s.RecordStream, primaryPath string) error {
	// 获取次级存储路径
	secondaryPath := t.getSecondaryPath(primaryPath)
	if secondaryPath == "" {
		t.Debug("migrateFile", "no secondary path found", "primaryPath", primaryPath)
		return fmt.Errorf("no secondary path configured for %s", primaryPath)
	}

	// 构建目标路径（保持相对路径结构）
	relativePath := strings.TrimPrefix(record.FilePath, primaryPath)
	relativePath = strings.TrimPrefix(relativePath, string(filepath.Separator))
	targetPath := filepath.Join(secondaryPath, relativePath)

	t.Debug("migrateFile", "preparing migration", "from", record.FilePath, "to", targetPath)

	// 创建目标目录
	targetDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Error("migrateFile", "create target directory failed", err, "dir", targetDir)
		return fmt.Errorf("create target directory failed: %w", err)
	}

	t.Debug("migrateFile", "target directory created", "dir", targetDir)

	// 移动文件
	if err := os.Rename(record.FilePath, targetPath); err != nil {
		t.Debug("migrateFile", "rename failed, trying copy", "error", err)
		// 如果跨磁盘移动失败，尝试复制后删除
		if err := t.copyAndRemove(record.FilePath, targetPath); err != nil {
			t.Error("migrateFile", "copy and remove failed", err)
			return fmt.Errorf("move file failed: %w", err)
		}
		t.Debug("migrateFile", "file copied and removed")
	} else {
		t.Debug("migrateFile", "file renamed successfully")
	}

	// 更新数据库记录
	oldPath := record.FilePath
	record.FilePath = targetPath
	record.StorageLevel = 2
	if err := t.DB.Save(record).Error; err != nil {
		t.Error("migrateFile", "database update failed, rolling back", err)
		// 如果数据库更新失败，尝试回滚文件移动
		if rollbackErr := os.Rename(targetPath, oldPath); rollbackErr != nil {
			t.Error("migrateFile", "rollback failed", rollbackErr, "file may be in inconsistent state")
		} else {
			t.Debug("migrateFile", "rollback successful")
		}
		return fmt.Errorf("update database failed: %w", err)
	}

	t.Debug("migrateFile", "database updated", "storageLevel", 2, "newPath", targetPath)

	return nil
}

// copyAndRemove 复制文件后删除原文件（用于跨磁盘移动）
func (t *StorageManagementTask) copyAndRemove(src, dst string) error {
	t.Debug("copyAndRemove", "starting cross-disk copy", "from", src, "to", dst)

	// 获取源文件信息
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	fileSize := srcInfo.Size()
	t.Debug("copyAndRemove", "source file info", "size", fileSize)

	// 打开源文件
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// 创建目标文件
	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	// 复制文件内容
	t.Debug("copyAndRemove", "copying file content")
	copiedBytes, err := io.Copy(dstFile, srcFile)
	if err != nil {
		os.Remove(dst) // 复制失败，删除目标文件
		t.Error("copyAndRemove", "copy failed", err, "copiedBytes", copiedBytes)
		return err
	}

	t.Debug("copyAndRemove", "file copied", "bytes", copiedBytes)

	// 同步到磁盘
	if err := dstFile.Sync(); err != nil {
		t.Error("copyAndRemove", "sync failed", err)
		return err
	}

	t.Debug("copyAndRemove", "synced to disk, removing source file")

	// 删除源文件
	if err := os.Remove(src); err != nil {
		t.Error("copyAndRemove", "remove source file failed", err)
		return err
	}

	t.Debug("copyAndRemove", "source file removed successfully")
	return nil
}

// getSecondaryPath 获取主路径对应的次级存储路径
func (t *StorageManagementTask) getSecondaryPath(primaryPath string) string {
	if len(t.plugin.GetCommonConf().OnPub.Record) > 0 {
		for _, conf := range t.plugin.GetCommonConf().OnPub.Record {
			dirPath := filepath.Dir(conf.FilePath)
			if dirPath == primaryPath && conf.SecondaryFilePath != "" {
				return filepath.Dir(conf.SecondaryFilePath)
			}
		}
	}
	return ""
}

// getDiskUsagePercent 获取磁盘使用率百分比
func (t *StorageManagementTask) getDiskUsagePercent(filePath string) float64 {
	exePath := filepath.Dir(filePath)
	d, err := disk.Usage(exePath)
	if err != nil || d == nil {
		return 0
	}
	return d.UsedPercent
}

// deleteExpiredFiles 删除过期文件
func (t *StorageManagementTask) deleteExpiredFiles() {
	if t.RecordFileExpireDays <= 0 {
		return
	}

	var records []m7s.RecordStream
	expireTime := time.Now().AddDate(0, 0, -t.RecordFileExpireDays)
	t.Debug("deleteExpiredFiles", "expireTime", expireTime.Format("2006-01-02 15:04:05"))

	err := t.DB.Find(&records, "end_time < ? AND end_time IS NOT NULL", expireTime).Error
	if err == nil {
		for _, record := range records {
			t.Info("deleteExpiredFiles", "ID", record.ID, "endTime", record.EndTime, "filepath", record.FilePath)
			err = os.Remove(record.FilePath)
			if err != nil {
				if os.IsNotExist(err) {
					t.Warn("deleteExpiredFiles", "file does not exist", record.FilePath)
				} else {
					t.Error("deleteExpiredFiles", "delete file error", err)
				}
			}
			// 无论文件是否存在，都删除数据库记录
			err = t.DB.Delete(&record).Error
			if err != nil {
				t.Error("deleteExpiredFiles", "delete record from db error", err)
			}
		}
	}
}
