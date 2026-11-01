package cloud

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeRouter 模拟被管理的路由终端（本机 mgmtd），只实现测试用到的两个接口。
func fakeRouter(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"token": "local-jwt"})
	})
	mux.HandleFunc("GET /api/v1/system/info", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer local-jwt" {
			writeJSON(w, 401, map[string]string{"error": "no"})
			return
		}
		writeJSON(w, 200, map[string]string{"hostname": "fake-router", "version": "9.9.9"})
	})
	mux.HandleFunc("POST /api/v1/system/reboot", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"ok": "rebooting"})
	})
	return httptest.NewServer(mux)
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	svc, err := NewService(Config{
		StorePath:     filepath.Join(dir, "state.json"),
		AdminUser:     "admin",
		AdminPassword: "pw",
		JWTSecret:     []byte("0123456789abcdef0123456789abcdef"),
		ExecTimeout:   5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

func consoleToken(t *testing.T, base string) string {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"username": "admin", "password": "pw"})
	resp, err := http.Post(base+"/cloud/v1/login", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("login HTTP %d", resp.StatusCode)
	}
	var out struct {
		Token string `json:"token"`
	}
	json.NewDecoder(resp.Body).Decode(&out)
	if out.Token == "" {
		t.Fatal("login 没有返回 token")
	}
	return out.Token
}

func doJSON(t *testing.T, method, url, tok string, body interface{}) (int, map[string]interface{}) {
	t.Helper()
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, _ := http.NewRequest(method, url, rd)
	req.Header.Set("Content-Type", "application/json")
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	var out map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// TestEndToEndTunnel 覆盖完整链路：发令牌 → 设备注册 → 建隧道 → 控制台下发命令 → 设备本地执行 → 结果回传。
func TestEndToEndTunnel(t *testing.T) {
	router := fakeRouter(t)
	defer router.Close()
	svc := newTestService(t)
	cloudSrv := httptest.NewServer(svc.Handler())
	defer cloudSrv.Close()

	// 1. 控制台登录并签发一次性令牌
	tok := consoleToken(t, cloudSrv.URL)
	code, out := doJSON(t, "POST", cloudSrv.URL+"/cloud/v1/tokens", tok, map[string]interface{}{"note": "test", "ttl_minutes": 10})
	if code != 200 {
		t.Fatalf("创建令牌失败: %d %v", code, out)
	}
	enrollTok, _ := out["token"].(string)
	if enrollTok == "" {
		t.Fatal("令牌为空")
	}

	// 2. 设备侧 agent 注册并连上
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ag, err := NewAgent(ctx, AgentConfig{
		CloudURL: cloudSrv.URL, EnrollToken: enrollTok,
		StatePath: filepath.Join(t.TempDir(), "agent.json"),
		LocalBase: router.URL, LocalUser: "admin", LocalPass: "pw",
		Name: "unit-test-router", Version: "1.2.3",
	})
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	go ag.Run(ctx)

	// 3. 等设备上线（轮询，不 sleep 固定值）
	var devID string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		code, out = doJSON(t, "GET", cloudSrv.URL+"/cloud/v1/devices", tok, nil)
		if code == 200 {
			if arr, ok := out["devices"].([]interface{}); ok && len(arr) > 0 {
				d := arr[0].(map[string]interface{})
				if d["online"] == true {
					devID, _ = d["id"].(string)
					if n, _ := d["name"].(string); n != "unit-test-router" {
						t.Fatalf("设备名应为 unit-test-router，实际 %q", n)
					}
					break
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if devID == "" {
		t.Fatal("设备未上线")
	}

	// 4. 读接口：应穿透到本机并拿到 JSON
	code, out = doJSON(t, "POST", cloudSrv.URL+"/cloud/v1/devices/"+devID+"/exec", tok,
		map[string]interface{}{"method": "GET", "path": "/api/v1/system/info"})
	if code != 200 {
		t.Fatalf("执行读接口失败: %d %v", code, out)
	}
	raw, _ := base64.StdEncoding.DecodeString(out["body"].(string))
	if !strings.Contains(string(raw), "fake-router") {
		t.Fatalf("返回内容不对: %s", raw)
	}

	// 5. 白名单外的路径：云端直接拒绝
	code, out = doJSON(t, "POST", cloudSrv.URL+"/cloud/v1/devices/"+devID+"/exec", tok,
		map[string]interface{}{"method": "GET", "path": "/api/v1/users"})
	if code != 403 {
		t.Fatalf("白名单外路径应 403，实际 %d %v", code, out)
	}

	// 6. 写操作：没有 confirm 必须拒绝
	code, _ = doJSON(t, "POST", cloudSrv.URL+"/cloud/v1/devices/"+devID+"/exec", tok,
		map[string]interface{}{"method": "POST", "path": "/api/v1/system/reboot"})
	if code != 400 {
		t.Fatalf("写操作无确认应 400，实际 %d", code)
	}

	// 7. 写操作：confirm 填设备名才放行
	code, out = doJSON(t, "POST", cloudSrv.URL+"/cloud/v1/devices/"+devID+"/exec", tok,
		map[string]interface{}{"method": "POST", "path": "/api/v1/system/reboot", "confirm": "unit-test-router"})
	if code != 200 {
		t.Fatalf("写操作带确认应成功，实际 %d %v", code, out)
	}

	// 8. 移除设备后，原 secret 不能再用
	code, _ = doJSON(t, "DELETE", cloudSrv.URL+"/cloud/v1/devices/"+devID, tok, nil)
	if code != 200 {
		t.Fatalf("删除设备失败: %d", code)
	}
	if _, ok := svc.Store().GetDevice(devID); ok {
		t.Fatal("设备删除后仍存在")
	}
}

func TestTokenIsSingleUse(t *testing.T) {
	svc := newTestService(t)
	_, raw := svc.Store().CreateToken("t", time.Hour)
	if !svc.Store().ConsumeToken(raw, "dev1") {
		t.Fatal("首次消费应成功")
	}
	if svc.Store().ConsumeToken(raw, "dev2") {
		t.Fatal("令牌被重复消费")
	}
}

func TestTokenExpiry(t *testing.T) {
	svc := newTestService(t)
	_, raw := svc.Store().CreateToken("t", -time.Second) // 已过期
	if svc.Store().ConsumeToken(raw, "dev1") {
		t.Fatal("过期令牌不该通过")
	}
}

func TestPolicy(t *testing.T) {
	cases := []struct {
		method, path string
		want         bool
	}{
		{"GET", "/api/v1/system/info", true},
		{"GET", "/api/v1/wan/status", true},
		{"GET", "/api/v1/health", true},
		{"GET", "/api/v1/users", false},
		{"GET", "/api/v1/system/info/../../users", false},
		{"GET", "/api/v1/system/infoX", false},
		{"POST", "/api/v1/system/reboot", true},
		{"POST", "/api/v1/firewall/rules", false}, // 不在白名单
		{"DELETE", "/api/v1/system/info", false},
		{"GET", "/api/v1/system/info?x=1", true},
	}
	for _, c := range cases {
		if got := PathAllowed(c.method, c.path); got != c.want {
			t.Errorf("PathAllowed(%s %s) = %v, 期望 %v", c.method, c.path, got, c.want)
		}
	}
}

func TestHubCallTimeout(t *testing.T) {
	h := NewHub()
	h.Attach("dev", nil)
	start := time.Now()
	_, err := h.Call("dev", Frame{Type: "exec", Method: "GET", Path: "/api/v1/health"}, 200*time.Millisecond)
	if err != ErrTimeout {
		t.Fatalf("应超时，实际 %v", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("超时太快或太慢: %v", d)
	}
	// 迟到的结果不该 panic
	if h.Deliver(Frame{Type: "result", ID: "nope"}) {
		t.Fatal("无主的 result 应该被丢弃")
	}
}

func TestStorePersistReload(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "s.json")
	s1, err := OpenStore(p)
	if err != nil {
		t.Fatal(err)
	}
	d, secret := s1.RegisterDevice(Device{Name: "r1"})
	if err := s1.Save(); err != nil {
		t.Fatal(err)
	}
	s2, err := OpenStore(p)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := s2.AuthDevice(d.ID, secret)
	if !ok {
		t.Fatal("重启后凭据失效")
	}
	if got.Name != "r1" {
		t.Fatalf("名称丢失: %q", got.Name)
	}
	// 错误 secret 必须被拒
	if _, ok := s2.AuthDevice(d.ID, secret+"x"); ok {
		t.Fatal("错误凭据通过了")
	}
}
