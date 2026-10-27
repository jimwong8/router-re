package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
	"github.com/jimwong/router-re/mgmtd/internal/remote"
)

// wolTargetsPath 是 WOL 目标的持久化路径。默认放配置目录；MGM_CONFIG_FILE 未设置时
// 用 mock store，不落盘，因此这里也允许通过环境变量覆盖（测试/部署可指定）。
func wolTargetsPath() string {
	if p := os.Getenv("MGM_WOL_TARGETS"); p != "" {
		return p
	}
	dir := os.Getenv("MGM_CONFIG_DIR")
	if dir == "" {
		dir = "/etc/router-re"
	}
	return filepath.Join(dir, "wol_targets.json")
}

// --- 远程访问 ---

// handleGetRemoteAccess 返回当前保存的远程访问配置（不落盘实时，读 store）。
func (s *Server) handleGetRemoteAccess(w http.ResponseWriter, r *http.Request) {
	cfg := remote.Access{}
	raw, _ := s.store.Get("remote", "access")
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	writeJSON(w, 200, map[string]interface{}{
		"access":   cfg,
		"warnings": remote.AccessWarnings(cfg),
	})
}

// handleSetRemoteAccess 校验、持久化并应用远程访问配置。POST 返回
// {applied, pending_manual, access, warnings}。应用是"尽力而为"——端口改动只回
// 提示不自动改文件，见 remote.ApplyAccess。
func (s *Server) handleSetRemoteAccess(w http.ResponseWriter, r *http.Request) {
	var cfg remote.Access
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, 400, errBody(10140, "json decode fail", err.Error()))
		return
	}
	if err := remote.ValidateAccess(cfg); err != nil {
		writeJSON(w, 422, errBody(10141, "validation failed", err.Error()))
		return
	}
	b, _ := json.Marshal(cfg)
	_ = s.store.Set("remote", "access", string(b))

	pending, err := remote.ApplyAccess(execx.DefaultRunner{}, cfg)
	res := map[string]interface{}{
		"access":         cfg,
		"applied":        err == nil,
		"pending_manual": pending,
		"warnings":       remote.AccessWarnings(cfg),
	}
	if err != nil {
		res["reason"] = err.Error()
	}
	writeJSON(w, 200, res)
}

// handleRemoteAccessStatus 返回 SSH/Telnet 运行状态。
func (s *Server) handleRemoteAccessStatus(w http.ResponseWriter, r *http.Request) {
	st, err := remote.AccessStatus(execx.DefaultRunner{})
	if err != nil {
		writeJSON(w, 500, errBody(10142, "remote access status failed", err.Error()))
		return
	}
	writeJSON(w, 200, st)
}

// --- 系统时间 ---

// handleGetSystemTime 返回当前保存的时间配置与实时状态。
func (s *Server) handleGetSystemTime(w http.ResponseWriter, r *http.Request) {
	cfg := remote.TimeConfig{}
	raw, _ := s.store.Get("system", "time")
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	status, _ := remote.TimeStatus(execx.DefaultRunner{})
	writeJSON(w, 200, map[string]interface{}{
		"config": cfg,
		"status": status,
	})
}

// handleSetSystemTime 校验、持久化并应用时间配置。POST 返回 {applied, time, status}。
func (s *Server) handleSetSystemTime(w http.ResponseWriter, r *http.Request) {
	var cfg remote.TimeConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, 400, errBody(10143, "json decode fail", err.Error()))
		return
	}
	if err := remote.ValidateTime(cfg); err != nil {
		writeJSON(w, 422, errBody(10144, "validation failed", err.Error()))
		return
	}
	b, _ := json.Marshal(cfg)
	_ = s.store.Set("system", "time", string(b))

	err := remote.ApplyTime(execx.DefaultRunner{}, cfg)
	status, _ := remote.TimeStatus(execx.DefaultRunner{})
	res := map[string]interface{}{
		"applied": err == nil,
		"time":    cfg,
		"status":  status,
	}
	if err != nil {
		// 应用失败也返回 200 但带 reason，前端据此提示。
		res["reason"] = err.Error()
	}
	writeJSON(w, 200, res)
}

// handleListTimezones 返回可用时区列表。
func (s *Server) handleListTimezones(w http.ResponseWriter, r *http.Request) {
	zones, err := remote.ListTimezones(execx.DefaultRunner{})
	if err != nil {
		writeJSON(w, 500, errBody(10145, "list timezones failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"timezones": zones,
		"count":     len(zones),
	})
}

// --- 网络唤醒 ---

// handleGetWOLTargets 返回持久化的 WOL 目标。
func (s *Server) handleGetWOLTargets(w http.ResponseWriter, r *http.Request) {
	targets, err := remote.LoadTargets(wolTargetsPath())
	if err != nil {
		writeJSON(w, 500, errBody(10146, "load wol targets failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"targets": targets,
		"count":   len(targets),
	})
}

// handleSetWOLTargets 校验并持久化 WOL 目标列表。
func (s *Server) handleSetWOLTargets(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Targets []remote.WOLTarget `json:"targets"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10147, "json decode fail", err.Error()))
		return
	}
	for _, t := range req.Targets {
		if err := remote.ValidateWOL(t); err != nil {
			writeJSON(w, 422, errBody(10148, "validation failed", err.Error()))
			return
		}
	}
	if err := remote.SaveTargets(wolTargetsPath(), req.Targets); err != nil {
		writeJSON(w, 500, errBody(10149, "save wol targets failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"targets": req.Targets,
		"count":   len(req.Targets),
	})
}

// handleWOLWake 按 name 或 {mac,iface} 唤醒。POST /wol/wake。
func (s *Server) handleWOLWake(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name  string `json:"name"`
		MAC   string `json:"mac"`
		Iface string `json:"iface"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10150, "json decode fail", err.Error()))
		return
	}
	target := remote.WOLTarget{Name: req.Name, MAC: req.MAC, Iface: req.Iface}
	// 若只给了 name，从已保存目标里解析 MAC/Iface。
	if req.MAC == "" && req.Name != "" {
		saved, _ := remote.LoadTargets(wolTargetsPath())
		for _, t := range saved {
			if t.Name == req.Name {
				target = t
				break
			}
		}
	}
	if err := remote.ValidateWOL(target); err != nil {
		writeJSON(w, 422, errBody(10151, "validation failed", err.Error()))
		return
	}
	if err := remote.Wake(execx.DefaultRunner{}, target); err != nil {
		writeJSON(w, 500, errBody(10152, "wake failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"woken": target.Name,
		"mac":   target.MAC,
		"iface": target.Iface,
	})
}
