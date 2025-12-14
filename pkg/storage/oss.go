//go:build oss

package storage

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// OSSStorageConfig OSS存储配置
type OSSStorageConfig struct {
	Endpoint        string `yaml:"endpoint" desc:"OSS服务端点"`
	AccessKeyID     string `yaml:"access_key_id" desc:"OSS访问密钥ID"`
	AccessKeySecret string `yaml:"access_key_secret" desc:"OSS访问密钥Secret"`
	Bucket          string `yaml:"bucket" desc:"OSS存储桶名称"`
	PathPrefix      string `yaml:"path_prefix" desc:"文件路径前缀"`
	UseSSL          bool   `yaml:"use_ssl" desc:"是否使用SSL" default:"true"`
	Timeout         int    `yaml:"timeout" desc:"上传超时时间（秒）" default:"30"`
}

func (c *OSSStorageConfig) GetType() StorageType {
	return StorageTypeOSS
}

func (c *OSSStorageConfig) Validate() error {
	if c.AccessKeyID == "" {
		return fmt.Errorf("access_key_id is required for OSS storage")
	}
	if c.AccessKeySecret == "" {
		return fmt.Errorf("access_key_secret is required for OSS storage")
	}
	if c.Bucket == "" {
		return fmt.Errorf("bucket is required for OSS storage")
	}
	if c.Endpoint == "" {
		return fmt.Errorf("endpoint is required for OSS storage")
	}
	return nil
}

// OSSStorage OSS存储实现
type OSSStorage struct {
	config *OSSStorageConfig
	client *oss.Client
	bucket *oss.Bucket
}

// NewOSSStorage 创建OSS存储实例
func NewOSSStorage(config *OSSStorageConfig) (*OSSStorage, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	// 设置默认值
	if config.Timeout == 0 {
		config.Timeout = 30
	}

	// 创建OSS客户端
	client, err := oss.New(config.Endpoint, config.AccessKeyID, config.AccessKeySecret)
	if err != nil {
		return nil, fmt.Errorf("failed to create OSS client: %w", err)
	}

	// 获取存储桶
	bucket, err := client.Bucket(config.Bucket)
	if err != nil {
		return nil, fmt.Errorf("failed to get OSS bucket: %w", err)
	}

	// 测试连接
	if err := testOSSConnection(bucket); err != nil {
		return nil, fmt.Errorf("OSS connection test failed: %w", err)
	}

	return &OSSStorage{
		config: config,
		client: client,
		bucket: bucket,
	}, nil
}

func (s *OSSStorage) GetKey() string {
	return "oss"
}

func (s *OSSStorage) CreateFile(ctx context.Context, path string) (File, error) {
	objectKey := s.getObjectKey(path)
	return &OSSFile{
		storage:   s,
		objectKey: objectKey,
		ctx:       ctx,
	}, nil
}

func (s *OSSStorage) Delete(ctx context.Context, path string) error {
	objectKey := s.getObjectKey(path)
	return s.bucket.DeleteObject(objectKey)
}

func (s *OSSStorage) Exists(ctx context.Context, path string) (bool, error) {
	objectKey := s.getObjectKey(path)
	exists, err := s.bucket.IsObjectExist(objectKey)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func (s *OSSStorage) GetSize(ctx context.Context, path string) (int64, error) {
	objectKey := s.getObjectKey(path)

	props, err := s.bucket.GetObjectDetailedMeta(objectKey)
	if err != nil {
		if strings.Contains(err.Error(), "NoSuchKey") {
			return 0, ErrFileNotFound
		}
		return 0, err
	}

	contentLength := props.Get("Content-Length")
	if contentLength == "" {
		return 0, nil
	}

	var size int64
	if _, err := fmt.Sscanf(contentLength, "%d", &size); err != nil {
		return 0, fmt.Errorf("failed to parse content length: %w", err)
	}

	return size, nil
}

func (s *OSSStorage) GetURL(ctx context.Context, path string) (string, error) {
	objectKey := s.getObjectKey(path)

	// 生成签名URL，24小时有效期
	url, err := s.bucket.SignURL(objectKey, oss.HTTPGet, 24*3600)
	if err != nil {
		return "", err
	}

	return url, nil
}

func (s *OSSStorage) List(ctx context.Context, prefix string) ([]FileInfo, error) {
	objectPrefix := s.getObjectKey(prefix)

	var files []FileInfo

	err := s.bucket.ListObjects(oss.Prefix(objectPrefix), func(result oss.ListObjectsResult) error {
		for _, obj := range result.Objects {
			// 移除路径前缀
			fileName := obj.Key
			if s.config.PathPrefix != "" {
				fileName = strings.TrimPrefix(fileName, strings.TrimSuffix(s.config.PathPrefix, "/")+"/")
			}

			files = append(files, FileInfo{
				Name:         fileName,
				Size:         obj.Size,
				LastModified: obj.LastModified,
				ETag:         obj.ETag,
			})
		}
		return nil
	})

	return files, err
}

func (s *OSSStorage) Close() error {
	// OSS客户端无需显式关闭
	return nil
}

// getObjectKey 获取OSS对象键
func (s *OSSStorage) getObjectKey(path string) string {
	if s.config.PathPrefix != "" {
		return strings.TrimSuffix(s.config.PathPrefix, "/") + "/" + path
	}
	return path
}

// testOSSConnection 测试OSS连接
func testOSSConnection(bucket *oss.Bucket) error {
	// 尝试列出对象来测试连接
	_, err := bucket.ListObjects(oss.MaxKeys(1))
	return err
}

// OSSFile OSS文件读写器
type OSSFile struct {
	storage   *OSSStorage
	objectKey string
	ctx       context.Context
	tempFile  *os.File // 本地临时文件，用于支持随机访问
	filePath  string   // 临时文件路径
}

func (f *OSSFile) Name() string {
	return f.objectKey
}

func (f *OSSFile) Write(p []byte) (n int, err error) {
	// 如果还没有创建临时文件，先创建
	if f.tempFile == nil {
		if err = f.createTempFile(); err != nil {
			return 0, err
		}
	}

	// 写入到临时文件
	return f.tempFile.Write(p)
}

func (f *OSSFile) Read(p []byte) (n int, err error) {
	// 如果还没有创建缓存文件，先下载到本地
	if f.tempFile == nil {
		if err = f.downloadToTemp(); err != nil {
			return 0, err
		}
	}

	// 从本地缓存文件读取
	return f.tempFile.Read(p)
}

func (f *OSSFile) WriteAt(p []byte, off int64) (n int, err error) {
	// 如果还没有创建临时文件，先创建
	if f.tempFile == nil {
		if err = f.createTempFile(); err != nil {
			return 0, err
		}
	}

	// 写入到临时文件的指定位置
	return f.tempFile.WriteAt(p, off)
}

func (f *OSSFile) ReadAt(p []byte, off int64) (n int, err error) {
	// 如果还没有创建缓存文件，先下载到本地
	if f.tempFile == nil {
		if err = f.downloadToTemp(); err != nil {
			return 0, err
		}
	}

	// 从本地缓存文件的指定位置读取
	return f.tempFile.ReadAt(p, off)
}

func (f *OSSFile) Sync() error {
	// 如果使用临时文件，先同步到磁盘
	if f.tempFile != nil {
		if err := f.tempFile.Sync(); err != nil {
			return err
		}
	}
	if err := f.uploadTempFile(); err != nil {
		return err
	}
	return nil
}

func (f *OSSFile) Seek(offset int64, whence int) (int64, error) {
	// 如果还没有创建临时文件，先创建或下载
	if f.tempFile == nil {
		if err := f.downloadToTemp(); err != nil {
			return 0, err
		}
	}

	// 使用临时文件进行随机访问
	return f.tempFile.Seek(offset, whence)
}

func (f *OSSFile) Close() error {
	if err := f.Sync(); err != nil {
		return err
	}
	if f.tempFile != nil {
		f.tempFile.Close()
	}
	// 清理临时文件
	if f.filePath != "" {
		os.Remove(f.filePath)
	}
	return nil
}

// createTempFile 创建临时文件
func (f *OSSFile) createTempFile() error {
	// 创建临时文件
	tempFile, err := os.CreateTemp("", "osswriter_*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	f.tempFile = tempFile
	f.filePath = tempFile.Name()
	return nil
}

func (f *OSSFile) Stat() (os.FileInfo, error) {
	return f.tempFile.Stat()
}

// uploadTempFile 上传临时文件到OSS
func (f *OSSFile) uploadTempFile() (err error) {
	// 上传到OSS
	err = f.storage.bucket.PutObjectFromFile(f.objectKey, f.filePath)
	if err != nil {
		return fmt.Errorf("failed to upload to OSS: %w", err)
	}

	return nil
}

// downloadToTemp 下载OSS对象到本地临时文件
func (f *OSSFile) downloadToTemp() error {
	// 创建临时文件
	tempFile, err := os.CreateTemp("", "ossreader_*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	f.tempFile = tempFile
	f.filePath = tempFile.Name()

	// 下载OSS对象
	err = f.storage.bucket.GetObjectToFile(f.objectKey, f.filePath)
	if err != nil {
		tempFile.Close()
		os.Remove(f.filePath)
		if strings.Contains(err.Error(), "NoSuchKey") {
			return ErrFileNotFound
		}
		return fmt.Errorf("failed to download from OSS: %w", err)
	}

	// 重置文件指针到开始位置
	_, err = tempFile.Seek(0, 0)
	if err != nil {
		tempFile.Close()
		os.Remove(f.filePath)
		return fmt.Errorf("failed to seek temp file: %w", err)
	}

	return nil
}

func init() {
	Factory["oss"] = func(config any) (Storage, error) {
		var ossConfig OSSStorageConfig
		config.Parse(&ossConfig, config.(map[string]any))
		return NewOSSStorage(ossConfig)
	}
}
