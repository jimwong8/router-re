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

func TestDHCPConfigEndpoint(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	// 登录拿 token
	login := `{"username":"admin","password":"admin123"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(login))
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

	// 未带 token → 401
	req2 := httptest.NewRequest("GET", "/api/v1/dhcp/config", nil)
	rr2 := httptest.NewRecorder()
	s.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for no token, got %d", rr2.Code)
	}

	// POST 配置
	post := `{"interface":"lan","range_start":"192.168.1.100","range_end":"192.168.1.200","netmask":"255.255.255.0","gateway":"192.168.1.1","lease_time":"12h","dns":["223.5.5.5","114.114.114.114"]}`
	req3 := httptest.NewRequest("POST", "/api/v1/dhcp/config", strings.NewReader(post))
	req3.Header.Set("Authorization", "Bearer "+resp.Token)
	rr3 := httptest.NewRecorder()
	s.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusOK {
		t.Fatalf("POST dhcp failed: %d %s", rr3.Code, rr3.Body.String())
	}
	if !strings.Contains(rr3.Body.String(), "dhcp-range=192.168.1.100,192.168.1.200") {
		t.Fatalf("POST response missing dhcp-range: %s", rr3.Body.String())
	}

	// GET 回读应返回同样的渲染文本
	req4 := httptest.NewRequest("GET", "/api/v1/dhcp/config", nil)
	req4.Header.Set("Authorization", "Bearer "+resp.Token)
	rr4 := httptest.NewRecorder()
	s.ServeHTTP(rr4, req4)
	if rr4.Code != http.StatusOK {
		t.Fatalf("GET dhcp failed: %d %s", rr4.Code, rr4.Body.String())
	}
	if !strings.Contains(rr4.Body.String(), "dhcp-option=3,192.168.1.1") {
		t.Fatalf("GET response missing gateway option: %s", rr4.Body.String())
	}
	if !strings.Contains(rr4.Body.String(), "dhcp-option=6,223.5.5.5,114.114.114.114") {
		t.Fatalf("GET response missing dns option: %s", rr4.Body.String())
	}
}
