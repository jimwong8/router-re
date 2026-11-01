package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 配置导出包含明文密钥（wifi 密码 / DDNS token / 云端凭据 / VPN 私钥）。
// 修复前 viewer 也能 GET /config/export：实测能拿到 ddns token 明文，
// 而同一份配置的普通 GET 接口返回的是掩码 —— "密钥只写不读"的约定被绕过。
// 现在只允许管理员导出。
func TestConfigExportRequiresAdmin(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	admin := loginToken(t, s)

	// 建一个 viewer
	body := `{"username":"vwexp","password":"Viewer12345","role":"viewer"}`
	req := httptest.NewRequest("POST", "/api/v1/users", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+admin)
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated && rr.Code != http.StatusOK {
		t.Fatalf("create viewer failed: %d %s", rr.Code, rr.Body.String())
	}

	loginAs := func(u, p string) string {
		t.Helper()
		r := httptest.NewRequest("POST", "/api/v1/auth/login",
			strings.NewReader(`{"username":"`+u+`","password":"`+p+`"}`))
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, r)
		if rec.Code != http.StatusOK {
			t.Fatalf("login %s failed: %d %s", u, rec.Code, rec.Body.String())
		}
		var out struct {
			Token string `json:"token"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode token: %v", err)
		}
		return out.Token
	}
	viewer := loginAs("vwexp", "Viewer12345")

	// viewer 导出必须 403
	r := httptest.NewRequest("GET", "/api/v1/config/export", nil)
	r.Header.Set("Authorization", "Bearer "+viewer)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, r)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer 导出配置应 403，得 %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "10210") {
		t.Fatalf("应返回 10210 需要 admin，实际: %s", rec.Body.String())
	}

	// admin 导出必须仍然 200
	r = httptest.NewRequest("GET", "/api/v1/config/export", nil)
	r.Header.Set("Authorization", "Bearer "+admin)
	rec = httptest.NewRecorder()
	s.ServeHTTP(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin 导出配置应 200，得 %d: %s", rec.Code, rec.Body.String())
	}
}

// 配置恢复会整体覆盖现有配置（含密钥），同样只允许管理员。
func TestConfigRestoreRequiresAdmin(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	admin := loginToken(t, s)

	body := `{"username":"vwrstr","password":"Viewer12345","role":"viewer"}`
	req := httptest.NewRequest("POST", "/api/v1/users", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+admin)
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated && rr.Code != http.StatusOK {
		t.Fatalf("create viewer failed: %d %s", rr.Code, rr.Body.String())
	}

	loginReq := httptest.NewRequest("POST", "/api/v1/auth/login",
		strings.NewReader(`{"username":"vwrstr","password":"Viewer12345"}`))
	loginRec := httptest.NewRecorder()
	s.ServeHTTP(loginRec, loginReq)
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(loginRec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode token: %v", err)
	}

	r := httptest.NewRequest("POST", "/api/v1/config/restore", strings.NewReader(`{"confirm":true}`))
	r.Header.Set("Authorization", "Bearer "+out.Token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, r)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer 恢复配置应 403，得 %d: %s", rec.Code, rec.Body.String())
	}
}
