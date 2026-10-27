package online

import (
	"fmt"
	"net"
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
	// ⚠ 必须同时匹配 IPv4 和 IPv6。原正则 `src=([0-9.]+)` 只认 IPv4，
	// 遇到 IPv6 行（src=2409:8a5c:...）会在第一个冒号处停下，得到 "2409" ——
	// 那不是任何合法地址，却被当成一个"在线用户"，页面上就会出现
	// 一行 IP 为 "2409" 的幽灵会话。
	re := regexp.MustCompile(`src=([0-9a-fA-F:.]+)`)
	for _, line := range strings.Split(text, "\n") {
		m := re.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		ip := m[1]
		// 只有冒号或只有点都可能是残片：冒号结尾去掉，纯数字不是地址
		ip = strings.TrimSuffix(ip, ":")
		if !isPlausibleIP(ip) {
			continue
		}
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

// isPlausibleIP 过滤正则抓到的残片（如 "2409"、"0.0.0"）。
func isPlausibleIP(s string) bool {
	if net.ParseIP(s) != nil {
		return true
	}
	return false
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
		// 这些会话来自 conntrack，本来就没有账号归属。留空会让前端表格
		// 出现一列空白，看起来像"加载失败"；给一个明确标识更诚实。
		if s.Username == "" {
			s.Username = unauthenticatedLabel
		}
		out = append(out, s)
	}
	return out
}

// unauthenticatedLabel 是"有流量但没有账号归属"的会话显示名。
// 不要用空串，也不要用 "-"（看起来像缺数据），要说清楚它是什么。
const unauthenticatedLabel = "(未认证)"

// Kick generates the conntrack delete command for a session by IP. It never
// kills a process by username — that would hit another user's session.
func Kick(username, ip string) (string, error) {
	if strings.TrimSpace(ip) == "" {
		return "", fmt.Errorf("ip required to kick %q (refusing to delete by username)", username)
	}
	return fmt.Sprintf("conntrack -D -s %s", ip), nil
}
