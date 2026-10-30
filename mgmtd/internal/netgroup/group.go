// Package netgroup 管理可复用的「网络分组对象」：IP 分组 / MAC 分组 / 终端分组。
//
// 爱快一类路由系统里，IP 分组 / MAC 分组 / 终端分组是独立对象，可以在 ACL、
// 分流、限速等多处复用；本包提供对应的纯数据结构与纯函数（校验 / 归一化 /
// 匹配 / nft 渲染），不涉及持久化、handler 与服务调用，便于单测。
package netgroup

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
)

// 分组类型取值。
const (
	KindIP       = "ip"       // IP 分组：单个 IP 或 CIDR
	KindMAC      = "mac"      // MAC 分组：MAC 地址
	KindTerminal = "terminal" // 终端分组：MAC / IP / 主机名
)

const (
	// MaxNameLen 分组名称长度上限（按字符计）。
	MaxNameLen = 64
	// MaxItemsPerGroup 单个分组允许的最大条目数（去重后）。
	MaxItemsPerGroup = 512
	// defaultSetName 名称清洗为空时的兜底 nft 集合名。
	defaultSetName = "rr_set"
	// setNamePrefix 名称以数字开头时补的前缀，保证集合名合法。
	setNamePrefix = "s_"
)

// Group 是可复用的网络分组对象。
type Group struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Kind        string   `json:"kind"` // "ip" | "mac" | "terminal"
	Items       []string `json:"items"`
	Description string   `json:"description"`
	CreatedAt   int64    `json:"created_at"`
}

// ValidateGroup 校验分组对象：名称非空且不超过 MaxNameLen 个字符、Kind 取值
// 合法、条目（去重归一化后）不超过 MaxItemsPerGroup 条且逐条符合 Kind 语义。
// 错误信息均为中文，便于直接回显给管理员。
func ValidateGroup(g Group) error {
	name := strings.TrimSpace(g.Name)
	if name == "" {
		return fmt.Errorf("分组名称不能为空")
	}
	// 按字符（rune）计数，避免中文名称被按字节误判超长。
	n := 0
	for range name {
		n++
	}
	if n > MaxNameLen {
		return fmt.Errorf("分组名称 %q 长度 %d 超过上限 %d", name, n, MaxNameLen)
	}
	switch g.Kind {
	case KindIP, KindMAC, KindTerminal:
	default:
		return fmt.Errorf("非法的分组类型 %q：必须是 ip/mac/terminal 之一", g.Kind)
	}
	items := NormalizeItems(g.Kind, g.Items)
	if len(items) > MaxItemsPerGroup {
		return fmt.Errorf("分组条目数量 %d 超过上限 %d", len(items), MaxItemsPerGroup)
	}
	for _, it := range items {
		if err := validateItem(g.Kind, it); err != nil {
			return err
		}
	}
	return nil
}

// validateItem 按分组类型校验单个条目（假定已 TrimSpace）。
func validateItem(kind, item string) error {
	switch kind {
	case KindIP:
		// 单个 IP 或 CIDR 任一通过即可。
		if net.ParseIP(item) == nil {
			if _, _, err := net.ParseCIDR(item); err != nil {
				return fmt.Errorf("IP 分组条目 %q 不是合法的 IP 或 CIDR", item)
			}
		}
	case KindMAC:
		if _, err := net.ParseMAC(item); err != nil {
			return fmt.Errorf("MAC 分组条目 %q 不是合法的 MAC 地址", item)
		}
	case KindTerminal:
		if !validTerminalItem(item) {
			return fmt.Errorf("终端分组条目 %q 不是合法的 MAC / IP / 主机名", item)
		}
	default:
		return fmt.Errorf("非法的分组类型 %q：必须是 ip/mac/terminal 之一", kind)
	}
	return nil
}

// validTerminalItem 判断终端条目是否为合法的 MAC / IP / 主机名。
// 主机名规则与正则 ^[a-zA-Z0-9._-]{1,64}$ 等价，这里逐字节手工检查：
// 允许的字符全是 ASCII，字节长度即字符长度，无需为此引入 regexp 依赖。
func validTerminalItem(item string) bool {
	if _, err := net.ParseMAC(item); err == nil {
		return true
	}
	if net.ParseIP(item) != nil {
		return true
	}
	if len(item) < 1 || len(item) > 64 {
		return false
	}
	for i := 0; i < len(item); i++ {
		c := item[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9',
			c == '.', c == '_', c == '-':
		default:
			return false
		}
	}
	return true
}

// NormalizeItems 归一化条目列表：
//   - 去首尾空白，空白条目直接丢弃；
//   - kind 为 mac 时统一小写；
//   - 去重，保持首次出现顺序；
//   - CIDR 原样保留（不折叠地址位）。
func NormalizeItems(kind string, items []string) []string {
	out := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, raw := range items {
		it := strings.TrimSpace(raw)
		if it == "" {
			continue
		}
		if kind == KindMAC {
			it = strings.ToLower(it)
		}
		if _, dup := seen[it]; dup {
			continue
		}
		seen[it] = struct{}{}
		out = append(out, it)
	}
	return out
}

// Match 判断 value 是否命中分组：
//   - ip 组：value 需能解析为 IP，逐条目按「单 IP 相等或 CIDR 包含」判断；
//   - mac 组：去空白并转小写后按字符串相等判断；
//   - terminal 组：MAC / IP / 主机名任一相等即命中，主机名不区分大小写。
func Match(g Group, value string) bool {
	value = strings.TrimSpace(value)
	switch g.Kind {
	case KindIP:
		return matchIP(g.Items, value)
	case KindMAC:
		if value == "" {
			return false
		}
		v := strings.ToLower(value)
		for _, it := range NormalizeItems(KindMAC, g.Items) {
			if it == v {
				return true
			}
		}
		return false
	case KindTerminal:
		return matchTerminal(g.Items, value)
	default:
		return false
	}
}

// matchIP 用 netip 判断 IP 是否落在任一条目内（条目可为单 IP 或 CIDR）。
func matchIP(items []string, value string) bool {
	addr, err := netip.ParseAddr(value)
	if err != nil {
		return false
	}
	// 去掉 IPv6 zone 与 v4-mapped 包装，保证与条目在同一地址族下比较。
	addr = addr.Unmap().WithZone("")
	for _, it := range NormalizeItems(KindIP, items) {
		if pfx, err := netip.ParsePrefix(it); err == nil {
			if pfx.Contains(addr) {
				return true
			}
			continue
		}
		if x, err := netip.ParseAddr(it); err == nil && x.Unmap().WithZone("") == addr {
			return true
		}
	}
	return false
}

// matchTerminal 按 MAC / IP / 主机名任一相等判断命中。
func matchTerminal(items []string, value string) bool {
	if value == "" {
		return false
	}
	_, valueMACErr := net.ParseMAC(value)
	valueIsMAC := valueMACErr == nil
	valueAddr, valueIPErr := netip.ParseAddr(value)
	valueIsIP := valueIPErr == nil
	for _, it := range NormalizeItems(KindTerminal, items) {
		if mac, err := net.ParseMAC(it); err == nil {
			// MAC 条目：按 MAC 语义相等（忽略大小写与分隔符写法差异）。
			if valueIsMAC {
				if b, err := net.ParseMAC(value); err == nil && mac.String() == b.String() {
					return true
				}
			}
			continue
		}
		if addr, err := netip.ParseAddr(it); err == nil {
			// IP 条目：按地址语义相等。
			if valueIsIP && addr.Unmap().WithZone("") == valueAddr.Unmap().WithZone("") {
				return true
			}
			continue
		}
		// 主机名条目：不区分大小写。
		if strings.EqualFold(it, value) {
			return true
		}
	}
	return false
}

// SetName 把任意名称洗成合法的 nft 集合名：只保留字母 / 数字 / 下划线，其余
// 字符一律转成下划线；不能以数字开头（必要时补 "s_" 前缀）；清洗后为空时
// 返回兜底名 "rr_set"。
func SetName(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	s := b.String()
	if s == "" {
		return defaultSetName
	}
	if s[0] >= '0' && s[0] <= '9' {
		s = setNamePrefix + s
	}
	return s
}

// GroupID 由 (kind, name) 生成稳定且不撞车的分组 ID。
//
// 为什么不能直接用 SetName：SetName 把非 ASCII 字符逐个替换成下划线，
// 于是"办公网""财务网""仓库网"都会变成 ip____ —— 三个分组抢同一个 ID。
// 调用方虽然能靠列表序号再加后缀去重，但那样 ID 会随列表顺序变化，
// 引用这些分组的 ACL/限速规则就会在下次保存后指向别的分组。
//
// 规则：名字里如果有可用的 ASCII 片段就保持可读（ip_office）；
// 否则用名字的短哈希（ip_3f2a1b9c）——稳定、与顺序无关、不同名字必不同。
func GroupID(kind, name string) string {
	if kind == "" {
		kind = "group"
	}
	// 只有名字里全部字符都能原样保留时，slug 才是无损的、可以当 ID 用。
	// 一旦有字符被替换（中文、空格、短横线、点……），"server farm" 和
	// "server-farm" 都会塌缩成 server_farm，必须改用哈希区分。
	if name == "" || SetName(name) != name {
		sum := sha1.Sum([]byte(name))
		return kind + "_" + hex.EncodeToString(sum[:6])
	}
	return SetName(kind + "_" + name)
}

// RenderNftSet 把分组渲染成 nft 集合定义文本。
//
// 仅支持 ip（type ipv4_addr，附 flags interval 以支持 CIDR 区间匹配）与 mac
// （type ether_addr）两种分组；IPv6 条目与 terminal 分组暂不支持，返回错误。
func RenderNftSet(g Group) (string, error) {
	if g.Kind == KindTerminal {
		return "", errors.New("terminal 分组不能渲染为 nft 集合")
	}
	if err := ValidateGroup(g); err != nil {
		return "", err
	}
	items := NormalizeItems(g.Kind, g.Items)
	// 集合名必须唯一：用 SetName(g.Name) 时"办公网""财务网"都会变成 ___，
	// 两条规则同时加载就会撞名（nft 会直接报 duplicate set）。
	// GroupID 已是合法的标识符（类型前缀 + ASCII 或短哈希），直接用它。
	name := g.ID
	if name == "" {
		name = GroupID(g.Kind, g.Name)
	}
	name = SetName(name)
	switch g.Kind {
	case KindIP:
		for _, it := range items {
			if isIPv6Item(it) {
				return "", fmt.Errorf("IPv6 条目暂不支持渲染为 nft 集合：%q", it)
			}
		}
		return fmt.Sprintf("set %s { type ipv4_addr; flags interval; elements = { %s } }",
			name, strings.Join(items, ", ")), nil
	case KindMAC:
		return fmt.Sprintf("set %s { type ether_addr; elements = { %s } }",
			name, strings.Join(items, ", ")), nil
	default:
		return "", fmt.Errorf("非法的分组类型 %q：必须是 ip/mac/terminal 之一", g.Kind)
	}
}

// isIPv6Item 判断条目（单 IP 或 CIDR）是否属于 IPv6 地址族。
func isIPv6Item(item string) bool {
	if pfx, err := netip.ParsePrefix(item); err == nil {
		return pfx.Addr().Is6()
	}
	if addr, err := netip.ParseAddr(item); err == nil {
		return addr.Is6()
	}
	return false
}
