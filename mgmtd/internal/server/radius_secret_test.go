package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// RADIUS 共享密钥属凭据：GET 只能返回 secret_set 布尔，不能回明文。
// 修复前 viewer（只读角色）GET /api/v1/radius/config 能拿到 secret 明文，
// 连 servers 文件渲染 "<server>\t<secret>" 也一起返回。
func TestRadiusConfigDoesNotLeakSecret(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	admin := loginToken(t, s)

	const secret = "RADIUS_SECRET_xyz789"
	body := `{"server":"10.0.0.9","port":1812,"secret":"` + secret + `","timeout_sec":5}`
	req := httptest.NewRequest("POST", "/api/v1/radius/config", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+admin)
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("设置 radius 配置应 200，得 %d: %s", rr.Code, rr.Body.String())
	}
	// 保存响应本身也不能回显明文
	if strings.Contains(rr.Body.String(), secret) {
		t.Fatalf("POST 响应不应回显明文 secret: %s", rr.Body.String())
	}

	// GET 也不能回明文
	req = httptest.NewRequest("GET", "/api/v1/radius/config", nil)
	req.Header.Set("Authorization", "Bearer "+admin)
	rr = httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("读取 radius 配置应 200，得 %d", rr.Code)
	}
	out := rr.Body.String()
	if strings.Contains(out, secret) {
		t.Fatalf("GET 不应返回明文 secret: %s", out)
	}
	if !strings.Contains(out, `"secret_set":true`) {
		t.Fatalf("应返回 secret_set=true 供前端判断: %s", out)
	}
}

// 只改 server/port 不传 secret 时，已存的 secret 必须保留（不能被清空）。
func TestRadiusConfigKeepsSecretWhenOmitted(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	admin := loginToken(t, s)

	post := func(body string) {
		t.Helper()
		req := httptest.NewRequest("POST", "/api/v1/radius/config", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+admin)
		rr := httptest.NewRecorder()
		s.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("POST %s 应 200，得 %d: %s", body, rr.Code, rr.Body.String())
		}
	}
	post(`{"server":"10.0.0.9","port":1812,"secret":"SECRET_KEEP_ME","timeout_sec":5}`)
	// 不传 secret
	post(`{"server":"10.0.0.10","port":1812,"timeout_sec":5}`)

	// 通过内部 store 断言 secret 仍在
	raw, err := s.store.Get("radius", "config")
	if err != nil {
		t.Fatalf("store get: %v", err)
	}
	if !strings.Contains(raw, "SECRET_KEEP_ME") {
		t.Fatalf("省略 secret 时不应清空已存密钥，实际存储: %s", raw)
	}
	if !strings.Contains(raw, "10.0.0.10") {
		t.Fatalf("server 应被更新，实际存储: %s", raw)
	}
}
