// Package alerting 提供日志告警规则：规则匹配、cooldown 抑制与导出。
package alerting

import (
	"fmt"
	"regexp"
	"strings"
)

// Rule 是一条日志告警规则。
type Rule struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Pattern  string   `json:"pattern"` // 关键字或正则
	IsRegex  bool     `json:"is_regex"`
	Units    []string `json:"units"`    // 限定 systemd unit，空=全部
	Severity string   `json:"severity"` // info/warning/critical
	Cooldown int      `json:"cooldown"` // 秒，同规则最小告警间隔
	Enabled  bool     `json:"enabled"`
}

var validSeverities = map[string]bool{"info": true, "warning": true, "critical": true}

// Validate 检查规则字段。
//
// ⚠ Pattern 绝不能被用于 shell 拼接：这里不限制 Pattern 的字符集
// （正则本身就含特殊字符），但调用方/数据面必须把它当作纯匹配字符串，
// 绝不能拼进 systemctl/journalctl/任何 shell 命令。如需执行命令，
// 必须走 execx.Runner 的 argv 形式，或把 Pattern 作为 stdin。
func Validate(r Rule) error {
	if strings.TrimSpace(r.ID) == "" {
		return fmt.Errorf("id required")
	}
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("name required")
	}
	if strings.TrimSpace(r.Pattern) == "" {
		return fmt.Errorf("pattern required")
	}
	if len(r.Pattern) > 512 {
		return fmt.Errorf("pattern must be <= 512 chars, got %d", len(r.Pattern))
	}
	if r.IsRegex {
		// 编译失败要把正则原文带出来，方便定位。
		if _, err := regexp.Compile(r.Pattern); err != nil {
			return fmt.Errorf("invalid regex %q: %w", r.Pattern, err)
		}
	}
	if !validSeverities[r.Severity] {
		return fmt.Errorf("severity must be info/warning/critical, got %q", r.Severity)
	}
	if r.Cooldown < 0 || r.Cooldown > 86400 {
		return fmt.Errorf("cooldown must be in [0,86400], got %d", r.Cooldown)
	}
	return nil
}

// Match 判断一条日志是否命中规则。
// unit 需在 r.Units 里（或 Units 为空）；message 命中 Pattern。
// IsRegex 用编译后的正则，否则 strings.Contains（大小写不敏感）。
func Match(r Rule, unit, message string) bool {
	// 每次编译开销小（规则少），但为可读性/正确性仍内联；若规则多可缓存。
	if len(r.Units) > 0 {
		ok := false
		for _, u := range r.Units {
			if u == unit {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if r.IsRegex {
		re, err := regexp.Compile(r.Pattern)
		if err != nil {
			return false
		}
		return re.MatchString(message)
	}
	return strings.Contains(strings.ToLower(message), strings.ToLower(r.Pattern))
}
