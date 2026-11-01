package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/tailscale"
)

const tailscaleStoreKey = "tailscale"

func (s *Server) loadTailscale() tailscale.Config {
	cfg := tailscale.Default()
	if raw, _ := s.store.Get("vpn", tailscaleStoreKey); raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	return cfg
}

// handleGetTailscale 返回配置 + 真实状态 + 能力探测。
//
// 三个能力必须分别给：二进制在不在、tailscaled 的 socket 在不在、这个 socket 是不是系统自身的。
// 少了任何一项，用户在"命令没装"和"守护进程没起"之间就只能靠猜。
func (s *Server) handleGetTailscale(w http.ResponseWriter, r *http.Request) {
	cfg := s.loadTailscale()
	out := cfg
	keySet := cfg.AuthKey != ""
	out.AuthKey = ""
	writeJSON(w, 200, map[string]interface{}{
		"config":           out,
		"key_set":          keySet,
		"status":           tailscale.FetchStatus(),
		"binary_exists":    tailscale.BinaryAvailable(),
		"socket":           tailscale.SocketPath(),
		"socket_exists":    tailscale.SocketExists(),
		"system_socket":    tailscale.IsDefaultSocket(),
		"up_args_preview":  tailscale.MaskArgs(tailscale.BuildUpArgs(cfg)),
	})
}

// handleSetTailscale 保存并立即生效；auth key 留空时沿用已保存的（除非本来就是空）。
func (s *Server) handleSetTailscale(w http.ResponseWriter, r *http.Request) {
	var cfg tailscale.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, 400, errBody(12020, "json decode fail", err.Error()))
		return
	}
	old := s.loadTailscale()
	if strings.TrimSpace(cfg.AuthKey) == "" {
		cfg.AuthKey = old.AuthKey
	}
	if err := tailscale.Validate(cfg); err != nil {
		writeJSON(w, 422, errBody(12021, "tailscale config invalid", err.Error()))
		return
	}

	applyErr := tailscale.Apply(cfg)
	// 失败也保存：参数是用户填的，配上真实状态更容易判断下一步。
	if b, err := json.Marshal(cfg); err == nil {
		if serr := s.store.Set("vpn", tailscaleStoreKey, string(b)); serr != nil && applyErr == nil {
			writeJSON(w, 500, errBody(12022, "save config failed", serr.Error()))
			return
		}
	}
	st := tailscale.FetchStatus()
	out := cfg
	out.AuthKey = ""
	args := tailscale.MaskArgs(tailscale.BuildUpArgs(cfg))
	if !cfg.Enabled {
		args = []string{"down"}
	}
	resp := map[string]interface{}{
		"config":    out,
		"key_set":   cfg.AuthKey != "",
		"status":    st,
		"applied":   applyErr == nil,
		"up_args":   args,
	}
	if applyErr != nil {
		resp["error"] = applyErr.Error()
	}
	writeJSON(w, 200, resp)
}

// handleTailscaleStatus 只读状态，供页面轮询。
func (s *Server) handleTailscaleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]interface{}{
		"status":        tailscale.FetchStatus(),
		"socket":        tailscale.SocketPath(),
		"system_socket": tailscale.IsDefaultSocket(),
	})
}

// handleTailscaleDown 断开本节点。
//
// 注意：如果管理页面本身是经 Tailscale 访问的，这一步会切断自己的连接。
// 前端在 system_socket 为真时要求二次确认。已经断开时不当作错误。
func (s *Server) handleTailscaleDown(w http.ResponseWriter, r *http.Request) {
	if err := tailscale.Down(); err != nil {
		writeJSON(w, 500, errBody(12023, "tailscale down failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"down":   true,
		"status": tailscale.FetchStatus(),
	})
}

// handleTailscalePreview 只回参数，不动守护进程。auth key 打码。
func (s *Server) handleTailscalePreview(w http.ResponseWriter, r *http.Request) {
	var cfg tailscale.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, 400, errBody(12024, "json decode fail", err.Error()))
		return
	}
	if strings.TrimSpace(cfg.AuthKey) == "" {
		cfg.AuthKey = s.loadTailscale().AuthKey
	}
	if err := tailscale.Validate(cfg); err != nil {
		writeJSON(w, 422, errBody(12025, "tailscale config invalid", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"args":   tailscale.MaskArgs(tailscale.BuildUpArgs(cfg)),
		"socket": tailscale.SocketPath(),
	})
}
