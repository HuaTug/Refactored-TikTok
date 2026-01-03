# API 总览

本项目的 HTTP 网关采用 CloudWeGo Hertz，RPC 采用 Kitex，接口以 Thrift IDL 为源。以下为核心接口概览，详细字段请参考 `idl/*.thrift` 以及各服务下的 handler 实现。

- 用户
  - POST /douyin/user/register/ — 用户注册
  - POST /douyin/user/login/ — 用户登录
  - GET  /douyin/user/ — 获取用户信息
- 视频
  - GET  /douyin/feed/ — 视频流（时间/推荐）
  - POST /douyin/publish/action/ — 发布视频（支持分片上传，见 docs/chunk_upload_api.md）
  - GET  /douyin/publish/list/ — 用户发布列表
- 互动
  - POST /douyin/favorite/action/ — 点赞/取消点赞
  - GET  /douyin/favorite/list/ — 点赞列表
  - POST /douyin/comment/action/ — 评论/删除
  - GET  /douyin/comment/list/ — 评论列表
- 社交关系
  - POST /douyin/relation/action/ — 关注/取关
  - GET  /douyin/relation/follow/list/ — 关注列表
  - GET  /douyin/relation/follower/list/ — 粉丝列表
- 消息（如启用）
  - POST /douyin/message/action/ — 发送消息
  - GET  /douyin/message/chat/ — 拉取会话消息

提示：
- 完整的请求/响应模型请查阅 `idl/users.thrift`, `idl/videos.thrift`, `idl/interaction.thrift`, `idl/relation.thrift`, `idl/message.thrift`。
- RPC 路由由 `kitex_gen/*` 生成；HTTP 路由由 `cmd/api/router.go` 及生成代码配置。

## 认证与风控
- 认证：基于 JWT（见 `pkg/jwt.go` 与 API 中间件）。
- 限流/熔断：集成 Sentinel（见 `config/sentinels/Flow.go`）。
- 日志与追踪：集成 logrus 与 Jaeger（见 `config/jaeger/`）。

## 错误码约定
统一错误码在 `pkg/errno/errno.go`，HTTP 接口统一返回 `{status_code, status_msg, data}` 结构。

## 变更与扩展
- 推荐新增搜索、话题、举报等接口，可在 `idl/` 中补充并通过脚本生成。
- 如需开放 API 文档，可选用 Swagger/Redoc 的反向描述，或在 docs/ 下维护 Markdown 版文档。
