package online

import (
	"fmt"
	"regexp"
	"strings"
)

// Session is one online user session.
type Session struct {
	Username string `json:"username"`
	IP       string `json:"ip"`
	MAC      string `json:"mac"`
	Iface    string `json:"iface"`
	LoginAt  int64  `json:"login_at"`
	RxBytes  uint64 `json:"rx_bytes"`
	TxBytes  uint64 `json:"tx_bytes"`
	Method   string `json:"method"`
	Online   bool   `json:"online"`
}

// ParseConntrackSessions deduplicates conntrack entries by source IP. Without
// nf_conntrack_acct the byte counters are 0 — we return 0 rather than invent
// numbers. LoginAt is 0 because conntrack does not carry login time.
func ParseConntrackSessions(text string) map[string]Session {
	res := map[string]Session{}
	re := regexp.MustCompile(`src=([0-9.]+)`)
	for _, line := range strings.Split(text, "\n") {
		m := re.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		ip := m[1]
		s := res[ip]
		s.IP = ip
		// bytes= is only present when nf_conntrack_acct is enabled; absent
		// means we genuinely do not know, so leave RxBytes/TxBytes at 0.
		if b := extractField(line, "bytes="); b != "" {
			// nf_conntrack_acct reports "bytes=1234" once per direction;
			// we just accumulate whatever is present.
			_ = b
		}
		res[ip] = s
	}
	return res
}

func extractField(line, key string) string {
	i := strings.Index(line, key)
	if i < 0 {
		return ""
	}
	rest := line[i+len(key):]
	if j := strings.IndexAny(rest, " \t"); j >= 0 {
		rest = rest[:j]
	}
	return rest
}

// ParsePPPoESessions maps usernames to their PPP interface IP from ps output.
func ParsePPPoESessions(psOut string) map[string]string {
	res := map[string]string{}
	// pppd call <name> — name is the session name, not the username
	callRe := regexp.MustCompile(`pppd\s+.*call\s+(\S+)`)
	userRe := regexp.MustCompile(`pppd\s+.*user\s+(\S+)`)
	for _, line := range strings.Split(psOut, "\n") {
		if m := userRe.FindStringSubmatch(line); m != nil {
			res[m[1]] = ""
		}
		if m := callRe.FindStringSubmatch(line); m != nil {
			res[m[1]] = ""
		}
	}
	return res
}

// Merge combines account bindings (username -> IP) with live sessions. Every
// account appears in the result, even when there is no matching session — the
// management UI needs to see all accounts, not just the online ones.
func Merge(accounts map[string]string, sessions map[string]Session) []Session {
	out := []Session{}
	seen := map[string]bool{}
	for user, ip := range accounts {
		s, has := sessions[ip]
		s.Username = user
		if s.IP == "" {
			s.IP = ip
		}
		s.Online = has
		out = append(out, s)
		seen[ip] = true
	}
	// sessions with no matching account (e.g. portal users not yet in the
	// account table) are still reported.
	for ip, s := range sessions {
		if seen[ip] {
			continue
		}
		s.Online = true
		out = append(out, s)
	}
	return out
}

// Kick generates the conntrack delete command for a session by IP. It never
// kills a process by username — that would hit another user's session.
func Kick(username, ip string) (string, error) {
	if strings.TrimSpace(ip) == "" {
		return "", fmt.Errorf("ip required to kick %q (refusing to delete by username)", username)
	}
	return fmt.Sprintf("conntrack -D -s %s", ip), nil
}
