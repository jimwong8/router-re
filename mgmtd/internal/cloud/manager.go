package cloud

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Device represents a cloud-managed router device.
type Device struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Model         string `json:"model"`
	Version       string `json:"version"`
	LastSeen      int64  `json:"last_seen"`
	Online        bool   `json:"online"`
	ConfigVersion int    `json:"config_version"`
}

// Manager is an in-memory cloud device manager (no DB).
type Manager struct {
	mu      sync.RWMutex
	devices map[string]Device
	configs map[string]map[string]string
}

// NewManager returns a Manager ready for use.
func NewManager() *Manager {
	return &Manager{
		devices: make(map[string]Device),
		configs: make(map[string]map[string]string),
	}
}

// newID returns a random 32-hex device ID.
func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("dev-%d", time.Now().Unix())
	}
	return hex.EncodeToString(b)
}

// Register adds a device, generating an ID if not provided. Existing IDs are
// updated with the latest metadata (but not overwritten with empty fields).
func (m *Manager) Register(dev Device) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if dev.ID == "" {
		dev.ID = newID()
	}
	cur, ok := m.devices[dev.ID]
	if ok {
		if dev.Name == "" {
			dev.Name = cur.Name
		}
		if dev.Model == "" {
			dev.Model = cur.Model
		}
		if dev.Version == "" {
			dev.Version = cur.Version
		}
	}
	if _, exists := m.configs[dev.ID]; !exists {
		m.configs[dev.ID] = make(map[string]string)
	}
	m.devices[dev.ID] = dev
	return nil
}

// Heartbeat updates the device's LastSeen/Online status.
func (m *Manager) Heartbeat(id string, ts int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	dev, ok := m.devices[id]
	if !ok {
		return fmt.Errorf("device %q not registered", id)
	}
	dev.LastSeen = ts
	dev.Online = true
	m.devices[id] = dev
	return nil
}

// List returns a copy of all devices, sorted by ID.
func (m *Manager) List() []Device {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Device, 0, len(m.devices))
	for _, d := range m.devices {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// PushConfig stores a new config snapshot for the device and bumps
// ConfigVersion. Returns the new version number.
func (m *Manager) PushConfig(id string, cfg map[string]string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	dev, ok := m.devices[id]
	if !ok {
		return 0, fmt.Errorf("device %q not registered", id)
	}
	store := m.configs[id]
	if store == nil {
		store = make(map[string]string)
		m.configs[id] = store
	}
	for k, v := range cfg {
		store[k] = v
	}
	dev.ConfigVersion++
	m.devices[id] = dev
	return dev.ConfigVersion, nil
}

// PollConfig returns the device's full config if its version is newer than
// sinceVersion, otherwise an empty map (no change). Returns (cfg, currentVersion).
func (m *Manager) PollConfig(id string, sinceVersion int) (map[string]string, int) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	dev, ok := m.devices[id]
	if !ok {
		return nil, 0
	}
	if dev.ConfigVersion <= sinceVersion {
		return nil, dev.ConfigVersion
	}
	cfg := make(map[string]string, len(m.configs[id]))
	for k, v := range m.configs[id] {
		cfg[k] = v
	}
	return cfg, dev.ConfigVersion
}
