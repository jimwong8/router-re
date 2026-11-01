package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// validateBPFFilter 应挡住参数注入，同时放行正常 BPF 表达式。
func TestValidateBPFFilter(t *testing.T) {
	bad := []string{
		"-z /tmp/evil.sh",        // -z 轮转执行命令
		" -F /etc/passwd",        // 前导空格后接 -
		"-w /root/x",             // 覆盖输出
		"tcp\nudp",               // 换行控制字符
		"tcp\x00udp",             // NUL
		strings.Repeat("a", 600), // 超长
	}
	for _, f := range bad {
		if err := validateBPFFilter(f); err == nil {
			t.Errorf("应拒绝非法 filter %q", f)
		}
	}
	good := []string{
		"",
		"tcp",
		"tcp port 80",
		"udp and host 10.0.0.1",
		"(tcp or udp) and port 443",
		"icmp[icmptype] == icmp-echo",
		"ether host 00:11:22:33:44:55",
	}
	for _, f := range good {
		if err := validateBPFFilter(f); err != nil {
			t.Errorf("应放行正常 filter %q，却报错 %v", f, err)
		}
	}
}

// handler 层：非法 interface / filter 必须在启动抓包前就 422 拒绝。
func TestCaptureStartRejectsInjection(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	tok := loginTokenAs(t, s, "admin", "admin123")

	cases := []string{
		`{"interface":"eth0","filter":"-z /tmp/x.sh"}`, // filter 参数注入
		`{"interface":"eth0;rm -rf /"}`,                // interface 非法字符
	}
	for _, body := range cases {
		req := httptest.NewRequest("POST", "/api/v1/tools/capture/start", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		s.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnprocessableEntity {
			t.Errorf("注入体 %q 应 422，得 %d (%s)", body, rr.Code, rr.Body.String())
		}
	}
}
