package plugin_s3

import (
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"m7s.live/v5"
	"m7s.live/v5/plugin/s3/pb"
)

type S3Plugin struct {
	pb.UnimplementedApiServer
	m7s.Plugin
	Endpoint        string `desc:"S3 service endpoint, such as MinIO address"`
	Region          string `default:"us-east-1" desc:"AWS region"`
	AccessKeyID     string `desc:"S3 access key ID"`
	SecretAccessKey string `desc:"S3 secret access key"`
	Bucket          string `desc:"S3 bucket name"`
	PathPrefix      string `desc:"file path prefix"`
	ForcePathStyle  bool   `desc:"force path style (required for MinIO)"`
	UseSSL          bool   `default:"true" desc:"whether to use SSL"`
	Auto            bool   `desc:"whether to automatically upload recorded files"`
	Timeout         int    `default:"30" desc:"upload timeout in seconds"`
	s3Client        *s3.S3
}

var _ = m7s.InstallPlugin[S3Plugin](m7s.PluginMeta{
	ServiceDesc:         &pb.Api_ServiceDesc,
	RegisterGRPCHandler: pb.RegisterApiHandler,
})

func (p *S3Plugin) Start() error {
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
