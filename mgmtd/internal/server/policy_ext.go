package server

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jimwong/router-re/mgmtd/internal/behavior"
	"github.com/jimwong/router-re/mgmtd/internal/dnsproxy"
	"github.com/jimwong/router-re/mgmtd/internal/execx"
	"github.com/jimwong/router-re/mgmtd/internal/policyroute"
)

const multiLineConfPath = "/etc/dnsmasq.d/rr-multiline.conf"

// ---------- 多线路 DNS ----------

func (s *Server) handleGetMultiLineDNS(w http.ResponseWriter, r *http.Request) {
	var cfg dnsproxy.MultiLineConfig
	if raw, _ := s.store.Get("dns", "multiline"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	rendered, err := dnsproxy.RenderMultiLine(cfg)
	resp := map[string]interface{}{"config": cfg, "rendered": rendered}
	if err != nil {
		resp["render_error"] = err.Error()
	}
	writeJSON(w, 200, resp)
}

func (s *Server) handleSetMultiLineDNS(w http.ResponseWriter, r *http.Request) {
	var req struct {
		dnsproxy.MultiLineConfig
		Apply bool `json:"apply"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(11120, "json decode fail", err.Error()))
		return
	}
	if err := dnsproxy.ValidateMultiLine(req.MultiLineConfig); err != nil {
		writeJSON(w, 422, errBody(11121, "multiline dns invalid", err.Error()))
		return
	}
	rendered, err := dnsproxy.RenderMultiLine(req.MultiLineConfig)
	if err != nil {
		writeJSON(w, 422, errBody(11122, "render fail", err.Error()))
		return
	}
	b, _ := json.Marshal(req.MultiLineConfig)
	if err := s.store.Set("dns", "multiline", string(b)); err != nil {
		writeJSON(w, 500, errBody(11123, "store fail", err.Error()))
		return
	}
	resp := map[string]interface{}{"config": req.MultiLineConfig, "rendered": rendered, "applied": false}
	if req.Apply {
		ok, detail := s.applyMultiLine(rendered)
		resp["applied"] = ok
		resp["status"] = detail
	}
	writeJSON(w, 200, resp)
}

// applyMultiLine 写入 dnsmasq 片段并校验配置。校验失败立刻回滚，
// 否则一次手误就能让全内网 DNS 起不来。
func (s *Server) applyMultiLine(conf string) (bool, string) {
	if strings.TrimSpace(conf) == "" {
		if err := os.Remove(multiLineConfPath); err != nil && !os.IsNotExist(err) {
			return false, err.Error()
		}
	} else {
		if err := writeFileSafe(multiLineConfPath, conf); err != nil {
			return false, err.Error()
		}
	}
	runner := execx.DefaultRunner{}
	if out, err := runner.Run("dnsmasq", "--test", "--conf-file=/etc/dnsmasq.conf"); err != nil {
		_ = os.Remove(multiLineConfPath)
		return false, "dnsmasq --test 失败，已回滚：" + strings.TrimSpace(out)
	}
	ok, detail := runCommands(runner, [][]string{{"systemctl", "restart", "dnsmasq"}})
	return ok, detail
}

// ---------- 域名分流 ----------

func (s *Server) loadDomainRules() []policyroute.DomainRule {
	out := make([]policyroute.DomainRule, 0)
	raw, _ := s.store.Get("policy", "domain_rules")
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &out)
	}
	return out
}

func (s *Server) handleGetDomainRules(w http.ResponseWriter, r *http.Request) {
	rules := s.loadDomainRules()
	writeJSON(w, 200, map[string]interface{}{"rules": rules, "count": len(rules)})
}

func (s *Server) handleSetDomainRules(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Rules []policyroute.DomainRule `json:"rules"`
		Apply bool                     `json:"apply"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(11130, "json decode fail", err.Error()))
		return
	}
	if err := policyroute.ValidateDomainRules(req.Rules); err != nil {
		writeJSON(w, 422, errBody(11131, "domain rules invalid", err.Error()))
		return
	}
	assigned := policyroute.AssignMarks(req.Rules)
	nft, err := policyroute.RenderNft(assigned, nil)
	if err != nil {
		writeJSON(w, 422, errBody(11132, "render nft fail", err.Error()))
		return
	}
	resp := map[string]interface{}{
		"rules":    assigned,
		"dnsmasq":  policyroute.RenderDnsmasqIpset(assigned),
		"nft":      nft,
		"ip_rules": policyroute.RenderIpRules(assigned),
		"applied":  false,
	}
	b, _ := json.Marshal(assigned)
	if err := s.store.Set("policy", "domain_rules", string(b)); err != nil {
		writeJSON(w, 500, errBody(11133, "store fail", err.Error()))
		return
	}
	if req.Apply {
		runner := execx.DefaultRunner{}
		// 先把 nft 规则整体替换到一个独立表里，失败不影响现有规则
		script := "table inet rr_dom\n" + strings.ReplaceAll(nft, "table inet rr_dom\n", "")
		if err := writeFileSafe("/etc/nftables.d/rr-domain.nft", script); err != nil {
			resp["error"] = err.Error()
		} else if out, err := runner.Run("nft", "-f", "/etc/nftables.d/rr-domain.nft"); err != nil {
			resp["error"] = strings.TrimSpace(out)
		} else {
			if err := writeFileSafe("/etc/dnsmasq.d/rr-domain-ipset.conf", policyroute.RenderDnsmasqIpset(assigned)); err == nil {
				if _, err := runner.Run("systemctl", "restart", "dnsmasq"); err == nil {
					resp["applied"] = true
				}
			}
		}
	}
	writeJSON(w, 200, resp)
}

// ---------- 行为记录 ----------

// 行为记录用进程内环形缓冲（默认 2 万条）。它记录的是"最近发生了什么"，
// 不是审计证据；要长期留存应该导出到 syslog / 时序库，不做在这里。
var (
	behaviorStore = behavior.NewStore(20000)
	onlineTracker = behavior.NewOnlineTracker()
)

func (s *Server) handleBehaviorRecords(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := behavior.Query{
		Kind:    q.Get("kind"),
		MAC:     q.Get("mac"),
		IP:      q.Get("ip"),
		Keyword: q.Get("keyword"),
	}
	if v := q.Get("since"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			query.Since = n
		}
	}
	if v := q.Get("until"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			query.Until = n
		}
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			query.Limit = n
		}
	}
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			query.Offset = n
		}
	}
	recs, total := behaviorStore.Query(query)
	writeJSON(w, 200, map[string]interface{}{"records": recs, "total": total})
}

func (s *Server) handleBehaviorStats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]interface{}{
		"stats":   behaviorStore.Stats(),
		"exempts": behaviorStore.Exempts(),
		"online":  onlineTracker.Online(),
	})
}

// handlePostBehaviorRecord 接受上报（DPI/认证/终端模块），并顺带更新上下线状态。
func (s *Server) handlePostBehaviorRecord(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Records []behavior.Record `json:"records"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(11140, "json decode fail", err.Error()))
		return
	}
	if len(req.Records) == 0 || len(req.Records) > 500 {
		writeJSON(w, 422, errBody(11141, "bad batch", "单次最多 500 条，且不能为空"))
		return
	}
	now := time.Now().Unix()
	for i := range req.Records {
		if req.Records[i].TS == 0 {
			req.Records[i].TS = now
		}
	}
	behaviorStore.Add(req.Records...)
	// 上下线记录同时更新在线表
	for _, rec := range req.Records {
		if rec.Kind == behavior.KindOnline && rec.MAC != "" {
			extra := onlineTracker.Observe(rec.MAC, rec.IP, rec.Terminal, rec.TS)
			behaviorStore.Add(extra...)
		}
	}
	writeJSON(w, 200, map[string]interface{}{"accepted": len(req.Records), "total": behaviorStore.Stats()["total"]})
}

func (s *Server) handleGetBehaviorExempt(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]interface{}{"exempts": behaviorStore.Exempts()})
}

func (s *Server) handleSetBehaviorExempt(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Exempts []behavior.Exempt `json:"exempts"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(11142, "json decode fail", err.Error()))
		return
	}
	if err := behaviorStore.SetExempt(req.Exempts); err != nil {
		writeJSON(w, 422, errBody(11143, "exempt invalid", err.Error()))
		return
	}
	b, _ := json.Marshal(req.Exempts)
	_ = s.store.Set("behavior", "exempts", string(b))
	writeJSON(w, 200, map[string]interface{}{"exempts": behaviorStore.Exempts(), "saved": true})
}
