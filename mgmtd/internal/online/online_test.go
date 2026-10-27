package online

import (
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
