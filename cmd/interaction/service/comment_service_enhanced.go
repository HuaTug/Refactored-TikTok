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

// Enhanced comment constants.
const (
	EnhancedMaxCommentLength    = 500
	EnhancedMinCommentLength    = 1
	EnhancedCommentRateLimit    = 10
	EnhancedDuplicateTimeWindow = 300
	EnhancedMaxReplyDepth       = 2

	enhancedDefaultPageSize   = 20
	enhancedHotScoreMultiply  = 3 // multiplier to over-fetch for hot sort
	enhancedHotLikeWeight     = 10.0
	enhancedHotReplyWeight    = 5.0
	enhancedHotTimeMaxScore   = 100.0
	enhancedHotTimeHalfLife   = 24.0
	enhancedCommentConcurrent = 10
)

// enhancedSpamKeywords extends the spam keyword list with Chinese terms.
var enhancedSpamKeywords = []string{
	"广告", "推广", "加微信", "加qq", "联系方式",
	"spam", "advertisement",
}

// enhancedSensitiveWords is a basic sensitive word list for content filtering.
var enhancedSensitiveWords = []string{"spam", "scam", "abuse"}

// EnhancedCommentService provides enhanced comment operations with rate limiting and spam detection.
type EnhancedCommentService struct {
	ctx            context.Context
	interactionMgr *redis.EnhancedInteractionManager
}

// NewEnhancedCommentService creates an enhanced comment service.
func NewEnhancedCommentService(ctx context.Context) *EnhancedCommentService {
	return &EnhancedCommentService{
		ctx:            ctx,
		interactionMgr: redis.NewEnhancedInteractionManager(redis.RedisDBInteraction),
	}
}

// --- Comment Creation ---

// CreateComment creates a comment with enhanced validation, rate limiting, and spam detection.
func (s *EnhancedCommentService) CreateComment(ctx context.Context, req *interactions.CreateCommentRequest) error {
	userID := req.UserId

	if err := s.validateContent(req.Content); err != nil {
		return err
	}
	if req.CommentId == 0 && req.VideoId == 0 {
		return errno.RequestErr.WithMessage("Either VideoId or CommentId must be provided")
	}

	// Rate limit check.
	result, err := s.interactionMgr.CheckUserCommentRateLimit(ctx, userID)
	if err != nil {
		hlog.CtxWarnf(ctx, "Rate limit check failed: %v", err)
	} else if !result.Allowed {
		waitSec := (result.RetryAt - time.Now().UnixMilli()) / 1000
		return errno.RequestErr.WithMessage(fmt.Sprintf("评论太频繁，请稍后再试 (剩余等待时间: %ds)", waitSec))
	}

	if err := s.checkDuplicate(userID, req.Content); err != nil {
		return err
	}

	// Filter sensitive words.
	content := s.filterSensitiveWords(req.Content)

	// Resolve hierarchy.
	parentID, videoID, replyToCommentID, err := s.resolveHierarchy(ctx, req)
	if err != nil {
		return err
	}

	// Verify video exists.
	if videoID != 0 {
		if _, err := rpc.VideoClient.VideoInfoV2(ctx, &videos.VideoInfoRequestV2{VideoId: videoID}); err != nil {
			return errors.WithMessage(err, "Video not found")
		}
	}

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

	if err := db.CreateCommentWithTransaction(ctx, comment); err != nil {
		return errors.WithMessage(err, "Failed to create comment")
	}

	go s.enhancedPostCommentActions(ctx, userID, videoID, commentID, req.Content)

	hlog.CtxInfof(ctx, "Comment created: user_id=%d, video_id=%d, comment_id=%d", userID, videoID, commentID)
	return nil
}

// resolveHierarchy determines parentID, videoID, and replyToCommentID.
func (s *EnhancedCommentService) resolveHierarchy(ctx context.Context, req *interactions.CreateCommentRequest) (parentID, videoID, replyToCommentID int64, err error) {
	parentID = -1
	videoID = req.VideoId

	if req.CommentId == 0 {
		return parentID, videoID, 0, nil
	}

	parentCommentID, err := db.GetParentCommentId(ctx, req.CommentId)
	if err != nil {
		return 0, 0, 0, errors.WithMessage(err, "Failed to get parent comment info")
	}

	replyToCommentID = req.CommentId
	if req.Mode != 0 && parentCommentID != 0 {
		parentID = parentCommentID
	} else {
		parentID = req.CommentId
	}

	if videoID == 0 {
		videoID, err = db.GetCommentVideoId(ctx, req.CommentId)
		if err != nil {
			return 0, 0, 0, errors.WithMessage(err, "Failed to get video ID")
		}
	}

	return parentID, videoID, replyToCommentID, nil
}

// enhancedPostCommentActions performs async actions after comment creation.
func (s *EnhancedCommentService) enhancedPostCommentActions(ctx context.Context, userID, videoID, commentID int64, content string) {
	key := fmt.Sprintf(rateLimitKeyFmt, userID)
	if err := redis.IncrementCommentRateLimit(key, rateLimitTTL); err != nil {
		hlog.CtxWarnf(ctx, "Failed to update rate limit: %v", err)
	}
	if err := redis.StoreCommentHash(userID, content, EnhancedDuplicateTimeWindow); err != nil {
		hlog.CtxWarnf(ctx, "Failed to store comment hash: %v", err)
	}
	if err := s.interactionMgr.TrackUserActivity(ctx, userID, "comment"); err != nil {
		hlog.CtxWarnf(ctx, "Failed to track activity: %v", err)
	}
}

// --- Content Validation & Filtering ---

// validateContent validates comment content with length and spam checks.
func (s *EnhancedCommentService) validateContent(content string) error {
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

	if s.isSpamContent(trimmed) {
		return errno.RequestErr.WithMessage("评论内容包含不当内容")
	}

	return nil
}

// isSpamContent detects spam content by repetition and keywords.
func (s *EnhancedCommentService) isSpamContent(content string) bool {
	lower := strings.ToLower(content)

	if hasExcessiveRepetition(lower) {
		return true
	}

	for _, kw := range enhancedSpamKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// filterSensitiveWords replaces sensitive words with asterisks (case-insensitive).
func (s *EnhancedCommentService) filterSensitiveWords(content string) string {
	filtered := content
	for _, word := range enhancedSensitiveWords {
		if word == "" {
			continue
		}
		replacement := strings.Repeat("*", len(word))
		lowerFiltered := strings.ToLower(filtered)
		lowerWord := strings.ToLower(word)

		for {
			idx := strings.Index(lowerFiltered, lowerWord)
			if idx == -1 {
				break
			}
			filtered = filtered[:idx] + replacement + filtered[idx+len(word):]
			lowerFiltered = lowerFiltered[:idx] + replacement + lowerFiltered[idx+len(word):]
		}
	}
	return filtered
}

// checkDuplicate checks for duplicate comments in the recent time window.
func (s *EnhancedCommentService) checkDuplicate(userID int64, content string) error {
	isDuplicate, err := redis.CheckDuplicateComment(userID, content, EnhancedDuplicateTimeWindow)
	if err != nil {
		hlog.Warnf("Duplicate check failed: %v", err)
		return nil
	}
	if isDuplicate {
		return errno.RequestErr.WithMessage("请勿重复发送相同内容")
	}
	return nil
}

// --- Comment Listing ---

// ListComment returns paginated comments with sorting support.
func (s *EnhancedCommentService) ListComment(ctx context.Context, req *interactions.ListCommentRequest) (*interactions.ListCommentResponse, error) {
	if req.PageNum <= 0 {
		req.PageNum = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = enhancedDefaultPageSize
	}
	if req.SortType == "" {
		req.SortType = "hot"
	}

	var (
		comments []*base.Comment
		err      error
	)

	switch {
	case req.VideoId != 0:
		comments, err = s.getVideoComments(ctx, req)
	case req.CommentId != 0:
		comments, err = s.getReplyComments(ctx, req)
	default:
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

// getVideoComments returns root-level comments for a video.
func (s *EnhancedCommentService) getVideoComments(ctx context.Context, req *interactions.ListCommentRequest) ([]*base.Comment, error) {
	var (
		commentIDs []int64
		err        error
	)

	switch req.SortType {
	case "hot":
		commentIDs, err = s.getHotComments(ctx, req.VideoId, req.PageNum, req.PageSize)
	case "latest":
		list, listErr := db.GetVideoCommentListByPartWithSort(ctx, req.VideoId, req.PageNum, req.PageSize, "latest")
		if listErr != nil {
			return nil, listErr
		}
		if list != nil {
			commentIDs = *list
		}
		// No separate err needed
	default:
		commentIDs, err = s.getHotComments(ctx, req.VideoId, req.PageNum, req.PageSize)
	}

	if err != nil {
		return nil, err
	}

	return s.batchBuildComments(ctx, commentIDs)
}

// getReplyComments returns replies to a specific comment.
func (s *EnhancedCommentService) getReplyComments(ctx context.Context, req *interactions.ListCommentRequest) ([]*base.Comment, error) {
	list, err := db.GetCommentChildListByPart(ctx, req.CommentId, req.PageNum, req.PageSize)
	if err != nil {
		return nil, err
	}
	if list == nil {
		return []*base.Comment{}, nil
	}
	return s.batchBuildComments(ctx, *list)
}

// getHotComments fetches, scores, sorts, and paginates comments by hot score.
func (s *EnhancedCommentService) getHotComments(ctx context.Context, videoID, pageNum, pageSize int64) ([]int64, error) {
	list, err := db.GetVideoCommentListForHotSort(ctx, videoID, 1, pageNum*pageSize*enhancedHotScoreMultiply)
	if err != nil {
		return nil, err
	}
	if list == nil || len(*list) == 0 {
		return []int64{}, nil
	}

	type commentScore struct {
		ID    int64
		Score float64
	}

	scores := make([]commentScore, 0, len(*list))
	for _, cid := range *list {
		score, scoreErr := s.calcHotScore(ctx, cid)
		if scoreErr != nil {
			continue
		}
		scores = append(scores, commentScore{ID: cid, Score: score})
	}

	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Score > scores[j].Score
	})

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
		result = append(result, scores[i].ID)
	}
	return result, nil
}

// calcHotScore computes a hot score combining likes, replies, and time decay.
func (s *EnhancedCommentService) calcHotScore(ctx context.Context, commentID int64) (float64, error) {
	likeCount, _ := redis.GetCommentLikeCount(commentID)
	replyCount, _ := db.GetChildCommentCount(ctx, commentID)

	info, err := db.GetCommentInfo(ctx, commentID)
	if err != nil {
		return 0, err
	}

	hours := time.Since(info.CreatedAt).Hours()
	timeFactor := enhancedHotTimeMaxScore * math.Pow(0.5, hours/enhancedHotTimeHalfLife)

	return float64(likeCount)*enhancedHotLikeWeight +
		float64(replyCount)*enhancedHotReplyWeight +
		timeFactor, nil
}

// --- Batch Comment Builder ---

// batchBuildComments builds comment data concurrently with bounded parallelism.
func (s *EnhancedCommentService) batchBuildComments(ctx context.Context, commentIDs []int64) ([]*base.Comment, error) {
	if len(commentIDs) == 0 {
		return []*base.Comment{}, nil
	}

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		results = make([]*base.Comment, 0, len(commentIDs))
	)

	semaphore := make(chan struct{}, enhancedCommentConcurrent)

	for _, cid := range commentIDs {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(id int64) {
			defer wg.Done()
			defer func() { <-semaphore }()

			comment, err := s.buildComment(ctx, id)
			if err != nil {
				hlog.CtxWarnf(ctx, "Failed to build comment %d: %v", id, err)
				return
			}

			mu.Lock()
			results = append(results, comment)
			mu.Unlock()
		}(cid)
	}

	wg.Wait()
	return results, nil
}

// buildComment fetches a single comment's data concurrently.
func (s *EnhancedCommentService) buildComment(ctx context.Context, commentID int64) (*base.Comment, error) {
	var (
		wg         sync.WaitGroup
		mu         sync.Mutex
		errs       []error
		commentRes *model.Comment
		likeCount  int64
		childCount int64
	)

	wg.Add(3)

	go func() {
		defer wg.Done()
		res, err := db.GetCommentInfo(ctx, commentID)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			errs = append(errs, err)
			return
		}
		commentRes = res
	}()

	go func() {
		defer wg.Done()
		count, _ := redis.GetCommentLikeCount(commentID)
		mu.Lock()
		likeCount = count
		mu.Unlock()
	}()

	go func() {
		defer wg.Done()
		count, _ := db.GetChildCommentCount(ctx, commentID)
		mu.Lock()
		childCount = count
		mu.Unlock()
	}()

	wg.Wait()

	if len(errs) > 0 {
		return nil, errs[0]
	}
	if commentRes == nil {
		return nil, fmt.Errorf("comment not found: %d", commentID)
	}

	var deletedAtStr string
	if commentRes.DeletedAt != nil {
		deletedAtStr = commentRes.DeletedAt.Format("2006-01-02 15:04:05")
	}

	return &base.Comment{
		CommentId:        commentRes.CommentId,
		VideoId:          commentRes.VideoId,
		UserId:           commentRes.UserId,
		ParentId:         commentRes.ParentId,
		LikeCount:        likeCount,
		ChildCount:       childCount,
		Content:          commentRes.Content,
		CreatedAt:        commentRes.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:        commentRes.UpdatedAt.Format("2006-01-02 15:04:05"),
		DeletedAt:        deletedAtStr,
		ReplyToCommentId: commentRes.ReplyToCommentId,
	}, nil
}

// --- Cache ---

// updateCommentCountCache updates the video comment count cache (placeholder).
func (s *EnhancedCommentService) updateCommentCountCache(_ context.Context, _ int64) {
	// TODO: implement atomic comment count cache update
}

// --- Delete ---

// DeleteComment deletes a comment with permission verification.
func (s *EnhancedCommentService) DeleteComment(ctx context.Context, req *interactions.CommentDeleteRequest) error {
	commentInfo, err := db.GetCommentInfo(ctx, req.CommentId)
	if err != nil {
		return errno.MysqlErr
	}

	// Check permission: comment owner or video owner.
	if commentInfo.UserId != req.FromUserId {
		if req.VideoId != 0 {
			videoInfo, vErr := rpc.VideoClient.VideoInfoV2(ctx, &videos.VideoInfoRequestV2{VideoId: req.VideoId})
			if vErr != nil || videoInfo == nil || videoInfo.Items.UserId != req.FromUserId {
				return errno.ServiceErr.WithMessage("无权删除此评论")
			}
		} else {
			return errno.ServiceErr.WithMessage("无权删除此评论")
		}
	}

	if err := db.DeleteComment(ctx, req.CommentId); err != nil {
		return errno.ServiceErr
	}

	go func() {
		if delErr := redis.DeleteCommentAndAllAbout(req.CommentId); delErr != nil {
			hlog.Warnf("Failed to clean Redis for comment %d: %v", req.CommentId, delErr)
		}
	}()

	hlog.CtxInfof(ctx, "Comment deleted: comment_id=%d, by_user=%d", req.CommentId, req.FromUserId)
	return nil
}
