package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestCaptureStopTerminatesProcess 证明 stop 端点真的杀掉了抓包进程，
// 而不只是返回 200 —— p107_capture_test.go 只验了 HTTP 状态码，
// 抓不到「stop 返回 200 但 tcpdump 继续写盘」这类回归。
//
// 手法：把一个假的 tcpdump 放到 PATH 最前面，它写下自己的 PID 后常驻。
// start 之后确认该 PID 活着，stop 之后确认 PID 真的消失。
// 用假进程而不是真 tcpdump，是因为真 tcpdump 需要 CAP_NET_RAW，
// 普通用户跑起来会立刻 permission denied，进程根本活不到 stop。
func TestCaptureStopTerminatesProcess(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "tcpdump.pid")
	script := filepath.Join(dir, "tcpdump")
	body := "#!/bin/sh\n" +
		"echo $$ > '" + pidFile + "'\n" +
		"while true; do sleep 0.2; done\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake tcpdump: %v", err)
	}
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", dir+string(os.PathListSeparator)+oldPath)
	defer os.Setenv("PATH", oldPath)

	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})

	login := `{"username":"admin","password":"admin123"}`
	lr := httptest.NewRecorder()
	s.ServeHTTP(lr, httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(login)))
	if lr.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", lr.Code, lr.Body.String())
	}
	var lresp struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(lr.Body).Decode(&lresp); err != nil {
		t.Fatal(err)
	}
	token := lresp.Token

	// 启动抓包
	sr := httptest.NewRecorder()
	sreq := httptest.NewRequest("POST", "/api/v1/tools/capture/start",
		strings.NewReader(`{"interface":"lo","seconds":60}`))
	sreq.Header.Set("Authorization", "Bearer "+token)
	sreq.Header.Set("Content-Type", "application/json")
	s.ServeHTTP(sr, sreq)
	if sr.Code != http.StatusOK {
		t.Fatalf("start failed: %d %s", sr.Code, sr.Body.String())
	}
	var sbody map[string]interface{}
	if err := json.NewDecoder(sr.Body).Decode(&sbody); err != nil {
		t.Fatal(err)
	}
	id, _ := sbody["id"].(string)
	if id == "" {
		t.Fatal("expected capture id")
	}

	// 等假 tcpdump 起进程
	pid := waitForPID(t, pidFile, 5*time.Second)
	if err := syscall.Kill(pid, 0); err != nil {
		t.Fatalf("fake tcpdump pid %d should be alive before stop: %v", pid, err)
	}

	// 停止抓包
	pr := httptest.NewRecorder()
	preq := httptest.NewRequest("POST", "/api/v1/tools/capture/"+id+"/stop", nil)
	preq.Header.Set("Authorization", "Bearer "+token)
	s.ServeHTTP(pr, preq)
	if pr.Code != http.StatusOK {
		t.Fatalf("stop failed: %d %s", pr.Code, pr.Body.String())
	}

	// 进程必须真的消失
	deadline := time.Now().Add(5 * time.Second)
	gone := false
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); err != nil {
			gone = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !gone {
		t.Fatalf("capture process %d still alive after stop: cancel did not reach the running command", pid)
	}

	// 状态也应落到 stopped
	qr := httptest.NewRecorder()
	qreq := httptest.NewRequest("GET", "/api/v1/tools/capture/status?id="+id, nil)
	qreq.Header.Set("Authorization", "Bearer "+token)
	s.ServeHTTP(qr, qreq)
	var qbody map[string]interface{}
	if err := json.NewDecoder(qr.Body).Decode(&qbody); err != nil {
		t.Fatal(err)
	}
	if st, _ := qbody["status"].(string); st != "stopped" {
		t.Fatalf("expected status stopped, got %q", st)
	}
}

// waitForPID 轮询 pid 文件直到拿到一个数字 PID。
func waitForPID(t *testing.T, path string, timeout time.Duration) int {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(path)
		if err == nil {
			var pid int
			if _, err := fmt.Sscan(strings.TrimSpace(string(raw)), &pid); err == nil && pid > 0 {
				return pid
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("fake tcpdump did not report a pid within %s", timeout)
	return 0
}
