package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	cache "HuaTug.com/pkg/infra/cache"
	"HuaTug.com/pkg/errno"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/gomodule/redigo/redis"
)

// ========== Search History ==========

const (
	searchHistoryPrefix = "search:history:"
	searchHistoryMax    = 20
	searchHistoryTTL    = 30 * 24 * 3600 // 30 days in seconds
)

// SearchHistoryItem represents a search history entry
type SearchHistoryItem struct {
	ID        string `json:"id"`
	Keyword   string `json:"keyword"`
	Timestamp int64  `json:"timestamp"`
}

// AddSearchHistory records a search keyword for a user
// POST /v1/search/history
func AddSearchHistory(ctx context.Context, c *app.RequestContext) {
	userID := getUserIDStr(c)
	if userID == "" {
		SendResponse(c, errno.AuthorizationFailedErr, nil)
		return
	}

	var req struct {
		Keyword string `json:"keyword" form:"keyword"`
	}
	if err := c.Bind(&req); err != nil || req.Keyword == "" {
		SendResponse(c, errno.ParamErr, nil)
		return
	}

	conn := cache.GetRedis()
	if conn == nil {
		hlog.Warn("[SearchHistory] Redis connection not available, using memory fallback")
		addSearchHistoryMemory(userID, req.Keyword)
		SendResponse(c, errno.Success, nil)
		return
	}
	defer conn.Close()

	key := searchHistoryPrefix + userID

	// Create history item
	item := SearchHistoryItem{
		ID:        fmt.Sprintf("%s_%d", req.Keyword, time.Now().UnixMilli()),
		Keyword:   req.Keyword,
		Timestamp: time.Now().Unix(),
	}
	data, _ := json.Marshal(item)

	// Remove existing item with same keyword first
	members, err := redis.Strings(conn.Do("ZRANGE", key, 0, -1))
	if err == nil {
		for _, m := range members {
			var existingItem SearchHistoryItem
			if json.Unmarshal([]byte(m), &existingItem) == nil && existingItem.Keyword == req.Keyword {
				conn.Do("ZREM", key, m)
				break
			}
		}
	}

	// Add to sorted set with timestamp as score
	conn.Do("ZADD", key, item.Timestamp, string(data))

	// Trim to max size (remove oldest)
	count, _ := redis.Int64(conn.Do("ZCARD", key))
	if count > searchHistoryMax {
		conn.Do("ZREMRANGEBYRANK", key, 0, count-searchHistoryMax-1)
	}

	// Set TTL
	conn.Do("EXPIRE", key, searchHistoryTTL)

	// Also store in memory as backup
	addSearchHistoryMemory(userID, req.Keyword)

	SendResponse(c, errno.Success, nil)
}

// GetSearchHistory retrieves search history for a user
// GET /v1/search/history
func GetSearchHistory(ctx context.Context, c *app.RequestContext) {
	userID := getUserIDStr(c)
	if userID == "" {
		SendResponse(c, errno.Success, map[string]interface{}{
			"history": []interface{}{},
		})
		return
	}

	conn := cache.GetRedis()
	if conn == nil {
		history := getSearchHistoryMemory(userID)
		SendResponse(c, errno.Success, map[string]interface{}{
			"history": history,
		})
		return
	}
	defer conn.Close()

	key := searchHistoryPrefix + userID

	// Get all items sorted by score (timestamp) descending
	members, err := redis.Strings(conn.Do("ZREVRANGE", key, 0, searchHistoryMax-1))
	if err != nil {
		hlog.Errorf("[SearchHistory] Redis ZREVRANGE failed: %v", err)
		history := getSearchHistoryMemory(userID)
		SendResponse(c, errno.Success, map[string]interface{}{
			"history": history,
		})
		return
	}

	var history []SearchHistoryItem
	for _, m := range members {
		var item SearchHistoryItem
		if json.Unmarshal([]byte(m), &item) == nil {
			history = append(history, item)
		}
	}

	if history == nil {
		history = []SearchHistoryItem{}
	}

	SendResponse(c, errno.Success, map[string]interface{}{
		"history": history,
	})
}

// DeleteSearchHistory deletes a specific search history item or all
// DELETE /v1/search/history
func DeleteSearchHistory(ctx context.Context, c *app.RequestContext) {
	userID := getUserIDStr(c)
	if userID == "" {
		SendResponse(c, errno.AuthorizationFailedErr, nil)
		return
	}

	keyword := c.Query("keyword")
	deleteAll := c.Query("all")

	conn := cache.GetRedis()
	key := searchHistoryPrefix + userID

	if deleteAll == "true" {
		if conn != nil {
			conn.Do("DEL", key)
			conn.Close()
		}
		clearSearchHistoryMemory(userID)
		SendResponse(c, errno.Success, nil)
		return
	}

	if keyword == "" {
		SendResponse(c, errno.ParamErr, nil)
		return
	}

	if conn != nil {
		defer conn.Close()
		members, _ := redis.Strings(conn.Do("ZRANGE", key, 0, -1))
		for _, m := range members {
			var item SearchHistoryItem
			if json.Unmarshal([]byte(m), &item) == nil && item.Keyword == keyword {
				conn.Do("ZREM", key, m)
				break
			}
		}
	}
	deleteSearchHistoryMemory(userID, keyword)

	SendResponse(c, errno.Success, nil)
}

// ========== Search Suggestions (猜你想搜) ==========

// GetSearchSuggestions generates personalized search suggestions
// GET /v1/search/suggestions
func GetSearchSuggestions(ctx context.Context, c *app.RequestContext) {
	userID := getUserIDStr(c)

	var suggestions []string

	// Strategy 1: Get recent search keywords from Redis
	conn := cache.GetRedis()
	if conn != nil && userID != "" {
		defer conn.Close()
		key := searchHistoryPrefix + userID
		members, _ := redis.Strings(conn.Do("ZREVRANGE", key, 0, 4))
		for _, m := range members {
			var item SearchHistoryItem
			if json.Unmarshal([]byte(m), &item) == nil {
				suggestions = append(suggestions, item.Keyword)
			}
		}
	}

	// Strategy 2: Add popular/trending keywords
	popularKeywords := []string{
		"美食教程", "旅行vlog", "编程教学", "健身训练",
		"音乐推荐", "科技数码", "搞笑视频", "生活日常",
		"学习方法", "摄影技巧",
	}

	seen := make(map[string]bool)
	for _, s := range suggestions {
		seen[s] = true
	}
	for _, kw := range popularKeywords {
		if !seen[kw] && len(suggestions) < 10 {
			suggestions = append(suggestions, kw)
			seen[kw] = true
		}
	}

	if len(suggestions) > 10 {
		suggestions = suggestions[:10]
	}

	SendResponse(c, errno.Success, map[string]interface{}{
		"suggestions": suggestions,
	})
}

// ========== Video Categories ==========

// VideoCategory represents a video category
type VideoCategory struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Icon string `json:"icon,omitempty"`
}

// GetVideoCategories returns the list of video categories
// GET /v1/video/categories
func GetVideoCategories(ctx context.Context, c *app.RequestContext) {
	categories := []VideoCategory{
		{ID: 1, Name: "娱乐", Icon: "🎬"},
		{ID: 2, Name: "音乐", Icon: "🎵"},
		{ID: 3, Name: "游戏", Icon: "🎮"},
		{ID: 4, Name: "知识", Icon: "📚"},
		{ID: 5, Name: "美食", Icon: "🍜"},
		{ID: 6, Name: "运动", Icon: "⚽"},
		{ID: 7, Name: "时尚", Icon: "👗"},
		{ID: 8, Name: "旅行", Icon: "✈️"},
		{ID: 9, Name: "科技", Icon: "💻"},
		{ID: 10, Name: "生活", Icon: "🏠"},
		{ID: 11, Name: "二次元", Icon: "🎨"},
		{ID: 12, Name: "汽车", Icon: "🚗"},
		{ID: 13, Name: "搞笑", Icon: "😂"},
		{ID: 14, Name: "影视", Icon: "🎥"},
		{ID: 15, Name: "教育", Icon: "🎓"},
		{ID: 16, Name: "其他", Icon: "📌"},
	}

	c.JSON(consts.StatusOK, map[string]interface{}{
		"code":    errno.SuccessCode,
		"message": "Success",
		"data": map[string]interface{}{
			"categories": categories,
		},
	})
}

// ========== Helpers ==========

func getUserIDStr(c *app.RequestContext) string {
	if uid, exists := c.Get("user_id"); exists {
		return fmt.Sprintf("%v", uid)
	}
	if uid := c.Query("user_id"); uid != "" {
		return uid
	}
	return ""
}

// ========== In-memory fallback ==========

var (
	memoryHistory   = make(map[string][]SearchHistoryItem)
	memoryHistoryMu sync.RWMutex
)

func addSearchHistoryMemory(userID, keyword string) {
	memoryHistoryMu.Lock()
	defer memoryHistoryMu.Unlock()

	items := memoryHistory[userID]
	for i, item := range items {
		if item.Keyword == keyword {
			items = append(items[:i], items[i+1:]...)
			break
		}
	}
	items = append([]SearchHistoryItem{{
		ID:        fmt.Sprintf("%s_%d", keyword, time.Now().UnixMilli()),
		Keyword:   keyword,
		Timestamp: time.Now().Unix(),
	}}, items...)

	if len(items) > searchHistoryMax {
		items = items[:searchHistoryMax]
	}
	memoryHistory[userID] = items
}

func getSearchHistoryMemory(userID string) []SearchHistoryItem {
	memoryHistoryMu.RLock()
	defer memoryHistoryMu.RUnlock()
	items := memoryHistory[userID]
	if items == nil {
		return []SearchHistoryItem{}
	}
	result := make([]SearchHistoryItem, len(items))
	copy(result, items)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp > result[j].Timestamp
	})
	return result
}

func clearSearchHistoryMemory(userID string) {
	memoryHistoryMu.Lock()
	defer memoryHistoryMu.Unlock()
	delete(memoryHistory, userID)
}

func deleteSearchHistoryMemory(userID, keyword string) {
	memoryHistoryMu.Lock()
	defer memoryHistoryMu.Unlock()
	items := memoryHistory[userID]
	for i, item := range items {
		if item.Keyword == keyword {
			memoryHistory[userID] = append(items[:i], items[i+1:]...)
			break
		}
	}
}
