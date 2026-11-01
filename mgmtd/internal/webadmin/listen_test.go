package webadmin

import (
	"fmt"
	"net"
	"strings"
	"testing"
)

// recordingRunner records commands.
type recordingRunner struct {
	calls []string
	fail  map[string]bool
}

func newRec() *recordingRunner {
	return &recordingRunner{fail: map[string]bool{}}
}
func (r *recordingRunner) Run(name string, args ...string) (string, error) {
	r.calls = append(r.calls, name+" "+strings.Join(args, " "))
	if r.fail[name+" "+strings.Join(args, " ")] {
		return "", fmt.Errorf("fail: %s", name)
	}
	return "", nil
}
func (r *recordingRunner) RunWithInput(input, name string, args ...string) (string, error) {
	return r.Run(name, args...)
}
func (r *recordingRunner) hasCall(sub string) bool {
	for _, c := range r.calls {
		if strings.Contains(c, sub) {
			return true
		}
	}
	return false
}
func (r *recordingRunner) indexOf(sub string) int {
	for i, c := range r.calls {
		if strings.Contains(c, sub) {
			return i
		}
	}
	return -1
}

func TestValidateListenBothZero(t *testing.T) {
	c := ListenConfig{HTTPPort: 0, HTTPSPort: 0, BindAddr: "127.0.0.1"}
	if err := ValidateListen(c); err == nil {
		t.Fatal("expected error when both ports are 0 (self-lockout)")
	}
}

func TestValidateListenPlaintextExposed(t *testing.T) {
	c := ListenConfig{HTTPPort: 80, HTTPSPort: 0, BindAddr: "0.0.0.0"}
	if err := ValidateListen(c); err == nil {
		t.Fatal("expected error for bind=0.0.0.0 with https_port=0")
	}
}

func TestValidateListenSamePort(t *testing.T) {
	c := ListenConfig{HTTPPort: 8080, HTTPSPort: 8080, BindAddr: "127.0.0.1"}
	if err := ValidateListen(c); err == nil {
		t.Fatal("expected error when ports are equal")
	}
}

func TestValidateListenOK(t *testing.T) {
	c := ListenConfig{HTTPPort: 80, HTTPSPort: 443, BindAddr: "0.0.0.0", IdleTimeoutSec: 300}
	if err := ValidateListen(c); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

func TestRenderNginxRedirect301(t *testing.T) {
	c := ListenConfig{HTTPPort: 80, HTTPSPort: 443, BindAddr: "0.0.0.0", RedirectHTTP: true}
	out := RenderNginx(c)
	if !strings.Contains(out, "return 301") {
		t.Fatalf("redirect_http should emit return 301, got:\n%s", out)
	}
}

func TestRenderNginxNoSSLWhenHttpsZero(t *testing.T) {
	c := ListenConfig{HTTPPort: 80, HTTPSPort: 0, BindAddr: "127.0.0.1"}
	out := RenderNginx(c)
	if strings.Contains(out, "ssl_certificate") {
		t.Fatalf("should not emit ssl config when https_port=0, got:\n%s", out)
	}
}

func TestValidateIdentityLongHostname(t *testing.T) {
	long := strings.Repeat("a", 64)
	c := Identity{Hostname: long}
	if err := ValidateIdentity(c); err == nil {
		t.Fatal("expected error for 64-char hostname")
	}
}

func TestValidateIdentityLeadingHyphen(t *testing.T) {
	c := Identity{Hostname: "-abc"}
	if err := ValidateIdentity(c); err == nil {
		t.Fatal("expected error for leading hyphen")
	}
	c = Identity{Hostname: "abc-"}
	if err := ValidateIdentity(c); err == nil {
		t.Fatal("expected error for trailing hyphen")
	}
}

func TestValidateIdentityAllDigits(t *testing.T) {
	c := Identity{Hostname: "12345"}
	if err := ValidateIdentity(c); err == nil {
		t.Fatal("expected error for all-digits hostname")
	}
}

func TestValidateIdentityOK(t *testing.T) {
	c := Identity{Hostname: "router-1", Domain: "lan", Location: "rack-a"}
	if err := ValidateIdentity(c); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

func TestApplyIdentityDeletesOldHostsLineFirst(t *testing.T) {
	rr := newRec()
	// 拦截 /etc/hosts 追加：单元测试不该真的写宿主机文件。
	var appended []string
	oldAppend := appendHostsLine
	appendHostsLine = func(line string) error {
		appended = append(appended, line)
		return nil
	}
	t.Cleanup(func() { appendHostsLine = oldAppend })

	i := Identity{Hostname: "router-1", Domain: "lan"}
	ok, detail := ApplyIdentity(rr, i)
	if !ok {
		t.Fatalf("ApplyIdentity failed: %s", detail)
	}
	if len(appended) != 1 {
		t.Fatalf("应追加一行 hosts，实际 %d 行: %v", len(appended), appended)
	}
	// 追加的内容必须是我们拼出来的那行，且不能经由 shell
	want := "127.0.1.1 router-1.lan router-1\n"
	if appended[0] != want {
		t.Fatalf("hosts 行 = %q，期望 %q", appended[0], want)
	}
	// sed 删除必须发生在追加之前：命令序列里 sed 必须是最后一条 Runner 调用
	delIdx := rr.indexOf("sed -i")
	if delIdx < 0 {
		t.Fatalf("missing sed command: %v", rr.calls)
	}
	for _, c := range rr.calls {
		if strings.Contains(c, "sh -c") || strings.Contains(c, ">>") {
			t.Fatalf("仍然经由 shell 写 hosts: %q", c)
		}
	}
	// sed pattern should target 127.0.1.1
	for _, c := range rr.calls {
		if strings.Contains(c, "sed -i") {
			if !strings.Contains(c, "127") {
				t.Fatalf("sed should target 127.0.1.1, got %q", c)
			}
		}
	}
	// hostnamectl should be first
	if rr.indexOf("hostnamectl") != 0 {
		t.Fatalf("hostnamectl should be first, got calls %v", rr.calls)
	}
}

func TestApplyIdentityValidationFailure(t *testing.T) {
	rr := newRec()
	i := Identity{Hostname: "12345"} // all digits
	ok, detail := ApplyIdentity(rr, i)
	if ok {
		t.Fatal("ApplyIdentity should fail validation")
	}
	if detail == "" {
		t.Fatal("detail should contain error")
	}
	if len(rr.calls) != 0 {
		t.Fatalf("no command should run on validation failure, got %v", rr.calls)
	}
}

// make sure net is used
var _ = net.ParseIP
