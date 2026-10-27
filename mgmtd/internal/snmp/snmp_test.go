package snmp

import (
	"os"
	"strings"
	"testing"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

type callRecord struct {
	name   string
	args   []string
	stdout string
}

type fakeRunner struct {
	outs  map[string]string // "cmd|arg1|arg2" -> stdout
	errs  map[string]error  // "cmd|arg1|arg2" -> err (checked only when present)
	calls []callRecord
}

func (f *fakeRunner) Run(name string, args ...string) (string, error) {
	key := name + "|" + strings.Join(args, "|")
	f.calls = append(f.calls, callRecord{name: name, args: args, stdout: f.outs[key]})
	if e, ok := f.errs[key]; ok {
		return f.outs[key], e
	}
	return f.outs[key], nil
}

func (f *fakeRunner) RunWithInput(input, name string, args ...string) (string, error) {
	return f.Run(name, args...)
}

func goodCfg() Config {
	return Config{
		Enabled:  true,
		Port:     161,
		Location: "Fangchenggang",
		Contact:  "admin@example.com",
		Communities: []Community{
			{Name: "public", Access: "ro"},
			{Name: "private", Access: "rw"},
		},
		V3Users: []V3User{
			{Name: "admin", AuthProto: "sha", AuthPass: "SuperSecret1", PrivProto: "aes", PrivPass: "TopSecret2"},
		},
		TrapTargets: []string{"192.168.1.1:162", "router.local:162"},
	}
}

// --- Validate 失败分支 ---

func TestValidatePortRange(t *testing.T) {
	c := goodCfg()
	c.Port = 70000
	if err := Validate(c); err == nil || !strings.Contains(err.Error(), "port") {
		t.Fatalf("expected port error, got %v", err)
	}
}

func TestValidatePortZeroDefaults(t *testing.T) {
	c := goodCfg()
	c.Port = 0
	if err := Validate(c); err != nil {
		t.Fatalf("port 0 should default to 161, got %v", err)
	}
}

func TestValidateCommunityNameLength(t *testing.T) {
	c := goodCfg()
	c.Communities = []Community{{Name: "", Access: "ro"}}
	if err := Validate(c); err == nil {
		t.Fatal("expected empty community name error")
	}
}

func TestValidateCommunityAccess(t *testing.T) {
	c := goodCfg()
	c.Communities = []Community{{Name: "public", Access: "xx"}}
	if err := Validate(c); err == nil || !strings.Contains(err.Error(), "access") {
		t.Fatalf("expected access error, got %v", err)
	}
}

func TestValidateV3PassTooShort(t *testing.T) {
	c := goodCfg()
	c.V3Users = []V3User{{Name: "u1", AuthProto: "sha", AuthPass: "12345", PrivProto: "aes", PrivPass: "longenough"}}
	err := Validate(c)
	if err == nil || !strings.Contains(err.Error(), "auth_pass too short") {
		t.Fatalf("expected auth_pass too short, got %v", err)
	}
	// 密码原文绝不能出现在错误信息里
	if strings.Contains(err.Error(), "12345") {
		t.Fatalf("password leaked in error: %v", err)
	}
}

func TestValidateV3PrivPassTooShort(t *testing.T) {
	c := goodCfg()
	c.V3Users = []V3User{{Name: "u1", AuthProto: "sha", AuthPass: "longenough1", PrivProto: "aes", PrivPass: "no"}}
	err := Validate(c)
	if err == nil || !strings.Contains(err.Error(), "priv_pass too short") {
		t.Fatalf("expected priv_pass too short, got %v", err)
	}
	if strings.Contains(err.Error(), "no") {
		t.Fatalf("priv pass leaked in error: %v", err)
	}
}

func TestValidateV3BadProto(t *testing.T) {
	c := goodCfg()
	c.V3Users = []V3User{{Name: "u1", AuthProto: "bogus", AuthPass: "longenough"}}
	if err := Validate(c); err == nil || !strings.Contains(err.Error(), "auth_proto") {
		t.Fatalf("expected auth_proto error, got %v", err)
	}
}

func TestValidateV3PrivBadProto(t *testing.T) {
	c := goodCfg()
	c.V3Users = []V3User{{Name: "u1", AuthProto: "sha", AuthPass: "longenough1", PrivProto: "blowfish", PrivPass: "longenough2"}}
	if err := Validate(c); err == nil || !strings.Contains(err.Error(), "priv_proto") {
		t.Fatalf("expected priv_proto error, got %v", err)
	}
}

func TestValidateEnabledButNoUsers(t *testing.T) {
	c := goodCfg()
	c.Communities = nil
	c.V3Users = nil
	if err := Validate(c); err == nil || !strings.Contains(err.Error(), "at least one") {
		t.Fatalf("expected at-least-one error, got %v", err)
	}
}

func TestValidateBadTrapTarget(t *testing.T) {
	c := goodCfg()
	c.TrapTargets = []string{"not a valid target!!"}
	if err := Validate(c); err == nil || !strings.Contains(err.Error(), "trap_targets") {
		t.Fatalf("expected trap_targets error, got %v", err)
	}
}

func TestValidateGoodConfig(t *testing.T) {
	if err := Validate(goodCfg()); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

// --- RenderSnmpd ---

func TestRenderSnmpdFields(t *testing.T) {
	out := RenderSnmpd(goodCfg())
	for _, want := range []string{
		"agentAddress udp:161",
		"sysLocation Fangchenggang",
		"sysContact admin@example.com",
		"rocommunity public",
		"rwcommunity private",
		"createUser admin sha SuperSecret1 aes TopSecret2",
		"trapsink 192.168.1.1:162",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("render missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "rouser") {
		t.Fatalf("render missing rouser comment:\n%s", out)
	}
	if !strings.Contains(out, "createUser 行含明文密码") {
		t.Fatalf("render missing security comment:\n%s", out)
	}
}

func TestRenderSnmpdDisabled(t *testing.T) {
	c := goodCfg()
	c.Enabled = false
	out := RenderSnmpd(c)
	for _, want := range []string{
		"# agentAddress udp:161",
		"# sysLocation",
		"# rocommunity public",
		"# createUser admin",
		"# trapsink",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("disabled render missing %q:\n%s", want, out)
		}
	}
}

// --- Masked ---

func TestMaskedHidesPasswords(t *testing.T) {
	c := goodCfg()
	m := c.Masked()
	for i := range m.V3Users {
		if m.V3Users[i].AuthPass != "***" || m.V3Users[i].PrivPass != "***" {
			t.Fatalf("password not masked for user %s", m.V3Users[i].Name)
		}
	}
}

func TestMaskedKeepsPlaintextInOriginal(t *testing.T) {
	c := goodCfg()
	_ = c.Masked()
	if c.V3Users[0].AuthPass != "SuperSecret1" {
		t.Fatal("Masked must not mutate the original Config")
	}
}

// --- Apply (FakeRunner) ---

func TestApplyWritesFileAndRestarts(t *testing.T) {
	r := &fakeRunner{outs: map[string]string{"systemctl|restart|snmpd": "ok"}}
	confPath := t.TempDir() + "/snmpd.conf"
	if err := Apply(r, goodCfg(), confPath); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	content, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatalf("config missing: %v", err)
	}
	s := string(content)
	if !strings.Contains(s, "agentAddress udp:161") {
		t.Fatalf("config missing agentAddress:\n%s", s)
	}
	fi, err := os.Stat(confPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Fatalf("expected 0600 perm, got %v", fi.Mode().Perm())
	}
	found := false
	for _, c := range r.calls {
		if c.name == "systemctl" && strings.Join(c.args, " ") == "restart snmpd" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected systemctl restart snmpd call")
	}
}

// --- Status (FakeRunner) ---

func TestStatusRunning(t *testing.T) {
	r := &fakeRunner{outs: map[string]string{"systemctl|is-active|snmpd": "active", "snmpd|-V": "NET-SNMP 5.9"}}
	st, err := Status(r)
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if st["running"] != "true" {
		t.Fatalf("expected running true, got %v", st)
	}
	// version 仅当 snmpd 可执行文件存在时才可断言
	if _, lerr := execx.Lookup("snmpd"); lerr == nil {
		if st["version"] != "NET-SNMP 5.9" {
			t.Fatalf("expected version, got %v", st["version"])
		}
	}
}

// --- QuerySystem ---

func TestQuerySystemMissingSnmpwalk(t *testing.T) {
	// execx.Lookup("snmpwalk") 受环境决定；若未装则必须返回 "snmpwalk not installed"。
	if _, err := execx.Lookup("snmpwalk"); err == nil {
		t.Skip("snmpwalk installed on this host; skipping missing-path assertion")
	}
	_, err := QuerySystem(&fakeRunner{})
	if err == nil {
		t.Fatal("expected error when snmpwalk missing")
	}
	if err.Error() != "snmpwalk not installed" {
		t.Fatalf("expected 'snmpwalk not installed', got %v", err)
	}
}
