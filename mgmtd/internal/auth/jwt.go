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
func NewTokenManager(secret []byte, ttl time.Duration) *TokenManager {
	return &TokenManager{secret: secret, ttl: ttl}
}

// Issue signs a JWT with subject in "sub" and iat/exp set from ttl.
func (t *TokenManager) Issue(subject string) (string, error) {
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
	parsed, err := jwt.Parse(token, func(tok *jwt.Token) (interface{}, error) {
		if _, ok := tok.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return t.secret, nil
	})
	if err != nil {
		return "", err
	}
	if !parsed.Valid {
		return "", jwt.ErrTokenInvalidClaims
	}
	sub, err := parsed.Claims.GetSubject()
	if err != nil {
		return "", err
	}
	return sub, nil
}
