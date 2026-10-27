// Package quota 提供应用限速与连接配额：把"限速维度（ip/cidr/mac/app）"与
// "动作（速率上限 + 连接数上限 + 每秒新建连接上限）"绑定成一条策略，渲染成
// nftables 规则下发到内核。这是独立于 internal/nac/connlimit.go 的新一层策略。
//
// 设计取舍（沿用 internal/qos / internal/nac 的注释风格）：
//   - 下发执行差异点走 execx.Runner，渲染/校验是纯 Go，可脱离真实系统单测。
//   - **安全红线**：
//     ① 绝不写 `flush ruleset`；
//     ② 绝不 `policy drop`（只 drop 命中策略的流量，默认放行）；
//     ③ **必须拒绝 cidr /0**（对整网段限速等于把管理员自己也限住，是能锁死
//     远程管理的高危操作）。
//   - app 维度：nft 无法直接按应用名匹配，只能由 DPI 模块给流打 mark 后按
//     meta mark 匹配。渲染层会为此生成说明，绝不假装能直接按应用名匹配。
package quota

import (
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"strings"
)

// TargetType 是四种限速维度。
type TargetType string

const (
	TargetIP   TargetType = "ip"
	TargetCIDR TargetType = "cidr"
	TargetMAC  TargetType = "mac"
	TargetApp  TargetType = "app"
)

// Policy 是一条应用限速 / 连接配额策略。
type Policy struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Target         string     `json:"target"`
	TargetType     TargetType `json:"target_type"`
	App            string     `json:"app"`
	RateMbit       float64    `json:"rate_mbit"`
	BurstKB        int        `json:"burst_kb"`
	MaxConns       int        `json:"max_conns"`
	MaxConnsPerSec int        `json:"max_conns_per_sec"`
	Enabled        bool       `json:"enabled"`
}

// validTargetTypes 是 TargetType 白名单。
var validTargetTypes = map[TargetType]bool{
	TargetIP:   true,
	TargetCIDR: true,
	TargetMAC:  true,
	TargetApp:  true,
}

var appNameRe = regexp.MustCompile(`^[A-Za-z0-9_\-]+$`)

// Validate 校验一条策略。
//
// **安全红线**：cidr 的 /0（如 0.0.0.0/0、::/0）**必须拒绝**——对整网段限速等于
// 全局限速，会把路由器自己（及其管理流量）也限进去，远程管理可能因此失联。
func Validate(p Policy) error {
	if strings.TrimSpace(p.ID) == "" {
		return fmt.Errorf("id required")
	}
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("name required")
	}
	if !validTargetTypes[p.TargetType] {
		return fmt.Errorf("target_type 必须是 ip/cidr/mac/app 之一, got %q", p.TargetType)
	}

	// 按维度校验 Target / App。
	switch p.TargetType {
	case TargetIP:
		if net.ParseIP(strings.TrimSpace(p.Target)) == nil {
			return fmt.Errorf("ip target 必须是合法 IP, got %q", p.Target)
		}
	case TargetCIDR:
		prefix, err := netip.ParsePrefix(strings.TrimSpace(p.Target))
		if err != nil {
			return fmt.Errorf("cidr target 必须是合法 CIDR, got %q", p.Target)
		}
		// **红线**：拒绝 /0（IPv4 与 IPv6 都要）。
		if prefix.Bits() == 0 {
			return fmt.Errorf("拒绝 cidr /0 %q：对整网段限速会把路由器自身也限住，导致远程管理失联", p.Target)
		}
	case TargetMAC:
		mac, err := net.ParseMAC(strings.TrimSpace(p.Target))
		if err != nil || len(mac) != 6 {
			return fmt.Errorf("mac target 必须是 6 字节 MAC 地址, got %q", p.Target)
		}
	case TargetApp:
		if !appNameRe.MatchString(p.App) {
			return fmt.Errorf("app target 需要 app 名（仅字母数字与 -_）, got %q", p.App)
		}
	default:
		return fmt.Errorf("target_type %q 不支持", p.TargetType)
	}

	// App 字段只对 app 维度有意义；其它维度非空是用户混淆，报错。
	if p.TargetType != TargetApp && strings.TrimSpace(p.App) != "" {
		return fmt.Errorf("app 字段只对 target_type=app 有意义, 当前 %q 不应携带 app 字段", p.TargetType)
	}

	// 速率：0 表示只限连接数不限速，此时连接数必须 >0。
	if p.RateMbit < 0 || p.RateMbit > 100000 {
		return fmt.Errorf("rate_mbit 必须在 [0,100000], got %v", p.RateMbit)
	}
	if p.BurstKB < 0 || p.BurstKB > 65535 {
		return fmt.Errorf("burst_kb 必须在 [0,65535], got %d", p.BurstKB)
	}
	if p.MaxConns < 0 || p.MaxConns > 100000 {
		return fmt.Errorf("max_conns 必须在 [0,100000], got %d", p.MaxConns)
	}
	if p.MaxConnsPerSec < 0 || p.MaxConnsPerSec > 10000 {
		return fmt.Errorf("max_conns_per_sec 必须在 [0,10000], got %d", p.MaxConnsPerSec)
	}
	if p.RateMbit == 0 && p.MaxConns == 0 && p.MaxConnsPerSec == 0 {
		return fmt.Errorf("空策略（rate=0 且 max_conns=0 且 max_conns_per_sec=0）永远不会生效，是假配置，必须至少设定一项")
	}
	return nil
}
