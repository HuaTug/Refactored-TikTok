package service

import (
	"context"
	"fmt"

	"HuaTug.com/cmd/model"
	"HuaTug.com/cmd/relation/dal/db"
	"HuaTug.com/kitex_gen/relations"
	"HuaTug.com/pkg/errno"
)

type RelationService struct {
	ctx      context.Context
	shardeDB *db.ShardedFollowDB
}

func NewRelationService(ctx context.Context, shardeDB *db.ShardedFollowDB) *RelationService {
	return &RelationService{
		ctx:      ctx,
		shardeDB: shardeDB,
	}
}

func (s *RelationService) RelationService(ctx context.Context, req *relations.RelationServiceRequest) error {
	if req.FromUserId == req.ToUserId {
		return errno.RequestErr
	}

	switch req.ActionType {
	case 1: // 关注
		return s.CreateFollow(ctx, req)
	case 2: // 取消关注
		return s.CancelFollow(ctx, req)
	default:
		return errno.ParamErr
	}
}

func (s *RelationService) CreateFollow(ctx context.Context, req *relations.RelationServiceRequest) error {
	// 检查是否已关注
	exists, err := s.shardeDB.IsFollowing(ctx, req.FromUserId, req.ToUserId)
	if err != nil {
		return fmt.Errorf("failed to check following status: %w", errno.ServiceErr)
	}
	if exists {
		return errno.RequestErr // 已关注
	}

	if err := s.shardeDB.InsertFollowWithTransaction(ctx, &model.FollowRelation{
		UserID:     req.ToUserId,
		FollowerID: req.FromUserId,
	}); err != nil {
		return fmt.Errorf("failed to create follow: %w", errno.ServiceErr)
	}
	return nil
}

func (s *RelationService) CancelFollow(ctx context.Context, req *relations.RelationServiceRequest) error {
	// 检查是否已关注
	exists, err := s.shardeDB.IsFollowing(ctx, req.FromUserId, req.ToUserId)
	if err != nil {
		return fmt.Errorf("failed to check following status: %w", errno.ServiceErr)
	}
	if !exists {
		return errno.RequestErr // 未关注
	}

	if err := s.shardeDB.DeleteFollow(ctx, req.FromUserId, req.ToUserId); err != nil {
		return fmt.Errorf("failed to delete follow: %w", errno.ServiceErr)
	}
	return nil
}
