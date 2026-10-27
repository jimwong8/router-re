package multidial

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateRejectsLo(t *testing.T) {
	if err := ValidateSession(Session{Name: "wan1", Iface: "lo", Username: "u"}); err == nil {
		t.Fatal("lo must be rejected")
	}
}

func TestValidateRejectsMRU1500(t *testing.T) {
	if err := ValidateSession(Session{Name: "wan1", Iface: "eth1", Username: "u", MRU: 1500}); err == nil {
		t.Fatal("MRU 1500 (>1492) must be rejected")
	}
}

func TestValidateRejectsNameWithSpace(t *testing.T) {
	if err := ValidateSession(Session{Name: "wan 1", Iface: "eth1", Username: "u"}); err == nil {
		t.Fatal("name with space must be rejected")
	}
}

func TestValidateRejectsMetric2000(t *testing.T) {
	if err := ValidateSession(Session{Name: "wan1", Iface: "eth1", Username: "u", Metric: 2000}); err == nil {
		t.Fatal("metric 2000 must be rejected")
	}
}

func TestRenderPeerContainsPlugin(t *testing.T) {
	s := Session{Name: "wan1", Iface: "eth1", Username: "alice", MRU: 1492, LCPEcho: 30}
	out := RenderPeer(s)
	if !strings.Contains(out, "plugin rp-pppoe.so") {
		t.Errorf("peer file must contain plugin rp-pppoe.so:\n%s", out)
	}
	if !strings.Contains(out, `user "alice"`) {
		t.Errorf("peer file must contain user line:\n%s", out)
	}
	if !strings.Contains(out, "mtu 1492") {
		t.Errorf("peer file must contain mtu 1492:\n%s", out)
	}
}

func TestRenderChapSecretFormat(t *testing.T) {
	s := Session{Username: "alice", Password: "secret"}
	out := RenderChapSecret(s)
	if !strings.HasPrefix(out, "alice * secret *") {
		t.Errorf("chap-secrets line wrong: %q", out)
	}
}

func TestLoadSessionsMissingFile(t *testing.T) {
	dir := t.TempDir()
	ss, err := LoadSessions(filepath.Join(dir, "missing.json"))
	if err != nil {
		t.Fatalf("missing file should not error, got: %v", err)
	}
	if len(ss) != 0 {
		t.Fatalf("expected empty list, got %d", len(ss))
	}
}

func TestLoadSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sessions.json")
	ss := []Session{{Name: "wan1", Iface: "eth1", Username: "u", Password: "p"}}
	if err := SaveSessions(p, ss); err != nil {
		t.Fatal(err)
	}
	// permissions must be 0600
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o777 != 0o600 {
		t.Errorf("sessions file permissions should be 0600, got %v", info.Mode())
	}
	got, err := LoadSessions(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "wan1" {
		t.Fatalf("round-trip wrong: %+v", got)
	}
}

func TestParseIfconfig(t *testing.T) {
	// realistic `ip -j addr show ppp0` JSON: one scope=link inet (peer) and
	// one scope=global inet (local).
	jsonStr := `[{"ifname":"ppp0","addr_info":[{"family":"inet","local":"10.44.0.1","scope":"link"},{"family":"inet","local":"10.44.0.2","scope":"global"}]}]`
	st, err := ParseIfconfig([]byte(jsonStr), "ppp0")
	if err != nil {
		t.Fatalf("ParseIfconfig error: %v", err)
	}
	if !st.Up {
		t.Fatal("session should be up")
	}
	if st.LocalIP != "10.44.0.2" {
		t.Errorf("LocalIP = %q, want 10.44.0.2", st.LocalIP)
	}
	if st.PeerIP != "10.44.0.1" {
		t.Errorf("PeerIP = %q, want 10.44.0.1", st.PeerIP)
	}
}

func TestParseIfconfigMissingInterface(t *testing.T) {
	// empty array = interface does not exist
	st, err := ParseIfconfig([]byte("[]"), "ppp0")
	if err != nil {
		t.Fatalf("missing interface should not error, got: %v", err)
	}
	if st.Up {
		t.Fatal("interface missing should be Up=false")
	}
}

func TestParsePppdProcs(t *testing.T) {
	ps := `  PID COMMAND
 1234 /usr/sbin/pppd call wan1
 1235 /usr/sbin/pppd call wan2
 1236 /usr/sbin/pppd call wan1
`
	m := ParsePppdProcs(ps)
	if m["wan1"] != 1236 {
		t.Errorf("wan1 pid = %d, want 1236", m["wan1"])
	}
	if m["wan2"] != 1235 {
		t.Errorf("wan2 pid = %d, want 1235", m["wan2"])
	}
}
