package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 三个"全量替换"端点曾对空数组一律返回 200 并清空配置：
//
//	POST /api/v1/alerts/rules     {rules:[]}   -> 清空所有告警规则
//	POST /api/v1/l7/rules         {rules:[]}   -> 清空所有 L7 规则
//	POST /api/v1/routing/tables   {tables:[]}  -> 清空所有策略路由表
//
// 任何客户端误发空 body 就会静默清空，且返回成功。
// 现在要求显式 confirm:true，与 timeschedule 的做法对齐。
func TestEmptyArrayRequiresConfirm(t *testing.T) {
	s := New(Config{
		Addr:          ":0",
		SecretKey:     []byte("0123456789abcdef0123456789abcdef"),
		AdminUser:     "admin",
		AdminPassword: "pw",
	})

	cases := []struct {
		name    string
		handler http.HandlerFunc
		body    string
		wantOK  bool
		code    string
	}{
		{"alerts 空数组被拒", s.handleSetAlertRules, `{"rules":[]}`, false, "10212"},
		{"alerts confirm 放行", s.handleSetAlertRules, `{"rules":[],"confirm":true}`, true, ""},
		{"l7 空数组被拒", s.handleSetL7Rules, `{"rules":[]}`, false, "10153"},
		{"l7 confirm 放行", s.handleSetL7Rules, `{"rules":[],"confirm":true}`, true, ""},
		{"tables 空数组被拒", s.handleSetPolicyTables, `{"tables":[]}`, false, "10318"},
		{"tables confirm 放行", s.handleSetPolicyTables, `{"tables":[],"confirm":true}`, true, ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(c.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c.handler(rec, req)

			if c.wantOK {
				if rec.Code != http.StatusOK {
					t.Fatalf("应放行，实际 %d body=%s", rec.Code, rec.Body.String())
				}
				return
			}
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("应返回 422，实际 %d body=%s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), c.code) {
				t.Fatalf("错误码应含 %s，实际 %s", c.code, rec.Body.String())
			}
		})
	}
}
