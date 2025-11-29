package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"
)

// StorageType 存储类型
type StorageType string

const (
	StorageTypeLocal StorageType = "local"
	StorageTypeS3    StorageType = "s3"
	StorageTypeOSS   StorageType = "oss"
	StorageTypeCOS   StorageType = "cos"
)

// StorageConfig 存储配置接口
type StorageConfig interface {
	GetType() StorageType
	Validate() error
}

// Storage 存储接口
type Storage interface {
	CreateFile(ctx context.Context, path string) (File, error)
	// OpenFile 以只读模式打开文件（不会上传修改）
	OpenFile(ctx context.Context, path string) (File, error)
	// Delete 删除文件
	Delete(ctx context.Context, path string) error

	// Exists 检查文件是否存在
	Exists(ctx context.Context, path string) (bool, error)

	// GetSize 获取文件大小
	GetSize(ctx context.Context, path string) (int64, error)

	// GetURL 获取文件访问URL
	GetURL(ctx context.Context, path string) (string, error)

	// List 列出文件
	List(ctx context.Context, prefix string) ([]FileInfo, error)

	// Close 关闭存储连接
	Close() error
}

// Writer 写入器接口
type Writer interface {
	io.Writer
	io.WriterAt
	io.Closer

	// Sync 同步到存储
	Sync() error

	// Seek 设置写入位置
	Seek(offset int64, whence int) (int64, error)
}

// Reader 读取器接口
type Reader interface {
	io.Reader
	io.ReaderAt
	io.Closer
	io.Seeker
}

type File interface {
	Writer
	Reader
	Stat() (os.FileInfo, error)
	Name() string
}

// FileInfo 文件信息
type FileInfo struct {
	Name         string    `json:"name"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"last_modified"`
	ETag         string    `json:"etag,omitempty"`
	ContentType  string    `json:"content_type,omitempty"`
}

// CreateStorage 创建存储实例的便捷函数
func CreateStorage(t string, config any) (Storage, error) {
	factory, exists := Factory[t]
	if !exists {
		return nil, ErrUnsupportedStorageType
	}
	return factory(config)
}

// 错误定义
var (
	ErrUnsupportedStorageType = fmt.Errorf("unsupported storage type")
	ErrFileNotFound           = fmt.Errorf("file not found")
	ErrStorageNotAvailable    = fmt.Errorf("storage not available")
)
