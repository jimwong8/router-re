package account

import (
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

// Manager is a thread-safe in-memory account store keyed by username.
type Manager struct {
	mu       sync.RWMutex
	accounts map[string]Account
}

// NewManager creates an empty Manager.
func NewManager() *Manager {
	return &Manager{accounts: make(map[string]Account)}
}

// Create inserts a new account. It fails if the username already exists.
func (m *Manager) Create(a Account) error {
	if err := Validate(a); err != nil {
		return err
	}
	key := strings.ToLower(a.Username)
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.accounts[key]; ok {
		return fmt.Errorf("account %q already exists", a.Username)
	}
	m.accounts[key] = a
	return nil
}

// CreateReloaded inserts an account that was restored from the config store.
// The store never persists plaintext passwords, so the password rule is
// skipped here; every other rule still applies. Use Update for operator edits.
func (m *Manager) CreateReloaded(a Account) error {
	if err := ValidateExceptPassword(a); err != nil {
		return err
	}
	key := strings.ToLower(a.Username)
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.accounts[key]; ok {
		return fmt.Errorf("account %q already exists", a.Username)
	}
	m.accounts[key] = a
	return nil
}

// Update replaces an existing account. It fails if the account is missing.
//
// An empty Password means "keep the stored credential": the account was
// reloaded from the config store (which never persists plaintext passwords),
// so validation skips the password rule and the existing password is
// preserved. A non-empty password is validated and replaces the old one.
func (m *Manager) Update(a Account) error {
	if a.Password == "" {
		if err := ValidateExceptPassword(a); err != nil {
			return err
		}
	} else if err := Validate(a); err != nil {
		return err
	}
	key := strings.ToLower(a.Username)
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.accounts[key]
	if !ok {
		return fmt.Errorf("account %q not found", a.Username)
	}
	if a.Password == "" {
		a.Password = cur.Password
	}
	m.accounts[key] = a
	return nil
}

// Delete removes an account by username. It reports whether it existed.
func (m *Manager) Delete(username string) bool {
	key := strings.ToLower(username)
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.accounts[key]; !ok {
		return false
	}
	delete(m.accounts, key)
	return true
}

// Get returns the account and whether it exists.
func (m *Manager) Get(username string) (Account, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	a, ok := m.accounts[strings.ToLower(username)]
	return a, ok
}

// List returns all accounts sorted by username (stable ascending order).
func (m *Manager) List() []Account {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Account, 0, len(m.accounts))
	for _, a := range m.accounts {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Username) < strings.ToLower(out[j].Username)
	})
	return out
}

// ResetUsage zeroes the used quota for an account. It reports whether it existed.
func (m *Manager) ResetUsage(username string) bool {
	key := strings.ToLower(username)
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.accounts[key]
	if !ok {
		return false
	}
	a.UsedMB = 0
	m.accounts[key] = a
	return true
}

// BindMAC attaches a MAC address to an account after validating it.
func (m *Manager) BindMAC(username, mac string) error {
	if _, err := net.ParseMAC(mac); err != nil {
		return fmt.Errorf("invalid mac address %q", mac)
	}
	key := strings.ToLower(username)
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.accounts[key]
	if !ok {
		return fmt.Errorf("account %q not found", username)
	}
	a.BoundMAC = strings.ToLower(mac)
	m.accounts[key] = a
	return nil
}

// BulkImport parses CSV lines of the form
// "username,password,type,profile,quota_mb,expire_days".
// Blank lines and lines starting with '#' are skipped. A failure on one line
// does not abort the rest; errors are collected and returned.
func (m *Manager) BulkImport(lines []string) (created int, errs []string) {
	now := time.Now().Unix()
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) < 3 {
			errs = append(errs, fmt.Sprintf("line %d: need at least 3 fields", i+1))
			continue
		}
		for j := range parts {
			parts[j] = strings.TrimSpace(parts[j])
		}
		quota := 0
		if len(parts) >= 5 && parts[4] != "" {
			if _, err := fmt.Sscanf(parts[4], "%d", &quota); err != nil {
				errs = append(errs, fmt.Sprintf("line %d: bad quota_mb %q", i+1, parts[4]))
				continue
			}
		}
		expire := int64(0)
		if len(parts) >= 6 && parts[5] != "" {
			days := 0
			if _, err := fmt.Sscanf(parts[5], "%d", &days); err != nil || days < 0 {
				errs = append(errs, fmt.Sprintf("line %d: bad expire_days %q", i+1, parts[5]))
				continue
			}
			if days > 0 {
				expire = now + int64(days)*86400
			}
		}
		profile := ""
		if len(parts) >= 4 {
			profile = parts[3]
		}
		a := Account{
			Username:  parts[0],
			Password:  parts[1],
			Type:      parts[2],
			Profile:   profile,
			QuotaMB:   quota,
			ExpireAt:  expire,
			Enabled:   true,
			CreatedAt: now,
		}
		if err := m.Create(a); err != nil {
			errs = append(errs, fmt.Sprintf("line %d: %v", i+1, err))
			continue
		}
		created++
	}
	return created, errs
}
