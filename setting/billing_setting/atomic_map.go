package billing_setting

import (
	"encoding/json"
	"sync/atomic"
)

// AtomicStringMap 是只读快照式的 map：config 管理器通过 UnmarshalJSON 整体替换，
// 请求定价路径通过 Load 读取不可变快照，二者无需加锁也不会产生数据竞争。
// 快照一经发布不再修改，调用方不得写入 Load 返回的 map。
type AtomicStringMap struct {
	p *atomic.Pointer[map[string]string]
}

func NewAtomicStringMap() AtomicStringMap {
	m := AtomicStringMap{p: &atomic.Pointer[map[string]string]{}}
	empty := map[string]string{}
	m.p.Store(&empty)
	return m
}

func (m AtomicStringMap) Load() map[string]string {
	if m.p == nil {
		return nil
	}
	if snapshot := m.p.Load(); snapshot != nil {
		return *snapshot
	}
	return nil
}

func (m AtomicStringMap) Get(key string) (string, bool) {
	value, ok := m.Load()[key]
	return value, ok
}

func (m AtomicStringMap) MarshalJSON() ([]byte, error) {
	snapshot := m.Load()
	if snapshot == nil {
		snapshot = map[string]string{}
	}
	return json.Marshal(snapshot)
}

// UnmarshalJSON 解析到新 map 后原子替换，删除的键随之清除；解析失败保留旧快照。
func (m *AtomicStringMap) UnmarshalJSON(data []byte) error {
	fresh := map[string]string{}
	if err := json.Unmarshal(data, &fresh); err != nil {
		return err
	}
	if m.p == nil {
		m.p = &atomic.Pointer[map[string]string]{}
	}
	m.p.Store(&fresh)
	return nil
}
