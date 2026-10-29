package server

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// 空 body 不能把所有在线设备踢下线——只能按来源 IP 匹配自己那台。
func TestPortalLogoutDoesNotMassRevoke(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	token := loginToken(t, s)

	// 造两台在线设备（IP 与请求来源都不同）
	if _, err := s.portalMgr.Authorize("aa:bb:cc:dd:ee:01", "10.9.9.1", time.Hour); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if _, err := s.portalMgr.Authorize("aa:bb:cc:dd:ee:02", "10.9.9.2", time.Hour); err != nil {
		t.Fatalf("authorize: %v", err)
	}

	req := httptest.NewRequest("POST", "/api/v1/portal/logout", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("logout HTTP %d: %s", rr.Code, rr.Body.String())
	}
	if got := len(s.portalMgr.Authorized(time.Now())); got != 2 {
		t.Fatalf("来源 IP 不匹配任何设备时不能下线任何设备，仍在线的有 %d 台", got)
	}
}

// 显式 mac 下线要真的移除，并且 status 反映出来。
func TestPortalLogoutByMAC(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	token := loginToken(t, s)
	if _, err := s.portalMgr.Authorize("aa:bb:cc:dd:ee:03", "10.9.9.3", time.Hour); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("POST", "/api/v1/portal/logout",
		strings.NewReader(`{"mac":"AA:BB:CC:DD:EE:03"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"revoked":1`) {
		t.Fatalf("按 mac 下线失败: %d %s", rr.Code, rr.Body.String())
	}

	req2 := httptest.NewRequest("GET", "/api/v1/portal/status", nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	rr2 := httptest.NewRecorder()
	s.ServeHTTP(rr2, req2)
	if rr2.Code != 200 || !strings.Contains(rr2.Body.String(), `"online":false`) {
		t.Fatalf("status 应显示离线: %d %s", rr2.Code, rr2.Body.String())
	}
}
