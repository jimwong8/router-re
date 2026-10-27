package online

import (
	"net"
	"strings"
	"testing"
)

const sampleConntrack = `ipv4 2 tcp 6 33 TIME_WAIT src=10.100.1.18 dst=10.100.1.180 sport=57392 dport=18443 src=10.100.1.180 dst=10.100.1.18 sport=18443 dport=57392 [ASSURED] mark=0 zone=0 use=2
ipv4 2 tcp 6 120 ESTABLISHED src=10.100.1.18 dst=10.100.1.180 sport=57393 dport=18443 src=10.100.1.180 dst=10.100.1.18 sport=18443 dport=57393 [ASSURED] mark=0 zone=0 use=2
ipv4 2 tcp 6 120 ESTABLISHED src=10.100.1.18 dst=10.100.1.180 sport=57394 dport=18443 src=10.100.1.180 dst=10.100.1.18 sport=18443 dport=57394 [ASSURED] mark=0 zone=0 use=2
ipv4 2 tcp 6 120 ESTABLISHED src=10.100.1.19 dst=10.100.1.180 sport=40000 dport=18443 src=10.100.1.180 dst=10.100.1.19 sport=18443 dport=40000 [ASSURED] mark=0 zone=0 use=2
`

func TestParseConntrackDedupByIP(t *testing.T) {
	sessions := ParseConntrackSessions(sampleConntrack)
	// 10.100.1.18 appears 3 times but must produce exactly 1 session
	if len(sessions) != 2 {
		t.Fatalf("expected 2 unique sessions, got %d: %+v", len(sessions), sessions)
	}
	if sessions["10.100.1.18"].IP != "10.100.1.18" {
		t.Errorf("session IP wrong: %+v", sessions["10.100.1.18"])
	}
}

func TestParseConntrackNoBytesIsZero(t *testing.T) {
	sessions := ParseConntrackSessions(sampleConntrack)
	for ip, s := range sessions {
		if s.RxBytes != 0 || s.TxBytes != 0 {
			t.Errorf("IP %s: bytes must be 0 without nf_conntrack_acct, got rx=%d tx=%d", ip, s.RxBytes, s.TxBytes)
		}
	}
}

func TestParseConntrackLoginAtIsZero(t *testing.T) {
	sessions := ParseConntrackSessions(sampleConntrack)
	for ip, s := range sessions {
		if s.LoginAt != 0 {
			t.Errorf("IP %s: LoginAt must be 0 (unknown), got %d", ip, s.LoginAt)
		}
	}
}

func TestParsePPPoESessions(t *testing.T) {
	ps := `  PID COMMAND
 1234 /usr/sbin/pppd call wan1
 1235 /usr/sbin/pppd call wan2
 1236 /usr/sbin/pppd user foo call wan3
`
	res := ParsePPPoESessions(ps)
	if len(res) != 4 {
		t.Fatalf("expected 4 usernames (wan1, wan2, foo, wan3), got %d: %+v", len(res), res)
	}
	for _, want := range []string{"wan1", "wan2", "foo", "wan3"} {
		if _, ok := res[want]; !ok {
			t.Errorf("%s missing: %+v", want, res)
		}
	}
}

func TestMergeOfflineAccountStillPresent(t *testing.T) {
	accounts := map[string]string{
		"alice": "10.100.1.50",
		"bob":   "10.100.1.51",
	}
	sessions := map[string]Session{
		"10.100.1.50": {IP: "10.100.1.50"},
	}
	res := Merge(accounts, sessions)
	if len(res) != 2 {
		t.Fatalf("expected 2 entries (all accounts), got %d: %+v", len(res), res)
	}
	offline := false
	online := false
	for _, s := range res {
		if s.Username == "bob" {
			if s.Online {
				t.Fatal("bob must be offline")
			}
			offline = true
		}
		if s.Username == "alice" {
			if !s.Online {
				t.Fatal("alice must be online")
			}
			online = true
		}
	}
	if !offline || !online {
		t.Fatal("merge must include both online and offline accounts")
	}
}

func TestKickEmptyIPErrors(t *testing.T) {
	_, err := Kick("alice", "")
	if err == nil {
		t.Fatal("Kick with empty IP must error")
	}
	if !strings.Contains(err.Error(), "ip required") {
		t.Errorf("error must mention ip: %v", err)
	}
}

func TestKickGeneratesConntrackDelete(t *testing.T) {
	cmd, err := Kick("alice", "10.100.1.50")
	if err != nil {
		t.Fatal(err)
	}
	if cmd != "conntrack -D -s 10.100.1.50" {
		t.Errorf("kick command = %q", cmd)
	}
}

// TestParseConntrackIPv6 锁死一个真机发现的 bug：
// 原正则 `src=([0-9.]+)` 只匹配 IPv4，遇到 IPv6 行
// （src=2409:8a5c:6419::1）会在第一个冒号处停下，得到 "2409"，
// 于是页面上出现一行 IP 为 "2409" 的幽灵会话。
// 真实行来自 `grep ipv6 /proc/net/nf_conntrack`。
func TestParseConntrackIPv6(t *testing.T) {
	text := "ipv6     10 udp      17 12 src=fe80:0000:0000:0000:020c:42ff:fe53:fd1b " +
		"dst=ff02:0000:0000:0000:0000:0000:0000:0001 sport=5678 dport=5678 [UNREPLIED] mark=0 use=1\n" +
		"ipv4     2 tcp      6 33 TIME_WAIT src=10.100.1.18 dst=10.100.1.180 sport=57392 dport=18443 " +
		"src=10.100.1.180 dst=10.100.1.18 sport=18443 dport=57392 [ASSURED] mark=0 use=2\n"

	got := ParseConntrackSessions(text)
	if _, bad := got["2409"]; bad {
		t.Fatal(`出现了 IP 为 "2409" 的幽灵会话 —— 正则把 IPv6 在冒号处截断了`)
	}
	if _, ok := got["fe80:0000:0000:0000:020c:42ff:fe53:fd1b"]; !ok {
		t.Fatalf("IPv6 地址没有被完整解析出来，实际 keys: %v", keysOf(got))
	}
	if _, ok := got["10.100.1.18"]; !ok {
		t.Fatalf("IPv4 地址丢了，实际 keys: %v", keysOf(got))
	}
	// 每条残留 IP 都必须是合法地址
	for ip := range got {
		if net.ParseIP(ip) == nil {
			t.Fatalf("解析出了非法地址 %q", ip)
		}
	}
}

func keysOf(m map[string]Session) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
