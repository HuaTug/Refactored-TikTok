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
	"HuaTug.com/pkg/constants"
	"HuaTug.com/pkg/errno"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/pkg/errors"
)

// Comment validation constants
const (
	MaxCommentLength    = 500 // Maximum comment length in characters
	MinCommentLength    = 1   // Minimum comment length
	CommentRateLimit    = 10  // Max comments per minute per user
	DuplicateTimeWindow = 300 // Seconds to check for duplicate comments
)

type CommentService struct {
	ctx context.Context
}

func NewCommentService(ctx context.Context) *CommentService {
	return &CommentService{ctx: ctx}
}

// validateCommentContent validates comment content with comprehensive checks
func (service *CommentService) validateCommentContent(content string) error {

	// Check if content is empty
	if strings.TrimSpace(content) == "" {
		return errno.RequestErr.WithMessage("Comment content cannot be empty")
	}

	// Check content length
	contentLength := utf8.RuneCountInString(content)
	if contentLength < MinCommentLength {
		return errno.RequestErr.WithMessage("Comment too short")
	}
	if contentLength > MaxCommentLength {
		return errno.RequestErr.WithMessage("Comment too long, maximum 500 characters allowed")
	}

	// Check for spam patterns (basic implementation)
	if service.isSpamContent(content) {
		return errno.RequestErr.WithMessage("Comment contains inappropriate content")
	}

	return nil
}

// isSpamContent performs basic spam detection
func (service *CommentService) isSpamContent(content string) bool {
	// Convert to lowercase for case-insensitive matching
	lowerContent := strings.ToLower(content)

	// Check for excessive repetition
	if service.hasExcessiveRepetition(lowerContent) {
		return true
	}

	// Check for spam keywords (basic implementation)
	spamKeywords := []string{"spam", "advertisement", "promotion"}
	for _, keyword := range spamKeywords {
		if strings.Contains(lowerContent, keyword) {
			return true
		}
	}

	return false
}

// hasExcessiveRepetition checks for repeated characters or patterns
func (service *CommentService) hasExcessiveRepetition(content string) bool {
	if len(content) < 10 {
		return false
	}

	// Check for repeated characters (more than 5 consecutive same characters)
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

// checkRateLimit validates user comment frequency
func (service *CommentService) checkRateLimit(userId int64) error {
	// Use Redis to implement rate limiting
	key := fmt.Sprintf("comment_rate_limit:%d", userId)
	count, err := redis.GetCommentRateLimit(key)
	if err != nil {
		hlog.Warnf("Failed to check rate limit for user %d: %v", userId, err)
		// Continue execution if Redis fails, don't block user
	} else if count >= CommentRateLimit {
		return errno.RequestErr.WithMessage("Comment rate limit exceeded, please try again later")
	}

	return nil
}

// checkDuplicateComment prevents duplicate comments
func (service *CommentService) checkDuplicateComment(userId int64, content string) error {
	// Check for duplicate content in recent time window
	isDuplicate, err := redis.CheckDuplicateComment(userId, content, DuplicateTimeWindow)
	if err != nil {
		hlog.Warnf("Failed to check duplicate comment for user %d: %v", userId, err)
		// Continue execution if Redis fails
		return nil
	}

	if isDuplicate {
		return errno.RequestErr.WithMessage("Duplicate comment detected, please wait before posting similar content")
	}

	return nil
}

func (service *CommentService) CreateComment(ctx context.Context, req *interactions.CreateCommentRequest) (err error) {
	uid := req.UserId

	// Enhanced input validation
	if err := service.validateCommentContent(req.Content); err != nil {
		return err
	}

	if req.CommentId == 0 && req.VideoId == 0 {
		return errno.RequestErr.WithMessage("Either VideoId or CommentId must be provided")
	}

	// Anti-spam checks
	// 通过redis设置key和过期时间来限制用户评论频率 采用管道满足原子执行的一致性
	if err := service.checkRateLimit(uid); err != nil {
		return err
	}

	if err := service.checkDuplicateComment(uid, req.Content); err != nil {
		return err
	}

	// Enhanced comment hierarchy logic with reply_to_comment_id support
	parentId := int64(-1) // Default for root comments
	videoId := req.VideoId
	replyToCommentId := int64(0) // Default: not replying to any specific comment

	if req.CommentId != 0 {
		// This is a reply to another comment
		parentCommentId, err := db.GetParentCommentId(service.ctx, req.CommentId)
		if err != nil {
			return errors.WithMessage(err, "Failed to get parent comment information")
		}

		// Set the actual reply target
		replyToCommentId = req.CommentId

		// Determine the parent for flattened structure
		if req.Mode != 0 && parentCommentId != 0 {
			// 扁平化模式：强制设为根评论，但保留实际回复目标
			parentId = parentCommentId
		} else {
			// 传统模式：直接回复指定评论
			parentId = req.CommentId
		}

		// Get video ID if not provided
		if videoId == 0 {
			videoId, err = db.GetCommentVideoId(service.ctx, req.CommentId)
			if err != nil {
				return errors.WithMessage(err, "Failed to get video ID from comment")
			}
		}
	}

	// Verify video exists (optional but recommended)
	if videoId != 0 {
		_, err := rpc.VideoClient.VideoInfo(ctx, &videos.VideoInfoRequest{VideoId: videoId})
		if err != nil {
			return errors.WithMessage(err, "Video not found or inaccessible")
		}
	}

	// Create comment with enhanced structure
	comment := &model.Comment{
		VideoId:          videoId,
		ParentId:         parentId,
		UserId:           uid,
		Content:          strings.TrimSpace(req.Content),
		CreatedAt:        time.Now().Format(constants.DataFormate),
		UpdatedAt:        time.Now().Format(constants.DataFormate),
		DeletedAt:        "",
		ReplyToCommentId: replyToCommentId, // 记录实际回复目标
	}

	// Use database transaction for consistency
	if err = db.CreateCommentWithTransaction(service.ctx, comment); err != nil {
		return errors.WithMessage(err, "Failed to create comment")
	}

	// Update rate limit counter
	go func() {
		key := fmt.Sprintf("comment_rate_limit:%d", uid)
		if err := redis.IncrementCommentRateLimit(key, 60); err != nil {
			hlog.Warnf("Failed to update rate limit for user %d: %v", uid, err)
		}
	}()

	// Store comment hash for duplicate detection
	go func() {
		if err := redis.StoreCommentHash(uid, req.Content, DuplicateTimeWindow); err != nil {
			hlog.Warnf("Failed to store comment hash for user %d: %v", uid, err)
		}
	}()

	// Record user behavior asynchronously with improved error handling
	userBehavior := &model.UserBehavior{
		UserId:       uid,
		VideoId:      videoId,
		BehaviorType: "comment",
		BehaviorTime: time.Now().Format(constants.DataFormate),
	}

	// Use goroutine with proper error handling and context
	go func() {
		// Create a new context with timeout for the async operation
		behaviorCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := db.AddUserCommentBehavior(behaviorCtx, userBehavior); err != nil {
			hlog.Warnf("Failed to record user comment behavior for user %d on video %d: %v",
				uid, videoId, err)
		}
	}()

	return nil
}

// getReplyRelationInfo 获取评论的回复关系信息
func (service *CommentService) getReplyRelationInfo(commentId int64) (replyToUserId int64, replyToContent string, err error) {
	if commentId == 0 {
		return 0, "", nil
	}

	// 获取被回复评论的信息
	replyToComment, err := db.GetCommentInfo(service.ctx, commentId)
	if err != nil {
		return 0, "", err
	}

	return replyToComment.UserId, replyToComment.Content, nil
}

// sortCommentsByHot implements hot comment sorting algorithm
// Combines like count and time factor for ranking
func (service *CommentService) sortCommentsByHot(commentIds []int64, pageNum, pageSize int64) ([]int64, error) {
	type CommentScore struct {
		CommentId int64
		Score     float64
		LikeCount int64
		CreatedAt string
	}

	scores := make([]CommentScore, 0, len(commentIds))

	// Calculate scores for each comment
	for _, commentId := range commentIds {
		// Get like count from Redis
		likeCount, err := redis.GetCommentLikeCount(commentId)
		if err != nil {
			hlog.Warnf("Failed to get like count for comment %d: %v", commentId, err)
			likeCount = 0
		}

		// Get comment info for creation time
		commentInfo, err := db.GetCommentInfo(service.ctx, commentId)
		if err != nil {
			hlog.Warnf("Failed to get comment info for comment %d: %v", commentId, err)
			continue
		}

		// Calculate hot score: like_count * 10 + time_factor
		// Time factor decreases as comment gets older (newer comments get higher scores)
		timeFactor := service.calculateTimeFactor(commentInfo.CreatedAt)
		hotScore := float64(likeCount)*10.0 + timeFactor

		scores = append(scores, CommentScore{
			CommentId: commentId,
			Score:     hotScore,
			LikeCount: likeCount,
			CreatedAt: commentInfo.CreatedAt,
		})
	}

	// Sort by score descending
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Score > scores[j].Score
	})

	// Apply pagination
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
		result = append(result, scores[i].CommentId)
	}

	return result, nil
}

// calculateTimeFactor calculates time-based factor for hot sorting
func (service *CommentService) calculateTimeFactor(createdAt string) float64 {
	// Parse creation time
	createdTime, err := time.Parse(constants.DataFormate, createdAt)
	if err != nil {
		return 0.0
	}

	// Calculate hours since creation
	hoursSinceCreation := time.Since(createdTime).Hours()

	// Time factor decreases exponentially: newer comments get higher scores
	// Max factor is 100, decreases by half every 24 hours
	timeFactor := 100.0 * math.Pow(0.5, hoursSinceCreation/24.0)

	return timeFactor
}

// buildCommentData builds a complete comment data structure
func (service *CommentService) buildCommentData(commentId int64) (*base.Comment, error) {
	var (
		wg         sync.WaitGroup
		errChan    = make(chan error, 3)
		res        *model.Comment
		likeCount  int64
		childCount int64
	)

	wg.Add(3)
	go func() {
		defer wg.Done()
		var err error
		res, err = db.GetCommentInfo(service.ctx, commentId)
		if err != nil {
			errChan <- errors.WithMessage(err, "Failed to get CommentInfo")
		}
	}()
	go func() {
		defer wg.Done()
		var err error
		likeCount, err = redis.GetCommentLikeCount(commentId)
		if err != nil {
			errChan <- errors.WithMessage(err, "Failed to get CommentLikeCount")
		}
	}()
	go func() {
		defer wg.Done()
		var err error
		childCount, err = db.GetChildCommentCount(service.ctx, commentId)
		if err != nil {
			errChan <- errors.WithMessage(err, "Failed to get ChildCommentCount")
		}
	}()

	wg.Wait()

	select {
	case err := <-errChan:
		return nil, err
	default:
	}

	return &base.Comment{
		CommentId:        res.CommentId,
		VideoId:          res.VideoId,
		UserId:           res.UserId,
		ParentId:         res.ParentId,
		LikeCount:        likeCount,
		ChildCount:       childCount,
		Content:          res.Content,
		CreatedAt:        res.CreatedAt,
		UpdatedAt:        res.UpdatedAt,
		DeletedAt:        res.DeletedAt,
		ReplyToCommentId: res.ReplyToCommentId,
	}, nil
}

// GetCommentsByReplyTarget 根据回复目标获取相关评论
func (service *CommentService) GetCommentsByReplyTarget(videoId, replyToCommentId int64, pageNum, pageSize int64) (*[]*base.Comment, error) {
	// 这个方法可以用于获取所有回复某个特定评论的评论列表
	// 实现逻辑需要在数据库层添加相应的查询方法
	// 这里先预留接口
	return nil, nil
}

func (service *CommentService) ListComment(ctx context.Context, req *interactions.ListCommentRequest) (resp *interactions.ListCommentResponse, err error) {
	resp = new(interactions.ListCommentResponse)
	if req.PageNum <= 0 {
		req.PageNum = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = constants.DefaultLimit
	}

	// Set default sort type to "hot" if not specified
	if req.SortType == "" {
		req.SortType = "hot"
	}

	var (
		data *[]*base.Comment
	)
	if req.VideoId != 0 {
		if data, err = service.GetVideoCommentWithSort(req); err != nil {
			return nil, err
		}
	} else if req.CommentId != 0 {
		if data, err = service.GetCommentComment(req); err != nil {
			return nil, err
		}
	} else {
		return nil, errno.RequestErr.WithMessage("Either VideoId or CommentId must be provided")
	}
	resp.Items = *data
	resp.Base = &base.Status{}
	return resp, nil
}

func (service *CommentService) NewDeleteEvent(ctx context.Context, req *interactions.CommentDeleteRequest) error {
	if req.VideoId != 0 {
		videoInfo, err := rpc.VideoClient.VideoInfo(ctx, &videos.VideoInfoRequest{VideoId: req.VideoId})
		if err != nil {
			hlog.Info("Error in VideoInfo RPC call:", err)
			return errno.RpcErr
		}
		if videoInfo == nil {
			hlog.Error("VideoInfo is nil")
			return errno.ServiceErr
		}
		if videoInfo.Items.UserId != req.FromUserId {
			return errno.ServiceErr
		}
		if err := service.DeleteVideo(req); err != nil {
			return err
		}
	} else if req.CommentId != 0 {
		commentInfo, err := db.GetCommentInfo(service.ctx, req.CommentId)
		if err != nil {
			return errno.MysqlErr
		}
		if commentInfo.UserId != req.FromUserId {
			return errno.ServiceErr
		}
		if err := service.DeleteComment(req); err != nil {
			return err
		}
	} else {
		return errno.RequestErr
	}
	return nil
}

// GetVideoCommentWithSort gets video comments with specified sorting
func (service *CommentService) GetVideoCommentWithSort(req *interactions.ListCommentRequest) (*[]*base.Comment, error) {
	data := make([]*base.Comment, 0)

	var list *[]int64
	var err error

	if req.SortType == "hot" {
		// For hot sorting, get more comments and sort by like count + time
		list, err = db.GetVideoCommentListForHotSort(service.ctx, req.VideoId, req.PageNum, req.PageSize)
		if err != nil {
			return nil, errno.ServiceErr
		}

		// Sort by hot algorithm (like count + time factor)
		sortedList, err := service.sortCommentsByHot(*list, req.PageNum, req.PageSize)
		if err != nil {
			return nil, errno.ServiceErr
		}
		list = &sortedList
	} else {
		// For latest or other sorting, use the standard method
		list, err = db.GetVideoCommentListByPartWithSort(service.ctx, req.VideoId, req.PageNum, req.PageSize, req.SortType)
		if err != nil {
			return nil, errno.ServiceErr
		}
	}

	// Build comment data
	for _, commentId := range *list {
		comment, err := service.buildCommentData(commentId)
		if err != nil {
			hlog.Warnf("Failed to build comment data for comment %d: %v", commentId, err)
			continue
		}
		data = append(data, comment)
	}

	return &data, nil
}

func (service *CommentService) GetVideoComment(req *interactions.ListCommentRequest) (*[]*base.Comment, error) {
	data := make([]*base.Comment, 0)
	list, err := db.GetVideoCommentListByPart(service.ctx, req.VideoId, req.PageNum, req.PageSize)
	if err != nil {
		return nil, errno.ServiceErr
	}
	var (
		wg         sync.WaitGroup
		errChan    = make(chan error, 3)
		res        *model.Comment
		likeCount  int64
		childCount int64
	)
	for _, item := range *list {
		wg.Add(3)
		go func() {
			res, err = db.GetCommentInfo(service.ctx, item)
			if err != nil {
				errChan <- errors.WithMessage(err, "Failed to get CommentInfo")
			}
			wg.Done()
		}()
		go func() {
			likeCount, err = redis.GetVideoLikeCount(item)
			if err != nil {
				errChan <- errors.WithMessage(err, "Failed to get VideoVisitCount")
			}
			wg.Done()
		}()
		go func() {
			childCount, err = db.GetChildCommentCount(service.ctx, item)
			if err != nil {
				errChan <- errors.WithMessage(err, "Failed to get ChildCommentCount")
			}
			wg.Done()
		}()
		wg.Wait()
		select {
		case result := <-errChan:
			return nil, result
		default:
		}
		data = append(data, &base.Comment{
			CommentId:        res.CommentId,
			VideoId:          res.VideoId,
			UserId:           res.UserId,
			ParentId:         res.ParentId,
			LikeCount:        likeCount,
			ChildCount:       childCount,
			Content:          res.Content,
			CreatedAt:        res.CreatedAt,
			UpdatedAt:        res.UpdatedAt,
			DeletedAt:        res.DeletedAt,
			ReplyToCommentId: res.ReplyToCommentId,
		})
	}
	return &data, nil
}

func (service *CommentService) GetCommentComment(req *interactions.ListCommentRequest) (*[]*base.Comment, error) {
	data := make([]*base.Comment, 0)
	list, err := db.GetCommentChildListByPart(service.ctx, req.CommentId, req.PageNum, req.PageSize)
	if err != nil {
		return nil, errno.ServiceErr
	}
	var (
		wg         sync.WaitGroup
		errChan    = make(chan error, 3)
		res        *model.Comment
		likeCount  int64
		childCount int64
	)
	for _, item := range *list {
		wg.Add(3)
		go func() {
			res, err = db.GetCommentInfo(service.ctx, item)

			if err != nil {
				errChan <- errors.WithMessage(err, "Failed to get CommentInfo")
			}
			wg.Done()
		}()
		go func() {
			likeCount, err = redis.GetCommentLikeCount(item)
			if err != nil {
				errChan <- errors.WithMessage(err, "Failed to get VideoVisitCount")
			}
			wg.Done()
		}()
		go func() {
			childCount, err = db.GetChildCommentCount(service.ctx, item)
			if err != nil {
				errChan <- errors.WithMessage(err, "Failed to get ChildCommentCount")
			}
			wg.Done()
		}()
		wg.Wait()
		select {
		case result := <-errChan:
			return nil, result
		default:
		}
		data = append(data, &base.Comment{
			CommentId:        res.CommentId,
			VideoId:          res.VideoId,
			UserId:           res.UserId,
			ParentId:         res.ParentId,
			LikeCount:        likeCount,
			ChildCount:       childCount,
			Content:          res.Content,
			CreatedAt:        res.CreatedAt,
			UpdatedAt:        res.UpdatedAt,
			DeletedAt:        res.DeletedAt,
			ReplyToCommentId: res.ReplyToCommentId,
		})
	}
	return &data, nil
}

func (service *CommentService) DeleteVideo(req *interactions.CommentDeleteRequest) error {
	list, err := db.GetVideoCommentList(context.Background(), req.VideoId)
	if err != nil {
		return errno.MysqlErr
	}
	if _, err := rpc.VideoClient.VideoDelete(service.ctx, &videos.VideoDeleteRequest{VideoId: req.VideoId, UserId: req.FromUserId}); err != nil {
		return errno.ServiceErr
	}

	var (
		wg      sync.WaitGroup
		errChan = make(chan error, len(*list))
	)

	wg.Add(len(*list))
	for _, item := range *list {
		go func(commentId int64) {
			if err := service.DeleteComment(&interactions.CommentDeleteRequest{CommentId: commentId}); err != nil {
				errChan <- err
			}
			wg.Done()
		}(item)
	}

	wg.Wait()
	select {
	case result := <-errChan:
		return result
	default:
	}
	return nil

}

func (service *CommentService) DeleteComment(req *interactions.CommentDeleteRequest) error {
	if err := db.DeleteComment(service.ctx, req.CommentId); err != nil {
		return errno.ServiceErr
	}

	var (
		wg      sync.WaitGroup
		errChan = make(chan error, 2)
	)
	wg.Add(2)
	go func() {
		if err := db.DeleteComment(context.Background(), req.CommentId); err != nil {
			errChan <- errno.RedisErr
		}
		wg.Done()
	}()
	go func() {
		if err := redis.DeleteCommentAndAllAbout(req.CommentId); err != nil {
			errChan <- errno.RedisErr
		}
		wg.Done()
	}()
	wg.Wait()
	select {
	case errr := <-errChan:
		return errr
	default:
	}
	return nil
}

func (service *CommentService) NewVideoPopularListEvent(req *interactions.VideoPopularListRequest) (*[]string, error) {
	list, err := redis.GetVideoPopularList(req.PageNum, req.PageSize)
	if err != nil {
		return nil, errno.RedisErr
	}
	return list, nil
}

// 删除操作
func (service *CommentService) NewDeleteVideoInfoEvent(req *interactions.DeleteVideoInfoRequest) error {
	var (
		err         error
		commentList *[]int64
		wg          sync.WaitGroup
		errChan     = make(chan error, 1)
	)
	// 将查询操作放入外面 保证数据的一致性
	if commentList, err = db.GetVideoCommentList(service.ctx, req.VideoId); err != nil {
		return errors.New("Failed to get VideoCommentList")
	}

	wg.Add(2)
	// 删除评论
	go func() {
		defer wg.Done()
		if err := redis.DeleteAllComment(*commentList); err != nil {
			errChan <- errors.New("Failed to delete VideoComment")
			return
		}
	}()
	// 删除点赞
	go func() {
		defer wg.Done()
		if err := redis.DeleteVideoAndAllAbout(req.VideoId); err != nil {
			errChan <- errors.New("Failed to delete VideoLike")
			return
		}
	}()
	wg.Wait()

	select {
	case err = <-errChan:
		return err
	default:
		return nil
	}
}
