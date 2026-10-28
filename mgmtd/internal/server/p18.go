package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
	"github.com/jimwong/router-re/mgmtd/internal/linkcheck"
)

// ---------- 链路检测 ----------

func (s *Server) loadLinkCheckTargets() []linkcheck.Target {
	raw, err := s.store.Get("linkcheck", "targets")
	if err != nil || strings.TrimSpace(raw) == "" {
		return []linkcheck.Target{}
	}
	var targets []linkcheck.Target
	if err := json.Unmarshal([]byte(raw), &targets); err != nil {
		return []linkcheck.Target{}
	}
	if targets == nil {
		targets = []linkcheck.Target{}
	}
	return targets
}

func (s *Server) saveLinkCheckTargets(targets []linkcheck.Target) error {
	if targets == nil {
		targets = []linkcheck.Target{}
	}
	b, err := json.Marshal(targets)
	if err != nil {
		return err
	}
	return s.store.Set("linkcheck", "targets", string(b))
}

func (s *Server) loadLinkCheckStates() map[string]linkcheck.State {
	raw, err := s.store.Get("linkcheck", "states")
	if err != nil || strings.TrimSpace(raw) == "" {
		return map[string]linkcheck.State{}
	}
	var states map[string]linkcheck.State
	if err := json.Unmarshal([]byte(raw), &states); err != nil {
		return map[string]linkcheck.State{}
	}
	if states == nil {
		states = map[string]linkcheck.State{}
	}
	return states
}

func (s *Server) saveLinkCheckStates(states map[string]linkcheck.State) error {
	if states == nil {
		states = map[string]linkcheck.State{}
	}
	b, err := json.Marshal(states)
	if err != nil {
		return err
	}
	return s.store.Set("linkcheck", "states", string(b))
}

// applyLinkCheckProbe 在锁内完成「读 states → 按状态机推进一条 → 写回」。
//
// 为什么必须整段持锁：saveLinkCheckStates 写的是整个 map。两个目标并发探测时，
// 都读到同一份快照，后写的那个把前一个刚写进去的整条覆盖掉。实测 32 并发只剩
// 2 条（丢 94%）。旧值也必须在锁内读，否则同一目标的两次探测会互相覆盖状态机。
func (s *Server) applyLinkCheckProbe(id string, ok bool, now int64, tgt linkcheck.Target) (linkcheck.State, error) {
	s.linkcheckMu.Lock()
	defer s.linkcheckMu.Unlock()

	states := s.loadLinkCheckStates()
	newState, _ := linkcheck.Step(states[id], ok, now, tgt)
	states[id] = newState
	return newState, s.saveLinkCheckStates(states)
}

// handleListLinkCheck GET — 返回 targets + states。
func (s *Server) handleListLinkCheck(w http.ResponseWriter, r *http.Request) {
	targets := s.loadLinkCheckTargets()
	states := s.loadLinkCheckStates()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count":   len(targets),
		"targets": targets,
		"states":  states,
	})
}

// handleSetLinkCheck POST — 校验并保存 targets。
func (s *Server) handleSetLinkCheck(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Targets []linkcheck.Target `json:"targets"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(10452, "invalid JSON body", err.Error()))
		return
	}
	if len(body.Targets) == 0 {
		writeJSON(w, http.StatusUnprocessableEntity, errBody(10450, "targets must not be empty",
			"要停用请把 enabled 设为 false，而不是清空列表"))
		return
	}
	for i, t := range body.Targets {
		if err := linkcheck.Validate(t); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, errBody(10451, "target "+strconv.Itoa(i)+" invalid", err.Error()))
			return
		}
	}
	if err := s.saveLinkCheckTargets(body.Targets); err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(10456, "save failed", err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count":   len(body.Targets),
		"targets": body.Targets,
	})
}

// handleProbeLink POST — 对单个 target 执行探测并推进状态机。
func (s *Server) handleProbeLink(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(10452, "invalid JSON body", err.Error()))
		return
	}
	targets := s.loadLinkCheckTargets()
	idx := -1
	for i, t := range targets {
		if t.ID == body.ID {
			idx = i
			break
		}
	}
	if idx < 0 {
		writeJSON(w, http.StatusNotFound, errBody(10453, "target not found", "no linkcheck target with id "+body.ID))
		return
	}
	tgt := targets[idx]
	args, err := linkcheck.ProbeArgs(tgt)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, errBody(10454, "cannot build probe args", err.Error()))
		return
	}
	cmdName := ""
	switch tgt.Kind {
	case "icmp":
		cmdName = "ping"
	case "dns":
		cmdName = "nslookup"
	case "http":
		cmdName = "curl"
	default:
		writeJSON(w, http.StatusUnprocessableEntity, errBody(10454, "unknown kind", tgt.Kind))
		return
	}
	out, runErr := (execx.DefaultRunner{}).Run(cmdName, args...)
	ok, reason := linkcheck.ParseProbeOK(tgt, out, runErr)

	newState, stateErr := s.applyLinkCheckProbe(body.ID, ok, time.Now().Unix(), tgt)
	if stateErr != nil {
		// 不阻断返回，但记录
	}

	output := out
	runes := []rune(output)
	if len(runes) > 400 {
		output = string(runes[:400])
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":     body.ID,
		"ok":     ok,
		"reason": reason,
		"state":  newState,
		"output": strings.TrimSpace(output),
	})
}

// handleLinkCheckStatus GET — 返回 up/down 统计。
func (s *Server) handleLinkCheckStatus(w http.ResponseWriter, r *http.Request) {
	targets := s.loadLinkCheckTargets()
	states := s.loadLinkCheckStates()
	up, down := 0, 0
	for _, st := range states {
		if st.Up {
			up++
		} else {
			down++
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count":  len(targets),
		"up":     up,
		"down":   down,
		"states": states,
	})
}

// handleDeleteLinkCheckTarget DELETE /api/v1/linkcheck/targets/{id}
// 从存储删除指定 id 的 target（同时清理其状态），id 不存在返回 200 deleted=false。
func (s *Server) handleDeleteLinkCheckTarget(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	targets := s.loadLinkCheckTargets()
	found := false
	filtered := make([]linkcheck.Target, 0, len(targets))
	for _, t := range targets {
		if t.ID == id {
			found = true
			continue
		}
		filtered = append(filtered, t)
	}
	if found {
		// 与探测共用一把锁：删除时读 states → 删一条 → 写回，同样不能让并发探测
		// 的整份写把刚删掉的 id 复活（或把别人的状态覆盖掉）。
		s.linkcheckMu.Lock()
		err := s.saveLinkCheckTargets(filtered)
		if err == nil {
			states := s.loadLinkCheckStates()
			delete(states, id)
			err = s.saveLinkCheckStates(states)
		}
		s.linkcheckMu.Unlock()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errBody(10482, "save failed", err.Error()))
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"deleted": found})
}
