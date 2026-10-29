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
	cfg.AppID = strings.TrimSpace(cfg.AppID)
	cfg.AppSecret = strings.TrimSpace(cfg.AppSecret)

	if cfg.AppID != "" && !validWechatAppID(cfg.AppID) {
		writeJSON(w, 422, errBody(10502, "invalid appid", "appid 形如 wx 开头、共 18 位"))
		return
	}

	// GET 返回的是掩码（ab****yz）。如果前端把 GET 的结果原样 POST 回来，
	// 这里绝不能把掩码当成新 secret 落库——否则真 secret 被覆盖成 "ab****yz"，
	// 而 secret_set 仍然是 true，之后每次登录都失败且看不出是配置问题。
	var cur wechatConfig
	s.loadJSON("wechat", "config", &cur)
	if cfg.AppSecret == "" || cfg.AppSecret == maskSecret(cur.AppSecret) {
		cfg.AppSecret = cur.AppSecret
	}

	b, _ := json.Marshal(cfg)
	if err := s.store.Set("wechat", "config", string(b)); err != nil {
		writeJSON(w, 500, errBody(10435, "save failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"ok":         true,
		"appid":      cfg.AppID,
		"secret_set": cfg.AppSecret != "",
	})
}

// validWechatAppID 粗校验：微信 appid 为 wx + 16 位字母数字。
// 只挡明显的手误（把 secret 填进 appid、少几位），不追求严格。
func validWechatAppID(id string) bool {
	if len(id) != 18 || !strings.HasPrefix(id, "wx") {
		return false
	}
	for _, c := range id[2:] {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
			return false
		}
	}
	return true
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
		writeJSON(w, 409, errBody(10500, "wechat not configured", "请先配置 appid/appsecret"))
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
