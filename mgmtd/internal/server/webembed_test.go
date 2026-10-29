package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newWebTestServer() *Server {
	return New(Config{
		Addr:          ":0",
		SecretKey:     []byte("0123456789abcdef0123456789abcdef"),
		AdminUser:     "admin",
		AdminPassword: "pw",
	})
}

// webdist 必须真的把构建产物嵌进来。
func TestWebEmbedFSExists(t *testing.T) {
	idx, err := webDist.ReadFile("webdist/index.html")
	if err != nil {
		t.Fatalf("嵌入 index.html 不存在: %v", err)
	}
	if !strings.Contains(strings.ToLower(string(idx)), "<html") {
		t.Fatalf("嵌入 index.html 内容异常: %.60s", string(idx))
	}
	ents, err := webDist.ReadDir("webdist/assets")
	if err != nil {
		t.Fatalf("嵌入 assets 目录不存在: %v", err)
	}
	if len(ents) == 0 {
		t.Fatal("嵌入 assets 为空")
	}
	if _, err := webDist.ReadFile("webdist/assets/" + ents[0].Name()); err != nil {
		t.Fatalf("读取嵌入资产失败: %v", err)
	}
}

// API 优先：/api/v1/health 不能被 SPA fallback 吞掉。
func TestWebEmbedDoesNotSwallowAPI(t *testing.T) {
	s := newWebTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("/api/v1/health 状态 %d, body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(strings.ToLower(body), "<html") {
		t.Fatalf("API 被 SPA fallback 吞掉: %s", body)
	}
	if !strings.Contains(body, `"ok"`) {
		t.Fatalf("health body 异常: %s", body)
	}
}

// 根路径与未知 SPA 路径返回 index.html；缺失静态资源必须 404。
func TestWebEmbedServesIndexAndFallback(t *testing.T) {
	s := newWebTestServer()

	for _, p := range []string{"/", "/dashboard"} {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s 状态 %d", p, rec.Code)
		}
		if !strings.Contains(strings.ToLower(rec.Body.String()), "<html") {
			t.Fatalf("%s 未返回 index.html", p)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/assets/nope-does-not-exist.js", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("缺失资源应 404，实际 %d", rec.Code)
	}
}
