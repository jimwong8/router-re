// Package rbac implements multi-administrator role-based access control.
// It depends only on the standard library.
package rbac

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"hash"
	"strconv"
	"strings"
)

// pbkdf2Params for HashPassword / VerifyPassword.
const (
	pbkdf2Algorithm = "pbkdf2-sha256"
	iterations      = 100000
	saltLen         = 16
	hashLen         = 32
)

func newHMAC(password []byte) func() hash.Hash {
	return func() hash.Hash { return hmac.New(sha256.New, password) }
}

// kdf derives dkLen bytes of key material via PBKDF2-HMAC-SHA256.
func kdf(password, salt []byte, iter, dkLen int) []byte {
	prf := newHMAC(password)
	dk := make([]byte, 0, dkLen)
	for block := 1; len(dk) < dkLen; block++ {
		t := pbkdf2Block(prf, salt, block, iter)
		rem := dkLen - len(dk)
		if rem > len(t) {
			rem = len(t)
		}
		dk = append(dk, t[:rem]...)
	}
	return dk
}

// pbkdf2Block computes one PBKDF2 block (SHA-256) per RFC 8018:
//
//	U_1 = PRF(P, S || INT_32_BE(block))
//	U_j = PRF(P, U_{j-1})   for j = 2..iter
//	T_i = U_1 ^ U_2 ^ ... ^ U_iter
func pbkdf2Block(prf func() hash.Hash, salt []byte, block, iter int) []byte {
	// U_1 = PRF(P, S || INT_32_BE(block))
	u := make([]byte, 0, len(salt)+4)
	u = append(u, salt...)
	u = appendU32(u, block)
	h := prf()
	h.Write(u)
	u = h.Sum(nil)

	acc := make([]byte, len(u))
	copy(acc, u) // T = U_1

	// U_j = PRF(P, U_{j-1}); T ^= U_j for j = 2..iter
	for i := 1; i < iter; i++ {
		h := prf()
		h.Write(u)
		u = h.Sum(nil)
		for j := 0; j < len(acc); j++ {
			acc[j] ^= u[j]
		}
	}
	return acc
}

// appendU32 appends block as a big-endian uint32.
func appendU32(b []byte, n int) []byte {
	return append(b, byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
}

// HashPassword hashes a password with a random 16-byte salt using PBKDF2-HMAC-SHA256.
// Output format: "pbkdf2-sha256$100000$<b64salt>$<b64hash>".
func HashPassword(pw string) (string, error) {
	if pw == "" {
		return "", fmt.Errorf("password required")
	}
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	dk := kdf([]byte(pw), salt, iterations, hashLen)
	return fmt.Sprintf("%s$%d$%s$%s",
		pbkdf2Algorithm,
		iterations,
		base64.StdEncoding.EncodeToString(salt),
		base64.StdEncoding.EncodeToString(dk)), nil
}

// VerifyPassword checks a plaintext password against an encoded hash in
// constant time. It returns false on any malformed input.
func VerifyPassword(pw, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != pbkdf2Algorithm {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter <= 0 {
		return false
	}
	salt, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := base64.StdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	got := kdf([]byte(pw), salt, iter, len(want))
	if len(got) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

// ValidatePassword enforces the password policy: 8-64 chars, at least one
// letter and one digit. Special characters are not required.
func ValidatePassword(pw string) error {
	if len(pw) < 8 || len(pw) > 64 {
		return fmt.Errorf("password must be 8-64 characters")
	}
	var hasLetter, hasDigit bool
	for _, c := range pw {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
			hasLetter = true
		case c >= '0' && c <= '9':
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return fmt.Errorf("password must contain at least one letter and one digit")
	}
	return nil
}
