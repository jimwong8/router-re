package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
	"github.com/jimwong/router-re/mgmtd/internal/multiwan"
	"github.com/jimwong/router-re/mgmtd/internal/webadmin"
)

const (
	webadminStoreKey = "webadmin/listen"
	identityStoreKey = "webadmin/identity"
	wanMembersKey    = "multiwan/members"
	wanProbeKey      = "multiwan/probe"
	wanStatesKey     = "multiwan/states"
)

// ---------- Web 管理端口 ----------

func (s *Server) handleGetWebAdmin(w http.ResponseWriter, r *http.Request) {
	raw, _ := s.store.Get("webadmin", webadminStoreKey)
	var cfg webadmin.ListenConfig
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	writeJSON(w, 200, map[string]interface{}{"config": cfg, "nginx": webadmin.RenderNginx(cfg)})
}

func (s *Server) handleSetWebAdmin(w http.ResponseWriter, r *http.Request) {
	var cfg webadmin.ListenConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, 422, errBody(10360, "invalid body", err.Error()))
		return
	}
	if err := webadmin.ValidateListen(cfg); err != nil {
		writeJSON(w, 422, errBody(10361, "invalid listen config", err.Error()))
		return
	}
	b, _ := json.Marshal(cfg)
	s.store.Set("webadmin", webadminStoreKey, string(b))
	writeJSON(w, 200, map[string]interface{}{
		"config": cfg, "nginx": webadmin.RenderNginx(cfg),
		"detail": "配置已保存。实际生效需要重新加载 nginx（本接口不会重启服务，避免把自己锁在外面）。",
	})
}

func (s *Server) handleGetIdentity(w http.ResponseWriter, r *http.Request) {
	raw, _ := s.store.Get("webadmin", identityStoreKey)
	var id webadmin.Identity
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &id)
	}
	writeJSON(w, 200, map[string]interface{}{"identity": id})
}

func (s *Server) handleSetIdentity(w http.ResponseWriter, r *http.Request) {
	var id webadmin.Identity
	if err := json.NewDecoder(r.Body).Decode(&id); err != nil {
		writeJSON(w, 422, errBody(10362, "invalid body", err.Error()))
		return
	}
	if err := webadmin.ValidateIdentity(id); err != nil {
		writeJSON(w, 422, errBody(10363, "invalid identity", err.Error()))
		return
	}
	ok, detail := webadmin.ApplyIdentity(execx.DefaultRunner{}, id)
	if ok {
		b, _ := json.Marshal(id)
		s.store.Set("webadmin", identityStoreKey, string(b))
	}
	writeJSON(w, 200, map[string]interface{}{"applied": ok, "identity": id, "detail": detail})
}

// ---------- 多 WAN 健康检查 ----------

func (s *Server) handleGetMultiWAN(w http.ResponseWriter, r *http.Request) {
	rawM, _ := s.store.Get("multiwan", wanMembersKey)
	var members []multiwan.Member
	if rawM != "" {
		_ = json.Unmarshal([]byte(rawM), &members)
	}
	rawP, _ := s.store.Get("multiwan", wanProbeKey)
	var probe multiwan.Probe
	if rawP != "" {
		_ = json.Unmarshal([]byte(rawP), &probe)
	}
	rawS, _ := s.store.Get("multiwan", wanStatesKey)
	states := map[string]multiwan.State{}
	if rawS != "" {
		_ = json.Unmarshal([]byte(rawS), &states)
	}
	writeJSON(w, 200, map[string]interface{}{
		"members": members, "probe": probe, "states": states,
		"count": len(members),
	})
}

func (s *Server) handleSetMultiWAN(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Members []multiwan.Member `json:"members"`
		Probe   multiwan.Probe    `json:"probe"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 422, errBody(10364, "invalid body", err.Error()))
		return
	}
	for i, m := range body.Members {
		if err := multiwan.ValidateMember(m); err != nil {
			writeJSON(w, 422, errBody(10365, "invalid member",
				"members["+itoa(i)+"] "+m.Name+": "+err.Error()))
			return
		}
	}
	if err := multiwan.ValidateProbe(body.Probe); err != nil {
		writeJSON(w, 422, errBody(10366, "invalid probe", err.Error()))
		return
	}
	b1, _ := json.Marshal(body.Members)
	b2, _ := json.Marshal(body.Probe)
	s.store.Set("multiwan", wanMembersKey, string(b1))
	s.store.Set("multiwan", wanProbeKey, string(b2))
	writeJSON(w, 200, map[string]interface{}{
		"members": body.Members, "probe": body.Probe, "count": len(body.Members),
	})
}

// POST /api/v1/multiwan/probe —— 记录一次探测结果并推动状态机。
//
// 这个接口收到的是"探测结果"，不是"探测动作" —— 真正的探测（ping/http）
// 由部署侧的探针组件执行后回报，这样管理进程不需要为每条线路起 goroutine。
func (s *Server) handleMultiWANProbeResult(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Member  string `json:"member"`
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 422, errBody(10367, "invalid body", err.Error()))
		return
	}
	if body.Member == "" {
		writeJSON(w, 422, errBody(10368, "缺少 member 参数", ""))
		return
	}
	rawP, _ := s.store.Get("multiwan", wanProbeKey)
	var probe multiwan.Probe
	_ = json.Unmarshal([]byte(rawP), &probe)

	rawS, _ := s.store.Get("multiwan", wanStatesKey)
	states := map[string]multiwan.State{}
	if rawS != "" {
		_ = json.Unmarshal([]byte(rawS), &states)
	}
	st, ok := states[body.Member]
	if !ok {
		st = multiwan.State{Name: body.Member, Up: true}
	}
	st = multiwan.Update(st, body.Success, body.Error, time.Now().Unix(), probe)
	states[body.Member] = st

	rawM, _ := s.store.Get("multiwan", wanMembersKey)
	var members []multiwan.Member
	_ = json.Unmarshal([]byte(rawM), &members)
	usable := multiwan.PickMembers(members, states, 0)

	b, _ := json.Marshal(states)
	s.store.Set("multiwan", wanStatesKey, string(b))

	resp := map[string]interface{}{
		"state": st, "usable": usable, "usable_count": len(usable),
	}
	// 全 down 时必须明确说出来：这时候流量无处可走
	if len(usable) == 0 && len(members) > 0 {
		resp["warning"] = "所有 WAN 线路都不可用，流量无法出网。检查物理链路和上游。"
	}
	writeJSON(w, 200, resp)
}
