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
	"HuaTug.com/pkg/utils"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/pkg/errors"
)

// Comment validation constants.
const (
	MaxCommentLength    = 500 // Maximum comment length in characters
	MinCommentLength    = 1   // Minimum comment length
	CommentRateLimit    = 10  // Max comments per minute per user
	DuplicateTimeWindow = 300 // Seconds to check for duplicate comments

	rateLimitKeyFmt = "comment_rate_limit:%d"
	rateLimitTTL    = 60 // seconds

	hotScoreLikeWeight   = 10.0 // Like count weight in hot score
	hotScoreTimeMaxScore = 100.0
	hotScoreTimeHalfLife = 24.0 // hours

	maxRepetitionCount     = 5
	minRepetitionCheckLen  = 10
	behaviorTimeout        = 5 * time.Second
	mentionProcessTimeout  = 10 * time.Second
	commentBuildConcurrent = 10 // max concurrent comment builds
)

// spamKeywords is the basic spam detection keyword list.
var spamKeywords = []string{"spam", "advertisement", "promotion"}

// CommentService handles comment CRUD and query operations.
type CommentService struct {
	ctx context.Context
}

// NewCommentService creates a comment service instance.
func NewCommentService(ctx context.Context) *CommentService {
	return &CommentService{ctx: ctx}
}

// --- Comment Creation ---

// CreateComment creates a comment with validation, rate limiting, and anti-spam checks.
func (s *CommentService) CreateComment(ctx context.Context, req *interactions.CreateCommentRequest) error {
	uid := req.UserId

	if err := s.validateCommentContent(req.Content); err != nil {
		return err
	}
	if req.CommentId == 0 && req.VideoId == 0 {
		return errno.RequestErr.WithMessage("Either VideoId or CommentId must be provided")
	}
	if err := s.checkRateLimit(uid); err != nil {
		return err
	}
	if err := s.checkDuplicateComment(uid, req.Content); err != nil {
		return err
	}

	// Resolve comment hierarchy.
	parentID, videoID, replyToCommentID, err := s.resolveCommentHierarchy(ctx, req)
	if err != nil {
		return err
	}

	// Verify video exists.
	if videoID != 0 {
		if _, err := rpc.VideoClient.VideoInfoV2(ctx, &videos.VideoInfoRequestV2{VideoId: videoID}); err != nil {
			return errors.WithMessage(err, "Video not found or inaccessible")
		}
	}

	commentID := utils.GenerateCommentID()
	comment := &model.Comment{
		CommentId:        commentID,
		VideoId:          videoID,
		ParentId:         parentID,
		UserId:           uid,
		Content:          strings.TrimSpace(req.Content),
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
		DeletedAt:        nil,
		ReplyToCommentId: replyToCommentID,
	}

	if err := db.CreateCommentWithTransaction(s.ctx, comment); err != nil {
		return errors.WithMessage(err, "Failed to create comment")
	}

	// Async post-create actions.
	go s.postCommentActions(uid, videoID, commentID, req.Content)

	return nil
}

// resolveCommentHierarchy determines parentID, videoID, and replyToCommentID for a new comment.
func (s *CommentService) resolveCommentHierarchy(ctx context.Context, req *interactions.CreateCommentRequest) (parentID, videoID, replyToCommentID int64, err error) {
	parentID = -1 // root comment
	videoID = req.VideoId

	if req.CommentId == 0 {
		return parentID, videoID, 0, nil
	}

	// This is a reply to another comment.
	parentCommentID, err := db.GetParentCommentId(s.ctx, req.CommentId)
	if err != nil {
		return 0, 0, 0, errors.WithMessage(err, "Failed to get parent comment information")
	}

	replyToCommentID = req.CommentId

	if req.Mode != 0 && parentCommentID != 0 {
		parentID = parentCommentID // flatten: hang under root
	} else {
		parentID = req.CommentId // traditional: direct reply
	}

	if videoID == 0 {
		videoID, err = db.GetCommentVideoId(s.ctx, req.CommentId)
		if err != nil {
			return 0, 0, 0, errors.WithMessage(err, "Failed to get video ID from comment")
		}
	}

	return parentID, videoID, replyToCommentID, nil
}

// postCommentActions runs async tasks after a comment is created.
func (s *CommentService) postCommentActions(uid, videoID, commentID int64, content string) {
	// Rate limit counter.
	key := fmt.Sprintf(rateLimitKeyFmt, uid)
	if err := redis.IncrementCommentRateLimit(key, rateLimitTTL); err != nil {
		hlog.Warnf("Failed to update rate limit for user %d: %v", uid, err)
	}

	// Duplicate detection hash.
	if err := redis.StoreCommentHash(uid, content, DuplicateTimeWindow); err != nil {
		hlog.Warnf("Failed to store comment hash for user %d: %v", uid, err)
	}

	// User behavior tracking.
	behaviorCtx, cancel := context.WithTimeout(context.Background(), behaviorTimeout)
	defer cancel()
	userBehavior := &model.UserBehavior{
		UserId:       uid,
		VideoId:      videoID,
		BehaviorType: "comment",
		BehaviorTime: time.Now(),
	}
	if err := db.AddUserCommentBehavior(behaviorCtx, userBehavior); err != nil {
		hlog.Warnf("Failed to record comment behavior: user_id=%d, video_id=%d, err=%v", uid, videoID, err)
	}

	// @mention notifications.
	mentionCtx, mentionCancel := context.WithTimeout(context.Background(), mentionProcessTimeout)
	defer mentionCancel()
	NewMentionNotificationService().ProcessMentions(mentionCtx, uid, videoID, commentID, content)
}

// --- Validation ---

// validateCommentContent checks content length and spam patterns.
func (s *CommentService) validateCommentContent(content string) error {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return errno.RequestErr.WithMessage("Comment content cannot be empty")
	}

	length := utf8.RuneCountInString(trimmed)
	if length < MinCommentLength {
		return errno.RequestErr.WithMessage("Comment too short")
	}
	if length > MaxCommentLength {
		return errno.RequestErr.WithMessage("Comment too long, maximum 500 characters allowed")
	}

	if s.isSpamContent(trimmed) {
		return errno.RequestErr.WithMessage("Comment contains inappropriate content")
	}

	return nil
}

// isSpamContent performs basic spam detection.
func (s *CommentService) isSpamContent(content string) bool {
	lower := strings.ToLower(content)

	if hasExcessiveRepetition(lower) {
		return true
	}

	for _, kw := range spamKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// hasExcessiveRepetition checks for more than maxRepetitionCount consecutive identical characters.
func hasExcessiveRepetition(content string) bool {
	if len(content) < minRepetitionCheckLen {
		return false
	}

	var prev rune
	count := 0
	for _, ch := range content {
		if ch == prev {
			count++
			if count > maxRepetitionCount {
				return true
			}
		} else {
			count = 1
			prev = ch
		}
	}
	return false
}

// checkRateLimit validates user comment frequency via Redis.
func (s *CommentService) checkRateLimit(userID int64) error {
	key := fmt.Sprintf(rateLimitKeyFmt, userID)
	count, err := redis.GetCommentRateLimit(key)
	if err != nil {
		hlog.Warnf("Rate limit check failed for user %d: %v", userID, err)
		return nil // do not block on Redis failure
	}
	if count >= CommentRateLimit {
		return errno.RequestErr.WithMessage("Comment rate limit exceeded, please try again later")
	}
	return nil
}

// checkDuplicateComment prevents duplicate comments in a time window.
func (s *CommentService) checkDuplicateComment(userID int64, content string) error {
	isDuplicate, err := redis.CheckDuplicateComment(userID, content, DuplicateTimeWindow)
	if err != nil {
		hlog.Warnf("Duplicate check failed for user %d: %v", userID, err)
		return nil
	}
	if isDuplicate {
		return errno.RequestErr.WithMessage("Duplicate comment detected, please wait before posting similar content")
	}
	return nil
}

// --- Comment Listing ---

// ListComment returns paginated comments for a video or comment thread.
func (s *CommentService) ListComment(ctx context.Context, req *interactions.ListCommentRequest) (*interactions.ListCommentResponse, error) {
	resp := &interactions.ListCommentResponse{Base: &base.Status{}}

	if req.PageNum <= 0 {
		req.PageNum = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = constants.DefaultLimit
	}
	if req.SortType == "" {
		req.SortType = "hot"
	}

	var (
		data *[]*base.Comment
		err  error
	)

	switch {
	case req.VideoId != 0:
		data, err = s.GetVideoCommentWithSort(req)
	case req.CommentId != 0:
		data, err = s.GetCommentComment(req)
	default:
		return resp, errno.RequestErr.WithMessage("Either VideoId or CommentId must be provided")
	}

	if err != nil {
		hlog.Errorf("ListComment failed: %v", err)
		return resp, err
	}

	if data != nil {
		resp.Items = *data
	} else {
		resp.Items = make([]*base.Comment, 0)
	}

	return resp, nil
}

// GetVideoCommentWithSort returns video comments sorted by the requested strategy.
func (s *CommentService) GetVideoCommentWithSort(req *interactions.ListCommentRequest) (*[]*base.Comment, error) {
	data := make([]*base.Comment, 0)

	var (
		list *[]int64
		err  error
	)

	switch req.SortType {
	case "hot":
		list, err = db.GetVideoCommentListForHotSort(s.ctx, req.VideoId, req.PageNum, req.PageSize)
		if err != nil {
			return nil, errno.ServiceErr
		}
		if list == nil {
			return &data, nil
		}
		sortedList, sortErr := s.sortCommentsByHot(*list, req.PageNum, req.PageSize)
		if sortErr != nil {
			return nil, errno.ServiceErr
		}
		list = &sortedList
	default:
		list, err = db.GetVideoCommentListByPartWithSort(s.ctx, req.VideoId, req.PageNum, req.PageSize, req.SortType)
		if err != nil {
			return nil, errno.ServiceErr
		}
	}

	if list == nil {
		return &data, nil
	}

	for _, commentID := range *list {
		comment, buildErr := s.buildCommentData(commentID)
		if buildErr != nil {
			hlog.Warnf("Failed to build comment %d: %v", commentID, buildErr)
			continue
		}
		data = append(data, comment)
	}

	return &data, nil
}

// GetVideoComment returns video comments (legacy, no sort).
func (s *CommentService) GetVideoComment(req *interactions.ListCommentRequest) (*[]*base.Comment, error) {
	data := make([]*base.Comment, 0)
	list, err := db.GetVideoCommentListByPart(s.ctx, req.VideoId, req.PageNum, req.PageSize)
	if err != nil {
		return nil, errno.ServiceErr
	}

	for _, commentID := range *list {
		comment, buildErr := s.buildCommentData(commentID)
		if buildErr != nil {
			hlog.Warnf("Failed to build comment %d: %v", commentID, buildErr)
			continue
		}
		data = append(data, comment)
	}

	return &data, nil
}

// GetCommentComment returns child comments (replies) for a given comment.
func (s *CommentService) GetCommentComment(req *interactions.ListCommentRequest) (*[]*base.Comment, error) {
	data := make([]*base.Comment, 0)
	list, err := db.GetCommentChildListByPart(s.ctx, req.CommentId, req.PageNum, req.PageSize)
	if err != nil {
		hlog.Errorf("Failed to get comment child list: %v", err)
		return nil, errno.ServiceErr
	}
	if list == nil {
		return &data, nil
	}

	for _, commentID := range *list {
		comment, buildErr := s.buildCommentData(commentID)
		if buildErr != nil {
			hlog.Warnf("Failed to build comment %d: %v", commentID, buildErr)
			continue
		}
		data = append(data, comment)
	}

	return &data, nil
}

// --- Hot Score ---

// sortCommentsByHot calculates hot scores and returns paginated sorted IDs.
func (s *CommentService) sortCommentsByHot(commentIDs []int64, pageNum, pageSize int64) ([]int64, error) {
	type commentScore struct {
		ID    int64
		Score float64
	}

	scores := make([]commentScore, 0, len(commentIDs))

	for _, cid := range commentIDs {
		likeCount, err := redis.GetCommentLikeCount(cid)
		if err != nil {
			likeCount = 0
		}

		info, err := db.GetCommentInfo(s.ctx, cid)
		if err != nil {
			hlog.Warnf("Failed to get comment info %d: %v", cid, err)
			continue
		}

		timeFactor := calculateTimeFactor(info.CreatedAt)
		hotScore := float64(likeCount)*hotScoreLikeWeight + timeFactor

		scores = append(scores, commentScore{ID: cid, Score: hotScore})
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

// calculateTimeFactor returns an exponentially decaying time factor.
// Max 100, halves every 24 hours.
func calculateTimeFactor(createdAt time.Time) float64 {
	hours := time.Since(createdAt).Hours()
	return hotScoreTimeMaxScore * math.Pow(0.5, hours/hotScoreTimeHalfLife)
}

// --- Comment Data Builder ---

// buildCommentData fetches comment info, like count, and child count concurrently.
func (s *CommentService) buildCommentData(commentID int64) (*base.Comment, error) {
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
		res, err := db.GetCommentInfo(s.ctx, commentID)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			errs = append(errs, errors.WithMessage(err, "GetCommentInfo"))
			return
		}
		commentRes = res
	}()

	go func() {
		defer wg.Done()
		count, err := redis.GetCommentLikeCount(commentID)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			errs = append(errs, errors.WithMessage(err, "GetCommentLikeCount"))
			return
		}
		likeCount = count
	}()

	go func() {
		defer wg.Done()
		count, err := db.GetChildCommentCount(s.ctx, commentID)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			errs = append(errs, errors.WithMessage(err, "GetChildCommentCount"))
			return
		}
		childCount = count
	}()

	wg.Wait()

	if len(errs) > 0 {
		return nil, errs[0]
	}
	if commentRes == nil {
		return nil, fmt.Errorf("comment %d not found", commentID)
	}

	createdAtStr := commentRes.CreatedAt.Format(constants.DataFormate)
	updatedAtStr := commentRes.UpdatedAt.Format(constants.DataFormate)
	var deletedAtStr string
	if commentRes.DeletedAt != nil {
		deletedAtStr = commentRes.DeletedAt.Format(constants.DataFormate)
	}

	return &base.Comment{
		CommentId:        commentRes.CommentId,
		VideoId:          commentRes.VideoId,
		UserId:           commentRes.UserId,
		ParentId:         commentRes.ParentId,
		LikeCount:        likeCount,
		ChildCount:       childCount,
		Content:          commentRes.Content,
		CreatedAt:        createdAtStr,
		UpdatedAt:        updatedAtStr,
		DeletedAt:        deletedAtStr,
		ReplyToCommentId: commentRes.ReplyToCommentId,
	}, nil
}

// GetCommentsByReplyTarget returns comments replying to a specific comment (reserved interface).
func (s *CommentService) GetCommentsByReplyTarget(videoID, replyToCommentID, pageNum, pageSize int64) (*[]*base.Comment, error) {
	return nil, nil
}

// getReplyRelationInfo retrieves reply target info (user ID and content).
func (s *CommentService) getReplyRelationInfo(commentID int64) (int64, string, error) {
	if commentID == 0 {
		return 0, "", nil
	}
	info, err := db.GetCommentInfo(s.ctx, commentID)
	if err != nil {
		return 0, "", err
	}
	return info.UserId, info.Content, nil
}

// --- Delete Operations ---

// NewDeleteEvent handles delete requests for videos or comments with permission checks.
func (s *CommentService) NewDeleteEvent(ctx context.Context, req *interactions.CommentDeleteRequest) error {
	if req.VideoId != 0 {
		return s.deleteVideoWithPermission(ctx, req)
	}
	if req.CommentId != 0 {
		return s.deleteCommentWithPermission(ctx, req)
	}
	return errno.RequestErr
}

// deleteVideoWithPermission verifies ownership before deleting a video and its comments.
func (s *CommentService) deleteVideoWithPermission(ctx context.Context, req *interactions.CommentDeleteRequest) error {
	videoInfo, err := rpc.VideoClient.VideoInfoV2(ctx, &videos.VideoInfoRequestV2{VideoId: req.VideoId})
	if err != nil {
		hlog.Errorf("VideoInfo RPC call failed: %v", err)
		return errno.RpcErr
	}
	if videoInfo == nil || videoInfo.Items == nil {
		hlog.Error("VideoInfo is nil")
		return errno.ServiceErr
	}
	if videoInfo.Items.UserId != req.FromUserId {
		return errno.ServiceErr
	}
	return s.DeleteVideo(req)
}

// deleteCommentWithPermission verifies ownership before deleting a comment.
func (s *CommentService) deleteCommentWithPermission(ctx context.Context, req *interactions.CommentDeleteRequest) error {
	commentInfo, err := db.GetCommentInfo(s.ctx, req.CommentId)
	if err != nil {
		return errno.MysqlErr
	}
	if commentInfo.UserId != req.FromUserId {
		return errno.ServiceErr
	}
	return s.DeleteComment(req)
}

// DeleteVideo deletes a video and all its comments.
func (s *CommentService) DeleteVideo(req *interactions.CommentDeleteRequest) error {
	list, err := db.GetVideoCommentList(context.Background(), req.VideoId)
	if err != nil {
		return errno.MysqlErr
	}
	if _, err := rpc.VideoClient.VideoDeleteV2(s.ctx, &videos.VideoDeleteRequestV2{
		VideoId: req.VideoId,
		UserId:  req.FromUserId,
	}); err != nil {
		return errno.ServiceErr
	}

	errChan := make(chan error, len(*list))
	var wg sync.WaitGroup

	for _, item := range *list {
		wg.Add(1)
		go func(commentID int64) {
			defer wg.Done()
			if delErr := s.DeleteComment(&interactions.CommentDeleteRequest{CommentId: commentID}); delErr != nil {
				errChan <- delErr
			}
		}(item)
	}

	wg.Wait()
	select {
	case err := <-errChan:
		return err
	default:
		return nil
	}
}

// DeleteComment soft-deletes a comment and cleans up associated Redis data.
func (s *CommentService) DeleteComment(req *interactions.CommentDeleteRequest) error {
	if err := db.DeleteComment(s.ctx, req.CommentId); err != nil {
		return errno.ServiceErr
	}

	// Async cache cleanup only (no duplicate DB delete).
	go func() {
		if err := redis.DeleteCommentAndAllAbout(req.CommentId); err != nil {
			hlog.Warnf("Failed to clean Redis for comment %d: %v", req.CommentId, err)
		}
	}()

	return nil
}

// --- Popular Videos ---

// NewVideoPopularListEvent returns the popular video list from Redis.
func (s *CommentService) NewVideoPopularListEvent(req *interactions.VideoPopularListRequest) (*[]string, error) {
	list, err := redis.GetVideoPopularList(req.PageNum, req.PageSize)
	if err != nil {
		return nil, errno.RedisErr
	}
	return list, nil
}

// NewDeleteVideoInfoEvent deletes all comments and likes associated with a video.
func (s *CommentService) NewDeleteVideoInfoEvent(req *interactions.DeleteVideoInfoRequest) error {
	commentList, err := db.GetVideoCommentList(s.ctx, req.VideoId)
	if err != nil {
		return errors.New("Failed to get VideoCommentList")
	}

	errChan := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		if delErr := redis.DeleteAllComment(*commentList); delErr != nil {
			errChan <- errors.New("Failed to delete VideoComment")
		}
	}()

	go func() {
		defer wg.Done()
		if delErr := redis.DeleteVideoAndAllAbout(req.VideoId); delErr != nil {
			errChan <- errors.New("Failed to delete VideoLike")
		}
	}()

	wg.Wait()

	select {
	case err := <-errChan:
		return err
	default:
		return nil
	}
}
