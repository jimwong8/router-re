package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
	"sync"
)

// FileStore is a JSON-file backed Store. It keeps the same (section, option)
// keying as MockStore but persists every Set to disk, so configuration
// survives a process restart. Snapshot/Rollback keep an in-memory copy of the
// last committed state, which is enough for the transactional semantics the
// router config UI relies on.
type FileStore struct {
	mu    sync.Mutex
	path  string
	data  map[string]string
	snaps map[string]map[string]string
	seq   int
}

// NewFileStore opens (or creates) the JSON file at path. A missing or empty
// file is treated as an empty configuration rather than an error.
func NewFileStore(path string) (*FileStore, error) {
	f := &FileStore{
		path:  path,
		data:  map[string]string{},
		snaps: map[string]map[string]string{},
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("filestore: mkdir %s: %w", filepath.Dir(path), err)
	}
	// 启动时就把"能不能写"查清楚。
	//
	// 只读文件能正常 Get，第一条 Set 才炸；更阴的是 /tmp 这种粘滞目录里
	// 目标文件属主是别人（曾被 root 跑过一次）——open 写 tmp 成功，
	// rename 覆盖目标 EPERM。线上表现为"卡密生成/模板保存 500 save failed"，
	// 而这些端点在此之前的读取全都正常，很难一眼看出是权限。
	if err := checkWritable(path); err != nil {
		return nil, fmt.Errorf("filestore: %w", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return f, nil
		}
		return nil, fmt.Errorf("filestore: read %s: %w", path, err)
	}
	if len(b) == 0 {
		return f, nil
	}
	var raw map[string]string
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("filestore: parse %s: %w", path, err)
	}
	for k, v := range raw {
		f.data[k] = v
	}
	return f, nil
}

// Get returns the value for (section, option), or "" when unset.
func (f *FileStore) Get(section, option string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.data[key(section, option)], nil
}

// Set writes (section, option) and persists the whole map to disk atomically
// (write to a temp file, then rename over the target).
func (f *FileStore) Set(section, option, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data[key(section, option)] = value
	return f.writeLocked()
}

// writeLocked persists the current map. Caller must hold f.mu.
func (f *FileStore) writeLocked() error {
	b, err := json.MarshalIndent(f.data, "", "  ")
	if err != nil {
		return fmt.Errorf("filestore: marshal: %w", err)
	}
	tmp := f.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return fmt.Errorf("filestore: write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, f.path); err != nil {
		return fmt.Errorf("filestore: rename %s: %w", f.path, err)
	}
	return nil
}

// Snapshot records the current state and returns an identifier for it.
// Unset 删除单个 (section, option)。键不存在时是 no-op。
func (f *FileStore) Unset(section, option string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.data, key(section, option))
	return f.writeLocked()
}

func (f *FileStore) Snapshot() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq++
	id := fmt.Sprintf("snap-%d", f.seq)
	cp := make(map[string]string, len(f.data))
	for k, v := range f.data {
		cp[k] = v
	}
	f.snaps[id] = cp
	return id, nil
}

// Export 返回全部配置项的只读副本（key 是 "section.option"）。
//
// 为什么需要它：Snapshot() 返回的是快照 ID 而不是内容，配置导出/迁移
// 需要真正的内容。这个方法是纯读的，不影响任何现有行为。
func (f *FileStore) Export() map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(map[string]string, len(f.data))
	for k, v := range f.data {
		out[k] = v
	}
	return out
}

// Rollback restores the state captured by Snapshot. Unknown ids are ignored.
func (f *FileStore) Rollback(snap string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp, ok := f.snaps[snap]
	if !ok {
		return fmt.Errorf("filestore: unknown snapshot %q", snap)
	}
	f.data = cp
	return f.writeLocked()
}

// Commit is a no-op for FileStore: every Set is already durable.
func (f *FileStore) Commit() error { return nil }

var _ Store = (*FileStore)(nil)

// checkWritable 在启动时验证「这个路径真的能保存」，而不是等第一次 Set 失败。
//
// 两类会静默潜伏的情况：
//  1. 目录不可写 —— 写 probe 就失败，能直接报出来；
//  2. 目标文件存在且属主不是当前用户，同时所在目录带粘滞位（/tmp、/var/tmp）
//     —— 写 tmp 成功、rename 覆盖目标 EPERM。这种最容易被误判成"程序 bug"。
func checkWritable(path string) error {
	dir := filepath.Dir(path)
	probe := filepath.Join(dir, fmt.Sprintf(".wprobe-%d-%d", os.Getpid(), time.Now().UnixNano()))
	if err := os.WriteFile(probe, []byte("{}"), 0o644); err != nil {
		return fmt.Errorf("目录不可写（%s），配置无法保存: %w", dir, err)
	}
	_ = os.Remove(probe)

	fi, err := os.Stat(path)
	if err != nil {
		return nil // 文件还不存在，首次创建没问题
	}
	di, err := os.Stat(dir)
	if err != nil || di.Mode()&os.ModeSticky == 0 {
		return nil
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	if uint32(os.Geteuid()) != st.Uid {
		return fmt.Errorf("配置文件 %s 属主是 uid=%d，当前 uid=%d 在粘滞目录里无法 rename 覆盖（会 EPERM）；请 chown 或换路径",
			path, st.Uid, os.Geteuid())
	}
	return nil
}
