package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jimwong/router-re/mgmtd/internal/rbac"
)

// login 返回登录 token（失败时 t.Fatal）。
func loginTokenAs(t *testing.T, s *Server, user, pass string) string {
	t.Helper()
	body := `{"username":"` + user + `","password":"` + pass + `"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("login %s failed: %d %s", user, rr.Code, rr.Body.String())
	}
	var resp struct {
		Token string `json:"token"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	return resp.Token
}

func postChangePassword(s *Server, token, oldPw, newPw string) int {
	body := `{"old_password":"` + oldPw + `","new_password":"` + newPw + `"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/password", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	return rr.Code
}

// viewer 这类只读角色也必须能改自己的密码；改后旧密码失效、新密码可登录；
// 且因为身份取自 token，这个端点根本无法改到别人的账号。
func TestChangeOwnPasswordViewer(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	hash, _ := rbac.HashPassword("ViewerOld123")
	if err := s.rbacMgr.Create(rbac.User{Username: "viewer1", PasswordHash: hash, Role: rbac.RoleViewer, Enabled: true}); err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	tok := loginTokenAs(t, s, "viewer1", "ViewerOld123")

	// 旧密码错误 -> 401，不允许改
	if code := postChangePassword(s, tok, "wrong-old", "ViewerNew456"); code != http.StatusUnauthorized {
		t.Fatalf("旧密码错误应 401，得 %d", code)
	}
	// 正确旧密码 -> 200
	if code := postChangePassword(s, tok, "ViewerOld123", "ViewerNew456"); code != http.StatusOK {
		t.Fatalf("改密应 200，得 %d", code)
	}
	// 旧密码现在应登录失败
	if code := func() int {
		req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{"username":"viewer1","password":"ViewerOld123"}`))
		rr := httptest.NewRecorder()
		s.ServeHTTP(rr, req)
		return rr.Code
	}(); code != http.StatusUnauthorized {
		t.Fatalf("改密后旧密码应 401，得 %d", code)
	}
	// 新密码可登录
	_ = loginTokenAs(t, s, "viewer1", "ViewerNew456")
}

// 弱密码必须被拒（422），不能绕过强度校验。
func TestChangeOwnPasswordRejectsWeak(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	hash, _ := rbac.HashPassword("StrongOld123")
	_ = s.rbacMgr.Create(rbac.User{Username: "op1", PasswordHash: hash, Role: rbac.RoleOperator, Enabled: true})
	tok := loginTokenAs(t, s, "op1", "StrongOld123")
	if code := postChangePassword(s, tok, "StrongOld123", "abc"); code != http.StatusUnprocessableEntity {
		t.Fatalf("弱密码应 422，得 %d", code)
	}
}

// 未登录（无 token）必须 401。
func TestChangeOwnPasswordRequiresAuth(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	req := httptest.NewRequest("POST", "/api/v1/auth/password", strings.NewReader(`{"old_password":"x","new_password":"1234567890"}`))
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("无 token 应 401，得 %d", rr.Code)
	}
}
