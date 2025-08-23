namespace go videos

include "base.thrift"

// ========== 共享数据结构 ==========
struct VideoResolution {
    1: i32 width
    2: i32 height
}

struct UserStorageQuota {
    1: i64 total_quota_bytes      // 总配额（字节）
    2: i64 used_quota_bytes       // 已使用配额（字节）
    3: i64 video_count           // 视频数量
    4: string quota_level        // 配额等级：basic, premium, vip
    5: i64 max_video_size_bytes  // 单个视频最大大小
    6: i32 max_video_count       // 最大视频数量
}

// ========== V2版本结构体（推荐使用） ==========
struct VideoPublishStartRequestV2 {
    1: i64 user_id
    2: string title (vt.min_size="1", vt.max_size="200")
    3: string description (vt.max_size="2000")
    4: list<string> tags                    // 替换lab_name的规范实现
    5: string category (vt.min_size="1")
    6: string privacy                       // 替换open的规范实现：public, private, friends
    7: i64 total_file_size
    8: i64 estimated_duration
    9: VideoResolution estimated_resolution
    10: i32 chunk_total_number (vt.gt="0")
    11: i64 chunk_size
    12: string original_filename
    13: string content_type
    14: string upload_session_expire        // 可选
}

struct VideoPublishStartResponseV2 {
    1: base.Status base
    2: string upload_session_uuid
    3: i64 video_id
    4: UserStorageQuota user_quota
    5: string temp_upload_path
    6: i64 session_expires_at
    7: list<string> presigned_urls         // 可选
}

struct VideoPublishUploadingRequestV2 {
    1: i64 user_id
    2: string upload_session_uuid
    3: i32 chunk_number (vt.gt="0")
    4: string chunk_presigned_url         // 可选，用于前端直传
    5: binary chunk_data                  // 可选，用于服务端上传
    6: string chunk_md5
    7: i64 chunk_size
    8: i64 chunk_offset
    9: bool is_compressed
    10: string compression_algorithm
}

struct VideoPublishUploadingResponseV2 {
    1: base.Status base
    2: i32 uploaded_chunk_number
    3: string chunk_upload_status
    4: double upload_progress_percent
    5: i64 next_chunk_offset
    6: string upload_speed_mbps
}

struct VideoPublishCompleteRequestV2 {
    1: i64 user_id
    2: string upload_session_uuid
    3: string final_file_md5
    4: i64 final_file_size
    5: bool enable_transcoding
    6: list<i32> target_resolutions
    7: bool generate_thumbnails
    8: bool generate_animated_cover
    9: map<string, string> custom_metadata
}

struct VideoPublishCompleteResponseV2 {
    1: base.Status base
    2: i64 video_id
    3: string video_source_url
    4: map<i32, string> processed_video_urls
    5: map<string, string> thumbnail_urls
    6: string animated_cover_url
    7: string metadata_url
    8: string processing_status
    9: i64 processing_job_id
    10: UserStorageQuota updated_quota
}

struct VideoPublishCancelRequestV2 {
    1: i64 user_id
    2: string upload_session_uuid
    3: string cancel_reason
}

struct VideoPublishCancelResponseV2 {
    1: base.Status base
    2: string cleanup_status
    3: i64 storage_recovered_bytes
    4: UserStorageQuota updated_quota
}

// ========== V2扩展功能：上传管理 ==========
struct VideoPublishProgressRequestV2 {
    1: i64 user_id
    2: string upload_session_uuid
}

struct VideoPublishProgressResponseV2 {
    1: base.Status base
    2: string session_status
    3: i32 total_chunks
    4: i32 uploaded_chunks
    5: double upload_progress_percent
    6: double processing_progress_percent
    7: i64 upload_speed_bytes_per_sec
    8: i64 eta_seconds
    9: list<i32> failed_chunks
    10: string current_stage
}

struct VideoPublishResumeRequestV2 {
    1: i64 user_id
    2: string upload_session_uuid
    3: string last_chunk_md5
}

struct VideoPublishResumeResponseV2 {
    1: base.Status base
    2: i32 last_uploaded_chunk
    3: list<i32> missing_chunks
    4: i64 session_remaining_time
    5: bool can_resume
    6: string resume_strategy
}

// ========== 视频查询和管理（V2版本） ==========
struct VideoFeedListRequestV2 {
    1: i64 user_id
    2: i64 page_num
    3: i64 page_size
    4: string category_filter
    5: string privacy_filter
    6: list<string> tag_filters
}

struct VideoFeedListResponseV2 {
    1: base.Status base
    2: list<base.Video> video_list
    3: i64 total
    4: bool has_more
    5: string next_cursor
}

struct VideoSearchRequestV2 {
    1: string keyword
    2: i64 page_num
    3: i64 page_size
    4: string from_date
    5: string to_date
    6: list<string> categories
    7: list<string> tags
    8: string sort_by
}

struct VideoSearchResponseV2 {
    1: base.Status base
    2: list<base.Video> video_search
    3: i64 count
    4: map<string, i64> facets
    5: list<string> suggestions
}

struct VideoPopularRequestV2 {
    1: i64 page_num
    2: i64 page_size
    3: string time_range
    4: string category
}

struct VideoPopularResponseV2 {
    1: base.Status base
    2: list<base.Video> Popular
    3: string ranking_algorithm
    4: string updated_at
}

struct VideoInfoRequestV2 {
    1: i64 video_id
    2: i64 requesting_user_id
    3: bool include_analytics
}

struct VideoInfoResponseV2 {
    1: base.Status base
    2: base.Video items
    3: map<string, string> analytics_data
    4: bool can_edit
    5: bool can_delete
}

struct VideoDeleteRequestV2 {
    1: i64 user_id
    2: i64 video_id
    3: string delete_reason
    4: bool permanent_delete
}

struct VideoDeleteResponseV2 {
    1: base.Status base
    2: i64 storage_recovered_bytes
    3: UserStorageQuota updated_quota
}

struct VideoVisitRequestV2 {
    1: i64 from_id
    2: i64 video_id
    3: string visit_source
    4: map<string, string> context
}

struct VideoVisitResponseV2 {
    1: base.Status base
    2: base.Video item
    3: bool view_counted
    4: list<base.Video> related_videos
}

// ========== 视频统计功能（V2版本） ==========
struct UpdateVisitCountRequestV2 {
    1: i64 video_id
    2: i64 visit_count
    3: string visitor_ip
    4: i64 visitor_user_id
}

struct UpdateVisitCountResponseV2 {
    1: base.Status base
    2: i64 new_total_count
}

struct UpdateVideoCommentCountRequestV2 {
    1: i64 video_id
    2: i64 comment_count
    3: string operation_type
}

struct UpdateVideoCommentCountResponseV2 {
    1: base.Status base
    2: i64 new_total_count
}

struct UpdateLikeCountRequestV2 {
    1: i64 video_id
    2: i64 like_count
    3: i64 user_id
    4: string operation_type
}

struct UpdateLikeCountResponseV2 {
    1: base.Status base
    2: i64 new_total_count
}

struct GetVideoVisitCountRequestV2 {
    1: i64 video_id
    2: string count_type
}

struct GetVideoVisitCountResponseV2 {
    1: base.Status base
    2: i64 visit_count
    3: map<string, i64> detailed_counts
}

// ========== 视频流播放（V2版本） ==========
struct StreamVideoRequestV2 {
    1: string video_id
    2: string quality
    3: string format
    4: i64 start_time
    5: i64 end_time
}

struct StreamVideoResponseV2 {
    1: base.Status base
    2: string stream_url
    3: map<string, string> stream_metadata
    4: i64 expires_at
}

// ========== 收藏夹功能（V2版本） ==========
struct CreateFavoriteRequestV2 {
    1: i64 user_id
    2: string name
    3: string description
    4: string cover_url
    5: string privacy
    6: list<string> tags
}

struct CreateFavoriteResponseV2 {
    1: base.Status base
    2: i64 favorite_id
}

struct GetFavoriteListRequestV2 {
    1: i64 user_id
    2: i64 page_num
    3: i64 page_size
    4: string privacy_filter
}

struct GetFavoriteListResponseV2 {
    1: base.Status base
    2: list<base.Favorite> favorite_list
    3: i64 total_count
}

struct AddFavoriteVideoRequestV2 {
    1: i64 favorite_id
    2: i64 user_id
    3: i64 video_id
    4: string note
}

struct AddFavoriteVideoResponseV2 {
    1: base.Status base
    2: bool already_exists
}

struct GetFavoriteVideoListRequestV2 {
    1: i64 user_id
    2: i64 favorite_id
    3: i64 page_num
    4: i64 page_size
    5: string sort_by
}

struct GetFavoriteVideoListResponseV2 {
    1: base.Status base
    2: list<base.Video> video_list
    3: i64 total_count
}

struct DeleteFavoriteRequestV2 {
    1: i64 user_id
    2: i64 favorite_id
    3: string delete_reason
}

struct DeleteFavoriteResponseV2 {
    1: base.Status base
    2: i64 videos_moved_count
}

struct DeleteVideoFromFavoriteRequestV2 {
    1: i64 favorite_id
    2: i64 user_id
    3: i64 video_id
    4: string remove_reason
}

struct DeleteVideoFromFavoriteResponseV2 {
    1: base.Status base
}

// ========== 分享功能（V2版本） ==========
struct SharedVideoRequestV2 {
    1: i64 user_id
    2: i64 to_user_id
    3: i64 video_id
    4: string share_message
    5: string share_platform
}

struct SharedVideoResponseV2 {
    1: base.Status base
    2: string share_url
    3: string share_code
}

struct RecommendVideoRequestV2 {
    1: i64 user_id
    2: i32 count
    3: list<string> categories
    4: string algorithm_type
}

struct RecommendVideoResponseV2 {
    1: base.Status base
    2: list<base.Video> video_list
    3: string recommendation_id
    4: string algorithm_used
}

// ========== V2扩展功能：存储管理 ==========
struct VideoStorageInfo {
    1: i64 user_id
    2: i64 video_id
    3: string source_path
    4: map<i32, string> processed_paths
    5: map<string, string> thumbnail_paths
    6: string animated_cover_path
    7: string metadata_path
    8: string storage_tier           // hot, warm, cold
    9: string bucket_name
    10: i64 file_size
    11: i32 duration_seconds
    12: VideoResolution resolution
    13: string format
}

struct VideoHeatManagementRequest {
    1: i64 video_id
    2: string operation             // promote_to_hot, demote_to_warm, archive_to_cold
    3: string reason
}

struct VideoHeatManagementResponse {
    1: base.Status base
    2: string old_tier
    3: string new_tier
    4: i64 operation_cost_bytes
}

struct UserQuotaManagementRequest {
    1: i64 user_id
    2: string operation            // get, update, reset
    3: UserStorageQuota new_quota  // 仅update时使用
}

struct UserQuotaManagementResponse {
    1: base.Status base
    2: UserStorageQuota current_quota
    3: list<string> quota_warnings
    4: bool quota_exceeded
}

struct BatchVideoOperationRequest {
    1: i64 user_id
    2: list<i64> video_ids
    3: string operation            // delete, change_privacy, move_to_tier
    4: map<string, string> operation_params
}

struct BatchVideoOperationResponse {
    1: base.Status base
    2: list<i64> success_video_ids
    3: map<i64, string> failed_video_errors
    4: UserStorageQuota updated_quota
}

// ========== V2扩展功能：转码服务 ==========
struct VideoTranscodingRequest {
    1: i64 user_id
    2: i64 video_id
    3: list<i32> target_qualities   // [240, 360, 480, 720, 1080, 1440, 2160]
    4: list<string> target_formats  // [mp4, webm, hls]
    5: bool generate_thumbnails
    6: i32 thumbnail_count
}

struct VideoTranscodingResponse {
    1: base.Status base
    2: i64 transcoding_job_id
    3: string job_status
    4: map<i32, string> transcoded_urls
    5: list<string> thumbnail_urls
    6: i64 estimated_completion_time
}

// ========== V2扩展功能：分析统计 ==========
struct VideoAnalyticsRequest {
    1: i64 user_id
    2: list<i64> video_ids         // 空列表表示查询用户所有视频
    3: string date_range_start     // YYYY-MM-DD
    4: string date_range_end       // YYYY-MM-DD
    5: list<string> metrics        // views, likes, shares, comments, download_bytes
}

struct VideoAnalyticsResponse {
    1: base.Status base
    2: map<i64, map<string, i64>> video_metrics
    3: map<string, i64> total_metrics
    4: list<string> top_performing_videos
    5: string report_generated_at
}

// ========== 统一视频服务（仅V2版本） ==========
service VideoService {
    // ========== V2版本API（推荐使用） ==========
    // 核心上传流程
    VideoPublishStartResponseV2 VideoPublishStartV2(1: VideoPublishStartRequestV2 req)(api.post="/v2/publish/start")
    VideoPublishUploadingResponseV2 VideoPublishUploadingV2(1: VideoPublishUploadingRequestV2 req)(api.post="/v2/publish/uploading")
    VideoPublishCompleteResponseV2 VideoPublishCompleteV2(1: VideoPublishCompleteRequestV2 req)(api.post="/v2/publish/complete")
    VideoPublishCancelResponseV2 VideoPublishCancelV2(1: VideoPublishCancelRequestV2 req)(api.post="/v2/publish/cancel")
    
    // 上传管理
    VideoPublishProgressResponseV2 GetUploadProgressV2(1: VideoPublishProgressRequestV2 req)(api.get="/v2/publish/progress")
    VideoPublishResumeResponseV2 ResumeUploadV2(1: VideoPublishResumeRequestV2 req)(api.post="/v2/publish/resume")
    
    // 视频查询和管理
    VideoFeedListResponseV2 VideoFeedListV2(1: VideoFeedListRequestV2 req)(api.get="/v2/video/feed")
    VideoSearchResponseV2 VideoSearchV2(1: VideoSearchRequestV2 req)(api.post="/v2/video/search")
    VideoPopularResponseV2 VideoPopularV2(1: VideoPopularRequestV2 req)(api.get="/v2/video/popular")
    VideoInfoResponseV2 VideoInfoV2(1: VideoInfoRequestV2 req)(api.get="/v2/video/info")
    VideoDeleteResponseV2 VideoDeleteV2(1: VideoDeleteRequestV2 req)(api.delete="/v2/video/delete")
    VideoVisitResponseV2 VideoVisitV2(1: VideoVisitRequestV2 req)(api.post="/v2/video/visit")
    
    // 视频统计
    UpdateVisitCountResponseV2 UpdateVisitCountV2(1: UpdateVisitCountRequestV2 req)(api.post="/v2/video/visit/update")
    UpdateVideoCommentCountResponseV2 UpdateVideoCommentCountV2(1: UpdateVideoCommentCountRequestV2 req)(api.post="/v2/video/comment/update")
    UpdateLikeCountResponseV2 UpdateVideoLikeCountV2(1: UpdateLikeCountRequestV2 req)(api.post="/v2/video/like/update")
    GetVideoVisitCountResponseV2 GetVideoVisitCountV2(1: GetVideoVisitCountRequestV2 req)(api.get="/v2/video/visit/count")
    
    // 视频流播放
    StreamVideoResponseV2 StreamVideoV2(1: StreamVideoRequestV2 req)(api.post="/v2/video/stream")
    
    // 收藏夹功能
    CreateFavoriteResponseV2 CreateFavoriteV2(1: CreateFavoriteRequestV2 req)(api.post="/v2/favorite/create")
    GetFavoriteVideoListResponseV2 GetFavoriteVideoListV2(1: GetFavoriteVideoListRequestV2 req)(api.get="/v2/favorite/video/list")
    GetFavoriteListResponseV2 GetFavoriteListV2(1: GetFavoriteListRequestV2 req)(api.get="/v2/favorite/list")
    AddFavoriteVideoResponseV2 AddFavoriteVideoV2(1: AddFavoriteVideoRequestV2 req)(api.post="/v2/favorite/video/add")
    DeleteFavoriteResponseV2 DeleteFavoriteV2(1: DeleteFavoriteRequestV2 req)(api.delete="/v2/favorite/delete")
    DeleteVideoFromFavoriteResponseV2 DeleteVideoFromFavoriteV2(1: DeleteVideoFromFavoriteRequestV2 req)(api.delete="/v2/favorite/video/delete")
    
    // 分享功能
    SharedVideoResponseV2 SharedVideoV2(1: SharedVideoRequestV2 req)(api.post="/v2/video/share")
    RecommendVideoResponseV2 RecommendVideoV2(1: RecommendVideoRequestV2 req)(api.get="/v2/video/recommend")
    
    // 存储管理
    VideoHeatManagementResponse ManageVideoHeatV2(1: VideoHeatManagementRequest req)(api.post="/v2/storage/heat/manage")
    UserQuotaManagementResponse ManageUserQuotaV2(1: UserQuotaManagementRequest req)(api.post="/v2/storage/quota/manage")
    BatchVideoOperationResponse BatchOperateVideosV2(1: BatchVideoOperationRequest req)(api.post="/v2/videos/batch")
    
    // 转码服务
    VideoTranscodingResponse TranscodeVideoV2(1: VideoTranscodingRequest req)(api.post="/v2/video/transcode")
    
    // 分析统计
    VideoAnalyticsResponse GetVideoAnalyticsV2(1: VideoAnalyticsRequest req)(api.get="/v2/video/analytics")
}
