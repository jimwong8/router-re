package server

import (
	"net/http"
	"strconv"

	"github.com/jimwong/router-re/mgmtd/internal/logcenter"
)

// 分类日志：ARP / DDNS / 拨号 / 推送 / 认证 / DHCP / 系统事件 / webadmin / weblogin。

// auditPathOf 返回当前生效的审计日志路径。
//
// 这个值来自环境变量 MGM_AUDIT_LOG，容器部署时通常不在默认位置，
// 所以必须从实际生效的 Logger 拿，不能写死默认路径——否则 webadmin
// 和 weblogin 两类永远是空的。
func (s *Server) auditPathOf() string {
	if s.audit != nil && s.audit.Path != "" {
		return s.audit.Path
	}
	return logcenter.DefaultAuditPath
}

func (s *Server) handleGetLogCategories(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]interface{}{
		"categories": logcenter.CategorySources(s.auditPathOf()),
	})
}

func (s *Server) handleGetLogsByCategory(w http.ResponseWriter, r *http.Request) {
	cat := r.URL.Query().Get("category")
	limit := 200
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 5000 {
		limit = 5000
	}
	res, err := logcenter.QueryCategory(cat, limit, r.URL.Query().Get("since"), s.auditPathOf())
	if err != nil {
		writeJSON(w, 422, errBody(11400, "log category query failed", err.Error()))
		return
	}
	writeJSON(w, 200, res)
}
