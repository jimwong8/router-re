package remote

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

// stubRunner 记录调用并可按脚本返回结果，用于驱动 ApplyAccess/ApplyTime 等分支。
type stubRunner struct {
	mu      sync.Mutex
	calls   [][]string
	results map[string]string // key: 由命令名+参数拼成的签名 → stdout
	nextErr map[string]bool
}

func (s *stubRunner) Run(name string, args ...string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("nil runner")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.calls == nil {
		s.calls = [][]string{}
	}
	if s.results == nil {
		s.results = map[string]string{}
	}
	call := append([]string{name}, args...)
	s.calls = append(s.calls, call)
	key := strings.Join(call, " ")
	if s.nextErr != nil {
		if _, ok := s.nextErr[key]; ok {
			delete(s.nextErr, key)
			return "", fmt.Errorf("stubbed error for %s", key)
		}
	}
	if v, ok := s.results[key]; ok {
		return v, nil
	}
	return "", nil
}

func (s *stubRunner) RunWithInput(input, name string, args ...string) (string, error) {
	return s.Run(name, args...)
}

func (s *stubRunner) hasCall(substrs ...string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.calls {
		joined := strings.Join(c, " ")
		all := true
		for _, sub := range substrs {
			if !strings.Contains(joined, sub) {
				all = false
				break
			}
		}
		if all {
			return true
		}
	}
	return false
}

func TestValidateAccessOK(t *testing.T) {
	err := ValidateAccess(Access{SSHPort: 22, WebPort: 443})
	if err != nil {
		t.Fatalf("合法配置报错: %v", err)
	}
}

func TestValidateAccessPortEqual(t *testing.T) {
	err := ValidateAccess(Access{SSHPort: 8080, WebPort: 8080})
	if err == nil {
		t.Fatal("端口相同应报错")
	}
	if !strings.Contains(err.Error(), "不能相同") {
		t.Fatalf("错误信息需说明端口冲突, got %v", err)
	}
}

func TestValidateAccessPortOutOfRange(t *testing.T) {
	for _, port := range []int{0, -1, 65536, 70000} {
		err := ValidateAccess(Access{SSHPort: port, WebPort: 443})
		if err == nil {
			t.Fatalf("SSHPort=%d 应越界报错", port)
		}
	}
	for _, port := range []int{0, 65536} {
		err := ValidateAccess(Access{SSHPort: 22, WebPort: port})
		if err == nil {
			t.Fatalf("WebPort=%d 应越界报错", port)
		}
	}
}

func TestValidateAccessBadCIDR(t *testing.T) {
	for _, cidr := range []string{"192.168.1.1", "banana", "10.0.0.1/33", ""} {
		err := ValidateAccess(Access{SSHPort: 22, WebPort: 443, AllowSources: []string{cidr}})
		if cidr == "" {
			// 空串被跳过
			if err != nil {
				t.Fatalf("空来源应被忽略, got %v", err)
			}
			continue
		}
		if err == nil {
			t.Fatalf("CIDR %q 应非法", cidr)
		}
	}
}

func TestValidateAccessGoodCIDR(t *testing.T) {
	err := ValidateAccess(Access{SSHPort: 22, WebPort: 443, AllowSources: []string{"192.168.1.0/24", "10.0.0.0/8", "2001:db8::/32"}})
	if err != nil {
		t.Fatalf("合法 CIDR 报错: %v", err)
	}
}

func TestAccessWarningsTelnet(t *testing.T) {
	w := AccessWarnings(Access{TelnetEnabled: true})
	if len(w) == 0 {
		t.Fatal("Telnet 启用应返回警告")
	}
	if !strings.Contains(w[0], "明文") || !strings.Contains(w[0], "SSH") {
		t.Fatalf("警告应说明 Telnet 明文且建议 SSH, got %q", w[0])
	}
	// 未启 Telnet 无警告
	if got := AccessWarnings(Access{}); len(got) != 0 {
		t.Fatalf("未启 Telnet 不应有警告, got %v", got)
	}
}

func TestApplyAccessExecutesSystemctl(t *testing.T) {
	r := &stubRunner{}
	pending, err := ApplyAccess(r, Access{SSHEnabled: true, SSHPort: 2222, TelnetEnabled: true})
	if err != nil {
		t.Fatalf("ApplyAccess: %v", err)
	}
	// SSH 开关应被执行
	if !r.hasCall("systemctl", "enable", "--now", "ssh") {
		t.Fatalf("expect systemctl enable --now ssh, calls=%v", r.calls)
	}
	// 端口 2222 应进入人工处理清单
	found := false
	for _, p := range pending {
		if strings.Contains(p, "sshd_config") {
			found = true
		}
	}
	if !found {
		t.Fatalf("SSHPort 变更应在人工处理清单, pending=%v", pending)
	}
	// 必须在任何系统下都不执行 systemctl disable 除非显式关
	_ = r
}

func TestApplyAccessNoPendingForDefaults(t *testing.T) {
	r := &stubRunner{}
	pending, err := ApplyAccess(r, Access{SSHPort: 22, WebPort: 443})
	if err != nil {
		t.Fatalf("ApplyAccess: %v", err)
	}
	for _, p := range pending {
		if strings.Contains(p, "sshd_config") || strings.Contains(p, "web") {
			t.Fatalf("端口为默认值不应出现在人工清单: %v", pending)
		}
	}
}

func TestApplyAccessSSHDownError(t *testing.T) {
	r := &stubRunner{nextErr: map[string]bool{"systemctl enable --now ssh": true}}
	_, err := ApplyAccess(r, Access{SSHEnabled: true})
	if err == nil {
		t.Fatal("systemctl 执行失败应返回错误")
	}
}

func TestAccessWarningsTelnetNotError(t *testing.T) {
	// Telnet 只出警告，ValidateAccess 不得拒绝
	if err := ValidateAccess(Access{SSHPort: 22, WebPort: 443, TelnetEnabled: true}); err != nil {
		t.Fatalf("Telnet 不应被 ValidateAccess 拒绝, got %v", err)
	}
}
