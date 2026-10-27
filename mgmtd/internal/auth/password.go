package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	saltLen = 16
	tCost   = 3
	memory  = 64 * 1024
	threads = 2
	keyLen  = 32
)

// HashPassword hashes pw with argon2id and returns a PHC-encoded string:
// $argon2id$v=19$m=<mem>,t=<time>,p=<threads>$<salt b64>$<hash b64>
func HashPassword(pw string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(pw), salt, tCost, memory, threads, keyLen)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		memory, tCost, threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash)), nil
}

// Bounds applied to attacker-controlled parameters parsed from a PHC string.
// They keep a hostile encoded hash from forcing a giant argon2 allocation
// (DoS) or from wrapping to a tiny cost that collapses verification (downgrade).
const (
	minMem     = 8 * 1024
	maxMem     = 1024 * 1024
	minCost    = 1
	maxCost    = 10
	minThreads = 1
	maxThreads = 8
)

// VerifyPassword checks pw against an argon2id PHC-encoded hash.
func VerifyPassword(encoded, pw string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 {
		return false
	}
	// Reject any algorithm/version other than the exact ones we emit.
	if parts[1] != "argon2id" || parts[2] != "v=19" {
		return false
	}
	var mem, t, p int
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &mem, &t, &p); err != nil {
		return false
	}
	// Clamp to sane ranges AFTER parsing, before any allocation. This both
	// prevents a memory-exhaustion DoS and a uint8/uint conversion wrap.
	if mem < minMem || mem > maxMem || t < minCost || t > maxCost || p < minThreads || p > maxThreads {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) != saltLen {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	// Enforce the canonical key length so a truncated hash cannot be
	// "verified" against a shorter key.
	if len(expected) != keyLen {
		return false
	}
	got := argon2.IDKey([]byte(pw), salt, uint32(t), uint32(mem), uint8(p), uint32(len(expected)))
	return subtle.ConstantTimeCompare(got, expected) == 1
}
