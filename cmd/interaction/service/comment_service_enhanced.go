package service

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"HuaTug.com/cmd/api/rpc"
	"HuaTug.com/cmd/interaction/dal/db"
	"HuaTug.com/cmd/interaction/infras/redis"
	"HuaTug.com/cmd/model"
	"HuaTug.com/kitex_gen/base"
	"HuaTug.com/kitex_gen/interactions"
	"HuaTug.com/kitex_gen/videos"
	"HuaTug.com/pkg/errno"
	"HuaTug.com/pkg/utils"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/pkg/errors"
)

// =============================================================================
// 抖音级评论服务 - 增强版架构设计
// =============================================================================
//
// 核心特性：
// 1. 层级评论（楼中楼）+ 扁平化展示
// 2. 热门评论排序算法
// 3. 敏感词过滤 + 垃圾评论检测
// 4. 防刷机制 + 频率限制
// 5. Redis缓存 + 异步持久化
//
// 评论结构设计：
// ┌─────────────────────────────────────────────────────────────────────────┐
// │                         视频评论区                                       │
// ├─────────────────────────────────────────────────────────────────────────┤
// │  一级评论 (parent_id = -1)                                              │
// │  ├── 内容：这个视频太棒了！                                              │
// │  ├── 点赞数：1234                                                       │
// │  ├── 回复数：56                                                         │
// │  └── 子评论 (扁平化展示)                                                 │
// │      ├── 回复 @用户A：同意！ (reply_to_comment_id = 父评论)              │
// │      ├── 回复 @用户B：+1 (reply_to_comment_id = 上一条)                  │
// │      └── ...                                                            │
// └─────────────────────────────────────────────────────────────────────────┘
//
// =============================================================================

// 评论验证常量
const (
	EnhancedMaxCommentLength    = 500 // 最大评论长度
	EnhancedMinCommentLength    = 1   // 最小评论长度
	EnhancedCommentRateLimit    = 10  // 每分钟最大评论数
	EnhancedDuplicateTimeWindow = 300 // 重复检测时间窗口（秒）
	EnhancedMaxReplyDepth       = 2   // 最大回复层级（扁平化）
)

// EnhancedCommentService 增强版评论服务
type EnhancedCommentService struct {
	ctx            context.Context
	interactionMgr *redis.EnhancedInteractionManager
}

// NewEnhancedCommentService 创建增强版评论服务
func NewEnhancedCommentService(ctx context.Context) *EnhancedCommentService {
	return &EnhancedCommentService{
		ctx:            ctx,
		interactionMgr: redis.NewEnhancedInteractionManager(redis.RedisDBInteraction),
	}
}

// =============================================================================
// 创建评论
// =============================================================================

// CreateComment 创建评论（带限流和防重复）
func (s *EnhancedCommentService) CreateComment(ctx context.Context, req *interactions.CreateCommentRequest) error {
	userID := req.UserId

	// 1. 内容验证
	if err := s.validateCommentContent(req.Content); err != nil {
		return err
	}

	if req.CommentId == 0 && req.VideoId == 0 {
		return errno.RequestErr.WithMessage("Either VideoId or CommentId must be provided")
	}

	// 2. 限流检查
	rateLimitResult, err := s.interactionMgr.CheckUserCommentRateLimit(ctx, userID)
	if err != nil {
		hlog.CtxWarnf(ctx, "Rate limit check failed: %v", err)
		// 不阻止用户，继续执行
	} else if !rateLimitResult.Allowed {
		return errno.RequestErr.WithMessage(fmt.Sprintf("评论太频繁，请稍后再试 (剩余等待时间: %ds)",
			(rateLimitResult.RetryAt-time.Now().UnixMilli())/1000))
	}

	// 3. 重复评论检查
	if err := s.checkDuplicateComment(userID, req.Content); err != nil {
		return err
	}

	// 4. 敏感词过滤
	content := s.filterSensitiveWords(req.Content)

	// 5. 处理评论层级关系
	parentID := int64(-1)
	videoID := req.VideoId
	replyToCommentID := int64(0)

	if req.CommentId != 0 {
		// 回复其他评论
		parentCommentID, err := db.GetParentCommentId(ctx, req.CommentId)
		if err != nil {
			return errors.WithMessage(err, "Failed to get parent comment info")
		}

		replyToCommentID = req.CommentId

		// 扁平化处理：所有回复都挂在一级评论下
		if req.Mode != 0 && parentCommentID != 0 {
			parentID = parentCommentID
		} else {
			parentID = req.CommentId
		}

		// 获取视频ID
		if videoID == 0 {
			videoID, err = db.GetCommentVideoId(ctx, req.CommentId)
			if err != nil {
				return errors.WithMessage(err, "Failed to get video ID")
			}
		}
	}

	// 6. 验证视频是否存在
	if videoID != 0 {
		_, err := rpc.VideoClient.VideoInfoV2(ctx, &videos.VideoInfoRequestV2{VideoId: videoID})
		if err != nil {
			return errors.WithMessage(err, "Video not found")
		}
	}

	// 7. 生成评论ID并创建评论
	commentID := utils.GenerateCommentID()
	comment := &model.Comment{
		CommentId:        commentID,
		VideoId:          videoID,
		ParentId:         parentID,
		UserId:           userID,
		Content:          strings.TrimSpace(content),
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
		ReplyToCommentId: replyToCommentID,
	}

	// 8. 使用事务写入数据库
	if err := db.CreateCommentWithTransaction(ctx, comment); err != nil {
		return errors.WithMessage(err, "Failed to create comment")
	}

	// 9. 异步更新缓存和统计
	go s.postCommentActions(ctx, userID, videoID, commentID, req.Content)

	hlog.CtxInfof(ctx, "Comment created: user_id=%d, video_id=%d, comment_id=%d", userID, videoID, commentID)
	return nil
}

// postCommentActions 评论后的异步操作
func (s *EnhancedCommentService) postCommentActions(ctx context.Context, userID, videoID, commentID int64, content string) {
	// 更新评论频率限制计数
	key := fmt.Sprintf("comment_rate_limit:%d", userID)
	if err := redis.IncrementCommentRateLimit(key, 60); err != nil {
		hlog.CtxWarnf(ctx, "Failed to update rate limit: %v", err)
	}

	// 存储评论哈希（用于重复检测）
	if err := redis.StoreCommentHash(userID, content, EnhancedDuplicateTimeWindow); err != nil {
		hlog.CtxWarnf(ctx, "Failed to store comment hash: %v", err)
	}

	// 更新评论计数缓存
	s.updateCommentCountCache(ctx, videoID)

	// 记录用户行为
	s.interactionMgr.TrackUserActivity(ctx, userID, "comment")
}

// =============================================================================
// 内容验证和过滤
// =============================================================================

// validateCommentContent 验证评论内容
func (s *EnhancedCommentService) validateCommentContent(content string) error {
	trimmed := strings.TrimSpace(content)

	if trimmed == "" {
		return errno.RequestErr.WithMessage("评论内容不能为空")
	}

	length := utf8.RuneCountInString(trimmed)
	if length < EnhancedMinCommentLength {
		return errno.RequestErr.WithMessage("评论内容太短")
	}
	if length > EnhancedMaxCommentLength {
		return errno.RequestErr.WithMessage(fmt.Sprintf("评论内容不能超过%d个字符", EnhancedMaxCommentLength))
	}

	// 垃圾内容检测
	if s.isSpamContent(trimmed) {
		return errno.RequestErr.WithMessage("评论内容包含不当内容")
	}

	return nil
}

// isSpamContent 垃圾内容检测
func (s *EnhancedCommentService) isSpamContent(content string) bool {
	lowerContent := strings.ToLower(content)

	// 检测重复字符
	if s.hasExcessiveRepetition(lowerContent) {
		return true
	}

	// 检测垃圾关键词
	spamKeywords := []string{
		"广告", "推广", "加微信", "加qq", "联系方式",
		"spam", "advertisement",
	}
	for _, keyword := range spamKeywords {
		if strings.Contains(lowerContent, keyword) {
			return true
		}
	}

	return false
}

// hasExcessiveRepetition 检测过度重复
func (s *EnhancedCommentService) hasExcessiveRepetition(content string) bool {
	if len(content) < 10 {
		return false
	}

	var prevChar rune
	var count int
	for _, char := range content {
		if char == prevChar {
			count++
			if count > 5 {
				return true
			}
		} else {
			count = 1
			prevChar = char
		}
	}
	return false
}

// filterSensitiveWords 敏感词过滤
func (s *EnhancedCommentService) filterSensitiveWords(content string) string {
	// TODO: 集成敏感词库进行过滤
	// 目前简单实现，实际应使用DFA或AC自动机
	return content
}

// checkDuplicateComment 检查重复评论
func (s *EnhancedCommentService) checkDuplicateComment(userID int64, content string) error {
	isDuplicate, err := redis.CheckDuplicateComment(userID, content, EnhancedDuplicateTimeWindow)
	if err != nil {
		hlog.Warnf("Duplicate check failed: %v", err)
		return nil // 检查失败不阻止用户
	}
	if isDuplicate {
		return errno.RequestErr.WithMessage("请勿重复发送相同内容")
	}
	return nil
}

// =============================================================================
// 评论列表查询
// =============================================================================

// ListComment 获取评论列表
func (s *EnhancedCommentService) ListComment(ctx context.Context, req *interactions.ListCommentRequest) (*interactions.ListCommentResponse, error) {
	if req.PageNum <= 0 {
		req.PageNum = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 20
	}
	if req.SortType == "" {
		req.SortType = "hot"
	}

	var comments []*base.Comment
	var err error

	if req.VideoId != 0 {
		comments, err = s.getVideoComments(ctx, req)
	} else if req.CommentId != 0 {
		comments, err = s.getReplyComments(ctx, req)
	} else {
		return nil, errno.RequestErr.WithMessage("VideoId or CommentId required")
	}

	if err != nil {
		return nil, err
	}

	return &interactions.ListCommentResponse{
		Base:  &base.Status{Code: 0, Msg: "success"},
		Items: comments,
	}, nil
}

// getVideoComments 获取视频的一级评论
func (s *EnhancedCommentService) getVideoComments(ctx context.Context, req *interactions.ListCommentRequest) ([]*base.Comment, error) {
	var commentIDs []int64
	var err error

	switch req.SortType {
	case "hot":
		// 热门排序：按热度算法排序
		commentIDs, err = s.getHotComments(ctx, req.VideoId, req.PageNum, req.PageSize)
	case "latest":
		// 最新排序：按时间倒序
		list, err := db.GetVideoCommentListByPartWithSort(ctx, req.VideoId, req.PageNum, req.PageSize, "latest")
		if err != nil {
			return nil, err
		}
		commentIDs = *list
	default:
		commentIDs, err = s.getHotComments(ctx, req.VideoId, req.PageNum, req.PageSize)
	}

	if err != nil {
		return nil, err
	}

	// 批量构建评论数据
	return s.batchBuildComments(ctx, commentIDs)
}

// getReplyComments 获取评论的回复列表
func (s *EnhancedCommentService) getReplyComments(ctx context.Context, req *interactions.ListCommentRequest) ([]*base.Comment, error) {
	list, err := db.GetCommentChildListByPart(ctx, req.CommentId, req.PageNum, req.PageSize)
	if err != nil {
		return nil, err
	}
	return s.batchBuildComments(ctx, *list)
}

// getHotComments 获取热门评论
func (s *EnhancedCommentService) getHotComments(ctx context.Context, videoID int64, pageNum, pageSize int64) ([]int64, error) {
	// 先获取足够多的评论用于排序
	multiplier := int64(3)
	list, err := db.GetVideoCommentListForHotSort(ctx, videoID, 1, pageNum*pageSize*multiplier)
	if err != nil {
		return nil, err
	}

	if len(*list) == 0 {
		return []int64{}, nil
	}

	// 计算热度分数并排序
	type commentScore struct {
		CommentID int64
		Score     float64
	}

	scores := make([]commentScore, 0, len(*list))

	for _, commentID := range *list {
		score, err := s.calculateHotScore(ctx, commentID)
		if err != nil {
			continue
		}
		scores = append(scores, commentScore{CommentID: commentID, Score: score})
	}

	// 按分数降序排序
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Score > scores[j].Score
	})

	// 分页
	start := int((pageNum - 1) * pageSize)
	end := int(pageNum * pageSize)
	if start >= len(scores) {
		return []int64{}, nil
	}
	if end > len(scores) {
		end = len(scores)
	}

	result := make([]int64, 0, end-start)
	for i := start; i < end; i++ {
		result = append(result, scores[i].CommentID)
	}
	return result, nil
}

// calculateHotScore 计算评论热度分数
// 算法：score = like_count * 10 + reply_count * 5 + time_factor
func (s *EnhancedCommentService) calculateHotScore(ctx context.Context, commentID int64) (float64, error) {
	// 获取点赞数
	likeCount, _ := redis.GetCommentLikeCount(commentID)

	// 获取回复数
	replyCount, _ := db.GetChildCommentCount(ctx, commentID)

	// 获取评论信息（用于计算时间因子）
	commentInfo, err := db.GetCommentInfo(ctx, commentID)
	if err != nil {
		return 0, err
	}

	// 时间因子：24小时内衰减
	hoursSinceCreation := time.Since(commentInfo.CreatedAt).Hours()
	timeFactor := 100.0 * math.Pow(0.5, hoursSinceCreation/24.0)

	// 综合评分
	score := float64(likeCount)*10.0 + float64(replyCount)*5.0 + timeFactor

	return score, nil
}

// =============================================================================
// 批量数据构建
// =============================================================================

// batchBuildComments 批量构建评论数据
func (s *EnhancedCommentService) batchBuildComments(ctx context.Context, commentIDs []int64) ([]*base.Comment, error) {
	if len(commentIDs) == 0 {
		return []*base.Comment{}, nil
	}

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		results = make([]*base.Comment, 0, len(commentIDs))
	)

	// 并发构建，限制并发数
	semaphore := make(chan struct{}, 10)

	for _, commentID := range commentIDs {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(cid int64) {
			defer wg.Done()
			defer func() { <-semaphore }()

			comment, err := s.buildCommentData(ctx, cid)
			if err != nil {
				hlog.CtxWarnf(ctx, "Failed to build comment data: comment_id=%d, err=%v", cid, err)
				return
			}

			mu.Lock()
			results = append(results, comment)
			mu.Unlock()
		}(commentID)
	}

	wg.Wait()
	return results, nil
}

// buildCommentData 构建单个评论数据
func (s *EnhancedCommentService) buildCommentData(ctx context.Context, commentID int64) (*base.Comment, error) {
	var (
		wg          sync.WaitGroup
		errChan     = make(chan error, 3)
		commentInfo *model.Comment
		likeCount   int64
		childCount  int64
	)

	wg.Add(3)

	// 获取评论基本信息
	go func() {
		defer wg.Done()
		var err error
		commentInfo, err = db.GetCommentInfo(ctx, commentID)
		if err != nil {
			errChan <- err
		}
	}()

	// 获取点赞数
	go func() {
		defer wg.Done()
		var err error
		likeCount, err = redis.GetCommentLikeCount(commentID)
		if err != nil {
			// 从DB回源
			likeCount = 0
		}
	}()

	// 获取子评论数
	go func() {
		defer wg.Done()
		var err error
		childCount, err = db.GetChildCommentCount(ctx, commentID)
		if err != nil {
			childCount = 0
		}
	}()

	wg.Wait()

	// 检查错误
	select {
	case err := <-errChan:
		return nil, err
	default:
	}

	if commentInfo == nil {
		return nil, fmt.Errorf("comment not found: %d", commentID)
	}

	return &base.Comment{
		CommentId:        commentInfo.CommentId,
		VideoId:          commentInfo.VideoId,
		UserId:           commentInfo.UserId,
		ParentId:         commentInfo.ParentId,
		LikeCount:        likeCount,
		ChildCount:       childCount,
		Content:          commentInfo.Content,
		CreatedAt:        commentInfo.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:        commentInfo.UpdatedAt.Format("2006-01-02 15:04:05"),
		ReplyToCommentId: commentInfo.ReplyToCommentId,
	}, nil
}

// =============================================================================
// 缓存更新
// =============================================================================

// updateCommentCountCache 更新评论计数缓存
func (s *EnhancedCommentService) updateCommentCountCache(ctx context.Context, videoID int64) {
	// 可以在这里更新视频的评论计数缓存
	// 实际实现中应该使用原子操作
}

// =============================================================================
// 删除评论
// =============================================================================

// DeleteComment 删除评论
func (s *EnhancedCommentService) DeleteComment(ctx context.Context, req *interactions.CommentDeleteRequest) error {
	// 验证权限
	commentInfo, err := db.GetCommentInfo(ctx, req.CommentId)
	if err != nil {
		return errno.MysqlErr
	}
	if commentInfo.UserId != req.FromUserId {
		// 检查是否是视频作者
		if req.VideoId != 0 {
			videoInfo, err := rpc.VideoClient.VideoInfoV2(ctx, &videos.VideoInfoRequestV2{VideoId: req.VideoId})
			if err != nil || videoInfo.Items.UserId != req.FromUserId {
				return errno.ServiceErr.WithMessage("无权删除此评论")
			}
		} else {
			return errno.ServiceErr.WithMessage("无权删除此评论")
		}
	}

	// 删除评论
	if err := db.DeleteComment(ctx, req.CommentId); err != nil {
		return errno.ServiceErr
	}

	// 异步清理缓存
	go redis.DeleteCommentAndAllAbout(req.CommentId)

	hlog.CtxInfof(ctx, "Comment deleted: comment_id=%d, by_user=%d", req.CommentId, req.FromUserId)
	return nil
}
