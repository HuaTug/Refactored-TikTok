namespace go interactions

include "base.thrift"

struct LikeActionRequest {
    1: i64 user_id
    2: i64 video_id
    3: i64 comment_id
    4: string action_type          // "like" or "unlike"
}

struct LikeActionResponse {
    1: base.Status base
    2: bool is_liked              // 当前点赞状态
    3: i64 like_count             // 最新点赞数 (从Redis读取)
}

struct LikeListRequest {
    1: i64 user_id
    2: i64 page_num
    3: i64 page_size
}
struct LikeListResponse {
    1: base.Status base
    2: list<base.Video> items
}

struct CreateCommentRequest {
    1: i64 user_id
    2: i64 video_id 
    3: i64 comment_id
    4: i64 mode
    5: string content
    6: i64 reply_to_comment_id  // 实际回复目标评论ID，用于记录互动关系
}

struct CreateCommentResponse {
    1: base.Status base
}
struct ListCommentRequest {
    1: i64 video_id
    2: i64 comment_id
    3: i64 page_num
    4: i64 page_size
    5: string sort_type  // "hot" for popular comments (default), "latest" for newest comments
}
struct ListCommentResponse {
    1: base.Status base
    2: list<base.Comment> items
}

struct CommentDeleteRequest {
    1: i64 video_id    
    2: i64 comment_id
    3: i64 from_user_id
}    
struct CommentDeleteResponse {
    1: base.Status base
}

struct VideoPopularListRequest {
    1: i64 page_num
    2: i64 page_size
}
struct VideoPopularListResponse {
    1: base.Status base
    2: list<string> data
}

// 删除与视频相关的信息
struct DeleteVideoInfoRequest {
    1: i64 video_id
}
struct DeleteVideoInfoResponse {
    1: base.Status base
}

// ========== 消息队列事件结构体 ==========
struct LikeEvent {
    1: i64 user_id
    2: i64 video_id
    3: i64 comment_id
    4: string action_type           // "like" or "unlike"
    5: string event_type           // "video_like" or "comment_like"
    6: i64 timestamp
    7: string event_id
}

struct NotificationEvent {
    1: i64 user_id                 // 接收通知的用户ID
    2: i64 from_user_id           // 发起操作的用户ID
    3: string notification_type    // "like", "comment", "mention"
    4: i64 target_id              // 被点赞/评论的视频/评论ID
    5: string content             // 通知内容
    6: i64 timestamp
    7: string event_id
}

// ========== 通知功能 ==========
struct GetNotificationsRequest {
    1: i64 user_id
    2: i64 page_num
    3: i64 page_size
    4: string notification_type   // 可选：筛选通知类型
}

struct GetNotificationsResponse {
    1: base.Status base
    2: list<NotificationInfo> notifications
    3: i64 total_count
    4: i64 unread_count
}

struct NotificationInfo {
    1: i64 notification_id
    2: i64 from_user_id
    3: string from_user_name
    4: string from_user_avatar
    5: string notification_type
    6: string content
    7: i64 target_id
    8: bool is_read
    9: string created_at
}

struct MarkNotificationReadRequest {
    1: i64 user_id
    2: list<i64> notification_ids  // 要标记为已读的通知ID列表
}

struct MarkNotificationReadResponse {
    1: base.Status base
    2: i64 marked_count
}

service InteractionService {
    LikeActionResponse LikeAction(1: LikeActionRequest req)(api.post="/v1/action/like")
    LikeListResponse LikeList(1: LikeListRequest req)(api.get="/v1/action/list")
    CreateCommentResponse CreateComment(1:CreateCommentRequest req)(api.post="/v1/comment/publish")
    ListCommentResponse ListComment(1:ListCommentRequest req)(api.get="/v1/comment/list")
    CommentDeleteResponse DeleteComment(1:CommentDeleteRequest req)(api.delete="/v1/comment/delete")
    VideoPopularListResponse VideoPopularList(1: VideoPopularListRequest req)
    DeleteVideoInfoResponse DeleteVideoInfo(1: DeleteVideoInfoRequest req)
    // 消息队列事件处理
    GetNotificationsResponse GetNotifications(1: GetNotificationsRequest req)(api.get="/v1/notifications")
    MarkNotificationReadResponse MarkNotificationRead(1: MarkNotificationReadRequest req)(api.post="/v1/notifications/read")
}    