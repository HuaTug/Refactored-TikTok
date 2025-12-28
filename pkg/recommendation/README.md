# 智能推荐系统使用指南

## 📖 概述

本推荐系统实现了完整的**召回-精排-重排**三阶段推荐流程,支持多种召回策略和个性化排序。

## 🏗️ 架构设计

```
┌─────────────────────────────────────────────────────────────┐
│                        推荐引擎                              │
│  ┌──────────────────────────────────────────────────────┐  │
│  │              第一阶段: 多路召回                        │  │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌─────────┐ │  │
│  │  │协同过滤  │ │热门视频  │ │内容召回  │ │社交召回 │ │  │
│  │  │  (30%)   │ │  (20%)   │ │  (25%)   │ │ (15%)   │ │  │
│  │  └────┬─────┘ └────┬─────┘ └────┬─────┘ └────┬────┘ │  │
│  │       │            │            │            │       │  │
│  │       └────────────┴────────────┴────────────┘       │  │
│  │                         ↓                            │  │
│  │               候选视频池 (300-500个)                  │  │
│  └──────────────────────────────────────────────────────┘  │
│                          ↓                                 │
│  ┌──────────────────────────────────────────────────────┐  │
│  │              第二阶段: 精排 (LTR)                     │  │
│  │  特征提取 → 模型打分 → 排序                           │  │
│  │  - 用户特征  - 视频特征  - 交叉特征                    │  │
│  └──────────────────────────────────────────────────────┘  │
│                          ↓                                 │
│  ┌──────────────────────────────────────────────────────┐  │
│  │              第三阶段: 重排 (多样性)                   │  │
│  │  MMR算法 → 过滤已看 → 最终推荐列表                     │  │
│  └──────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

## 🚀 快速开始

### 1. 初始化推荐引擎

```go
package main

import (
    "context"
    "HuaTug.com/pkg/recommendation"
    "github.com/go-redis/redis/v8"
)

func main() {
    // 初始化 Redis 客户端
    redisClient := redis.NewClient(&redis.Options{
        Addr:     "localhost:6379",
        Password: "",
        DB:       0,
    })
    
    // 创建推荐引擎
    engine := recommendation.NewRecommendationEngine(redisClient)
    
    // 生成推荐列表
    ctx := context.Background()
    userID := int64(12345)
    limit := 20
    
    videos, err := engine.Recommend(ctx, userID, limit)
    if err != nil {
        panic(err)
    }
    
    // 处理推荐结果
    for i, video := range videos {
        fmt.Printf("%d. 视频ID: %d, 分数: %.4f, 理由: %v\n", 
            i+1, video.VideoID, video.Score, video.Reasons)
    }
}
```

### 2. 集成到 API Gateway

```go
// cmd/api/handlers/feed_handler.go
package handlers

import (
    "context"
    "HuaTug.com/pkg/recommendation"
    "github.com/cloudwego/hertz/pkg/app"
)

type FeedHandler struct {
    recEngine *recommendation.RecommendationEngine
}

func (h *FeedHandler) GetRecommendFeed(ctx context.Context, c *app.RequestContext) {
    userID := c.GetInt64("user_id")
    limit := c.DefaultQuery("limit", "20")
    
    // 调用推荐引擎
    videos, err := h.recEngine.Recommend(ctx, userID, parseLimit(limit))
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    
    // 返回推荐结果
    c.JSON(200, gin.H{
        "videos": videos,
        "total":  len(videos),
    })
}
```

### 3. 更新用户画像

```go
// 用户观看视频
engine.UpdateUserProfile(ctx, userID, "view", videoID)

// 用户点赞视频
engine.UpdateUserProfile(ctx, userID, "like", videoID)

// 用户评论视频
engine.UpdateUserProfile(ctx, userID, "comment", videoID)

// 用户分享视频
engine.UpdateUserProfile(ctx, userID, "share", videoID)

// 用户完整观看视频
engine.UpdateUserProfile(ctx, userID, "finish", videoID)
```

## 📊 召回策略详解

### 1. 协同过滤召回 (30% 权重)

基于用户行为相似度推荐,原理: "喜欢相似内容的用户可能喜欢相同的视频"

**数据结构**:
```
Redis Key: user:similar:{userID}
Type: Sorted Set
Value: {similarUserID: similarity_score}

Redis Key: user:likes:{userID}
Type: Set
Value: {videoID1, videoID2, ...}
```

**示例**:
```bash
# 存储用户相似度
ZADD user:similar:12345 0.85 67890 0.78 54321

# 存储用户点赞
SADD user:likes:12345 1001 1002 1003
```

### 2. 热门视频召回 (20% 权重)

多时间窗口热榜,包括: 1小时热榜、24小时热榜、7天热榜

**数据结构**:
```
Redis Key: hot:video:hour:{YYYYMMDDHH}
Redis Key: hot:video:day:{YYYYMMDD}
Redis Key: hot:video:week
Type: Sorted Set
Value: {videoID: hot_score}
```

**示例**:
```bash
# 小时热榜
ZADD hot:video:hour:2025122818 95.5 2001 87.3 2002

# 日榜
ZADD hot:video:day:20251228 1250.8 2001

# 周榜
ZADD hot:video:week 8956.2 2001
```

### 3. 基于内容召回 (25% 权重)

根据用户兴趣标签匹配视频

**数据结构**:
```
Redis Key: user:interests:{userID}
Type: Sorted Set
Value: {tag: weight}

Redis Key: tag:videos:{tag}
Type: Sorted Set
Value: {videoID: relevance_score}
```

**示例**:
```bash
# 用户兴趣
ZADD user:interests:12345 0.95 "搞笑" 0.82 "美食" 0.75 "旅游"

# 标签视频
ZADD tag:videos:搞笑 98.5 3001 87.2 3002
```

### 4. 社交关系召回 (15% 权重)

推荐关注用户的最新视频

**数据结构**:
```
Redis Key: user:following:{userID}
Type: Set
Value: {authorID1, authorID2, ...}

Redis Key: author:videos:{authorID}
Type: Sorted Set (按时间排序)
Value: {videoID: timestamp}
```

### 5. 新视频探索召回 (10% 权重)

推荐最近24小时发布的新视频,解决冷启动问题

**数据结构**:
```
Redis Key: videos:timeline
Type: Sorted Set
Value: {videoID: timestamp}
```

## 🎯 精排模型 (LTR)

### 特征维度 (20+)

#### 用户特征
- `user_active_level`: 用户活跃度 [0-1]
- `user_avg_watch_time`: 平均观看时长 (秒)
- `user_interact_rate`: 互动率 [0-1]

#### 视频特征
- `video_quality_score`: 内容质量分 [0-1]
- `video_duration`: 视频时长 (秒)
- `video_freshness`: 新鲜度 [0-1] (时间衰减)
- `video_ctr`: 点击率 [0-1]
- `video_finish_rate`: 完播率 [0-1]
- `video_like_rate`: 点赞率 [0-1]
- `video_comment_rate`: 评论率 [0-1]
- `video_share_rate`: 分享率 [0-1]

#### 交叉特征
- `user_author_affinity`: 用户对作者的亲和度 [0-1]
- `user_category_match`: 用户对分类的匹配度 [0-1]
- `user_tag_overlap`: 用户兴趣标签重叠度 [0-1]

#### 上下文特征
- `time_match`: 时间匹配度 [0-1]
- `device_type`: 设备类型 (mobile=1, pc=0.8)
- `network_quality`: 网络质量 [0-1]

#### 热度特征
- `realtime_hot_score`: 实时热度 [0-1]
- `trending_score`: 趋势分 [0-1]

### 评分公式

```
Score = Σ (weight_i × normalize(feature_i))
Final_Score = sigmoid(Score)
```

### 特征权重分配

```go
weights := map[string]float64{
    // 质量和互动 (47%)
    "video_quality_score":   0.15,
    "video_ctr":             0.12,
    "video_finish_rate":     0.18,
    "video_like_rate":       0.08,
    "video_comment_rate":    0.05,
    "video_share_rate":      0.06,
    
    // 个性化 (26%)
    "user_author_affinity":  0.10,
    "user_category_match":   0.09,
    "user_tag_overlap":      0.07,
    
    // 新鲜度和热度 (19%)
    "video_freshness":       0.08,
    "realtime_hot_score":    0.06,
    "trending_score":        0.05,
    
    // 其他 (8%)
    "user_active_level":     0.05,
    "time_match":            0.04,
    "network_quality":       0.03,
}
```

## 🔄 重排序算法 (MMR)

**Maximal Marginal Relevance** - 平衡相关性和多样性

```
MMR_Score = λ × Relevance - (1-λ) × MaxSimilarity

λ = 0.7  (相关性权重)
1-λ = 0.3 (多样性权重)
```

**流程**:
1. 选择分数最高的视频作为种子
2. 对剩余视频计算MMR分数
3. 选择MMR分数最高的加入结果
4. 重复直到达到目标数量

## 📈 数据准备

### 初始化 Redis 数据

```bash
#!/bin/bash
# scripts/init_recommendation_data.sh

# 1. 生成用户相似度矩阵
python scripts/compute_user_similarity.py

# 2. 计算视频热度分
python scripts/compute_video_hot_score.py

# 3. 提取用户兴趣标签
python scripts/extract_user_interests.py

# 4. 建立标签-视频倒排索引
python scripts/build_tag_video_index.py
```

### 离线任务调度

```yaml
# cron jobs
schedule:
  - name: "user_similarity"
    cron: "0 2 * * *"  # 每天凌晨2点
    command: "python scripts/compute_user_similarity.py"
    
  - name: "video_hot_score"
    cron: "*/10 * * * *"  # 每10分钟
    command: "python scripts/compute_video_hot_score.py"
    
  - name: "user_interests"
    cron: "0 3 * * *"  # 每天凌晨3点
    command: "python scripts/extract_user_interests.py"
```

## 🔧 配置优化

### 召回策略权重调整

```go
// 修改 pkg/recommendation/recall_strategies.go
func (cf *CollaborativeFilteringRecall) Weight() float64 {
    return 0.35  // 提高协同过滤权重
}
```

### 精排特征权重调整

```go
// 修改 pkg/recommendation/ranking_model.go
weights := map[string]float64{
    "video_finish_rate": 0.20,  // 提高完播率权重
    "video_quality_score": 0.18,
    // ...
}
```

### MMR 多样性调整

```go
// 修改 pkg/recommendation/engine.go
lambda := 0.8  // 增大相关性权重,减少多样性
```

## 📊 效果评估

### 在线指标

- **CTR (点击率)**: 推荐视频被点击的比例
- **完播率**: 推荐视频被完整观看的比例
- **互动率**: 点赞/评论/分享的比例
- **人均观看时长**: 用户平均观看推荐视频的时长
- **留存率**: 用户次日/7日留存

### 离线指标

- **召回率**: 候选集中包含用户真实喜欢视频的比例
- **准确率**: 推荐列表中用户真实喜欢视频的比例
- **多样性**: 推荐列表的内容多样性
- **覆盖率**: 被推荐的视频占总视频的比例

### A/B测试

```go
// pkg/abtest/recommendation_test.go
type ABTestConfig struct {
    Name:           "recommendation_v2"
    TrafficPercent: 10  // 10%流量
    Variants: []Variant{
        {Name: "control", Percent: 50},
        {Name: "test", Percent: 50},
    }
}
```

## 🚀 性能优化

### 缓存策略

```go
// 缓存推荐结果 (5分钟)
key := fmt.Sprintf("rec:cache:%d", userID)
redis.Set(ctx, key, videos, 5*time.Minute)

// 缓存特征数据 (1小时)
featureKey := fmt.Sprintf("video:features:%d", videoID)
redis.Set(ctx, featureKey, features, 1*time.Hour)
```

### 异步计算

```go
// 异步更新用户画像
go engine.UpdateUserProfile(ctx, userID, "view", videoID)

// 异步记录推荐日志
go logRecommendation(userID, videos)
```

### 批量查询

```go
// 批量获取视频特征
videoIDs := []int64{1001, 1002, 1003}
features := batchGetVideoFeatures(ctx, videoIDs)
```

## 🔮 未来优化方向

1. **深度学习模型**
   - Wide & Deep
   - DeepFM
   - DIN (Deep Interest Network)

2. **实时特征**
   - Flink 实时计算
   - 特征存储 (Feature Store)

3. **强化学习**
   - Multi-Armed Bandit
   - 上下文老虎机

4. **图神经网络**
   - GraphSAGE
   - GAT (Graph Attention Network)

5. **冷启动优化**
   - 内容理解 (CV + NLP)
   - 迁移学习

## 📚 参考资料

- [YouTube推荐系统论文](https://research.google/pubs/pub45530/)
- [抖音推荐系统架构](https://mp.weixin.qq.com/s/xxx)
- [美团推荐系统实践](https://tech.meituan.com/xxx)

---

**维护者**: HuaTug Team  
**更新时间**: 2025-12-28
