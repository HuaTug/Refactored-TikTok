package oss

// 存储桶常量定义
const (
	BUCKET_USER_CONTENT  = "tiktok-user-content"  // 用户生成内容（视频、头像、背景等）
	BUCKET_SYSTEM_ASSETS = "tiktok-system-assets" // 系统静态资源
	BUCKET_CACHE_HOT     = "tiktok-cache-hot"     // 热点缓存（高频访问的视频）
	BUCKET_CACHE_WARM    = "tiktok-cache-warm"    // 温数据缓存（中等访问频率）
	BUCKET_CACHE_COLD    = "tiktok-cache-cold"    // 冷数据存储（归档数据）
	BUCKET_ANALYTICS     = "tiktok-analytics"     // 分析数据和日志
)

// 路径模板常量定义
const (
	// 用户目录路径模板
	USER_DIR_TEMPLATE            = "users/%d/"
	USER_VIDEOS_DIR_TEMPLATE     = "users/%d/videos/"
	USER_DRAFTS_DIR_TEMPLATE     = "users/%d/drafts/"
	USER_PROFILE_DIR_TEMPLATE    = "users/%d/profile/"
	USER_AVATAR_DIR_TEMPLATE     = "users/%d/profile/avatar/"
	USER_BACKGROUND_DIR_TEMPLATE = "users/%d/profile/background/"
	USER_VIDEO_TEMPLATE          = "users/%d/videos/%d/"

	// 视频文件路径模板
	VIDEO_SOURCE_TEMPLATE         = "users/%d/videos/%d/source/original.mp4"
	VIDEO_PROCESSED_TEMPLATE      = "users/%d/videos/%d/processed/video_%dp.mp4"
	VIDEO_THUMBNAIL_TEMPLATE      = "users/%d/videos/%d/thumbnails/thumb_%s.jpg"
	VIDEO_ANIMATED_COVER_TEMPLATE = "users/%d/videos/%d/thumbnails/animated_cover.gif"
	VIDEO_METADATA_TEMPLATE       = "users/%d/videos/%d/metadata/info.json"

	// 用户资源路径模板
	USER_AVATAR_TEMPLATE     = "users/%d/profile/avatar/avatar_%s%s" // 第二个%s为文件扩展名
	USER_BACKGROUND_TEMPLATE = "users/%d/profile/background/bg_image.jpg"

	// 热点存储路径模板
	HOT_VIDEO_TEMPLATE = "hot/users/%d/videos/%d/video_720p.mp4"

	// 临时分片路径模板
	TEMP_PART_TEMPLATE     = "temp_parts/%s/part_%d"
	TEMP_PART_DIR_TEMPLATE = "temp_parts/%s/"
)

// GetAllBuckets 获取所有需要初始化的存储桶列表
func GetAllBuckets() []string {
	return []string{
		BUCKET_USER_CONTENT,
		BUCKET_SYSTEM_ASSETS,
		BUCKET_CACHE_HOT,
		BUCKET_CACHE_WARM,
		BUCKET_CACHE_COLD,
		BUCKET_ANALYTICS,
	}
}

// GetAllBucketsNeedingPublicPolicy 获取需要设置公共读权限的存储桶列表
func GetAllBucketsNeedingPublicPolicy() []string {
	return []string{
		BUCKET_USER_CONTENT,
		BUCKET_CACHE_HOT,
	}
}
