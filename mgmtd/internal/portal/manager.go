package portal

import (
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

// Client represents an authenticated network device.
type Client struct {
	MAC          string `json:"mac"`
	IP           string `json:"ip"`
	Authorized   bool   `json:"authorized"`
	AuthorizedAt int64  `json:"authorized_at"`
	ExpiresAt    int64  `json:"expires_at"`
}

// Manager stores clients in a thread-safe in-memory map keyed by MAC.
type Manager struct {
	mu      sync.Mutex
	clients map[string]*Client
}

// NewManager creates an empty portal manager.
func NewManager() *Manager {
	return &Manager{
		clients: make(map[string]*Client),
	}
}

// Authorize creates (or renews) authorization for a MAC/IP pair.
// Duration must be positive; if d <= 0 it defaults to 60 minutes.
func (m *Manager) Authorize(mac, ip string, d time.Duration) (Client, error) {
	if d <= 0 {
		d = 60 * time.Minute
	}
	if _, err := net.ParseMAC(mac); err != nil {
		return Client{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	client := &Client{
		MAC:          mac,
		IP:           ip,
		Authorized:   true,
		AuthorizedAt: now.Unix(),
		ExpiresAt:    now.Add(d).Unix(),
	}
	m.clients[mac] = client
	return *client, nil
}

// Revoke removes authorization for a MAC. It returns true if a client was found.
func (m *Manager) Revoke(mac string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.clients[mac]
	if ok {
		delete(m.clients, mac)
	}
	return ok
}

// List returns clients sorted by MAC (ascending string order).
func (m *Manager) List() []Client {
	m.mu.Lock()
	defer m.mu.Unlock()
	keys := make([]string, 0, len(m.clients))
	for k := range m.clients {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]Client, 0, len(keys))
	for _, k := range keys {
		out = append(out, *m.clients[k])
	}
	return out
}

// Authorized returns clients that are authorized and not expired.
// Expired clients are NOT removed from storage here (Revoke does that);
// they are simply excluded from the result.
func (m *Manager) Authorized(now time.Time) []Client {
	m.mu.Lock()
	defer m.mu.Unlock()
	keys := make([]string, 0, len(m.clients))
	for k := range m.clients {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]Client, 0)
	for _, k := range keys {
		c := m.clients[k]
		if !c.Authorized || now.Unix() > c.ExpiresAt {
			continue
		}
		out = append(out, *c)
	}
	return out
}

// NormalizeMAC trims whitespace and converts to lowercase canonical form.
func NormalizeMAC(mac string) string {
	return strings.ToLower(strings.TrimSpace(mac))
}
