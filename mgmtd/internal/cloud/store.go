// Package cloud 实现 router-re 的远程管理中心：设备注册、反向隧道、命令下发、审计。
//
// 设计取舍（与爱快那套云管的对比，结论来自 work/bootchain/ikrc_remote_management_center.md）：
//   - 认证：一次性注册令牌换 device_id + secret，secret 只存 sha256(salt||secret)，永不明文回显；
//     爱快是固件里硬编码客户端证书，云凭证可离线还原。
//   - 隧道：设备主动外连（NAT/CGNAT 后也能用），HTTP 流式 NDJSON，不需要 WebSocket 依赖；
//     爱快是 protobuf 套 HTTP 字节，且逐帧"加密"用的是固定 ASCII 密钥。
//   - 权限：设备侧与云端各有一份白名单，默认拒绝，写操作要二次确认；
//     爱快缺的正是设备侧 ACL。
package cloud

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ErrNotFound 表示设备/令牌不存在。
var ErrNotFound = errors.New("not found")

// Device 是一台被管理的路由终端。
type Device struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Model     string            `json:"model"`
	Version   string            `json:"version"`
	Labels    map[string]string `json:"labels,omitempty"`
	CreatedAt int64             `json:"created_at"`
	LastSeen  int64             `json:"last_seen"`
	RemoteIP  string            `json:"remote_ip,omitempty"`
	UptimeSec int64             `json:"uptime_sec,omitempty"`
	Meta      json.RawMessage   `json:"meta,omitempty"`

	// 这两个字段必须落盘（否则重启后设备凭据全失效），所以不能标 json:"-"。
	// 对外响应统一走 PublicView()，在 API 层抹掉，别指望结构体标签同时满足两种用途。
	SecretSalt string `json:"secret_salt"`
	SecretHash string `json:"secret_hash"`

	// Online 由 Hub 在运行时填充，不落盘。
	Online bool `json:"online"`
}

// EnrollToken 是一次性注册令牌。明文只在创建时返回一次。
type EnrollToken struct {
	ID        string `json:"id"`
	Note      string `json:"note"`
	Salt      string `json:"salt"`
	Hash      string `json:"hash"`
	CreatedAt int64  `json:"created_at"`
	ExpiresAt int64  `json:"expires_at"`
	UsedBy    string `json:"used_by,omitempty"`
	UsedAt    int64  `json:"used_at,omitempty"`
}

// AuditEntry 是控制台与服务端共享的审计记录。
type AuditEntry struct {
	TS     int64  `json:"ts"`
	Actor  string `json:"actor"`
	Device string `json:"device,omitempty"`
	Action string `json:"action"`
	Result string `json:"result"`
	Detail string `json:"detail,omitempty"`
}

type state struct {
	Devices       []*Device      `json:"devices"`
	Tokens        []*EnrollToken `json:"tokens"`
	Audit         []AuditEntry   `json:"audit"`
	AdminPassword string         `json:"admin_password,omitempty"`
	// SessionEpoch 是会话代际。改密码时 +1，此前签发的令牌全部失效。
	// 没有它的话，密码泄露后改密码并不能把已泄露的令牌踢下线，要等 12 小时自然过期。
	SessionEpoch int64 `json:"session_epoch,omitempty"`
}

const maxAudit = 5000

// Store 是带落盘的设备/令牌仓库。单进程写，用一把互斥锁串行化。
type Store struct {
	path string
	mu   sync.RWMutex
	st   state
}

// GetAdminPassword 返回存储的管理员密码（空表示未修改，使用配置默认值）。
func (s *Store) GetAdminPassword() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.st.AdminPassword
}

// GetSessionEpoch 返回当前会话代际。
func (s *Store) GetSessionEpoch() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.st.SessionEpoch
}

// BumpSessionEpoch 递增会话代际并落盘，使此前签发的所有会话令牌立即失效。
func (s *Store) BumpSessionEpoch() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.st.SessionEpoch++
	return s.saveLocked()
}

// SetAdminPassword 更新管理员密码并落盘。
//
// 注意：这里不能调 Save()。Save() 内部会重新取 RLock，而本函数已持有写锁，
// sync.RWMutex 不允许写锁未释放时再取读锁 —— 那是一次死锁，且会把之后所有
// 需要读状态的请求（包括登录）一起挂死。所以统一走 saveLocked。
func (s *Store) SetAdminPassword(pw string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.st.AdminPassword = pw
	return s.saveLocked()
}

// saveLocked 在调用方已持有写锁时把状态落盘。
func (s *Store) saveLocked() error {
	b, err := json.MarshalIndent(s.st, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
func OpenStore(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("store path required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("mkdir: %w", err)
	}
	s := &Store{path: path}
	b, err := os.ReadFile(path)
	switch {
	case err == nil:
		if len(b) > 0 {
			if err := json.Unmarshal(b, &s.st); err != nil {
				// 状态文件坏了就别继续：静默从空表开始会让所有设备"失联且无法注册"。
				return nil, fmt.Errorf("状态文件损坏 %s: %w", path, err)
			}
		}
	case os.IsNotExist(err):
		// 首次启动
	default:
		return nil, err
	}
	return s, nil
}

// Save 原子落盘：先写临时文件 fsync，再 rename。避免半截文件把状态写成空白。
func (s *Store) Save() error {
	s.mu.RLock()
	b, err := json.MarshalIndent(s.st, "", "  ")
	s.mu.RUnlock()
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// ---------------------------------------------------------------- 工具函数

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand 不可用: " + err.Error())
	}
	return hex.EncodeToString(b)
}

func hashSecret(salt, secret string) string {
	sum := sha256.Sum256([]byte(salt + ":" + secret))
	return hex.EncodeToString(sum[:])
}

func ctEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// ---------------------------------------------------------------- 令牌

// CreateToken 生成一次性注册令牌，返回明文（只此一次）。
func (s *Store) CreateToken(note string, ttl time.Duration) (EnrollToken, string) {
	raw := base64.RawURLEncoding.EncodeToString(mustRand(24))
	salt := randHex(8)
	tok := EnrollToken{
		ID:        randHex(8),
		Note:      note,
		Salt:      salt,
		Hash:      hashSecret(salt, raw),
		CreatedAt: time.Now().Unix(),
		ExpiresAt: time.Now().Add(ttl).Unix(),
	}
	s.mu.Lock()
	s.st.Tokens = append(s.st.Tokens, &tok)
	s.mu.Unlock()
	return tok, raw
}

func mustRand(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand 不可用: " + err.Error())
	}
	return b
}

// ListTokens 返回令牌列表（不含哈希细节）。
func (s *Store) ListTokens() []EnrollToken {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]EnrollToken, 0, len(s.st.Tokens))
	for _, t := range s.st.Tokens {
		c := *t
		c.Salt, c.Hash = "", ""
		out = append(out, c)
	}
	return out
}

// DeleteToken 删除令牌。
func (s *Store) DeleteToken(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, t := range s.st.Tokens {
		if t.ID == id {
			s.st.Tokens = append(s.st.Tokens[:i], s.st.Tokens[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
}

// ConsumeToken 校验并消费令牌；返回 false 表示令牌无效/过期/已用。
func (s *Store) ConsumeToken(raw, deviceID string) bool {
	if raw == "" {
		return false
	}
	now := time.Now().Unix()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.st.Tokens {
		if t.UsedAt != 0 || t.ExpiresAt < now {
			continue
		}
		if ctEqual(t.Hash, hashSecret(t.Salt, raw)) {
			t.UsedBy = deviceID
			t.UsedAt = now
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- 设备

// RegisterDevice 创建一台设备并返回 secret 明文（只此一次）。
func (s *Store) RegisterDevice(d Device) (Device, string) {
	secret := base64.RawURLEncoding.EncodeToString(mustRand(32))
	salt := randHex(8)
	d.ID = randHex(8)
	d.CreatedAt = time.Now().Unix()
	d.SecretSalt = salt
	d.SecretHash = hashSecret(salt, secret)
	if d.Labels == nil {
		d.Labels = map[string]string{}
	}
	s.mu.Lock()
	s.st.Devices = append(s.st.Devices, &d)
	s.mu.Unlock()
	// 注意：返回的必须是副本。之前这里直接清空 d 的哈希字段，而 d 的地址已经存进了
	// s.st.Devices —— 等于把落盘的凭据一起清掉，设备注册完就再也连不上（测试暴露）。
	ret := d
	ret.SecretSalt, ret.SecretHash = "", ""
	return ret, secret
}

// AuthDevice 校验设备凭据。成功返回设备副本。
func (s *Store) AuthDevice(id, secret string) (Device, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, d := range s.st.Devices {
		if d.ID != id {
			continue
		}
		if ctEqual(d.SecretHash, hashSecret(d.SecretSalt, secret)) {
			c := *d
			c.SecretSalt, c.SecretHash = "", ""
			return c, true
		}
	}
	return Device{}, false
}

// Touch 更新在线信息（心跳/上报）。
func (s *Store) Touch(id string, upd func(*Device)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.st.Devices {
		if d.ID == id {
			d.LastSeen = time.Now().Unix()
			if upd != nil {
				upd(d)
			}
			return
		}
	}
}

// ListDevices 按名称排序返回设备列表。
// ListDevices 返回设备列表副本，并且**必须**清掉密钥的盐与哈希。
//
// 这是对外响应的唯一入口：/cloud/v1/devices 直接把它序列化出去。
// 一旦这里漏出哈希，任何拿到控制台会话的人都能离线爆破设备密钥，
// 然后冒充设备连上云端——注意别为了图方便在别处另起一个"原始"列表。
func (s *Store) ListDevices() []Device {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Device, 0, len(s.st.Devices))
	for _, d := range s.st.Devices {
		c := *d
		c.SecretSalt, c.SecretHash = "", ""
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return out[i].ID < out[j].ID
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// GetDevice 取单台设备。
func (s *Store) GetDevice(id string) (Device, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, d := range s.st.Devices {
		if d.ID == id {
			c := *d
			c.SecretSalt, c.SecretHash = "", ""
			return c, true
		}
	}
	return Device{}, false
}

// DeleteDevice 删除设备（同时断开它的隧道）。
func (s *Store) DeleteDevice(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, d := range s.st.Devices {
		if d.ID == id {
			s.st.Devices = append(s.st.Devices[:i], s.st.Devices[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
}

// ---------------------------------------------------------------- 审计

// AddAudit 追加一条审计（内存环形，落盘由调用方决定）。
func (s *Store) AddAudit(e AuditEntry) {
	if e.TS == 0 {
		e.TS = time.Now().Unix()
	}
	s.mu.Lock()
	s.st.Audit = append(s.st.Audit, e)
	if len(s.st.Audit) > maxAudit {
		s.st.Audit = s.st.Audit[len(s.st.Audit)-maxAudit:]
	}
	s.mu.Unlock()
}

// ListAudit 返回最近 n 条审计（新的在前）。
func (s *Store) ListAudit(n int) []AuditEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if n <= 0 || n > len(s.st.Audit) {
		n = len(s.st.Audit)
	}
	out := make([]AuditEntry, 0, n)
	for i := len(s.st.Audit) - 1; i >= 0 && len(out) < n; i-- {
		out = append(out, s.st.Audit[i])
	}
	return out
}
