package server

import (
	"encoding/json"
	"net/http"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
	"github.com/jimwong/router-re/mgmtd/internal/htb"
)

// htbStoreKey 保存最近一次配置，供 GET 回显与再次 apply。
const htbStoreKey = "htb/config"

// GET /api/v1/htb/config
func (s *Server) handleGetHTBConfig(w http.ResponseWriter, r *http.Request) {
	raw, _ := s.store.Get("htb", htbStoreKey)
	if raw == "" {
		writeJSON(w, 200, map[string]interface{}{
			"config":  htb.Config{},
			"applied": false,
		})
		return
	}
	var cfg htb.Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		writeJSON(w, 500, errBody(10180, "config unmarshal error", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{"config": cfg, "applied": true})
}

// POST /api/v1/htb/config —— 只持久化配置，不下发（下发走 /htb/apply 且需 confirm）。
func (s *Server) handleSetHTBConfig(w http.ResponseWriter, r *http.Request) {
	var cfg htb.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, 422, errBody(10181, "invalid body", err.Error()))
		return
	}
	if err := htb.Validate(cfg); err != nil {
		writeJSON(w, 422, errBody(10182, "invalid htb config", err.Error()))
		return
	}
	b, _ := json.Marshal(cfg)
	// p95: 与 handleHTBApply 的「读存档 → 下发 → 回写存档」串行化：不加锁时 apply
	// 回写读到的旧存档会盖掉本处刚写入的新配置（rmw_concurrency_test.go 第 4 例）。
	s.htbMu.Lock()
	s.store.Set("htb", htbStoreKey, string(b))
	s.htbMu.Unlock()
	writeJSON(w, 200, map[string]interface{}{
		"config": cfg,
		"script": htb.RenderScript(cfg),
	})
}

// POST /api/v1/htb/apply —— 破坏性操作：默认 dry-run，必须 confirm=true 才真下发。
//
// 为什么默认 dry-run：apply 会先 `tc qdisc del ... root` 再重建整棵树，
// 该接口上所有现存限速规则会被清掉。误点一次就等于把线上限速全丢了。
func (s *Server) handleHTBApply(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Confirm bool       `json:"confirm"`
		Config  htb.Config `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 422, errBody(10183, "invalid body", err.Error()))
		return
	}
	// p95: 「读存档 → 下发 → 回写存档」须与 handleSetHTBConfig 串行化：若 set 落在
	// 读与回写之间，回写会用旧存档盖掉刚写入的新配置（rmw_concurrency_test.go 第 4 例）。
	s.htbMu.Lock()
	defer s.htbMu.Unlock()
	if req.Config.Iface == "" {
		// 允许不传 config，直接用已保存的
		if raw, _ := s.store.Get("htb", htbStoreKey); raw != "" {
			_ = json.Unmarshal([]byte(raw), &req.Config)
		}
	}
	if err := htb.Validate(req.Config); err != nil {
		writeJSON(w, 422, errBody(10184, "invalid htb config", err.Error()))
		return
	}
	if s.aclApplier == nil {
		writeJSON(w, 500, errBody(10185, "exec runner not initialized", ""))
		return
	}

	script := htb.RenderScript(req.Config)
	if !req.Confirm {
		writeJSON(w, 200, map[string]interface{}{
			"applied": false,
			"dry_run": true,
			"script":  script,
			"detail":  "dry-run：未下发。要用同一份 config 真正应用，请带 confirm=true。这会重置该接口现有的 qdisc。",
		})
		return
	}

	// p95: 走 s.aclApplier（生产即 execx.DefaultRunner{}，行为不变；测试可注入
	// fake runner，见 rmw_concurrency_test.go 第 4 例）。
	applied, detail := htb.Apply(s.aclApplier, req.Config)
	if applied {
		b, _ := json.Marshal(req.Config)
		s.store.Set("htb", htbStoreKey, string(b))
	}
	writeJSON(w, 200, map[string]interface{}{
		"applied": applied,
		"dry_run": false,
		"script":  script,
		"detail":  detail,
	})
}

// POST /api/v1/htb/cleanup —— 同样需要 confirm。
func (s *Server) handleHTBCleanup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Confirm bool   `json:"confirm"`
		Iface   string `json:"iface"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 422, errBody(10186, "invalid body", err.Error()))
		return
	}
	if err := htb.ValidateIface(req.Iface); err != nil {
		writeJSON(w, 422, errBody(10187, "invalid iface", err.Error()))
		return
	}
	if !req.Confirm {
		writeJSON(w, 200, map[string]interface{}{
			"applied": false,
			"dry_run": true,
			"detail":  "dry-run：未清理。要真正删除该接口的根 qdisc，请带 confirm=true。",
		})
		return
	}
	if s.aclApplier == nil {
		writeJSON(w, 500, errBody(10188, "exec runner not initialized", ""))
		return
	}
	if err := htb.Cleanup(execx.DefaultRunner{}, req.Iface); err != nil {
		writeJSON(w, 500, errBody(10189, "cleanup failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{"applied": true, "iface": req.Iface})
}

// GET /api/v1/htb/show?iface=ens18 —— 真实 tc 输出。
func (s *Server) handleHTBShow(w http.ResponseWriter, r *http.Request) {
	iface := r.URL.Query().Get("iface")
	if err := htb.ValidateIface(iface); err != nil {
		writeJSON(w, 422, errBody(10190, "invalid iface", err.Error()))
		return
	}
	if s.aclApplier == nil {
		writeJSON(w, 500, errBody(10191, "exec runner not initialized", ""))
		return
	}
	out, err := htb.Show(execx.DefaultRunner{}, iface)
	if err != nil {
		// 接口没有 qdisc 不算服务故障，如实返回原因
		writeJSON(w, 200, map[string]interface{}{
			"iface": iface,
			"raw":   "",
			"error": err.Error(),
		})
		return
	}
	writeJSON(w, 200, map[string]interface{}{"iface": iface, "raw": out})
}

// GET /api/v1/htb/report —— 渲染当前保存的配置的层级说明。
func (s *Server) handleHTBReport(w http.ResponseWriter, r *http.Request) {
	raw, _ := s.store.Get("htb", htbStoreKey)
	var cfg htb.Config
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	writeJSON(w, 200, map[string]interface{}{
		"config": cfg,
		"script": htb.RenderScript(cfg),
	})
}

// handleHTBLimitRules exposes LimitRulesToHTB as an HTTP endpoint.
func (s *Server) handleHTBLimitRules(w http.ResponseWriter, r *http.Request) {
	iface := r.URL.Query().Get("iface")
	if iface == "" {
		iface = "eth0"
	}
	var rootMbit float64 = 100
	cfg := htb.LimitRulesToHTB(nil, iface, rootMbit)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cfg)
}
