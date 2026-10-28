package rbac

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// persist.go —— 账号落到磁盘。
//
// 原来 Manager 只有内存 map，Create 出来的账号重启就没了。实测：建了 viewer
// 账号、重启 mgmtd 之后该账号直接登录不上（token 拿不到），等于每次重启都
// 把多用户管理清空一次 —— 那这个功能就只是演示。
//
// 设计取舍：
//   - 存 JSON 数组，字段就是 User（含 PasswordHash，已经是 PBKDF2，不是明文）；
//   - 0600 + 目录 0700，只有跑 mgmtd 的账号能读；
//   - 写用「临时文件 + rename」，避免写一半崩了把账号表弄坏；
//   - 文件不存在 = 空表（首次启动），不算错误。

// fileData 是落盘格式。带 version 便于以后改结构。
//
// 注意不能直接复用 User：它的 PasswordHash 是 `json:"-"`（API 响应绝不能
// 泄漏哈希），拿它序列化会把哈希整列丢掉，重启后所有密码都失效。
// 落盘用自己的结构，显式写 password_hash。
type fileData struct {
	Version int             `json:"version"`
	Users   []persistedUser `json:"users"`
}

type persistedUser struct {
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
	Role         Role   `json:"role"`
	Enabled      bool   `json:"enabled"`
	CreatedAt    int64  `json:"created_at"`
	LastLogin    int64  `json:"last_login"`
}

func toPersisted(u User) persistedUser {
	return persistedUser{
		Username:     u.Username,
		PasswordHash: u.PasswordHash,
		Role:         u.Role,
		Enabled:      u.Enabled,
		CreatedAt:    u.CreatedAt,
		LastLogin:    u.LastLogin,
	}
}

func (p persistedUser) toUser() User {
	return User{
		Username:     p.Username,
		PasswordHash: p.PasswordHash,
		Role:         p.Role,
		Enabled:      p.Enabled,
		CreatedAt:    p.CreatedAt,
		LastLogin:    p.LastLogin,
	}
}

const userFileVersion = 1

// SetFile 让 Manager 把账号变更同步到 path。传空串表示纯内存（测试用）。
func (m *Manager) SetFile(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.file = path
}

// loadFromFile 读取已存在的账号表并合并进内存。
//
// 种子 admin（来自环境变量）总是存在：如果文件里也有同名账号，以**文件为准**
// ——否则改过的密码重启后会被环境变量的初始密码覆盖回去。
func (m *Manager) loadFromFile(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("rbac: 读取 %s: %w", path, err)
	}
	if len(b) == 0 {
		return nil
	}
	var fd fileData
	if err := json.Unmarshal(b, &fd); err != nil {
		// 文件坏了不要静默当空表——那会让所有非 admin 账号突然登录不上，
		// 而且下一次写盘还会把坏文件覆盖掉，现场就没了。
		return fmt.Errorf("rbac: 解析 %s: %w", path, err)
	}
	for _, pu := range fd.Users {
		if pu.Username == "" {
			continue
		}
		m.users[pu.Username] = pu.toUser()
	}
	return nil
}

// persistLocked 把当前账号表写盘。调用方必须已持写锁。
func (m *Manager) persistLocked() error {
	if m.file == "" {
		return nil
	}
	users := make([]persistedUser, 0, len(m.users))
	for _, u := range m.users {
		users = append(users, toPersisted(u))
	}
	b, err := json.MarshalIndent(fileData{Version: userFileVersion, Users: users}, "", "  ")
	if err != nil {
		return fmt.Errorf("rbac: 序列化账号表: %w", err)
	}
	dir := filepath.Dir(m.file)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("rbac: 建目录 %s: %w", dir, err)
	}
	tmp := m.file + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("rbac: 写临时文件: %w", err)
	}
	if err := os.Rename(tmp, m.file); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rbac: 落盘 %s: %w", m.file, err)
	}
	return nil
}

// 保证并发安全的辅助：所有公开的写操作在改完 map 后调用 m.persistLocked()。
var _ = sync.Mutex{} // 保留 sync 引用，避免文件被裁剪时误删 import
