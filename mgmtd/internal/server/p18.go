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

	states := s.loadLinkCheckStates()
	old := states[body.ID]
	newState, _ := linkcheck.Step(old, ok, time.Now().Unix(), tgt)
	states[body.ID] = newState
	if err := s.saveLinkCheckStates(states); err != nil {
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
