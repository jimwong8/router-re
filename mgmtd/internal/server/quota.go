package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
	"github.com/jimwong/router-re/mgmtd/internal/quota"
)

// quotaStoreKey 是策略数组在配置存储里的持久化键。
const quotaStoreKey = "policies"

// loadQuotaPolicies 从 store 读取策略数组。空或不存在返回空切片。
func (s *Server) loadQuotaPolicies() []quota.Policy {
	var policies []quota.Policy
	if raw, err := s.store.Get("quota", quotaStoreKey); err == nil && raw != "" {
		_ = json.Unmarshal([]byte(raw), &policies)
	}
	if policies == nil {
		policies = []quota.Policy{}
	}
	return policies
}

// handleGetQuotaPolicies 返回当前策略列表与渲染的报告。
func (s *Server) handleGetQuotaPolicies(w http.ResponseWriter, r *http.Request) {
	policies := s.loadQuotaPolicies()
	writeJSON(w, 200, map[string]interface{}{
		"policies": policies,
		"count":    len(policies),
		"report":   quota.RenderReport(policies),
	})
}

// handleSetQuotaPolicies 整体替换策略列表并持久化到 store。逐条校验；存在非法
// 策略则不落库，返回具体错误（含拒绝 cidr /0 的红线）。
func (s *Server) handleSetQuotaPolicies(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Policies []quota.Policy `json:"policies"`
		Confirm bool `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10170, "json decode fail", err.Error()))
		return
	}
	if len(req.Policies) == 0 && !req.Confirm {
		writeJSON(w, 422, errBody(10173, "policies must not be empty",
			"to clear all quota policies, send {policies:[],confirm:true}"))
		return
	}
	for _, p := range req.Policies {
		if err := quota.Validate(p); err != nil {
			writeJSON(w, 422, errBody(10171, "quota policy validation failed", fmt.Sprintf("[%s %s] %v", p.ID, p.Name, err)))
			return
		}
	}
	if err := s.store.Set("quota", quotaStoreKey, mustMarshalPolicies(req.Policies)); err != nil {
		writeJSON(w, 500, errBody(10172, "store write failed", err.Error()))
		return
	}
	s.logAudit(r, "quota.policies.set", fmt.Sprintf("%d policies", len(req.Policies)), "ok", "")
	writeJSON(w, 200, map[string]interface{}{
		"policies": req.Policies,
		"count":    len(req.Policies),
		"report":   quota.RenderReport(req.Policies),
	})
}

// handleQuotaApply 下发 router_quota 表到内核（破坏性：del+recreate 该表）。
// 要求 body 带 {"confirm": true}，否则只返回需要确认的提示，不做任何改动。
func (s *Server) handleQuotaApply(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Confirm bool `json:"confirm"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if !req.Confirm {
		writeJSON(w, 409, errBody(10173, "confirmation required",
			"破坏性操作：将重建内核 router_quota 表，请带 {\"confirm\": true} 确认"))
		return
	}
	policies := s.loadQuotaPolicies()
	ok, msg := quota.Apply(execx.DefaultRunner{}, policies)
	if !ok {
		writeJSON(w, 500, errBody(10174, "quota apply failed", msg))
		return
	}
	s.logAudit(r, "quota.apply", fmt.Sprintf("%d policies", len(policies)), "ok", "")
	writeJSON(w, 200, map[string]interface{}{
		"applied": true,
		"count":   len(policies),
		"nft":     msg,
	})
}

// handleQuotaTest 调试：给定连接上下文，返回第一个命中的启用策略。
func (s *Server) handleQuotaTest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SrcIP   string `json:"src_ip"`
		SrcMAC  string `json:"src_mac"`
		DstIP   string `json:"dst_ip"`
		DstPort string `json:"dst_port"`
		App     string `json:"app"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10175, "json decode fail", err.Error()))
		return
	}
	ctx := quota.ConnCtx{
		SrcIP:   req.SrcIP,
		SrcMAC:  req.SrcMAC,
		DstIP:   req.DstIP,
		DstPort: req.DstPort,
		App:     req.App,
	}
	policies := s.loadQuotaPolicies()
	hit := quota.FirstMatch(policies, ctx)
	if hit == nil {
		writeJSON(w, 200, map[string]interface{}{
			"hit":        nil,
			"matched":    false,
			"ctx":        ctx,
			"scan_count": len(policies),
		})
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"hit":        hit,
		"matched":    true,
		"policy_id":  hit.ID,
		"ctx":        ctx,
		"scan_count": len(policies),
	})
}

// handleQuotaReport 返回人类可读的策略摘要。
func (s *Server) handleQuotaReport(w http.ResponseWriter, r *http.Request) {
	policies := s.loadQuotaPolicies()
	writeJSON(w, 200, map[string]interface{}{
		"count":  len(policies),
		"report": strings.Split(quota.RenderReport(policies), "\n"),
	})
}

// handleQuotaNft 返回内核里 router_quota 表的实际内容（只读）。
func (s *Server) handleQuotaNft(w http.ResponseWriter, r *http.Request) {
	out, err := execx.DefaultRunner{}.Run("nft", "list", "table", "inet", "router_quota")
	if err != nil {
		// 表未定义/未下发也是合法状态。
		writeJSON(w, 200, map[string]interface{}{
			"exists": false,
			"nft":    "",
			"note":   "内核中暂无 router_quota 表（尚未下发）",
		})
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"exists": true,
		"nft":    out,
	})
}

// mustMarshalPolicies 编码策略数组；理论上不会失败（均为可序列化结构）。
func mustMarshalPolicies(p []quota.Policy) string {
	b, _ := json.Marshal(p)
	return string(b)
}
