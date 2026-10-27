package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
	"github.com/jimwong/router-re/mgmtd/internal/l7filter"
)

const l7RulesKey = "l7/rules"

// loadL7Rules reads the persisted L7 filter rules.
func (s *Server) loadL7Rules() []l7filter.Rule {
	rs := []l7filter.Rule{}
	s.loadJSON("l7", "rules", &rs)
	return rs
}

// l7DnsmasqDomains returns the enabled block-rule domains (sni/http_host/dns).
// UA 规则不能在 dnsmasq 层表达——只有域名类规则能写成 address=/domain/黑洞，
// 使该域名（及子域）的所有 DNS 查询都被黑洞掉，从而拦截对应 L7 流量。
func l7DnsmasqDomains(rules []l7filter.Rule) []string {
	var domains []string
	for _, r := range rules {
		if !r.Enabled || r.Action != l7filter.ActionBlock {
			continue
		}
		switch r.MatchType {
		case l7filter.MatchSNI, l7filter.MatchHTTPHost, l7filter.MatchDNS:
			domains = append(domains, r.Pattern)
		}
	}
	return domains
}

// renderL7Dnsmasq builds the dnsmasq block text for the current block rules.
func renderL7Dnsmasq(rules []l7filter.Rule) string {
	return l7filter.RenderDnsmasqBlock(l7DnsmasqDomains(rules))
}

// handleGetL7Rules returns the rules plus the generated dnsmasq text.
func (s *Server) handleGetL7Rules(w http.ResponseWriter, r *http.Request) {
	rules := s.loadL7Rules()
	writeJSON(w, 200, map[string]interface{}{
		"rules":   rules,
		"dnsmasq": renderL7Dnsmasq(rules),
	})
}

// handleSetL7Rules validates, persists and renders the rule set.
func (s *Server) handleSetL7Rules(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Rules []l7filter.Rule `json:"rules"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10150, "invalid body", err.Error()))
		return
	}
	// 校验每一条，同时保证 ID 唯一。
	seen := map[string]bool{}
	for i, rule := range req.Rules {
		if err := l7filter.Validate(rule); err != nil {
			writeJSON(w, 422, errBody(10151, "validation failed", "rule "+itoa(i)+": "+err.Error()))
			return
		}
		if seen[rule.ID] {
			writeJSON(w, 422, errBody(10152, "duplicate rule id", rule.ID))
			return
		}
		seen[rule.ID] = true
	}
	s.storeJSON("l7", "rules", req.Rules)
	writeJSON(w, 200, map[string]interface{}{
		"rules":   req.Rules,
		"applied": false,
		"dnsmasq": renderL7Dnsmasq(req.Rules),
		"detail":  "规则已保存；调用 /api/v1/l7/apply 写入 dnsmasq 并重启",
	})
}

// handleL7Test evaluates a request context against the enabled rules —
// 纯调试用，不改动任何配置。
func (s *Server) handleL7Test(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SNI       string `json:"sni"`
		HTTPHost  string `json:"http_host"`
		UserAgent string `json:"user_agent"`
		DNSName   string `json:"dns_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10153, "invalid body", err.Error()))
		return
	}
	rules := s.loadL7Rules()
	ctx := l7filter.Request{
		SNI:       req.SNI,
		HTTPHost:  req.HTTPHost,
		UserAgent: req.UserAgent,
		DNSName:   req.DNSName,
	}
	blocked, by := l7filter.ShouldBlock(rules, ctx)
	writeJSON(w, 200, map[string]interface{}{
		"hit":     l7filter.MatchRequest(rules, ctx),
		"blocked": blocked,
		"by":      by,
	})
}

// handleGetL7Stats returns counts grouped by type and by action.
func (s *Server) handleGetL7Stats(w http.ResponseWriter, r *http.Request) {
	rules := s.loadL7Rules()
	enabled := 0
	byType := map[string]int{}
	byAction := map[string]int{}
	for _, rule := range rules {
		if rule.Enabled {
			enabled++
		}
		byType[rule.MatchType]++
		byAction[rule.Action]++
	}
	writeJSON(w, 200, map[string]interface{}{
		"total":     len(rules),
		"enabled":   enabled,
		"by_type":   byType,
		"by_action": byAction,
	})
}

// applyL7Dnsmasq writes the block config and restarts dnsmasq, reporting both
// restart failures explicitly (systemctl + init.d). Shares logic with the
// webfilter path so the L7 block file coexists with it.
func applyL7Dnsmasq(r execx.Runner, conf string) error {
	const confPath = "/etc/dnsmasq.d/router-re-l7.conf"
	if err := os.MkdirAll(filepath.Dir(confPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(confPath, []byte(conf), 0o644); err != nil {
		return err
	}
	sysErr := error(nil)
	if _, err := r.Run("systemctl", "restart", "dnsmasq"); err == nil {
		return nil
	} else {
		sysErr = err
	}
	initErr := error(nil)
	if _, err := r.Run("/etc/init.d/dnsmasq", "restart"); err == nil {
		return nil
	} else {
		initErr = err
	}
	return fmt.Errorf("配置已写入 %s，但 dnsmasq 未重启：systemctl: %v；init.d: %v", confPath, sysErr, initErr)
}

// handleL7Apply writes block domains into dnsmasq config and restarts the
// service. 重启失败时返回 {applied:false, detail:两条真实错误}。
func (s *Server) handleL7Apply(w http.ResponseWriter, r *http.Request) {
	rules := s.loadL7Rules()
	conf := renderL7Dnsmasq(rules)
	if s.aclApplier == nil {
		writeJSON(w, 500, errBody(10154, "no runner", "exec runner not initialized"))
		return
	}
	err := applyL7Dnsmasq(s.aclApplier, conf)
	if err != nil {
		writeJSON(w, 200, map[string]interface{}{"applied": false, "detail": err.Error(), "dnsmasq": conf})
		return
	}
	writeJSON(w, 200, map[string]interface{}{"applied": true, "detail": "dnsmasq restarted with L7 block rules", "dnsmasq": conf})
}
