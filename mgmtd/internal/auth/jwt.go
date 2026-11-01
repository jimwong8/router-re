package auth

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TokenManager issues and verifies HS256 JWT session tokens.
type TokenManager struct {
	secret []byte
	ttl    time.Duration
}

// NewTokenManager creates a TokenManager. secret should be >= 32 random bytes.
// A defensive copy of secret is taken so later mutation of the caller's slice
// cannot invalidate issued tokens or corrupt verification.
func NewTokenManager(secret []byte, ttl time.Duration) *TokenManager {
	cp := make([]byte, len(secret))
	copy(cp, secret)
	return &TokenManager{secret: cp, ttl: ttl}
}

// Issue signs a JWT with subject in "sub" and iat/exp set from ttl.
func (t *TokenManager) Issue(subject string) (string, error) {
	return t.IssueWithVersion(subject, 0)
}

// IssueWithVersion 与 Issue 相同，但把会话代数写进 "ver" claim。
//
// 改密时用户记录的 TokenVersion 会 +1，校验时要求完全相等——这样"登录后
// 立刻改密"这种落在同一秒的场景也能精确踢掉旧 token，不依赖时间精度。
func (t *TokenManager) IssueWithVersion(subject string, ver int64) (string, error) {
	if subject == "" {
		return "", jwt.ErrTokenInvalidSubject
	}
	now := time.Now()
	claims := jwt.MapClaims{
		"sub": subject,
		"iat": now.Unix(),
		"exp": now.Add(t.ttl).Unix(),
		"ver": ver,
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(t.secret)
}

// Verify validates the token's signature and expiry, returning its subject.
func (t *TokenManager) Verify(token string) (string, error) {
	sub, _, err := t.VerifyWithIssuedAt(token)
	return sub, err
}

// VerifyWithIssuedAt 除了 subject 还返回签发时间（unix 秒）。
func (t *TokenManager) VerifyWithIssuedAt(token string) (string, int64, error) {
	sub, iat, _, err := t.VerifyWithVersion(token)
	return sub, iat, err
}

// VerifyWithVersion 返回 subject、签发时间与会话代数（ver claim）。
//
// ver 缺失的老 token 返回 0：用户改过密码（TokenVersion>0）后这些 token
// 会被 sessionValid 拒绝，属于预期的一次性强制重新登录。
func (t *TokenManager) VerifyWithVersion(token string) (string, int64, int64, error) {
	parsed, err := jwt.Parse(token, func(tok *jwt.Token) (interface{}, error) {
		if _, ok := tok.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return t.secret, nil
	})
	if err != nil {
		return "", 0, 0, err
	}
	if !parsed.Valid {
		return "", 0, 0, jwt.ErrTokenInvalidClaims
	}
	sub, err := parsed.Claims.GetSubject()
	if err != nil {
		return "", 0, 0, err
	}
	var ver int64
	if v, ok := parsed.Claims.(jwt.MapClaims)["ver"]; ok {
		switch n := v.(type) {
		case float64:
			ver = int64(n)
		case int64:
			ver = n
		}
	}
	iat, err := parsed.Claims.GetIssuedAt()
	if err != nil || iat == nil {
		// iat 缺失的 token 视作 0：比任何改密时间都早，会被踢。
		return sub, 0, ver, nil
	}
	return sub, iat.Unix(), ver, nil
}
