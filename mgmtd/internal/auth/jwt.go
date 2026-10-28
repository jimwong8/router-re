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
	if subject == "" {
		return "", jwt.ErrTokenInvalidSubject
	}
	now := time.Now()
	claims := jwt.MapClaims{
		"sub": subject,
		"iat": now.Unix(),
		"exp": now.Add(t.ttl).Unix(),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(t.secret)
}

// Verify validates the token's signature and expiry, returning its subject.
func (t *TokenManager) Verify(token string) (string, error) {
	sub, _, err := t.VerifyWithIssuedAt(token)
	return sub, err
}

// VerifyWithIssuedAt 除了 subject 还返回签发时间（unix 秒）。
//
// 需要它的唯一原因是「改密踢旧会话」：token 是无状态的，改密本身不会让已签发的
// token 失效，只能靠调用方比对签发时间与用户最后一次改密时间。
func (t *TokenManager) VerifyWithIssuedAt(token string) (string, int64, error) {
	parsed, err := jwt.Parse(token, func(tok *jwt.Token) (interface{}, error) {
		if _, ok := tok.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return t.secret, nil
	})
	if err != nil {
		return "", 0, err
	}
	if !parsed.Valid {
		return "", 0, jwt.ErrTokenInvalidClaims
	}
	sub, err := parsed.Claims.GetSubject()
	if err != nil {
		return "", 0, err
	}
	iat, err := parsed.Claims.GetIssuedAt()
	if err != nil || iat == nil {
		// iat 缺失的 token 视作 0：比任何改密时间都早，会被踢。
		return sub, 0, nil
	}
	return sub, iat.Unix(), nil
}
