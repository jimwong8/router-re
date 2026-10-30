package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCaptureStartEndpoint 测试启动抓包端点
func TestCaptureStartEndpoint(t *testing.T) {
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
	token := resp.Token

	// 1. 无 token → 401
	req1 := httptest.NewRequest("POST", "/api/v1/tools/capture/start", strings.NewReader(`{"interface":"eth0"}`))
	req1.Header.Set("Content-Type", "application/json")
	rr1 := httptest.NewRecorder()
	s.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", rr1.Code)
	}

	// 2. 缺少 interface → 422
	req2 := httptest.NewRequest("POST", "/api/v1/tools/capture/start", strings.NewReader(`{"filter":"tcp"}`))
	req2.Header.Set("Authorization", "Bearer "+token)
	req2.Header.Set("Content-Type", "application/json")
	rr2 := httptest.NewRecorder()
	s.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for missing interface, got %d: %s", rr2.Code, rr2.Body.String())
	}
	var errResp map[string]interface{}
	_ = json.NewDecoder(rr2.Body).Decode(&errResp)
	assertErrorCode(t, errResp, 10492)

	// 3. 正常请求 → 200 (虽然接口可能不存在，但端点应返回任务 ID)
	req3 := httptest.NewRequest("POST", "/api/v1/tools/capture/start", strings.NewReader(`{"interface":"lo","seconds":5,"max_bytes":1024}`))
	req3.Header.Set("Authorization", "Bearer "+token)
	req3.Header.Set("Content-Type", "application/json")
	rr3 := httptest.NewRecorder()
	s.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr3.Code, rr3.Body.String())
	}
	var body map[string]interface{}
	if err := json.NewDecoder(rr3.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["id"] == nil || body["id"] == "" {
		t.Fatal("expected capture id in response")
	}
	if body["status"] != "running" {
		t.Fatalf("expected status running, got %v", body["status"])
	}
	captureID := body["id"].(string)

	// 4. 查询状态
	req4 := httptest.NewRequest("GET", "/api/v1/tools/capture/status?id="+captureID, nil)
	req4.Header.Set("Authorization", "Bearer "+token)
	rr4 := httptest.NewRecorder()
	s.ServeHTTP(rr4, req4)
	if rr4.Code != http.StatusOK {
		t.Fatalf("status query failed: %d %s", rr4.Code, rr4.Body.String())
	}
	var statusResp map[string]interface{}
	if err := json.NewDecoder(rr4.Body).Decode(&statusResp); err != nil {
		t.Fatal(err)
	}
	if statusResp["id"] != captureID {
		t.Fatalf("status id mismatch: %v", statusResp["id"])
	}

	// 5. 停止抓包
	req5 := httptest.NewRequest("POST", "/api/v1/tools/capture/"+captureID+"/stop", nil)
	req5.Header.Set("Authorization", "Bearer "+token)
	rr5 := httptest.NewRecorder()
	s.ServeHTTP(rr5, req5)
	if rr5.Code != http.StatusOK {
		t.Fatalf("stop failed: %d %s", rr5.Code, rr5.Body.String())
	}

	// 6. 查询所有任务列表
	req6 := httptest.NewRequest("GET", "/api/v1/tools/capture/status", nil)
	req6.Header.Set("Authorization", "Bearer "+token)
	rr6 := httptest.NewRecorder()
	s.ServeHTTP(rr6, req6)
	if rr6.Code != http.StatusOK {
		t.Fatalf("list failed: %d %s", rr6.Code, rr6.Body.String())
	}
	var listResp map[string]interface{}
	if err := json.NewDecoder(rr6.Body).Decode(&listResp); err != nil {
		t.Fatal(err)
	}
	if listResp["count"] == nil {
		t.Fatal("expected count in list response")
	}
}

// TestCaptureValidation 测试参数验证
func TestCaptureValidation(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})

	login := `{"username":"admin","password":"admin123"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(login))
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	var resp struct{ Token string }
	json.NewDecoder(rr.Body).Decode(&resp)
	token := resp.Token

	testCases := []struct {
		name      string
		body      string
		wantCode  int
		wantError float64
	}{
		{
			name:      "invalid json",
			body:      `{invalid}`,
			wantCode:  http.StatusBadRequest,
			wantError: 10491,
		},
		{
			name:      "missing interface",
			body:      `{"filter":"tcp"}`,
			wantCode:  http.StatusUnprocessableEntity,
			wantError: 10492,
		},
		{
			name:      "seconds too large capped",
			body:      `{"interface":"lo","seconds":10000}`,
			wantCode:  http.StatusOK,
			wantError: 0,
		},
		{
			name:      "max_bytes too large capped",
			body:      `{"interface":"lo","max_bytes":10000000000}`,
			wantCode:  http.StatusOK,
			wantError: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/tools/capture/start", strings.NewReader(tc.body))
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			s.ServeHTTP(rr, req)
			if rr.Code != tc.wantCode {
				t.Fatalf("expected %d, got %d: %s", tc.wantCode, rr.Code, rr.Body.String())
			}
			if tc.wantError > 0 {
				var errResp map[string]interface{}
				_ = json.NewDecoder(rr.Body).Decode(&errResp)
				assertErrorCode(t, errResp, tc.wantError)
			}
		})
	}
}

// TestCaptureNotFound 测试不存在的任务
func TestCaptureNotFound(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})

	login := `{"username":"admin","password":"admin123"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(login))
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	var resp struct{ Token string }
	json.NewDecoder(rr.Body).Decode(&resp)
	token := resp.Token

	// 查询不存在的任务
	req1 := httptest.NewRequest("GET", "/api/v1/tools/capture/status?id=nonexistent", nil)
	req1.Header.Set("Authorization", "Bearer "+token)
	rr1 := httptest.NewRecorder()
	s.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr1.Code)
	}
	var errResp map[string]interface{}
	_ = json.NewDecoder(rr1.Body).Decode(&errResp)
	assertErrorCode(t, errResp, 10493)

	// 下载不存在的任务
	req2 := httptest.NewRequest("GET", "/api/v1/tools/capture/nonexistent/download", nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	rr2 := httptest.NewRecorder()
	s.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for download, got %d", rr2.Code)
	}

	// 停止不存在的任务
	req3 := httptest.NewRequest("POST", "/api/v1/tools/capture/nonexistent/stop", nil)
	req3.Header.Set("Authorization", "Bearer "+token)
	rr3 := httptest.NewRecorder()
	s.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for stop, got %d", rr3.Code)
	}
}
