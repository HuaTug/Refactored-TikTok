package service

import (
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"HuaTug.com/cmd/video/dal"
	"HuaTug.com/cmd/video/infras/redis"
	"HuaTug.com/config"
	"HuaTug.com/kitex_gen/videos"
	"HuaTug.com/pkg/oss"
)

// setupTestEnvironment initializes the required dependencies for testing
func setupTestEnvironment(t *testing.T) {
	// Change to the project root directory to find config files
	originalDir, _ := os.Getwd()
	projectRoot := filepath.Join(originalDir, "../../../")
	os.Chdir(projectRoot)
	defer os.Chdir(originalDir)

	// Initialize configuration
	config.Init()

	// Try to initialize database - if it fails, skip the test
	defer func() {
		if r := recover(); r != nil {
			t.Skipf("Database connection failed, skipping test: %v", r)
		}
	}()

	// Initialize database
	dal.Init()
	// Initialize Redis
	redis.Load()
	// Initialize MinIO
	oss.InitMinio()
} // TestMinIOChunkedUpload 演示MinIO分片上传的完整流程
func TestMinIOChunkedUpload(t *testing.T) {
	// 在实际测试环境中跳过
	if testing.Short() {
		t.Skip("跳过MinIO上传测试")
	}

	// Initialize test environment
	setupTestEnvironment(t)

	ctx := context.Background()
	service := NewVideoUploadServiceV2(ctx)

	// 1. 开始上传 - 初始化分片上传
	startReq := &videos.VideoPublishStartRequestV2{
		Title:            "Test Video",
		Description:      "MinIO分片上传测试",
		Category:         "tech",
		Tags:             []string{"test", "minio"},
		ChunkTotalNumber: 3, // 3个分片
		UserId:           12345,
	}

	session, err := service.StartUpload(startReq)
	if err != nil {
		t.Fatalf("启动上传失败: %v", err)
	}

	t.Logf("上传会话创建成功:\n")
	t.Logf("- UUID: %s\n", session.UUID)
	t.Logf("- MinIO UploadID: %s\n", session.MinIOUploadID)
	t.Logf("- 存储桶: %s\n", session.BucketName)
	t.Logf("- 对象名: %s\n", session.ObjectName)
	t.Logf("- 总分片数: %d\n", session.TotalChunks)

	// 2. 上传分片
	chunks := [][]byte{
		bytes.Repeat([]byte("A"), 5*1024*1024), // 5MB 分片1
		bytes.Repeat([]byte("B"), 5*1024*1024), // 5MB 分片2
		bytes.Repeat([]byte("C"), 3*1024*1024), // 3MB 分片3（最后一个分片可以小于5MB）
	}

	for i, chunkData := range chunks {
		chunkReq := &videos.VideoPublishUploadingRequestV2{
			UploadSessionUuid: session.UUID,
			UserId:            12345,
			ChunkNumber:       int32(i + 1),
			ChunkData:         chunkData,
			ChunkMd5:          "dummy-md5", // 实际应该计算真实MD5
		}

		err = service.UploadChunk(chunkReq)
		if err != nil {
			t.Fatalf("上传分片 %d 失败: %v", i+1, err)
		}

		t.Logf("分片 %d 上传成功\n", i+1)
	}

	// 3. 完成上传 - 合并所有分片
	completeReq := &videos.VideoPublishCompleteRequestV2{
		UploadSessionUuid: session.UUID,
		UserId:            12345,
	}

	err = service.CompleteUpload(completeReq)
	if err != nil {
		t.Fatalf("完成上传失败: %v", err)
	}

	t.Logf("视频上传完成！文件已在MinIO中合并为完整视频\n")
}

// TestCancelUpload 演示如何取消分片上传
func TestCancelUpload(t *testing.T) {
	// 在实际测试环境中跳过
	if testing.Short() {
		t.Skip("跳过MinIO取消上传测试")
	}

	// Initialize test environment
	setupTestEnvironment(t)

	ctx := context.Background()
	service := NewVideoUploadServiceV2(ctx)

	// 开始上传
	startReq := &videos.VideoPublishStartRequestV2{
		Title:            "Test Video to Cancel",
		Description:      "将被取消的测试视频",
		Category:         "test",
		ChunkTotalNumber: 2,
		UserId:           12345,
	}

	session, err := service.StartUpload(startReq)
	if err != nil {
		t.Fatalf("启动上传失败: %v", err)
	}

	// 上传一个分片
	chunkReq := &videos.VideoPublishUploadingRequestV2{
		UploadSessionUuid: session.UUID,
		UserId:            12345,
		ChunkNumber:       1,
		ChunkData:         bytes.Repeat([]byte("X"), 5*1024*1024),
		ChunkMd5:          "dummy-md5",
	}

	err = service.UploadChunk(chunkReq)
	if err != nil {
		t.Fatalf("上传分片失败: %v", err)
	}

	// 取消上传
	cancelReq := &videos.VideoPublishCancelRequestV2{
		UploadSessionUuid: session.UUID,
		UserId:            12345,
	}

	err = service.CancelUpload(cancelReq)
	if err != nil {
		t.Fatalf("取消上传失败: %v", err)
	}

	t.Logf("上传已取消，MinIO中的临时分片已清理\n")
}

// TestMinIOUploadFlow 测试MinIO上传流程的完整性
func TestMinIOUploadFlow(t *testing.T) {
	// 在实际测试环境中运行
	if testing.Short() {
		t.Skip("跳过MinIO上传测试")
	}

	// 这里可以调用上面的测试函数
	// TestMinIOChunkedUpload(t)
	t.Log("MinIO分片上传流程示例代码已准备好")
}

// TestRealVideoUpload 使用真实视频文件进行分片上传测试
func TestRealVideoUpload(t *testing.T) {
	// 在实际测试环境中跳过
	if testing.Short() {
		t.Skip("跳过真实视频上传测试")
	}

	// Initialize test environment
	setupTestEnvironment(t)

	// 真实视频文件路径
	videoPath := "/Users/xuzhihua/Downloads/jordan.mp4"

	// 检查文件是否存在
	if _, err := os.Stat(videoPath); os.IsNotExist(err) {
		t.Skipf("视频文件不存在: %s", videoPath)
	}

	// 获取文件信息
	fileInfo, err := os.Stat(videoPath)
	if err != nil {
		t.Fatalf("无法获取文件信息: %v", err)
	}

	fileSize := fileInfo.Size()
	t.Logf("视频文件大小: %d bytes (%.2f MB)", fileSize, float64(fileSize)/(1024*1024))

	// 计算需要的分片数量（每片5MB）
	chunkSize := int64(5 * 1024 * 1024)                          // 5MB per chunk
	totalChunks := int32((fileSize + chunkSize - 1) / chunkSize) // 向上取整

	t.Logf("分片大小: %d bytes (5MB)", chunkSize)
	t.Logf("总分片数: %d", totalChunks)

	ctx := context.Background()
	service := NewVideoUploadServiceV2(ctx)

	// 1. 开始上传 - 初始化分片上传
	startReq := &videos.VideoPublishStartRequestV2{
		Title:            "Real Video Test",
		Description:      "使用真实视频文件的MinIO分片上传测试",
		Category:         "tech",
		Tags:             []string{"test", "real-video", "minio"},
		ChunkTotalNumber: totalChunks,
		UserId:           12345,
	}

	session, err := service.StartUpload(startReq)
	if err != nil {
		t.Fatalf("启动上传失败: %v", err)
	}

	t.Logf("上传会话创建成功:")
	t.Logf("- UUID: %s", session.UUID)
	t.Logf("- MinIO UploadID: %s", session.MinIOUploadID)
	t.Logf("- 存储桶: %s", session.BucketName)
	t.Logf("- 对象名: %s", session.ObjectName)
	t.Logf("- 总分片数: %d", session.TotalChunks)

	// 2. 打开视频文件并分片上传
	file, err := os.Open(videoPath)
	if err != nil {
		t.Fatalf("无法打开视频文件: %v", err)
	}
	defer file.Close()

	for chunkNum := int32(1); chunkNum <= totalChunks; chunkNum++ {
		// 计算当前分片的大小
		currentChunkSize := chunkSize
		if chunkNum == totalChunks {
			// 最后一个分片可能小于5MB
			remainingBytes := fileSize - (int64(chunkNum-1) * chunkSize)
			if remainingBytes < chunkSize {
				currentChunkSize = remainingBytes
			}
		}

		// 读取分片数据
		chunkData := make([]byte, currentChunkSize)
		bytesRead, err := file.Read(chunkData)
		if err != nil && err != io.EOF {
			t.Fatalf("读取分片 %d 失败: %v", chunkNum, err)
		}

		// 如果实际读取的字节数小于预期，调整分片大小
		if int64(bytesRead) < currentChunkSize {
			chunkData = chunkData[:bytesRead]
		}

		// 计算分片的MD5
		hash := md5.Sum(chunkData)
		chunkMd5 := fmt.Sprintf("%x", hash)

		t.Logf("分片 %d: 大小=%d bytes, MD5=%s", chunkNum, len(chunkData), chunkMd5)

		// 上传分片
		chunkReq := &videos.VideoPublishUploadingRequestV2{
			UploadSessionUuid: session.UUID,
			UserId:            12345,
			ChunkNumber:       chunkNum,
			ChunkData:         chunkData,
			ChunkMd5:          chunkMd5,
		}

		err = service.UploadChunk(chunkReq)
		if err != nil {
			t.Fatalf("上传分片 %d 失败: %v", chunkNum, err)
		}

		t.Logf("分片 %d/%d 上传成功 ✓", chunkNum, totalChunks)
	}

	// 3. 完成上传 - 合并所有分片
	completeReq := &videos.VideoPublishCompleteRequestV2{
		UploadSessionUuid: session.UUID,
		UserId:            12345,
	}

	err = service.CompleteUpload(completeReq)
	if err != nil {
		t.Fatalf("完成上传失败: %v", err)
	}

	t.Logf("✅ 真实视频文件上传完成！文件已在MinIO中合并为完整视频")
	t.Logf("原始文件: %s (%.2f MB)", videoPath, float64(fileSize)/(1024*1024))
	t.Logf("分片数量: %d", totalChunks)
}
