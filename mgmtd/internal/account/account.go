package account

import (
	"fmt"
	"regexp"
	"strings"
)

// Type values supported by an Account.
const (
	TypePPPoE  = "pppoe"
	TypePortal = "portal"
)

// Account is a billing/authentication account for PPPoE or captive-portal users.
type Account struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	Type      string `json:"type"`
	Profile   string `json:"profile"`
	QuotaMB   int    `json:"quota_mb"`
	UsedMB    int    `json:"used_mb"`
	ExpireAt  int64  `json:"expire_at"`
	Enabled   bool   `json:"enabled"`
	CreatedAt int64  `json:"created_at"`
	Online    bool   `json:"online"`
	BoundMAC  string `json:"bound_mac"`
}

var userRe = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

// Validate checks username/password/type/quota constraints.
func Validate(a Account) error {
	if err := ValidateExceptPassword(a); err != nil {
		return err
	}
	if len(a.Password) < 6 || len(a.Password) > 64 {
		return fmt.Errorf("password must be 6-64 characters")
	}
	return nil
}

// ValidateExceptPassword checks every rule except the password length.
// It is used on the update path, where an empty password means the stored
// credential is being preserved rather than replaced.
func ValidateExceptPassword(a Account) error {
	u := strings.TrimSpace(a.Username)
	if len(u) < 3 || len(u) > 32 {
		return fmt.Errorf("username must be 3-32 characters")
	}
	if !userRe.MatchString(u) {
		return fmt.Errorf("username may only contain [a-zA-Z0-9_.-]")
	}
	if a.Type != TypePPPoE && a.Type != TypePortal {
		return fmt.Errorf("type must be pppoe or portal")
	}
	if a.QuotaMB < 0 {
		return fmt.Errorf("quota_mb must be >= 0 (0 means unlimited)")
	}
	return nil
}

// Status returns the account state label.
// Priority: disabled > expired > over_quota > online > ok.
func (a Account) Status(now int64) string {
	if !a.Enabled {
		return "disabled"
	}
	if a.ExpireAt > 0 && now > a.ExpireAt {
		return "expired"
	}
	if a.QuotaMB > 0 && a.UsedMB >= a.QuotaMB {
		return "over_quota"
	}
	if a.Online {
		return "online"
	}
	return "ok"
}
