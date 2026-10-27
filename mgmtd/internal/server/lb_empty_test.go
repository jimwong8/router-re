package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// handleLBCheck 曾在没有任何 LB 成员时 panic：
//
//	ch.Snapshot()[0].LastCheck   // Snapshot() 返回 make([]Status, 0)，取 [0] 越界
//
// panic 让 HTTP 连接直接断开（客户端看到 RemoteDisconnected），
// 表现为"接口莫名超时"而不是可读错误。由空输入探针抓到。
func TestHandleLBCheckEmptyConfigNoPanic(t *testing.T) {
	s := New(Config{
		Addr:          ":0",
		SecretKey:     []byte("0123456789abcdef0123456789abcdef"),
		AdminUser:     "admin",
		AdminPassword: "pw",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/lb/check", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	// 任何 panic 都会让这个测试失败
	s.handleLBCheck(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("空配置应返回 200，实际 %d，body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"status":[]`) {
		t.Fatalf("空配置应返回空的 status 列表，实际 body=%s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"checked_at":null`) {
		t.Fatalf("无成员时 checked_at 应为 null，实际 body=%s", rec.Body.String())
	}
}
