package server

import (
	"encoding/json"
	"net/http"

	"github.com/jimwong/router-re/mgmtd/internal/attackguard"
	"github.com/jimwong/router-re/mgmtd/internal/webfilter"
)

func (s *Server) handleGetWebfilterRules(w http.ResponseWriter, r *http.Request) {
	rules := []webfilter.Rule{}
	if raw, _ := s.store.Get("webfilter", "rules"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &rules)
	}
	writeJSON(w, 200, map[string]interface{}{
		"rules":   rules,
		"dnsmasq": webfilter.RenderDnsmasq(rules),
	})
}

func (s *Server) handleSetWebfilterRules(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Rules []webfilter.Rule `json:"rules"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10120, "invalid body", err.Error()))
		return
	}
	if len(req.Rules) == 0 {
		writeJSON(w, 422, errBody(10121, "empty rule set", "POST /webfilter/rules must carry at least one rule"))
		return
	}
	if err := webfilter.Validate(req.Rules); err != nil {
		writeJSON(w, 422, errBody(10122, "validation failed", err.Error()))
		return
	}
	b, _ := json.Marshal(req.Rules)
	_ = s.store.Set("webfilter", "rules", string(b))
	applied := false
	detail := ""
	if s.aclApplier != nil {
		if err := webfilter.Apply(s.aclApplier, req.Rules, "/etc/dnsmasq.d/router-re-webfilter.conf"); err != nil {
			detail = err.Error()
		} else {
			applied = true
			detail = "dnsmasq restarted with new rules"
		}
	}
	writeJSON(w, 200, map[string]interface{}{
		"applied": applied,
		"rules":   req.Rules,
		"dnsmasq": webfilter.RenderDnsmasq(req.Rules),
		"detail":  detail,
	})
}

func (s *Server) handleGetAttackGuard(w http.ResponseWriter, r *http.Request) {
	cfg := attackguard.Config{}
	raw, _ := s.store.Get("security", "attackguard")
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	// read back live nft state if possible
	nft := ""
	if s.aclApplier != nil {
		if out, err := s.aclApplier.Run("nft", "list", "table", "inet", "router_guard"); err == nil {
			nft = out
		}
	}
	writeJSON(w, 200, map[string]interface{}{"config": cfg, "nft": nft})
}

func (s *Server) handleSetAttackGuard(w http.ResponseWriter, r *http.Request) {
	var cfg attackguard.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, 400, errBody(10123, "invalid body", err.Error()))
		return
	}
	if err := attackguard.Validate(cfg); err != nil {
		writeJSON(w, 422, errBody(10124, "validation failed", err.Error()))
		return
	}
	// Persist first: the operator's configuration must survive even when the
	// live apply fails (no CAP_NET_ADMIN in a dev box, nft not installed, a
	// transient kernel error). The response reports `applied` separately so
	// the UI can show "saved but not active" instead of losing the edit.
	b, _ := json.Marshal(cfg)
	_ = s.store.Set("security", "attackguard", string(b))

	applied := false
	detail := ""
	nftOut := attackguard.RenderNft(cfg)
	if s.aclApplier != nil {
		if err := attackguard.Apply(s.aclApplier, cfg); err != nil {
			detail = err.Error()
		} else {
			applied = true
			detail = "nft table router_guard applied"
			if out, err := s.aclApplier.Run("nft", "list", "table", "inet", "router_guard"); err == nil {
				nftOut = out
			}
		}
	}
	writeJSON(w, 200, map[string]interface{}{
		"applied": applied,
		"config":  cfg,
		"nft":     nftOut,
		"detail":  detail,
	})
}
