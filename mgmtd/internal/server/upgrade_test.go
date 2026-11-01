package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newUpgradeServer(t *testing.T) *Server {
	t.Helper()
	return New(Config{
		Addr:          ":0",
		SecretKey:     []byte("0123456789abcdef0123456789abcdef"),
		AdminUser:     "admin",
		AdminPassword: "pw",
	})
}

func callUpgrade(t *testing.T, s *Server, body string) (int, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/upgrade", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleSystemUpgrade(w, req)
	var out map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

// 升级接口是把二进制写到磁盘上再重启的路径，任何一条校验缺失都可能把设备刷成砖。
// 这几条断言对应真实会发生的失败：URL 返回 404 页面、对端只给半个文件、忘了传 checksum。
func TestUpgradeRejectsUnsafeInput(t *testing.T) {
	s := newUpgradeServer(t)
	goodSum := strings.Repeat("a", 64)

	cases := []struct {
		name string
		body string
		want int
	}{
		{"缺 confirm", `{"url":"http://x/y","checksum":"` + goodSum + `"}`, 422},
		{"缺 checksum", `{"url":"http://x/y","confirm":true}`, 400},
		{"checksum 长度不对", `{"url":"http://x/y","checksum":"abc","confirm":true}`, 400},
		{"checksum 非十六进制", `{"url":"http://x/y","checksum":"` + strings.Repeat("z", 64) + `","confirm":true}`, 400},
		{"URL 是相对路径", `{"url":"/local/file","checksum":"` + goodSum + `","confirm":true}`, 400},
		{"URL 是 file 协议", `{"url":"file:///etc/passwd","checksum":"` + goodSum + `","confirm":true}`, 400},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, _ := callUpgrade(t, s, c.body)
			if code != c.want {
				t.Fatalf("状态码 = %d，期望 %d", code, c.want)
			}
		})
	}
}

// 下载回来的内容不是 ELF 时必须拒绝。
// 真实场景：发布地址填错，服务器返回一个 200 的 HTML 404 页，旧实现会拿它覆盖二进制。
func TestUpgradeRejectsNonELF(t *testing.T) {
	s := newUpgradeServer(t)
	// 造一个超过 4KB 的页面：太小的响应会先被"文件太小"拦掉，
	// 这里要验证的是"体积像正常发布包、内容却不是 ELF"这条分支。
	page := []byte("<html><body>" + strings.Repeat("404 not found. ", 400) + "</body></html>")
	sum := sha256.Sum256(page)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(page)
	}))
	defer srv.Close()

	code, out := callUpgrade(t, s, `{"url":"`+srv.URL+`","checksum":"`+
		hex.EncodeToString(sum[:])+`","confirm":true}`)
	if code == 200 {
		t.Fatalf("HTML 页面被当成升级包接受了：%v", out)
	}
	if !strings.Contains(strings.ToLower(fmtStr(out)), "elf") {
		t.Fatalf("错误信息没说明原因：%v", out)
	}
}

// 校验和不匹配必须拒绝，且不能留下 .new 残留文件。
func TestUpgradeRejectsChecksumMismatch(t *testing.T) {
	s := newUpgradeServer(t)
	// 一段以 ELF 魔数开头的假二进制，确保它先过了 ELF 检查、卡在校验和上
	payload := make([]byte, 8192)
	copy(payload, []byte{0x7f, 'E', 'L', 'F'})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	wrong := strings.Repeat("0", 64)
	code, out := callUpgrade(t, s, `{"url":"`+srv.URL+`","checksum":"`+wrong+`","confirm":true}`)
	if code == 200 {
		t.Fatalf("校验和不匹配却升级成功：%v", out)
	}
	if !strings.Contains(strings.ToLower(fmtStr(out)), "checksum") {
		t.Fatalf("错误信息没指出校验和问题：%v", out)
	}
}

// 超过大小上限必须拒绝，不能把根分区写满。
func TestUpgradeRejectsOversize(t *testing.T) {
	s := newUpgradeServer(t)
	// 不真的传 128MB：声明一个超大 Content-Length 也会被 LimitReader 截断，
	// 这里只验证"下不完"的情况会被当成错误而不是静默成功。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1024")
		_, _ = w.Write([]byte{0x7f, 'E', 'L', 'F'}) // 只给 4 字节就断开
	}))
	defer srv.Close()

	sum := sha256.Sum256([]byte{0x7f, 'E', 'L', 'F'})
	code, out := callUpgrade(t, s, `{"url":"`+srv.URL+`","checksum":"`+
		hex.EncodeToString(sum[:])+`","confirm":true}`)
	if code == 200 {
		t.Fatalf("4 字节的残包被接受：%v", out)
	}
}

// 下载地址不可达时不能 panic，要给出 5xx 和可读原因。
func TestUpgradeHandlesUnreachableURL(t *testing.T) {
	s := newUpgradeServer(t)
	code, out := callUpgrade(t, s, `{"url":"http://127.0.0.1:1/nope","checksum":"`+
		strings.Repeat("a", 64)+`","confirm":true}`)
	if code != 502 {
		t.Fatalf("状态码 = %d，期望 502（out=%v）", code, out)
	}
}

// 看门狗脚本必须被真正写出来，且包含回滚动作。
func TestUpgradeWatchdogScriptWritten(t *testing.T) {
	s := newUpgradeServer(t)
	dir := t.TempDir()
	exe := filepath.Join(dir, "mgmtd")
	if err := os.WriteFile(exe, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	bak := exe + ".bak"
	path := s.startUpgradeWatchdog(exe, bak, "mgmtd")
	if path == "" {
		t.Fatal("看门狗脚本没有生成")
	}
	t.Cleanup(func() { _ = os.Remove(path) })
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读不到看门狗脚本: %v", err)
	}
	body := string(b)
	for _, want := range []string{"curl", "wget", "cp -f", "systemctl start", "sleep 25"} {
		if !strings.Contains(body, want) {
			t.Fatalf("看门狗脚本缺少 %q，回滚链路不完整:\n%s", want, body)
		}
	}
}

func fmtStr(m map[string]interface{}) string {
	b, _ := json.Marshal(m)
	return string(b)
}
