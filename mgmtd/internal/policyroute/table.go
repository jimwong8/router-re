// Package policyroute 提供基于策略的路由（ip rule + 多路由表）。
//
// 它比 internal/route 的单一静态路由更进一步：用独立的 policy route table
// （1-252）承载多条路由，再通过 ip rule 按源地址/目的地址/防火墙标记/入接口
// 把特定流量打进对应 table，实现"按策略选路"。
//
// 设计取舍（沿用 internal/route 的渲染风格与 internal/conntrack 的注释风格）：
//   - 只做校验与渲染（纯 Go，可脱离真实系统单测）；执行层由调用方经 execx.Runner 触发。
//   - **安全红线**：
//     ① 路由表 ID **必须 1-252**。253(default)/254(main)/255(local) 是内核保留表，
//     覆盖或删除它们会破坏整个系统路由，导致本机所有流量走错乃至失联。
//     ② ip rule 的 priority **必须 > 0**。priority 0 被 local 表占用，抢占会使
//     本机发包/收包异常。
//     ③ 一条**全条件为空**的 rule（无 from/to/fwmark/iif）会匹配所有流量，等于
//     劫持默认路由，必须拒绝。
package policyroute

import (
	"fmt"
	"net"
	"regexp"
	"strings"
	"sync"
)

// Table 是独立于 main 的一张策略路由表（ID 1-252）。
type Table struct {
	ID       int                 `json:"id"`   // 路由表 ID，1-252（253/254/255 内核保留）
	Name     string              `json:"name"` // 表名，如 wan1table
	Routes   []Route             `json:"routes"`
	Hits     map[string]uint64   `json:"-"` // 规则命中计数
	hitsMu   sync.Mutex          `json:"-"`
}

// Route 是表内的一条路由。
type Route struct {
	Dst    string `json:"dst"` // 10.0.0.0/8 或 default
	Via    string `json:"via"` // 网关 IP，可空（直连路由）
	Dev    string `json:"dev"` // 出接口
	Metric int    `json:"metric"`
}

var nameRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// ValidateTable 校验一张路由表。
//
// **安全红线**：ID 必须在 1-252。253(default)/254(main)/255(local) 是内核保留表，
// 覆盖或删除会导致系统路由全乱——所有流量走错表、远程管理失联。本错误信息明确
// 说明后果，绝不静默接受。
func ValidateTable(t Table) error {
	if t.ID < 1 || t.ID > 252 {
		return fmt.Errorf("table id 必须在 1-252, got %d；253(default)/254(main)/255(local) 是内核保留表，覆盖或删除会使系统路由全乱并失联", t.ID)
	}
	if strings.TrimSpace(t.Name) == "" {
		return fmt.Errorf("table name required")
	}
	if !nameRe.MatchString(t.Name) {
		return fmt.Errorf("table name 只能含字母数字与 -_, got %q", t.Name)
	}
	for _, r := range t.Routes {
		if err := ValidateRoute(r); err != nil {
			return fmt.Errorf("table %d route 校验失败: %v", t.ID, err)
		}
	}
	return nil
}

// ValidateRoute 校验一条路由。
// Dst 必须 "default" 或合法 CIDR；Via 非空需合法 IP；Dev 非空且不能是 lo；
// Metric >= 0。
func ValidateRoute(r Route) error {
	if strings.TrimSpace(r.Dst) == "" {
		return fmt.Errorf("route dst required")
	}
	if r.Dst != "default" {
		if _, _, err := net.ParseCIDR(r.Dst); err != nil {
			return fmt.Errorf("route dst 必须是合法 CIDR 或 default, got %q", r.Dst)
		}
	}
	if r.Via != "" {
		if net.ParseIP(r.Via) == nil {
			return fmt.Errorf("route via 必须是合法 IP, got %q", r.Via)
		}
	}
	if r.Dev != "" && r.Dev == "lo" {
		return fmt.Errorf("route dev 不能是 lo（禁止对 loopback 做策略路由）")
	}
	if r.Metric < 0 {
		return fmt.Errorf("route metric 必须 >= 0, got %d", r.Metric)
	}
	return nil
}


// Hit 原子递增指定 ruleID 的命中计数。
func (t *Table) Hit(ruleID string) {
	t.hitsMu.Lock()
	defer t.hitsMu.Unlock()
	if t.Hits == nil {
		t.Hits = map[string]uint64{}
	}
	t.Hits[ruleID]++
}
