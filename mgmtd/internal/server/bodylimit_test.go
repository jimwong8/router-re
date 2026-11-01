package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 请求体必须有上限：路由器内存有限，已鉴权不代表可信。
// 一个愿意发 1GB POST 的低权限账号（或泄漏的 token）就能把进程撑爆。
// 这里对一个曾经无上限读 body 的端点验证：超限时必须快速失败，而不是照单全收。
func TestJSONBodyLimitRejectsOversized(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	tok := loginToken(t, s)

	// 构造超过 maxJSONBody 的请求体
	big := `{"x":"` + strings.Repeat("A", maxJSONBody+1024) + `"}`
	req := httptest.NewRequest("POST", "/api/v1/vlan", strings.NewReader(big))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)

	if rr.Code == http.StatusOK {
		t.Fatalf("超大请求体被接受了（HTTP 200），上限没生效")
	}
	// 具体码取决于实现：MaxBytesReader 触发后 handler 返回 400；关键是必须拒绝
	if rr.Code != http.StatusBadRequest && rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("超大请求体期望 400/413，得到 %d: %s", rr.Code, rr.Body.String())
	}
	// 错误信息应指向"body too large"，证明是上限拦下的，而非业务校验
	if !strings.Contains(rr.Body.String(), "too large") {
		t.Fatalf("拒绝原因不像超限: %s", rr.Body.String())
	}
}

// 未超限的请求不能被误伤：正常大小仍应进入业务逻辑（这里因为缺字段返回校验错误，但不是"too large"）。
func TestJSONBodyLimitAllowsNormalSize(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	tok := loginToken(t, s)

	req := httptest.NewRequest("POST", "/api/v1/vlan", strings.NewReader(`{"vlans":[]}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)

	if strings.Contains(rr.Body.String(), "too large") {
		t.Fatalf("正常大小请求被误判超限: %s", rr.Body.String())
	}
}
