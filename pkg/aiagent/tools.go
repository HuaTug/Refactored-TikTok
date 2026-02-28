package aiagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"HuaTug.com/cmd/api/rpc"
	"HuaTug.com/kitex_gen/videos"
	"HuaTug.com/pkg/recommendation"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/hertz/pkg/common/hlog"
)

// ===== Search Videos Tool =====

// SearchVideosInput defines the input schema for the search_videos tool.
type SearchVideosInput struct {
	Keyword string `json:"keyword" jsonschema:"description=The search keyword to find videos"`
	Limit   int    `json:"limit" jsonschema:"description=Maximum number of results (default 5)"`
}

// NewSearchVideosTool creates an Eino-compatible tool for searching videos on the platform.
func NewSearchVideosTool() tool.InvokableTool {
	t, err := utils.InferOptionableTool(
		"search_videos",
		"Search for videos by keyword on the ZhiShi platform. Use this tool when the user asks about finding videos or content.",
		func(ctx context.Context, input *SearchVideosInput, opts ...tool.Option) (string, error) {
			limit := input.Limit
			if limit <= 0 {
				limit = 5
			}
			resp, err := rpc.VideoSearch(ctx, &videos.VideoSearchRequestV2{
				Keyword:  input.Keyword,
				PageNum:  1,
				PageSize: int64(limit),
			})
			if err != nil || resp == nil || len(resp.VideoSearch) == 0 {
				return fmt.Sprintf("No videos found for keyword '%s'.", input.Keyword), nil
			}
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Found %d videos for '%s':\n\n", len(resp.VideoSearch), input.Keyword))
			for i, v := range resp.VideoSearch {
				sb.WriteString(fmt.Sprintf("Video %d:\n", i+1))
				sb.WriteString(fmt.Sprintf("  Title: %s\n", v.Title))
				if v.Description != "" {
					desc := v.Description
					if len([]rune(desc)) > 100 {
						desc = string([]rune(desc)[:100]) + "..."
					}
					sb.WriteString(fmt.Sprintf("  Description: %s\n", desc))
				}
				sb.WriteString(fmt.Sprintf("  Views: %d, Likes: %d, Comments: %d\n", v.VisitCount, v.LikesCount, v.CommentCount))
				if v.LabelNames != "" {
					sb.WriteString(fmt.Sprintf("  Tags: %s\n", v.LabelNames))
				}
				sb.WriteString("\n")
			}
			return sb.String(), nil
		},
	)
	if err != nil {
		hlog.Fatalf("[AI Agent] Failed to create search_videos tool: %v", err)
	}
	return t
}

// ===== Get Hot Topics Tool =====

// GetHotTopicsInput defines the input schema for the get_hot_topics tool.
type GetHotTopicsInput struct {
	Limit int `json:"limit" jsonschema:"description=Number of hot topics to return (default 10)"`
}

// NewGetHotTopicsTool creates an Eino-compatible tool for fetching trending content.
func NewGetHotTopicsTool() tool.InvokableTool {
	t, err := utils.InferOptionableTool(
		"get_hot_topics",
		"Get current hot/trending topics and popular videos on the platform. Use this when the user asks about trends or popular content.",
		func(ctx context.Context, input *GetHotTopicsInput, opts ...tool.Option) (string, error) {
			limit := input.Limit
			if limit <= 0 {
				limit = 10
			}
			// Try hot score service first
			service := recommendation.GetHotScoreService()
			if service != nil {
				videoIds, err := service.GetTopHotVideos(ctx, "24h", limit)
				if err == nil && len(videoIds) > 0 {
					var sb strings.Builder
					sb.WriteString(fmt.Sprintf("Top %d hot videos in the last 24 hours:\n\n", len(videoIds)))
					for i, vid := range videoIds {
						sb.WriteString(fmt.Sprintf("%d. Video ID: %d\n", i+1, vid))
					}
					trends, tErr := service.GetTrendingVideos(ctx, 5)
					if tErr == nil && len(trends) > 0 {
						sb.WriteString("\nTrending (fastest rising):\n")
						for _, t := range trends {
							sb.WriteString(fmt.Sprintf("  - Video ID %d (trend score: %.1f)\n", t.VideoID, t.TrendScore))
						}
					}
					return sb.String(), nil
				}
			}
			// Fallback: use VideoPopular RPC
			resp, err := rpc.VideoPopular(ctx, &videos.VideoPopularRequestV2{
				PageNum:  1,
				PageSize: int64(limit),
			})
			if err != nil || resp == nil || len(resp.Popular) == 0 {
				return "Currently unable to fetch hot topics. Please try again later.", nil
			}
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Top %d popular videos:\n\n", len(resp.Popular)))
			for i, v := range resp.Popular {
				sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, v.Title))
				if v.Description != "" {
					desc := v.Description
					if len([]rune(desc)) > 80 {
						desc = string([]rune(desc)[:80]) + "..."
					}
					sb.WriteString(fmt.Sprintf("   %s\n", desc))
				}
				sb.WriteString(fmt.Sprintf("   Views: %d | Likes: %d | Comments: %d\n", v.VisitCount, v.LikesCount, v.CommentCount))
				sb.WriteString("\n")
			}
			return sb.String(), nil
		},
	)
	if err != nil {
		hlog.Fatalf("[AI Agent] Failed to create get_hot_topics tool: %v", err)
	}
	return t
}

// ===== Content Strategy Tool =====

// SuggestContentStrategyInput defines the input schema for the suggest_content_strategy tool.
type SuggestContentStrategyInput struct {
	Topic       string `json:"topic" jsonschema:"description=The content topic or theme"`
	ContentType string `json:"content_type" jsonschema:"description=Type of content: video, image, or panoramic"`
}

// NewSuggestContentStrategyTool creates an Eino-compatible tool for content creation suggestions.
func NewSuggestContentStrategyTool() tool.InvokableTool {
	t, err := utils.InferOptionableTool(
		"suggest_content_strategy",
		"Provide content creation suggestions including title, description, tags, and best posting time based on current trends.",
		func(ctx context.Context, input *SuggestContentStrategyInput, opts ...tool.Option) (string, error) {
			contentType := input.ContentType
			if contentType == "" {
				contentType = "video"
			}
			var trendingInfo string
			resp, searchErr := rpc.VideoSearch(ctx, &videos.VideoSearchRequestV2{
				Keyword:  input.Topic,
				PageNum:  1,
				PageSize: 5,
			})
			if searchErr == nil && resp != nil && len(resp.VideoSearch) > 0 {
				var avgViews, avgLikes int64
				var topTags []string
				for _, v := range resp.VideoSearch {
					avgViews += v.VisitCount
					avgLikes += v.LikesCount
					if v.LabelNames != "" {
						topTags = append(topTags, v.LabelNames)
					}
				}
				avgViews /= int64(len(resp.VideoSearch))
				avgLikes /= int64(len(resp.VideoSearch))
				trendingInfo = fmt.Sprintf(`
Platform data for topic "%s":
- Existing videos: %d found
- Average views: %d
- Average likes: %d
- Common tags: %s
`, input.Topic, resp.Count, avgViews, avgLikes, strings.Join(topTags, ", "))
			}
			result := fmt.Sprintf(`Content strategy analysis for topic "%s" (type: %s):
%s
Best posting times (based on platform patterns):
- Weekdays: 12:00-13:00 (lunch), 18:00-20:00 (evening peak)
- Weekends: 10:00-12:00 (morning), 20:00-22:00 (highest engagement)
`, input.Topic, contentType, trendingInfo)
			return result, nil
		},
	)
	if err != nil {
		hlog.Fatalf("[AI Agent] Failed to create suggest_content_strategy tool: %v", err)
	}
	return t
}

// ===== RAG Knowledge Query Tool =====

// QueryKnowledgeInput defines the input schema for the query_knowledge tool.
type QueryKnowledgeInput struct {
	Query string `json:"query" jsonschema:"description=The query string to search in the platform knowledge base for relevant information"`
}

// NewQueryKnowledgeTool creates an Eino-compatible tool for RAG-based knowledge retrieval.
func NewQueryKnowledgeTool() tool.InvokableTool {
	t, err := utils.InferOptionableTool(
		"query_knowledge",
		"Search the platform's internal knowledge base for relevant information using RAG (Retrieval-Augmented Generation). Use this tool when the user asks about platform features, policies, help topics, or internal documentation.",
		func(ctx context.Context, input *QueryKnowledgeInput, opts ...tool.Option) (string, error) {
			retriever, err := NewMilvusRetriever(ctx)
			if err != nil {
				hlog.Errorf("[AI Agent] Failed to create retriever for knowledge query: %v", err)
				return "Knowledge base is currently unavailable.", nil
			}
			docs, err := retriever.Retrieve(ctx, input.Query)
			if err != nil {
				hlog.Errorf("[AI Agent] Knowledge retrieval failed: %v", err)
				return "Failed to search knowledge base.", nil
			}
			if len(docs) == 0 {
				return "No relevant knowledge found for this query.", nil
			}
			respBytes, _ := json.Marshal(docs)
			return string(respBytes), nil
		},
	)
	if err != nil {
		hlog.Fatalf("[AI Agent] Failed to create query_knowledge tool: %v", err)
	}
	return t
}

// ===== Get Current Time Tool =====

// GetCurrentTimeInput defines the input schema for the get_current_time tool.
type GetCurrentTimeInput struct{}

// NewGetCurrentTimeTool creates an Eino-compatible tool for getting the current time.
func NewGetCurrentTimeTool() tool.InvokableTool {
	t, err := utils.InferOptionableTool(
		"get_current_time",
		"Get the current date and time. Use this when you need to know the current time for time-sensitive queries.",
		func(ctx context.Context, input *GetCurrentTimeInput, opts ...tool.Option) (string, error) {
			return fmt.Sprintf("Current time: %s", time.Now().Format("2006-01-02 15:04:05")), nil
		},
	)
	if err != nil {
		hlog.Fatalf("[AI Agent] Failed to create get_current_time tool: %v", err)
	}
	return t
}

// =====================================================
// Recommendation-Specific Tools (Agent × Rec Integration)
// =====================================================

// ===== Get User Recommendation Tool =====

// GetUserRecommendationInput defines the input schema for the get_user_recommendation tool.
type GetUserRecommendationInput struct {
	UserID int64 `json:"user_id" jsonschema:"description=The user ID to generate recommendations for"`
	Limit  int   `json:"limit" jsonschema:"description=Maximum number of recommended videos (default 10)"`
}

// NewGetUserRecommendationTool creates an Eino-compatible tool that invokes
// RecommendationAgent.Recommend() and returns a formatted video list.
func NewGetUserRecommendationTool() tool.InvokableTool {
	t, err := utils.InferOptionableTool(
		"get_user_recommendation",
		"Generate personalized video recommendations for a user. The system uses an intelligent Agent "+
			"that dynamically selects the best recommendation strategy based on the user's current browsing state.",
		func(ctx context.Context, input *GetUserRecommendationInput, opts ...tool.Option) (string, error) {
			agent := recommendation.GetRecommendationAgent()
			if agent == nil {
				return "Recommendation service is not available.", nil
			}

			limit := input.Limit
			if limit <= 0 {
				limit = 10
			}

			req := &recommendation.RecommendRequest{
				UserID:      input.UserID,
				Limit:       limit,
				RequestID:   fmt.Sprintf("agent_tool_%d_%d", input.UserID, time.Now().UnixNano()),
				RequestType: "feed",
			}

			resp, err := agent.Recommend(ctx, req)
			if err != nil {
				return fmt.Sprintf("Failed to generate recommendations: %v", err), nil
			}

			if len(resp.Videos) == 0 {
				return "No recommendations available for this user at the moment.", nil
			}

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Recommended %d videos for user %d:\n\n", len(resp.Videos), input.UserID))
			for i, v := range resp.Videos {
				sb.WriteString(fmt.Sprintf("%d. Video ID: %d (score: %.3f", i+1, v.VideoID, v.Score))
				if v.RecallSource != "" {
					sb.WriteString(fmt.Sprintf(", source: %s", v.RecallSource))
				}
				sb.WriteString(")")
				if len(v.Reasons) > 0 {
					sb.WriteString(fmt.Sprintf(" — %s", strings.Join(v.Reasons, ", ")))
				}
				sb.WriteString("\n")
			}

			if resp.RecallStats != nil {
				sb.WriteString(fmt.Sprintf("\nRecall stats: %v\n", resp.RecallStats))
			}

			return sb.String(), nil
		},
	)
	if err != nil {
		hlog.Fatalf("[AI Agent] Failed to create get_user_recommendation tool: %v", err)
	}
	return t
}

// ===== Get User State Tool =====

// GetUserStateInput defines the input schema for the get_user_state tool.
type GetUserStateInput struct {
	UserID int64 `json:"user_id" jsonschema:"description=The user ID to query realtime state for"`
}

// NewGetUserStateTool creates an Eino-compatible tool that retrieves a user's
// realtime behavioral state for diagnostic or conversational use.
func NewGetUserStateTool() tool.InvokableTool {
	t, err := utils.InferOptionableTool(
		"get_user_state",
		"Get a user's real-time behavioral state including engagement level, browsing speed, "+
			"exploration diversity, focused topic, and skip patterns. Useful for understanding user behavior.",
		func(ctx context.Context, input *GetUserStateInput, opts ...tool.Option) (string, error) {
			svc := recommendation.GetRealtimeStateServiceInstance()
			if svc == nil {
				return "Realtime state service is not available.", nil
			}

			state, err := svc.GetUserRealtimeState(ctx, input.UserID)
			if err != nil {
				return fmt.Sprintf("Failed to get user state: %v", err), nil
			}

			data, err := json.Marshal(state)
			if err != nil {
				return fmt.Sprintf("Failed to serialize state: %v", err), nil
			}

			return fmt.Sprintf("User %d realtime state:\n%s", input.UserID, string(data)), nil
		},
	)
	if err != nil {
		hlog.Fatalf("[AI Agent] Failed to create get_user_state tool: %v", err)
	}
	return t
}

// ===== Get Video Hot Ranking Tool =====

// GetVideoHotRankingInput defines the input schema for the get_video_hot_ranking tool.
type GetVideoHotRankingInput struct {
	TimeWindow string `json:"time_window" jsonschema:"description=Time window for the ranking: 1h, 6h, 24h, or 7d (default 24h)"`
	Limit      int    `json:"limit" jsonschema:"description=Number of top videos to return (default 10)"`
}

// NewGetVideoHotRankingTool creates an Eino-compatible tool for fetching the hot video ranking.
func NewGetVideoHotRankingTool() tool.InvokableTool {
	t, err := utils.InferOptionableTool(
		"get_video_hot_ranking",
		"Get the current hot video ranking by time window. Shows the most popular videos "+
			"based on real-time interaction data (views, likes, comments, shares).",
		func(ctx context.Context, input *GetVideoHotRankingInput, opts ...tool.Option) (string, error) {
			service := recommendation.GetHotScoreService()
			if service == nil {
				return "Hot score service is not available.", nil
			}

			timeWindow := input.TimeWindow
			if timeWindow == "" {
				timeWindow = "24h"
			}
			limit := input.Limit
			if limit <= 0 {
				limit = 10
			}

			videoIDs, err := service.GetTopHotVideos(ctx, timeWindow, limit)
			if err != nil {
				return fmt.Sprintf("Failed to get hot ranking: %v", err), nil
			}

			if len(videoIDs) == 0 {
				return fmt.Sprintf("No hot videos found for time window '%s'.", timeWindow), nil
			}

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Top %d hot videos (window: %s):\n\n", len(videoIDs), timeWindow))
			for i, vid := range videoIDs {
				sb.WriteString(fmt.Sprintf("%d. Video ID: %d\n", i+1, vid))
			}

			return sb.String(), nil
		},
	)
	if err != nil {
		hlog.Fatalf("[AI Agent] Failed to create get_video_hot_ranking tool: %v", err)
	}
	return t
}
