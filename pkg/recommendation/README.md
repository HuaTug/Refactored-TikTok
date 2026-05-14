# 智能推荐系统

本包实现 **召回 — 精排 — 重排** 三阶段推荐流程，由 `integrated_engine.go`
统一对外提供能力，真实调用方见 `cmd/video/service/videorecommend.go`。

## 架构

```
┌────────────────────────────────────────────────────────────┐
│                      推荐引擎                               │
│  ┌──────────────────────────────────────────────────────┐  │
│  │            第一阶段：多路召回（recall_*）              │  │
│  │   协同过滤 │ 热门 │ 内容 │ 社交 │ 探索 │ 实时 │ 兴趣   │  │
│  │                        ↓                             │  │
│  │                候选池（300 – 500 视频）              │  │
│  └──────────────────────────────────────────────────────┘  │
│                            ↓                               │
│  ┌──────────────────────────────────────────────────────┐  │
│  │      第二阶段：粗排（ranking_enhanced.go）            │  │
│  │      18 维特征打分，筛选 Top-N                        │  │
│  └──────────────────────────────────────────────────────┘  │
│                            ↓                               │
│  ┌──────────────────────────────────────────────────────┐  │
│  │      第三阶段：精排（ctr_client.go + DeepCTR）         │  │
│  │      DeepFM / DIN / MMoE 打分                         │  │
│  └──────────────────────────────────────────────────────┘  │
│                            ↓                               │
│  ┌──────────────────────────────────────────────────────┐  │
│  │      第四阶段：重排（MMR 多样性 + 个性化 boost）      │  │
│  └──────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────┘
```

## 主要文件

| 文件 | 作用 |
|---|---|
| `integrated_engine.go` | 对外入口，串联召回 / 粗排 / CTR / 重排 |
| `engine_enhanced.go` | 多路召回引擎增强版 |
| `recall_strategies.go` / `recall_enhanced.go` | 各路召回策略实现 |
| `ranking_model.go` / `ranking_enhanced.go` | 粗排特征与打分 |
| `ctr_client.go` | 调用外部 DeepCTR FastAPI（:8000） |
| `decision_strategy.go` | 策略路由（冷启动 / 标准 / 个性化） |
| `recommendation_agent.go` | Agent 封装，感知用户实时行为 |
| `hot_score_service.go` / `hot_score_init.go` | 热度分计算与初始化 |
| `realtime_state.go` | 用户实时状态（Redis ZSET） |
| `user_profile_service.go` | 用户画像读写 |

## 召回策略

### 1. 协同过滤（约 30% 权重）

基于"喜欢相似内容的用户可能喜欢相同的视频"。

```
user:similar:{userID}   ZSET  {similarUserID: similarity}
user:likes:{userID}     SET   {videoID, ...}
```

### 2. 热门视频（约 20%）

多时间窗口热榜（小时 / 日 / 周）。

```
hot:video:hour:{YYYYMMDDHH}   ZSET  {videoID: hot_score}
hot:video:day:{YYYYMMDD}      ZSET
hot:video:week                ZSET
```

### 3. 内容召回（约 25%）

基于用户兴趣标签匹配。

```
user:interests:{userID}  ZSET  {tag: weight}
tag:videos:{tag}         ZSET  {videoID: relevance}
```

### 4. 社交召回（约 15%）

推荐关注作者的新视频。

```
user:following:{userID}    SET   {authorID, ...}
author:videos:{authorID}   ZSET  {videoID: timestamp}
```

### 5. 新视频探索（约 10%）

解决冷启动，推近 24 小时的新视频。

```
videos:timeline    ZSET   {videoID: timestamp}
```

## 粗排特征（18 维）

**视频维度**
- `video_quality_score` — 内容质量分
- `video_ctr` — 点击率
- `video_finish_rate` — 完播率
- `video_like_rate` / `video_comment_rate` / `video_share_rate`
- `video_freshness` — 新鲜度（时间衰减）
- `video_duration` — 时长

**用户维度**
- `user_active_level` — 活跃度
- `user_avg_watch_time` — 平均观看时长
- `user_interact_rate` — 互动率

**交叉维度**
- `user_author_affinity` — 对作者的亲和度
- `user_category_match` — 分类匹配度
- `user_tag_overlap` — 标签重叠度

**上下文与热度**
- `time_match` / `device_type` / `network_quality`
- `realtime_hot_score` / `trending_score`

## 精排（DeepCTR）

通过 HTTP 调用外部推理服务，见 `DeepCTR/tiktok_rec_service/serve.py`：

```
POST http://localhost:8000/predict
POST http://localhost:8000/predict/ensemble
```

支持 DeepFM / DIN / MMoE 三模型，通过 `POST /reload` 热更新。

## MMR 重排序

Maximal Marginal Relevance：

```
MMR = λ × Relevance − (1 − λ) × max Similarity
λ ≈ 0.7（相关性）   1 − λ ≈ 0.3（多样性）
```

## 个性化 Boost（重排后置 boost）

`integrated_engine.go::applyPersonalizedBoost`：

| 条件 | 乘子 |
|---|---|
| focused（用户聚焦品类） | 1.5× |
| 强偏好 | 1.3× |
| 弱偏好 | 1.1× |
| 偏离品类 | 0.7× |

## 数据初始化 & 调度

推荐相关 Redis Key 的离线预热由 `scripts/warmup_redis.py` 完成；
训练与重训守护见 `DeepCTR/tiktok_rec_service/scripts/`（`retrain_and_reload.sh`、
`auto_retrain_loop.sh`）。

## 效果指标

**在线**：CTR、完播率、互动率、人均观看时长、次日/7 日留存。
**离线**：召回率、准确率、多样性、覆盖率。

## 未来规划

- Wide & Deep / DeepFM / DIN 线上升级（训练侧已有 DeepCTR 支持）
- Flink 实时特征计算
- 图神经网络（GraphSAGE / GAT）召回
- 冷启动优化（内容理解 + 迁移学习）
