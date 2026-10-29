package server

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// 没有写权限时必须 403 并说清原因，不能是 500 "apply failed"。
// 500 会让运维去翻代码找 bug，而真实原因只是进程没有 /etc/dnsmasq.d 的权限。
func TestWebfilterApplyNoPermissionIsForbidden(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root 无视权限")
	}
	dir := t.TempDir()
	ro := filepath.Join(dir, "ro")
	if err := os.Mkdir(ro, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MGM_WEBFILTER_CONF", filepath.Join(ro, "wf.conf"))

	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	token := loginToken(t, s)
	req := httptest.NewRequest("POST", "/api/v1/webfilter/apply",
		strings.NewReader(`{"rules":[{"domain":"example.com","action":"block","enabled":true}]}`))
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)

	if rr.Code != 403 {
		t.Fatalf("无权限应 403，got %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "10503") {
		t.Fatalf("应带 10503 错误码: %s", rr.Body.String())
	}
}

// body 写错必须 400，而且绝不能被当成"空规则"一路下发。
// 原来 decode 错误被吞：手滑的请求会把过滤清空并返回 200。
func TestWebfilterApplyBadBodyIsRejected(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MGM_WEBFILTER_CONF", filepath.Join(dir, "wf.conf"))

	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	token := loginToken(t, s)

	req := httptest.NewRequest("POST", "/api/v1/webfilter/apply", strings.NewReader(`{"rules":`))
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)

	if rr.Code != 400 {
		t.Fatalf("坏 body 应 400，got %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "10504") {
		t.Fatalf("应带 10504 错误码: %s", rr.Body.String())
	}
	// 关键：不能走到下发环节（那会真的改 /etc/dnsmasq.d）
	if ok, _, _, _ := webfilterResult(); ok {
		t.Fatal("坏请求不应被记为下发成功")
	}
}

// 并发 apply + status 不应有数据竞争（用 -race 跑这个测试）。
func TestWebfilterStatusRace(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MGM_WEBFILTER_CONF", filepath.Join(dir, "wf.conf"))
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	token := loginToken(t, s)

	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("POST", "/api/v1/webfilter/apply",
				strings.NewReader(`{"rules":[{"domain":"a.example","action":"block","enabled":true}]}`))
			req.Header.Set("Authorization", "Bearer "+token)
			s.ServeHTTP(httptest.NewRecorder(), req)
		}()
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/api/v1/webfilter/apply-status", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			s.ServeHTTP(httptest.NewRecorder(), req)
		}()
	}
	wg.Wait()
}
