package service

import (
	"context"
	"time"

	"HuaTug.com/cmd/model"
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
// Returns error if video already exists in the favorite
func (s *VideoFavoritesService) AddFavoriteVideo(req *videos.AddFavoriteVideoRequestV2) error {
	favoriteId := req.FavoriteId

	// 如果没有指定收藏夹，使用默认收藏夹
	if favoriteId <= 0 {
		defaultFav, err := db.GetOrCreateDefaultFavorite(s.ctx, req.UserId)
		if err != nil {
			return errors.WithMessage(err, "Failed to get or create default favorite")
		}
		favoriteId = defaultFav.FavoriteId
	}

	// 检查视频是否已存在于收藏夹
	exists, err := db.CheckVideoInFavorite(s.ctx, req.UserId, favoriteId, req.VideoId)
	if err != nil {
		return errors.WithMessage(err, "Failed to check video in favorite")
	}
	if exists {
		return errors.New("video already exists in this favorite")
	}

	if err := db.AddVideoToFavorite(s.ctx, &model.FavoritesVideos{
		UserId:     req.UserId,
		FavoriteId: favoriteId,
		VideoId:    req.VideoId,
	}); err != nil {
		return errors.WithMessage(err, "Failed to AddFavoriteVideo")
	}
	return nil
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
