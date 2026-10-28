// mgmtd/internal/config/mock.go
package config

import (
	"strconv"
	"sync"
)

// MockStore 是基于内存 map 的 Store 实现，用于测试。
// 快照采用浅拷贝 map，Rollback 用快照整体替换当前数据，实现回滚语义。
type MockStore struct {
	mu    sync.Mutex
	data  map[string]string
	snaps map[string]map[string]string
	seq   int
}

// NewMockStore 创建一个空的 MockStore。
func NewMockStore() *MockStore {
	return &MockStore{
		data:  map[string]string{},
		snaps: map[string]map[string]string{},
	}
}

// Export 返回全部配置项的只读副本（key 是 "section.option"）。
// 与 FileStore.Export 同义：配置导出/迁移需要真实内容，
// 而 Snapshot() 返回的是快照 ID。两个实现都必须有，
// 否则服务在没配 MGM_CONFIG_FILE 时会跑 MockStore，
// 导出端点会因为类型断言失败而永远返回空配置（已踩到）。
func (m *MockStore) Export() map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]string, len(m.data))
	for k, v := range m.data {
		out[k] = v
	}
	return out
}

// key 将 (section, option) 拼成内部存储键。
func key(section, option string) string { return section + "." + option }

// Get 返回指定配置项的值。不存在时返回空字符串。
func (m *MockStore) Get(section, option string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.data[key(section, option)], nil
}

// Set 写入一个配置项。
func (m *MockStore) Set(section, option, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key(section, option)] = value
	return nil
}

// Unset 删除单个 (section, option)。
func (m *MockStore) Unset(section, option string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key(section, option))
	return nil
}

// Snapshot 保存当前配置的副本，返回快照标识符。
func (m *MockStore) Snapshot() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	id := strconv.Itoa(m.seq)
	cp := make(map[string]string, len(m.data))
	for k, v := range m.data {
		cp[k] = v
	}
	m.snaps[id] = cp
	return id, nil
}

// Rollback 将配置恢复到指定快照的状态。快照不存在时静默返回 nil。
func (m *MockStore) Rollback(snap string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cp, ok := m.snaps[snap]; ok {
		m.data = cp
	}
	return nil
}

// Commit 在内存实现中无操作，持久化存储于此处固化为提交。
func (m *MockStore) Commit() error { return nil }
