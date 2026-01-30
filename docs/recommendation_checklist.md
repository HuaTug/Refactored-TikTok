# 短视频推荐系统 - 快速参考清单

## 🎯 核心目标
构建一个工业级短视频推荐系统，实现 **CTR预估 + 多目标优化**。

---

## ✅ 已完成模块清单

### 1️⃣ 数据层
| 模块 | 文件 | 状态 |
|------|------|------|
| 数据模型定义 | `cmd/model/recommendation.go` | ✅ |
| SQL迁移脚本 | `config/mysql/recommendation_init.sql` | ✅ |
| 数据访问层DAL | `cmd/video/dal/db/recommendation.go` | ✅ |

### 2️⃣ 召回层
| 策略 | 说明 | 状态 |
|------|------|------|
| 协同过滤 | 基于用户行为相似度 | ✅ |
| 热门召回 | 多时间窗口热度 | ✅ |
| 内容召回 | 标签/分类/作者 | ✅ |
| 社交召回 | 关注/好友/同城 | ✅ |
| 新视频召回 | 冷启动扶持 | ✅ |
| 相似召回 | 向量相似度 | ✅ |
| 趋势召回 | 上升最快 | ✅ |

**文件**: `pkg/recommendation/recall_enhanced.go`

### 3️⃣ 排序层
| 模块 | 说明 | 状态 |
|------|------|------|
| 粗排模型 | 加权线性模型 (Go) | ✅ |
| 精排模型 | DeepFM/DIN/MMoE (Python) | ✅ |
| Thompson采样 | 探索与利用平衡 | ✅ |

**文件**: 
- Go粗排: `pkg/recommendation/ranking_enhanced.go`
- Python精排: `ml/deepctr_service/models/trainer.py`

### 4️⃣ 重排序层
| 功能 | 说明 | 状态 |
|------|------|------|
| MMR多样性 | 平衡相关性与多样性 | ✅ |
| 探索注入 | 10%随机探索 | ✅ |
| 去重过滤 | 布隆过滤器 | ✅ |

**文件**: `pkg/recommendation/engine_enhanced.go`

### 5️⃣ 服务层
| 服务 | 说明 | 状态 |
|------|------|------|
| 用户画像服务 | 实时更新+持久化 | ✅ |
| 热度计算服务 | 定时任务+多窗口 | ✅ |
| CTR推理服务 | Flask API | ✅ |
| 集成推荐引擎 | Go端主入口 | ✅ |

**文件**:
- `pkg/recommendation/user_profile_service.go`
- `pkg/recommendation/hot_score_service.go`
- `ml/deepctr_service/serving/inference_server.py`
- `pkg/recommendation/integrated_engine.go`

### 6️⃣ 测试工具
| 工具 | 说明 | 状态 |
|------|------|------|
| 模拟数据生成 | 可配置用户/视频数量 | ✅ |
| 公开数据集 | MovieLens/Criteo | ✅ |
| 端到端测试 | 自动化测试脚本 | ✅ |
| 快速启动 | 一键测试脚本 | ✅ |

**文件**:
- `ml/deepctr_service/data/generate_mock_data.py`
- `ml/deepctr_service/data/download_public_datasets.py`
- `ml/deepctr_service/scripts/test_e2e.py`
- `ml/deepctr_service/scripts/quick_start.sh`

---

## 🚀 快速启动命令

```bash
# 1. 初始化数据库
mysql -u root -p tiktok < config/mysql/recommendation_init.sql

# 2. 编译Go服务
cd /Users/zhihuaxu/Desktop/go/Refactored-TikTok
go build ./...

# 3. 启动CTR服务（一键）
cd ml/deepctr_service
./scripts/quick_start.sh

# 4. 或手动启动
python3 -m venv venv && source venv/bin/activate
pip install -r requirements.txt
python data/generate_mock_data.py --output ./test_data
python serving/inference_server.py
```

---

## 📊 关键算法公式

### 热度分数
```
HotScore = (BaseScore × 0.3 × Decay + DeltaScore × 0.7) × QualityBonus

BaseScore = Views×1 + Likes×3 + Comments×5 + Shares×8 + Favorites×4
Decay = e^(-λt), λ = ln(2)/6小时
```

### CTR预估 (DeepFM)
```
ŷ = sigmoid(y_FM + y_DNN)

y_FM = w₀ + Σwᵢxᵢ + Σ<vᵢ,vⱼ>xᵢxⱼ  (特征交叉)
y_DNN = DNN(embedding)             (深度学习)
```

### MMR重排序
```
MMR = λ × Relevance(d) - (1-λ) × max Similarity(d, dᵢ)

λ = 0.7 (相关性权重)
```

### 多目标融合
```
FinalScore = 0.4×CTR + 0.3×完播率 + 0.2×点赞率 + 0.1×分享率
```

---

## 🔧 核心配置

```go
// 推荐配置
RecallLimit:       500   // 召回数量
CoarseRankLimit:   200   // 粗排数量
FinalLimit:        20    // 最终推荐
ExplorationRatio:  0.1   // 探索比例
DiversityWeight:   0.3   // 多样性权重
```

```python
# CTR模型配置
EMBEDDING_DIM = 16
DNN_HIDDEN_UNITS = (256, 128, 64)
LEARNING_RATE = 0.001
BATCH_SIZE = 256
EPOCHS = 10
```

---

## 📁 文件索引

| 功能 | 文件路径 |
|------|----------|
| 推荐引擎 | `pkg/recommendation/engine_enhanced.go` |
| 召回策略 | `pkg/recommendation/recall_enhanced.go` |
| Go排序 | `pkg/recommendation/ranking_enhanced.go` |
| 用户画像 | `pkg/recommendation/user_profile_service.go` |
| 热度服务 | `pkg/recommendation/hot_score_service.go` |
| CTR客户端 | `pkg/recommendation/ctr_client.go` |
| 集成引擎 | `pkg/recommendation/integrated_engine.go` |
| 数据模型 | `cmd/model/recommendation.go` |
| DAL层 | `cmd/video/dal/db/recommendation.go` |
| SQL脚本 | `config/mysql/recommendation_init.sql` |
| 特征配置 | `ml/deepctr_service/config/feature_config.py` |
| 模型训练 | `ml/deepctr_service/models/trainer.py` |
| 推理服务 | `ml/deepctr_service/serving/inference_server.py` |
| 测试脚本 | `ml/deepctr_service/scripts/test_e2e.py` |
| 详细文档 | `docs/recommendation_system_guide.md` |

---

## 🎯 下一步优化方向

1. **实时特征** - Redis 实时特征存储
2. **向量检索** - Faiss/Milvus 加速相似召回
3. **增量训练** - 模型在线更新
4. **A/B测试** - 完善实验框架
5. **强化学习** - Bandit算法优化探索

---

*快速参考 v1.0 | 2026-01-30*
