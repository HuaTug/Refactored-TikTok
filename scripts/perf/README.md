# Refactored-TikTok 性能测试套件

> 配套论文《基于事件驱动的短视频平台设计与实现》第 6 章实验设计，
> 提供从环境搭建、数据准备到 6 个用例 (P01–P06) 端到端可复现的脚本。

---

## 0. 目录速览

```
scripts/perf/
├── README.md                  # 本文件
├── colima-bootstrap.sh        # macOS 上以 4C8G 启动容器
├── seed_data.sql              # 造 10w 用户 + 100w 视频骨架（与论文 §6.1 对齐）
├── gen_tokens.go              # 批量登录 / 注册并导出 tokens.csv
├── p01_kitex_vs_grpc.sh       # P01 · Kitex vs gRPC 通信框架对比
├── p02_core_api.js            # P02 · k6 核心 HTTP 接口 TP99
├── p03_burst.jmx              # P03 · 20× 洪峰对比 (JMeter)
├── p03_burst.js               # P03 · 同场景的 k6 备用版本
├── p03_toggle_sync.sh         # P03 · 切换 LIKE_SYNC=1/0
├── p04_consistency.js         # P04 · 千并发点赞一致性
├── p04_verify.sh              # P04 · 校验 Redis vs MySQL 计数
├── p05_chunk_upload.py        # P05 · 200MB 分片上传断点续传
├── p06_feed_cursor.py         # P06 · Feed 翻页不重复
├── kitex_dolike_bench/        # P01 子项目（Kitex 客户端压测程序）
├── reports/                   # 每次跑完输出 markdown 报告
└── figs/                      # 截图 / 火焰图归档
```

---

## 1. 接口与端口对照（项目实际值）

> 论文模板里的接口路径是示意，本套件已全部对齐到项目真实路由。

| 名称 | 方法 | 路径 | 鉴权 |
|---|---|---|---|
| 登录 | POST | `/v1/user/login` | 否 |
| 注册 | POST | `/v1/user/create/` | 否 |
| 推荐 Feed | GET | `/v1/video/feed?page_num=&page_size=` | 是 |
| 点赞 | POST | `/v1/action/like` | 是 |
| 评论列表 | GET | `/v1/comment/list?video_id=&page_num=&page_size=` | 是 |
| 未读通知 | GET | `/v2/notification/unread` | 是 |
| 上传开始 | POST | `/v2/publish/start` | 是 |
| 上传分片 | POST | `/v2/publish/uploading` | 是 |
| 上传完成 | POST | `/v2/publish/complete` | 是 |

**全局约定**：

- 网关：`http://localhost:8888`（`cmd/api/main.go` 写死）
- 鉴权 header：`Access-Token: <jwt>`（**不是** `Authorization: Bearer`）
- 响应包：`{"code":10000,"message":"...","data":...}`，`code==10000` 视为成功
- MySQL：`127.0.0.1:3307`，库 `TikTok`，密码 `TikTok@MySQL#2025!Secure`
- Redis：`127.0.0.1:6379`，密码 `Redis@TikTok2025_SecurePass`，DB 1（互动）

---

## 2. 准备工作

### 2.1 容器环境（4C8G）

```bash
bash scripts/perf/colima-bootstrap.sh
```

脚本会：
- macOS 下用 `colima start --cpu 4 --memory 8 --disk 60`
- 进入 `deploy/` 启动基础组件
- 等待 healthcheck 全绿

### 2.2 编译并启动微服务

```bash
# 推荐用 tmux 拆 6 个面板，便于看日志
tmux new -s tt
go run ./cmd/user
go run ./cmd/video
go run ./cmd/interaction
go run ./cmd/relation
# 推荐 / 通知如已合入 video / interaction，可省略
go run ./cmd/api    # 网关，最后启动
```

确认网关：

```bash
curl -s http://localhost:8888/ping || echo "gateway not ready"
```

### 2.3 灌数据 + 造 token

```bash
# 1) 演示数据（等同毕业答辩复现的 24 条视频 + 5 用户）
bash scripts/setup_demo.sh

# 2) 大规模压测数据（10w 用户 + 100w 视频骨架）
docker exec -i kitex_mysql mysql -uroot -p'TikTok@MySQL#2025!Secure' TikTok \
  < scripts/perf/seed_data.sql

# 3) 批量登录拿 token（注意 -tags perf）
go run -tags perf scripts/perf/gen_tokens.go -n 1000 -out scripts/perf/tokens.csv
```

---

## 3. 用例索引

| 编号 | 论文章节 | 入口 | 通过标准 |
|---|---|---|---|
| P01 | §6.3.1 | `bash scripts/perf/p01_kitex_vs_grpc.sh` | Kitex QPS ≥ 1.5×gRPC、TP99 优势 ≥ 40% |
| P02 | §2.3 / §6.3 | `k6 run scripts/perf/p02_core_api.js` | 5 接口全部 TP99 < 100ms、错误率 < 0.1% |
| P03 | §6.3.2 | `bash scripts/perf/p03_toggle_sync.sh A` 后 `jmeter -n -t scripts/perf/p03_burst.jmx -l p03_A.jtl` | A 组 30s 内崩溃；B 组 TP99<80ms 不崩溃 |
| P04 | §6.2 | `k6 run scripts/perf/p04_consistency.js && bash scripts/perf/p04_verify.sh 1001` | Redis = SCARD = MySQL = 1000 |
| P05 | §6.2 | `python3 scripts/perf/p05_chunk_upload.py path/to/200mb.mp4` | MD5 一致、断网分片仅落地 1 次 |
| P06 | §6.2 | `python3 scripts/perf/p06_feed_cursor.py --pages 30` | 30 页累计无重复 video_id |

---

## 4. 报告生成

每次执行完会在 `reports/` 输出一份 markdown，文件名 `report-YYYYMMDD-HHMM-Pxx.md`。
所有用例的报告都遵循《方案 §8》的同一模板。

---

## 5. 注意事项

> **关于 IDE 报 "No packages found"**：`gen_tokens.go` 与 `kitex_dolike_bench/main.go` 顶部带
> `//go:build perf` 编译标签，目的是不被 `go build ./...` 误编译进生产二进制。
> 跑的时候必须带上 `-tags perf`，例如：
> ```bash
> go run -tags perf scripts/perf/gen_tokens.go -n 1000
> go run -tags perf ./scripts/perf/kitex_dolike_bench -c 100 -duration 5m
> ```

1. **本机自压会和被测进程争 CPU**，正式答辩复现建议用一台 4C8G 物理机/虚机做被测端，
   另一台开 wrk/k6/JMeter 做压测端，俩机器在同一 LAN。
2. **P01** 本地压满 100% CPU 后所测的"绝对 QPS"会比论文（独立机器）数字小，
   但**两个框架的相对差距**（Kitex 高出 gRPC ~80–100%）应当复现得出来。
3. **P03 A 组**会真把进程压崩，跑之前关闭无关的 IDE / 浏览器，避免误把整机搞死机；
   崩溃后用 `bash scripts/perf/p03_toggle_sync.sh B` 切回正常模式重启服务。
4. P02/P04 用的是大量重复点赞，会被业务层去重；脚本里已经在每个 VU 携带不同 `user_id`
   并固定 `action_type=1`，因此不会触发"取消点赞 → 点赞"震荡。
