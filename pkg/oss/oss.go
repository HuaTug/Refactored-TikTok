package oss

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/minio/minio-go/v7"
)

var (
	minioClient *minio.Client
)

// GetMinIOEndpoint returns the MinIO API endpoint for URL generation
func GetMinIOEndpoint() string {
	// Use the same endpoint as configured in InitMinio
	endpoint := getEnvOrDefault("MINIO_ENDPOINT", "localhost:9002")
	useSSL := getEnvOrDefault("MINIO_USE_SSL", "false") == "true"
	scheme := "http"
	if useSSL {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, endpoint)
}

func UploadAvatar(data *[]byte, dataSize int64, uid string, tag string) (string, error) {
	// 在上传头像时需要满足 先删除旧的头像后 再上传新的头像
	deleteAvatar(uid)
	bucketName := "picture"
	location := "us-east-1" // MinIO默认区域，根据实际情况修改

	// 检查存储桶是否存在，不存在则创建
	exists, err := minioClient.BucketExists(context.Background(), bucketName)
	if err != nil {
		return "", fmt.Errorf("check bucket error: %w", err)
	}
	if !exists {
		err = minioClient.MakeBucket(context.Background(), bucketName, minio.MakeBucketOptions{Region: location})
		if err != nil {
			return "", fmt.Errorf("create bucket error: %w", err)
		}
	}
	var suffix string
	switch tag {
	case "image/jpeg", "image/jpg":
		suffix = ".jpg"
	case "image/png":
		suffix = ".png"
	default:
		return "", fmt.Errorf("unsupported image format: %s", tag)
	}

	objectName := "avatar/" + uid + suffix
	_, err = minioClient.PutObject(context.Background(), bucketName, objectName, bytes.NewReader(*data), dataSize, minio.PutObjectOptions{ContentType: tag})
	if err != nil {
		log.Fatal("Failed to upload avatar:", err)
		return "", err
	}

	// Use MinIO API URL format for direct access
	return fmt.Sprintf("%s/%s/%s", GetMinIOEndpoint(), bucketName, objectName), nil
}

func deleteAvatar(uid string) {
	bucketName := "picture"
	keys := []string{
		"avatar/" + uid + ".jpg",
		"avatar/" + uid + ".jpeg",
		"avatar/" + uid + ".png",
	}
	for _, key := range keys {
		err := minioClient.RemoveObject(context.Background(), bucketName, key, minio.RemoveObjectOptions{})
		if err != nil {
			log.Printf("Failed to delete %s: %v", key, err)
		}
	}
}

func UploadVideo(path, vid string) (string, error) {
	// 这是我上传视频文件的路径
	objectName := "video/" + vid + "/video.mp4"
	bucketName := "video"
	location := "us-east-1" // MinIO默认区域，根据实际情况修改

	// 检查存储桶是否存在，不存在则创建
	exists, err := minioClient.BucketExists(context.Background(), bucketName)
	if err != nil {
		return "", fmt.Errorf("check bucket error: %w", err)
	}
	if !exists {
		err = minioClient.MakeBucket(context.Background(), bucketName, minio.MakeBucketOptions{Region: location})
		if err != nil {
			return "", fmt.Errorf("create bucket error: %w", err)
		}
	}

	// 上传视频文件到 MinIO
	_, err = minioClient.FPutObject(context.Background(), bucketName, objectName, path, minio.PutObjectOptions{ContentType: "video/mp4"})
	if err != nil {
		hlog.Info(err)
		return "", err
	}

	// 返回视频的 URL - Use MinIO API URL format for direct access
	return fmt.Sprintf("%s/%s/%s", GetMinIOEndpoint(), bucketName, objectName), nil
}

// ToDo: 上传视频封面时 也可以让用户自定义封面
func UploadVideoCover(path, vid string) (string, error) {
	objectName := "picture/" + vid + "/cover.jpg"
	bucketName := "picture"
	location := "us-east-1" // MinIO默认区域，根据实际情况修改

	// 检查存储桶是否存在，不存在则创建
	exists, err := minioClient.BucketExists(context.Background(), bucketName)
	if err != nil {
		return "", fmt.Errorf("check bucket error: %w", err)
	}
	if !exists {
		err = minioClient.MakeBucket(context.Background(), bucketName, minio.MakeBucketOptions{Region: location})
		if err != nil {
			return "", fmt.Errorf("create bucket error: %w", err)
		}
	}
	// 上传封面文件到 MinIO
	_, err = minioClient.FPutObject(context.Background(), bucketName, objectName, path, minio.PutObjectOptions{ContentType: "image/jpeg"})
	if err != nil {
		return "", err
	}

	// 返回封面的 URL - Use MinIO API URL format for direct access
	return fmt.Sprintf("%s/%s/%s", GetMinIOEndpoint(), bucketName, objectName), nil
}

// 生成了下载的预签名地址，然后下载视频
func GeneratePreUrl(bucketName, objectName, vid string) (string, error) {
	presignedURL, err := minioClient.PresignedGetObject(context.Background(), bucketName, objectName, 30*time.Minute, nil)
	if err != nil {
		hlog.Info(err)
		return "", err
	}
	hlog.Info("Download URL:", presignedURL)

	// 发起 GET 请求下载视频
	resp, err := http.Get(presignedURL.String())
	if err != nil {
		hlog.Info(err)
	}
	defer resp.Body.Close()

	outputDir := "../../Download_video/"

	// 创建文件夹
	err = os.MkdirAll(outputDir, os.ModePerm)
	if err != nil {
		hlog.Info("Error creating directory: ", err)
		return "", err
	}

	// 创建文件
	filePath := outputDir + "videos" + vid + ".mp4"

	out, err := os.Create(filePath)
	if err != nil {
		hlog.Info(err)
	}
	defer out.Close()

	// 将下载的内容写入文件
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		hlog.Info("Error saving video: ", err)
	}

	hlog.Info("Video Downloaded successfully!")

	return presignedURL.String(), nil
}

// GeneratePresignedPutURL 生成预签名上传 URL
func GeneratePresignedPutURL(bucketName, objectName string, expires time.Duration) (string, error) {
	if minioClient == nil {
		return "", fmt.Errorf("MinIO client not initialized")
	}

	// 确保 bucket 存在
	exists, err := minioClient.BucketExists(context.Background(), bucketName)
	if err != nil {
		return "", fmt.Errorf("check bucket error: %w", err)
	}
	if !exists {
		err = minioClient.MakeBucket(context.Background(), bucketName, minio.MakeBucketOptions{Region: "us-east-1"})
		if err != nil {
			return "", fmt.Errorf("create bucket error: %w", err)
		}
		// 设置公开读取策略
		SetBucketPublicReadPolicy(bucketName)
	}

	presignedURL, err := minioClient.PresignedPutObject(context.Background(), bucketName, objectName, expires)
	if err != nil {
		return "", fmt.Errorf("generate presigned URL error: %w", err)
	}

	return presignedURL.String(), nil
}
