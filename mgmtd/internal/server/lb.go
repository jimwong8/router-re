package server

import (
	"encoding/json"
	"net/http"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
	"github.com/jimwong/router-re/mgmtd/internal/lb"
	"github.com/jimwong/router-re/mgmtd/internal/stats"
)

func (s *Server) handleGetLBConfig(w http.ResponseWriter, r *http.Request) {
	cfg := lb.Config{}
	if raw, _ := s.store.Get("lb", "config"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	writeJSON(w, 200, map[string]interface{}{
		"config":  cfg,
		"marks":   lbMarks(cfg),
		"iprules": lb.RenderIpRuleScript(cfg),
	})
}

func (s *Server) handleSetLBConfig(w http.ResponseWriter, r *http.Request) {
	var cfg lb.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, 400, errBody(10160, "invalid body", err.Error()))
		return
	}
	if err := lb.Validate(cfg); err != nil {
		writeJSON(w, 422, errBody(10161, "validation failed", err.Error()))
		return
	}
	applied := false
	detail := ""
	if s.aclApplier != nil {
		if err := lb.ApplyMarkSet(s.aclApplier, lbMarks(cfg)); err != nil {
			detail = err.Error()
		} else if err := lb.ApplyIpRules(s.aclApplier, cfg); err != nil {
			detail = err.Error()
		} else {
			applied = true
			detail = "mark table + ip rules applied"
		}
	}
	b, _ := json.Marshal(cfg)
	_ = s.store.Set("lb", "config", string(b))
	writeJSON(w, 200, map[string]interface{}{
		"applied": applied,
		"config":  cfg,
		"marks":   lbMarks(cfg),
		"iprules": lb.RenderIpRuleScript(cfg),
		"detail":  detail,
	})
}

func (s *Server) handleGetLBStatus(w http.ResponseWriter, r *http.Request) {
	cfg := lb.Config{}
	if raw, _ := s.store.Get("lb", "config"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	ch := lb.NewChecker(cfg.Health)
	for _, m := range cfg.Members {
		if m.Enabled {
			ch.Check(execx.DefaultRunner{}, m)
		}
	}
	// per-member traffic from interface counters (null if unavailable)
	var perMember map[string]interface{}
	if ifaces, err := stats.ReadInterfaces(); err == nil {
		perMember = map[string]interface{}{}
		byName := map[string]stats.IfaceStats{}
		for _, ifc := range ifaces {
			byName[ifc.Name] = ifc
		}
		for _, m := range cfg.Members {
			if v, ok := byName[m.Iface]; ok {
				vv := v
				perMember[m.Name] = &vv
			} else {
				perMember[m.Name] = nil
			}
		}
	}
	writeJSON(w, 200, map[string]interface{}{
		"config":           cfg,
		"status":           ch.Snapshot(),
		"per_member_rx_tx": perMember,
	})
}

func (s *Server) handleLBCheck(w http.ResponseWriter, r *http.Request) {
	cfg := lb.Config{}
	if raw, _ := s.store.Get("lb", "config"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	ch := lb.NewChecker(cfg.Health)
	for _, m := range cfg.Members {
		if m.Enabled {
			ch.Check(execx.DefaultRunner{}, m)
		}
	}
	// Snapshot() 在没有任何成员时返回空切片（make([]Status, 0, ...)），
	// 直接取 [0] 会 panic: index out of range [0] with length 0，
	// 而 panic 会让 HTTP 连接直接断开（客户端看到 RemoteDisconnected），
	// 表现为"接口莫名其妙超时"而不是任何可读的错误。
	// 实测由空输入探针抓到：POST /api/v1/lb/check 带 {} 就断连。
	snap := ch.Snapshot()
	var checkedAt interface{}
	if len(snap) > 0 {
		checkedAt = snap[0].LastCheck
	}
	writeJSON(w, 200, map[string]interface{}{
		"checked_at": checkedAt,
		"status":     snap,
	})
}

func (s *Server) handleLBApply(w http.ResponseWriter, r *http.Request) {
	cfg := lb.Config{}
	if raw, _ := s.store.Get("lb", "config"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	applied := false
	detail := ""
	if s.aclApplier != nil {
		if err := lb.ApplyMarkSet(s.aclApplier, lbMarks(cfg)); err != nil {
			detail = err.Error()
		} else if err := lb.ApplyIpRules(s.aclApplier, cfg); err != nil {
			detail = err.Error()
		} else {
			applied = true
			detail = "mark table + ip rules applied"
		}
	}
	// read back ip rules to confirm
	out, _ := execx.DefaultRunner{}.Run("ip", "rule", "show")
	writeJSON(w, 200, map[string]interface{}{
		"applied": applied,
		"config":  cfg,
		"marks":   lbMarks(cfg),
		"iprules": lb.RenderIpRuleScript(cfg),
		"rules":   out,
		"detail":  detail,
	})
}

func (s *Server) handleLBPreview(w http.ResponseWriter, r *http.Request) {
	cfg := lb.Config{}
	if raw, _ := s.store.Get("lb", "config"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	q := r.URL.Query()
	hashKey := q.Get("hash_key")
	state := map[string]int{}
	m, err := lb.Pick(cfg.Mode, cfg.Members, state, hashKey)
	if err != nil {
		writeJSON(w, 422, errBody(10162, "no member", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"mode":     cfg.Mode,
		"hash_key": hashKey,
		"member":   m,
	})
}

// lbMarks assigns each enabled member a deterministic mark value.
func lbMarks(cfg lb.Config) map[string]int {
	marks := map[string]int{}
	for _, m := range cfg.Members {
		if m.Enabled {
			marks[m.Name] = lb.MemberMark(m.Name)
		}
	}
	return marks
}
