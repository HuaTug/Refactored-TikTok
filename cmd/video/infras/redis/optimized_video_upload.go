package redis

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/go-redis/redis"
)

// OptimizedUploadManager 优化的上传管理器
type OptimizedUploadManager struct {
	client *redis.Client
	ctx    context.Context
	ttl    time.Duration
}

// UploadSessionData 上传会话数据结构
type UploadSessionData struct {
	UUID        string    `json:"uuid"`
	UserID      int64     `json:"user_id"`
	VideoID     int64     `json:"video_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Category    string    `json:"category"`
	LabName     string    `json:"lab_name"`
	TotalChunks int64     `json:"total_chunks"`
	Status      string    `json:"status"` // pending, uploading, processing, completed, failed
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// UploadProgress 上传进度
type UploadProgress struct {
	UUID          string   `json:"uuid"`
	TotalChunks   int64    `json:"total_chunks"`
	UploadedCount int64    `json:"uploaded_count"`
	Progress      float64  `json:"progress"`
	MissingChunks []string `json:"missing_chunks"`
	Status        string   `json:"status"`
	EstimatedTime int64    `json:"estimated_time_seconds"`
}

// NewOptimizedUploadManager 创建优化的上传管理器
func NewOptimizedUploadManager(ctx context.Context) *OptimizedUploadManager {
	return &OptimizedUploadManager{
		client: redisDBVideoUpload,
		ctx:    ctx,
		ttl:    24 * time.Hour, // 24小时TTL
	}
}

// generateSecureUUID 生成安全的UUID
func (o *OptimizedUploadManager) generateSecureUUID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// CreateUploadSession 原子性创建上传会话
func (o *OptimizedUploadManager) CreateUploadSession(userID int64, title, description, category, labName string, totalChunks int64) (*UploadSessionData, error) {
	uuid := o.generateSecureUUID()
	sessionKey := fmt.Sprintf("session:%d:%s", userID, uuid)
	chunksKey := fmt.Sprintf("chunks:%d:%s", userID, uuid)
	lockKey := fmt.Sprintf("lock:%d:%s", userID, uuid)

	// 1. 尝试获取分布式锁
	locked, err := o.client.SetNX(lockKey, "1", 10*time.Second).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to acquire lock: %w", err)
	}
	if !locked {
		return nil, fmt.Errorf("session creation in progress")
	}
	defer o.client.Del(lockKey)

	// 2. 创建会话数据
	session := &UploadSessionData{
		UUID:        uuid,
		UserID:      userID,
		VideoID:     0, // 将在后续分配
		Title:       title,
		Description: description,
		Category:    category,
		LabName:     labName,
		TotalChunks: totalChunks,
		Status:      "pending",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		ExpiresAt:   time.Now().Add(o.ttl),
	}

	// 3. 序列化会话数据
	sessionJSON, err := json.Marshal(session)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal session: %w", err)
	}

	// 4. 使用事务原子性创建
	pipe := o.client.TxPipeline()

	// 检查会话是否已存在
	pipe.Exists(sessionKey)

	// 创建会话
	pipe.Set(sessionKey, sessionJSON, o.ttl)

	// 初始化分片集合
	pipe.Del(chunksKey)
	pipe.Expire(chunksKey, o.ttl)

	// 为兼容性，也创建List格式（向后兼容）
	compatKey := fmt.Sprintf("l:%d:%s", userID, uuid)
	pipe.RPush(compatKey, totalChunks, title, description, labName, category)
	pipe.Expire(compatKey, o.ttl)

	results, err := pipe.Exec()
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	// 检查会话是否已存在
	if exists := results[0].(*redis.IntCmd).Val(); exists > 0 {
		return nil, fmt.Errorf("session already exists")
	}

	hlog.Infof("Created upload session %s for user %d with %d chunks", uuid, userID, totalChunks)
	return session, nil
}

// MarkChunkUploaded 原子性标记分片上传（使用Lua脚本）
func (o *OptimizedUploadManager) MarkChunkUploaded(userID int64, uuid string, chunkNumber int64) (*UploadProgress, error) {
	sessionKey := fmt.Sprintf("session:%d:%s", userID, uuid)
	chunksKey := fmt.Sprintf("chunks:%d:%s", userID, uuid)
	bitmapKey := fmt.Sprintf("b:%d:%s", userID, uuid) // 兼容性

	// Lua脚本确保原子性
	luaScript := `
		local session_key = KEYS[1]
		local chunks_key = KEYS[2]
		local bitmap_key = KEYS[3]
		local chunk_num = ARGV[1]
		local timestamp = ARGV[2]
		
		-- 检查会话是否存在
		local session_data = redis.call('GET', session_key)
		if not session_data then
			return {error = 'session_not_found'}
		end
		
		-- 解析会话数据
		local session = cjson.decode(session_data)
		
		-- 验证分片编号
		if tonumber(chunk_num) < 1 or tonumber(chunk_num) > tonumber(session.total_chunks) then
			return {error = 'invalid_chunk_number'}
		end
		
		-- 检查分片是否已上传（幂等性）
		if redis.call('SISMEMBER', chunks_key, chunk_num) == 1 then
			local uploaded_count = redis.call('SCARD', chunks_key)
			return {
				uploaded = uploaded_count,
				total = session.total_chunks,
				progress = (uploaded_count / session.total_chunks) * 100,
				already_uploaded = true
			}
		end
		
		-- 添加分片到Set
		redis.call('SADD', chunks_key, chunk_num)
		redis.call('EXPIRE', chunks_key, 86400)
		
		-- 更新Bitmap（兼容性）
		redis.call('SETBIT', bitmap_key, chunk_num - 1, 1)
		redis.call('EXPIRE', bitmap_key, 86400)
		
		-- 更新会话状态
		session.status = 'uploading'
		session.updated_at = timestamp
		local updated_session = cjson.encode(session)
		redis.call('SET', session_key, updated_session, 'EX', 86400)
		
		-- 返回进度信息
		local uploaded_count = redis.call('SCARD', chunks_key)
		local is_complete = (uploaded_count == tonumber(session.total_chunks))
		
		if is_complete then
			session.status = 'ready_to_merge'
			updated_session = cjson.encode(session)
			redis.call('SET', session_key, updated_session, 'EX', 86400)
		end
		
		return {
			uploaded = uploaded_count,
			total = session.total_chunks,
			progress = (uploaded_count / session.total_chunks) * 100,
			complete = is_complete,
			already_uploaded = false
		}
	`

	result, err := o.client.Eval(luaScript,
		[]string{sessionKey, chunksKey, bitmapKey},
		chunkNumber, time.Now().Format(time.RFC3339)).Result()

	if err != nil {
		return nil, fmt.Errorf("failed to mark chunk uploaded: %w", err)
	}

	// 解析结果
	resultMap, ok := result.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected result format")
	}

	if errMsg, exists := resultMap["error"]; exists {
		return nil, fmt.Errorf("script error: %v", errMsg)
	}

	uploaded := int64(resultMap["uploaded"].(int64))
	total := int64(resultMap["total"].(int64))
	progress := resultMap["progress"].(float64)
	isComplete := false
	if complete, exists := resultMap["complete"]; exists {
		isComplete = complete.(bool)
	}

	hlog.Infof("Chunk %d uploaded for session %s, progress: %.2f%% (%d/%d)",
		chunkNumber, uuid, progress, uploaded, total)

	return &UploadProgress{
		UUID:          uuid,
		TotalChunks:   total,
		UploadedCount: uploaded,
		Progress:      progress,
		Status:        map[bool]string{true: "ready_to_merge", false: "uploading"}[isComplete],
	}, nil
}

// GetUploadProgress 获取上传进度
func (o *OptimizedUploadManager) GetUploadProgress(userID int64, uuid string) (*UploadProgress, error) {
	sessionKey := fmt.Sprintf("session:%d:%s", userID, uuid)
	chunksKey := fmt.Sprintf("chunks:%d:%s", userID, uuid)

	// 使用Pipeline批量获取数据
	pipe := o.client.Pipeline()
	sessionCmd := pipe.Get(sessionKey)
	uploadedCountCmd := pipe.SCard(chunksKey)
	uploadedChunksCmd := pipe.SMembers(chunksKey)

	_, err := pipe.Exec()
	if err == redis.Nil {
		return nil, fmt.Errorf("session not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get progress: %w", err)
	}

	// 解析会话数据
	var session UploadSessionData
	if err := json.Unmarshal([]byte(sessionCmd.Val()), &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	uploadedCount := uploadedCountCmd.Val()
	uploadedChunks := uploadedChunksCmd.Val()

	// 计算缺失的分片
	missingChunks := make([]string, 0)
	uploadedSet := make(map[string]bool)
	for _, chunk := range uploadedChunks {
		uploadedSet[chunk] = true
	}

	for i := int64(1); i <= session.TotalChunks; i++ {
		if !uploadedSet[strconv.FormatInt(i, 10)] {
			missingChunks = append(missingChunks, strconv.FormatInt(i, 10))
		}
	}

	// 估算剩余时间
	estimatedTime := int64(0)
	if uploadedCount > 0 {
		elapsed := time.Since(session.CreatedAt).Seconds()
		avgTimePerChunk := elapsed / float64(uploadedCount)
		estimatedTime = int64(avgTimePerChunk * float64(len(missingChunks)))
	}

	return &UploadProgress{
		UUID:          uuid,
		TotalChunks:   session.TotalChunks,
		UploadedCount: uploadedCount,
		Progress:      float64(uploadedCount) / float64(session.TotalChunks) * 100,
		MissingChunks: missingChunks,
		Status:        session.Status,
		EstimatedTime: estimatedTime,
	}, nil
}

// GetSession 获取会话信息
func (o *OptimizedUploadManager) GetSession(userID int64, uuid string) (*UploadSessionData, error) {
	sessionKey := fmt.Sprintf("session:%d:%s", userID, uuid)

	sessionJSON, err := o.client.Get(sessionKey).Result()
	if err == redis.Nil {
		return nil, fmt.Errorf("session not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	var session UploadSessionData
	if err := json.Unmarshal([]byte(sessionJSON), &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	return &session, nil
}

// CleanupExpiredSessions 清理过期会话
func (o *OptimizedUploadManager) CleanupExpiredSessions() error {
	// 使用SCAN命令遍历所有会话
	iter := o.client.Scan(0, "session:*", 100).Iterator()

	var expiredKeys []string
	for iter.Next() {
		key := iter.Val()

		// 检查TTL
		ttl, err := o.client.TTL(key).Result()
		if err != nil {
			continue
		}

		if ttl <= 0 { // 已过期或没有TTL
			expiredKeys = append(expiredKeys, key)
		}
	}

	if len(expiredKeys) > 0 {
		// 批量删除过期会话
		pipe := o.client.TxPipeline()
		for _, key := range expiredKeys {
			pipe.Del(key)
		}
		_, err := pipe.Exec()

		hlog.Infof("Cleaned up %d expired upload sessions", len(expiredKeys))
		return err
	}

	return nil
}
