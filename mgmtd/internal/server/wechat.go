package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/wechat"
)

type wechatConfig struct {
	AppID     string `json:"appid"`
	AppSecret string `json:"appsecret"`
}

func (s *Server) handleGetWechatConfig(w http.ResponseWriter, r *http.Request) {
	var cfg wechatConfig
	s.loadJSON("wechat", "config", &cfg)
	writeJSON(w, 200, map[string]interface{}{
		"appid":      cfg.AppID,
		"secret_set": cfg.AppSecret != "",
		"appsecret":  maskSecret(cfg.AppSecret),
	})
}

func (s *Server) handleSetWechatConfig(w http.ResponseWriter, r *http.Request) {
	var cfg wechatConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, 400, errBody(10497, "invalid body", err.Error()))
		return
	}
	b, _ := json.Marshal(cfg)
	s.store.Set("wechat", "config", string(b))
	writeJSON(w, 200, map[string]interface{}{"ok": true})
}

func (s *Server) handleWechatLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10497, "invalid body", err.Error()))
		return
	}
	var cfg wechatConfig
	if !s.loadJSON("wechat", "config", &cfg) || cfg.AppID == "" || cfg.AppSecret == "" {
		writeJSON(w, 503, errBody(10500, "wechat not configured", "请先配置 appid/appsecret"))
		return
	}
	client := wechat.NewClient(cfg.AppID, cfg.AppSecret)
	sess, err := client.Code2Session(strings.TrimSpace(req.Code))
	if err != nil {
		status := 502
		if we, ok := err.(*wechat.ErrCodeError); ok {
			writeJSON(w, status, map[string]interface{}{
				"error":   we.Error(),
				"errcode": we.ErrCode,
				"errmsg":  we.ErrMsg,
			})
			return
		}
		writeJSON(w, status, errBody(10501, "wechat failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"openid":     sess.OpenID,
		"authorized": true,
	})
}

func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 4 {
		return "****"
	}
	return s[:2] + "****" + s[len(s)-2:]
}
