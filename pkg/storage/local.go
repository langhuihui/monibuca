package storage

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/shirou/gopsutil/v4/disk"
	"gorm.io/gorm"
)

// RecordFile 录像文件记录（避免循环引用，只包含必要字段）
type RecordFile struct {
	ID           uint           `gorm:"primarykey"`
	StartTime    time.Time      `gorm:"column:start_time"`
	EndTime      time.Time      `gorm:"column:end_time"`
	FilePath     string         `gorm:"column:file_path"`
	StreamPath   string         `gorm:"column:stream_path"`
	StorageLevel int            `gorm:"column:storage_level"` // 1=主存储，2=备用存储
	StorageType  string         `gorm:"column:storage_type"`  // 存储类型：local/s3/oss/cos
	RecordLevel  string         `gorm:"column:record_level"`  // 'high'=重要录像，其他=普通录像
	CreatedAt    time.Time      `gorm:"column:created_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index"` // 软删除支持
}

// TableName 指定表名
func (RecordFile) TableName() string {
	return "record_streams"
}

// LocalStorageConfig 本地存储配置（主备两级）
type LocalStorageConfig struct {
	Path                   string `json:"path" yaml:"path"`                                     // 主存储路径
	BackupPath             string `json:"backuppath" yaml:"backuppath"`                         // 备用存储路径（可选）
	OverwritePercent       int    `json:"overwritepercent" yaml:"overwritepercent"`             // 主存储磁盘使用率阈值（0-100，0表示不检查）
	BackupOverwritePercent int    `json:"backupoverwritepercent" yaml:"backupoverwritepercent"` // 备用存储磁盘使用率阈值（0-100）
	FilePathPattern        string `json:"-" yaml:"-"`                                           // 文件路径模式（从 record.filepath 传入，用于数据库查询）
}

func (c *LocalStorageConfig) GetType() StorageType {
	return StorageTypeLocal
}

func (c *LocalStorageConfig) Validate() error {
	if c.OverwritePercent < 0 || c.OverwritePercent > 100 {
		return fmt.Errorf("overwritepercent must be between 0 and 100")
	}
	if c.BackupOverwritePercent < 0 || c.BackupOverwritePercent > 100 {
		return fmt.Errorf("backupoverwritepercent must be between 0 and 100")
	}
	// 如果配置了备用路径，必须配置备用阈值
	return nil
}

// parseLocalStorageConfig 解析配置，支持字符串和对象格式
func parseLocalStorageConfig(config any) (*LocalStorageConfig, error) {
	switch v := config.(type) {
	case string:
		// 兼容旧配置：字符串路径
		return &LocalStorageConfig{
			Path:                   v,
			BackupPath:             "",
			OverwritePercent:       0, // 0 表示不检查磁盘使用率
			BackupOverwritePercent: 0,
		}, nil

	case map[string]any:
		// 新配置：对象格式
		cfg := &LocalStorageConfig{}

		// 解析 path（必填）
		if path, ok := v["path"].(string); ok {
			cfg.Path = path
		} else {
			return nil, fmt.Errorf("path is required")
		}

		// 解析 backuppath（可选）
		if backupPath, ok := v["backuppath"].(string); ok {
			cfg.BackupPath = backupPath
		}

		// 解析 overwritepercent（可选）
		if percent, ok := v["overwritepercent"].(int); ok {
			cfg.OverwritePercent = percent
		} else if percent, ok := v["overwritepercent"].(float64); ok {
			cfg.OverwritePercent = int(percent)
		}

		// 解析 backupoverwritepercent（可选）
		if percent, ok := v["backupoverwritepercent"].(int); ok {
			cfg.BackupOverwritePercent = percent
		} else if percent, ok := v["backupoverwritepercent"].(float64); ok {
			cfg.BackupOverwritePercent = int(percent)
		}

		return cfg, nil

	default:
		return nil, fmt.Errorf("invalid config type for local storage: %T, expected string or map", config)
	}
}

// LocalStorage 本地存储实现
type LocalStorage struct {
	config          *LocalStorageConfig // 存储配置
	db              *gorm.DB            // 数据库连接（用于查询和更新记录）
	globalThreshold float64             // 全局磁盘使用率阈值（来自 mp4.autooverwritediskpercent）
}

// NewLocalStorage 创建本地存储实例
func NewLocalStorage(configAny any) (*LocalStorage, error) {
	config, err := parseLocalStorageConfig(configAny)
	if err != nil {
		return nil, err
	}

	if err := config.Validate(); err != nil {
		return nil, err
	}

	// 验证并创建主存储路径
	absPath, err := filepath.Abs(config.Path)
	if err != nil {
		return nil, fmt.Errorf("invalid path: %w", err)
	}
	if err := os.MkdirAll(absPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create path: %w", err)
	}

	// 如果配置了备用路径，验证并创建
	if config.BackupPath != "" {
		backupAbsPath, err := filepath.Abs(config.BackupPath)
		if err != nil {
			return nil, fmt.Errorf("invalid backup path: %w", err)
		}
		if err := os.MkdirAll(backupAbsPath, 0755); err != nil {
			return nil, fmt.Errorf("failed to create backup path: %w", err)
		}
	}

	return &LocalStorage{
		config: config,
	}, nil
}

// selectStoragePath 选择存储路径（主存储或备用存储）
// TODO: 后续可在此处实现磁盘使用率检查和自动降级
func (s *LocalStorage) selectStoragePath() (string, error) {
	// 当前简单返回主存储路径
	// 后续可根据磁盘使用率动态选择主存储或备用存储
	return s.config.Path, nil
}

func (s *LocalStorage) GetKey() string {
	return string(s.config.GetType())
}

// GetStoragePath 根据存储级别返回对应的存储路径
// storageLevel: 1=主存储, 2=备用存储
func (s *LocalStorage) GetStoragePath(storageLevel int) string {
	if storageLevel == 2 && s.config.BackupPath != "" {
		return s.config.BackupPath
	}
	return s.config.Path
}

// GetFullPath 根据存储级别和相对路径返回完整路径
// storageLevel: 1=主存储, 2=备用存储
// relativePath: 相对路径（如数据库中的 FilePath）
func (s *LocalStorage) GetFullPath(relativePath string, storageLevel int) string {
	basePath := s.GetStoragePath(storageLevel)
	return filepath.Join(basePath, relativePath)
}

func (s *LocalStorage) CreateFile(ctx context.Context, path string) (File, error) {
	// 选择存储路径
	basePath, err := s.selectStoragePath()
	if err != nil {
		return nil, err
	}

	// 构建完整路径
	fullPath := filepath.Join(basePath, path)
	dir := filepath.Dir(fullPath)

	// 确保目录存在
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// 使用 O_RDWR 而不是 O_WRONLY,因为某些场景(如 MP4 writeTrailer)需要读取文件内容
	file, err := os.OpenFile(fullPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
	}

	return file, nil
}

func (s *LocalStorage) OpenFile(ctx context.Context, path string) (File, error) {
	// 选择存储路径
	basePath, err := s.selectStoragePath()
	if err != nil {
		return nil, err
	}

	// 构建完整路径
	fullPath := filepath.Join(basePath, path)

	// 只读模式打开文件
	file, err := os.Open(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	return file, nil
}

func (s *LocalStorage) Delete(ctx context.Context, path string) error {
	basePath, err := s.selectStoragePath()
	if err != nil {
		return err
	}
	fullPath := filepath.Join(basePath, path)
	return os.Remove(fullPath)
}

func (s *LocalStorage) Exists(ctx context.Context, path string) (bool, error) {
	basePath, err := s.selectStoragePath()
	if err != nil {
		return false, err
	}
	fullPath := filepath.Join(basePath, path)
	_, err = os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *LocalStorage) GetSize(ctx context.Context, path string) (int64, error) {
	basePath, err := s.selectStoragePath()
	if err != nil {
		return 0, err
	}
	fullPath := filepath.Join(basePath, path)
	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, ErrFileNotFound
		}
		return 0, err
	}
	return info.Size(), nil
}

func (s *LocalStorage) GetURL(ctx context.Context, path string) (string, error) {
	basePath, err := s.selectStoragePath()
	if err != nil {
		return "", err
	}
	// 本地存储返回完整文件路径
	return filepath.Join(basePath, path), nil
}

func (s *LocalStorage) List(ctx context.Context, prefix string) ([]FileInfo, error) {
	basePath, err := s.selectStoragePath()
	if err != nil {
		return nil, err
	}
	searchPath := filepath.Join(basePath, prefix)
	var files []FileInfo

	err = filepath.Walk(searchPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			relPath, err := filepath.Rel(searchPath, path)
			if err != nil {
				return err
			}

			files = append(files, FileInfo{
				Name:         relPath,
				Size:         info.Size(),
				LastModified: info.ModTime(),
			})
		}
		return nil
	})

	return files, err
}

func (s *LocalStorage) Close() error {
	// 本地存储无需关闭连接
	return nil
}

// SetDB 设置数据库连接
func (s *LocalStorage) SetDB(db *gorm.DB) {
	s.db = db
}

// SetGlobalThreshold 设置全局磁盘使用率阈值
func (s *LocalStorage) SetGlobalThreshold(threshold float64) {
	s.globalThreshold = threshold
}

// SetFilePathPattern 设置文件路径模式（用于数据库查询）
func (s *LocalStorage) SetFilePathPattern(pattern string) {
	s.config.FilePathPattern = pattern
}

// getDiskUsagePercent 获取指定路径的磁盘使用率百分比
func (s *LocalStorage) getDiskUsagePercent(path string) float64 {
	d, err := disk.Usage(path)
	if err != nil || d == nil {
		return 0
	}
	return d.UsedPercent
}

// getEffectiveThreshold 获取有效的磁盘使用率阈值（本地配置优先，0 则使用全局）
func (s *LocalStorage) getEffectiveThreshold(localPercent int) float64 {
	if localPercent > 0 {
		return float64(localPercent)
	}
	return s.globalThreshold
}

// CheckAndManageStorage 检查并管理存储空间（迁移或删除文件）
func (s *LocalStorage) CheckAndManageStorage() error {
	if s.db == nil {
		return fmt.Errorf("database connection not set")
	}

	// 获取有效阈值
	primaryThreshold := s.getEffectiveThreshold(s.config.OverwritePercent)
	backupThreshold := s.getEffectiveThreshold(s.config.BackupOverwritePercent)

	// 检查主存储使用率
	primaryUsage := s.getDiskUsagePercent(s.config.Path)

	// 打印当前存储配置和使用情况
	log.Printf("[LocalStorage] CheckAndManageStorage - Config: path=%s, backupPath=%s, overwritePercent=%d, backupOverwritePercent=%d, globalThreshold=%.2f",
		s.config.Path, s.config.BackupPath, s.config.OverwritePercent, s.config.BackupOverwritePercent, s.globalThreshold)
	log.Printf("[LocalStorage] CheckAndManageStorage - Primary: usage=%.2f%%, threshold=%.2f%%",
		primaryUsage, primaryThreshold)

	// 主存储管理：循环处理直到低于阈值
	if primaryThreshold > 0 {
		for primaryUsage >= primaryThreshold {
			log.Printf("[LocalStorage] Primary storage exceeded threshold: %.2f%% >= %.2f%%", primaryUsage, primaryThreshold)

			if s.config.BackupPath != "" {
				// 有备用存储：迁移一个文件
				log.Printf("[LocalStorage] Action: Migrating one file to backup storage")
				if err := s.migrateOneFile(); err != nil {
					if err.Error() == "query record failed: record not found" {
						log.Printf("[LocalStorage] No more files to migrate, stopping")
						break
					}
					log.Printf("[LocalStorage] Migrate file failed: %v, continuing to next file", err)
					// 继续处理下一个文件（已在 migrateOneFile 中软删除失败的记录）
				}
			} else {
				// 无备用存储：删除一个文件
				log.Printf("[LocalStorage] Action: Deleting one file from primary storage")
				if err := s.deleteOldestFiles(s.config.Path); err != nil {
					if err.Error() == "query oldest record failed: record not found" {
						log.Printf("[LocalStorage] No more files to delete, stopping")
						break
					}
					log.Printf("[LocalStorage] Delete file failed: %v, continuing to next file", err)
					// 继续处理下一个文件（已在 deleteOldestFiles 中软删除失败的记录）
				}
			}

			// 重新检查磁盘使用率
			primaryUsage = s.getDiskUsagePercent(s.config.Path)
			log.Printf("[LocalStorage] Primary storage after operation: %.2f%%", primaryUsage)

			// 避免无限循环
			time.Sleep(100 * time.Millisecond)
		}
		log.Printf("[LocalStorage] Primary storage OK: %.2f%% < %.2f%%", primaryUsage, primaryThreshold)
	}

	// 备用存储管理：循环处理直到低于阈值
	if s.config.BackupPath != "" && backupThreshold > 0 {
		backupUsage := s.getDiskUsagePercent(s.config.BackupPath)
		log.Printf("[LocalStorage] CheckAndManageStorage - Backup: usage=%.2f%%, threshold=%.2f%%",
			backupUsage, backupThreshold)

		for backupUsage >= backupThreshold {
			log.Printf("[LocalStorage] Backup storage exceeded threshold: %.2f%% >= %.2f%%", backupUsage, backupThreshold)
			log.Printf("[LocalStorage] Action: Deleting one file from backup storage")

			if err := s.deleteOldestFiles(s.config.BackupPath); err != nil {
				if err.Error() == "query oldest record failed: record not found" {
					log.Printf("[LocalStorage] No more files to delete, stopping")
					break
				}
				log.Printf("[LocalStorage] Delete file failed: %v, continuing to next file", err)
				// 继续处理下一个文件（已在 deleteOldestFiles 中软删除失败的记录）
			}

			// 重新检查磁盘使用率
			backupUsage = s.getDiskUsagePercent(s.config.BackupPath)
			log.Printf("[LocalStorage] Backup storage after operation: %.2f%%", backupUsage)

			// 避免无限循环
			time.Sleep(100 * time.Millisecond)
		}
		log.Printf("[LocalStorage] Backup storage OK: %.2f%% < %.2f%%", backupUsage, backupThreshold)
	}

	return nil
}

// migrateOneFile 迁移一个最旧的文件到备用存储
func (s *LocalStorage) migrateOneFile() error {
	if s.config.BackupPath == "" {
		return fmt.Errorf("backup path not configured")
	}

	// 查询主存储中最旧的一个文件（storage_level=1 表示主存储）
	var record RecordFile
	err := s.db.Where("storage_level = ?", 1).
		Where("storage_type = ?", "local").
		Where("type = ?", "mp4").
		Where("end_time IS NOT NULL").
		Order("end_time ASC").
		First(&record).Error

	if err != nil {
		return fmt.Errorf("query record failed: %w", err)
	}

	// 迁移文件
	err = s.migrateFile(&record)
	if err != nil {
		// 迁移失败，软删除数据库记录，避免永远卡在这个文件上
		log.Printf("[LocalStorage] migrateOneFile - migration failed, soft deleting record: %s (ID=%d), error: %v", record.FilePath, record.ID, err)
		if deleteErr := s.db.Delete(&record).Error; deleteErr != nil {
			log.Printf("[LocalStorage] migrateOneFile - failed to soft delete record: %v", deleteErr)
		}
		return err
	}

	return nil
}

// migrateFile 迁移单个文件
func (s *LocalStorage) migrateFile(record *RecordFile) error {
	log.Printf("[LocalStorage] migrateFile - migrating file: %s (ID=%d, StorageLevel=%d -> 2)", record.FilePath, record.ID, record.StorageLevel)

	// 构建源文件和目标文件的绝对路径
	var srcPath, destPath string
	if filepath.IsAbs(record.FilePath) {
		// 已经是绝对路径（不应该出现这种情况，但做兼容处理）
		log.Printf("[LocalStorage] migrateFile - WARNING: file_path is absolute, this should not happen")
		srcPath = record.FilePath
		// 尝试提取相对路径部分用于目标路径
		relPath, err := filepath.Rel(s.config.Path, record.FilePath)
		if err != nil {
			return fmt.Errorf("cannot extract relative path: %w", err)
		}
		destPath = filepath.Join(s.config.BackupPath, relPath)
	} else {
		// 相对路径（正常情况）
		srcPath = filepath.Join(s.config.Path, record.FilePath)
		destPath = filepath.Join(s.config.BackupPath, record.FilePath)
	}
	destDir := filepath.Dir(destPath)

	log.Printf("[LocalStorage] migrateFile - source: %s, destination: %s", srcPath, destPath)

	// 确保目标目录存在
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("create dest directory failed: %w", err)
	}

	// 尝试使用 os.Rename（同磁盘快速移动）
	err := os.Rename(srcPath, destPath)
	if err != nil {
		// 跨磁盘移动，需要复制后删除
		log.Printf("[LocalStorage] migrateFile - cross-disk migration, using copy and remove")
		if err := s.copyAndRemove(srcPath, destPath); err != nil {
			return fmt.Errorf("copy and remove failed: %w", err)
		}
	}

	// 更新数据库记录（file_path 保持相对路径不变，只更新 storage_level）
	err = s.db.Model(record).
		Updates(map[string]interface{}{
			"storage_level": 2, // 2 表示备用存储
		}).Error

	if err != nil {
		return fmt.Errorf("update database failed: %w", err)
	}

	log.Printf("[LocalStorage] migrateFile - successfully migrated and updated database (ID=%d)", record.ID)

	return nil
}

// copyAndRemove 复制文件并删除源文件（用于跨磁盘迁移）
func (s *LocalStorage) copyAndRemove(src, dst string) error {
	// 打开源文件
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source file failed: %w", err)
	}
	defer srcFile.Close()

	// 创建目标文件
	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create dest file failed: %w", err)
	}
	defer dstFile.Close()

	// 复制内容
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("copy file failed: %w", err)
	}

	// 同步到磁盘
	if err := dstFile.Sync(); err != nil {
		return fmt.Errorf("sync file failed: %w", err)
	}

	// 删除源文件
	if err := os.Remove(src); err != nil {
		return fmt.Errorf("remove source file failed: %w", err)
	}

	return nil
}

// deleteOldestFiles 删除指定路径下最旧的文件（优先删除非重要录像）
func (s *LocalStorage) deleteOldestFiles(path string) error {
	// 判断是主存储还是备用存储
	storageLevel := 1 // 默认主存储
	if path == s.config.BackupPath {
		storageLevel = 2 // 备用存储
	}

	log.Printf("[LocalStorage] deleteOldestFiles - path=%s, storageLevel=%d", path, storageLevel)

	// 查询该存储级别下最旧的文件（record_level != 'high' 表示非重要录像）
	var record RecordFile
	err := s.db.Where("storage_type = ?", "local").
		Where("type = ?", "mp4").
		Where("storage_level = ?", storageLevel).
		Where("record_level != ?", "high").
		Where("end_time IS NOT NULL").
		Order("end_time ASC").
		First(&record).Error

	if err != nil {
		return fmt.Errorf("query oldest record failed: %w", err)
	}

	log.Printf("[LocalStorage] deleteOldestFiles - deleting file: %s (ID=%d, StorageLevel=%d)", record.FilePath, record.ID, record.StorageLevel)

	// 构建绝对路径
	var absolutePath string
	if filepath.IsAbs(record.FilePath) {
		// 已经是绝对路径，直接使用
		absolutePath = record.FilePath
		log.Printf("[LocalStorage] deleteOldestFiles - file_path is absolute, using directly")
	} else {
		// 相对路径，根据 storageLevel 拼接
		if storageLevel == 1 {
			// 主存储
			absolutePath = filepath.Join(s.config.Path, record.FilePath)
		} else {
			// 备用存储
			absolutePath = filepath.Join(s.config.BackupPath, record.FilePath)
		}
		log.Printf("[LocalStorage] deleteOldestFiles - file_path is relative, joined with storage path")
	}

	log.Printf("[LocalStorage] deleteOldestFiles - absolute path: %s", absolutePath)

	// 删除文件
	fileDeleteErr := os.Remove(absolutePath)
	if fileDeleteErr != nil && !os.IsNotExist(err) {
		// 文件删除失败，记录错误日志
		log.Printf("[LocalStorage] deleteOldestFiles - remove file failed: %v, will soft delete record anyway", fileDeleteErr)
	}

	// 删除数据库记录（软删除）
	// 即使文件删除失败，也要删除数据库记录，避免永远卡在这个文件上
	if err := s.db.Delete(&record).Error; err != nil {
		log.Printf("[LocalStorage] deleteOldestFiles - soft delete record failed: %v", err)
		return fmt.Errorf("delete database record failed: %w", err)
	}

	log.Printf("[LocalStorage] deleteOldestFiles - successfully deleted file and record (ID=%d)", record.ID)

	return nil
}

func init() {
	Factory["local"] = func(config any) (Storage, error) {
		return NewLocalStorage(config)
	}
}
