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

// ========== V1版本结构体（保持兼容性） ==========
struct FeedServiceRequest{
    1: string last_time
}
struct FeedServiceResponse{
    1: base.Status base
    2: list<base.Video> video_list
}

struct VideoPublishStartRequest{
    1: i64 user_id
    2: string title (vt.min_size="1")
    3: string description
    4: string lab_name                    // 保留兼容性，建议使用V2版本
    5: string category
    6: i64 open                          // 保留兼容性，建议使用V2版本
    7: i64 chunk_total_number (vt.gt="0")
}
struct VideoPublishStartResponse{
    1: base.Status base
    2: string uuid
}

struct VideoPublishUploadingRequest{
    1: i64 user_id
    2: string uuid
    3: binary data
    4: string md5
    5: bool is_m3u8
    6: string filename
    7: i64 chunk_number
}
struct VideoPublishUploadingResponse{
    1: base.Status base
}

struct VideoPublishCompleteRequest{
    1: i64 user_id
    2: string uuid
}
struct VideoPublishCompleteResponse{
    1: base.Status base
}

struct VideoPublishCancleRequest{
    1: i64 user_id
    2: string uuid
}
struct VideoPublishCancleResponse{
    1: base.Status base
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

// ========== 视频查询和管理（V1功能） ==========
struct VideoFeedListRequest{
    1: i64 user_id
    2: i64 page_num
    3: i64 page_size
}
struct VideoFeedListResponse{
    1: base.Status base
    2: list<base.Video> video_list
    3: i64 total
}

struct VideoSearchRequest{
    1: string keyword
    2: i64 page_num
    3: i64 page_size
    4: string from_date
    5: string to_date
}
struct VideoSearchResponse{
    1: base.Status base
    2: list<base.Video> video_search
    3: i64 count
}

struct VideoPopularRequest{
    1: i64 page_num
    2: i64 page_size
}
struct VideoPopularResponse{
    1: base.Status base
    2: list<base.Video> Popular
}

struct VideoInfoRequest{
    1: i64 video_id
}
struct VideoInfoResponse{
    1: base.Status base
    2: base.Video items
}

struct VideoDeleteRequest{
    1: i64 user_id
    2: i64 video_id
}
struct VideoDeleteResponse{
    1: base.Status base
}

struct VideoVisitRequest{
    1: i64 from_id
    2: i64 video_id
}
struct VideoVisitResponse{
    1: base.Status base
    2: base.Video item
}

struct VideoIdListRequest{
    1: i64 page_num
    2: i64 page_size
}
struct VideoIdListResponse{
    1: base.Status base
    2: bool is_end
    3: list<string> list
}

// ========== 视频统计功能 ==========
struct UpdateVisitCountRequest{
    1: i64 video_id
    2: i64 visit_count
}
struct UpdateVisitCountResponse{
    1: base.Status base
}

struct UpdateVideoCommentCountRequest{
    1: i64 video_id
    2: i64 comment_count
}
struct UpdateVideoCommentCountResponse{
    1: base.Status base
}

struct UpdateLikeCountRequest{
    1: i64 video_id
    2: i64 like_count
}
struct UpdateLikeCountResponse{
    1: base.Status base
}

struct UpdateVideoHisLikeCountRequest{
    1: i64 video_id
    2: i64 his_like_count
}
struct UpdateVideoHisLikeCountResponse{
    1: base.Status base
}

struct GetVideoVisitCountRequest{
    1: i64 video_id
}
struct GetVideoVisitCountResponse{
    1: base.Status base
    2: i64 visit_count
}

struct GetVideoVisitCountInRedisRequest{
    1: i64 video_id
}
struct GetVideoVisitCountInRedisResponse{
    1: i64 visit_count
    2: base.Status base
}

// ========== 视频流播放 ==========
struct StreamVideoRequest{
    1: string index
}
struct StreamVideoResponse{
    1: base.Status base
    2: byte data
}

// ========== 收藏夹功能（V1） ==========
struct CreateFavoriteRequest{
    1: i64 user_id
    2: string name
    3: string description
    4: string cover_url
}
struct CreateFavoriteResponse{
    1: base.Status base
}

struct GetFavoriteListRequest{
    1: i64 user_id
    2: i64 page_num
    3: i64 page_size
}
struct GetFavoriteListResponse{
    1: base.Status base
    2: list<base.Favorite> favorite_list
}

struct AddFavoriteVideoRequest{
    1: i64 favorite_id
    2: i64 user_id
    3: i64 video_id
}
struct AddFavoriteVideoResponse{
    1: base.Status base
}

struct GetFavoriteVideoListRequest{
    1: i64 user_id
    2: i64 favorite_id
    3: i64 page_num
    4: i64 page_size
}
struct GetFavoriteVideoListResponse{
    1: base.Status base
    2: list<base.Video> video_list
}

struct GetVideoFromFavoriteRequest{
    1: i64 favorite_id
    2: i64 user_id
    3: i64 video_id
    4: i64 page_num
    5: i64 page_size
}
struct GetVideoFromFavoriteResponse{
    1: base.Status base
    2: base.Video video
}

struct DeleteFavoriteRequest{
    1: i64 user_id
    2: i64 favorite_id
}
struct DeleteFavoriteResponse{
    1: base.Status base
}

struct DeleteVideoFromFavoriteRequest{
    1: i64 favorite_id
    2: i64 user_id
    3: i64 video_id
}
struct DeleteVideoFromFavoriteResponse{
    1: base.Status base
}

// ========== 分享功能（V1） ==========
struct SharedVideoRequest{
    1: i64 user_id
    2: i64 to_user_id
    3: i64 video_id
}
struct SharedVideoResponse{
    1: base.Status base
}

struct GetPopularVideoRequest{
    1: i64 page_num
    2: i64 page_size
}
struct GetPopularVideoResponse{
    1: base.Status base
    2: list<base.Video> video_list
}

struct RecommendVideoRequest{
    1: i64 user_id
}
struct RecommendVideoResponse{
    1: base.Status base
    2: list<base.Video> video_list
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

// ========== 统一视频服务 ==========
service VideoService {
    // ========== V1版本API（保持兼容性） ==========
    FeedServiceResponse FeedService(1: FeedServiceRequest req)(api.get="/v1/video/feed")
    VideoPublishStartResponse VideoPublishStart(1: VideoPublishStartRequest req)(api.post="/v1/publish/start")
    VideoPublishUploadingResponse VideoPublishUploading(1: VideoPublishUploadingRequest req)(api.post="/v1/publish/uploading")
    VideoPublishCompleteResponse VideoPublishComplete(1: VideoPublishCompleteRequest req)(api.post="/v1/publish/complete")
    VideoPublishCancleResponse VideoPublishCancle(1: VideoPublishCancleRequest req)(api.post="/v1/publish/cancle")
    
    VideoDeleteResponse VideoDelete(1: VideoDeleteRequest req)(api.delete="/v1/video/delete")
    VideoIdListResponse VideoIdList(1: VideoIdListRequest req)
    VideoFeedListResponse VideoFeedList(1: VideoFeedListRequest req)(api.get="/v1/video/list")
    VideoSearchResponse VideoSearch(1: VideoSearchRequest req)(api.post="/v1/video/search")
    VideoPopularResponse VideoPopular(1: VideoPopularRequest req)(api.get="/v1/video/popular")
    VideoInfoResponse VideoInfo(1: VideoInfoRequest req)
    VideoVisitResponse VideoVisit(1: VideoVisitRequest req)(api.post="/v1/visit/:id")
    
    UpdateVisitCountResponse UpdateVisitCount(1: UpdateVisitCountRequest req)
    UpdateVideoCommentCountResponse UpdateVideoCommentCount(1: UpdateVideoCommentCountRequest req)
    UpdateLikeCountResponse UpdateVideoLikeCount(1: UpdateLikeCountRequest req)
    UpdateVideoHisLikeCountResponse UpdateVideoHisLikeCount(1: UpdateVideoHisLikeCountRequest req)
    GetVideoVisitCountResponse GetVideoVisitCount(1: GetVideoVisitCountRequest req)
    GetVideoVisitCountInRedisResponse GetVideoVisitCountInRedis(1: GetVideoVisitCountInRedisRequest req)
    
    StreamVideoResponse StreamVideo(1: StreamVideoRequest req)(api.post="/v1/stream")
    
    CreateFavoriteResponse CreateFavorite(1: CreateFavoriteRequest req)(api.post="/v1/favorite/create")
    GetFavoriteVideoListResponse GetFavoriteVideoList(1: GetFavoriteVideoListRequest req)(api.get="/v1/favorite/video/list")
    GetFavoriteListResponse GetFavoriteList(1: GetFavoriteListRequest req)(api.get="/v1/favorite/list")
    GetVideoFromFavoriteResponse GetVideoFromFavorite(1: GetVideoFromFavoriteRequest req)(api.get="/v1/favorite/video")
    AddFavoriteVideoResponse AddFavoriteVideo(1: AddFavoriteVideoRequest req)(api.post="/v1/favorite/video/add")
    DeleteFavoriteResponse DeleteFavorite(1: DeleteFavoriteRequest req)(api.delete="/v1/favorite/delete")
    DeleteVideoFromFavoriteResponse DeleteVideoFromFavorite(1: DeleteVideoFromFavoriteRequest req)(api.delete="/v1/favorite/video/delete")
    
    SharedVideoResponse SharedVideo(1: SharedVideoRequest req)(api.post="/v1/share/video")
    RecommendVideoResponse RecommendVideo(1: RecommendVideoRequest req)(api.get="/v1/recommend/video")
    
    // ========== V2版本API（推荐使用） ==========
    // 核心上传流程
    VideoPublishStartResponseV2 VideoPublishStartV2(1: VideoPublishStartRequestV2 req)(api.post="/v2/publish/start")
    VideoPublishUploadingResponseV2 VideoPublishUploadingV2(1: VideoPublishUploadingRequestV2 req)(api.post="/v2/publish/uploading")
    VideoPublishCompleteResponseV2 VideoPublishCompleteV2(1: VideoPublishCompleteRequestV2 req)(api.post="/v2/publish/complete")
    VideoPublishCancelResponseV2 VideoPublishCancelV2(1: VideoPublishCancelRequestV2 req)(api.post="/v2/publish/cancel")
    
    // 上传管理
    VideoPublishProgressResponseV2 GetUploadProgressV2(1: VideoPublishProgressRequestV2 req)(api.get="/v2/publish/progress")
    VideoPublishResumeResponseV2 ResumeUploadV2(1: VideoPublishResumeRequestV2 req)(api.post="/v2/publish/resume")
    
    // 存储管理
    VideoHeatManagementResponse ManageVideoHeatV2(1: VideoHeatManagementRequest req)(api.post="/v2/storage/heat/manage")
    UserQuotaManagementResponse ManageUserQuotaV2(1: UserQuotaManagementRequest req)(api.post="/v2/storage/quota/manage")
    BatchVideoOperationResponse BatchOperateVideosV2(1: BatchVideoOperationRequest req)(api.post="/v2/videos/batch")
    
    // 转码服务
    VideoTranscodingResponse TranscodeVideoV2(1: VideoTranscodingRequest req)(api.post="/v2/video/transcode")
    
    // 分析统计
    VideoAnalyticsResponse GetVideoAnalyticsV2(1: VideoAnalyticsRequest req)(api.get="/v2/video/analytics")
    
} 
