// Package porttrigger 提供端口触发（Port Triggering）配置：内网某端口有
// 出站流量时，临时打开对应的外网端口映射，超时后自动关闭。同时也提供
// ALG（应用层网关）helper 加载与 mDNS 中继配置渲染。
//
// 设计取舍（沿用 internal/nat 的 nft 渲染风格与 internal/conntrack 的注释风格）：
//   - 只做校验与渲染（纯 Go，可脱离真实系统单测）；执行层由调用方负责。
//   - **安全铁律**：
//     ① **绝不写 `policy drop`** —— 默认拒绝属于 firewall 模块的职责，本模块
//     只负责触发开洞，写 drop 会越权把默认策略改成拒绝。
//     ② **空集合绝不写 `elements = {}`** —— 这是 nft 语法错误，要么不写该
//     集合声明，要么写至少一个元素。
package porttrigger

import (
	"fmt"
	"strings"
)

// Trigger 是端口触发规则：内网某端口有出站流量时，临时打开外网端口映射。
type Trigger struct {
	Name        string `json:"name"`
	TriggerPort int    `json:"trigger_port"` // 内网触发的起始端口
	TriggerEnd  int    `json:"trigger_end"`  // 0 表示单端口；否则区间
	OpenPort    int    `json:"open_port"`    // 要打开的外网端口
	OpenEnd     int    `json:"open_end"`
	Protocol    string `json:"protocol"`    // tcp / udp / both
	TimeoutSec  int    `json:"timeout_sec"` // 触发后保持多久，60-86400
	Enabled     bool   `json:"enabled"`
}

// Validate 校验一条触发规则。
// TriggerPort 与 OpenPort 不能相同——二者相同没有意义，且容易让人误以为
// 规则已生效（其实只是把同一端口原样放行）。
func Validate(t Trigger) error {
	if strings.TrimSpace(t.Name) == "" {
		return fmt.Errorf("name required")
	}
	if t.TriggerPort < 1 || t.TriggerPort > 65535 {
		return fmt.Errorf("trigger_port 必须在 1-65535, got %d", t.TriggerPort)
	}
	if t.TriggerEnd != 0 && t.TriggerEnd < t.TriggerPort {
		return fmt.Errorf("trigger_end 必须为 0 或 >= trigger_port, got %d", t.TriggerEnd)
	}
	if t.TriggerEnd > 65535 {
		return fmt.Errorf("trigger_end 不能在 65535 之上, got %d", t.TriggerEnd)
	}
	if t.OpenPort < 1 || t.OpenPort > 65535 {
		return fmt.Errorf("open_port 必须在 1-65535, got %d", t.OpenPort)
	}
	if t.OpenEnd != 0 && t.OpenEnd < t.OpenPort {
		return fmt.Errorf("open_end 必须为 0 或 >= open_port, got %d", t.OpenEnd)
	}
	switch t.Protocol {
	case "tcp", "udp", "both":
	default:
		return fmt.Errorf("protocol 必须是 tcp/udp/both, got %q", t.Protocol)
	}
	if t.TimeoutSec < 60 || t.TimeoutSec > 86400 {
		return fmt.Errorf("timeout_sec 必须在 60-86400, got %d", t.TimeoutSec)
	}
	if t.TriggerPort == t.OpenPort {
		return fmt.Errorf("trigger_port 与 open_port 不能相同（都 %d）：无意义且易误以为已生效", t.TriggerPort)
	}
	return nil
}

// RenderTable 生成 nft 的 `inet router` 表内容，用于端口触发。
//
// 铁律：
//   - 绝不 `policy drop`；链默认 accept。
//   - 空集合绝不写 `elements = {}`（语法错误）。只有存在启用规则时才声明集合，
//     并写入实际元素。
//   - 禁用规则不生成。全部禁用时返回空字符串（不生成空表）。
func RenderTable(t []Trigger) string {
	var enabled []Trigger
	for _, tr := range t {
		if tr.Enabled {
			enabled = append(enabled, tr)
		}
	}
	if len(enabled) == 0 {
		return ""
	}

	// 动态集合：按 (ip saddr, 触发端口) 记录，超时自动过期。
	var b strings.Builder
	b.WriteString("table inet router {\n")
	b.WriteString("  set @triggered {\n")
	b.WriteString("    type ipv4_addr . inet_service\n")
	b.WriteString("    timeout 60s\n")
	b.WriteString("  }\n")
	b.WriteString("  chain trigger {\n")
	b.WriteString("    type filter hook prerouting priority filter; policy accept;\n")
	for _, tr := range enabled {
		tmo := fmt.Sprintf("%ds", tr.TimeoutSec)
		proto := "tcp"
		if tr.Protocol == "udp" {
			proto = "udp"
		}
		// both 时拆成 tcp 与 udp 两条。
		protos := []string{proto}
		if tr.Protocol == "both" {
			protos = []string{"tcp", "udp"}
		}
		for _, p := range protos {
			fmt.Fprintf(&b, "    %s dport %d ct state new add @triggered { ip saddr . %s dport timeout %s }\n",
				p, tr.TriggerPort, p, tmo)
		}
	}
	b.WriteString("  }\n")
	b.WriteString("}\n")
	return b.String()
}
