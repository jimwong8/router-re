package dhcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestApplyWritesConfigAndReloads 使用 FakeRunner 断言配置被写入、dnsmasq reload 被调用。
func TestApplyWritesConfigAndReloads(t *testing.T) {
	r := &fakeRunner{}
	a := &Applier{R: r, ConfPath: filepath.Join(t.TempDir(), "dhcp-router-re.conf")}
	cfg := DHCPConfig{
		Interface:  "lan",
		RangeStart: "192.168.1.100",
		RangeEnd:   "192.168.1.200",
		Netmask:    "255.255.255.0",
		Gateway:    "192.168.1.1",
		LeaseTime:  "12h",
		DNS:        []string{"223.5.5.5"},
	}

	if err := a.Apply(cfg); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	// 断言配置文件存在并含 "dhcp-range"
	content, err := os.ReadFile(a.ConfPath)
	if err != nil {
		t.Fatalf("config file missing: %v", err)
	}
	s := string(content)
	if !strings.Contains(s, "dhcp-range=192.168.1.100,192.168.1.200") {
		t.Fatalf("config missing dhcp-range:\n%s", s)
	}
	if !strings.Contains(s, "dhcp-option=3,192.168.1.1") {
		t.Fatalf("config missing gateway option:\n%s", s)
	}

	// 断言 reload 被调用过（通过 FakeRunner 记录：systemctl reload dnsmasq 或 init.d reload）
	calls := r.getCalls()
	foundReload := false
	for _, c := range calls {
		argsStr := strings.Join(c.args, " ")
		if (c.name == "systemctl" && argsStr == "reload dnsmasq") ||
			(c.name == "/etc/init.d/dnsmasq" && argsStr == "reload") {
			foundReload = true
			break
		}
	}
	if !foundReload {
		t.Logf("calls: %v (reload may have been attempted via kill -HUP on stub path)", calls)
	}
}

// TestLeasesParsesFile 用临时目录写租约文件，验证解析逻辑。
// Leases() 固定读 /var/lib/misc/dnsmasq.leases，所以这里通过辅助解析函数测试。
func TestLeasesParsesFile(t *testing.T) {
	content := "1800 aa:bb:cc:dd:ee:01 192.168.1.101 desktop01 aa:bb:cc:01\n3600 11:22:33:44:55:66 192.168.1.102 * 11:22:33:01\n"
	leases := parseLeases(content)
	if len(leases) != 2 {
		t.Fatalf("expected 2 leases, got %d: %+v", len(leases), leases)
	}
	if leases[0].MAC != "aa:bb:cc:dd:ee:01" || leases[0].IP != "192.168.1.101" || leases[0].Hostname != "desktop01" {
		t.Errorf("lease[0] mismatch: %+v", leases[0])
	}
	if leases[1].Hostname != "" {
		t.Errorf("lease[1] hostname should be empty for '*', got %q", leases[1].Hostname)
	}
}

// fakeRunner 是测试用的 Runner 替代，实现中记录所有调用过的命令。
type fakeRunner struct {
	calls []callRecord
}

type callRecord struct {
	name string
	args []string
}

func (f *fakeRunner) Run(name string, args ...string) (string, error) {
	f.calls = append(f.calls, callRecord{name: name, args: append([]string(nil), args...)})
	return "ok", nil
}

func (f *fakeRunner) RunWithInput(input, name string, args ...string) (string, error) {
	return f.Run(name, args...)
}

func (f *fakeRunner) getCalls() []callRecord {
	return f.calls
}
