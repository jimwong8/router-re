package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthEndpoint(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("test-secret-key-32-bytes-long!!")})
	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestLoginAndProtectedEndpoint(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	// 登录
	body := `{"username":"admin","password":"admin123"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", rr.Code, rr.Body.String())
	}
	// 未带 token 访问受保护端点 → 401
	req2 := httptest.NewRequest("GET", "/api/v1/system/info", nil)
	rr2 := httptest.NewRecorder()
	s.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr2.Code)
	}
}

func TestLoginBadCredentials(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	body := `{"username":"admin","password":"wrong"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestProtectedEndpointWithToken(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	body := `{"username":"admin","password":"admin123"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Token == "" {
		t.Fatal("empty token")
	}
	// 带 token 访问受保护端点 → 200
	req2 := httptest.NewRequest("GET", "/api/v1/system/info", nil)
	req2.Header.Set("Authorization", "Bearer "+resp.Token)
	rr2 := httptest.NewRecorder()
	s.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d %s", rr2.Code, rr2.Body.String())
	}
}
