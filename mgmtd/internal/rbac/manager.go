package rbac

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// Manager stores administrator accounts in memory and manages their lifecycle.
// It is safe for concurrent use.
type Manager struct {
	mu    sync.RWMutex
	users map[string]User
}

// NewManager builds a manager seeded with a single admin account whose
// credentials come from the same source as the legacy login (cfg.AdminUser /
// cfg.AdminPassword, default admin / admin123). The password is stored only as
// its PBKDF2 hash.
func NewManager(adminUser, adminPassword string) *Manager {
	if adminUser == "" {
		adminUser = "admin"
	}
	m := &Manager{users: make(map[string]User)}
	if adminPassword == "" {
		adminPassword = "admin123"
	}
	now := time.Now().Unix()
	// Hash the seeded admin password; if hashing fails we still create the
	// account with an empty hash (locked out) rather than crashing startup.
	hash, _ := HashPassword(adminPassword)
	m.users[adminUser] = User{
		Username:     adminUser,
		PasswordHash: hash,
		Role:         RoleAdmin,
		Enabled:      true,
		CreatedAt:    now,
	}
	return m
}

// Create adds a new user (username must be unique). The caller supplies a
// PasswordHash that is already hashed via HashPassword.
func (m *Manager) Create(u User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.users[u.Username]; exists {
		return fmt.Errorf("user %q already exists", u.Username)
	}
	if err := ValidateUser(u); err != nil {
		return err
	}
	u.CreatedAt = time.Now().Unix()
	if u.LastLogin == 0 {
		u.LastLogin = 0
	}
	m.users[u.Username] = u
	return nil
}

// Update overwrites a user's role/enabled flags. PasswordHash is preserved if empty.
func (m *Manager) Update(u User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.users[u.Username]
	if !ok {
		return fmt.Errorf("user %q not found", u.Username)
	}
	if err := ValidateUser(u); err != nil {
		return err
	}
	if u.PasswordHash == "" {
		u.PasswordHash = existing.PasswordHash
	}
	u.CreatedAt = existing.CreatedAt
	m.users[u.Username] = u
	return nil
}

// Delete removes a user; rejecting removal of the last admin prevents lockout.
func (m *Manager) Delete(username string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	target, ok := m.users[username]
	if !ok {
		return fmt.Errorf("user %q not found", username)
	}
	if target.Role == RoleAdmin && !m.hasAnotherAdminLocked(username) {
		return fmt.Errorf("cannot delete the last admin")
	}
	delete(m.users, username)
	return nil
}

// hasAnotherAdminLocked requires the caller to already hold m.mu.
func (m *Manager) hasAnotherAdminLocked(username string) bool {
	for name, u := range m.users {
		if name == username {
			continue
		}
		if u.Role == RoleAdmin && u.Enabled {
			return true
		}
	}
	return false
}

// Get returns the user by username.
func (m *Manager) Get(username string) (User, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	u, ok := m.users[username]
	return u, ok
}

// List returns all users sorted by username. PasswordHash is scrubbed.
func (m *Manager) List() []User {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]User, 0, len(m.users))
	for _, u := range m.users {
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Username < out[j].Username })
	return out
}

// RoleOf resolves a username to its role (false if absent).
func (m *Manager) RoleOf(username string) (Role, bool) {
	u, ok := m.Get(username)
	return u.Role, ok
}

// Authenticate verifies a username/password pair against stored hashes.
func (m *Manager) Authenticate(username, password string) (User, bool) {
	u, ok := m.Get(username)
	if !ok || !u.Enabled {
		return User{}, false
	}
	if !VerifyPassword(password, u.PasswordHash) {
		return User{}, false
	}
	return u, true
}

// ChangePassword re-hashes and updates a user's password.
func (m *Manager) ChangePassword(username, newPassword string) error {
	if err := ValidatePassword(newPassword); err != nil {
		return err
	}
	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[username]
	if !ok {
		return fmt.Errorf("user %q not found", username)
	}
	u.PasswordHash = hash
	m.users[username] = u
	return nil
}
