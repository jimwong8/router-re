package ips

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Engine 持有一组已解析的检测规则，提供匹配与统计。
type Engine struct {
	mu    sync.RWMutex
	rules []Rule
}

// NewEngine 返回空引擎。
func NewEngine() *Engine { return &Engine{} }

// Load 替换当前规则集。
func (e *Engine) Load(rs []Rule) {
	e.mu.Lock()
	e.rules = rs
	e.mu.Unlock()
}

// Match 返回命中所有规则（按 severity 升序）。
func (e *Engine) Match(proto string, dstPort int, payload []byte) []Rule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	var hits []Rule
	for _, r := range e.rules {
		if !r.Enabled {
			continue
		}
		if r.Match(proto, dstPort, payload) {
			hits = append(hits, r)
		}
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].Severity < hits[j].Severity })
	return hits
}

// Stats 返回引擎统计 {total, enabled, dropped, alerted}。
func (e *Engine) Stats() map[string]int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	st := map[string]int{"total": len(e.rules), "enabled": 0, "dropped": 0, "alerted": 0}
	for _, r := range e.rules {
		if r.Enabled {
			st["enabled"]++
		}
		switch r.Action {
		case "drop":
			st["dropped"]++
		case "alert":
			st["alerted"]++
		}
	}
	return st
}

// ParseRulesFile 逐行解析 Suricata 规则文本。
// 跳过空行与 # 注释；单行解析失败记录行号并继续，末尾返回聚合错误。
func ParseRulesFile(text string) ([]Rule, error) {
	lines := strings.Split(text, "\n")
	var rules []Rule
	var fails []string
	for i, raw := range lines {
		lineno := i + 1
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		r, err := ParseRule(line)
		if err != nil {
			fails = append(fails, fmt.Sprintf("line %d: %s", lineno, err))
			continue
		}
		rules = append(rules, r)
	}
	if len(fails) > 0 {
		return rules, fmt.Errorf("rule parse errors (%d): %s", len(fails), strings.Join(fails, "; "))
	}
	return rules, nil
}

// RenderNft 渲染封禁 IP 列表到 nftables 规则片段：
// table inet router_ips { set blackhole { type ipv4_addr; flags timeout; ... } }
// 绝不写 flush ruleset。多行声明。空集时不输出 elements 行（nft 语法要求）。
func RenderNft(blocked map[string]bool) string {
	var b strings.Builder
	b.WriteString("table inet router_ips {\n")
	b.WriteString("    set blackhole {\n")
	b.WriteString("        type ipv4_addr;\n")
	b.WriteString("        flags timeout;\n")
	if len(blocked) > 0 {
		elems := make([]string, 0, len(blocked))
		for ip := range blocked {
			elems = append(elems, ip)
		}
		sort.Strings(elems)
		b.WriteString("        elements = { " + strings.Join(elems, ", ") + " }\n")
	}
	b.WriteString("    }\n")
	b.WriteString("}\n")
	return b.String()
}
