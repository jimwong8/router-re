package acl

import (
	"strings"
	"testing"
)

// recorder 复用 acl_test.go 里的 fakeRunner，只把调用记下来。
type recorder struct {
	calls [][]string
}

func newRecorder() (*fakeRunner, *recorder) {
	r := &recorder{}
	f := &fakeRunner{}
	f.runHook = func(name string, args ...string) (string, error) {
		r.calls = append(r.calls, append([]string{name}, args...))
		// sysctl -n <key> 是回读校验，得返回刚写进去的值，
		// 否则会被判成 verify mismatch，走不到我们想断言的分支。
		if len(args) >= 2 && args[0] == "-n" {
			key := args[1]
			if strings.HasSuffix(key, "arp_ignore") {
				return "1", nil
			}
			if strings.HasSuffix(key, "arp_announce") {
				return "2", nil
			}
		}
		return "", nil
	}
	return f, r
}

// 网卡名会拼进 sysctl 键名。带 shell 元字符的名字必须在校验层就被拒，
// 否则 `all; touch /tmp/x; echo` 会被 sh -c 当成两条命令执行（已实测可落地）。
func TestValidateIfaceRejectsShellMetacharacters(t *testing.T) {
	bad := []string{
		"all; touch /tmp/pwned; echo",
		"eth0 && id",
		"eth0|id",
		"$(id)",
		"`id`",
		"eth0\nid",
		"eth0'",
		`eth0"`,
		"a b",
		"eth0> /etc/passwd",
		strings.Repeat("a", 33),
	}
	for _, name := range bad {
		if err := ValidateIface(name); err == nil {
			t.Fatalf("非法网卡名 %q 通过了校验", name)
		}
	}

	good := []string{"all", "eth0", "br-lan", "eth0.2", "wlan0", "vgh370000118", "wg0", "br_lan", "eth0:1"}
	for _, name := range good {
		if err := ValidateIface(name); err != nil {
			t.Fatalf("合法网卡名 %q 被拒: %v", name, err)
		}
	}
}

// ApplyArpGuard 对非法 iface 必须直接返回错误，且一次命令都不执行。
func TestApplyArpGuardRejectsBeforeExec(t *testing.T) {
	fr, rec := newRecorder()
	err := ApplyArpGuard(fr, "all; touch /tmp/pwned; echo")
	if err == nil {
		t.Fatal("非法 iface 没有返回错误")
	}
	if len(rec.calls) != 0 {
		t.Fatalf("校验失败却已经执行了 %d 条命令: %v", len(rec.calls), rec.calls)
	}
}

// 合法路径也不能经过 shell：每条命令都必须是 execx 直接调用。
func TestApplyArpGuardNeverUsesShell(t *testing.T) {
	fr, rec := newRecorder()
	if err := ApplyArpGuard(fr, "eth0"); err != nil {
		t.Fatalf("ApplyArpGuard: %v", err)
	}
	if len(rec.calls) == 0 {
		t.Fatal("没有执行任何命令")
	}
	for _, c := range rec.calls {
		if len(c) == 0 {
			continue
		}
		base := c[0]
		if strings.HasSuffix(base, "sh") || strings.HasSuffix(base, "bash") {
			t.Fatalf("命令仍然经过 shell: %v", c)
		}
		if strings.Contains(strings.Join(c, " "), ";") {
			t.Fatalf("命令参数里出现了分号，可能被解释为命令分隔: %v", c)
		}
	}
	// 至少要真的调用了 sysctl
	var sawSysctl bool
	for _, c := range rec.calls {
		if strings.HasSuffix(c[0], "sysctl") && len(c) == 3 && c[1] == "-w" {
			sawSysctl = true
			if !strings.Contains(c[2], "arp_ignore") && !strings.Contains(c[2], "arp_announce") {
				t.Fatalf("sysctl 参数不像 ARP 加固: %v", c)
			}
		}
	}
	if !sawSysctl {
		t.Fatalf("没有看到 sysctl -w 调用: %v", rec.calls)
	}
}
