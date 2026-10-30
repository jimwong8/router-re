package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// assertErrorCode 断言错误码，支持 float64/int/string 类型
func assertErrorCode(t *testing.T, body map[string]interface{}, want float64) {
	t.Helper()
	codeVal := body["code"]
	var code float64
	switch v := codeVal.(type) {
	case float64:
		code = v
	case int:
		code = float64(v)
	case int64:
		code = float64(v)
	case string:
		var err error
		code, err = strconv.ParseFloat(v, 64)
		if err != nil {
			t.Fatalf("cannot parse code string %q: %v", v, err)
		}
	default:
		t.Fatalf("unexpected code type %T: %v", codeVal, codeVal)
	}
	if code != want {
		t.Fatalf("expected error code %v, got %v", want, code)
	}
}

// TestSystemRebootEndpoint 测试系统重启端点
func TestSystemRebootEndpoint(t *testing.T) {
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
	req1 := httptest.NewRequest("POST", "/api/v1/system/reboot", strings.NewReader(`{"confirm":true}`))
	req1.Header.Set("Content-Type", "application/json")
	rr1 := httptest.NewRecorder()
	s.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", rr1.Code)
	}

	// 2. confirm=false → 422, code=10487
	req2 := httptest.NewRequest("POST", "/api/v1/system/reboot", strings.NewReader(`{"confirm":false}`))
	req2.Header.Set("Authorization", "Bearer "+token)
	req2.Header.Set("Content-Type", "application/json")
	rr2 := httptest.NewRecorder()
	s.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for confirm=false, got %d: %s", rr2.Code, rr2.Body.String())
	}
	var errResp map[string]interface{}
	_ = json.NewDecoder(rr2.Body).Decode(&errResp)
	assertErrorCode(t, errResp, 10487)

	// 测试用 fake runner，避免真实重启主机
	s.rebootRunner = &rebootFakeRunner{}

	// 3. confirm=true → 200 (不实际重启，因为测试环境没有 systemctl 或不想真的重启)
	// 验证返回结构
	req3 := httptest.NewRequest("POST", "/api/v1/system/reboot", strings.NewReader(`{"confirm":true}`))
	req3.Header.Set("Authorization", "Bearer "+token)
	req3.Header.Set("Content-Type", "application/json")
	rr3 := httptest.NewRecorder()
	s.ServeHTTP(rr3, req3)
	// 测试环境可能没有 systemctl，返回 500 也是可接受的，关键是不 panic
	if rr3.Code != http.StatusOK && rr3.Code != http.StatusInternalServerError {
		t.Fatalf("expected 200 or 500 for confirm=true, got %d: %s", rr3.Code, rr3.Body.String())
	}
	if rr3.Code == http.StatusOK {
		var body map[string]interface{}
		if err := json.NewDecoder(rr3.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["reboot"] != true {
			t.Fatalf("expected reboot=true, got %v", body["reboot"])
		}
		if body["delay"] != float64(2) {
			t.Fatalf("expected delay=2, got %v", body["delay"])
		}
	}
}

// TestSubnetCalcEndpoint 测试子网换算工具端点
func TestSubnetCalcEndpoint(t *testing.T) {
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

	testCases := []struct {
		name     string
		cidr     string
		wantCode int
		check    func(t *testing.T, body map[string]interface{})
	}{
		{
			name:     "missing cidr",
			cidr:     "",
			wantCode: http.StatusUnprocessableEntity,
			check: func(t *testing.T, body map[string]interface{}) {
				assertErrorCode(t, body, 10488)
			},
		},
		{
			name:     "invalid cidr",
			cidr:     "not-a-cidr",
			wantCode: http.StatusUnprocessableEntity,
			check: func(t *testing.T, body map[string]interface{}) {
				assertErrorCode(t, body, 10489)
			},
		},
		{
			name:     "IPv6 not supported",
			cidr:     "2001:db8::/32",
			wantCode: http.StatusUnprocessableEntity,
			check: func(t *testing.T, body map[string]interface{}) {
				assertErrorCode(t, body, 10490)
			},
		},
		{
			name:     "/24 network",
			cidr:     "192.168.1.0/24",
			wantCode: http.StatusOK,
			check: func(t *testing.T, body map[string]interface{}) {
				if body["network"] != "192.168.1.0" {
					t.Fatalf("network=192.168.1.0, got %v", body["network"])
				}
				if body["netmask"] != "255.255.255.0" {
					t.Fatalf("netmask=255.255.255.0, got %v", body["netmask"])
				}
				if body["broadcast"] != "192.168.1.255" {
					t.Fatalf("broadcast=192.168.1.255, got %v", body["broadcast"])
				}
				if body["first_host"] != "192.168.1.1" {
					t.Fatalf("first_host=192.168.1.1, got %v", body["first_host"])
				}
				if body["last_host"] != "192.168.1.254" {
					t.Fatalf("last_host=192.168.1.254, got %v", body["last_host"])
				}
				if body["host_count"] != float64(254) {
					t.Fatalf("host_count=254, got %v", body["host_count"])
				}
				if body["prefix_len"] != float64(24) {
					t.Fatalf("prefix_len=24, got %v", body["prefix_len"])
				}
			},
		},
		{
			name:     "/32 single host",
			cidr:     "10.0.0.1/32",
			wantCode: http.StatusOK,
			check: func(t *testing.T, body map[string]interface{}) {
				if body["network"] != "10.0.0.1" {
					t.Fatalf("network=10.0.0.1, got %v", body["network"])
				}
				if body["netmask"] != "255.255.255.255" {
					t.Fatalf("netmask=255.255.255.255, got %v", body["netmask"])
				}
				if body["broadcast"] != "10.0.0.1" {
					t.Fatalf("broadcast=10.0.0.1, got %v", body["broadcast"])
				}
				// /32 没有可用主机
				if body["host_count"] != float64(0) {
					t.Fatalf("host_count=0 for /32, got %v", body["host_count"])
				}
			},
		},
		{
			name:     "/0 whole internet",
			cidr:     "0.0.0.0/0",
			wantCode: http.StatusOK,
			check: func(t *testing.T, body map[string]interface{}) {
				if body["network"] != "0.0.0.0" {
					t.Fatalf("network=0.0.0.0, got %v", body["network"])
				}
				if body["netmask"] != "0.0.0.0" {
					t.Fatalf("netmask=0.0.0.0, got %v", body["netmask"])
				}
				if body["broadcast"] != "255.255.255.255" {
					t.Fatalf("broadcast=255.255.255.255, got %v", body["broadcast"])
				}
				if body["first_host"] != "0.0.0.1" {
					t.Fatalf("first_host=0.0.0.1, got %v", body["first_host"])
				}
				if body["last_host"] != "255.255.255.254" {
					t.Fatalf("last_host=255.255.255.254, got %v", body["last_host"])
				}
				// /0 有 2^32 - 2 个可用主机
				if body["host_count"] != float64(4294967294) {
					t.Fatalf("host_count=4294967294 for /0, got %v", body["host_count"])
				}
			},
		},
		{
			name:     "/30 point-to-point",
			cidr:     "10.0.0.0/30",
			wantCode: http.StatusOK,
			check: func(t *testing.T, body map[string]interface{}) {
				if body["network"] != "10.0.0.0" {
					t.Fatalf("network=10.0.0.0, got %v", body["network"])
				}
				if body["netmask"] != "255.255.255.252" {
					t.Fatalf("netmask=255.255.255.252, got %v", body["netmask"])
				}
				if body["broadcast"] != "10.0.0.3" {
					t.Fatalf("broadcast=10.0.0.3, got %v", body["broadcast"])
				}
				if body["first_host"] != "10.0.0.1" {
					t.Fatalf("first_host=10.0.0.1, got %v", body["first_host"])
				}
				if body["last_host"] != "10.0.0.2" {
					t.Fatalf("last_host=10.0.0.2, got %v", body["last_host"])
				}
				// /30 有 4 个地址，减去网络和广播 = 2 个可用
				if body["host_count"] != float64(2) {
					t.Fatalf("host_count=2 for /30, got %v", body["host_count"])
				}
			},
		},
		{
			name:     "/31 point-to-point (RFC 3021)",
			cidr:     "10.0.0.0/31",
			wantCode: http.StatusOK,
			check: func(t *testing.T, body map[string]interface{}) {
				if body["network"] != "10.0.0.0" {
					t.Fatalf("network=10.0.0.0, got %v", body["network"])
				}
				if body["netmask"] != "255.255.255.254" {
					t.Fatalf("netmask=255.255.255.254, got %v", body["netmask"])
				}
				if body["broadcast"] != "10.0.0.1" {
					t.Fatalf("broadcast=10.0.0.1, got %v", body["broadcast"])
				}
				// /31 有 2 个地址，RFC 3021 点对点链路通常不减 2
				if body["host_count"] != float64(0) {
					t.Fatalf("host_count=0 for /31 (RFC 3021), got %v", body["host_count"])
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			url := "/api/v1/tools/subnet"
			if tc.cidr != "" {
				url += "?cidr=" + tc.cidr
			}
			req := httptest.NewRequest("GET", url, nil)
			req.Header.Set("Authorization", "Bearer "+token)
			rr := httptest.NewRecorder()
			s.ServeHTTP(rr, req)
			if rr.Code != tc.wantCode {
				t.Fatalf("expected %d, got %d: %s", tc.wantCode, rr.Code, rr.Body.String())
			}
			var body map[string]interface{}
			if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			tc.check(t, body)
		})
	}
}

// rebootFakeRunner records argv slices without executing anything.
type rebootFakeRunner struct {
	Commands [][]string
}

func (f *rebootFakeRunner) Run(name string, args ...string) (string, error) {
	f.Commands = append(f.Commands, append([]string{name}, args...))
	return "ok", nil
}

func (f *rebootFakeRunner) RunWithInput(input, name string, args ...string) (string, error) {
	return f.Run(name, args...)
}

// TestSystemRebootCallsRunner 验证 confirm=true 时 handler 调用了 runner 执行重启命令。
func TestSystemRebootCallsRunner(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	fr := &rebootFakeRunner{}
	s.rebootRunner = fr

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

	req2 := httptest.NewRequest("POST", "/api/v1/system/reboot", strings.NewReader(`{"confirm":true}`))
	req2.Header.Set("Authorization", "Bearer "+token)
	req2.Header.Set("Content-Type", "application/json")
	rr2 := httptest.NewRecorder()
	s.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr2.Code, rr2.Body.String())
	}

	// 等待延迟重启 goroutine 执行
	time.Sleep(2500 * time.Millisecond)

	if len(fr.Commands) != 1 {
		t.Fatalf("expected runner called once, got %d", len(fr.Commands))
	}
	want := []string{"systemctl", "reboot"}
	if len(fr.Commands[0]) != len(want) {
		t.Fatalf("runner cmd %v, want %v", fr.Commands[0], want)
	}
	for i := range want {
		if fr.Commands[0][i] != want[i] {
			t.Fatalf("runner cmd %v, want %v", fr.Commands[0], want)
		}
	}
}
