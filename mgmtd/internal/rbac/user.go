package rbac

import (
	"fmt"
	"regexp"
	"strings"
)

// Role is the administrative role granted to a user.
type Role string

const (
	RoleAdmin    Role = "admin"
	RoleOperator Role = "operator"
	RoleViewer   Role = "viewer"
)

var validRoles = map[Role]bool{RoleAdmin: true, RoleOperator: true, RoleViewer: true}

// ValidRole reports whether role is one of admin/operator/viewer.
func ValidRole(r Role) bool { return validRoles[r] }

// User is a persisted administrator account. PasswordHash is never serialized.
type User struct {
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	Role         Role   `json:"role"`
	Enabled      bool   `json:"enabled"`
	CreatedAt    int64  `json:"created_at"`
	LastLogin    int64  `json:"last_login"`
	// PasswordChangedAt 是最后一次改密的时间（unix 秒）。签发早于它的 token 一律失效，
	// 否则密码泄漏后改密不踢人，攻击者手上那 15 分钟照样能用。
	PasswordChangedAt int64 `json:"password_changed_at,omitempty"`
	// TokenVersion 是会话代数：每次改密 +1，token 里带上签发时的代数，
	// 校验时要求完全相等。它解决 PasswordChangedAt 的秒级歧义——
	// "登录后立刻改密"落在同一秒时，iat >= PasswordChangedAt 会让旧 token
	// 继续有效，而这恰好是密码泄漏后最常见也最需要踢人的场景。
	TokenVersion int64 `json:"token_version,omitempty"`
}

var usernameRe = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

// ValidateUser checks structural rules for a user record.
func ValidateUser(u User) error {
	uname := strings.TrimSpace(u.Username)
	if len(uname) < 3 || len(uname) > 32 {
		return fmt.Errorf("username must be 3-32 characters")
	}
	if !usernameRe.MatchString(uname) {
		return fmt.Errorf("username contains invalid characters (allowed: a-z A-Z 0-9 _ . -)")
	}
	if !validRoles[u.Role] {
		return fmt.Errorf("invalid role %q (must be admin/operator/viewer)", u.Role)
	}
	return nil
}
