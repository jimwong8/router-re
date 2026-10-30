package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/plugin"
)

// 应用商店 / 插件管理。
//
// 和 iKuai 的"插件管理 + Docker + 应用商店"对应，但这里的边界是明确的：
// 只管理"现场已经装好二进制的插件"的启停与自启，不假装能联网下载。
// 没装就显示"未安装"并给出项目地址，让人照着装——比一个转圈然后失败强。

func (s *Server) pluginMgr() *plugin.Manager {
	dir := os.Getenv("MGM_PLUGIN_DIR")
	if dir == "" {
		dir = plugin.StateDir
	}
	return plugin.NewManager(dir)
}

func (s *Server) handleListPlugins(w http.ResponseWriter, r *http.Request) {
	m := s.pluginMgr()
	writeJSON(w, 200, map[string]interface{}{
		"plugins":   m.List(),
		"conflicts": plugin.PortConflicts(),
		"state_dir": m.Dir,
		"note":      "这里只管理已安装二进制的插件的启停与自启；未安装的条目请按项目地址自行安装后刷新。",
	})
}

func (s *Server) handlePluginAction(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Action string `json:"action"` // enable / disable / start / stop
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, errBody(11500, "json decode fail", err.Error()))
		return
	}
	m := s.pluginMgr()
	p, ok := plugin.Find(id)
	if !ok {
		writeJSON(w, 422, errBody(11501, "unknown plugin", "未知插件："+id))
		return
	}

	resp := map[string]interface{}{"id": id, "action": body.Action, "ok": false}
	switch body.Action {
	case "enable":
		if err := m.SetEnabled(id, true); err != nil {
			writeJSON(w, 500, errBody(11502, "enable failed", err.Error()))
			return
		}
		resp["ok"] = true
		resp["note"] = "已设置开机自启（不会立即启动，需要点启动）"
	case "disable":
		if err := m.SetEnabled(id, false); err != nil {
			writeJSON(w, 500, errBody(11503, "disable failed", err.Error()))
			return
		}
		resp["ok"] = true
	case "start":
		pid, err := m.Start(id)
		if err != nil {
			// 已安装但启动失败是真实结果，如实返回，不谎报成功
			writeJSON(w, 422, errBody(11504, "start failed", err.Error()))
			return
		}
		resp["ok"] = true
		resp["pid"] = pid
		resp["note"] = "进程已拉起，日志在 " + filepath.Join(m.Dir, id+".log")
	case "stop":
		if err := m.Stop(id); err != nil {
			writeJSON(w, 422, errBody(11505, "stop failed", err.Error()))
			return
		}
		resp["ok"] = true
	default:
		writeJSON(w, 422, errBody(11506, "unknown action", "只支持 enable/disable/start/stop"))
		return
	}
	// 返回最新状态，界面不用再猜
	resp["status"] = m.StatusOf(p)
	writeJSON(w, 200, resp)
}

// handlePluginLog 返回插件日志尾部，便于"点了启动但没起来"时直接看原因。
func (s *Server) handlePluginLog(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := plugin.Find(id); !ok {
		writeJSON(w, 422, errBody(11507, "unknown plugin", "未知插件："+id))
		return
	}
	m := s.pluginMgr()
	path := filepath.Join(m.Dir, id+".log")
	data, err := os.ReadFile(path)
	if err != nil {
		writeJSON(w, 200, map[string]interface{}{
			"id": id, "path": path, "lines": []string{},
			"note": "还没有日志（插件未启动过）",
		})
		return
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) > 200 {
		lines = lines[len(lines)-200:]
	}
	writeJSON(w, 200, map[string]interface{}{"id": id, "path": path, "lines": lines})
}

// handlePluginVersion 单独取版本号。列表页不探测版本，避免任何一条外部
// 命令把 /plugins 拖死。
func (s *Server) handlePluginVersion(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ver, err := s.pluginMgr().Version(id)
	if err != nil {
		writeJSON(w, 422, errBody(11508, "version probe failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{"id": id, "version": ver})
}
