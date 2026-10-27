package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 缺 name 曾被当成服务端故障：直接拿空字符串去查 DNS，失败后返回 500
// 「dns lookup failed」。参数校验问题必须返回 4xx，否则监控里会误报成服务挂了。
func TestHandleDiagDNSMissingName(t *testing.T) {
	s := New(Config{
		Addr:          ":0",
		SecretKey:     []byte("0123456789abcdef0123456789abcdef"),
		AdminUser:     "admin",
		AdminPassword: "pw",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/diag/dns", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	s.handleDiagDNS(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("缺 name 应返回 422，实际 %d，body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "10142") {
		t.Fatalf("错误码应为 10142，实际 body=%s", rec.Body.String())
	}
}
