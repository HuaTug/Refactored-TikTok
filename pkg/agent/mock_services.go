package agent

import (
	"context"
	"fmt"
	"math/rand"
	"time"
)

// MockVideoService Mock视频服务实现
type MockVideoService struct {
	videos []Video
}

// MockUserService Mock用户服务实现
type MockUserService struct {
	userProfiles map[int64]*UserProfile
	interactions map[int64][]UserInteraction
}

// NewMockVideoService 创建Mock视频服务
func NewMockVideoService() *MockVideoService {
	return &MockVideoService{
		videos: generateMockVideos(),
	}
}

// NewMockUserService 创建Mock用户服务
func NewMockUserService() *MockUserService {
	return &MockUserService{
		userProfiles: make(map[int64]*UserProfile),
		interactions: make(map[int64][]UserInteraction),
	}
}

// GetVideosByCategory 根据类别获取视频
func (mvs *MockVideoService) GetVideosByCategory(ctx context.Context, category string, limit int) ([]Video, error) {
	var result []Video
	for _, video := range mvs.videos {
		if video.Category == category && len(result) < limit {
			result = append(result, video)
		}
	}
	return result, nil
}

// GetTrendingVideos 获取热门视频
func (mvs *MockVideoService) GetTrendingVideos(ctx context.Context, limit int) ([]Video, error) {
	// 按观看量排序返回热门视频
	var trendingVideos []Video
	for _, video := range mvs.videos {
		if video.ViewCount > 1000 { // 简单的热门标准
			trendingVideos = append(trendingVideos, video)
		}
	}

	// 限制数量
	if len(trendingVideos) > limit {
		trendingVideos = trendingVideos[:limit]
	}

	return trendingVideos, nil
}

// GetPersonalizedVideos 获取个性化视频
func (mvs *MockVideoService) GetPersonalizedVideos(ctx context.Context, userID int64, interests []string, limit int) ([]Video, error) {
	var result []Video
	interestMap := make(map[string]bool)
	for _, interest := range interests {
		interestMap[interest] = true
	}

	// 先按兴趣匹配
	for _, video := range mvs.videos {
		if interestMap[video.Category] && len(result) < limit {
			result = append(result, video)
		}
	}

	// 如果兴趣匹配不足，尝试标签匹配
	if len(result) < limit {
		for _, video := range mvs.videos {
			for _, tag := range video.Tags {
				if interestMap[tag] && len(result) < limit {
					result = append(result, video)
					break
				}
			}
		}
	}

	// 如果还是不足，添加高质量的视频
	if len(result) < limit {
		for _, video := range mvs.videos {
			if video.Quality.OverallScore > 0.7 && len(result) < limit {
				// 避免重复
				found := false
				for _, existingVideo := range result {
					if existingVideo.VideoID == video.VideoID {
						found = true
						break
					}
				}
				if !found {
					result = append(result, video)
				}
			}
		}
	}

	return result, nil
}

// GetSimilarVideos 获取相似视频
func (mvs *MockVideoService) GetSimilarVideos(ctx context.Context, videoID int64, limit int) ([]Video, error) {
	var targetVideo *Video
	for _, video := range mvs.videos {
		if video.VideoID == videoID {
			targetVideo = &video
			break
		}
	}

	if targetVideo == nil {
		return nil, fmt.Errorf("video not found: %d", videoID)
	}

	var result []Video
	for _, video := range mvs.videos {
		if video.VideoID != videoID && video.Category == targetVideo.Category && len(result) < limit {
			result = append(result, video)
		}
	}

	return result, nil
}

// GetVideosByTags 根据标签获取视频
func (mvs *MockVideoService) GetVideosByTags(ctx context.Context, tags []string, limit int) ([]Video, error) {
	tagMap := make(map[string]bool)
	for _, tag := range tags {
		tagMap[tag] = true
	}

	var result []Video
	for _, video := range mvs.videos {
		for _, videoTag := range video.Tags {
			if tagMap[videoTag] && len(result) < limit {
				result = append(result, video)
				break
			}
		}
	}

	return result, nil
}

// GetUserInteractionHistory 获取用户交互历史
func (mus *MockUserService) GetUserInteractionHistory(ctx context.Context, userID int64, days int) ([]UserInteraction, error) {
	if interactions, exists := mus.interactions[userID]; exists {
		cutoffTime := time.Now().AddDate(0, 0, -days)
		var result []UserInteraction
		for _, interaction := range interactions {
			if interaction.Timestamp.After(cutoffTime) {
				result = append(result, interaction)
			}
		}
		return result, nil
	}
	return []UserInteraction{}, nil
}

// GetUserProfile 获取用户画像
func (mus *MockUserService) GetUserProfile(ctx context.Context, userID int64) (*UserProfile, error) {
	if profile, exists := mus.userProfiles[userID]; exists {
		return profile, nil
	}

	// 如果不存在，创建默认画像
	profile := &UserProfile{
		InterestTags:     []string{"entertainment", "music"},
		ConsumptionLevel: ConsumptionMedium,
		ActiveHours:      []int{19, 20, 21, 22}, // 晚上活跃
		PreferredTopics: map[string]float64{
			"entertainment": 0.7,
			"music":         0.6,
			"sports":        0.4,
		},
	}

	mus.userProfiles[userID] = profile
	return profile, nil
}

// UpdateUserInterests 更新用户兴趣
func (mus *MockUserService) UpdateUserInterests(ctx context.Context, userID int64, interests []string) error {
	if profile, exists := mus.userProfiles[userID]; exists {
		profile.InterestTags = interests
	} else {
		profile := &UserProfile{
			InterestTags:     interests,
			ConsumptionLevel: ConsumptionMedium,
			ActiveHours:      []int{19, 20, 21, 22},
			PreferredTopics:  make(map[string]float64),
		}
		mus.userProfiles[userID] = profile
	}
	return nil
}

// generateMockVideos 生成Mock视频数据
func generateMockVideos() []Video {
	categories := []string{"entertainment", "education", "music", "sports", "technology", "food", "travel", "gaming"}
	videos := make([]Video, 0, 100)

	for i := 1; i <= 100; i++ {
		category := categories[rand.Intn(len(categories))]
		video := Video{
			VideoID:      int64(i),
			Title:        fmt.Sprintf("Amazing %s Video #%d", category, i),
			Description:  fmt.Sprintf("This is a wonderful %s video that will entertain you", category),
			Category:     category,
			Tags:         []string{category, "trending", "popular"},
			AuthorID:     int64(rand.Intn(20) + 1),
			AuthorName:   fmt.Sprintf("Creator%d", rand.Intn(20)+1),
			Duration:     time.Duration(rand.Intn(300)+30) * time.Second, // 30-330秒
			ViewCount:    int64(rand.Intn(10000) + 100),
			LikeCount:    int64(rand.Intn(1000) + 10),
			ShareCount:   int64(rand.Intn(100) + 1),
			CommentCount: int64(rand.Intn(200) + 5),
			CreatedAt:    time.Now().AddDate(0, 0, -rand.Intn(30)), // 最近30天内
			Quality: VideoQuality{
				OverallScore:     0.5 + rand.Float64()*0.4, // 0.5-0.9
				ContentQuality:   0.6 + rand.Float64()*0.3,
				TechnicalQuality: 0.7 + rand.Float64()*0.2,
				EngagementRate:   0.4 + rand.Float64()*0.5,
				FreshnessScore:   0.3 + rand.Float64()*0.6,
			},
			Metadata: map[string]interface{}{
				"language": "zh-CN",
				"region":   "China",
			},
		}
		videos = append(videos, video)
	}

	return videos
}
