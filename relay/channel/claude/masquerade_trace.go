package claude

import (
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	// MaxTraceRecords 最大追踪记录数
	MaxTraceRecords = 100
)

// MasqueradeTraceRecord 伪装追踪记录
type MasqueradeTraceRecord struct {
	ID          string `json:"id"`           // 唯一标识 (UUID)
	Timestamp   int64  `json:"timestamp"`    // 请求时间戳
	Model       string `json:"model"`        // 请求的模型名
	ChannelID   int    `json:"channel_id"`   // 渠道ID
	ChannelName string `json:"channel_name"` // 渠道名称

	// 原始请求
	OriginalHeaders map[string]string `json:"original_headers"`
	OriginalBody    string            `json:"original_body"`

	// 伪装后请求
	MaskedHeaders map[string]string `json:"masked_headers"`
	MaskedBody    string            `json:"masked_body"`

	// 伪装元信息对比
	OriginalUserID  string `json:"original_user_id"` // 原始用户ID
	MaskedUserID    string `json:"masked_user_id"`   // 伪装后用户ID
	OriginalSession string `json:"original_session"` // 原始会话ID
	MaskedSession   string `json:"masked_session"`   // 伪装后会话ID
}

// MasqueradeTraceStore 环形缓冲区存储
type MasqueradeTraceStore struct {
	records [MaxTraceRecords]*MasqueradeTraceRecord
	index   int          // 当前写入位置
	count   int          // 实际记录数
	mutex   sync.RWMutex // 读写锁
}

var (
	globalMasqueradeTraceStore     *MasqueradeTraceStore
	globalMasqueradeTraceStoreOnce sync.Once
)

// GetMasqueradeTraceStore 获取全局追踪存储实例
func GetMasqueradeTraceStore() *MasqueradeTraceStore {
	globalMasqueradeTraceStoreOnce.Do(func() {
		globalMasqueradeTraceStore = &MasqueradeTraceStore{}
	})
	return globalMasqueradeTraceStore
}

// Add 添加追踪记录
func (s *MasqueradeTraceStore) Add(record *MasqueradeTraceRecord) {
	if record == nil {
		return
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	// 生成唯一ID
	if record.ID == "" {
		record.ID = uuid.New().String()
	}

	// 设置时间戳
	if record.Timestamp == 0 {
		record.Timestamp = time.Now().Unix()
	}

	// 写入环形缓冲区
	s.records[s.index] = record
	s.index = (s.index + 1) % MaxTraceRecords

	if s.count < MaxTraceRecords {
		s.count++
	}
}

// GetAll 获取所有追踪记录（按时间倒序）
func (s *MasqueradeTraceStore) GetAll() []*MasqueradeTraceRecord {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	result := make([]*MasqueradeTraceRecord, 0, s.count)

	// 从最新到最旧遍历
	for i := 0; i < s.count; i++ {
		// 计算实际索引（从最新开始）
		idx := (s.index - 1 - i + MaxTraceRecords) % MaxTraceRecords
		if s.records[idx] != nil {
			result = append(result, s.records[idx])
		}
	}

	return result
}

// Clear 清空所有记录
func (s *MasqueradeTraceStore) Clear() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	for i := 0; i < MaxTraceRecords; i++ {
		s.records[i] = nil
	}
	s.index = 0
	s.count = 0
}

// Count 获取当前记录数
func (s *MasqueradeTraceStore) Count() int {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.count
}

func extractSessionFromUserID(userID string) string {
	if userID == "" || userID == "<empty>" {
		return ""
	}
	if idx := strings.Index(userID, "session_"); idx != -1 {
		return userID[idx+8:]
	}
	return ""
}
