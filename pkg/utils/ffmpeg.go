package utils

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	ffmpeg "github.com/u2takey/ffmpeg-go"
)

// M3u8ToMp4 将 m3u8 格式转换为 mp4 格式
func M3u8ToMp4(input, output string) error {
	err := ffmpeg.Input(input).
		Output(output, ffmpeg.KwArgs{
			"c:v":     "copy",
			"absf":    "aac_adtstoasc",
			"b:v":     "4000k",
			"bufsize": "4000k",
		}).
		OverWriteOutput().
		Run()
	if err != nil {
		return err
	}
	return nil
}

// GetVideoThumnail 从视频中抽取第一帧作为缩略图（已弃用，使用 GetVideoThumbnail）
// 保留此函数以向后兼容
func GetVideoThumnail(videoPath, outputDir string) (string, error) {
	return GetVideoThumbnail(videoPath, outputDir, 0, 0, 0)
}

// ThumbnailSize 缩略图尺寸定义
type ThumbnailSize struct {
	Name   string
	Width  int
	Height int
}

// 标准缩略图尺寸
var (
	ThumbnailSmall  = ThumbnailSize{Name: "small", Width: 160, Height: 90}
	ThumbnailMedium = ThumbnailSize{Name: "medium", Width: 320, Height: 180}
	ThumbnailLarge  = ThumbnailSize{Name: "large", Width: 640, Height: 360}
)

// GetVideoThumbnail 从视频中抽取指定时间点的帧作为缩略图
// videoPath: 视频文件路径
// outputDir: 输出目录
// timeOffset: 时间偏移（秒），0表示抽取第一帧，-1表示自动选择最佳帧
// width: 输出宽度，0表示保持原始宽度
// height: 输出高度，0表示保持原始高度
func GetVideoThumbnail(videoPath, outputDir string, timeOffset, width, height int) (string, error) {
	if err := os.MkdirAll(outputDir, os.ModePerm); err != nil {
		return "", errors.WithMessage(err, "Failed to create folders")
	}

	outputPath := filepath.Join(outputDir, "thumbnail.jpg")

	// 构建 FFmpeg 参数
	args := ffmpeg.KwArgs{
		"vframes": "1",
	}

	// 设置时间偏移
	if timeOffset >= 0 {
		args["ss"] = fmt.Sprintf("00:00:%02d", timeOffset)
	}

	// 设置输出尺寸
	if width > 0 && height > 0 {
		args["s"] = fmt.Sprintf("%dx%d", width, height)
	}

	err := ffmpeg.Input(videoPath).
		Output(outputPath, args).
		OverWriteOutput().
		Run()

	if err != nil {
		return "", errors.WithMessage(err, "Failed to generate the thumbnail")
	}

	return outputPath, nil
}

// GetVideoThumbnails 批量生成多尺寸缩略图
// videoPath: 视频文件路径
// outputDir: 输出目录
// timeOffset: 时间偏移（秒），0表示抽取第一帧，-1表示自动选择最佳帧
// sizes: 需要生成的尺寸列表
func GetVideoThumbnails(videoPath, outputDir string, timeOffset int, sizes []ThumbnailSize) (map[string]string, error) {
	result := make(map[string]string)

	for _, size := range sizes {
		sizeDir := filepath.Join(outputDir, size.Name)
		if err := os.MkdirAll(sizeDir, os.ModePerm); err != nil {
			return nil, errors.WithMessage(err, fmt.Sprintf("Failed to create folder for size %s", size.Name))
		}

		outputPath := filepath.Join(sizeDir, fmt.Sprintf("thumb_%s.jpg", size.Name))

		// 构建 FFmpeg 参数
		args := ffmpeg.KwArgs{
			"vframes": "1",
		}

		// 设置时间偏移
		if timeOffset >= 0 {
			args["ss"] = fmt.Sprintf("00:00:%02d", timeOffset)
		} else {
			// 自动选择最佳帧（通常在第5秒或10秒处）
			args["ss"] = "00:00:05"
		}

		// 设置输出尺寸
		args["s"] = fmt.Sprintf("%dx%d", size.Width, size.Height)

		err := ffmpeg.Input(videoPath).
			Output(outputPath, args).
			OverWriteOutput().
			Run()

		if err != nil {
			return nil, errors.WithMessage(err, fmt.Sprintf("Failed to generate thumbnail for size %s", size.Name))
		}

		result[size.Name] = outputPath
	}

	return result, nil
}

// GetVideoThumbnailToBytes 从视频中抽取帧并返回字节流
// videoPath: 视频文件路径
// timeOffset: 时间偏移（秒）
// width: 输出宽度
// height: 输出高度
func GetVideoThumbnailToBytes(videoPath string, timeOffset, width, height int) ([]byte, error) {
	// 使用临时文件
	tempDir := os.TempDir()
	tempPath := filepath.Join(tempDir, fmt.Sprintf("thumb_%d_%d.jpg", timeOffset, width))

	// 生成缩略图
	_, err := GetVideoThumbnail(videoPath, tempDir, timeOffset, width, height)
	if err != nil {
		return nil, err
	}
	defer os.Remove(tempPath)

	// 读取文件内容
	data, err := os.ReadFile(tempPath)
	if err != nil {
		return nil, errors.WithMessage(err, "Failed to read thumbnail file")
	}

	return data, nil
}

// GetAnimatedCover 生成动态封面（GIF）
// videoPath: 视频文件路径
// outputDir: 输出目录
// startTime: 开始时间（秒）
// duration: 持续时间（秒）
// fps: 帧率
// width: 输出宽度
func GetAnimatedCover(videoPath, outputDir string, startTime, duration, fps, width int) (string, error) {
	if err := os.MkdirAll(outputDir, os.ModePerm); err != nil {
		return "", errors.WithMessage(err, "Failed to create folders")
	}

	outputPath := filepath.Join(outputDir, "animated_cover.gif")

	// 构建 FFmpeg 参数
	args := ffmpeg.KwArgs{
		"ss":   fmt.Sprintf("00:00:%02d", startTime),
		"t":    fmt.Sprintf("%d", duration),
		"vf":   fmt.Sprintf("fps=%d,scale=%d:-1:flags=lanczos,split[s0][s1];[s0]palettegen[p];[s1][p]paletteuse", fps, width),
		"loop": "0",
	}

	err := ffmpeg.Input(videoPath).
		Output(outputPath, args).
		OverWriteOutput().
		Run()

	if err != nil {
		return "", errors.WithMessage(err, "Failed to generate animated cover")
	}

	return outputPath, nil
}

// GetVideoMetadata 获取视频元数据信息
func GetVideoMetadata(videoPath string) (map[string]interface{}, error) {
	// 这是一个简化版本，实际应该使用 ffprobe
	info := make(map[string]interface{})

	// 读取文件大小
	fileInfo, err := os.Stat(videoPath)
	if err != nil {
		return nil, errors.WithMessage(err, "Failed to get video file info")
	}
	info["size"] = fileInfo.Size()

	// 这里可以扩展使用 ffprobe 获取更多信息
	// 如：时长、分辨率、编码格式等

	return info, nil
}
