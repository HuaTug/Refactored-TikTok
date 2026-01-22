package db

import (
	"context"
	"fmt"
	"sync"

	"HuaTug.com/cmd/model"
	"HuaTug.com/kitex_gen/base"
	"HuaTug.com/kitex_gen/videos"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func Feedlist(ctx context.Context, req *videos.VideoFeedListRequestV2) ([]*base.Video, error) {
	var video []*base.Video
	query := DB.WithContext(ctx).Model(&base.Video{})

	// 添加分类过滤
	if req.CategoryFilter != "" {
		query = query.Where("category = ?", req.CategoryFilter)
	}

	// 添加隐私过滤
	if req.PrivacyFilter != "" {
		query = query.Where("privacy = ?", req.PrivacyFilter)
	}

	// 分页查询
	if err := query.Limit(int(req.PageSize)).Offset(int((req.PageNum - 1) * req.PageSize)).Find(&video); err != nil {
		return video, errors.Wrapf(err.Error, "FeedList failed,err:%v", err)
	}
	return video, nil
}

func GetAllFeedList(ctx context.Context, req *videos.VideoFeedListRequestV2) ([]*base.Video, error) {
	var video []*base.Video
	if err := DB.WithContext(ctx).Model(&base.Video{}).Find(&video); err != nil {
		return video, errors.Wrapf(err.Error, "GetAllFeedList failed,err:%v", err)
	}
	return video, nil
}

// 获取用户发布的视频
func Videolist(ctx context.Context, req *videos.VideoFeedListRequestV2) ([]*base.Video, int64, error) {
	var video []*base.Video
	var count int64
	if err := DB.WithContext(ctx).Model(&base.Video{}).Where("user_id=?", req.UserId).Count(&count).Limit(int(req.PageSize)).
		Offset(int((req.PageNum - 1) * req.PageSize)).Find(&video); err != nil {
		logrus.Info(err)
		return video, count, errors.Wrapf(err.Error, "VideoList failed,err:%v", err)
	}
	return video, count, nil
}

func Videosearch(ctx context.Context, req *videos.VideoSearchRequestV2) ([]*base.Video, int64, error) {
	var wg sync.WaitGroup
	var video2 []*base.Video
	var count int64
	var err error
	if req.Keyword != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			query := DB.WithContext(ctx).Model(&base.Video{}).
				Where("title like ?", "%"+req.Keyword+"%")

			// 添加日期范围过滤
			if req.FromDate != "" {
				query = query.Where("created_at > ?", req.FromDate)
			}
			if req.ToDate != "" {
				query = query.Where("created_at < ?", req.ToDate)
			}

			// 添加分类过滤
			if len(req.Categories) > 0 {
				query = query.Where("category IN ?", req.Categories)
			}

			err = query.Count(&count).
				Limit(int(req.PageSize)).Offset(int((req.PageNum - 1) * req.PageSize)).
				Find(&video2).Error
		}()
		if err != nil {
			return video2, count, errors.Wrapf(err, "VideoSearch failed,err:%v", err)
		}
		wg.Wait()
	}
	return video2, count, nil
}

func FindVideo(ctx context.Context, videoId int64) (video *base.Video, err error) {
	if err := DB.WithContext(ctx).Model(&base.Video{}).Where("video_id=?", videoId).Find(&video); err != nil {
		return video, errors.Wrapf(err.Error, "FindVideo failed,err:%v", err)
	}
	return video, nil
}

func InsertVideo(ctx context.Context, video *base.Video) error {
	if err := DB.WithContext(ctx).Create(video).Error; err != nil {
		return err
	}
	return nil
}
func GetMaxVideoId(ctx context.Context, userID int64) (string, error) {
	var maxId *int64
	if err := DB.WithContext(ctx).Model(&base.Video{}).Where("user_id = ?", userID).Select("MAX(video_id)").Scan(&maxId).Error; err != nil {
		return "", err
	}
	if maxId == nil {
		return "1", nil
	}

	return fmt.Sprint(*maxId + 1), nil
}
func GetVideo(ctx context.Context, vid int64) (*base.Video, error) {
	var data base.Video
	if err := DB.WithContext(ctx).Model(&base.Video{}).Where("video_id = ?", vid).Find(&data).Error; err != nil {
		return nil, err
	}
	return &data, nil
}

// 对于视频列表的查询
func GetVideoByVideoId(ctx context.Context, vid []int64) ([]*base.Video, error) {
	var data []*base.Video
	if err := DB.WithContext(ctx).Model(&base.Video{}).Where("video_id IN (?)", vid).Find(&data).Error; err != nil {
		return nil, err
	}
	return data, nil
}

func UpdateVideoUrl(ctx context.Context, videoUrl, coverUrl, vid string) error {
	if err := DB.WithContext(ctx).Model(&base.Video{}).Where("video_id = ?", vid).Update("video_url", videoUrl).Error; err != nil {
		return err
	}
	if err := DB.WithContext(ctx).Model(&base.Video{}).Where("video_id = ?", vid).Update("cover_url", coverUrl).Error; err != nil {
		return err
	}
	return nil
}

func UpdateVideoVisit(ctx context.Context, vid, visitCount int64) error {
	if err := DB.WithContext(ctx).Model(&base.Video{}).Where("video_id = ?", vid).Update("visit_count", visitCount).Error; err != nil {
		return err
	}
	return nil
}

func UpdateVideoCommentCount(ctx context.Context, vid, commentCount int64) error {
	if err := DB.WithContext(ctx).Model(&base.Video{}).Where("video_id =?", vid).Update("comment_count", commentCount).Error; err != nil {
		return err
	}
	return nil
}

func UpdateVideoLikeCount(ctx context.Context, vid, likeCount int64) error {
	if err := DB.WithContext(ctx).Model(&base.Video{}).Where("video_id =?", vid).Update("likes_count", likeCount).Error; err != nil {
		return err
	}
	return nil
}

func UpdateVideoHisLikeCount(ctx context.Context, vid, hisLikeCount int64) error {
	if err := DB.WithContext(ctx).Model(&base.Video{}).Where("video_id =?", vid).Update("history_count", hisLikeCount).Error; err != nil {
		return err
	}
	return nil
}

func UpdateVideoShareCount(ctx context.Context, vid, shareCount int64) error {
	if err := DB.WithContext(ctx).Model(&base.Video{}).Where("video_id =?", vid).Update("share_count", shareCount).Error; err != nil {
		return err
	}
	return nil
}

func DeleteVideo(ctx context.Context, vid, uid string) error {
	result := DB.Model(&base.Video{}).Where("video_id = ? And user_id=? ", vid, uid).Delete(&base.Video{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("No rows has been affected")
	}
	return nil
}

func GetVideoVisitCount(ctx context.Context, vid string) (count int64, err error) {
	//Scan用于将查询结果集映射到某一个值上 Scan和Count的区别使用·
	if err = DB.Model(&base.Video{}).Select("visit_count").Where("video_id = ?", vid).Scan(&count).Error; err != nil {
		return 0, err
	}
	return count, err

}

func GetVideoShareCount(ctx context.Context, vid string) (count int64, err error) {
	if err := DB.WithContext(ctx).Model(&model.VideoShare{}).Where("video_id = ?", vid).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, err
}

func GetVideoIdList(ctx context.Context, pageNum, pageSize int64) (*[]string, error) {
	list := make([]string, 0)
	if err := DB.Model(&base.Video{}).Select("video_id").Offset(int(pageNum-1) * int(pageSize)).Limit(int(pageSize)).Scan(&list).Error; err != nil {
		hlog.Info(err)
		return nil, err
	}
	return &list, nil
}

func GetVideoInfo(ctx context.Context, videoId int64) (*base.Video, error) {
	video := new(base.Video)
	var err error
	if err = DB.WithContext(ctx).Model(&base.Video{}).Where("video_id = ?", videoId).Find(video).Error; err != nil {
		return nil, errors.WithMessage(err, "Failed to get VideoInfo")
	}
	return video, nil
}

func CreateFavorite(ctx context.Context, fav *base.Favorite) error {
	if err := DB.WithContext(ctx).Model(&base.Favorite{}).Create(fav).Error; err != nil {
		return errors.WithMessage(err, "Failed to create Favorite")
	}
	return nil
}

// 获取用户收藏列表（有多少个收藏夹）
func GetFavoriteList(ctx context.Context, req *videos.GetFavoriteListRequestV2) ([]*base.Favorite, error) {
	var favList []*base.Favorite
	hlog.Info(req.UserId)
	query := DB.WithContext(ctx).Model(&base.Favorite{}).Where("user_id=?", req.UserId)

	// 添加隐私过滤
	if req.PrivacyFilter != "" {
		query = query.Where("privacy = ?", req.PrivacyFilter)
	}

	if err := query.Offset((int(req.PageNum) - 1) * int(req.PageSize)).Limit(int(req.PageSize)).Find(&favList).Error; err != nil {
		return nil, errors.WithMessage(err, "Failed to get FavoriteList")
	}
	return favList, nil
}

func AddVideoToFavorite(ctx context.Context, fav_vid *model.FavoritesVideos) error {
	if err := DB.WithContext(ctx).Model(&model.FavoritesVideos{}).Create(fav_vid).Error; err != nil {
		return errors.WithMessage(err, "Failed to add VideoToFavorite")
	}
	return nil
}

func GetVideoIdFromFavorite(ctx context.Context, user_id, favorite_id int64) ([]int64, error) {
	var videoIds []int64
	if err := DB.WithContext(ctx).Model(&model.FavoritesVideos{}).Where("user_id = ? and favorite_id = ?", user_id, favorite_id).Select("video_id").Scan(&videoIds).Error; err != nil {
		return videoIds, errors.WithMessage(err, "Failed to get VideoFromList")
	}
	hlog.Info(videoIds)
	return videoIds, nil
}

// 从视频收藏中获取视频列表
func GetFavoriteVideoList(ctx context.Context, req *videos.GetFavoriteVideoListRequestV2) ([]*base.Video, error) {
	var video []*base.Video
	videoIds, err := GetVideoIdFromFavorite(ctx, req.UserId, req.FavoriteId)
	if err != nil {
		return video, err
	}
	if len(videoIds) == 0 {
		return video, nil
	}

	query := DB.WithContext(ctx).Model(&base.Video{}).Where("video_id in?", videoIds)

	// 添加排序
	if req.SortBy != "" {
		query = query.Order(req.SortBy)
	}

	// 添加分页
	query = query.Limit(int(req.PageSize)).Offset(int((req.PageNum - 1) * req.PageSize))

	if err := query.Find(&video).Error; err != nil {
		return video, errors.WithMessage(err, "Failed to get VideoFromList")
	}
	return video, nil
}

func GetVideoFromFavorite(ctx context.Context, userId, videoId int64) (*base.Video, error) {
	var video *base.Video
	if err := DB.WithContext(ctx).Model(&base.Video{}).Where("user_id = ? and video_id = ?", userId, videoId).Find(&video).Error; err != nil {
		return nil, errors.WithMessage(err, "Failed to get VideoFromFavorite")
	}
	return video, nil
}

func DeleteFavorite(ctx context.Context, req *videos.DeleteFavoriteRequestV2) error {
	go DeleteAllVideoFromFavorite(ctx, req.UserId, req.FavoriteId)
	if err := DB.WithContext(ctx).Model(&model.Favorite{}).Where("user_id =? and favorite_id =?", req.UserId, req.FavoriteId).Delete(&base.Favorite{}).Error; err != nil {
		return errors.WithMessage(err, "Failed to delete Favorite")
	}
	return nil
}

func DeleteVideoFromFavorite(ctx context.Context, req *videos.DeleteVideoFromFavoriteRequestV2) error {
	if err := DB.WithContext(ctx).Model(&model.FavoritesVideos{}).Where("user_id =? and video_id =?", req.UserId, req.VideoId).Delete(&model.FavoritesVideos{}).Error; err != nil {
		return errors.WithMessage(err, "Failed to delete VideoFromFavorite")
	}
	return nil
}

func DeleteAllVideoFromFavorite(ctx context.Context, user_id, favorite_id int64) error {
	if err := DB.WithContext(ctx).Model(&model.FavoritesVideos{}).Where("user_id =? and favorite_id =?", user_id, favorite_id).Delete(&model.FavoritesVideos{}).Error; err != nil {
		return errors.WithMessage(err, "Failed to delete VideoFromFavorite")
	}
	return nil
}

func SharedVideo(ctx context.Context, share *model.VideoShare) error {
	if err := DB.WithContext(ctx).Model(&model.VideoShare{}).Create(share).Error; err != nil {
		return errors.WithMessage(err, "Failed to shared Video")
	}
	return nil
}

func AddUserVideoWatchHistory(ctx context.Context, watch *model.UserVideoWatchHistory) error {
	if err := DB.WithContext(ctx).Model(&model.UserVideoWatchHistory{}).Create(watch).Error; err != nil {
		return errors.WithMessage(err, "Failed to add UserVideoWatchHistory")
	}
	return nil
}

func AddUserViewBehavior(ctx context.Context, behavior *model.UserBehavior) error {
	if err := DB.WithContext(ctx).Model(&model.UserBehavior{}).Create(behavior).Error; err != nil {
		return errors.WithMessage(err, "Failed to add UserViewBehavior")
	}
	return nil
}

func AddUserShareBehavior(ctx context.Context, behavior *model.UserBehavior) error {
	if err := DB.WithContext(ctx).Model(&model.UserBehavior{}).Create(behavior).Error; err != nil {
		return errors.WithMessage(err, "Failed to add UserShareBehavior")
	}
	return nil
}

// ========================================
// Video Counter Operations (for high concurrency)
// ========================================

// GetOrCreateVideoCounter gets or creates a video counter record
func GetOrCreateVideoCounter(ctx context.Context, videoId int64) (*model.VideoCounter, error) {
	var counter model.VideoCounter
	err := DB.WithContext(ctx).Where("video_id = ?", videoId).First(&counter).Error
	if err != nil {
		// Create new counter
		counter = model.VideoCounter{
			VideoId: videoId,
		}
		if err := DB.WithContext(ctx).Create(&counter).Error; err != nil {
			return nil, errors.WithMessage(err, "Failed to create VideoCounter")
		}
	}
	return &counter, nil
}

// IncrementVideoCounter increments a specific counter field
func IncrementVideoCounter(ctx context.Context, videoId int64, field string, delta int64) error {
	var updateExpr string
	switch field {
	case "visit_count":
		updateExpr = "visit_count + ?"
	case "like_count":
		updateExpr = "like_count + ?"
	case "comment_count":
		updateExpr = "comment_count + ?"
	case "share_count":
		updateExpr = "share_count + ?"
	case "favorite_count":
		updateExpr = "favorite_count + ?"
	case "download_count":
		updateExpr = "download_count + ?"
	default:
		return errors.Errorf("invalid field: %s", field)
	}

	if err := DB.WithContext(ctx).Model(&model.VideoCounter{}).
		Where("video_id = ?", videoId).
		UpdateColumn(field, DB.Raw(updateExpr, delta)).Error; err != nil {
		return errors.WithMessage(err, "Failed to increment VideoCounter")
	}
	return nil
}

// SyncVideoCountersToMainTable syncs counter values to main video table (batch operation)
func SyncVideoCountersToMainTable(ctx context.Context, videoId int64) error {
	var counter model.VideoCounter
	if err := DB.WithContext(ctx).Where("video_id = ?", videoId).First(&counter).Error; err != nil {
		return errors.WithMessage(err, "Failed to get VideoCounter")
	}

	updates := map[string]interface{}{
		"visit_count":     counter.VisitCount,
		"likes_count":     counter.LikeCount,
		"comment_count":   counter.CommentCount,
		"share_count":     counter.ShareCount,
		"favorites_count": counter.FavoriteCount,
	}

	if err := DB.WithContext(ctx).Model(&base.Video{}).Where("video_id = ?", videoId).Updates(updates).Error; err != nil {
		return errors.WithMessage(err, "Failed to sync VideoCounters")
	}
	return nil
}

// ========================================
// Campus Video Operations
// ========================================

// GetVideosBySchool gets videos by school_id
func GetVideosBySchool(ctx context.Context, schoolId int64, page, pageSize int64) ([]*base.Video, int64, error) {
	db := DB.WithContext(ctx).Model(&base.Video{}).Where("school_id = ? AND deleted_at IS NULL AND audit_status = 1 AND open = 1", schoolId)

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, errors.WithMessage(err, "Failed to count videos by school")
	}

	var videos []*base.Video
	if err := db.Order("created_at DESC").Limit(int(pageSize)).Offset(int(pageSize * (page - 1))).Find(&videos).Error; err != nil {
		return nil, 0, errors.WithMessage(err, "Failed to get videos by school")
	}

	return videos, total, nil
}

// UpdateVideoSchool updates video school_id
func UpdateVideoSchool(ctx context.Context, videoId, schoolId int64) error {
	if err := DB.WithContext(ctx).Model(&base.Video{}).Where("video_id = ?", videoId).Update("school_id", schoolId).Error; err != nil {
		return errors.WithMessage(err, "Failed to update video school")
	}
	return nil
}

// UpdateVideoMetadata updates video metadata fields
func UpdateVideoMetadata(ctx context.Context, videoId int64, duration, width, height uint, fileSize uint64) error {
	updates := map[string]interface{}{
		"duration":  duration,
		"width":     width,
		"height":    height,
		"file_size": fileSize,
	}
	if err := DB.WithContext(ctx).Model(&base.Video{}).Where("video_id = ?", videoId).Updates(updates).Error; err != nil {
		return errors.WithMessage(err, "Failed to update video metadata")
	}
	return nil
}

// UpdateVideoPermissions updates video permission settings
func UpdateVideoPermissions(ctx context.Context, videoId int64, allowComment, allowDuet, allowDownload int8) error {
	updates := map[string]interface{}{
		"allow_comment":  allowComment,
		"allow_duet":     allowDuet,
		"allow_download": allowDownload,
	}
	if err := DB.WithContext(ctx).Model(&base.Video{}).Where("video_id = ?", videoId).Updates(updates).Error; err != nil {
		return errors.WithMessage(err, "Failed to update video permissions")
	}
	return nil
}

// UpdateVideoLocation updates video publish location
func UpdateVideoLocation(ctx context.Context, videoId int64, location string) error {
	if err := DB.WithContext(ctx).Model(&base.Video{}).Where("video_id = ?", videoId).Update("location", location).Error; err != nil {
		return errors.WithMessage(err, "Failed to update video location")
	}
	return nil
}

// UpdateVideoAuditStatus updates video audit status
func UpdateVideoAuditStatus(ctx context.Context, videoId int64, auditStatus int8) error {
	if err := DB.WithContext(ctx).Model(&base.Video{}).Where("video_id = ?", videoId).Update("audit_status", auditStatus).Error; err != nil {
		return errors.WithMessage(err, "Failed to update video audit status")
	}
	return nil
}

// GetPendingAuditVideos gets videos pending audit (for admin)
func GetPendingAuditVideos(ctx context.Context, page, pageSize int64) ([]*base.Video, int64, error) {
	db := DB.WithContext(ctx).Model(&base.Video{}).Where("audit_status = 0 AND deleted_at IS NULL")

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, errors.WithMessage(err, "Failed to count pending audit videos")
	}

	var videos []*base.Video
	if err := db.Order("created_at ASC").Limit(int(pageSize)).Offset(int(pageSize * (page - 1))).Find(&videos).Error; err != nil {
		return nil, 0, errors.WithMessage(err, "Failed to get pending audit videos")
	}

	return videos, total, nil
}

// UpdateFavoriteVideoCount updates the video count in a favorites folder
func UpdateFavoriteVideoCount(ctx context.Context, favoriteId int64, delta int) error {
	if err := DB.WithContext(ctx).Model(&model.Favorite{}).
		Where("favorite_id = ?", favoriteId).
		UpdateColumn("video_count", DB.Raw("video_count + ?", delta)).Error; err != nil {
		return errors.WithMessage(err, "Failed to update favorite video count")
	}
	return nil
}

// GetUserVideoWatchHistory gets user's video watch history
func GetUserVideoWatchHistory(ctx context.Context, userId int64, page, pageSize int64) ([]*model.UserVideoWatchHistory, int64, error) {
	db := DB.WithContext(ctx).Model(&model.UserVideoWatchHistory{}).Where("user_id = ? AND deleted_at IS NULL", userId)

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, errors.WithMessage(err, "Failed to count watch history")
	}

	var history []*model.UserVideoWatchHistory
	if err := db.Order("watch_time DESC").Limit(int(pageSize)).Offset(int(pageSize * (page - 1))).Find(&history).Error; err != nil {
		return nil, 0, errors.WithMessage(err, "Failed to get watch history")
	}

	return history, total, nil
}

// UpdateWatchHistory updates or creates watch history record
func UpdateWatchHistory(ctx context.Context, userId, videoId int64, watchDuration uint, completionRate float64) error {
	var history model.UserVideoWatchHistory
	err := DB.WithContext(ctx).Where("user_id = ? AND video_id = ?", userId, videoId).First(&history).Error

	if err != nil {
		// Create new record
		history = model.UserVideoWatchHistory{
			UserId:         userId,
			VideoId:        videoId,
			WatchDuration:  watchDuration,
			CompletionRate: completionRate,
		}
		return DB.WithContext(ctx).Create(&history).Error
	}

	// Update existing record
	updates := map[string]interface{}{
		"watch_duration":  watchDuration,
		"completion_rate": completionRate,
	}
	return DB.WithContext(ctx).Model(&model.UserVideoWatchHistory{}).Where("user_video_watch_history_id = ?", history.UserVideoWatchHistoryId).Updates(updates).Error
}

// ClearUserWatchHistory clears user's watch history
func ClearUserWatchHistory(ctx context.Context, userId int64) error {
	if err := DB.WithContext(ctx).Where("user_id = ?", userId).Delete(&model.UserVideoWatchHistory{}).Error; err != nil {
		return errors.WithMessage(err, "Failed to clear watch history")
	}
	return nil
}
