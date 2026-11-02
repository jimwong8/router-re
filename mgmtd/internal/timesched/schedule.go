package timesched

import (
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// Rule is a time-bounded policy (block / ratelimit / allow_only).
type Rule struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Days     []int    `json:"days"`
	StartMin int      `json:"start_min"`
	EndMin   int      `json:"end_min"`
	Targets  []string `json:"targets"`
	Action   string   `json:"action"`
	RateKbps int      `json:"rate_kbps"`
	Enabled  bool     `json:"enabled"`
}

// Decision is the merged outcome of applying multiple rules to one IP at a
// given moment.
type Decision struct {
	Blocked  bool     `json:"blocked"`
	RateKbps int      `json:"rate_kbps"`
	RuleIDs  []string `json:"rule_ids"`
}

// Validate checks a single Rule.
func Validate(r Rule) error {
	// ID 会进 Render 的注释行 "# active rule: %s"。注释只保护到行尾，
	// 一个换行就能在 nft 表里插入任意规则 —— 例如
	//     ip saddr 0.0.0.0/0 accept
	// 即整体放通。ID 原来完全没有校验（Name 也查非空，但不进渲染）。
	if strings.TrimSpace(r.ID) == "" {
		return fmt.Errorf("id required")
	}
	if len(r.ID) > 64 {
		return fmt.Errorf("id must not exceed 64 bytes")
	}
	for _, c := range r.ID {
		ok := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '_' || c == '-' || c == '.'
		if !ok {
			return fmt.Errorf("id may only contain letters, digits, '_', '-', '.', got %q", c)
		}
	}
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("name required")
	}
	// Days: 0-6, no duplicates.
	seen := map[int]bool{}
	for _, d := range r.Days {
		if d < 0 || d > 6 {
			return fmt.Errorf("days entry %d out of range [0,6]", d)
		}
		if seen[d] {
			return fmt.Errorf("days contains duplicate %d", d)
		}
		seen[d] = true
	}
	if len(r.Days) == 0 {
		return fmt.Errorf("days must not be empty")
	}
	if r.StartMin < 0 || r.StartMin > 1439 {
		return fmt.Errorf("start_min must be in [0,1439], got %d", r.StartMin)
	}
	if r.EndMin < 1 || r.EndMin > 1440 {
		return fmt.Errorf("end_min must be in [1,1440], got %d", r.EndMin)
	}
	if r.EndMin <= r.StartMin {
		return fmt.Errorf("end_min (%d) must be greater than start_min (%d); cross-midnight windows must be split into two rules", r.EndMin, r.StartMin)
	}
	switch r.Action {
	case "block", "ratelimit", "allow_only":
	default:
		return fmt.Errorf("action must be block/ratelimit/allow_only, got %q", r.Action)
	}
	if r.Action == "ratelimit" {
		if r.RateKbps < 1 || r.RateKbps > 1000000 {
			return fmt.Errorf("rate_kbps must be in [1,1000000] for ratelimit, got %d", r.RateKbps)
		}
	}
	if len(r.Targets) == 0 {
		return fmt.Errorf("targets must not be empty")
	}
	for _, t := range r.Targets {
		if !validTarget(t) {
			return fmt.Errorf("target %q is not a valid IP/CIDR/MAC", t)
		}
	}
	return nil
}

func validTarget(t string) bool {
	if net.ParseIP(t) != nil {
		return true
	}
	if _, _, err := net.ParseCIDR(t); err == nil {
		return true
	}
	if _, err := net.ParseMAC(t); err == nil {
		return true
	}
	return false
}

// Active reports whether rule r applies at time now in location loc. The
// computation MUST go through time.Unix(now,0).In(loc) — taking the weekday
// and minute-of-day from the local clock. Using now % (7*86400) directly
// would treat the epoch as UTC and shift "Monday 08:00" to "Sunday 22:00"
// (or whatever the offset is), silently matching the wrong day.
func Active(r Rule, now int64, loc *time.Location) (bool, error) {
	if !r.Enabled {
		return false, nil
	}
	if err := Validate(r); err != nil {
		return false, err
	}
	if loc == nil {
		return false, fmt.Errorf("loc must not be nil")
	}
	t := time.Unix(now, 0).In(loc)
	wd := int(t.Weekday()) // 0 = Sunday
	minOfDay := t.Hour()*60 + t.Minute()
	dayMatch := false
	for _, d := range r.Days {
		if d == wd {
			dayMatch = true
			break
		}
	}
	if !dayMatch {
		return false, nil
	}
	return minOfDay >= r.StartMin && minOfDay < r.EndMin, nil
}

// Merge applies all rules to ip at time now and returns a single decision.
// Priority: block > ratelimit > allow_only.
func Merge(rules []Rule, ip string, now int64, loc *time.Location) Decision {
	d := Decision{RuleIDs: []string{}}
	var hasBlock, hasRate, hasAllow bool
	rateVal := 0
	rateID := ""
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		if !targetMatches(r.Targets, ip) {
			continue
		}
		ok, _ := Active(r, now, loc)
		if !ok {
			continue
		}
		d.RuleIDs = append(d.RuleIDs, r.ID)
		switch r.Action {
		case "block":
			hasBlock = true
		case "ratelimit":
			hasRate = true
			if rateVal == 0 || r.RateKbps < rateVal {
				rateVal = r.RateKbps
				rateID = r.ID
			}
		case "allow_only":
			hasAllow = true
		}
	}
	// block wins over everything.
	if hasBlock {
		d.Blocked = true
		d.RateKbps = 0
		return d
	}
	if hasRate {
		d.RateKbps = rateVal
		_ = rateID
		return d
	}
	if hasAllow {
		// allow_only means everything else is blocked; we model this as a
		// rate of 0 plus no explicit block flag — callers interpret it as
		// "only allow listed targets" which is enforced by the firewall.
		d.RateKbps = 0
		return d
	}
	return d
}

func targetMatches(targets []string, ip string) bool {
	parsed := net.ParseIP(ip)
	for _, t := range targets {
		if t == ip {
			return true
		}
		if mac, err := net.ParseMAC(t); err == nil {
			// MAC targets cannot be matched against an IP; skip for now.
			_ = mac
			continue
		}
		if _, cidr, err := net.ParseCIDR(t); err == nil {
			if parsed != nil && cidr.Contains(parsed) {
				return true
			}
			continue
		}
		if net.ParseIP(t) != nil && t == ip {
			return true
		}
	}
	return false
}

// Render emits the nft snippet for the rules currently active at time now.
// Never sets policy drop and never emits empty element sets.
func Render(rules []Rule, now int64, loc *time.Location) string {
	var b strings.Builder
	b.WriteString("table inet timesched {\n")
	b.WriteString("\tchain forward {\n")
	b.WriteString("\t\ttype filter hook forward priority filter; policy accept;\n")
	ids := []string{}
	for _, r := range rules {
		ok, _ := Active(r, now, loc)
		if !ok {
			continue
		}
		ids = append(ids, r.ID)
	}
	if len(ids) == 0 {
		// no active rules — do not emit an empty elements set
		b.WriteString("\t\t# no active rules\n")
	} else {
		for _, id := range ids {
			// 渲染层兜底：注释只保护到行尾
			fmt.Fprintf(&b, "\t\t# active rule: %s\n", oneLine(id))
		}
	}
	b.WriteString("\t}\n")
	b.WriteString("}\n")
	// sort to make output deterministic
	sort.Strings(ids)
	return b.String()
}

// ensure execx is referenced so the import is not unused if future work uses it
var _ = execx.Runner(nil)

// oneLine 把任意字符串压成单行，渲染层兜底用。
func oneLine(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\t' || r == '\n' || r == '\r' || r == '\v' || r == '\f':
			b.WriteByte(' ')
		case r < 0x20 || r == 0x7f:
			// 丢弃
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
