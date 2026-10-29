package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jimwong/router-re/mgmtd/internal/sms"
)

// smsConfig 从 config.Store 读出的短信网关配置（section=sms, option=config）。
type smsConfig struct {
	URL      string            `json:"url"`
	Method   string            `json:"method"`
	BodyTmpl string            `json:"body_template"`
	Headers  map[string]string `json:"headers"`
}

// handleSMSSend 发送短信验证码。POST /api/v1/auth/sms/send  { "phone": "..." }
// 规则：未配置网关返回 503；同手机号 60 秒限发一条；每小时最多 5 次；超限 429。
func (s *Server) handleSMSSend(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Phone string `json:"phone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10497, "invalid body", err.Error()))
		return
	}
	phone := strings.TrimSpace(req.Phone)
	if phone == "" {
		writeJSON(w, 422, errBody(10498, "missing phone", "phone is required"))
		return
	}

	var cfg smsConfig
	if !s.loadJSON(sms.Section(), sms.Option(), &cfg) || cfg.URL == "" {
		writeJSON(w, 503, errBody(10500, "sms not configured", "请先在配置中设置短信网关 url/模板"))
		return
	}

	retry, ok := s.smsStore.CanSend(phone)
	if !ok {
		writeJSON(w, 429, errBody(10499, "rate limited", fmt.Sprintf("请 %s 后再试", retry.Round(time.Second))))
		return
	}

	code := fmt.Sprintf("%06d", s.smsStore.GenerateCode())
	content := fmt.Sprintf("您的验证码是 %s，5 分钟内有效，请勿泄露。", code)

	provider := sms.NewGenericHTTPProvider(cfg.URL, cfg.Method, cfg.BodyTmpl, cfg.Headers)
	if err := provider.Send(phone, content); err != nil {
		status := 502
		var httpErr *sms.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode > 0 {
			status = httpErr.StatusCode
		}
		writeJSON(w, status, errBody(10501, "send failed", err.Error()))
		return
	}

	// 网关发送成功后才落库，失败的发送不占用验证码名额。
	s.smsStore.SaveCode(phone, code)
	writeJSON(w, 200, map[string]interface{}{"sent": true, "phone": phone})
}

// handleSMSVerify 校验验证码。POST /api/v1/auth/sms/verify  { "phone": "...", "code": "..." }
// 校验成功即作废该验证码。
func (s *Server) handleSMSVerify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Phone string `json:"phone"`
		Code  string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10497, "invalid body", err.Error()))
		return
	}
	phone := strings.TrimSpace(req.Phone)
	code := strings.TrimSpace(req.Code)
	if phone == "" || code == "" {
		writeJSON(w, 422, errBody(10498, "missing phone or code", "phone and code are required"))
		return
	}

	ok, reason := s.smsStore.Verify(phone, code)
	if !ok {
		switch reason {
		case "expired":
			writeJSON(w, 422, errBody(10503, "code expired", "验证码已过期，请重新获取"))
		case "mismatch":
			writeJSON(w, 422, errBody(10502, "invalid code", "验证码错误"))
		default:
			writeJSON(w, 422, errBody(10503, "code not found", "未找到该手机号的验证码"))
		}
		return
	}
	writeJSON(w, 200, map[string]interface{}{"verified": true})
}
