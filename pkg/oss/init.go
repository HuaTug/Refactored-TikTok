package oss

import (
	"os"

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
	return nil
}

// getEnvOrDefault 获取环境变量，如果不存在则返回默认值
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
