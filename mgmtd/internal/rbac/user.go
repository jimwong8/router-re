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

// User is a persisted administrator account. PasswordHash is never serialized.
type User struct {
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	Role         Role   `json:"role"`
	Enabled      bool   `json:"enabled"`
	CreatedAt    int64  `json:"created_at"`
	LastLogin    int64  `json:"last_login"`
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
