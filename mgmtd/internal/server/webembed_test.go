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

// webDist must actually contain the built front-end.
func TestWebEmbedFSExists(t *testing.T) {
	ents, err := webDist.ReadDir("dist")
	if err != nil {
		t.Fatalf("embed dist 不存在: %v", err)
	}
	if len(ents) == 0 {
		t.Fatal("embed dist 为空")
	}
	found := false
	for _, e := range ents {
		if e.Name() == "index.html" {
			found = true
		}
	}
	if !found {
		t.Fatal("embed dist 内没有 index.html")
	}
	if _, err := webDist.ReadFile("dist/index.html"); err != nil {
		t.Fatalf("读取嵌入 index.html 失败: %v", err)
	}
}

// API 路由必须优先于 SPA fallback，不能被 handleWeb 吞掉。
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
		t.Fatalf("API 被 SPA fallback 吞掉，返回了 HTML: %s", body)
	}
	if !strings.Contains(body, `"ok"`) {
		t.Fatalf("health body 异常: %s", body)
	}
}

// 浏览器根路径返回 index.html，SPA 未知路径也回落到 index.html。
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
			t.Fatalf("%s 未返回 index.html: %s", p, rec.Body.String()[:80])
		}
	}

	// 不存在的静态资源必须 404，不能回落到 index.html
	req := httptest.NewRequest(http.MethodGet, "/assets/nope-does-not-exist.js", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("缺失资源应 404，实际 %d", rec.Code)
	}
}
