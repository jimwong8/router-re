package rbac

import (
	"crypto/pbkdf2"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"hash"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestHashVerifyRoundTrip(t *testing.T) {
	enc, err := HashPassword("S3cretPass!")
	if err != nil {
		t.Fatalf("hash error: %v", err)
	}
	if !strings.HasPrefix(enc, "pbkdf2-sha256$") {
		t.Fatalf("bad prefix: %s", enc)
	}
	if !VerifyPassword("S3cretPass!", enc) {
		t.Fatal("expected verify to pass")
	}
	if VerifyPassword("wrong-password", enc) {
		t.Fatal("expected verify to fail on wrong password")
	}
}

func TestVerifyRejectsMalformed(t *testing.T) {
	bad := []string{"", "nope", "pbkdf2-sha256$abc$bad$bad", "md5$100000$salt$hash"}
	for _, b := range bad {
		if VerifyPassword("x", b) {
			t.Fatalf("expected false for malformed %q", b)
		}
	}
}

func TestHashIsSaltedRandom(t *testing.T) {
	a, _ := HashPassword("SamePass123")
	b, _ := HashPassword("SamePass123")
	if a == b {
		t.Fatal("two hashes of same password must differ (salt)")
	}
}

// TestPBKDF2CrossCheck confirms the manual implementation matches the stdlib.
func TestPBKDF2CrossCheck(t *testing.T) {
	pw := "cross-check-7"
	salt := []byte("salt-salt-salt")
	mine := kdf([]byte(pw), salt, iterations, hashLen)
	std, serr := pbkdf2.Key(sha256.New, pw, salt, iterations, hashLen)
	if serr != nil {
		t.Fatalf("pbkdf2.Key failed: %v", serr)
	}
	if !bytesEqual(mine, std) {
		t.Fatalf("manual kdf != crypto/pbkdf2:\n got %v\n std %v", mine, std)
	}
}

func sha256Hash() hash.Hash { return sha256.New() } // helper for crypto/pbkdf2
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestHashFormatRoundTrip(t *testing.T) {
	enc, _ := HashPassword("FormatCheck1")
	parts := strings.Split(enc, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" || parts[1] != "100000" {
		t.Fatalf("unexpected format: %s", enc)
	}
	if _, err := base64.StdEncoding.DecodeString(parts[2]); err != nil {
		t.Fatalf("salt not valid b64: %v", err)
	}
	if _, err := base64.StdEncoding.DecodeString(parts[3]); err != nil {
		t.Fatalf("hash not valid b64: %v", err)
	}
}

func TestValidatePasswordPolicy(t *testing.T) {
	cases := []struct {
		pw string
		ok bool
	}{
		{"short1", false},                 // too short
		{"allletters", false},             // no digit
		{"12345678", false},               // no letter
		{"goodpass1", true},               // 8+ with letter+digit
		{strings.Repeat("a1", 33), false}, // >64
		{"Abcdef1234567890", true},
	}
	for _, c := range cases {
		err := ValidatePassword(c.pw)
		if c.ok && err != nil {
			t.Errorf("expected ok for %q, got %v", c.pw, err)
		}
		if !c.ok && err == nil {
			t.Errorf("expected error for %q", c.pw)
		}
	}
}

func TestValidateUser(t *testing.T) {
	cases := []struct {
		u  User
		ok bool
	}{
		{User{Username: "ab", Role: RoleAdmin}, false}, // too short
		{User{Username: "good.user-1_name", Role: RoleAdmin}, true},
		{User{Username: "bad name!", Role: RoleAdmin}, false}, // invalid chars
		{User{Username: "alice", Role: Role("root")}, false},  // bad role
		{User{Username: "alice", Role: RoleOperator}, true},
		{User{Username: "alice", Role: RoleViewer}, true},
	}
	for _, c := range cases {
		err := ValidateUser(c.u)
		if c.ok && err != nil {
			t.Errorf("expected ok for %+v, got %v", c.u, err)
		}
		if !c.ok && err == nil {
			t.Errorf("expected error for %+v", c.u)
		}
	}
}

func TestPasswordHashNeverSerialized(t *testing.T) {
	enc, _ := HashPassword("SeriCheck1")
	u := User{Username: "bob", PasswordHash: enc, Role: RoleViewer, Enabled: true}
	b, err := json.Marshal(u)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), enc) {
		t.Fatal("PasswordHash leaked into JSON")
	}
	if regexp.MustCompile(`"password_hash"`).Match(b) {
		t.Fatal("password_hash key present in JSON")
	}
}

func TestCanMatrix(t *testing.T) {
	cases := []struct {
		role   Role
		method string
		path   string
		want   bool
	}{
		{RoleAdmin, "POST", "/api/v1/users", true},
		{RoleAdmin, "DELETE", "/api/v1/users/alice", true},
		{RoleAdmin, "GET", "/api/v1/acl/rules", true},
		{RoleOperator, "POST", "/api/v1/users", false},
		{RoleOperator, "DELETE", "/api/v1/users/alice", false},
		{RoleOperator, "POST", "/api/v1/acl/rules", true},
		{RoleOperator, "GET", "/api/v1/users", true},
		{RoleViewer, "GET", "/api/v1/users", true},
		{RoleViewer, "POST", "/api/v1/acl/rules", false},
		{RoleViewer, "DELETE", "/api/v1/stats/current", false},
		{Role("nobody"), "GET", "/", false},
	}
	for _, c := range cases {
		got := Can(c.role, c.method, c.path)
		if got != c.want {
			t.Errorf("Can(%s,%s,%s)=%v want %v", c.role, c.method, c.path, got, c.want)
		}
	}
}

func TestManagerSeedAdmin(t *testing.T) {
	m := NewManager("admin", "admin123")
	u, ok := m.Get("admin")
	if !ok {
		t.Fatal("default admin must exist")
	}
	if u.Role != RoleAdmin || !u.Enabled || u.PasswordHash == "" {
		t.Fatalf("bad seed admin: %+v", u)
	}
	if _, ok := m.Authenticate("admin", "admin123"); !ok {
		t.Fatal("seeded admin must authenticate with seed password")
	}
	if _, ok := m.Authenticate("admin", "wrong"); ok {
		t.Fatal("wrong password must fail")
	}
}

func TestManagerCreateDuplicate(t *testing.T) {
	m := NewManager("admin", "admin123")
	h, _ := HashPassword("OpPass123")
	if err := m.Create(User{Username: "admin", PasswordHash: h, Role: RoleOperator}); err == nil {
		t.Fatal("expected duplicate error")
	}
	if err := m.Create(User{Username: "op1", PasswordHash: h, Role: RoleOperator}); err != nil {
		t.Fatalf("create ok failed: %v", err)
	}
}

func TestManagerDeleteLastAdmin(t *testing.T) {
	m := NewManager("theadmin", "admin123")
	if err := m.Delete("theadmin"); err == nil {
		t.Fatal("expected reject deleting last admin")
	}
}

func TestManagerListSorted(t *testing.T) {
	m := NewManager("admin", "admin123")
	h, _ := HashPassword("OpPass123")
	for _, n := range []string{"zoe", "alice", "mike"} {
		_ = m.Create(User{Username: n, PasswordHash: h, Role: RoleOperator})
	}
	list := m.List()
	names := make([]string, len(list))
	for i, u := range list {
		names[i] = u.Username
	}
	if !sort.StringsAreSorted(names) {
		t.Fatalf("list not sorted: %v", names)
	}
	if len(list) != 4 {
		t.Fatalf("expected 4 users, got %d", len(list))
	}
}

func TestManagerChangePassword(t *testing.T) {
	m := NewManager("admin", "admin123")
	if err := m.ChangePassword("admin", "short"); err == nil {
		t.Fatal("expected validation error for weak password")
	}
	if err := m.ChangePassword("admin", "NewStronger1"); err != nil {
		t.Fatalf("change failed: %v", err)
	}
	if _, ok := m.Authenticate("admin", "NewStronger1"); !ok {
		t.Fatal("new password must verify")
	}
	if _, ok := m.Authenticate("admin", "admin123"); ok {
		t.Fatal("old password must no longer work")
	}
	if err := m.ChangePassword("nobody", "NewStronger1"); err == nil {
		t.Fatal("expected error for unknown user")
	}
}

func TestManagerUpdate(t *testing.T) {
	m := NewManager("admin", "admin123")
	h, _ := HashPassword("OpPass123")
	_ = m.Create(User{Username: "op1", PasswordHash: h, Role: RoleOperator})
	u, _ := m.Get("op1")
	u.Role = RoleViewer
	u.Enabled = false
	if err := m.Update(u); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	got, _ := m.Get("op1")
	if got.Role != RoleViewer || got.Enabled {
		t.Fatalf("update did not persist: %+v", got)
	}
}

func TestKDFDeterministicWithSalt(t *testing.T) {
	// sanity: same inputs => same output (no randomness inside kdf itself)
	a := kdf([]byte("pw"), []byte("salt"), 1000, 32)
	b := kdf([]byte("pw"), []byte("salt"), 1000, 32)
	if !bytesEqual(a, b) {
		t.Fatal("kdf must be deterministic for fixed salt")
	}
}
