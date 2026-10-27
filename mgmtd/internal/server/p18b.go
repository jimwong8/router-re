package server

import (
	"encoding/json"
	"net/http"

	"github.com/jimwong/router-re/mgmtd/internal/authportal"
)

// ---------- 认证门户模板 ----------

// loadPortalTemplate 读取已保存的门户模板；未配置时回退到内置第一个。
func (s *Server) loadPortalTemplate() authportal.Template {
	raw, _ := s.store.Get("portal", "template")
	if raw != "" {
		var t authportal.Template
		if err := json.Unmarshal([]byte(raw), &t); err == nil {
			if authportal.Validate(t) == nil {
				return t
			}
		}
	}
	builtin := authportal.Builtin()
	if len(builtin) > 0 {
		return builtin[0]
	}
	return authportal.Template{}
}

// handleListPortalTemplates GET /api/v1/portal/templates
func (s *Server) handleListPortalTemplates(w http.ResponseWriter, r *http.Request) {
	current := s.loadPortalTemplate()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count":   len(authportal.Builtin()),
		"builtin": authportal.Builtin(),
		"current": current,
	})
}

// handleSetPortalTemplate POST /api/v1/portal/templates
func (s *Server) handleSetPortalTemplate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Template authportal.Template `json:"template"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(10460, "invalid body", err.Error()))
		return
	}
	if err := authportal.Validate(body.Template); err != nil {
		// javascript:/data:/vbscript: 伪协议、非法颜色等都是 XSS 入口，必须拒绝并说明原因
		writeJSON(w, http.StatusUnprocessableEntity, errBody(10461, "invalid template", err.Error()))
		return
	}
	b, err := json.Marshal(body.Template)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(10462, "marshal failed", err.Error()))
		return
	}
	if err := s.store.Set("portal", "template", string(b)); err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(10463, "save failed", err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"saved":    true,
		"template": body.Template,
	})
}

// handlePreviewPortal GET /api/v1/portal/preview?title=...&kind=...&notice=...&redirect_url=...
// 唯一返回 HTML 的端点；收窄 CSP 防止注入脚本执行。
func (s *Server) handlePreviewPortal(w http.ResponseWriter, r *http.Request) {
	t := s.loadPortalTemplate()
	q := r.URL.Query()
	if v := q.Get("title"); v != "" {
		t.Title = v
	}
	if v := q.Get("kind"); v != "" {
		t.Kind = v
	}
	if v := q.Get("notice"); v != "" {
		t.Notice = v
	}
	if v := q.Get("redirect_url"); v != "" {
		t.RedirectURL = v
	}
	// 校验失败也渲染（预览用于调试），但 XSS 伪协议字段仍按 ValidateRedirect 拒绝
	if err := authportal.Validate(t); err != nil {
		// 仍返回 422 JSON，避免把不可信内容以 HTML 形式输出
		writeJSON(w, http.StatusUnprocessableEntity, errBody(10464, "invalid template", err.Error()))
		return
	}
	html := authportal.RenderHTML(t)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(html))
}
