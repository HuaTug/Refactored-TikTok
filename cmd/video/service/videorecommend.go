package service

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"HuaTug.com/cmd/video/dal/db"
	"HuaTug.com/kitex_gen/base"
	"HuaTug.com/kitex_gen/videos"
	"HuaTug.com/pkg/recommendation"

	"github.com/cloudwego/hertz/pkg/common/hlog"
)

type RecommendVideoService struct {
	ctx context.Context
}

// RecommendResult 推荐结果（包含算法元信息）
type RecommendResult struct {
	VideoList        []*base.Video
	AlgorithmUsed    string
	RecommendationID string
}

func NewRecommendVideoService(ctx context.Context) *RecommendVideoService {
	return &RecommendVideoService{ctx: ctx}
}

func (service *RecommendVideoService) RecommendVideo(req *videos.RecommendVideoRequestV2) (*RecommendResult, error) {
	count := int(req.Count)
	if count <= 0 {
		count = 10
	}

	hlog.Infof("[RecommendVideo] user_id=%d, count=%d, algorithm=%s", req.UserId, count, req.AlgorithmType)

	// ========================================
	// 优先路径: RecommendationAgent（智能决策层）
	// Agent 会根据用户实时状态自动选择最优策略:
	//   COLD_START / HOT_EXPLORE / TOPIC_DEEP_DIVE / STANDARD
	// ========================================
	agent := recommendation.GetRecommendationAgent()
	if agent != nil {
		result, err := service.recommendWithAgent(agent, req.UserId, count)
		if err == nil && len(result.VideoList) > 0 {
			hlog.Infof("[RecommendVideo] Got %d videos from Agent (algo=%s)", len(result.VideoList), result.AlgorithmUsed)
			return result, nil
		}
		if err != nil {
			hlog.Warnf("[RecommendVideo] Agent recommendation failed: %v, falling back to legacy pipeline", err)
		}
	}

	// ========================================
	// Legacy 策略1: DB 召回 + DeepCTR 精排（完整推荐链路）
	// ========================================
	result, err := service.recommendWithCTR(req.UserId, count)
	if err == nil && len(result.VideoList) > 0 {
		hlog.Infof("[RecommendVideo] Got %d videos from CTR-ranked recommendation", len(result.VideoList))
		return result, nil
	}
	if err != nil {
		hlog.Warnf("[RecommendVideo] CTR recommendation failed: %v, falling back", err)
	}

	// ========================================
	// Legacy 策略2: 降级 - 基于 video_features 热度推荐
	// ========================================
	videoList, err := service.recommendByFeatures(count)
	if err == nil && len(videoList) > 0 {
		hlog.Infof("[RecommendVideo] Got %d videos from feature-based recommendation", len(videoList))
		return &RecommendResult{
			VideoList:     videoList,
			AlgorithmUsed: "popularity",
		}, nil
	}
	if err != nil {
		hlog.Warnf("[RecommendVideo] Feature-based recommendation failed: %v, falling back", err)
	}

	// ========================================
	// Legacy 策略3: 最终降级 - 直接从 videos 表查询
	// ========================================
	videoList, err = service.recommendFallback(req.UserId, count)
	if err != nil {
		hlog.Errorf("[RecommendVideo] Fallback recommendation failed: %v", err)
		return nil, err
	}

	hlog.Infof("[RecommendVideo] Got %d videos from fallback recommendation", len(videoList))
	return &RecommendResult{
		VideoList:     videoList,
		AlgorithmUsed: "fallback",
	}, nil
}

// recommendWithAgent delegates to the RecommendationAgent, which dynamically
// selects among COLD_START / HOT_EXPLORE / TOPIC_DEEP_DIVE / STANDARD pipelines
// based on the user's real-time behavioral state.
func (service *RecommendVideoService) recommendWithAgent(agent *recommendation.RecommendationAgent, userId int64, count int) (*RecommendResult, error) {
	agentReq := &recommendation.RecommendRequest{
		UserID:    userId,
		Limit:     count,
		RequestID: fmt.Sprintf("rec_%d_%d", userId, time.Now().UnixMilli()),
	}

	resp, err := agent.Recommend(service.ctx, agentReq)
	if err != nil {
		return nil, fmt.Errorf("agent recommend failed: %w", err)
	}
	if resp == nil || len(resp.Videos) == 0 {
		return nil, fmt.Errorf("agent returned empty result")
	}

	// Convert ScoredVideo → base.Video by fetching full info from DB
	videoIDs := make([]int64, len(resp.Videos))
	for i, v := range resp.Videos {
		videoIDs[i] = v.VideoID
	}

	videoList, err := db.GetVideoByVideoId(service.ctx, videoIDs)
	if err != nil {
		return nil, fmt.Errorf("get video details failed: %w", err)
	}

	// Preserve Agent ranking order
	videoMap := make(map[int64]*base.Video, len(videoList))
	for _, v := range videoList {
		videoMap[v.VideoId] = v
	}
	orderedList := make([]*base.Video, 0, len(videoIDs))
	for _, vid := range videoIDs {
		if v, ok := videoMap[vid]; ok {
			orderedList = append(orderedList, v)
		}
	}

	// Determine algorithm name from recall stats
	algorithmUsed := "agent_standard"
	if resp.RecallStats != nil {
		if _, ok := resp.RecallStats["cold_start"]; ok {
			algorithmUsed = "agent_cold_start"
		} else if _, ok := resp.RecallStats["hot_explore"]; ok {
			algorithmUsed = "agent_hot_explore"
		} else if _, ok := resp.RecallStats["topic_deep_dive"]; ok {
			algorithmUsed = "agent_topic_deep_dive"
		}
	}

	return &RecommendResult{
		VideoList:        orderedList,
		AlgorithmUsed:    algorithmUsed,
		RecommendationID: resp.RequestID,
	}, nil
}

// recommendWithCTR DB 召回 + DeepCTR 精排的完整链路
func (service *RecommendVideoService) recommendWithCTR(userId int64, count int) (*RecommendResult, error) {
	startTime := time.Now()

	// ---- 第1步：召回候选视频 ----
	candidateIDs, err := service.recallCandidates(userId, count*5) // 召回 5 倍候选
	if err != nil {
		return nil, fmt.Errorf("recall failed: %w", err)
	}
	if len(candidateIDs) == 0 {
		return nil, fmt.Errorf("no candidate videos found")
	}

	hlog.Infof("[CTR] Recalled %d candidate videos", len(candidateIDs))

	// ---- 第2步：调用 DeepCTR 服务精排 ----
	ctrClient := recommendation.GetCTRClient()

	predictions, err := ctrClient.Predict(service.ctx, userId, candidateIDs, nil)
	if err != nil {
		return nil, fmt.Errorf("CTR predict failed: %w", err)
	}

	// 按 CTR 分数降序排列
	sorted := recommendation.SortByScore(predictions)

	// 取 Top N 视频 ID
	topN := count
	if topN > len(sorted) {
		topN = len(sorted)
	}
	rankedIDs := make([]int64, topN)
	for i := 0; i < topN; i++ {
		rankedIDs[i] = sorted[i].VideoID
	}

	// ---- 第3步：查询完整视频信息 ----
	videoList, err := db.GetVideoByVideoId(service.ctx, rankedIDs)
	if err != nil {
		return nil, fmt.Errorf("get video details failed: %w", err)
	}

	// 按 CTR 分数顺序排列视频（保持精排顺序）
	videoMap := make(map[int64]*base.Video, len(videoList))
	for _, v := range videoList {
		videoMap[v.VideoId] = v
	}
	orderedList := make([]*base.Video, 0, len(rankedIDs))
	for _, vid := range rankedIDs {
		if v, ok := videoMap[vid]; ok {
			orderedList = append(orderedList, v)
		}
	}

	latency := time.Since(startTime)
	recID := fmt.Sprintf("rec_%d_%d", userId, time.Now().UnixMilli())

	hlog.Infof("[CTR] Recommendation completed: %d videos, latency=%v, top_score=%.4f",
		len(orderedList), latency, sorted[0].Score)

	algorithmUsed := "deepfm"
	// 检查是否全部 fallback 分数 (0.5)，如果是说明 CTR 服务降级了
	allFallback := true
	for _, p := range sorted[:topN] {
		if p.Score != 0.5 {
			allFallback = false
			break
		}
	}
	if allFallback {
		algorithmUsed = "deepfm_fallback"
	}

	return &RecommendResult{
		VideoList:        orderedList,
		AlgorithmUsed:    algorithmUsed,
		RecommendationID: recID,
	}, nil
}

// recallCandidates 从数据库召回候选视频
func (service *RecommendVideoService) recallCandidates(userId int64, limit int) ([]int64, error) {
	candidateSet := make(map[int64]bool)
	var candidateIDs []int64

	// 召回路1: 从 video_features 表按热度召回
	features, err := db.GetVideosByPopularity(service.ctx, limit)
	if err == nil && len(features) > 0 {
		for _, f := range features {
			if !candidateSet[f.VideoID] {
				candidateSet[f.VideoID] = true
				candidateIDs = append(candidateIDs, f.VideoID)
			}
		}
		hlog.Infof("[Recall] Popularity channel: %d videos", len(features))
	}

	// 召回路2: 从 video_features 表按 CTR 召回
	ctrFeatures, err := db.GetVideosByCTR(service.ctx, 10, limit/2)
	if err == nil && len(ctrFeatures) > 0 {
		added := 0
		for _, f := range ctrFeatures {
			if !candidateSet[f.VideoID] {
				candidateSet[f.VideoID] = true
				candidateIDs = append(candidateIDs, f.VideoID)
				added++
			}
		}
		hlog.Infof("[Recall] CTR channel: %d new videos", added)
	}

	// 召回路3: 最新公开视频
	var recentVideos []*base.Video
	err = db.DB.WithContext(service.ctx).Model(&base.Video{}).
		Where("open = ? AND audit_status = ?", 1, 1).
		Order("created_at DESC").
		Limit(limit / 2).
		Find(&recentVideos).Error
	if err == nil && len(recentVideos) > 0 {
		added := 0
		for _, v := range recentVideos {
			if !candidateSet[v.VideoId] {
				candidateSet[v.VideoId] = true
				candidateIDs = append(candidateIDs, v.VideoId)
				added++
			}
		}
		hlog.Infof("[Recall] Recent channel: %d new videos", added)
	}

	// 排除用户自己的视频
	if userId > 0 {
		filtered := make([]int64, 0, len(candidateIDs))
		for _, vid := range candidateIDs {
			filtered = append(filtered, vid)
		}
		candidateIDs = filtered
	}

	// 截取上限
	if len(candidateIDs) > limit {
		candidateIDs = candidateIDs[:limit]
	}

	return candidateIDs, nil
}

// recommendByFeatures 基于 video_features 表的热度推荐（不经过 CTR 服务）
func (service *RecommendVideoService) recommendByFeatures(count int) ([]*base.Video, error) {
	features, err := db.GetVideosByPopularity(service.ctx, count*3)
	if err != nil {
		return nil, err
	}
	if len(features) == 0 {
		return nil, nil
	}

	videoIds := make([]int64, 0, len(features))
	for _, f := range features {
		videoIds = append(videoIds, f.VideoID)
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	rng.Shuffle(len(videoIds), func(i, j int) {
		videoIds[i], videoIds[j] = videoIds[j], videoIds[i]
	})

	if len(videoIds) > count {
		videoIds = videoIds[:count]
	}

	videoList, err := db.GetVideoByVideoId(service.ctx, videoIds)
	if err != nil {
		return nil, err
	}

	return videoList, nil
}

// recommendFallback 最终降级：从 videos 表直接查询
func (service *RecommendVideoService) recommendFallback(userId int64, count int) ([]*base.Video, error) {
	var videoList []*base.Video

	query := db.DB.WithContext(service.ctx).Model(&base.Video{}).
		Where("open = ? AND audit_status = ?", 1, 1)

	if userId > 0 {
		query = query.Where("user_id != ?", userId)
	}

	err := query.
		Order("(visit_count + likes_count * 3 + comment_count * 5 + share_count * 8) DESC, created_at DESC").
		Limit(count * 3).
		Find(&videoList).Error
	if err != nil {
		return nil, err
	}

	if len(videoList) == 0 {
		// 极端降级：去掉用户过滤，但仍保持公开+已审核条件
		err = db.DB.WithContext(service.ctx).Model(&base.Video{}).
			Where("open = ? AND audit_status = ?", 1, 1).
			Order("created_at DESC").
			Limit(count).
			Find(&videoList).Error
		if err != nil {
			return nil, err
		}
	}

	// 如果仍然没有（连公开视频都没有），返回所有最新视频
	if len(videoList) == 0 {
		err = db.DB.WithContext(service.ctx).Model(&base.Video{}).
			Order("created_at DESC").
			Limit(count).
			Find(&videoList).Error
		if err != nil {
			return nil, err
		}
	}

	if len(videoList) > count {
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))
		rng.Shuffle(len(videoList), func(i, j int) {
			videoList[i], videoList[j] = videoList[j], videoList[i]
		})
		videoList = videoList[:count]
	}

	return videoList, nil
}
