package service

import (
	"context"
	"time"

	"HuaTug.com/internal/model"
	"HuaTug.com/cmd/video/dal/db"
	"HuaTug.com/kitex_gen/base"
	"HuaTug.com/kitex_gen/videos"

	"github.com/pkg/errors"
)

type VideoFavoritesService struct {
	ctx context.Context
}

func NewVideoFavoritesService(ctx context.Context) *VideoFavoritesService {
	return &VideoFavoritesService{
		ctx: ctx,
	}
}

func (s *VideoFavoritesService) CreateFavorite(req *videos.CreateFavoriteRequestV2) error {
	// 设置默认隐私状态
	var isPublic int8 = 0 // 默认私密
	if req.Privacy == "public" {
		isPublic = 1
	}

	now := time.Now()
	if err := db.CreateFavoriteModel(s.ctx, &model.Favorite{
		UserId:      req.UserId,
		Name:        req.Name,
		Description: req.Description,
		CoverUrl:    req.CoverUrl,
		IsPublic:    isPublic,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		return err
	}
	return nil
}

func (s *VideoFavoritesService) GetFavoriteList(req *videos.GetFavoriteListRequestV2) ([]*base.Favorite, error) {
	var favList []*base.Favorite
	favList, err := db.GetFavoriteList(s.ctx, req)
	if err != nil {
		return favList, errors.WithMessage(err, "Failed to get FavoriteList")
	}

	// 为没有封面的收藏夹自动抽取第一个收藏视频的封面
	for _, fav := range favList {
		if fav.CoverUrl == "" && fav.VideoCount > 0 {
			coverUrl, err := db.GetFirstVideoCoverByFavoriteId(s.ctx, fav.FavoriteId)
			if err == nil && coverUrl != "" {
				fav.CoverUrl = coverUrl
			}
		}
	}

	return favList, nil
}

func (s *VideoFavoritesService) GetFavoriteVideoList(req *videos.GetFavoriteVideoListRequestV2) ([]*base.Video, error) {
	favoriteId := req.FavoriteId

	// 如果没有指定收藏夹，使用默认收藏夹
	if favoriteId <= 0 {
		defaultFav, err := db.GetOrCreateDefaultFavorite(s.ctx, req.UserId)
		if err != nil {
			return nil, errors.WithMessage(err, "Failed to get default favorite")
		}
		favoriteId = defaultFav.FavoriteId
	}

	// 创建新请求使用实际的 favoriteId
	newReq := &videos.GetFavoriteVideoListRequestV2{
		UserId:     req.UserId,
		FavoriteId: favoriteId,
		PageNum:    req.PageNum,
		PageSize:   req.PageSize,
		SortBy:     req.SortBy,
	}

	var video []*base.Video
	video, err := db.GetFavoriteVideoList(s.ctx, newReq)
	if err != nil {
		return video, errors.WithMessage(err, "Failed to get FavoriteVideoList")
	}
	return video, nil
}

// AddFavoriteVideo adds a video to a favorite folder
// Returns (alreadyExists bool, error) - alreadyExists is true if video was already in the favorite (idempotent operation)
func (s *VideoFavoritesService) AddFavoriteVideo(req *videos.AddFavoriteVideoRequestV2) (bool, error) {
	favoriteId := req.FavoriteId

	// 如果没有指定收藏夹，使用默认收藏夹
	if favoriteId <= 0 {
		defaultFav, err := db.GetOrCreateDefaultFavorite(s.ctx, req.UserId)
		if err != nil {
			return false, errors.WithMessage(err, "Failed to get or create default favorite")
		}
		favoriteId = defaultFav.FavoriteId
	}

	// 检查视频是否已存在于收藏夹
	exists, err := db.CheckVideoInFavorite(s.ctx, req.UserId, favoriteId, req.VideoId)
	if err != nil {
		return false, errors.WithMessage(err, "Failed to check video in favorite")
	}
	if exists {
		// 视频已存在，返回成功（幂等操作），但标记已存在
		return true, nil
	}

	if err := db.AddVideoToFavorite(s.ctx, &model.FavoritesVideos{
		UserId:     req.UserId,
		FavoriteId: favoriteId,
		VideoId:    req.VideoId,
	}); err != nil {
		return false, errors.WithMessage(err, "Failed to AddFavoriteVideo")
	}
	return false, nil
}

// CheckVideoInFavorite checks if a video is already in a favorite folder
func (s *VideoFavoritesService) CheckVideoInFavorite(userId, favoriteId, videoId int64) (bool, error) {
	return db.CheckVideoInFavorite(s.ctx, userId, favoriteId, videoId)
}

// 在删除收藏夹的同时 删除视频收藏夹中的视频
func (s *VideoFavoritesService) DeleteFavorite(req *videos.DeleteFavoriteRequestV2) error {
	if err := db.DeleteFavorite(s.ctx, req); err != nil {
		return errors.WithMessage(err, "Failed to DeleteFavorite")
	}
	return nil
}

func (s *VideoFavoritesService) DeleteVideoFromFavorite(req *videos.DeleteVideoFromFavoriteRequestV2) error {
	if err := db.DeleteVideoFromFavorite(s.ctx, req); err != nil {
		return errors.WithMessage(err, "Failed to DeleteFavorite")
	}
	return nil
}

// UpdateFavoriteParams 更新收藏夹的参数
type UpdateFavoriteParams struct {
	FavoriteId  int64
	UserId      int64
	Name        string
	Description string
	CoverUrl    string
	Privacy     string // "public" or "private"
}

// UpdateFavorite 更新收藏夹信息
func (s *VideoFavoritesService) UpdateFavorite(params *UpdateFavoriteParams) error {
	updates := make(map[string]interface{})

	if params.Name != "" {
		updates["name"] = params.Name
	}
	if params.Description != "" {
		updates["description"] = params.Description
	}
	if params.CoverUrl != "" {
		updates["cover_url"] = params.CoverUrl
	}
	// 处理公开状态
	if params.Privacy != "" {
		if params.Privacy == "public" {
			updates["is_public"] = int8(1)
		} else {
			updates["is_public"] = int8(0)
		}
	}

	if len(updates) == 0 {
		return errors.New("no fields to update")
	}

	return db.UpdateFavorite(s.ctx, params.FavoriteId, params.UserId, updates)
}

// SyncFavoriteVideoCount 同步收藏夹视频数量
func (s *VideoFavoritesService) SyncFavoriteVideoCount(favoriteId int64) error {
	return db.SyncFavoriteVideoCount(s.ctx, favoriteId)
}

// GetFavoriteById 获取收藏夹详情
func (s *VideoFavoritesService) GetFavoriteById(favoriteId, userId int64) (*model.Favorite, error) {
	return db.GetFavoriteById(s.ctx, favoriteId, userId)
}

// SyncVideoFavoritesCount 同步视频的收藏数量
func (s *VideoFavoritesService) SyncVideoFavoritesCount(videoId int64) (int64, error) {
	return db.SyncVideoFavoritesCount(s.ctx, videoId)
}

// SyncAllVideosFavoritesCount 同步所有视频的收藏数量
func (s *VideoFavoritesService) SyncAllVideosFavoritesCount() error {
	return db.SyncAllVideosFavoritesCount(s.ctx)
}
