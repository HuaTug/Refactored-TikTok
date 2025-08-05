package service

import (
	"context"
	"fmt"

	"HuaTug.com/cmd/relation/dal/db"
	"HuaTug.com/cmd/relation/infras"
	"HuaTug.com/kitex_gen/base"
	"HuaTug.com/kitex_gen/relations"
	"HuaTug.com/kitex_gen/users"
	"HuaTug.com/pkg/constants"
	"HuaTug.com/pkg/errno"
	"github.com/cloudwego/hertz/pkg/common/hlog"
)

type FollowingListService struct {
	ctx      context.Context
	shardeDB *db.ShardedFollowDB
}

func NewFollowingListService(ctx context.Context, shardeDB *db.ShardedFollowDB) *FollowingListService {
	return &FollowingListService{
		ctx:      ctx,
		shardeDB: shardeDB,
	}
}

func (s *FollowingListService) FollowingList(ctx context.Context, req *relations.FollowingListRequest) (*relations.FollowingListResponse, error) {
	resp := &relations.FollowingListResponse{
		Items: make([]*base.UserLite, 0),
	}

	// 参数验证
	if req.PageNum <= 0 {
		req.PageNum = 1
	}
	if req.PageSize <= 0 || req.PageSize > 100 {
		req.PageSize = constants.DefaultLimit
	}

	// 获取关注列表
	offset := int((req.PageNum - 1) * req.PageSize)
	limit := int(req.PageSize)

	userlist, err := s.shardeDB.GetFollowingList(ctx, req.UserId, offset, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get following list: %w", errno.ServiceErr)
	}

	var userIds []int64
	for _, v := range userlist {
		userIds = append(userIds, v.UserID)
	}

	hlog.Info("userIds:", userIds)
	// 批量获取用户信息
	if len(userIds) > 0 {
		userInfos, err := s.batchGetUserInfo(ctx, userIds)
		if err != nil {
			return nil, fmt.Errorf("failed to get user info: %w", errno.ServiceErr)
		}
		resp.Items = userInfos
	}

	hlog.Info("userInfos:", resp.Items)

	// // 获取总数
	// total, err := s.shardeDB.GetFollowingCount(ctx, req.UserId)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to get following count: %w", errno.ServiceErr)
	// }
	resp.Total = int64(len(userlist))
	return resp, nil
}

// batchGetUserInfo 批量获取用户信息
func (s *FollowingListService) batchGetUserInfo(ctx context.Context, userIds []int64) ([]*base.UserLite, error) {
	if len(userIds) == 0 {
		return []*base.UserLite{}, nil
	}

	// 使用现有的GetUserInfo进行批量查询
	result := make([]*base.UserLite, 0, len(userIds))
	for _, userId := range userIds {
		userInfo, err := infras.UserClient.GetUserInfo(ctx, &users.GetUserInfoRequest{UserId: userId})
		if err != nil {
			continue // 跳过失败的用户
		}
		if userInfo.User != nil {
			result = append(result, &base.UserLite{
				Uid:       userInfo.User.UserId,
				UserName:  userInfo.User.UserName,
				AvatarUrl: userInfo.User.AvatarUrl,
			})
		}
	}

	return result, nil
}
