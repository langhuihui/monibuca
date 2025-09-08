package plugin_s3

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"m7s.live/v5"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/plugin/s3/pb"
)

// 上传任务队列工作器
type UploadQueueTask struct {
	task.Work
}

var uploadQueueTask UploadQueueTask

// 文件上传任务
type FileUploadTask struct {
	task.Task
	plugin            *S3Plugin
	filePath          string
	objectKey         string
	deleteAfterUpload bool
}

type S3Plugin struct {
	pb.UnimplementedApiServer
	m7s.Plugin
	Endpoint          string `desc:"S3 service endpoint, such as MinIO address"`
	Region            string `default:"us-east-1" desc:"AWS region"`
	AccessKeyID       string `desc:"S3 access key ID"`
	SecretAccessKey   string `desc:"S3 secret access key"`
	Bucket            string `desc:"S3 bucket name"`
	PathPrefix        string `desc:"file path prefix"`
	ForcePathStyle    bool   `desc:"force path style (required for MinIO)"`
	UseSSL            bool   `default:"true" desc:"whether to use SSL"`
	Auto              bool   `desc:"whether to automatically upload recorded files"`
	DeleteAfterUpload bool   `desc:"whether to delete local file after successful upload"`
	Timeout           int    `default:"30" desc:"upload timeout in seconds"`
	s3Client          *s3.S3
}

var _ = m7s.InstallPlugin[S3Plugin](m7s.PluginMeta{
	ServiceDesc:         &pb.Api_ServiceDesc,
	RegisterGRPCHandler: pb.RegisterApiHandler,
})

// 全局S3插件实例
var s3PluginInstance *S3Plugin

func init() {
	// 将上传队列任务添加到服务器
	m7s.Servers.AddTask(&uploadQueueTask)
}

func (p *S3Plugin) Start() error {
	// 设置全局实例
	s3PluginInstance = p

	// Set default configuration
	if p.Region == "" {
		p.Region = "us-east-1"
	}
	if p.Timeout == 0 {
		p.Timeout = 30
	}

	// Create AWS session configuration
	config := &aws.Config{
		Region:           aws.String(p.Region),
		Credentials:      credentials.NewStaticCredentials(p.AccessKeyID, p.SecretAccessKey, ""),
		S3ForcePathStyle: aws.Bool(p.ForcePathStyle),
	}

	// Set endpoint if provided (for MinIO or other S3-compatible services)
	if p.Endpoint != "" {
		protocol := "http"
		if p.UseSSL {
			protocol = "https"
		}
		endpoint := p.Endpoint
		if !strings.HasPrefix(endpoint, "http") {
			endpoint = protocol + "://" + endpoint
		}
		config.Endpoint = aws.String(endpoint)
		config.DisableSSL = aws.Bool(!p.UseSSL)
	}

	// Create AWS session
	sess, err := session.NewSession(config)
	if err != nil {
		return fmt.Errorf("failed to create AWS session: %v", err)
	}

	// Create S3 client
	p.s3Client = s3.New(sess)

	// Test connection
	if err := p.testConnection(); err != nil {
		return fmt.Errorf("S3 connection test failed: %v", err)
	}

	p.Info("S3 plugin initialized successfully")
	return nil
}

// testConnection tests the S3 connection
func (p *S3Plugin) testConnection() error {
	// Try to list buckets to test connection
	_, err := p.s3Client.ListBuckets(&s3.ListBucketsInput{})
	if err != nil {
		return err
	}

	p.Info("S3 connection test successful")
	return nil
}

// uploadFile uploads a file to S3
func (p *S3Plugin) uploadFile(filePath, objectKey string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return err
	}

	// Add path prefix if configured
	if p.PathPrefix != "" {
		objectKey = strings.TrimSuffix(p.PathPrefix, "/") + "/" + objectKey
	}

	// Upload file to S3
	input := &s3.PutObjectInput{
		Bucket:        aws.String(p.Bucket),
		Key:           aws.String(objectKey),
		Body:          file,
		ContentLength: aws.Int64(fileInfo.Size()),
		ContentType:   aws.String("application/octet-stream"),
	}

	_, err = p.s3Client.PutObject(input)
	if err != nil {
		return err
	}

	p.Info("File uploaded successfully", "objectKey", objectKey, "size", fileInfo.Size())
	return nil
}

// FileUploadTask的Start方法
func (task *FileUploadTask) Start() error {
	task.Info("Starting file upload", "filePath", task.filePath, "objectKey", task.objectKey)
	return nil
}

// FileUploadTask的Run方法
func (task *FileUploadTask) Run() error {
	// 检查文件是否存在
	if _, err := os.Stat(task.filePath); os.IsNotExist(err) {
		return fmt.Errorf("file does not exist: %s", task.filePath)
	}

	// 执行上传
	err := task.plugin.uploadFile(task.filePath, task.objectKey)
	if err != nil {
		task.Error("Failed to upload file", "error", err, "filePath", task.filePath)
		return err
	}

	// 如果配置了上传后删除，则删除本地文件
	if task.deleteAfterUpload {
		if err := os.Remove(task.filePath); err != nil {
			task.Warn("Failed to delete local file after upload", "error", err, "filePath", task.filePath)
		} else {
			task.Info("Local file deleted after successful upload", "filePath", task.filePath)
		}
	}

	task.Info("File upload completed successfully", "filePath", task.filePath, "objectKey", task.objectKey)
	return nil
}

// 队列上传文件方法
func (p *S3Plugin) QueueUpload(filePath, objectKey string, deleteAfter bool) {
	if !p.Auto {
		p.Debug("Auto upload is disabled, skipping upload", "filePath", filePath)
		return
	}

	uploadTask := &FileUploadTask{
		plugin:            p,
		filePath:          filePath,
		objectKey:         objectKey,
		deleteAfterUpload: deleteAfter || p.DeleteAfterUpload,
	}

	// 将上传任务添加到队列
	uploadQueueTask.AddTask(uploadTask, p.Logger.With("filePath", filePath, "objectKey", objectKey))
	p.Info("File upload queued", "filePath", filePath, "objectKey", objectKey)
}

// 生成S3对象键的辅助方法
func (p *S3Plugin) generateObjectKey(filePath string) string {
	// 获取文件名
	fileName := filepath.Base(filePath)

	// 如果配置了路径前缀，则添加前缀
	if p.PathPrefix != "" {
		return strings.TrimSuffix(p.PathPrefix, "/") + "/" + fileName
	}

	return fileName
}

// TriggerUpload 全局函数，供其他插件调用以触发S3上传
func TriggerUpload(filePath string, deleteAfter bool) {
	if s3PluginInstance == nil {
		return // S3插件未启用或未初始化
	}

	objectKey := s3PluginInstance.generateObjectKey(filePath)
	s3PluginInstance.QueueUpload(filePath, objectKey, deleteAfter)
}
