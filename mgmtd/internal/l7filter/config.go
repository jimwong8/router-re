package l7filter

import (
	"fmt"
	"strings"
	"unicode"
)

// MatchType 是真正能从流量里拿到的 L7 字段。
//
//   - sni       TLS ClientHello 的 Server Name Indication（加密流量也能读到）
//   - http_host HTTP/1.1 的 Host 头（明文 HTTP）
//   - user_agent HTTP 的 User-Agent 头
//   - dns       DNS 查询的 qname（dnsmasq/dnsproxy 日志或 nft 旁路）
//
// 不做 "ip" / "port" 这类 L3/L4 匹配——那是 firewall/portmap 的职责，
// 混进来会让 L7 规则表失去语义边界。
const (
	MatchSNI       = "sni"
	MatchHTTPHost  = "http_host"
	MatchUserAgent = "user_agent"
	MatchDNS       = "dns"
)

// Action 是白名单化的处置动作。
const (
	ActionBlock = "block"
	ActionAllow = "allow"
	ActionLog   = "log"
)

// Rule 是一条 L7 应用层过滤规则。
type Rule struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	MatchType string `json:"match_type"`
	Pattern   string `json:"pattern"`
	Action    string `json:"action"`
	Enabled   bool   `json:"enabled"`
}

// Request 是一次待判定的 L7 请求上下文。四个字段与四种 MatchType 一一对应；
// 未提供的字段留空字符串，MatchRequest 会跳过对应类型的规则。
type Request struct {
	SNI       string
	HTTPHost  string
	UserAgent string
	DNSName   string
}

// domainLikeTypes 是需要按域名格式校验 Pattern 的类型。
var domainLikeTypes = map[string]bool{
	MatchSNI:      true,
	MatchHTTPHost: true,
	MatchDNS:      true,
}

// Validate 校验单条规则的结构与取值。
func Validate(r Rule) error {
	if strings.TrimSpace(r.ID) == "" {
		return fmt.Errorf("rule id is required")
	}
	switch r.MatchType {
	case MatchSNI, MatchHTTPHost, MatchUserAgent, MatchDNS:
	default:
		return fmt.Errorf("match_type must be one of sni/http_host/user_agent/dns, got %q", r.MatchType)
	}
	switch r.Action {
	case ActionBlock, ActionAllow, ActionLog:
	default:
		return fmt.Errorf("action must be one of block/allow/log, got %q", r.Action)
	}
	p := r.Pattern
	if strings.TrimSpace(p) == "" {
		return fmt.Errorf("pattern is required")
	}
	if len([]rune(p)) > 256 {
		return fmt.Errorf("pattern longer than 256 characters")
	}
	if domainLikeTypes[r.MatchType] {
		if err := ValidateDomainPattern(p); err != nil {
			return fmt.Errorf("invalid %s pattern %q: %v", r.MatchType, p, err)
		}
		return nil
	}
	// user_agent：允许任意可打印字符，但控制字符会破坏日志/配置渲染，必须拒绝。
	for _, c := range p {
		if unicode.IsControl(c) {
			return fmt.Errorf("user_agent pattern must not contain control characters (U+%04X)", c)
		}
	}
	return nil
}

// ValidateDomainPattern 校验域名形态的 Pattern。
//
// 允许：
//   - 普通域名：example.com、a.b.example.com
//   - 通配前缀：*.example.com（仅允许一个前导 "*."）
//
// 拒绝：空格、以 . 或 - 开头/结尾、连续点、空标签、含下划线等非法字符。
// 这是防注入的第一道闸——Pattern 最终会被拼进 dnsmasq 配置和 nft 文本，
// 任何能改变配置语义的字符（空格、换行、分号、/）都必须在这里挡掉。
func ValidateDomainPattern(p string) error {
	s := strings.TrimSpace(p)
	if s == "" {
		return fmt.Errorf("empty")
	}
	// 通配：剥掉前导 "*." 再校验主体
	wildcard := false
	if strings.HasPrefix(s, "*.") {
		wildcard = true
		s = s[2:]
	}
	if s == "" {
		return fmt.Errorf("wildcard must be followed by a domain")
	}
	if strings.Contains(s, "*") {
		return fmt.Errorf("only a leading \"* .\" wildcard is allowed")
	}
	if strings.ContainsAny(s, " \t\n\r/;,\"'`$(){}[]<>|&!#~") {
		return fmt.Errorf("contains illegal characters")
	}
	labels := strings.Split(s, ".")
	if len(labels) < 2 {
		return fmt.Errorf("must contain at least two labels (e.g. example.com)")
	}
	for i, l := range labels {
		if l == "" {
			return fmt.Errorf("empty label (consecutive dots)")
		}
		if strings.HasPrefix(l, "-") || strings.HasSuffix(l, "-") {
			return fmt.Errorf("label %q must not start or end with '-'", l)
		}
		for _, c := range l {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
				return fmt.Errorf("label %q contains illegal character %q", l, c)
			}
		}
		// TLD 不能以数字开头（宽松：仅拒绝全数字 TLD）
		if i == len(labels)-1 && allDigits(l) {
			return fmt.Errorf("tld %q must not be all digits", l)
		}
	}
	// *.example.com → 剥掉 "*." 后剩 example.com（2 个标签），合法；
	// *.com → 只剩 1 个标签，无子域可言，拒绝。
	if wildcard && len(labels) < 2 {
		return fmt.Errorf("wildcard pattern needs a concrete domain (e.g. *.example.com)")
	}
	return nil
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// MatchDomain 判断 host 是否命中 pattern（通配符语义）。
//
// 语义（重点，注释即契约）：
//   - `*.example.com` 匹配 `a.example.com`、`b.c.example.com` 等任意深度的子域，
//     但 **不匹配** `example.com` 本身（裸域名不是自己的子域）。
//   - `example.com` 精确匹配（同时也精确匹配 `example.com.`——尾部点被忽略）。
//   - 匹配前双方都转小写并去掉尾部点。
//   - 其它不存在的子域不匹配（不做后缀包含，那会误伤无关域名）。
func MatchDomain(pattern, host string) bool {
	p := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(pattern), "."))
	h := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if p == "" || h == "" {
		return false
	}
	if strings.HasPrefix(p, "*.") {
		suffix := p[2:]
		// host 必须以 suffix 结尾，且 host 必须比 suffix 多至少一个子域标签。
		if !strings.HasSuffix(h, suffix) {
			return false
		}
		prefix := h[:len(h)-len(suffix)]
		if prefix == "" {
			return false // host == suffix（裸域名不匹配通配）
		}
		// 前缀必须以 "." 结束（否则 aexample.com 会误配 *.example.com）
		return strings.HasSuffix(prefix, ".")
	}
	return p == h
}

// MatchRequest 返回所有命中请求的启用规则。
//
// 注意：这里的"命中"只按对应字段匹配，Action 不做排序——ShouldBlock 才决定
// 最终处置（allow 覆盖 block）。
func MatchRequest(rules []Rule, req Request) []Rule {
	var hit []Rule
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		var candidate string
		switch r.MatchType {
		case MatchSNI:
			candidate = req.SNI
		case MatchHTTPHost:
			candidate = req.HTTPHost
		case MatchUserAgent:
			candidate = req.UserAgent
		case MatchDNS:
			candidate = req.DNSName
		default:
			continue
		}
		if r.MatchType == MatchUserAgent {
			// UA 是子串匹配（不是域名），大小写不敏感。
			if strings.Contains(strings.ToLower(candidate), strings.ToLower(r.Pattern)) {
				hit = append(hit, r)
			}
			continue
		}
		if MatchDomain(r.Pattern, candidate) {
			hit = append(hit, r)
		}
	}
	return hit
}

// ShouldBlock 判断这批命中规则是否应阻断。
//
// 策略顺序（注释即契约）：**allow 覆盖 block** —— 只要有一条 allow 命中，
// 就放行，哪怕同时有 block 命中。这是显式白名单优先的安全策略：
// 运营商/管理员可能想"先放行某服务，再拦其余"，allow 提供了逃生舱。
// 若没有 allow，任何一条 block 命中都会阻断；纯 log 命中不影响放行。
func ShouldBlock(rules []Rule, req Request) (bool, string) {
	hit := MatchRequest(rules, req)
	for _, r := range hit {
		if r.Action == ActionAllow {
			return false, r.Name
		}
	}
	for _, r := range hit {
		if r.Action == ActionBlock {
			return true, r.Name
		}
	}
	return false, ""
}
