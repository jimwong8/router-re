package ddos

import (
	"strings"
	"testing"
)

func TestValidateRejectsBadCIDR(t *testing.T) {
	c := Config{PortScanWindow: 10, BlockSeconds: 60, Whitelist: []string{"10.0.0.0/33"}}
	if err := Validate(c); err == nil {
		t.Fatal("invalid CIDR must fail")
	}
}

func TestValidateRejectsZeroSlashZero(t *testing.T) {
	c := Config{PortScanWindow: 10, BlockSeconds: 60, Whitelist: []string{"0.0.0.0/0"}}
	if err := Validate(c); err == nil {
		t.Fatal("0.0.0.0/0 whitelist must fail (disables protection)")
	}
}

func TestValidateRejectsBadAction(t *testing.T) {
	c := Config{
		SynFlood:       Threshold{Pps: 10, Action: "nuke"},
		PortScanWindow: 10,
		BlockSeconds:   60,
	}
	if err := Validate(c); err == nil {
		t.Fatal("invalid action must fail")
	}
}

func TestRenderNoPolicyDrop(t *testing.T) {
	c := Config{
		SynFlood:       Threshold{Pps: 100, Action: "drop"},
		PortScanWindow: 10,
		BlockSeconds:   60,
	}
	out := Render(c)
	if strings.Contains(out, "policy drop") {
		t.Fatalf("render must never contain 'policy drop':\n%s", out)
	}
}

func TestRenderNoEmptyElements(t *testing.T) {
	c := Config{
		SynFlood:       Threshold{Pps: 100, Action: "drop"},
		PortScanWindow: 10,
		BlockSeconds:   60,
	}
	out := Render(c)
	if strings.Contains(out, "elements = {}") {
		t.Fatalf("render must never contain 'elements = {}':\n%s", out)
	}
}

func TestRenderNumbersFromConfig(t *testing.T) {
	c := Config{
		SynFlood:       Threshold{Pps: 12345, Action: "drop"},
		PortScanWindow: 10,
		BlockSeconds:   60,
	}
	out := Render(c)
	if !strings.Contains(out, "12345") {
		t.Fatalf("render must use configured pps 12345:\n%s", out)
	}
}

func TestIsWhitelistedExact(t *testing.T) {
	if !IsWhitelisted("1.2.3.4", []string{"1.2.3.4"}) {
		t.Fatal("exact IP must match")
	}
	if IsWhitelisted("1.2.3.5", []string{"1.2.3.4"}) {
		t.Fatal("non-matching IP must not match")
	}
}

func TestIsWhitelistedCIDR(t *testing.T) {
	if !IsWhitelisted("10.0.0.5", []string{"10.0.0.0/24"}) {
		t.Fatal("IP in CIDR must match")
	}
	if IsWhitelisted("11.0.0.5", []string{"10.0.0.0/24"}) {
		t.Fatal("IP outside CIDR must not match")
	}
}

func TestIsWhitelistedInvalid(t *testing.T) {
	if IsWhitelisted("not-an-ip", []string{"1.2.3.4"}) {
		t.Fatal("invalid IP must return false")
	}
	if IsWhitelisted("1.2.3.4", []string{"garbage"}) {
		t.Fatal("invalid whitelist entry must not crash/panic; result false expected but here just no panic")
	}
}

// TestValidateWhitelistAllZeroIsReportedFirst 锁死错误信息的精确性：
// 0.0.0.0/0 是"关闭全部防护"的致命配置，必须优先报出来，
// 不能被 port_scan_window=0 这种次要缺省值抢先遮住 —— 否则运维看到的
// 是"窗口参数不对"，改完再提交才发现真正的问题是整网白名单。
func TestValidateWhitelistAllZeroIsReportedFirst(t *testing.T) {
	err := Validate(Config{Whitelist: []string{"0.0.0.0/0"}})
	if err == nil {
		t.Fatal("0.0.0.0/0 必须被拒绝")
	}
	msg := err.Error()
	if !strings.Contains(msg, "0.0.0.0/0") {
		t.Fatalf("错误信息应指出 0.0.0.0/0 本身，实际: %s", msg)
	}
	if strings.Contains(msg, "port_scan_window") {
		t.Fatalf("被无关字段抢先报错了，实际: %s", msg)
	}
}
