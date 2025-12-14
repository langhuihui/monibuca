package plugin_mp4

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	task "github.com/langhuihui/gotask"
	"github.com/shirou/gopsutil/v4/disk"
	"gorm.io/gorm"
	"m7s.live/v5"
	"m7s.live/v5/pkg/storage"
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
		"disk free", d.Free, "disk usage", d.Used, "OverwritePercent", p.OverwritePercent, "DiskMaxPercent", p.DiskMaxPercent)
	return d.UsedPercent >= p.OverwritePercent
}

func (p *DeleteRecordTask) deleteOldestFile() {
	//当当前磁盘使用量大于OverwritePercent自动覆盖磁盘使用量配置时，自动删除最旧的文件
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
						p.Info("deleteOldestFile", "action", "add path", "original", conf.FilePath, "processed", dirPath)
					} else {
						p.Debug("deleteOldestFile", "status", "duplicate path ignored", "path", dirPath)
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
	p.Debug("deleteOldestFile", "stage", "after onpub.record", "count", len(filePaths))
	if p.plugin.EventRecordFilePath != "" {
		// 同样处理EventRecordFilePath
		dirPath := filepath.Dir(p.plugin.EventRecordFilePath)
		filePaths = append(filePaths, dirPath)
	}
	p.Debug("deleteOldestFile", "stage", "after get eventrecordfilepath", "count", len(filePaths))
	for _, filePath := range filePaths {
		for p.getDiskOutOfSpace(filePath) {
			var recordStreams []m7s.RecordStream
			// 使用不同的方法进行路径匹配，避免ESCAPE语法问题
			// 解决方案：用MySQL能理解的简单方式匹配路径前缀
			basePath := filePath
			// 直接替换所有反斜杠，不需要判断是否包含
			basePath = strings.Replace(basePath, "\\", "\\\\", -1)
			searchPattern := basePath + "%"
			p.Info("deleteOldestFile", "action", "searching", "pattern", searchPattern)

			err := p.DB.Where(" record_level!='high' AND end_time IS NOT NULL").
				Where("file_path LIKE ?", searchPattern).
				Order("end_time ASC").Limit(1).Find(&recordStreams).Error
			if err == nil {
				if len(recordStreams) > 0 {
					p.Info("deleteOldestFile", "found", len(recordStreams), "unit", "records")
					for _, record := range recordStreams {
						p.Info("deleteOldestFile", "action", "deleting", "ID", record.ID, "endTime", record.EndTime, "filepath", record.FilePath)
						err = os.Remove(record.FilePath)
						if err != nil {
							// 检查是否为文件不存在的错误
							if os.IsNotExist(err) {
								// 文件不存在，记录日志但视为删除成功
								p.Warn("deleteOldestFile", "status", "file not exist, continuing", "filepath", record.FilePath)
								// 继续删除数据库记录
								err = p.DB.Delete(&record).Error
								if err != nil {
									p.Error("deleteOldestFile", "error", "delete record from db", "err", err)
								}
							} else {
								// 其他错误，记录并跳过此记录
								p.Error("deleteOldestFile", "error", "delete file from disk", "err", err)
								continue
							}
						} else {
							// 文件删除成功，继续删除数据库记录
							err = p.DB.Delete(&record).Error
							if err != nil {
								p.Error("deleteOldestFile", "error", "delete record from db", "err", err)
							}
						}
					}
				}
			} else {
				p.Error("deleteOldestFile", "error", "search record from db", "err", err)
			}
			time.Sleep(time.Second * 3)
		}
	}
}

type StorageManagementTask struct {
	task.TickTask
	DiskMaxPercent       float64
	OverwritePercent     float64
	RecordFileExpireDays int
	DB                   *gorm.DB
	plugin               *MP4Plugin
}

// 为了兼容性，保留 DeleteRecordTask 作为别名
type DeleteRecordTask = StorageManagementTask

func (t *DeleteRecordTask) GetTickInterval() time.Duration {
	return 1 * time.Minute
}

func (t *StorageManagementTask) Tick(any) {
	t.Debug("StorageManagementTask", "status", "tick started")

	// 阶段1：LocalStorage 存储管理（迁移或删除）
	t.Debug("StorageManagementTask", "phase", "1", "action", "local storage management")
	t.manageLocalStorage()

	// 阶段2：删除过期文件
	t.Debug("StorageManagementTask", "phase", "2", "action", "delete expired files")
	t.deleteExpiredFiles()

	// 注意：阶段3 deleteOldestFile 已被移除，因为阶段1的 manageLocalStorage 已经处理了磁盘空间管理

	t.Debug("StorageManagementTask", "status", "tick completed")
}

// manageLocalStorage 管理本地存储（通过 LocalStorage 进行迁移或删除）
func (t *StorageManagementTask) manageLocalStorage() {
	t.Debug("manageLocalStorage", "status", "starting")

	// 检查全局存储是否存在且为 LocalStorage 类型
	st := t.plugin.Server.Storage
	if st == nil {
		t.Debug("manageLocalStorage", "status", "global storage not initialized, using fallback logic")
		t.manageFallbackStorage()
		return
	}

	localStorage, ok := st.(*storage.LocalStorage)
	if !ok {
		t.Debug("manageLocalStorage", "status", "global storage is not LocalStorage, using fallback logic")
		t.manageFallbackStorage()
		return
	}

	// 设置数据库连接和全局阈值
	localStorage.SetDB(t.DB)
	localStorage.SetGlobalThreshold(t.OverwritePercent)

	// 执行存储管理
	if err := localStorage.CheckAndManageStorage(); err != nil {
		t.Error("manageLocalStorage", "error", "check and manage storage failed", "err", err)
	} else {
		t.Debug("manageLocalStorage", "status", "success")
	}

	t.Debug("manageLocalStorage", "status", "completed")
}

// manageFallbackStorage 兜底逻辑：当全局存储不是 LocalStorage 时，使用全局配置管理磁盘空间
func (t *StorageManagementTask) manageFallbackStorage() {
	t.Debug("manageFallbackStorage", "status", "starting")

	// 尝试从全局存储获取路径
	var storagePath string
	if st := t.plugin.Server.Storage; st != nil {
		if localStorage, ok := st.(*storage.LocalStorage); ok {
			// 使用主存储路径
			storagePath = localStorage.GetStoragePath(1)
		} else {
			// 非本地存储，尝试使用 GetURL 获取路径（可能不适用）
			t.Debug("manageFallbackStorage", "status", "global storage is not LocalStorage, cannot get path")
		}
	}

	// 如果无法从全局存储获取路径，使用第一个录像配置的 filepath 目录
	if storagePath == "" {
		recordConfigs := t.plugin.GetCommonConf().OnPub.Record
		if len(recordConfigs) > 0 {
			// 遍历 map 获取第一个配置的 filepath 目录
			for _, conf := range recordConfigs {
				storagePath = filepath.Dir(conf.FilePath)
				t.Debug("manageFallbackStorage", "action", "using first record config path", "path", storagePath)
				break
			}
		} else {
			t.Debug("manageFallbackStorage", "status", "no storage path found")
			return
		}
	}

	// 检查磁盘使用率（使用存储路径）
	diskUsage := t.getDiskUsagePercent(storagePath)
	t.Debug("manageFallbackStorage", "storagePath", storagePath, "diskUsage", diskUsage, "threshold", t.OverwritePercent)

	// 如果超过阈值，删除最旧的文件
	for diskUsage >= t.OverwritePercent {
		t.Info("manageFallbackStorage", "action", "disk usage exceeded", "storagePath", storagePath, "usage", diskUsage, "threshold", t.OverwritePercent)

		// 查询最旧的文件
		var record m7s.RecordStream
		err := t.DB.Where("storage_type = ?", "local").
			Where("type = ?", "mp4").
			Where("storage_level = ?", 1).
			Where("record_level != ?", "high").
			Where("end_time IS NOT NULL").
			Order("end_time ASC").
			First(&record).Error

		if err != nil {
			if err == gorm.ErrRecordNotFound {
				// 没有非重要录像，查询所有录像
				err = t.DB.Where("storage_type = ?", "local").
					Where("type = ?", "mp4").
					Where("storage_level = ?", 1).
					Where("end_time IS NOT NULL").
					Order("end_time ASC").
					First(&record).Error
			}
			if err != nil {
				t.Error("manageFallbackStorage", "error", "query oldest record failed", "err", err, "storagePath", storagePath)
				break
			}
		}

		t.Info("manageFallbackStorage", "action", "deleting file", "ID", record.ID, "filePath", record.FilePath)

		// 判断 file_path 是相对路径还是绝对路径
		var absolutePath string
		if filepath.IsAbs(record.FilePath) {
			// 绝对路径，直接使用
			absolutePath = record.FilePath
		} else {
			// 相对路径，使用全局存储的 GetFullPath 方法
			if st := t.plugin.Server.Storage; st != nil {
				if localStorage, ok := st.(*storage.LocalStorage); ok {
					absolutePath = localStorage.GetFullPath(record.FilePath, record.StorageLevel)
				} else {
					absolutePath = record.FilePath
					t.Warn("manageFallbackStorage", "warning", "file_path is relative and storage is not LocalStorage", "filePath", record.FilePath)
				}
			} else {
				absolutePath = record.FilePath
				t.Warn("manageFallbackStorage", "warning", "file_path is relative and no global storage", "filePath", record.FilePath)
			}
		}

		t.Debug("manageFallbackStorage", "action", "removing file", "absolutePath", absolutePath)

		// 删除文件
		fileDeleteErr := os.Remove(absolutePath)
		if fileDeleteErr != nil && !os.IsNotExist(fileDeleteErr) {
			// 文件删除失败，记录错误日志
			t.Error("manageFallbackStorage", "error", "remove file failed", "err", fileDeleteErr, "filePath", absolutePath)
		}

		// 删除数据库记录（软删除）
		// 即使文件删除失败，也要删除数据库记录，避免永远卡在这个文件上
		if err := t.DB.Delete(&record).Error; err != nil {
			t.Error("manageFallbackStorage", "error", "delete database record failed", "err", err, "ID", record.ID)
			break
		}

		t.Info("manageFallbackStorage", "status", "file deleted successfully", "ID", record.ID)

		// 重新检查磁盘使用率
		diskUsage = t.getDiskUsagePercent(storagePath)
		t.Debug("manageFallbackStorage", "status", "rechecking disk usage", "storagePath", storagePath, "usage", diskUsage)

		// 避免无限循环，休眠一下
		time.Sleep(time.Second)
	}

	t.Debug("manageFallbackStorage", "status", "completed")
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
					t.Warn("deleteExpiredFiles", "status", "file not exist", "filepath", record.FilePath)
				} else {
					t.Error("deleteExpiredFiles", "error", "delete file", "err", err)
				}
			}
			// 无论文件是否存在，都删除数据库记录
			err = t.DB.Delete(&record).Error
			if err != nil {
				t.Error("deleteExpiredFiles", "error", "delete record from db", "err", err)
			}
		}
	}
}
