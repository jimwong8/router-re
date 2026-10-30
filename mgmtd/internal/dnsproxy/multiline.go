package dnsproxy

import (
	"fmt"
	"net/netip"
	"regexp"
	"strings"
)

// 多线路 DNS。
//
// 场景：一条电信、一条联通、一条移动，希望解析走各自的 DNS 递归，
// 或者只让某些域名走某条线路（例如公司内网域名优先走专线 DNS）。
//
// 落地方式是 dnsmasq 的 server 指令：
//
//	server=/corp.example.com/10.0.0.53@pppoe-wan1   # 只这个域名，只走这条线路
//	server=202.96.128.86@pppoe-wan1                 # 其余都被这条线路兜底
//
// @interface 让 dnsmasq 用指定网卡发出查询——这一步才是"按线路"的关键，
// 只写上游 IP 而不绑源接口，内核仍可能按主路由表把包发到另一条线路。

// LineUpstream 描述一条 WAN 线路的 DNS 配置。
type LineUpstream struct {
	Line      string   `json:"line"`      // 线路名，如 wan1/telecom
	Interface string   `json:"interface"` // 物理接口，如 pppoe-wan1；空表示不绑定源接口
	Servers   []string `json:"servers"`   // 该线路 DNS，IP 或 IP:port
	Domains   []string `json:"domains"`   // 空表示作为全局默认上游
	Enabled   bool     `json:"enabled"`
}

// MultiLineConfig 是多线路 DNS 的总配置。
type MultiLineConfig struct {
	Lines       []LineUpstream `json:"lines"`
	DefaultLine string         `json:"default_line"`
	Enabled     bool           `json:"enabled"`
}

// 域名：可选 *. 前缀 + 字母数字点短横线
var mlDomainRe = regexp.MustCompile(`^(\*\.)?[a-zA-Z0-9]([a-zA-Z0-9.-]*[a-zA-Z0-9])?$`)

// ValidateMultiLine 校验多线路 DNS 配置。
func ValidateMultiLine(c MultiLineConfig) error {
	if len(c.Lines) > 8 {
		return fmt.Errorf("线路最多 8 条，当前 %d 条", len(c.Lines))
	}
	seen := map[string]bool{}
	enabled := 0
	names := map[string]bool{}
	for i, l := range c.Lines {
		name := strings.TrimSpace(l.Line)
		if name == "" {
			return fmt.Errorf("第 %d 条线路缺少名称", i+1)
		}
		if len(name) > 32 {
			return fmt.Errorf("线路名 %q 超过 32 个字符", name)
		}
		if seen[name] {
			return fmt.Errorf("线路名重复：%s", name)
		}
		seen[name] = true
		names[name] = true

		if !l.Enabled {
			continue
		}
		enabled++
		if len(l.Servers) == 0 {
			return fmt.Errorf("线路 %s 已启用但没有配置 DNS 上游", name)
		}
		if len(l.Servers) > 4 {
			return fmt.Errorf("线路 %s 的上游最多 4 个", name)
		}
		for _, s := range l.Servers {
			if err := validateUpstreamAddr(s); err != nil {
				return fmt.Errorf("线路 %s 的 DNS 上游 %q 非法：%v", name, s, err)
			}
		}
		if len(l.Domains) > 256 {
			return fmt.Errorf("线路 %s 的域名最多 256 个", name)
		}
		for _, d := range l.Domains {
			if len(d) == 0 || len(d) > 253 {
				return fmt.Errorf("线路 %s 的域名长度非法：%q", name, d)
			}
			if !mlDomainRe.MatchString(d) {
				return fmt.Errorf("线路 %s 的域名非法（只允许字母数字点短横线，可带 *. 前缀）：%q", name, d)
			}
		}
	}
	if c.Enabled && enabled == 0 {
		return fmt.Errorf("已启用多线路 DNS，但没有任何一条启用的线路")
	}
	if c.DefaultLine != "" && !names[c.DefaultLine] {
		return fmt.Errorf("默认线路 %q 不在线路列表中", c.DefaultLine)
	}
	return nil
}

func validateUpstreamAddr(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return fmt.Errorf("空值")
	}
	if _, err := netip.ParseAddr(s); err == nil {
		return nil
	}
	ap, err := netip.ParseAddrPort(s)
	if err != nil {
		return fmt.Errorf("既不是 IP 也不是 IP:端口")
	}
	if ap.Port() == 0 {
		return fmt.Errorf("端口不能为 0")
	}
	return nil
}

// RenderMultiLine 生成 dnsmasq 片段。未启用或没有线路时返回空串。
func RenderMultiLine(c MultiLineConfig) (string, error) {
	if !c.Enabled || len(c.Lines) == 0 {
		return "", nil
	}
	if err := ValidateMultiLine(c); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("# router-re multi-line dns\n")
	for _, l := range c.Lines {
		if !l.Enabled {
			continue
		}
		fmt.Fprintf(&b, "# line %s\n", l.Line)
		for _, s := range l.Servers {
			target := s
			if l.Interface != "" {
				target = s + "@" + l.Interface
			}
			if len(l.Domains) == 0 {
				fmt.Fprintf(&b, "server=%s\n", target)
				continue
			}
			for _, d := range l.Domains {
				fmt.Fprintf(&b, "server=/%s/%s\n", d, target)
			}
		}
	}
	return b.String(), nil
}

// ParseMultiLine 把 RenderMultiLine 的输出解析回来（往返一致）。
// 非法行跳过并返回第一个错误，同时返回已解析部分——配置损坏时
// 至少还能把能认的线路读出来。
func ParseMultiLine(text string) (MultiLineConfig, error) {
	var c MultiLineConfig
	var firstErr error
	cur := -1
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			body := strings.TrimSpace(strings.TrimPrefix(line, "#"))
			if strings.HasPrefix(body, "line ") {
				c.Lines = append(c.Lines, LineUpstream{
					Line:    strings.TrimSpace(strings.TrimPrefix(body, "line ")),
					Enabled: true,
				})
				cur = len(c.Lines) - 1
			}
			continue
		}
		if !strings.HasPrefix(line, "server=") {
			if firstErr == nil {
				firstErr = fmt.Errorf("无法识别的行：%s", line)
			}
			continue
		}
		if cur < 0 {
			// 没有 # line 头的 server 行，归到一个隐式线路里，不丢配置
			c.Lines = append(c.Lines, LineUpstream{Line: "default", Enabled: true})
			cur = len(c.Lines) - 1
		}
		val := strings.TrimPrefix(line, "server=")
		domain, target := parseServerValue(val)
		if i := strings.Index(target, "@"); i >= 0 {
			c.Lines[cur].Interface = target[i+1:]
			target = target[:i]
		}
		if domain != "" {
			c.Lines[cur].Domains = append(c.Lines[cur].Domains, domain)
		}
		if target != "" {
			c.Lines[cur].Servers = append(c.Lines[cur].Servers, target)
		}
	}
	if len(c.Lines) > 0 {
		c.Enabled = true
	}
	// 去重：同一线路同一上游会在每个域名行里重复出现
	for i := range c.Lines {
		c.Lines[i].Servers = dedupeStrings(c.Lines[i].Servers)
	}
	return c, firstErr
}

// parseServerValue 处理 "/domain/server" 与纯 "server" 两种形式。
func parseServerValue(v string) (domain, target string) {
	if !strings.HasPrefix(v, "/") {
		return "", v
	}
	// server=/a.com/1.1.1.1  →  domain=a.com  target=1.1.1.1
	rest := strings.TrimPrefix(v, "/")
	i := strings.LastIndex(rest, "/")
	if i < 0 {
		return "", v
	}
	return rest[:i], rest[i+1:]
}

func dedupeStrings(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
