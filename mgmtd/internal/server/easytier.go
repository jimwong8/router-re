package server

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/easytier"
)

const easyTierStoreKey = "easytier"

// loadEasyTier 从 store 读配置；没有时返回默认值。
func (s *Server) loadEasyTier() easytier.Config {
	cfg := easytier.Default()
	if raw, _ := s.store.Get("vpn", easyTierStoreKey); raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	return cfg
}

// handleGetEasyTierConfig 返回配置 + 运行态 + 能力探测。
//
// 能力必须提前给：容器里没有 /dev/net/tun 时 EasyTier 建不了 TUN，
// 界面要能直接告诉用户"只能当中继"，而不是等点了启用再看进程退出。
func (s *Server) handleGetEasyTierConfig(w http.ResponseWriter, r *http.Request) {
	cfg := s.loadEasyTier()
	out := cfg
	secretSet := cfg.NetworkSecret != ""
	out.NetworkSecret = "" // 密钥永不回显
	// 非 root 时即使 /dev/net/tun 可读也建不出接口（TUNSETIFF 需要 CAP_NET_ADMIN），
	// 只报 tun=true 会让用户点了启用才吃到权限错误。
	privileged := os.Geteuid() == 0
	writeJSON(w, 200, map[string]interface{}{
		"config":        out,
		"secret_set":    secretSet,
		"status":        easytier.CurrentStatus(),
		"tun":           tunUsable() && privileged,
		"privileged":    privileged,
		"binary_exists": easytier.BinaryAvailable(),
		"version":       easytier.Version(),
		"config_path":   easytier.ConfigPath(),
	})
}

// handleSetEasyTierConfig 保存并立即生效。
// 密钥留空 = 沿用已保存的（改个实例名不该逼用户重输密钥）。
func (s *Server) handleSetEasyTierConfig(w http.ResponseWriter, r *http.Request) {
	var cfg easytier.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, 400, errBody(12010, "json decode fail", err.Error()))
		return
	}
	old := s.loadEasyTier()
	if strings.TrimSpace(cfg.NetworkSecret) == "" {
		cfg.NetworkSecret = old.NetworkSecret
	}
	if err := easytier.Validate(cfg); err != nil {
		writeJSON(w, 422, errBody(12011, "easytier config invalid", err.Error()))
		return
	}

	applyErr := easytier.Apply(cfg)
	// 即使启动失败也保存配置：用户填的东西不该因为二进制没装就丢掉。
	if b, err := json.Marshal(cfg); err == nil {
		if serr := s.store.Set("vpn", easyTierStoreKey, string(b)); serr != nil && applyErr == nil {
			writeJSON(w, 500, errBody(12012, "save config failed", serr.Error()))
			return
		}
	}
	out := cfg
	out.NetworkSecret = ""
	resp := map[string]interface{}{
		"config":     out,
		"secret_set": cfg.NetworkSecret != "",
		"status":     easytier.CurrentStatus(),
		"applied":    applyErr == nil,
		"rendered":   easytier.RenderConfig(cfg),
	}
	if applyErr != nil {
		resp["error"] = applyErr.Error()
	}
	writeJSON(w, 200, resp)
}

// handleEasyTierStatus 返回运行态 + 对等节点列表。
// 未运行时 peers 必须是 []（不是 null）——前端 v-for 拿到 null 会报错。
func (s *Server) handleEasyTierStatus(w http.ResponseWriter, r *http.Request) {
	cfg := s.loadEasyTier()
	st := easytier.CurrentStatus()
	peers, running, err := easytier.Peers(cfg.RPCPortal)
	if peers == nil {
		peers = []easytier.Peer{}
	}
	resp := map[string]interface{}{
		"status":      st,
		"running":     running,
		"peers":       peers,
		"count":       len(peers),
		"rpc_portal":  cfg.RPCPortal,
		"config_path": easytier.ConfigPath(),
	}
	if err != nil {
		resp["error"] = err.Error()
	}
	writeJSON(w, 200, resp)
}

// handleEasyTierStop 停止实例，保留配置。
func (s *Server) handleEasyTierStop(w http.ResponseWriter, r *http.Request) {
	if err := easytier.Stop(); err != nil {
		writeJSON(w, 500, errBody(12013, "stop failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"stopped": true,
		"status":  easytier.CurrentStatus(),
	})
}

// handleEasyTierPreview 只渲染配置文本，不落盘、不启动——给用户核对用。
func (s *Server) handleEasyTierPreview(w http.ResponseWriter, r *http.Request) {
	var cfg easytier.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, 400, errBody(12014, "json decode fail", err.Error()))
		return
	}
	if strings.TrimSpace(cfg.NetworkSecret) == "" {
		cfg.NetworkSecret = s.loadEasyTier().NetworkSecret
	}
	if err := easytier.Validate(cfg); err != nil {
		writeJSON(w, 422, errBody(12015, "easytier config invalid", err.Error()))
		return
	}
	text := easytier.RenderConfig(cfg)
	// 预览里把密钥打码，避免它出现在浏览器历史/截图里
	if cfg.NetworkSecret != "" {
		text = strings.ReplaceAll(text, cfg.NetworkSecret, "********")
	}
	writeJSON(w, 200, map[string]interface{}{"config": text})
}
