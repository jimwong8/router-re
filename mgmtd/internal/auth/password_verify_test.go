package auth

import (
	"testing"
)

func TestVerifyPasswordAcceptsOwnHash(t *testing.T) {
	encoded, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if !VerifyPassword(encoded, "correct horse battery staple") {
		t.Fatal("expected own hash to verify")
	}
	if VerifyPassword(encoded, "wrong password") {
		t.Fatal("expected wrong password to fail")
	}
}

func TestVerifyPasswordRejectsWrongAlgorithmAndVersion(t *testing.T) {
	encoded, _ := HashPassword("pw")
	// $argon2i$... (algorithm substitution)
	i := "$argon2i$" + encoded[len("$argon2id$"):]
	if VerifyPassword(i, "pw") {
		t.Fatal("argon2i variant must be rejected")
	}
	// wrong version
	v := "$argon2id$v=18$" + stringsAfter(encoded)
	if VerifyPassword(v, "pw") {
		t.Fatal("wrong version must be rejected")
	}
}

func TestVerifyPasswordRejectsOutOfRangeParams(t *testing.T) {
	encoded, _ := HashPassword("pw")
	// m=1 -> below minMem, must be rejected (would also be a downgrade)
	parts := splitPHC(encoded)
	parts[3] = "m=1,t=3,p=2"
	if VerifyPassword(joinPHC(parts), "pw") {
		t.Fatal("undersized memory must be rejected")
	}
	// p=256 -> wraps to uint8(0) if unchecked
	parts[3] = "m=65536,t=3,p=256"
	if VerifyPassword(joinPHC(parts), "pw") {
		t.Fatal("oversized threads must be rejected")
	}
	// m=4294967295 -> memory-exhaustion DoS if unchecked
	parts[3] = "m=4294967295,t=3,p=2"
	if VerifyPassword(joinPHC(parts), "pw") {
		t.Fatal("oversized memory must be rejected")
	}
}

func splitPHC(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '$' {
			if i > start {
				out = append(out, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

func joinPHC(parts []string) string {
	res := ""
	for i, p := range parts {
		if i > 0 {
			res += "$"
		}
		res += p
	}
	return res
}

func stringsAfter(s string) string {
	i := 0
	dollars := 0
	for ; i < len(s); i++ {
		if s[i] == '$' {
			dollars++
			if dollars == 2 {
				break
			}
		}
	}
	return s[i+1:]
}
