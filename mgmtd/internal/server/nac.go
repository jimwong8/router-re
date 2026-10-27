package server

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
	"github.com/jimwong/router-re/mgmtd/internal/nac"
)

func (s *Server) handleGetNACConfig(w http.ResponseWriter, r *http.Request) {
	cfg := nac.Config{}
	if raw, _ := s.store.Get("nac", "config"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	writeJSON(w, 200, map[string]interface{}{
		"config": cfg,
		"nft":    nac.RenderNft(cfg),
	})
}

func (s *Server) handleSetNACConfig(w http.ResponseWriter, r *http.Request) {
	var cfg nac.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, 400, errBody(10150, "invalid body", err.Error()))
		return
	}
	if err := nac.Validate(cfg); err != nil {
		writeJSON(w, 422, errBody(10151, "validation failed", err.Error()))
		return
	}
	applied := false
	detail := ""
	if s.aclApplier != nil {
		if err := nac.ApplyNAC(s.aclApplier, cfg); err != nil {
			detail = err.Error()
		} else {
			applied = true
			detail = "router_nac table applied"
		}
	}
	b, _ := json.Marshal(cfg)
	_ = s.store.Set("nac", "config", string(b))
	writeJSON(w, 200, map[string]interface{}{
		"applied": applied,
		"config":  cfg,
		"nft":     nac.RenderNft(cfg),
		"detail":  detail,
	})
}

func (s *Server) handleGetNACNeighbors(w http.ResponseWriter, r *http.Request) {
	devs, err := nac.Neighbors(execx.DefaultRunner{})
	if err != nil {
		writeJSON(w, 500, errBody(10152, "neighbors failed", err.Error()))
		return
	}
	if devs == nil {
		devs = []nac.Device{}
	}
	writeJSON(w, 200, map[string]interface{}{"neighbors": devs, "count": len(devs)})
}

func (s *Server) handleGetNACConnlimits(w http.ResponseWriter, r *http.Request) {
	ls := []nac.Limit{}
	if raw, _ := s.store.Get("nac", "connlimits"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &ls)
	}
	writeJSON(w, 200, map[string]interface{}{
		"limits": ls,
		"nft":    nac.RenderConnlimit(ls),
	})
}

func (s *Server) handleSetNACConnlimits(w http.ResponseWriter, r *http.Request) {
	var ls []nac.Limit
	if err := json.NewDecoder(r.Body).Decode(&ls); err != nil {
		writeJSON(w, 400, errBody(10150, "invalid body", err.Error()))
		return
	}
	for _, l := range ls {
		if err := nac.ValidateLimit(l); err != nil {
			writeJSON(w, 422, errBody(10153, "validation failed", err.Error()))
			return
		}
	}
	b, _ := json.Marshal(ls)
	_ = s.store.Set("nac", "connlimits", string(b))
	writeJSON(w, 200, map[string]interface{}{
		"limits": ls,
		"nft":    nac.RenderConnlimit(ls),
		"saved":  true,
	})
}

func (s *Server) handleApplyNACConnlimits(w http.ResponseWriter, r *http.Request) {
	ls := []nac.Limit{}
	if raw, _ := s.store.Get("nac", "connlimits"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &ls)
	}
	applied := false
	detail := ""
	if s.aclApplier != nil {
		if err := nac.Apply(s.aclApplier, ls); err != nil {
			detail = err.Error()
		} else {
			applied = true
			detail = "router_connlimit table applied"
		}
	}
	writeJSON(w, 200, map[string]interface{}{
		"applied": applied,
		"limits":  ls,
		"nft":     nac.RenderConnlimit(ls),
		"detail":  detail,
	})
}

func (s *Server) handleGetNACTopConnections(w http.ResponseWriter, r *http.Request) {
	counts, err := nac.TopConnections(execx.DefaultRunner{})
	if err != nil {
		writeJSON(w, 500, errBody(10154, "top connections failed", err.Error()))
		return
	}
	type entry struct {
		IP    string `json:"ip"`
		Count int    `json:"count"`
	}
	entries := []entry{}
	for ip, c := range counts {
		entries = append(entries, entry{IP: ip, Count: c})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Count > entries[j].Count })
	if len(entries) > 20 {
		entries = entries[:20]
	}
	writeJSON(w, 200, map[string]interface{}{"top": entries, "count": len(entries)})
}
