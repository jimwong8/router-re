package l7filter

import (
	"fmt"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// NftTableL7 是 L7 过滤使用的 nftables 表名。
const NftTableL7 = "router_l7"

// RenderDnsmasqBlock 把一组域名渲染成 dnsmasq 屏蔽配置。
//
// 每个域名输出两条 `address=` 规则：一条指向 0.0.0.0（IPv4 黑洞），
// 一条指向 ::（IPv6 黑洞）。这样双栈客户端都拿不到真实地址，等同于
// 在 DNS 层完成域名阻断——这是"域名层面"的 L7 过滤实现方式，不需要把
// 域名解析成 IP（域名往往对应 CDN 的成百上千个动态 IP，逐 IP 封不现实）。
func RenderDnsmasqBlock(domains []string) string {
	var b strings.Builder
	for _, d := range domains {
		d = strings.TrimSuffix(strings.TrimSpace(d), ".")
		if d == "" {
			continue
		}
		fmt.Fprintf(&b, "address=/%s/0.0.0.0\n", d)
		fmt.Fprintf(&b, "address=/%s/::\n", d)
	}
	return b.String()
}

// RenderNftIpBlock 把一组 IP 渲染成 nftables 丢弃规则。
//
// 输出 table inet router_l7：一个 named set blocked_ip，外加一个 forward
// 链把命中 set 的流量 drop。链声明必须多行（chain 名单独一行、type hook
// 单独一行）。
//
// 绝不写 flush ruleset：幂等替换应 delete table 再重建，而不是 flush，
// 避免把同表里其它进程/会话链进的链误删。
func RenderNftIpBlock(ips []string) string {
	var b strings.Builder
	b.WriteString("table inet " + NftTableL7 + " {\n")
	b.WriteString("  set blocked_ip {\n")
	b.WriteString("    type ipv4_addr\n")
	b.WriteString("    flags interval\n")
	b.WriteString("    elements = {")
	parts := make([]string, 0, len(ips))
	for _, ip := range ips {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			continue
		}
		parts = append(parts, ip)
	}
	b.WriteString(strings.Join(parts, ", "))
	b.WriteString("}\n")
	b.WriteString("  }\n")
	b.WriteString("  chain forward {\n")
	b.WriteString("    type filter hook forward priority filter;\n")
	b.WriteString("    ip daddr @blocked_ip drop\n")
	b.WriteString("  }\n")
	b.WriteString("}\n")
	return b.String()
}

// ResolveDomains 用 getent 把一组域名解析成 IP 列表（map 域名→IPs）。
//
// 域名在交给 getent 之前必须先过 ValidateDomainPattern 的格式校验，防止
// shell 注入——虽然 execx 用 argv 传参会规避 shell，但白名单是额外的纵深
// 防御，且能让"非法域名导致的解析失败"变成明确的拒绝而非静默跳过。
// 解析不到的域名跳过（不报错）：域名可能是私网/本地名，getent 查不到，
// 让这些域名保持 DNS 层拦截即可，不必阻塞整批应用。
func ResolveDomains(r execx.Runner, domains []string) map[string][]string {
	res := make(map[string][]string)
	for _, d := range domains {
		d = strings.TrimSuffix(strings.TrimSpace(d), ".")
		if d == "" {
			continue
		}
		if ValidateDomainPattern(d) != nil {
			// 非法域名，防御性拒绝：不解析、不记录。
			continue
		}
		if d == "" {
			continue
		}
		if out, err := r.Run("getent", "hosts", d); err == nil {
			ips := parseGetentIPs(out)
			if len(ips) > 0 {
				res[d] = ips
			}
		}
	}
	return res
}

// parseGetentIPs 提取 getent hosts 输出里每行首个 IPv4 字段。
func parseGetentIPs(out string) []string {
	var ips []string
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		// getent 行形如 "<ip> <name>..."；只关心 IPv4（本功能是 IPv4 集合）。
		ip := fields[0]
		if strings.Contains(ip, ":") {
			continue // IPv6
		}
		ips = append(ips, ip)
	}
	return ips
}
