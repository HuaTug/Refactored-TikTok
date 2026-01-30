# 抖音类短视频推荐系统 - 完整实施指南

> 本文档是构建工业级短视频推荐系统的完整指南，涵盖从数据库设计到模型部署的全流程。

---

## 📋 项目总览

### 系统架构图

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                              客户端 (App/Web)                                    │
└───────────────────────────────────┬─────────────────────────────────────────────┘
                                    │ HTTP/WebSocket
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│                            API Gateway (Hertz)                                   │
│                        cmd/api/handlers/video/*                                  │
└───────────────────────────────────┬─────────────────────────────────────────────┘
                                    │ RPC (Kitex)
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│                           推荐服务 (Recommendation)                              │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐ │
│  │   召回层    │  │   粗排层    │  │   精排层    │  │       重排序层          │ │
│  │  (500候选)  │→│  (200候选)  │→│  (50候选)   │→│   (最终10-20条)         │ │
│  │            │  │            │  │            │  │                         │ │
│  │ - 协同过滤 │  │ - 规则筛选 │  │ - DeepFM   │  │ - MMR多样性            │ │
│  │ - 热门召回 │  │ - 特征预筛 │  │ - DIN      │  │ - 探索注入             │ │
│  │ - 内容召回 │  │            │  │ - MMoE     │  │ - 业务规则             │ │
│  │ - 社交召回 │  │            │  │            │  │                         │ │
│  │ - 新视频   │  │            │  │            │  │                         │ │
│  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────────────┘
                                    │
              ┌─────────────────────┼─────────────────────┐
              ▼                     ▼                     ▼
┌─────────────────────┐ ┌─────────────────────┐ ┌─────────────────────┐
│      MySQL          │ │      Redis          │ │   CTR Service       │
│   (持久化存储)       │ │   (实时特征缓存)     │ │   (Python/TF)       │
│                     │ │                     │ │                     │
│ - 用户画像表        │ │ - 用户实时行为      │ │ - DeepFM模型        │
│ - 视频特征表        │ │ - 视频热度缓存      │ │ - DIN模型           │
│ - 交互记录表        │ │ - 推荐结果缓存      │ │ - MMoE模型          │
│ - 热度表           │ │ - 布隆过滤器        │ │                     │
└─────────────────────┘ └─────────────────────┘ └─────────────────────┘
```

---

## ✅ 实施清单

### 阶段一：基础设施搭建 (已完成 ✅)

#### 1.1 数据库表设计
- [x] 用户画像表 `user_profiles`
- [x] 视频特征表 `video_features`
- [x] 视频向量表 `video_embeddings`
- [x] 用户向量表 `user_embeddings`
- [x] 视频相似度表 `video_similarities`
- [x] 推荐曝光表 `recommendation_exposures`
- [x] 负反馈表 `negative_feedbacks`
- [x] 热度表 `video_hot_scores`
- [x] 作者评分表 `author_scores`
- [x] 分类统计表 `category_stats`
- [x] A/B测试表 `ab_test_*`

**文件位置**：
- 模型定义：`cmd/model/recommendation.go`
- SQL脚本：`config/mysql/recommendation_init.sql`

#### 1.2 数据访问层 (DAL)
- [x] 用户画像 CRUD
- [x] 视频特征 CRUD
- [x] 向量存储与查询
- [x] 相似度计算与存储
- [x] 曝光记录
- [x] 热度查询与更新
- [x] A/B测试分配

**文件位置**：`cmd/video/dal/db/recommendation.go`

---

### 阶段二：推荐引擎开发 (已完成 ✅)

#### 2.1 召回策略
- [x] 协同过滤召回 `EnhancedCFRecall`
- [x] 热门视频召回 `EnhancedHotRecall`
- [x] 内容召回 `EnhancedContentRecall`
- [x] 社交召回 `EnhancedSocialRecall`
- [x] 新视频召回 `EnhancedNewVideoRecall`
- [x] 相似视频召回 `SimilarVideoRecall`
- [x] 趋势视频召回 `TrendingVideoRecall`

**文件位置**：`pkg/recommendation/recall_enhanced.go`

#### 2.2 排序模型
- [x] 特征权重配置
- [x] 加权线性模型
- [x] 批量并行计算
- [x] Thompson Sampling 探索

**文件位置**：`pkg/recommendation/ranking_enhanced.go`

#### 2.3 推荐引擎主体
- [x] 多路召回合并
- [x] 过滤逻辑（已曝光、负反馈、低质量）
- [x] MMR重排序（多样性）
- [x] 探索性注入
- [x] 布隆过滤器去重
- [x] 推荐理由生成

**文件位置**：`pkg/recommendation/engine_enhanced.go`

---

### 阶段三：用户画像服务 (已完成 ✅)

#### 3.1 实时画像更新
- [x] 异步事件队列
- [x] 兴趣标签更新
- [x] 分类偏好更新
- [x] 作者偏好更新
- [x] 行为统计更新
- [x] 活跃时段更新

#### 3.2 画像持久化
- [x] 定时批量写入数据库
- [x] 画像衰减机制
- [x] 冷启动处理

**文件位置**：`pkg/recommendation/user_profile_service.go`

---

### 阶段四：热度计算服务 (已完成 ✅)

#### 4.1 热度算法
- [x] 多时间窗口（1h/6h/24h/7d）
- [x] 加权互动分数
- [x] 时间衰减模型
- [x] 质量加成

#### 4.2 定时任务
- [x] 热度计算调度器
- [x] 分类统计更新
- [x] 旧数据清理

**文件位置**：
- 服务：`pkg/recommendation/hot_score_service.go`
- 初始化：`pkg/recommendation/hot_score_init.go`
- Handler：`cmd/api/handlers/video/hot_ranking_handler.go`

---

### 阶段五：CTR精排服务 (已完成 ✅)

#### 5.1 特征工程
- [x] 用户特征定义（15+）
- [x] 视频特征定义（20+）
- [x] 上下文特征定义（10+）
- [x] 交叉特征定义（10+）
- [x] 序列特征定义（DIN用）

**文件位置**：`ml/deepctr_service/config/feature_config.py`

#### 5.2 模型训练
- [x] DeepFM 模型
- [x] DIN 模型
- [x] MMoE/PLE 多目标模型
- [x] 训练流程封装

**文件位置**：`ml/deepctr_service/models/trainer.py`

#### 5.3 推理服务
- [x] Flask API 服务
- [x] 健康检查
- [x] 单请求预测
- [x] 批量预测
- [x] 模型热切换

**文件位置**：`ml/deepctr_service/serving/inference_server.py`

#### 5.4 Go客户端
- [x] CTR服务客户端
- [x] 连接池管理
- [x] 超时和降级
- [x] 集成推荐引擎

**文件位置**：
- 客户端：`pkg/recommendation/ctr_client.go`
- 集成引擎：`pkg/recommendation/integrated_engine.go`

---

### 阶段六：测试与验证 (已完成 ✅)

#### 6.1 数据准备
- [x] 模拟数据生成器
- [x] 公开数据集下载
- [x] 数据格式转换

**文件位置**：
- 模拟数据：`ml/deepctr_service/data/generate_mock_data.py`
- 公开数据：`ml/deepctr_service/data/download_public_datasets.py`

#### 6.2 测试脚本
- [x] 端到端测试
- [x] 快速启动脚本

**文件位置**：
- E2E测试：`ml/deepctr_service/scripts/test_e2e.py`
- 快速启动：`ml/deepctr_service/scripts/quick_start.sh`

---

## 🚀 快速启动命令

### 1. 初始化数据库

```bash
# 执行 SQL 迁移脚本
mysql -u root -p tiktok < config/mysql/recommendation_init.sql
```

### 2. 启动 Go 服务

```bash
# 编译
cd /Users/zhihuaxu/Desktop/go/Refactored-TikTok
go build ./...

# 启动 video 服务（包含热度计算）
cd cmd/video && go run main.go
```

### 3. 启动 CTR 服务

```bash
# 进入 ML 目录
cd ml/deepctr_service

# 一键启动测试
./scripts/quick_start.sh

# 或手动启动
python3 -m venv venv
source venv/bin/activate
pip install -r requirements.txt
python serving/inference_server.py
```

### 4. 测试推荐接口

```bash
# 热门排行榜
curl http://localhost:8080/api/video/hot/ranking?limit=10

# CTR 预测
curl -X POST http://localhost:8000/predict \
  -H "Content-Type: application/json" \
  -d '{"user_id": 1, "video_ids": [1,2,3,4,5]}'
```

---

## 📁 完整文件结构

```
Refactored-TikTok/
│
├── cmd/
│   ├── api/
│   │   └── handlers/video/
│   │       └── hot_ranking_handler.go     # 热门排行 API
│   │
│   ├── model/
│   │   └── recommendation.go              # 推荐相关数据模型
│   │
│   └── video/
│       ├── dal/db/
│       │   └── recommendation.go          # 推荐数据 DAL
│       ├── main.go                        # 服务入口（集成热度服务）
│       └── service/
│           └── hot_score_starter.go       # 热度服务启动器
│
├── config/
│   └── mysql/
│       └── recommendation_init.sql        # 数据库迁移脚本
│
├── pkg/
│   └── recommendation/
│       ├── engine_enhanced.go             # 推荐引擎主体
│       ├── recall_enhanced.go             # 召回策略
│       ├── ranking_enhanced.go            # 排序模型
│       ├── user_profile_service.go        # 用户画像服务
│       ├── hot_score_service.go           # 热度计算服务
│       ├── hot_score_init.go              # 热度服务初始化
│       ├── ctr_client.go                  # CTR 服务客户端
│       └── integrated_engine.go           # 集成推荐引擎
│
├── ml/
│   └── deepctr_service/
│       ├── config/
│       │   ├── feature_config.py          # 特征配置
│       │   └── serving_config.json        # 服务配置
│       ├── data/
│       │   ├── feature_extractor.py       # 特征提取
│       │   ├── generate_mock_data.py      # 模拟数据生成
│       │   └── download_public_datasets.py # 公开数据集
│       ├── models/
│       │   └── trainer.py                 # 模型训练
│       ├── serving/
│       │   └── inference_server.py        # 推理服务
│       ├── scripts/
│       │   ├── quick_start.sh             # 快速启动
│       │   ├── test_e2e.py                # 端到端测试
│       │   └── train.sh                   # 训练脚本
│       ├── Dockerfile
│       ├── docker-compose.yml
│       ├── requirements.txt
│       └── README.md
│
└── docs/
    └── recommendation_system_guide.md     # 本文档
```

---

## 🔧 配置参数

### 推荐引擎配置

```go
// pkg/recommendation/engine_enhanced.go
config := &RecommendationConfig{
    RecallLimit:        500,    // 召回数量
    CoarseRankLimit:    200,    // 粗排数量
    FinalLimit:         20,     // 最终推荐数
    EnableExploration:  true,   // 开启探索
    ExplorationRatio:   0.1,    // 探索比例 10%
    DiversityWeight:    0.3,    // 多样性权重
    MinQualityScore:    0.3,    // 最低质量分
}
```

### 热度计算配置

```go
// pkg/recommendation/hot_score_service.go
config := &HotScoreConfig{
    CalculationInterval: 5 * time.Minute,  // 计算间隔
    TimeWindows:         []string{"1h", "6h", "24h", "7d"},
    Weights: InteractionWeights{
        View:     1.0,
        Like:     3.0,
        Comment:  5.0,
        Share:    8.0,
        Favorite: 4.0,
    },
    DecayHalfLife: 6 * time.Hour,  // 衰减半衰期
}
```

### CTR 服务配置

```go
// pkg/recommendation/ctr_client.go
config := &CTRServiceConfig{
    ServiceURL:     "http://localhost:8000",
    Timeout:        200 * time.Millisecond,
    MaxRetries:     2,
    DefaultModel:   "deepfm",
    EnableFallback: true,
}
```

---

## 📊 监控指标

### 业务指标
| 指标 | 说明 | 目标 |
|------|------|------|
| CTR | 点击率 | > 5% |
| 完播率 | 观看 >80% 的比例 | > 30% |
| 互动率 | 点赞+评论+分享 | > 3% |
| 人均观看时长 | 单用户日均 | > 30分钟 |

### 系统指标
| 指标 | 说明 | 目标 |
|------|------|------|
| 推荐延迟 P99 | 99分位响应时间 | < 200ms |
| CTR服务 QPS | 每秒查询数 | > 1000 |
| 召回覆盖率 | 视频被召回的比例 | > 80% |
| 模型 AUC | 预测准确性 | > 0.70 |

---

## 🔄 迭代优化路线

### Phase 1: 基础功能 (当前) ✅
- 多路召回
- 简单排序
- 热度计算
- 基础 CTR 模型

### Phase 2: 模型升级
- [ ] 实时特征更新
- [ ] 用户 Embedding 在线更新
- [ ] 增量训练
- [ ] 模型 A/B 测试框架

### Phase 3: 高级特性
- [ ] 多目标优化 (CTR + 时长 + 互动)
- [ ] 强化学习探索
- [ ] 知识图谱增强
- [ ] 跨域推荐

### Phase 4: 工程优化
- [ ] 特征服务独立
- [ ] 向量检索加速 (Faiss/Milvus)
- [ ] GPU 推理加速
- [ ] 分布式召回

---

## ❓ 常见问题

### Q1: 冷启动如何处理？
**新用户**：
- 使用热门视频召回
- 基于注册信息（年龄、性别、城市）推荐
- 逐步收集行为数据

**新视频**：
- 流量扶持期（前24小时）
- 内容分析匹配相似视频的观众
- 作者粉丝优先推荐

### Q2: 如何防止信息茧房？
- 探索性注入 10% 随机内容
- MMR 重排序保证多样性
- 定期衰减用户画像
- 兴趣拓展推荐

### Q3: 热门视频霸屏怎么办？
- 热度分数加时间衰减
- 曝光次数惩罚
- 召回配额限制
- 用户偏好加权

### Q4: 模型更新频率？
- 热度分数：每 5 分钟
- 用户画像：实时 + 每小时持久化
- CTR 模型：每天/每周离线训练
- 特征：实时 + 小时级

---

## 📚 参考资料

### 论文
1. [DeepFM](https://arxiv.org/abs/1703.04247) - Wide & Deep 特征交叉
2. [DIN](https://arxiv.org/abs/1706.06978) - 用户兴趣序列建模
3. [MMoE](https://dl.acm.org/doi/10.1145/3219819.3220007) - 多任务学习
4. [DIEN](https://arxiv.org/abs/1809.03672) - 兴趣演化网络

### 开源项目
1. [DeepCTR](https://github.com/shenweichen/DeepCTR) - CTR 模型库
2. [FuxiCTR](https://github.com/xue-pai/FuxiCTR) - 工业级 CTR
3. [RecBole](https://github.com/RUCAIBox/RecBole) - 推荐系统框架

### 技术博客
1. 抖音推荐系统架构
2. 快手实时推荐系统
3. 美团搜索推荐实践

---

## 📞 联系方式

如有问题，请提交 Issue 或联系开发团队。

---

*最后更新: 2026-01-30*
