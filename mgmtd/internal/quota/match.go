package quota

import (
	"net"
	"net/netip"
	"strings"
)

// ConnCtx 是策略测试/匹配时的连接上下文。
type ConnCtx struct {
	SrcIP   string
	SrcMAC  string
	DstIP   string
	DstPort string
	App     string
}

// Match 判断一条策略是否命中给定的连接上下文。按 TargetType 判断：
//   - ip:    源 IP 精确匹配
//   - cidr:  源 IP 是否落在该 CIDR 网段内（用 prefix.Contains，不做字符串比较）
//   - mac:   源 MAC 精确匹配，大小写不敏感
//   - app:   应用名等值匹配
//
// 注意：对 ip/mac 维度，这里同时会考虑目标（dst_ip/dst_port）吗？——本模块语义是
// "对来自该目标维度的流量限速"，因此匹配只看源维度，dst 字段留给调试观察。若后续
// 需要按目的限制，扩展 ConnCtx 即可。
func Match(p Policy, ctx ConnCtx) bool {
	if !p.Enabled {
		return false
	}
	switch p.TargetType {
	case TargetIP:
		return ctx.SrcIP != "" && normalizeIP(ctx.SrcIP) == normalizeIP(strings.TrimSpace(p.Target))
	case TargetCIDR:
		if ctx.SrcIP == "" {
			return false
		}
		prefix, err := netip.ParsePrefix(strings.TrimSpace(p.Target))
		if err != nil {
			return false
		}
		addr, err := netip.ParseAddr(ctx.SrcIP)
		if err != nil {
			return false
		}
		return prefix.Contains(addr)
	case TargetMAC:
		return ctx.SrcMAC != "" && strings.EqualFold(ctx.SrcMAC, strings.TrimSpace(p.Target))
	case TargetApp:
		return ctx.App != "" && ctx.App == p.App
	}
	return false
}

// FirstMatch 返回第一条命中的**启用**策略。列表顺序即优先级：越靠前越先匹配，
// 一旦命中即返回（不做"最精确优先"，那是另一套语义）。全部不命中返回 nil。
func FirstMatch(policies []Policy, ctx ConnCtx) *Policy {
	for i := range policies {
		if Match(policies[i], ctx) {
			return &policies[i]
		}
	}
	return nil
}

// normalizeIP 规整 IP 文本，方便精确比较（容忍 IPv4-mapped IPv6 表示）。
func normalizeIP(s string) string {
	ip := net.ParseIP(strings.TrimSpace(s))
	if ip == nil {
		return strings.TrimSpace(s)
	}
	return ip.String()
}
