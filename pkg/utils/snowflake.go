package utils

import (
	"errors"
	"sync"
	"time"
)

const (
	epoch             = int64(1577836800000)                           // 起始时间戳 (2020-01-01)
	timestampBits     = uint(41)                                       // 时间戳位数
	datacenterIDBits  = uint(5)                                        // 数据中心ID位数
	workerIDBits      = uint(5)                                        // 工作节点ID位数
	sequenceBits      = uint(12)                                       // 序列号位数
	maxDatacenterID   = int64(-1 ^ (-1 << datacenterIDBits))           // 最大数据中心ID
	maxWorkerID       = int64(-1 ^ (-1 << workerIDBits))               // 最大工作节点ID
	maxSequence       = int64(-1 ^ (-1 << sequenceBits))               // 最大序列号
	timestampShift    = sequenceBits + workerIDBits + datacenterIDBits // 时间戳左移位数
	datacenterIDShift = sequenceBits + workerIDBits                    // 数据中心ID左移位数
	workerIDShift     = sequenceBits                                   // 工作节点ID左移位数
)

// Snowflake 雪花算法结构体
type Snowflake struct {
	mutex        sync.Mutex
	lastTime     int64
	workerID     int64
	datacenterID int64
	sequence     int64
}

// NewSnowflake 创建新的雪花算法实例
func NewSnowflake(workerID, datacenterID int64) (*Snowflake, error) {
	if workerID < 0 || workerID > maxWorkerID {
		return nil, errors.New("worker ID out of range")
	}
	if datacenterID < 0 || datacenterID > maxDatacenterID {
		return nil, errors.New("datacenter ID out of range")
	}
	return &Snowflake{
		workerID:     workerID,
		datacenterID: datacenterID,
	}, nil
}

// GenerateID 生成唯一ID
func (s *Snowflake) GenerateID() int64 {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	currentTime := time.Now().UnixNano() / 1e6 // 毫秒
	if currentTime < s.lastTime {
		// 时钟回拨，等待
		time.Sleep(time.Duration(s.lastTime-currentTime) * time.Millisecond)
		currentTime = time.Now().UnixNano() / 1e6
	}

	if currentTime == s.lastTime {
		s.sequence = (s.sequence + 1) & maxSequence
		if s.sequence == 0 {
			// 序列号用完，等待下一毫秒
			for currentTime <= s.lastTime {
				currentTime = time.Now().UnixNano() / 1e6
			}
		}
	} else {
		s.sequence = 0
	}

	s.lastTime = currentTime
	id := ((currentTime - epoch) << timestampShift) |
		(s.datacenterID << datacenterIDShift) |
		(s.workerID << workerIDShift) |
		s.sequence

	return id
}

// ParseID 解析ID获取各部分信息
func (s *Snowflake) ParseID(id int64) (timestamp int64, datacenterID, workerID, sequence int64) {
	timestamp = (id >> timestampShift) + epoch
	datacenterID = (id >> datacenterIDShift) & maxDatacenterID
	workerID = (id >> workerIDShift) & maxWorkerID
	sequence = id & maxSequence
	return
}

// GlobalSnowflake 全局雪花算法实例
var GlobalSnowflake *Snowflake

// InitSnowflake 初始化全局雪花算法实例
func InitSnowflake(workerID, datacenterID int64) error {
	var err error
	GlobalSnowflake, err = NewSnowflake(workerID, datacenterID)
	return err
}

// GenerateCommentID 生成评论ID
func GenerateCommentID() int64 {
	if GlobalSnowflake == nil {
		// 如果未初始化，使用默认配置
		_ = InitSnowflake(1, 1)
	}
	return GlobalSnowflake.GenerateID()
}
