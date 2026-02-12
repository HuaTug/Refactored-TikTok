package db

import (
	"context"
	stdErrors "errors"
	"fmt"
	"sync"
	"time"

	"HuaTug.com/cmd/model"
	"HuaTug.com/cmd/video/infras/redis"
	"HuaTug.com/kitex_gen/base"
	"HuaTug.com/kitex_gen/videos"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
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
	if err := query.Limit(int(req.PageSize)).Offset(int((req.PageNum - 1) * req.PageSize)).Find(&video).Error; err != nil {
		return video, errors.Wrapf(err, "FeedList failed,err:%v", err)
	}
	return video, nil
}

func GetAllFeedList(ctx context.Context, req *videos.VideoFeedListRequestV2) ([]*base.Video, error) {
	var video []*base.Video
	if err := DB.WithContext(ctx).Model(&base.Video{}).Find(&video).Error; err != nil {
		return video, errors.Wrapf(err, "GetAllFeedList failed,err:%v", err)
	}
	return video, nil
}

// 获取用户发布的视频
func Videolist(ctx context.Context, req *videos.VideoFeedListRequestV2) ([]*base.Video, int64, error) {
	var video []*base.Video
	var count int64
	if err := DB.WithContext(ctx).Model(&base.Video{}).Where("user_id=?", req.UserId).Count(&count).Limit(int(req.PageSize)).
		Offset(int((req.PageNum - 1) * req.PageSize)).Find(&video).Error; err != nil {
		logrus.Info(err)
		return video, count, errors.Wrapf(err, "VideoList failed,err:%v", err)
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
	video = &base.Video{}
	result := DB.WithContext(ctx).Model(&base.Video{}).Where("video_id=?", videoId).First(video)
	if result.Error != nil {
		if stdErrors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, errors.Wrapf(result.Error, "FindVideo: video %d not found", videoId)
		}
		return nil, errors.Wrapf(result.Error, "FindVideo failed,err:%v", result.Error)
	}
	return video, nil
}

func InsertVideo(ctx context.Context, video *base.Video) error {
	if err := DB.WithContext(ctx).Omit("deleted_at").Create(video).Error; err != nil {
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

// UpdateVideoCoverUrl 单独更新视频封面URL
func UpdateVideoCoverUrl(ctx context.Context, videoId int64, coverUrl string) error {
	if err := DB.WithContext(ctx).Model(&base.Video{}).Where("video_id = ?", videoId).Update("cover_url", coverUrl).Error; err != nil {
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

// CreateFavoriteModel 使用 model.Favorite 创建收藏夹
func CreateFavoriteModel(ctx context.Context, fav *model.Favorite) error {
	if err := DB.WithContext(ctx).Create(fav).Error; err != nil {
		return errors.WithMessage(err, "Failed to create Favorite")
	}
	return nil
}

// GetOrCreateDefaultFavorite 获取或创建用户的默认收藏夹
// 使用 FirstOrCreate 保证原子性，避免并发创建重复收藏夹
func GetOrCreateDefaultFavorite(ctx context.Context, userId int64) (*model.Favorite, error) {
	now := time.Now()
	fav := model.Favorite{
		UserId:      userId,
		Name:        "默认收藏夹",
		Description: "系统自动创建的默认收藏夹",
		IsPublic:    0, // 私密
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// 使用 FirstOrCreate 原子操作，根据 user_id 和 name 查找或创建
	result := DB.WithContext(ctx).
		Where("user_id = ? AND name = ?", userId, "默认收藏夹").
		FirstOrCreate(&fav)

	if result.Error != nil {
		return nil, errors.WithMessage(result.Error, "Failed to get or create default favorite")
	}

	// RowsAffected > 0 表示新创建了记录
	if result.RowsAffected > 0 {
		hlog.Infof("Created default favorite for user %d, favorite_id=%d", userId, fav.FavoriteId)
	}

	return &fav, nil
}

// UpdateFavorite 更新收藏夹信息（名称、描述、封面、公开状态等）
func UpdateFavorite(ctx context.Context, favoriteId, userId int64, updates map[string]interface{}) error {
	updates["updated_at"] = time.Now()
	result := DB.WithContext(ctx).Model(&model.Favorite{}).
		Where("favorite_id = ? AND user_id = ?", favoriteId, userId).
		Updates(updates)
	if result.Error != nil {
		return errors.WithMessage(result.Error, "Failed to update favorite")
	}
	if result.RowsAffected == 0 {
		return errors.New("favorite not found or not owned by user")
	}
	return nil
}

// SyncFavoriteVideoCount 同步收藏夹视频数量（根据实际收藏记录计算）
func SyncFavoriteVideoCount(ctx context.Context, favoriteId int64) error {
	var count int64
	if err := DB.WithContext(ctx).Model(&model.FavoritesVideos{}).
		Where("favorite_id = ?", favoriteId).
		Count(&count).Error; err != nil {
		return errors.WithMessage(err, "Failed to count favorite videos")
	}

	if err := DB.WithContext(ctx).Model(&model.Favorite{}).
		Where("favorite_id = ?", favoriteId).
		Update("video_count", count).Error; err != nil {
		return errors.WithMessage(err, "Failed to sync favorite video count")
	}

	hlog.Infof("Synced favorite %d video count to %d", favoriteId, count)
	return nil
}

// GetFavoriteById 根据ID获取收藏夹
func GetFavoriteById(ctx context.Context, favoriteId, userId int64) (*model.Favorite, error) {
	var fav model.Favorite
	err := DB.WithContext(ctx).
		Where("favorite_id = ? AND user_id = ?", favoriteId, userId).
		First(&fav).Error
	if err != nil {
		if stdErrors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("favorite not found")
		}
		return nil, errors.WithMessage(err, "Failed to get favorite")
	}
	return &fav, nil
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
	// 检查是否已经收藏过
	var count int64
	if err := DB.WithContext(ctx).Model(&model.FavoritesVideos{}).
		Where("user_id = ? AND favorite_id = ? AND video_id = ?", fav_vid.UserId, fav_vid.FavoriteId, fav_vid.VideoId).
		Count(&count).Error; err != nil {
		return errors.WithMessage(err, "Failed to check existing favorite")
	}
	if count > 0 {
		return errors.New("video already in favorite")
	}

	// 添加收藏
	if err := DB.WithContext(ctx).Model(&model.FavoritesVideos{}).Create(fav_vid).Error; err != nil {
		return errors.WithMessage(err, "Failed to add VideoToFavorite")
	}

	// 更新收藏夹视频数量
	if err := UpdateFavoriteVideoCount(ctx, fav_vid.FavoriteId, 1); err != nil {
		hlog.Warnf("Failed to update favorite video count: %v", err)
	}

	// 更新视频主表的收藏数 (favorites_count 字段)
	if err := DB.WithContext(ctx).Model(&model.Video{}).
		Where("video_id = ?", fav_vid.VideoId).
		UpdateColumn("favorites_count", gorm.Expr("favorites_count + ?", 1)).Error; err != nil {
		hlog.Warnf("Failed to increment video favorites count: %v", err)
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
	videoIds, err := GetVideoIdFromFavorite(ctx, req.UserId, req.FavoriteId)
	if err != nil {
		return nil, err
	}
	if len(videoIds) == 0 {
		return []*base.Video{}, nil
	}

	// 使用 model.Video 进行查询
	var modelVideos []*model.Video
	query := DB.WithContext(ctx).Table("videos").Where("video_id IN ?", videoIds)

	// 添加排序
	if req.SortBy != "" {
		query = query.Order(req.SortBy)
	}

	// 添加分页
	if req.PageSize > 0 {
		query = query.Limit(int(req.PageSize)).Offset(int((req.PageNum - 1) * req.PageSize))
	}

	if err := query.Find(&modelVideos).Error; err != nil {
		return nil, errors.WithMessage(err, "Failed to get VideoFromList")
	}

	// 转换为 base.Video
	result := make([]*base.Video, 0, len(modelVideos))
	for _, v := range modelVideos {
		result = append(result, &base.Video{
			VideoId:        v.VideoId,
			UserId:         v.UserId,
			VideoUrl:       v.VideoUrl,
			CoverUrl:       v.CoverUrl,
			Title:          v.Title,
			Description:    v.Description,
			VisitCount:     int64(v.VisitCount),
			LikesCount:     int64(v.LikesCount),
			CommentCount:   int64(v.CommentCount),
			FavoritesCount: int64(v.FavoritesCount),
			CreatedAt:      v.CreatedAt.Format("2006-01-02 15:04:05"),
			UpdatedAt:      v.UpdatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	return result, nil
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
	hlog.Infof("DeleteVideoFromFavorite: userId=%d, videoId=%d, favoriteId=%d", req.UserId, req.VideoId, req.FavoriteId)
	
	// 如果 favorite_id 为 0，则从用户所有收藏夹中删除该视频
	if req.FavoriteId == 0 {
		// 先查询视频在哪些收藏夹中
		var records []model.FavoritesVideos
		if err := DB.WithContext(ctx).Model(&model.FavoritesVideos{}).
			Where("user_id = ? AND video_id = ?", req.UserId, req.VideoId).
			Find(&records).Error; err != nil {
			return errors.WithMessage(err, "Failed to find video in favorites")
		}

		hlog.Infof("DeleteVideoFromFavorite: found %d records for userId=%d, videoId=%d", len(records), req.UserId, req.VideoId)

		if len(records) == 0 {
			// 视频本来就不在收藏夹中，返回成功（幂等操作）
			hlog.Infof("DeleteVideoFromFavorite: video %d not found in any favorite for user %d, returning success (idempotent)", req.VideoId, req.UserId)
			return nil
		}

		// 删除所有收藏记录
		if err := DB.WithContext(ctx).Model(&model.FavoritesVideos{}).
			Where("user_id = ? AND video_id = ?", req.UserId, req.VideoId).
			Delete(&model.FavoritesVideos{}).Error; err != nil {
			return errors.WithMessage(err, "Failed to delete VideoFromFavorite")
		}

		// 更新每个收藏夹的视频数量
		for _, record := range records {
			if err := UpdateFavoriteVideoCount(ctx, record.FavoriteId, -1); err != nil {
				hlog.Warnf("Failed to update favorite video count for favorite_id %d: %v", record.FavoriteId, err)
			}
		}

		// 更新视频主表的收藏数
		if err := DB.WithContext(ctx).Model(&model.Video{}).
			Where("video_id = ? AND favorites_count > 0", req.VideoId).
			UpdateColumn("favorites_count", gorm.Expr("favorites_count - ?", len(records))).Error; err != nil {
			hlog.Warnf("Failed to decrement video favorites count: %v", err)
		}

		return nil
	}

	// 指定了 favorite_id，则只从该收藏夹中删除
	// 先检查记录是否存在
	var count int64
	if err := DB.WithContext(ctx).Model(&model.FavoritesVideos{}).
		Where("user_id = ? AND favorite_id = ? AND video_id = ?", req.UserId, req.FavoriteId, req.VideoId).
		Count(&count).Error; err != nil {
		return errors.WithMessage(err, "Failed to check favorite record")
	}
	if count == 0 {
		// 视频本来就不在指定收藏夹中，返回成功（幂等操作）
		hlog.Infof("DeleteVideoFromFavorite: video %d not found in favorite %d for user %d, returning success (idempotent)", req.VideoId, req.FavoriteId, req.UserId)
		return nil
	}

	// 删除收藏记录
	if err := DB.WithContext(ctx).Model(&model.FavoritesVideos{}).
		Where("user_id = ? AND favorite_id = ? AND video_id = ?", req.UserId, req.FavoriteId, req.VideoId).
		Delete(&model.FavoritesVideos{}).Error; err != nil {
		return errors.WithMessage(err, "Failed to delete VideoFromFavorite")
	}

	// 更新收藏夹视频数量
	if err := UpdateFavoriteVideoCount(ctx, req.FavoriteId, -1); err != nil {
		hlog.Warnf("Failed to update favorite video count: %v", err)
	}

	// 更新视频主表的收藏数 (favorites_count 字段)
	if err := DB.WithContext(ctx).Model(&model.Video{}).
		Where("video_id = ? AND favorites_count > 0", req.VideoId).
		UpdateColumn("favorites_count", gorm.Expr("favorites_count - ?", 1)).Error; err != nil {
		hlog.Warnf("Failed to decrement video favorites count: %v", err)
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

// SyncVideoFavoritesCount 同步视频的收藏数量（根据实际收藏记录计算）
func SyncVideoFavoritesCount(ctx context.Context, videoId int64) (int64, error) {
	var count int64
	if err := DB.WithContext(ctx).Model(&model.FavoritesVideos{}).
		Where("video_id = ?", videoId).
		Count(&count).Error; err != nil {
		return 0, errors.WithMessage(err, "Failed to count video favorites")
	}

	if err := DB.WithContext(ctx).Model(&model.Video{}).
		Where("video_id = ?", videoId).
		Update("favorites_count", count).Error; err != nil {
		return 0, errors.WithMessage(err, "Failed to sync video favorites count")
	}

	hlog.Infof("Synced video %d favorites count to %d", videoId, count)
	return count, nil
}

// SyncAllVideosFavoritesCount 同步所有视频的收藏数量
func SyncAllVideosFavoritesCount(ctx context.Context) error {
	// 获取所有有收藏记录的视频ID
	var videoIds []int64
	if err := DB.WithContext(ctx).Model(&model.FavoritesVideos{}).
		Distinct("video_id").
		Pluck("video_id", &videoIds).Error; err != nil {
		return errors.WithMessage(err, "Failed to get video ids")
	}

	// 同步每个视频的收藏数量
	for _, videoId := range videoIds {
		var count int64
		if err := DB.WithContext(ctx).Model(&model.FavoritesVideos{}).
			Where("video_id = ?", videoId).
			Count(&count).Error; err != nil {
			hlog.Warnf("Failed to count favorites for video %d: %v", videoId, err)
			continue
		}

		if err := DB.WithContext(ctx).Model(&model.Video{}).
			Where("video_id = ?", videoId).
			Update("favorites_count", count).Error; err != nil {
			hlog.Warnf("Failed to sync favorites count for video %d: %v", videoId, err)
			continue
		}
	}

	// 将没有收藏记录的视频的收藏数重置为0
	if err := DB.WithContext(ctx).Model(&model.Video{}).
		Where("video_id NOT IN ?", videoIds).
		Where("favorites_count > 0").
		Update("favorites_count", 0).Error; err != nil {
		hlog.Warnf("Failed to reset favorites count for videos without favorites: %v", err)
	}

	hlog.Infof("Synced all videos favorites count, total videos: %d", len(videoIds))
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

// ========================================
// Video Visit/View Operations
// ========================================

// IncrementVisitCount increments video visit count with Redis cache
func IncrementVisitCount(ctx context.Context, videoId, userId int64) error {
	// 1. 先更新Redis缓存（快速响应）
	if _, err := redis.IncrementVideoVisitCount(ctx, videoId); err != nil {
		hlog.Warnf("Failed to increment visit count in Redis: %v, falling back to DB", err)
	}

	// 2. 更新Redis观看历史缓存
	if userId > 0 {
		if err := redis.AddToWatchHistory(ctx, userId, videoId); err != nil {
			hlog.Warnf("Failed to add to watch history cache: %v", err)
		}
	}

	// 3. 异步更新数据库（使用goroutine避免阻塞）
	go func() {
		// 使用计数器表进行原子增加
		if err := IncrementVideoCounter(context.Background(), videoId, "visit_count", 1); err != nil {
			hlog.Warnf("Failed to increment visit count in counter table: %v", err)
		}

		// 同时更新主表的浏览量
		if err := DB.Model(&base.Video{}).
			Where("video_id = ?", videoId).
			UpdateColumn("visit_count", DB.Raw("visit_count + 1")).Error; err != nil {
			hlog.Warnf("Failed to update main video visit count: %v", err)
		}
	}()

	return nil
}

// GetVideoVisitCountById gets video visit count by video id (with Redis cache)
func GetVideoVisitCountById(ctx context.Context, videoId int64) (uint64, error) {
	// 1. 先从Redis获取
	count, found, err := redis.GetVideoVisitCountCached(ctx, videoId)
	if err == nil && found {
		return uint64(count), nil
	}

	// 2. Redis没有，从数据库获取
	var dbCount uint64
	// 优先从计数器表获取
	var counter model.VideoCounter
	err = DB.WithContext(ctx).Where("video_id = ?", videoId).First(&counter).Error
	if err == nil {
		dbCount = counter.VisitCount
	} else {
		// 回退到主表
		if err := DB.WithContext(ctx).Model(&base.Video{}).
			Select("visit_count").
			Where("video_id = ?", videoId).
			Scan(&dbCount).Error; err != nil {
			return 0, errors.WithMessage(err, "Failed to get video visit count")
		}
	}

	// 3. 写入Redis缓存
	if err := redis.SetVideoVisitCount(ctx, videoId, int64(dbCount)); err != nil {
		hlog.Warnf("Failed to set visit count to Redis: %v", err)
	}

	return dbCount, nil
}

// GetVideoDetailedCounts gets all count metrics for a video
func GetVideoDetailedCounts(ctx context.Context, videoId int64) (map[string]int64, error) {
	counts := make(map[string]int64)

	var counter model.VideoCounter
	err := DB.WithContext(ctx).Where("video_id = ?", videoId).First(&counter).Error
	if err == nil {
		counts["visit_count"] = int64(counter.VisitCount)
		counts["like_count"] = int64(counter.LikeCount)
		counts["comment_count"] = int64(counter.CommentCount)
		counts["share_count"] = int64(counter.ShareCount)
		counts["favorite_count"] = int64(counter.FavoriteCount)
		counts["download_count"] = int64(counter.DownloadCount)
		return counts, nil
	}

	// 回退到主表
	var video base.Video
	if err := DB.WithContext(ctx).Model(&base.Video{}).
		Select("visit_count, likes_count, comment_count, share_count, favorites_count").
		Where("video_id = ?", videoId).
		First(&video).Error; err != nil {
		return nil, errors.WithMessage(err, "Failed to get video counts")
	}

	counts["visit_count"] = video.VisitCount
	counts["like_count"] = video.LikesCount
	counts["comment_count"] = video.CommentCount
	counts["share_count"] = video.ShareCount
	counts["favorite_count"] = video.FavoritesCount
	return counts, nil
}

// ========================================
// Watch History Extended Operations
// ========================================

// AddOrUpdateWatchHistory adds or updates watch history with Redis cache
func AddOrUpdateWatchHistory(ctx context.Context, userId, videoId int64, watchDuration uint, completionRate float64) (bool, error) {
	// 1. 先更新Redis缓存
	if err := redis.AddToWatchHistory(ctx, userId, videoId); err != nil {
		hlog.Warnf("Failed to add to watch history cache: %v", err)
	}

	// 2. 检查数据库中是否已存在
	var history model.UserVideoWatchHistory
	err := DB.WithContext(ctx).Where("user_id = ? AND video_id = ?", userId, videoId).First(&history).Error

	isNew := err != nil
	if isNew {
		// 新记录：增加浏览量（已经在IncrementVisitCount中处理Redis）
		if err := IncrementVisitCount(ctx, videoId, userId); err != nil {
			hlog.Warnf("Failed to increment visit count: %v", err)
		}

		// 创建新的观看历史（数据库）
		history = model.UserVideoWatchHistory{
			UserId:         userId,
			VideoId:        videoId,
			WatchDuration:  watchDuration,
			CompletionRate: completionRate,
			WatchTime:      time.Now(),
		}
		if err := DB.WithContext(ctx).Create(&history).Error; err != nil {
			return false, errors.WithMessage(err, "Failed to create watch history")
		}
	} else {
		// 更新现有记录 - 只更新观看时间（使用Go本地时间，避免数据库时区问题）
		updates := map[string]interface{}{
			"watch_time": time.Now(),
		}
		// 只有当传入了有效的观看时长时才更新这些字段（避免浏览量记录覆盖已有的观看数据）
		if watchDuration > 0 {
			updates["watch_duration"] = watchDuration
			updates["completion_rate"] = completionRate
		}
		if err := DB.WithContext(ctx).Model(&model.UserVideoWatchHistory{}).
			Where("user_video_watch_history_id = ?", history.UserVideoWatchHistoryId).
			Updates(updates).Error; err != nil {
			return false, errors.WithMessage(err, "Failed to update watch history")
		}
	}

	return isNew, nil
}

// DeleteWatchHistoryItem deletes a specific watch history item (with Redis cache)
func DeleteWatchHistoryItem(ctx context.Context, userId, videoId int64) error {
	// 1. 删除Redis缓存
	if err := redis.RemoveFromWatchHistory(ctx, userId, videoId); err != nil {
		hlog.Warnf("Failed to remove from watch history cache: %v", err)
	}

	// 2. 删除数据库记录
	result := DB.WithContext(ctx).
		Where("user_id = ? AND video_id = ?", userId, videoId).
		Delete(&model.UserVideoWatchHistory{})
	if result.Error != nil {
		return errors.WithMessage(result.Error, "Failed to delete watch history item")
	}
	if result.RowsAffected == 0 {
		return errors.New("watch history item not found")
	}
	return nil
}

// GetWatchHistoryWithVideos gets watch history with video details
func GetWatchHistoryWithVideos(ctx context.Context, userId int64, page, pageSize int64, dateFilter string) ([]*model.UserVideoWatchHistory, []*base.Video, int64, error) {
	db := DB.WithContext(ctx).Model(&model.UserVideoWatchHistory{}).Where("user_id = ? AND deleted_at IS NULL", userId)

	// 添加日期过滤
	switch dateFilter {
	case "today":
		db = db.Where("DATE(watch_time) = CURDATE()")
	case "week":
		db = db.Where("watch_time >= DATE_SUB(CURDATE(), INTERVAL 7 DAY)")
	case "month":
		db = db.Where("watch_time >= DATE_SUB(CURDATE(), INTERVAL 30 DAY)")
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, nil, 0, errors.WithMessage(err, "Failed to count watch history")
	}

	var history []*model.UserVideoWatchHistory
	if err := db.Order("watch_time DESC").
		Limit(int(pageSize)).
		Offset(int((page - 1) * pageSize)).
		Find(&history).Error; err != nil {
		return nil, nil, 0, errors.WithMessage(err, "Failed to get watch history")
	}

	if len(history) == 0 {
		return history, nil, total, nil
	}

	// 获取视频详情
	videoIds := make([]int64, len(history))
	for i, h := range history {
		videoIds[i] = h.VideoId
	}

	videos, err := GetVideoByVideoId(ctx, videoIds)
	if err != nil {
		hlog.Warnf("Failed to get video details: %v", err)
		return history, nil, total, nil
	}

	return history, videos, total, nil
}

// ClearUserWatchHistoryByDate clears user's watch history by date range (with Redis cache)
func ClearUserWatchHistoryByDate(ctx context.Context, userId int64, dateRange string) (int64, error) {
	// 1. 清除Redis缓存（如果是清空全部）
	if dateRange == "all" || dateRange == "" {
		if err := redis.ClearWatchHistoryCache(ctx, userId); err != nil {
			hlog.Warnf("Failed to clear watch history cache: %v", err)
		}
	}

	// 2. 清除数据库记录
	db := DB.WithContext(ctx).Where("user_id = ?", userId)

	switch dateRange {
	case "today":
		db = db.Where("DATE(watch_time) = CURDATE()")
	case "week":
		db = db.Where("watch_time >= DATE_SUB(CURDATE(), INTERVAL 7 DAY)")
	case "month":
		db = db.Where("watch_time >= DATE_SUB(CURDATE(), INTERVAL 30 DAY)")
	// "all" or default: no date filter
	}

	result := db.Delete(&model.UserVideoWatchHistory{})
	if result.Error != nil {
		return 0, errors.WithMessage(result.Error, "Failed to clear watch history")
	}
	return result.RowsAffected, nil
}

// CheckVideoInFavorite checks if video is already in a specific favorite
func CheckVideoInFavorite(ctx context.Context, userId, favoriteId, videoId int64) (bool, error) {
	var count int64
	if err := DB.WithContext(ctx).Model(&model.FavoritesVideos{}).
		Where("user_id = ? AND favorite_id = ? AND video_id = ?", userId, favoriteId, videoId).
		Count(&count).Error; err != nil {
		return false, errors.WithMessage(err, "Failed to check video in favorite")
	}
	return count > 0, nil
}

// BatchCheckUserFavorites 批量检查用户是否收藏了指定视频
func BatchCheckUserFavorites(ctx context.Context, userId int64, videoIds []int64) (map[int64]bool, error) {
	result := make(map[int64]bool)
	if len(videoIds) == 0 {
		return result, nil
	}

	// 初始化所有 videoId 为 false
	for _, vid := range videoIds {
		result[vid] = false
	}

	// 查询用户收藏的视频
	var favoriteRecords []model.FavoritesVideos
	if err := DB.WithContext(ctx).Model(&model.FavoritesVideos{}).
		Where("user_id = ? AND video_id IN ?", userId, videoIds).
		Find(&favoriteRecords).Error; err != nil {
		return result, errors.WithMessage(err, "Failed to batch check user favorites")
	}

	// 标记已收藏的视频
	for _, record := range favoriteRecords {
		result[record.VideoId] = true
	}

	return result, nil
}

// GetHotVideosByLikes 获取热门视频排行榜（按点赞数排序）
func GetHotVideosByLikes(ctx context.Context, limit int) ([]model.Video, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	var videos []model.Video
	err := DB.WithContext(ctx).
		Where("deleted_at IS NULL").
		Order("likes_count DESC, visit_count DESC, created_at DESC").
		Limit(limit).
		Find(&videos).Error
	if err != nil {
		return nil, errors.WithMessage(err, "Failed to get hot videos by likes")
	}
	return videos, nil
}
