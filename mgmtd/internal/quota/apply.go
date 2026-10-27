package quota

import (
	"fmt"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// Apply 校验全部策略后，渲染并下发改动到 router_quota 表，随后回读确认规则数量。
//
// 幂等策略（沿用 internal/qos / internal/nac 的 del+recreate 模式，绝不 flush ruleset）：
//  1. `nft delete table inet router_quota` —— 忽略"表不存在"（首次下发时无所谓）。
//  2. 用 RenderNft 的结果通过 stdin `nft -f -`（RunWithInput）。
//  3. 回读 `nft list table inet router_quota`，统计 drop 规则数；与渲染出的实际
//     drop 规则数不一致则报错——不能只看 nft -f - 的退出码（可能被时序掩盖）。
//
// 返回 (true, 回读内容)。失败返回 (false, 错误说明)。
func Apply(r execx.Runner, policies []Policy) (bool, string) {
	// 校验全部。
	for _, p := range policies {
		if err := Validate(p); err != nil {
			return false, fmt.Sprintf("校验失败 [%s %s]: %v", p.ID, p.Name, err)
		}
	}
	if r == nil {
		r = execx.DefaultRunner{}
	}
	// 幂等：先删旧表（不存在不算错）。
	if _, err := r.Run("nft", "delete", "table", "inet", "router_quota"); err != nil {
		// 表不存在不是错误；其它错误静默留待 nft -f 时报。
		_ = err
	}
	script := RenderNft(policies)
	if _, err := r.RunWithInput(script, "nft", "-f", "-"); err != nil {
		return false, "nft -f - 下发失败: " + err.Error()
	}
	// 回读确认。
	out, err := r.Run("nft", "list", "table", "inet", "router_quota")
	if err != nil {
		return false, "回读 router_quota 失败: " + err.Error()
	}
	want := countDropRules(policies)
	got := countDropRulesInText(out)
	if got != want {
		return false, fmt.Sprintf("回读规则数不匹配: 期望 %d 条 drop 规则, 内核实际 %d 条", want, got)
	}
	return true, out
}

// Cleanup 删除整张 router_quota 表（不存在时不算错误）。调用方需负责确认。
func Cleanup(r execx.Runner) error {
	if r == nil {
		r = execx.DefaultRunner{}
	}
	if _, err := r.Run("nft", "delete", "table", "inet", "router_quota"); err != nil {
		// 表不存在不算错；其它错误也不致命（无表可删时视为已清理）。
		_ = err
	}
	return nil
}

// countDropRules 统计启用策略实际会渲染出的 drop 规则条数（用于回读比对）。
func countDropRules(policies []Policy) int {
	n := 0
	for _, p := range policies {
		if !p.Enabled {
			continue
		}
		if p.RateMbit > 0 {
			n++
		}
		if p.MaxConns > 0 {
			n++
		}
		if p.MaxConnsPerSec > 0 {
			n++
		}
	}
	return n
}

// countDropRulesInText 统计 nft 输出中出现的 " drop;" / " drop\n" drop 规则条数。
func countDropRulesInText(text string) int {
	n := 0
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		// 跳过注释行与集合/结构行；只要行尾是 drop 结尾的规则。
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasSuffix(line, "drop") || strings.HasSuffix(line, "drop;") {
			n++
		}
	}
	return n
}
