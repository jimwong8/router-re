package portmap

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// Policy 描述一条策略路由规则（ip rule + 对应 default 路由）。
type Policy struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	SrcCIDR  string `json:"src_cidr"` // 可为空（不限制源）
	DstCIDR  string `json:"dst_cidr"` // 可为空
	Table    int    `json:"table"`    // 路由表号 1-252
	Priority int    `json:"priority"` // 1-32765，不得重复
	Enabled  bool   `json:"enabled"`
}

// ValidatePolicy 校验策略路由规则列表。
func ValidatePolicy(ps []Policy) error {
	prios := make(map[int]bool)
	for _, p := range ps {
		if p.SrcCIDR != "" {
			if _, _, err := net.ParseCIDR(p.SrcCIDR); err != nil {
				return fmt.Errorf("policy %d (name=%q): invalid src_cidr %q: %v", p.ID, p.Name, p.SrcCIDR, err)
			}
		}
		if p.DstCIDR != "" {
			if _, _, err := net.ParseCIDR(p.DstCIDR); err != nil {
				return fmt.Errorf("policy %d (name=%q): invalid dst_cidr %q: %v", p.ID, p.Name, p.DstCIDR, err)
			}
		}
		if p.Table < 1 || p.Table > 252 {
			return fmt.Errorf("policy %d (name=%q): table %d out of range 1-252", p.ID, p.Name, p.Table)
		}
		if p.Priority < 1 || p.Priority > 32765 {
			return fmt.Errorf("policy %d (name=%q): priority %d out of range 1-32765", p.ID, p.Name, p.Priority)
		}
		if prios[p.Priority] {
			return fmt.Errorf("policy %d (name=%q): duplicate priority %d", p.ID, p.Name, p.Priority)
		}
		prios[p.Priority] = true
	}
	return nil
}

// RenderCommands 生成 ip rule add 命令（可选 from/to），若 ifaceByTable 有对应 table
// 则追加 ip route replace default dev <iface> table <table>。
func RenderCommands(ps []Policy, ifaceByTable map[int]string) []string {
	var cmds []string
	for _, p := range ps {
		if !p.Enabled {
			continue
		}
		var parts []string
		parts = append(parts, "ip", "rule", "add")
		if p.SrcCIDR != "" {
			parts = append(parts, "from", p.SrcCIDR)
		}
		if p.DstCIDR != "" {
			parts = append(parts, "to", p.DstCIDR)
		}
		parts = append(parts, "lookup", fmt.Sprintf("%d", p.Table), "priority", fmt.Sprintf("%d", p.Priority))
		cmds = append(cmds, strings.Join(parts, " "))
		if iface, ok := ifaceByTable[p.Table]; ok {
			cmds = append(cmds, fmt.Sprintf("ip route replace default dev %s table %d", iface, p.Table))
		}
	}
	return cmds
}

// ApplyPolicy 逐条执行策略路由命令。错误含 "File exists" 时忽略继续（重复添加同一规则）。
func ApplyPolicy(r execx.Runner, ps []Policy, ifaceByTable map[int]string) error {
	if err := ValidatePolicy(ps); err != nil {
		return err
	}
	for _, cmd := range RenderCommands(ps, ifaceByTable) {
		fields := strings.Fields(cmd)
		if len(fields) == 0 {
			continue
		}
		if _, err := r.Run(fields[0], fields[1:]...); err != nil {
			if strings.Contains(err.Error(), "File exists") {
				continue // 已存在的规则，幂等忽略
			}
			return fmt.Errorf("policy command %q: %w", cmd, err)
		}
	}
	return nil
}

// ListRules 用 `ip -j rule show` 解析当前策略路由规则列表。
func ListRules(r execx.Runner) ([]map[string]string, error) {
	out, err := r.Run("ip", "-j", "rule", "show")
	if err != nil {
		return nil, fmt.Errorf("ip -j rule show: %w", err)
	}
	var raw []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, fmt.Errorf("parse ip -j rule show: %w", err)
	}
	var rules []map[string]string
	for _, item := range raw {
		m := make(map[string]string)
		for k := range item {
			m[k] = fmt.Sprintf("%v", item[k])
		}
		rules = append(rules, m)
	}
	return rules, nil
}
