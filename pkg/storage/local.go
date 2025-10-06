package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// LocalStorageConfig 本地存储配置
type LocalStorageConfig string

func (c LocalStorageConfig) GetType() StorageType {
	return StorageTypeLocal
}

func (c LocalStorageConfig) Validate() error {
	if c == "" {
		return fmt.Errorf("base_path is required for local storage")
	}
	return nil
}

// LocalStorage 本地存储实现
type LocalStorage struct {
	basePath string
}

// NewLocalStorage 创建本地存储实例
func NewLocalStorage(config LocalStorageConfig) (*LocalStorage, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	basePath, err := filepath.Abs(string(config))
	if err != nil {
		return nil, fmt.Errorf("invalid base path: %w", err)
	}

	// 确保基础路径存在
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base path: %w", err)
	}

	return &LocalStorage{
		basePath: basePath,
	}, nil
}

func (s *LocalStorage) CreateFile(ctx context.Context, path string) (File, error) {
	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// 使用 O_RDWR 而不是 O_WRONLY,因为某些场景(如 MP4 writeTrailer)需要读取文件内容
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
	}

	return file, nil
}

func (s *LocalStorage) Delete(ctx context.Context, path string) error {
	return os.Remove(path)
}

func (s *LocalStorage) Exists(ctx context.Context, path string) (bool, error) {
	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *LocalStorage) GetSize(ctx context.Context, path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, ErrFileNotFound
		}
		return 0, err
	}
	return info.Size(), nil
}

func (s *LocalStorage) GetURL(ctx context.Context, path string) (string, error) {
	// 本地存储返回文件路径
	return path, nil
}

func (s *LocalStorage) List(ctx context.Context, prefix string) ([]FileInfo, error) {
	searchPath := filepath.Join(prefix)
	var files []FileInfo

	err := filepath.Walk(searchPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			relPath, err := filepath.Rel(prefix, path)
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

func init() {
	Factory["local"] = func(config any) (Storage, error) {
		localConfig, ok := config.(string)
		if !ok {
			return nil, fmt.Errorf("invalid config type for local storage")
		}
		return NewLocalStorage(LocalStorageConfig(localConfig))
	}
}
