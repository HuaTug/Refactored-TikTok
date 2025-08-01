package model

import (
	"fmt"
	"time"
)

// SyncEvent 同步事件表
type SyncEvent struct {
	ID             string     `gorm:"primaryKey;size:36" json:"id"`
	EventType      string     `gorm:"not null;size:50;index" json:"event_type"`
	ResourceType   string     `gorm:"not null;size:20" json:"resource_type"`
	ResourceID     int64      `gorm:"not null;index" json:"resource_id"`
	UserID         int64      `gorm:"not null;index" json:"user_id"`
	ActionType     string     `gorm:"not null;size:20" json:"action_type"`
	Status         string     `gorm:"not null;size:20;default:'pending';index" json:"status"` // pending, processing, completed, failed
	Data           string     `gorm:"type:text" json:"data"`
	RetryCount     int        `gorm:"default:0" json:"retry_count"`
	MaxRetries     int        `gorm:"default:3" json:"max_retries"`
	Priority       int        `gorm:"default:0;index" json:"priority"`
	IdempotencyKey string     `gorm:"size:255;unique;index" json:"idempotency_key"`
	ErrorMessage   string     `gorm:"type:text" json:"error_message"`
	ProcessedAt    *time.Time `json:"processed_at"`
	CreatedAt      time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (SyncEvent) TableName() string {
	return "sync_events"
}

// SyncMetrics 同步指标表
type SyncMetrics struct {
	ID          int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	MetricType  string    `gorm:"not null;size:50;index" json:"metric_type"`
	MetricName  string    `gorm:"not null;size:100" json:"metric_name"`
	MetricValue float64   `gorm:"not null" json:"metric_value"`
	Tags        string    `gorm:"type:text" json:"tags"` // JSON格式的标签
	Timestamp   time.Time `gorm:"not null;index" json:"timestamp"`
	CreatedAt   time.Time `gorm:"not null" json:"created_at"`
}

// TableName 指定表名
func (SyncMetrics) TableName() string {
	return "sync_metrics"
}

// IdempotencyRecord 幂等性记录表
type IdempotencyRecord struct {
	ID             int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	IdempotencyKey string    `gorm:"not null;size:255;unique;index" json:"idempotency_key"`
	EventID        string    `gorm:"not null;size:36;index" json:"event_id"`
	Status         string    `gorm:"not null;size:20" json:"status"` // processed, failed
	Result         string    `gorm:"type:text" json:"result"`
	ExpiresAt      time.Time `gorm:"not null;index" json:"expires_at"`
	CreatedAt      time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (IdempotencyRecord) TableName() string {
	return "idempotency_records"
}

// DistributedLock 分布式锁表
type DistributedLock struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	LockKey   string    `gorm:"not null;size:255;unique;index" json:"lock_key"`
	LockValue string    `gorm:"not null;size:255" json:"lock_value"`
	ExpiresAt time.Time `gorm:"not null;index" json:"expires_at"`
	CreatedAt time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (DistributedLock) TableName() string {
	return "distributed_locks"
}

// CacheVersion 缓存版本表
type CacheVersion struct {
	ID           int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	CacheKey     string    `gorm:"not null;size:255;unique;index" json:"cache_key"`
	Version      int64     `gorm:"not null" json:"version"`
	LastModified time.Time `gorm:"not null" json:"last_modified"`
	CreatedAt    time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (CacheVersion) TableName() string {
	return "cache_versions"
}

// SyncJobStatus 同步任务状态表
type SyncJobStatus struct {
	ID           int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	JobType      string     `gorm:"not null;size:50;index" json:"job_type"`
	JobName      string     `gorm:"not null;size:100" json:"job_name"`
	Status       string     `gorm:"not null;size:20;index" json:"status"` // running, completed, failed, paused
	Progress     float64    `gorm:"default:0" json:"progress"`            // 0-100
	StartTime    *time.Time `json:"start_time"`
	EndTime      *time.Time `json:"end_time"`
	ErrorMessage string     `gorm:"type:text" json:"error_message"`
	Metadata     string     `gorm:"type:text" json:"metadata"` // JSON格式的元数据
	CreatedAt    time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt    time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (SyncJobStatus) TableName() string {
	return "sync_job_status"
}

// EventProcessingLog 事件处理日志表
type EventProcessingLog struct {
	ID            int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	EventID       string     `gorm:"not null;size:36;index" json:"event_id"`
	EventType     string     `gorm:"not null;size:50;index" json:"event_type"`
	ProcessorName string     `gorm:"not null;size:100" json:"processor_name"`
	Status        string     `gorm:"not null;size:20;index" json:"status"` // started, completed, failed
	StartTime     time.Time  `gorm:"not null" json:"start_time"`
	EndTime       *time.Time `json:"end_time"`
	Duration      int64      `json:"duration"` // 处理时长（毫秒）
	ErrorMessage  string     `gorm:"type:text" json:"error_message"`
	Metadata      string     `gorm:"type:text" json:"metadata"`
	CreatedAt     time.Time  `gorm:"not null" json:"created_at"`
}

// TableName 指定表名
func (EventProcessingLog) TableName() string {
	return "event_processing_logs"
}

// DataConsistencyCheck 数据一致性检查表
type DataConsistencyCheck struct {
	ID            int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	CheckType     string     `gorm:"not null;size:50;index" json:"check_type"`
	ResourceType  string     `gorm:"not null;size:20" json:"resource_type"`
	ResourceID    int64      `gorm:"not null;index" json:"resource_id"`
	CacheValue    string     `gorm:"type:text" json:"cache_value"`
	DatabaseValue string     `gorm:"type:text" json:"database_value"`
	IsConsistent  bool       `gorm:"not null;index" json:"is_consistent"`
	Difference    string     `gorm:"type:text" json:"difference"`
	CheckTime     time.Time  `gorm:"not null;index" json:"check_time"`
	FixedAt       *time.Time `json:"fixed_at"`
	CreatedAt     time.Time  `gorm:"not null" json:"created_at"`
}

// TableName 指定表名
func (DataConsistencyCheck) TableName() string {
	return "data_consistency_checks"
}

// AutoMigrate 自动迁移所有表
func AutoMigrateEventDrivenTables(db interface{}) error {
	type Migrator interface {
		AutoMigrate(dst ...interface{}) error
	}

	migrator, ok := db.(Migrator)
	if !ok {
		return fmt.Errorf("database does not support auto migration")
	}

	return migrator.AutoMigrate(
		&SyncEvent{},
		&SyncMetrics{},
		&IdempotencyRecord{},
		&DistributedLock{},
		&CacheVersion{},
		&SyncJobStatus{},
		&EventProcessingLog{},
		&DataConsistencyCheck{},
	)
}
