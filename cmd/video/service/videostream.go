package service

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"strconv"

	"HuaTug.com/kitex_gen/base"
	"HuaTug.com/kitex_gen/videos"
	"HuaTug.com/pkg/oss"
	"github.com/cloudwego/hertz/pkg/common/hlog"
)

var VideoFiles []*base.Video

type StreamVideoService struct {
	ctx context.Context
}

func NewStreamVideoService(ctx context.Context) *StreamVideoService {
	return &StreamVideoService{ctx: ctx}
}
func (s *StreamVideoService) VideoStream(req *videos.StreamVideoRequestV2) (string, error) {

	//db.UpdateVideoUrl(s.ctx, url, "", fmt.Sprint(VideoFiles[index].VideoId))
	if req.VideoId == "" {
		return "", fmt.Errorf("Missing video ID")
	}

	// Convert video ID to index for now - this may need to be updated based on your logic
	index, err := strconv.Atoi(req.VideoId)
	if err != nil {
		hlog.Info(err)
	}

	if err != nil || index < 0 || index > len(VideoFiles) {
		return "", fmt.Errorf("invalid video index")
	}
	VideoFiles, err = NewFeedListService(s.ctx).FeedList(&videos.VideoFeedListRequestV2{
		PageNum:  1,
		PageSize: 10,
	})
	hlog.Info(VideoFiles)
	if err != nil {
		hlog.Info(err)
	}

	hlog.Info(index)
	hlog.Info(VideoFiles)
	//通过这个预签名的url，可以来访问minio中的视频文件
	url, err := oss.GeneratePreUrl("video", "video/"+fmt.Sprint(VideoFiles[index].VideoId)+"/video.mp4", fmt.Sprint(VideoFiles[index].VideoId))
	if err != nil {
		hlog.Info(err)
	}

	hlog.Info(url)

	videoFilePath := "../../Download_video/videos" + fmt.Sprint(VideoFiles[index].VideoId) + ".mp4"
	//videoFile, err := os.Open(VideoFiles[index].VideoUrl)
	videoFile, err := os.Open(videoFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to open video file: %v", err)
	}
	defer videoFile.Close()

	return videoFilePath, nil
}

func (s *StreamVideoService) ServeVideo(w http.ResponseWriter, r *http.Request) {
	req := &videos.StreamVideoRequestV2{
		VideoId: r.URL.Query().Get("video_id"),
	}
	videoFilePath, err := s.VideoStream(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	fileInfo, err := os.Stat(videoFilePath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Content-Length", strconv.FormatInt(fileInfo.Size(), 10))
	w.Header().Set("Accept-Ranges", "bytes")

	videoFile, err := os.Open(videoFilePath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer videoFile.Close()

	http.ServeContent(w, r, filepath.Base(videoFilePath), fileInfo.ModTime(), videoFile)
}

// func main() {
// 	service := &StreamVideoService{}
// 	http.HandleFunc("/video/stream", service.ServeVideo) // 使用服务的 ServeVideo 方法
// 	if err := http.ListenAndServe(":8080", nil); err != nil {
// 		hlog.Fatal("Failed to start server:", err)
// 	}
// }
