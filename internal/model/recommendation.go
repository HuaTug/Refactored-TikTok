package model

import (
	"encoding/json"
	"time"
)

// =====================================================
// 推荐系统相关数据模型
// =====================================================

// UserProfile 用户画像持久化表
// 存储用户的兴趣偏好、行为特征等，用于个性化推荐
type UserProfile struct {
	UserID             int64            `json:"user_id" gorm:"column:user_id;primaryKey"`
	InterestTags       *json.RawMessage `json:"interest_tags" gorm:"column:interest_tags;type:json"`         // 兴趣标签权重 {"搞笑":0.8, "美食":0.6}
	CategoryPreference *json.RawMessage `json:"category_preference" gorm:"column:category_preference;type:json"` // 分类偏好 {"娱乐":0.9, "科技":0.5}
	AuthorPreference   *json.RawMessage `json:"author_preference" gorm:"column:author_preference;type:json"` // 喜好的作者ID列表及权重
	TopicPreference    *json.RawMessage `json:"topic_preference" gorm:"column:topic_preference;type:json"`   // 话题偏好
	ActiveTimeSlots    *json.RawMessage `json:"active_time_slots" gorm:"column:active_time_slots;type:json"` // 活跃时段 [8,9,12,18,19,20,21,22]
	AvgWatchDuration   float64          `json:"avg_watch_duration" gorm:"column:avg_watch_duration;default:0"` // 平均观看时长(秒)
	AvgCompletionRate  float64          `json:"avg_completion_rate" gorm:"column:avg_completion_rate;default:0"` // 平均完播率
	LikeRate           float64          `json:"like_rate" gorm:"column:like_rate;default:0"`     // 点赞率
	CommentRate        float64          `json:"comment_rate" gorm:"column:comment_rate;default:0"` // 评论率
	ShareRate          float64          `json:"share_rate" gorm:"column:share_rate;default:0"`    // 分享率
	TotalViewCount     int64            `json:"total_view_count" gorm:"column:total_view_count;default:0"` // 总观看数
	TotalLikeCount     int64            `json:"total_like_count" gorm:"column:total_like_count;default:0"` // 总点赞数
	TotalCommentCount  int64            `json:"total_comment_count" gorm:"column:total_comment_count;default:0"`
	TotalShareCount    int64            `json:"total_share_count" gorm:"column:total_share_count;default:0"`
	UserLevel          int8             `json:"user_level" gorm:"column:user_level;default:1"` // 用户活跃等级 1-5
	ContentQualityPref int8             `json:"content_quality_pref" gorm:"column:content_quality_pref;default:3"` // 内容质量偏好 1-5
	VideoDurationPref  int8             `json:"video_duration_pref" gorm:"column:video_duration_pref;default:2"`   // 视频时长偏好 1:短 2:中 3:长
	LastActiveAt       *time.Time       `json:"last_active_at" gorm:"column:last_active_at"`
	CreatedAt          time.Time        `json:"created_at" gorm:"column:created_at"`
	UpdatedAt          time.Time        `json:"updated_at" gorm:"column:updated_at"`
}

func (UserProfile) TableName() string {
	return "user_profiles"
}

// VideoFeature 视频特征/质量表
// 存储视频的各项指标，用于推荐排序
type VideoFeature struct {
	VideoID         int64     `json:"video_id" gorm:"column:video_id;primaryKey"`
	QualityScore    float64   `json:"quality_score" gorm:"column:quality_score;default:0"`    // 内容质量分 0-10
	PopularityScore float64   `json:"popularity_score" gorm:"column:popularity_score;default:0"` // 热度分
	FreshnessScore  float64   `json:"freshness_score" gorm:"column:freshness_score;default:0"`   // 新鲜度分 (时间衰减)
	CTR             float64   `json:"ctr" gorm:"column:ctr;default:0"`             // 点击通过率
	FinishRate      float64   `json:"finish_rate" gorm:"column:finish_rate;default:0"`      // 完播率
	LikeRate        float64   `json:"like_rate" gorm:"column:like_rate;default:0"`         // 点赞率
	CommentRate     float64   `json:"comment_rate" gorm:"column:comment_rate;default:0"`     // 评论率
	ShareRate       float64   `json:"share_rate" gorm:"column:share_rate;default:0"`       // 分享率
	FavoriteRate    float64   `json:"favorite_rate" gorm:"column:favorite_rate;default:0"`   // 收藏率
	InteractScore   float64   `json:"interact_score" gorm:"column:interact_score;default:0"` // 综合互动分
	AvgWatchDuration float64  `json:"avg_watch_duration" gorm:"column:avg_watch_duration;default:0"` // 平均观看时长
	ExposureCount   int64     `json:"exposure_count" gorm:"column:exposure_count;default:0"`  // 曝光次数
	ClickCount      int64     `json:"click_count" gorm:"column:click_count;default:0"`     // 点击次数
	AuthorScore     float64   `json:"author_score" gorm:"column:author_score;default:0"`    // 作者权重分
	IsHighQuality   int8      `json:"is_high_quality" gorm:"column:is_high_quality;default:0"` // 是否优质内容
	CreatedAt       time.Time `json:"created_at" gorm:"column:created_at"`
	UpdatedAt       time.Time `json:"updated_at" gorm:"column:updated_at"`
}

func (VideoFeature) TableName() string {
	return "video_features"
}

// VideoEmbedding 视频向量嵌入表
// 存储视频的特征向量，用于相似推荐和内容理解
type VideoEmbedding struct {
	VideoID        int64            `json:"video_id" gorm:"column:video_id;primaryKey"`
	EmbeddingType  string           `json:"embedding_type" gorm:"column:embedding_type;default:'content'"` // content/visual/audio/text
	EmbeddingVector *json.RawMessage `json:"embedding_vector" gorm:"column:embedding_vector;type:json"` // 向量数据 [0.1, 0.2, ...]
	Dimension      int              `json:"dimension" gorm:"column:dimension;default:128"` // 向量维度
	ModelVersion   string           `json:"model_version" gorm:"column:model_version;default:'v1'"`
	CreatedAt      time.Time        `json:"created_at" gorm:"column:created_at"`
	UpdatedAt      time.Time        `json:"updated_at" gorm:"column:updated_at"`
}

func (VideoEmbedding) TableName() string {
	return "video_embeddings"
}

// UserEmbedding 用户兴趣向量表
// 存储用户的兴趣向量，用于协同过滤推荐
type UserEmbedding struct {
	UserID          int64            `json:"user_id" gorm:"column:user_id;primaryKey"`
	EmbeddingType   string           `json:"embedding_type" gorm:"column:embedding_type;default:'interest'"` // interest/behavior/social
	EmbeddingVector *json.RawMessage `json:"embedding_vector" gorm:"column:embedding_vector;type:json"`
	Dimension       int              `json:"dimension" gorm:"column:dimension;default:128"`
	ModelVersion    string           `json:"model_version" gorm:"column:model_version;default:'v1'"`
	CreatedAt       time.Time        `json:"created_at" gorm:"column:created_at"`
	UpdatedAt       time.Time        `json:"updated_at" gorm:"column:updated_at"`
}

func (UserEmbedding) TableName() string {
	return "user_embeddings"
}

// VideoSimilarity 视频相似度表
// 预计算视频间的相似度，加速推荐
type VideoSimilarity struct {
	ID              int64     `json:"id" gorm:"column:id;primaryKey;autoIncrement"`
	VideoID         int64     `json:"video_id" gorm:"column:video_id;index:idx_video_sim"`
	SimilarVideoID  int64     `json:"similar_video_id" gorm:"column:similar_video_id;index:idx_video_sim"`
	SimilarityScore float64   `json:"similarity_score" gorm:"column:similarity_score"` // 相似度分数 0-1
	SimilarityType  string    `json:"similarity_type" gorm:"column:similarity_type;default:'content'"` // content/collaborative/tag
	CreatedAt       time.Time `json:"created_at" gorm:"column:created_at"`
}

func (VideoSimilarity) TableName() string {
	return "video_similarities"
}

// RecommendationExposure 推荐曝光记录表
// 记录推荐给用户的视频，用于去重和效果追踪
type RecommendationExposure struct {
	ID             int64     `json:"id" gorm:"column:id;primaryKey;autoIncrement"`
	UserID         int64     `json:"user_id" gorm:"column:user_id;index:idx_user_exposure"`
	VideoID        int64     `json:"video_id" gorm:"column:video_id;index:idx_user_exposure"`
	RecallSource   string    `json:"recall_source" gorm:"column:recall_source"` // 召回来源: hot/cf/content/social/new
	Position       int       `json:"position" gorm:"column:position"`        // 曝光位置
	Score          float64   `json:"score" gorm:"column:score"`           // 推荐分数
	IsClicked      int8      `json:"is_clicked" gorm:"column:is_clicked;default:0"`  // 是否点击
	IsLiked        int8      `json:"is_liked" gorm:"column:is_liked;default:0"`    // 是否点赞
	IsCommented    int8      `json:"is_commented" gorm:"column:is_commented;default:0"` // 是否评论
	IsShared       int8      `json:"is_shared" gorm:"column:is_shared;default:0"`   // 是否分享
	WatchDuration  int       `json:"watch_duration" gorm:"column:watch_duration;default:0"` // 观看时长(秒)
	CompletionRate float64   `json:"completion_rate" gorm:"column:completion_rate;default:0"` // 完播率
	ExposureTime   time.Time `json:"exposure_time" gorm:"column:exposure_time;index:idx_exposure_time"` // 曝光时间
	RequestID      string    `json:"request_id" gorm:"column:request_id;index"` // 请求ID，用于追踪
}

func (RecommendationExposure) TableName() string {
	return "recommendation_exposures"
}

// NegativeFeedback 用户负反馈表
// 记录用户"不感兴趣"等负反馈，用于过滤推荐
type NegativeFeedback struct {
	ID           int64      `json:"id" gorm:"column:id;primaryKey;autoIncrement"`
	UserID       int64      `json:"user_id" gorm:"column:user_id;index:idx_user_negative"`
	TargetType   int8       `json:"target_type" gorm:"column:target_type"` // 1:video 2:author 3:category 4:tag
	TargetID     int64      `json:"target_id" gorm:"column:target_id"`    // 目标ID (视频ID/作者ID等)
	TargetValue  *string    `json:"target_value" gorm:"column:target_value"` // 目标值 (分类名/标签名)
	FeedbackType int8       `json:"feedback_type" gorm:"column:feedback_type"` // 1:不感兴趣 2:看过了 3:内容重复 4:内容低质
	Reason       *string    `json:"reason" gorm:"column:reason"`
	ExpireAt     *time.Time `json:"expire_at" gorm:"column:expire_at"` // 过期时间 (部分负反馈可过期)
	CreatedAt    time.Time  `json:"created_at" gorm:"column:created_at"`
}

func (NegativeFeedback) TableName() string {
	return "negative_feedbacks"
}

// Negative feedback target type constants
const (
	NegativeFeedbackTargetVideo    = 1
	NegativeFeedbackTargetAuthor   = 2
	NegativeFeedbackTargetCategory = 3
	NegativeFeedbackTargetTag      = 4
)

// Negative feedback type constants
const (
	NegativeFeedbackNotInterested = 1
	NegativeFeedbackAlreadySeen   = 2
	NegativeFeedbackDuplicate     = 3
	NegativeFeedbackLowQuality    = 4
)

// VideoHotScore 视频实时热度表
// 按时间窗口统计视频热度，用于热门推荐
type VideoHotScore struct {
	ID            int64     `json:"id" gorm:"column:id;primaryKey;autoIncrement"`
	VideoID       int64     `json:"video_id" gorm:"column:video_id;index:idx_video_hot"`
	TimeWindow    string    `json:"time_window" gorm:"column:time_window;index:idx_video_hot"` // 时间窗口: 1h/6h/24h/7d
	ViewCount     int64     `json:"view_count" gorm:"column:view_count;default:0"`
	LikeCount     int64     `json:"like_count" gorm:"column:like_count;default:0"`
	CommentCount  int64     `json:"comment_count" gorm:"column:comment_count;default:0"`
	ShareCount    int64     `json:"share_count" gorm:"column:share_count;default:0"`
	HotScore      float64   `json:"hot_score" gorm:"column:hot_score;default:0;index:idx_hot_score"` // 综合热度分
	Rank          int       `json:"rank" gorm:"column:rank;default:0"` // 排名
	WindowStart   time.Time `json:"window_start" gorm:"column:window_start"`
	WindowEnd     time.Time `json:"window_end" gorm:"column:window_end"`
	UpdatedAt     time.Time `json:"updated_at" gorm:"column:updated_at"`
}

func (VideoHotScore) TableName() string {
	return "video_hot_scores"
}

// AuthorScore 作者评分表
// 存储创作者的质量评分，用于推荐加权
type AuthorScore struct {
	AuthorID          int64     `json:"author_id" gorm:"column:author_id;primaryKey"`
	QualityScore      float64   `json:"quality_score" gorm:"column:quality_score;default:0"`    // 内容质量分
	ActivityScore     float64   `json:"activity_score" gorm:"column:activity_score;default:0"`   // 活跃度分
	InfluenceScore    float64   `json:"influence_score" gorm:"column:influence_score;default:0"` // 影响力分
	GrowthScore       float64   `json:"growth_score" gorm:"column:growth_score;default:0"`     // 成长潜力分
	OverallScore      float64   `json:"overall_score" gorm:"column:overall_score;default:0"`    // 综合评分
	TotalVideos       int64     `json:"total_videos" gorm:"column:total_videos;default:0"`
	AvgVideoQuality   float64   `json:"avg_video_quality" gorm:"column:avg_video_quality;default:0"`
	AvgVideoViews     float64   `json:"avg_video_views" gorm:"column:avg_video_views;default:0"`
	AvgEngagementRate float64   `json:"avg_engagement_rate" gorm:"column:avg_engagement_rate;default:0"`
	LastPublishAt     *time.Time `json:"last_publish_at" gorm:"column:last_publish_at"`
	Level             int8      `json:"level" gorm:"column:level;default:1"` // 作者等级 1-10
	IsVerified        int8      `json:"is_verified" gorm:"column:is_verified;default:0"` // 是否认证
	CreatedAt         time.Time `json:"created_at" gorm:"column:created_at"`
	UpdatedAt         time.Time `json:"updated_at" gorm:"column:updated_at"`
}

func (AuthorScore) TableName() string {
	return "author_scores"
}

// RecommendationBloomFilter 推荐布隆过滤器状态表
// 存储用户的布隆过滤器状态，用于高效去重
type RecommendationBloomFilter struct {
	UserID      int64     `json:"user_id" gorm:"column:user_id;primaryKey"`
	FilterData  []byte    `json:"filter_data" gorm:"column:filter_data;type:blob"` // 布隆过滤器序列化数据
	FilterSize  int64     `json:"filter_size" gorm:"column:filter_size"`        // 过滤器大小
	ItemCount   int64     `json:"item_count" gorm:"column:item_count;default:0"` // 已添加元素数量
	LastResetAt time.Time `json:"last_reset_at" gorm:"column:last_reset_at"`    // 上次重置时间
	CreatedAt   time.Time `json:"created_at" gorm:"column:created_at"`
	UpdatedAt   time.Time `json:"updated_at" gorm:"column:updated_at"`
}

func (RecommendationBloomFilter) TableName() string {
	return "recommendation_bloom_filters"
}

// TagVideoMapping 标签-视频映射表
// 用于基于标签的内容推荐
type TagVideoMapping struct {
	ID        int64     `json:"id" gorm:"column:id;primaryKey;autoIncrement"`
	TagName   string    `json:"tag_name" gorm:"column:tag_name;index:idx_tag_video"`
	VideoID   int64     `json:"video_id" gorm:"column:video_id;index:idx_tag_video"`
	Weight    float64   `json:"weight" gorm:"column:weight;default:1.0"` // 标签权重
	Source    string    `json:"source" gorm:"column:source;default:'manual'"` // 来源: manual/ai/user
	CreatedAt time.Time `json:"created_at" gorm:"column:created_at"`
}

func (TagVideoMapping) TableName() string {
	return "tag_video_mappings"
}

// CategoryVideoStats 分类视频统计表
// 用于按分类推荐
type CategoryVideoStats struct {
	ID           int64     `json:"id" gorm:"column:id;primaryKey;autoIncrement"`
	Category     string    `json:"category" gorm:"column:category;uniqueIndex"`
	TotalVideos  int64     `json:"total_videos" gorm:"column:total_videos;default:0"`
	TotalViews   int64     `json:"total_views" gorm:"column:total_views;default:0"`
	TotalLikes   int64     `json:"total_likes" gorm:"column:total_likes;default:0"`
	AvgQuality   float64   `json:"avg_quality" gorm:"column:avg_quality;default:0"`
	HotScore     float64   `json:"hot_score" gorm:"column:hot_score;default:0"`
	DailyNewVideos int64   `json:"daily_new_videos" gorm:"column:daily_new_videos;default:0"`
	UpdatedAt    time.Time `json:"updated_at" gorm:"column:updated_at"`
}

func (CategoryVideoStats) TableName() string {
	return "category_video_stats"
}

// UserVideoInteraction 用户视频详细交互记录表
// 补充现有的 user_behaviors，记录更详细的交互数据
type UserVideoInteraction struct {
	ID                int64     `json:"id" gorm:"column:id;primaryKey;autoIncrement"`
	UserID            int64     `json:"user_id" gorm:"column:user_id;index:idx_user_video_interact"`
	VideoID           int64     `json:"video_id" gorm:"column:video_id;index:idx_user_video_interact"`
	ImpressionCount   int       `json:"impression_count" gorm:"column:impression_count;default:0"`   // 曝光次数
	ClickCount        int       `json:"click_count" gorm:"column:click_count;default:0"`        // 点击次数
	TotalWatchTime    int       `json:"total_watch_time" gorm:"column:total_watch_time;default:0"` // 总观看时长(秒)
	MaxWatchProgress  float64   `json:"max_watch_progress" gorm:"column:max_watch_progress;default:0"` // 最大观看进度
	LastWatchPosition int       `json:"last_watch_position" gorm:"column:last_watch_position;default:0"` // 上次观看位置
	ReplayCount       int       `json:"replay_count" gorm:"column:replay_count;default:0"`      // 重播次数
	IsLiked           int8      `json:"is_liked" gorm:"column:is_liked;default:0"`
	IsFavorited       int8      `json:"is_favorited" gorm:"column:is_favorited;default:0"`
	IsShared          int8      `json:"is_shared" gorm:"column:is_shared;default:0"`
	CommentCount      int       `json:"comment_count" gorm:"column:comment_count;default:0"`
	EngagementScore   float64   `json:"engagement_score" gorm:"column:engagement_score;default:0"` // 综合互动分
	FirstInteractAt   time.Time `json:"first_interact_at" gorm:"column:first_interact_at"`
	LastInteractAt    time.Time `json:"last_interact_at" gorm:"column:last_interact_at"`
}

func (UserVideoInteraction) TableName() string {
	return "user_video_interactions"
}

// ABTestExperiment A/B测试实验表
// 用于推荐算法的 A/B 测试
type ABTestExperiment struct {
	ID             int64            `json:"id" gorm:"column:id;primaryKey;autoIncrement"`
	ExperimentName string           `json:"experiment_name" gorm:"column:experiment_name;uniqueIndex"`
	Description    *string          `json:"description" gorm:"column:description"`
	TrafficRatio   float64          `json:"traffic_ratio" gorm:"column:traffic_ratio;default:0"` // 实验流量比例
	Status         int8             `json:"status" gorm:"column:status;default:0"` // 0:draft 1:running 2:paused 3:finished
	Config         *json.RawMessage `json:"config" gorm:"column:config;type:json"` // 实验配置
	Metrics        *json.RawMessage `json:"metrics" gorm:"column:metrics;type:json"` // 实验指标结果
	StartTime      *time.Time       `json:"start_time" gorm:"column:start_time"`
	EndTime        *time.Time       `json:"end_time" gorm:"column:end_time"`
	CreatedAt      time.Time        `json:"created_at" gorm:"column:created_at"`
	UpdatedAt      time.Time        `json:"updated_at" gorm:"column:updated_at"`
}

func (ABTestExperiment) TableName() string {
	return "ab_test_experiments"
}

// ABTestGroup A/B测试分组表
type ABTestGroup struct {
	ID           int64            `json:"id" gorm:"column:id;primaryKey;autoIncrement"`
	ExperimentID int64            `json:"experiment_id" gorm:"column:experiment_id;index"`
	GroupName    string           `json:"group_name" gorm:"column:group_name"` // control/treatment_a/treatment_b
	TrafficRatio float64          `json:"traffic_ratio" gorm:"column:traffic_ratio;default:0"`
	Config       *json.RawMessage `json:"config" gorm:"column:config;type:json"` // 分组特定配置
	CreatedAt    time.Time        `json:"created_at" gorm:"column:created_at"`
}

func (ABTestGroup) TableName() string {
	return "ab_test_groups"
}

// UserABTestAssignment 用户A/B测试分配表
type UserABTestAssignment struct {
	ID           int64     `json:"id" gorm:"column:id;primaryKey;autoIncrement"`
	UserID       int64     `json:"user_id" gorm:"column:user_id;index:idx_user_ab"`
	ExperimentID int64     `json:"experiment_id" gorm:"column:experiment_id;index:idx_user_ab"`
	GroupID      int64     `json:"group_id" gorm:"column:group_id"`
	AssignedAt   time.Time `json:"assigned_at" gorm:"column:assigned_at"`
}

func (UserABTestAssignment) TableName() string {
	return "user_ab_test_assignments"
}
