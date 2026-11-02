package lb

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// oneLine 把任意字符串压成单行，用于渲染层兜底。
// 注释只保护到行尾 —— Name/Iface 里的换行会让后面的内容变成一条新命令。
func oneLine(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '	' || r == '\n' || r == '\r' || r == '\v' || r == '\f':
			b.WriteByte(' ')
		case r < 0x20 || r == 0x7f:
			// 丢弃控制字符
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ipRuleRe 是 ApplyIpRules 允许执行的唯一命令行形态。
// 执行侧按白名单匹配，而不是「切分后执行 argv[0]」：后者一旦渲染层被绕过，
// 注入行里的 argv[0] 就会被当成程序名直接 exec —— 换行注入等于任意命令执行。
var ipRuleRe = regexp.MustCompile(`^ip rule add fwmark (\d+) lookup (\d+)$`)

// RenderNftMarkSet emits the nft mark table. Never flushes the ruleset and
// never sets a policy drop — marks are just labels consumed by the routing
// table (fwmark → lookup).
func RenderNftMarkSet(marks map[string]int) string {
	var b strings.Builder
	b.WriteString("table inet router_lb_marks {\n")
	b.WriteString("\tchain prerouting {\n")
	b.WriteString("\t\ttype nat hook prerouting priority filter;\n")
	for name, v := range marks {
		// 渲染层兜底：注释只保护到行尾，name 里的换行会变成一条生效的 nft 规则。
		fmt.Fprintf(&b, "		# mark %d = %s\n", v, oneLine(name))
	}
	b.WriteString("\t}\n")
	b.WriteString("}\n")
	return b.String()
}

// RenderIpRuleScript emits `ip rule add fwmark <mark> lookup <table>` lines
// for each member, one per line with a comment.
func RenderIpRuleScript(c Config) string {
	var b strings.Builder
	for _, m := range c.Members {
		mark := memberMark(m.Name)
		// 渲染层兜底：下面每个字段都会进注释行，而 ApplyIpRules 是逐行执行的。
		fmt.Fprintf(&b, "# %s (%s gw %s): fwmark %d -> lookup %d\n",
			oneLine(m.Name), oneLine(m.Iface), oneLine(m.Gateway), mark, mark)
		fmt.Fprintf(&b, "ip rule add fwmark %d lookup %d\n", mark, mark)
	}
	return b.String()
}

// ApplyMarkSet validates and applies the mark table via nft -f -.
func ApplyMarkSet(r execx.Runner, marks map[string]int) error {
	if _, err := r.Run("nft", "delete", "table", "inet", "router_lb_marks"); err != nil {
		_ = err
	}
	nft := RenderNftMarkSet(marks)
	if _, err := r.RunWithInput(nft, "nft", "-f", "-"); err != nil {
		return err
	}
	return nil
}

// ApplyIpRules runs the ip rule script line by line.
//
// 执行侧按白名单匹配命令形态，绝不「切分后执行 argv[0]」：
// 渲染出的文本里有用户可控字段（member name/iface），一旦换行逃出注释，
// 切分执行就等于把注入行的第一个词当程序名 exec（root 下即任意命令执行）。
// 只要命令行不匹配 ip rule add fwmark <n> lookup <n>，就拒绝执行并报错。
func ApplyIpRules(r execx.Runner, c Config) error {
	script := RenderIpRuleScript(c)
	for _, line := range strings.Split(script, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m := ipRuleRe.FindStringSubmatch(line)
		if m == nil {
			return fmt.Errorf("refusing to execute unexpected ip rule line %q", line)
		}
		mark, err := strconv.Atoi(m[1])
		if err != nil {
			return fmt.Errorf("invalid fwmark in %q: %w", line, err)
		}
		table, err := strconv.Atoi(m[2])
		if err != nil {
			return fmt.Errorf("invalid lookup table in %q: %w", line, err)
		}
		if _, err := r.Run("ip", "rule", "add", "fwmark", strconv.Itoa(mark), "lookup", strconv.Itoa(table)); err != nil {
			return fmt.Errorf("ip rule: %v", err)
		}
	}
	return nil
}

// memberMark maps a member name to a deterministic mark value (1..255).
func memberMark(name string) int {
	s := 0
	for _, ch := range name {
		s = (s*31 + int(ch)) % 255
	}
	if s < 1 {
		s = 1
	}
	return s
}

// MemberMark is the exported form of memberMark.
var MemberMark = memberMark
