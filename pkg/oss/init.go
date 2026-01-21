package oss

import (
	"context"
	"os"
	"time"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func InitMinio() error {
	// 从环境变量获取配置，如果没有则使用智能默认值
	endpoint := getEnvOrDefault("MINIO_ENDPOINT", "localhost:9002")
	accessKeyID := getEnvOrDefault("MINIO_ACCESS_KEY", "tiktok_minio_admin")
	secretAccessKey := getEnvOrDefault("MINIO_SECRET_KEY", "MainMinIO@TikTok#2025!SecurePass")
	useSSL := getEnvOrDefault("MINIO_USE_SSL", "false") == "true"

	hlog.Infof("Initializing MinIO client with endpoint: %s, accessKey: %s", endpoint, accessKeyID)

	var err error
	minioClient, err = minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		hlog.Errorf("Failed to create MinIO client: %v", err)
		return err
	}

	hlog.Info("Connect Minio Success")

	// Set public read policy for all buckets that need cross-origin access
	// Run this in a goroutine to avoid blocking the main initialization
	go func() {
		bucketsToSetPolicy := []string{"tiktok-user-content", "video", "picture", "tiktok-cache-hot"}
		for _, bucketName := range bucketsToSetPolicy {
			// Add retry logic with exponential backoff
			if err := SetBucketPublicReadPolicyWithRetry(bucketName, 3); err != nil {
				hlog.Warnf("Failed to set policy for %s bucket after retries: %v", bucketName, err)
			}
			// Add delay between bucket operations to avoid rate limiting
			time.Sleep(500 * time.Millisecond)
		}
	}()

	return nil
}

// SetBucketPublicReadPolicyWithRetry sets public read policy with retry logic
func SetBucketPublicReadPolicyWithRetry(bucketName string, maxRetries int) error {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			// Exponential backoff: 1s, 2s, 4s...
			backoff := time.Duration(1<<uint(i)) * time.Second
			hlog.Infof("Retrying set policy for bucket %s in %v (attempt %d/%d)", bucketName, backoff, i+1, maxRetries)
			time.Sleep(backoff)
		}

		lastErr = SetBucketPublicReadPolicy(bucketName)
		if lastErr == nil {
			return nil
		}
		hlog.Warnf("Attempt %d failed for bucket %s: %v", i+1, bucketName, lastErr)
	}
	return lastErr
}

// SetBucketPublicReadPolicy sets public read policy for a bucket to allow cross-origin video access
func SetBucketPublicReadPolicy(bucketName string) error {
	ctx := context.Background()

	// Check if bucket exists first
	exists, err := minioClient.BucketExists(ctx, bucketName)
	if err != nil {
		return err
	}
	if !exists {
		hlog.Infof("Bucket %s does not exist yet, creating it...", bucketName)
		// Create the bucket if it doesn't exist
		err = minioClient.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{Region: "us-east-1"})
		if err != nil {
			hlog.Warnf("Failed to create bucket %s: %v", bucketName, err)
			return err
		}
		hlog.Infof("Successfully created bucket: %s", bucketName)
	}

	// Set bucket policy to allow public read access for videos
	policy := `{
		"Version": "2012-10-17",
		"Statement": [
			{
				"Effect": "Allow",
				"Principal": {"AWS": ["*"]},
				"Action": ["s3:GetObject"],
				"Resource": ["arn:aws:s3:::` + bucketName + `/*"]
			}
		]
	}`

	err = minioClient.SetBucketPolicy(ctx, bucketName, policy)
	if err != nil {
		return err
	}

	hlog.Infof("Successfully set public read policy for bucket: %s", bucketName)
	return nil
}

// getEnvOrDefault 获取环境变量，如果不存在则返回默认值
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
