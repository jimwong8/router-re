package alerting

import (
	"sync"
)

// Alert 是一条触发后的告警。
type Alert struct {
	RuleID   string
	RuleName string
	Severity string
	Unit     string
	Message  string
	Ts       int64
}

// Engine 持有一组规则的 cooldown 状态：ruleID -> 上次实际告警时间。
type Engine struct {
	mu   sync.Mutex
	last map[string]int64
}

// NewEngine 返回空引擎。
func NewEngine() *Engine {
	return &Engine{last: make(map[string]int64)}
}

// Evaluate 对一条日志跑所有规则。
// 命中且未在 cooldown 内的 → 产生 Alert，并更新该规则的 last 时间。
// cooldown 内的命中 → 计数为 suppressed（不重复告警），调用方可据此知道发生了抑制。
// 只考虑 Enabled 规则。
func (e *Engine) Evaluate(rules []Rule, unit, message string, now int64) (alerts []Alert, suppressed int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		if !Match(r, unit, message) {
			continue
		}
		lastAt := e.last[r.ID]
		if r.Cooldown > 0 && now-lastAt < int64(r.Cooldown) {
			// 仍在 cooldown 内：抑制。
			suppressed++
			continue
		}
		e.last[r.ID] = now
		alerts = append(alerts, Alert{
			RuleID:   r.ID,
			RuleName: r.Name,
			Severity: r.Severity,
			Unit:     unit,
			Message:  message,
			Ts:       now,
		})
	}
	return alerts, suppressed
}

// Reset 清空所有 cooldown 状态。
func (e *Engine) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.last = make(map[string]int64)
}
