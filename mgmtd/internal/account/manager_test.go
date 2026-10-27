package account

import (
	"strings"
	"testing"
)

func validAcct() Account {
	return Account{Username: "alice", Password: "secret123", Type: TypePPPoE, Profile: "basic", QuotaMB: 100, Enabled: true}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*Account)
		want string
	}{
		{"ok", func(a *Account) {}, ""},
		{"short user", func(a *Account) { a.Username = "ab" }, "username"},
		{"long user", func(a *Account) { a.Username = strings.Repeat("a", 33) }, "username"},
		{"bad char", func(a *Account) { a.Username = "bad user!" }, "username"},
		{"short pass", func(a *Account) { a.Password = "12345" }, "password"},
		{"long pass", func(a *Account) { a.Password = strings.Repeat("p", 65) }, "password"},
		{"bad type", func(a *Account) { a.Type = "radius" }, "type"},
		{"neg quota", func(a *Account) { a.QuotaMB = -1 }, "quota_mb"},
	}
	for _, c := range cases {
		a := validAcct()
		c.mut(&a)
		err := Validate(a)
		if c.want == "" {
			if err != nil {
				t.Errorf("%s: unexpected error %v", c.name, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want error containing %q, got %v", c.name, c.want, err)
		}
	}
}

func TestStatusPriority(t *testing.T) {
	now := int64(1_000_000)
	// disabled beats expired
	a := validAcct()
	a.Enabled = false
	a.ExpireAt = now - 10
	if got := a.Status(now); got != "disabled" {
		t.Errorf("disabled should win, got %s", got)
	}
	// expired beats over_quota
	a = validAcct()
	a.ExpireAt = now - 10
	a.UsedMB = 500
	if got := a.Status(now); got != "expired" {
		t.Errorf("expired should win over over_quota, got %s", got)
	}
	// over_quota beats online
	a = validAcct()
	a.QuotaMB = 100
	a.UsedMB = 100
	a.Online = true
	if got := a.Status(now); got != "over_quota" {
		t.Errorf("over_quota should win over online, got %s", got)
	}
	// online
	a = validAcct()
	a.Online = true
	if got := a.Status(now); got != "online" {
		t.Errorf("want online, got %s", got)
	}
	// ok
	a = validAcct()
	if got := a.Status(now); got != "ok" {
		t.Errorf("want ok, got %s", got)
	}
	// unlimited quota never over_quota
	a = validAcct()
	a.QuotaMB = 0
	a.UsedMB = 100000
	if got := a.Status(now); got != "ok" {
		t.Errorf("unlimited quota should be ok, got %s", got)
	}
}

func TestManagerCreateDuplicate(t *testing.T) {
	m := NewManager()
	if err := m.Create(validAcct()); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if err := m.Create(validAcct()); err == nil {
		t.Fatal("duplicate create should fail")
	}
	// case-insensitive duplicate
	dup := validAcct()
	dup.Username = "ALICE"
	if err := m.Create(dup); err == nil {
		t.Fatal("case-insensitive duplicate should fail")
	}
}

func TestManagerListSorted(t *testing.T) {
	m := NewManager()
	for _, n := range []string{"charlie", "alice", "Bob"} {
		a := validAcct()
		a.Username = n
		if err := m.Create(a); err != nil {
			t.Fatalf("create %s: %v", n, err)
		}
	}
	got := m.List()
	want := []string{"alice", "Bob", "charlie"}
	if len(got) != len(want) {
		t.Fatalf("want %d accounts, got %d", len(want), len(got))
	}
	for i, w := range want {
		if got[i].Username != w {
			t.Errorf("index %d: want %s got %s", i, w, got[i].Username)
		}
	}
}

func TestManagerResetUsageAndBindMAC(t *testing.T) {
	m := NewManager()
	a := validAcct()
	a.UsedMB = 50
	if err := m.Create(a); err != nil {
		t.Fatal(err)
	}
	if !m.ResetUsage("alice") {
		t.Fatal("reset should find account")
	}
	got, _ := m.Get("alice")
	if got.UsedMB != 0 {
		t.Errorf("used_mb should be 0, got %d", got.UsedMB)
	}
	if m.ResetUsage("nobody") {
		t.Error("reset on missing account should return false")
	}
	if err := m.BindMAC("alice", "aa:bb:cc:dd:ee:ff"); err != nil {
		t.Errorf("bind mac: %v", err)
	}
	got, _ = m.Get("alice")
	if got.BoundMAC != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("bound_mac = %q", got.BoundMAC)
	}
	if err := m.BindMAC("alice", "not-a-mac"); err == nil {
		t.Error("invalid mac should fail")
	}
}

func TestBulkImport(t *testing.T) {
	m := NewManager()
	lines := []string{
		"# comment line",
		"",
		"user1,pass123,pppoe,basic,100,30",
		"user2,pass123,portal,premium,0,0",
		"bad",                 // too few fields
		"x,short,pppoe",       // short password
		"user1,pass123,pppoe", // duplicate
		"user3,pass123,pppoe,basic,notanumber,10", // bad quota
	}
	created, errs := m.BulkImport(lines)
	if created != 2 {
		t.Errorf("created = %d, want 2", created)
	}
	if len(errs) != 4 {
		t.Errorf("errs = %d, want 4: %v", len(errs), errs)
	}
	for _, e := range errs {
		t.Logf("err: %s", e)
	}
	// verify imported account fields
	u, ok := m.Get("user1")
	if !ok {
		t.Fatal("user1 missing")
	}
	if u.QuotaMB != 100 || u.Profile != "basic" || u.Type != TypePPPoE {
		t.Errorf("user1 fields wrong: %+v", u)
	}
	if u.ExpireAt == 0 {
		t.Error("user1 should have an expiry")
	}
	u2, _ := m.Get("user2")
	if u2.ExpireAt != 0 {
		t.Error("user2 should never expire")
	}
}

func TestUpdateKeepsPasswordWhenEmpty(t *testing.T) {
	m := NewManager()
	a := validAcct()
	a.UsedMB = 40
	if err := m.Create(a); err != nil {
		t.Fatal(err)
	}
	// Simulate a reload from the config store: password stripped.
	reloaded := a
	reloaded.Password = ""
	reloaded.QuotaMB = 2048
	if err := m.Update(reloaded); err != nil {
		t.Fatalf("update with empty password should succeed: %v", err)
	}
	got, ok := m.Get("alice")
	if !ok {
		t.Fatal("account missing after update")
	}
	if got.Password != "secret123" {
		t.Errorf("password should be preserved, got %q", got.Password)
	}
	if got.QuotaMB != 2048 {
		t.Errorf("quota should be updated to 2048, got %d", got.QuotaMB)
	}
}

func TestUpdateRejectsMissingAccount(t *testing.T) {
	m := NewManager()
	if err := m.Update(validAcct()); err == nil {
		t.Fatal("update on missing account should fail")
	}
}

func TestUpdateWithNewPassword(t *testing.T) {
	m := NewManager()
	if err := m.Create(validAcct()); err != nil {
		t.Fatal(err)
	}
	changed := validAcct()
	changed.Password = "newsecret456"
	if err := m.Update(changed); err != nil {
		t.Fatalf("update with new password: %v", err)
	}
	got, _ := m.Get("alice")
	if got.Password != "newsecret456" {
		t.Errorf("password not replaced, got %q", got.Password)
	}
}

func TestUpdateRejectsShortPassword(t *testing.T) {
	m := NewManager()
	if err := m.Create(validAcct()); err != nil {
		t.Fatal(err)
	}
	bad := validAcct()
	bad.Password = "12345"
	if err := m.Update(bad); err == nil {
		t.Fatal("short password should be rejected on update")
	}
}

func TestValidateExceptPassword(t *testing.T) {
	// empty password is allowed by ValidateExceptPassword
	a := validAcct()
	a.Password = ""
	if err := ValidateExceptPassword(a); err != nil {
		t.Errorf("empty password should pass ValidateExceptPassword: %v", err)
	}
	// but other rules still apply
	a.Username = "ab"
	if err := ValidateExceptPassword(a); err == nil {
		t.Error("short username should still fail")
	}
	// and Validate still requires the password
	a = validAcct()
	a.Password = ""
	if err := Validate(a); err == nil {
		t.Error("Validate should reject an empty password")
	}
}

func TestManagerReloadRoundTrip(t *testing.T) {
	// Simulate the full persist -> reload cycle used by the server.
	m := NewManager()
	for _, n := range []string{"user1", "user2", "user3"} {
		a := validAcct()
		a.Username = n
		a.Password = "pass" + n + "123"
		if err := m.Create(a); err != nil {
			t.Fatalf("create %s: %v", n, err)
		}
	}
	// persist strips passwords
	stored := make([]Account, 0)
	for _, a := range m.List() {
		a.Password = ""
		stored = append(stored, a)
	}
	// reload into a fresh manager via the load path the server uses
	m2 := NewManager()
	for _, a := range stored {
		if err := m2.CreateReloaded(a); err != nil {
			t.Fatalf("reload create %s: %v (empty password must be allowed on load)", a.Username, err)
		}
	}
	if len(m2.List()) != 3 {
		t.Errorf("reloaded %d accounts, want 3", len(m2.List()))
	}
}
