package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
