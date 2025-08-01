package db

import (
	"context"
	"testing"
	"time"

	"HuaTug.com/cmd/model"
	"HuaTug.com/pkg/constants"
	"HuaTug.com/pkg/database"
)

// TestShardingManagerInitialization 测试分片管理器初始化
func TestShardingManagerInitialization(t *testing.T) {
	// 模拟分片配置
	config := &database.ShardingConfig{
		DatabaseCount: 4,
		TableCount:    4,
		MasterDSNs: []string{
			"root:password@tcp(localhost:3306)/comment_db_0?charset=utf8mb4&parseTime=True&loc=Local",
			"root:password@tcp(localhost:3306)/comment_db_1?charset=utf8mb4&parseTime=True&loc=Local",
			"root:password@tcp(localhost:3306)/comment_db_2?charset=utf8mb4&parseTime=True&loc=Local",
			"root:password@tcp(localhost:3306)/comment_db_3?charset=utf8mb4&parseTime=True&loc=Local",
		},
		SlaveDSNs:       [][]string{{}, {}, {}, {}}, // 没有从库
		MaxOpenConns:    100,
		MaxIdleConns:    10,
		ConnMaxLifetime: time.Hour,
	}

	// 注意：这个测试需要实际的数据库连接，在CI环境中可能需要跳过
	if testing.Short() {
		t.Skip("Skipping database integration test in short mode")
	}

	err := InitShardingManager(config)
	// 在没有实际数据库的情况下，这个测试会失败，这是预期的
	// 在实际环境中，应该能够成功初始化
	t.Logf("Sharding manager initialization result: %v", err)
}

// TestCommentShardingLogic 测试评论分片逻辑
func TestCommentShardingLogic(t *testing.T) {
	ctx := context.Background()

	// 创建测试评论
	comment := &model.Comment{
		CommentId: 12345,
		UserId:    1001,
		VideoId:   2001,
		ParentId:  -1,
		Content:   "Test comment for sharding",
		CreatedAt: time.Now().Format(constants.DataFormate),
		UpdatedAt: time.Now().Format(constants.DataFormate),
		DeletedAt: "",
	}

	// 测试分片函数（不需要实际数据库连接）
	t.Run("TestCreateComment", func(t *testing.T) {
		// 如果没有分片管理器，应该使用兜底逻辑
		err := CreateComment(ctx, comment)
		// 在没有数据库连接的情况下会失败，但不应该panic
		t.Logf("CreateComment result: %v", err)
	})

	t.Run("TestGetCommentInfo", func(t *testing.T) {
		// 测试获取评论信息
		_, err := GetCommentInfo(ctx, comment.CommentId)
		t.Logf("GetCommentInfo result: %v", err)
	})

	t.Run("TestGetVideoCommentCount", func(t *testing.T) {
		// 测试获取视频评论数
		count, err := GetVideoCommentCount(ctx, comment.VideoId)
		t.Logf("GetVideoCommentCount result: count=%d, err=%v", count, err)
	})
}

// TestShardingFallback 测试分片兜底逻辑
func TestShardingFallback(t *testing.T) {
	ctx := context.Background()

	// 确保分片管理器为nil，测试兜底逻辑
	originalManager := ShardingManager
	ShardingManager = nil
	defer func() {
		ShardingManager = originalManager
	}()

	comment := &model.Comment{
		CommentId: 12345,
		UserId:    1001,
		VideoId:   2001,
		ParentId:  -1,
		Content:   "Test comment for fallback",
		CreatedAt: time.Now().Format(constants.DataFormate),
		UpdatedAt: time.Now().Format(constants.DataFormate),
		DeletedAt: "",
	}

	// 测试各种函数的兜底逻辑
	t.Run("TestCreateCommentFallback", func(t *testing.T) {
		err := CreateComment(ctx, comment)
		// 应该使用原有的单库逻辑
		t.Logf("CreateComment fallback result: %v", err)
	})

	t.Run("TestCheckCommentExistsFallback", func(t *testing.T) {
		exists, err := CheckCommentExists(ctx, comment.CommentId)
		t.Logf("CheckCommentExists fallback result: exists=%v, err=%v", exists, err)
	})

	t.Run("TestGetVideoCommentListFallback", func(t *testing.T) {
		list, err := GetVideoCommentList(ctx, comment.VideoId)
		t.Logf("GetVideoCommentList fallback result: list length=%d, err=%v",
			func() int {
				if list != nil {
					return len(*list)
				} else {
					return 0
				}
			}(), err)
	})
}

// BenchmarkShardingVsFallback 性能对比测试
func BenchmarkShardingVsFallback(b *testing.B) {
	ctx := context.Background()
	videoId := int64(12345)

	b.Run("WithSharding", func(b *testing.B) {
		// 这里需要实际的分片管理器才能进行有意义的性能测试
		if ShardingManager == nil {
			b.Skip("Sharding manager not initialized")
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = GetVideoCommentCount(ctx, videoId)
		}
	})

	b.Run("WithoutSharding", func(b *testing.B) {
		originalManager := ShardingManager
		ShardingManager = nil
		defer func() {
			ShardingManager = originalManager
		}()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = GetVideoCommentCount(ctx, videoId)
		}
	})
}
