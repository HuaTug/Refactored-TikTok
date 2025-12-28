package kafka

import "time"

// Topic 定义
const (
	// 用户行为 Topics
	TopicUserBehavior    = "user_behavior"     // 用户行为日志
	TopicVideoView       = "video_view"        // 视频播放事件
	TopicVideoExposure   = "video_exposure"    // 视频曝光事件
	TopicSearchLog       = "search_log"        // 搜索日志
	TopicUserActivityLog = "user_activity_log" // 用户活动日志

	// 推荐系统 Topics
	TopicRecommendation = "recommendation"       // 推荐事件
	TopicUserProfile    = "user_profile_update"  // 用户画像更新
	TopicVideoFeature   = "video_feature_update" // 视频特征更新

	// 实时统计 Topics
	TopicRealtimeStats = "realtime_stats" // 实时统计
	TopicVideoStats    = "video_stats"    // 视频统计

	// CDC (Change Data Capture) Topics
	TopicCDCVideo = "cdc_video" // 视频数据变更
	TopicCDCUser  = "cdc_user"  // 用户数据变更
)

// ConsumerGroup 定义
const (
	GroupAnalytics     = "analytics_group"      // 数据分析消费组
	GroupRecommend     = "recommend_group"      // 推荐系统消费组
	GroupRealtime      = "realtime_group"       // 实时处理消费组
	GroupDataWarehouse = "data_warehouse_group" // 数据仓库消费组
)

// BehaviorType 用户行为类型
type BehaviorType string

const (
	BehaviorPlay       BehaviorType = "play"        // 播放
	BehaviorPause      BehaviorType = "pause"       // 暂停
	BehaviorResume     BehaviorType = "resume"      // 继续播放
	BehaviorComplete   BehaviorType = "complete"    // 完播
	BehaviorSkip       BehaviorType = "skip"        // 跳过
	BehaviorLike       BehaviorType = "like"        // 点赞
	BehaviorUnlike     BehaviorType = "unlike"      // 取消点赞
	BehaviorComment    BehaviorType = "comment"     // 评论
	BehaviorShare      BehaviorType = "share"       // 分享
	BehaviorFollow     BehaviorType = "follow"      // 关注
	BehaviorUnfollow   BehaviorType = "unfollow"    // 取消关注
	BehaviorSearch     BehaviorType = "search"      // 搜索
	BehaviorScrollUp   BehaviorType = "scroll_up"   // 上滑
	BehaviorScrollDown BehaviorType = "scroll_down" // 下滑
	BehaviorClick      BehaviorType = "click"       // 点击
	BehaviorStay       BehaviorType = "stay"        // 停留
)

// UserBehaviorEvent 用户行为事件 - 高吞吐量场景
type UserBehaviorEvent struct {
	EventID    string            `json:"event_id"`    // 事件唯一ID
	UserID     int64             `json:"user_id"`     // 用户ID
	VideoID    int64             `json:"video_id"`    // 视频ID (可选)
	Behavior   BehaviorType      `json:"behavior"`    // 行为类型
	Timestamp  time.Time         `json:"timestamp"`   // 事件时间戳
	Duration   int64             `json:"duration"`    // 持续时长(ms)
	Position   int64             `json:"position"`    // 播放位置(ms)
	DeviceType string            `json:"device_type"` // 设备类型
	Platform   string            `json:"platform"`    // 平台 (ios/android/web)
	AppVersion string            `json:"app_version"` // 应用版本
	IP         string            `json:"ip"`          // 用户IP
	Location   string            `json:"location"`    // 地理位置
	SessionID  string            `json:"session_id"`  // 会话ID
	Extra      map[string]string `json:"extra"`       // 扩展字段
}

// VideoViewEvent 视频播放事件 - 播放统计
type VideoViewEvent struct {
	EventID       string    `json:"event_id"`       // 事件唯一ID
	VideoID       int64     `json:"video_id"`       // 视频ID
	UserID        int64     `json:"user_id"`        // 用户ID
	AuthorID      int64     `json:"author_id"`      // 作者ID
	Timestamp     time.Time `json:"timestamp"`      // 播放时间
	WatchTime     int64     `json:"watch_time"`     // 观看时长(ms)
	VideoDuration int64     `json:"video_duration"` // 视频总时长(ms)
	WatchPercent  float64   `json:"watch_percent"`  // 观看百分比
	IsComplete    bool      `json:"is_complete"`    // 是否完播
	Source        string    `json:"source"`         // 来源 (feed/search/profile/share)
	DeviceType    string    `json:"device_type"`    // 设备类型
	Quality       string    `json:"quality"`        // 播放质量
}

// VideoExposureEvent 视频曝光事件 - 用于推荐效果评估
type VideoExposureEvent struct {
	EventID    string    `json:"event_id"`    // 事件唯一ID
	UserID     int64     `json:"user_id"`     // 用户ID
	VideoIDs   []int64   `json:"video_ids"`   // 曝光的视频列表
	Timestamp  time.Time `json:"timestamp"`   // 曝光时间
	Source     string    `json:"source"`      // 来源 (feed/search/recommend)
	Position   int       `json:"position"`    // 在列表中的位置
	RecallType string    `json:"recall_type"` // 召回类型
	ModelScore float64   `json:"model_score"` // 模型预估分数
}

// SearchLogEvent 搜索日志事件
type SearchLogEvent struct {
	EventID     string    `json:"event_id"`     // 事件唯一ID
	UserID      int64     `json:"user_id"`      // 用户ID
	Query       string    `json:"query"`        // 搜索词
	Timestamp   time.Time `json:"timestamp"`    // 搜索时间
	ResultCount int       `json:"result_count"` // 结果数量
	ClickedIDs  []int64   `json:"clicked_ids"`  // 点击的视频ID
	Filter      string    `json:"filter"`       // 过滤条件
	SortBy      string    `json:"sort_by"`      // 排序方式
}

// UserProfileUpdateEvent 用户画像更新事件 - 推荐系统
type UserProfileUpdateEvent struct {
	EventID     string             `json:"event_id"`     // 事件唯一ID
	UserID      int64              `json:"user_id"`      // 用户ID
	Timestamp   time.Time          `json:"timestamp"`    // 更新时间
	UpdateType  string             `json:"update_type"`  // 更新类型 (interest/tag/preference)
	Tags        []string           `json:"tags"`         // 兴趣标签
	Categories  []string           `json:"categories"`   // 偏好分类
	Scores      map[string]float64 `json:"scores"`       // 兴趣分数
	ActiveHours []int              `json:"active_hours"` // 活跃时段
}

// VideoFeatureUpdateEvent 视频特征更新事件 - 推荐系统
type VideoFeatureUpdateEvent struct {
	EventID      string            `json:"event_id"`      // 事件唯一ID
	VideoID      int64             `json:"video_id"`      // 视频ID
	Timestamp    time.Time         `json:"timestamp"`     // 更新时间
	UpdateType   string            `json:"update_type"`   // 更新类型 (stats/tag/embedding)
	PlayCount    int64             `json:"play_count"`    // 播放次数
	LikeCount    int64             `json:"like_count"`    // 点赞数
	CommentCount int64             `json:"comment_count"` // 评论数
	ShareCount   int64             `json:"share_count"`   // 分享数
	Tags         []string          `json:"tags"`          // 视频标签
	Categories   []string          `json:"categories"`    // 视频分类
	HotScore     float64           `json:"hot_score"`     // 热度分数
	QualityScore float64           `json:"quality_score"` // 质量分数
	Extra        map[string]string `json:"extra"`         // 扩展字段
}

// RealtimeStatsEvent 实时统计事件
type RealtimeStatsEvent struct {
	EventID    string            `json:"event_id"`    // 事件唯一ID
	MetricName string            `json:"metric_name"` // 指标名称
	MetricType string            `json:"metric_type"` // 指标类型 (counter/gauge/histogram)
	Value      float64           `json:"value"`       // 指标值
	Timestamp  time.Time         `json:"timestamp"`   // 时间戳
	Dimensions map[string]string `json:"dimensions"`  // 维度信息
}

// VideoStatsEvent 视频统计事件 - 实时更新视频计数
type VideoStatsEvent struct {
	EventID   string    `json:"event_id"`   // 事件唯一ID
	VideoID   int64     `json:"video_id"`   // 视频ID
	StatsType string    `json:"stats_type"` // 统计类型 (play/like/comment/share)
	Delta     int64     `json:"delta"`      // 增量值 (+1 或 -1)
	Timestamp time.Time `json:"timestamp"`  // 事件时间
}

// CDCEvent CDC (Change Data Capture) 事件
type CDCEvent struct {
	EventID    string                 `json:"event_id"`    // 事件唯一ID
	TableName  string                 `json:"table_name"`  // 表名
	Operation  string                 `json:"operation"`   // 操作类型 (insert/update/delete)
	Timestamp  time.Time              `json:"timestamp"`   // 变更时间
	Before     map[string]interface{} `json:"before"`      // 变更前数据
	After      map[string]interface{} `json:"after"`       // 变更后数据
	PrimaryKey map[string]interface{} `json:"primary_key"` // 主键
}

// RecommendationEvent 推荐事件 - 记录推荐结果
type RecommendationEvent struct {
	EventID      string            `json:"event_id"`      // 事件唯一ID
	UserID       int64             `json:"user_id"`       // 用户ID
	Timestamp    time.Time         `json:"timestamp"`     // 推荐时间
	RecallType   string            `json:"recall_type"`   // 召回类型
	VideoIDs     []int64           `json:"video_ids"`     // 推荐的视频列表
	Scores       []float64         `json:"scores"`        // 推荐分数
	RequestID    string            `json:"request_id"`    // 请求ID
	ABTestGroup  string            `json:"ab_test_group"` // AB测试分组
	ModelVersion string            `json:"model_version"` // 模型版本
	Extra        map[string]string `json:"extra"`         // 扩展字段
}
