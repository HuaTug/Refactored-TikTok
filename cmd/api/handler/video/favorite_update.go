package handlers

import (
	"context"

	"HuaTug.com/cmd/api/dal"
	"HuaTug.com/cmd/video/service"
	jwt "HuaTug.com/pkg/auth"
	"HuaTug.com/pkg/errno"
	"HuaTug.com/pkg/utils"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/hlog"
)

// UpdateFavoriteParam 更新收藏夹的请求参数
type UpdateFavoriteParam struct {
	FavoriteId  int64  `form:"favorite_id" json:"favorite_id"`
	Title       string `form:"title" json:"title"`
	Name        string `form:"name" json:"name"`
	Description string `form:"description" json:"description"`
	CoverUrl    string `form:"cover_url" json:"cover_url"`
	ShowStatus  string `form:"show_status" json:"show_status"` // "0" = public, "1" = private
}

// UpdateFavorite 更新收藏夹信息（V1 API）
func UpdateFavorite(ctx context.Context, c *app.RequestContext) {
	var req UpdateFavoriteParam
	var err error
	var v interface{}
	var userId int64

	// 绑定请求参数
	if err = c.Bind(&req); err != nil {
		hlog.Error("UpdateFavorite Bind error:", err)
		SendResponse(c, errno.ParamErr, nil)
		return
	}

	// 获取用户ID
	if v, err = jwt.ConvertJWTPayloadToString(ctx, c); err != nil {
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}
	userId = utils.Transfer(v)

	// 验证收藏夹ID
	if req.FavoriteId <= 0 {
		hlog.Error("UpdateFavorite: FavoriteId is required")
		SendResponse(c, errno.ParamErr, nil)
		return
	}

	// 获取收藏夹名字（兼容 title 和 name 字段）
	name := req.Name
	if name == "" {
		name = req.Title
	}

	// 转换公开状态
	privacy := "private"
	if req.ShowStatus == "0" {
		privacy = "public"
	}

	// 调用 service 更新
	favService := service.NewVideoFavoritesService(ctx)
	err = favService.UpdateFavorite(&service.UpdateFavoriteParams{
		FavoriteId:  req.FavoriteId,
		UserId:      userId,
		Name:        name,
		Description: req.Description,
		CoverUrl:    req.CoverUrl,
		Privacy:     privacy,
	})

	if err != nil {
		hlog.Errorf("UpdateFavorite error: %v", err)
		SendResponse(c, errno.ServiceErr, nil)
		return
	}

	SendResponse(c, errno.Success, true)
}

// SyncFavoriteVideoCount 同步收藏夹视频数量
func SyncFavoriteVideoCount(ctx context.Context, c *app.RequestContext) {
	var err error
	var v interface{}
	var userId int64

	// 获取用户ID
	if v, err = jwt.ConvertJWTPayloadToString(ctx, c); err != nil {
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}
	userId = utils.Transfer(v)

	// 获取收藏夹ID
	favoriteIdStr := c.Query("favorite_id")
	favoriteId := utils.Transfer(favoriteIdStr)

	if favoriteId <= 0 {
		SendResponse(c, errno.ParamErr, nil)
		return
	}

	// 验证收藏夹归属
	favService := service.NewVideoFavoritesService(ctx)
	_, err = favService.GetFavoriteById(favoriteId, userId)
	if err != nil {
		hlog.Errorf("SyncFavoriteVideoCount: favorite not found or not owned by user: %v", err)
		SendResponse(c, errno.ParamErr, nil)
		return
	}

	// 同步视频数量
	err = favService.SyncFavoriteVideoCount(favoriteId)
	if err != nil {
		hlog.Errorf("SyncFavoriteVideoCount error: %v", err)
		SendResponse(c, errno.ServiceErr, nil)
		return
	}

	SendResponse(c, errno.Success, true)
}

// SyncVideoFavoritesCount 同步单个视频的收藏数量
func SyncVideoFavoritesCount(ctx context.Context, c *app.RequestContext) {
	// 获取视频ID
	videoIdStr := c.Query("video_id")
	videoId := utils.Transfer(videoIdStr)

	if videoId <= 0 {
		SendResponse(c, errno.ParamErr, nil)
		return
	}

	// 直接使用 dal.DB 进行同步
	if dal.DB == nil {
		hlog.Error("SyncVideoFavoritesCount: database not initialized")
		SendResponse(c, errno.ServiceErr, nil)
		return
	}

	var count int64
	if err := dal.DB.WithContext(ctx).Table("favorites_videos").
		Where("video_id = ?", videoId).
		Count(&count).Error; err != nil {
		hlog.Errorf("SyncVideoFavoritesCount count error: %v", err)
		SendResponse(c, errno.ServiceErr, nil)
		return
	}

	if err := dal.DB.WithContext(ctx).Table("videos").
		Where("video_id = ?", videoId).
		Update("favorites_count", count).Error; err != nil {
		hlog.Errorf("SyncVideoFavoritesCount update error: %v", err)
		SendResponse(c, errno.ServiceErr, nil)
		return
	}

	hlog.Infof("Synced video %d favorites count to %d", videoId, count)

	SendResponse(c, errno.Success, map[string]interface{}{
		"video_id":        videoId,
		"favorites_count": count,
	})
}

// SyncAllVideosFavoritesCount 同步所有视频的收藏数量（管理员功能）
func SyncAllVideosFavoritesCount(ctx context.Context, c *app.RequestContext) {
	// 直接使用 dal.DB 进行同步
	if dal.DB == nil {
		hlog.Error("SyncAllVideosFavoritesCount: database not initialized")
		SendResponse(c, errno.ServiceErr, nil)
		return
	}

	// 获取所有有收藏记录的视频ID
	var videoIds []int64
	if err := dal.DB.WithContext(ctx).Table("favorites_videos").
		Distinct("video_id").
		Pluck("video_id", &videoIds).Error; err != nil {
		hlog.Errorf("SyncAllVideosFavoritesCount get video ids error: %v", err)
		SendResponse(c, errno.ServiceErr, nil)
		return
	}

	// 同步每个视频的收藏数量
	syncedCount := 0
	for _, videoId := range videoIds {
		var count int64
		if err := dal.DB.WithContext(ctx).Table("favorites_videos").
			Where("video_id = ?", videoId).
			Count(&count).Error; err != nil {
			hlog.Warnf("Failed to count favorites for video %d: %v", videoId, err)
			continue
		}

		if err := dal.DB.WithContext(ctx).Table("videos").
			Where("video_id = ?", videoId).
			Update("favorites_count", count).Error; err != nil {
			hlog.Warnf("Failed to sync favorites count for video %d: %v", videoId, err)
			continue
		}
		syncedCount++
	}

	// 将没有收藏记录的视频的收藏数重置为0
	if len(videoIds) > 0 {
		if err := dal.DB.WithContext(ctx).Table("videos").
			Where("video_id NOT IN ?", videoIds).
			Where("favorites_count > 0").
			Update("favorites_count", 0).Error; err != nil {
			hlog.Warnf("Failed to reset favorites count for videos without favorites: %v", err)
		}
	} else {
		// 没有任何收藏记录，重置所有视频的收藏数
		if err := dal.DB.WithContext(ctx).Table("videos").
			Where("favorites_count > 0").
			Update("favorites_count", 0).Error; err != nil {
			hlog.Warnf("Failed to reset all videos favorites count: %v", err)
		}
	}

	hlog.Infof("Synced all videos favorites count, total videos: %d, synced: %d", len(videoIds), syncedCount)

	SendResponse(c, errno.Success, map[string]interface{}{
		"total_videos": len(videoIds),
		"synced_count": syncedCount,
	})
}
