package porttrigger

import (
	"strings"
	"testing"
)

func validTrigger() Trigger {
	return Trigger{
		Name:        "game",
		TriggerPort: 6112,
		OpenPort:    3956,
		Protocol:    "udp",
		TimeoutSec:  300,
		Enabled:     true,
	}
}

// TestValidateSamePort —— TriggerPort == OpenPort 必须拒绝。
func TestValidateSamePort(t *testing.T) {
	tr := validTrigger()
	tr.TriggerPort = 5000
	tr.OpenPort = 5000
	err := Validate(tr)
	if err == nil {
		t.Fatal("trigger_port == open_port 必须被拒绝")
	}
	// 另一端口不同则通过
	tr.OpenPort = 5001
	if err := Validate(tr); err != nil {
		t.Fatalf("不同端口应通过: %v", err)
	}
}

// TestValidateTimeoutTooLow —— TimeoutSec=30 低于 60，必须拒绝。
func TestValidateTimeoutTooLow(t *testing.T) {
	tr := validTrigger()
	tr.TimeoutSec = 30
	err := Validate(tr)
	if err == nil {
		t.Fatal("timeout_sec=30（<60）必须被拒绝")
	}
	if !strings.Contains(err.Error(), "60") {
		t.Logf("错误信息: %v", err)
	}
	// 边界 60 通过
	tr.TimeoutSec = 60
	if err := Validate(tr); err != nil {
		t.Fatalf("timeout=60 应通过: %v", err)
	}
}

func TestValidateRanges(t *testing.T) {
	tr := validTrigger()
	tr.TriggerPort = 0
	if err := Validate(tr); err == nil {
		t.Fatal("trigger_port=0 应拒绝")
	}
	tr = validTrigger()
	tr.OpenPort = 65536
	if err := Validate(tr); err == nil {
		t.Fatal("open_port=65536 应拒绝")
	}
	tr = validTrigger()
	tr.Protocol = "icmp"
	if err := Validate(tr); err == nil {
		t.Fatal("非法 protocol 应拒绝")
	}
	tr = validTrigger()
	tr.Name = ""
	if err := Validate(tr); err == nil {
		t.Fatal("空 name 应拒绝")
	}
	// TriggerEnd 区间合法
	tr = validTrigger()
	tr.TriggerEnd = 6120
	if err := Validate(tr); err != nil {
		t.Fatalf("合法 TriggerEnd 区间应通过: %v", err)
	}
}

// TestRenderTableNoPolicyDrop —— 项目铁律回归测试：绝不能出现 policy drop。
func TestRenderTableNoPolicyDrop(t *testing.T) {
	out := RenderTable([]Trigger{validTrigger()})
	if strings.Contains(out, "policy drop") {
		t.Fatalf("RenderTable 绝不能含 policy drop:\n%s", out)
	}
	if !strings.Contains(out, "policy accept") {
		t.Fatalf("应显式 policy accept:\n%s", out)
	}
}

// TestRenderTableNoEmptyElements —— 空集合语法错误：绝不能出现 elements = {}。
func TestRenderTableNoEmptyElements(t *testing.T) {
	out := RenderTable([]Trigger{validTrigger()})
	if strings.Contains(out, "elements = {}") {
		t.Fatalf("RenderTable 绝不能含空的 elements = {}（语法错误）:\n%s", out)
	}
	// 应有触发内容
	if !strings.Contains(out, "trigger") {
		t.Fatalf("应含 trigger chain:\n%s", out)
	}
}

// TestRenderTableAllDisabledEmpty —— 全部禁用时返回空字符串。
func TestRenderTableAllDisabledEmpty(t *testing.T) {
	tr := validTrigger()
	tr.Enabled = false
	out := RenderTable([]Trigger{tr})
	if out != "" {
		t.Fatalf("全部禁用应返回空字符串, got:\n%s", out)
	}
	// 空输入也返回空
	if RenderTable(nil) != "" {
		t.Fatal("空输入应返回空字符串")
	}
}

func TestRenderTableTimeoutIncluded(t *testing.T) {
	tr := validTrigger()
	tr.TimeoutSec = 300
	out := RenderTable([]Trigger{tr})
	if !strings.Contains(out, "timeout 300s") {
		t.Fatalf("应含 timeout 300s:\n%s", out)
	}
}

// TestRenderALGEmptyAndSecurityNote —— 全 false 返回空字符串；PPTP 开启时含安全提示。
func TestRenderALGEmptyAndSecurityNote(t *testing.T) {
	if RenderALG(ALGConfig{}) != "" {
		t.Fatal("全 false 应返回空字符串")
	}
	cfg := ALGConfig{FTP: true, PPTP: true, SIP: true}
	out := RenderALG(cfg)
	if out == "" {
		t.Fatal("启用 ALG 不应为空")
	}
	if !strings.Contains(out, "nf_conntrack_ftp") {
		t.Fatalf("应含 ftp helper:\n%s", out)
	}
	if !strings.Contains(out, "nf_conntrack_pptp") {
		t.Fatalf("应含 pptp helper:\n%s", out)
	}
	// PPTP/SIP 安全提示
	if !strings.Contains(out, "WARNING") || !strings.Contains(strings.ToLower(out), "安全") {
		t.Fatalf("PPTP/SIP 应含安全提示:\n%s", out)
	}
}

// TestValidateMDNSRejectsSameInOut —— in 和 out 完全相同（组播环路）。
func TestValidateMDNSRejectsSameInOut(t *testing.T) {
	m := MDNSRelay{
		InIfaces:  []string{"eth1", "eth2"},
		OutIfaces: []string{"eth2", "eth1"},
		Enabled:   true,
	}
	err := ValidateMDNS(m)
	if err == nil {
		t.Fatal("in 与 out 完全相同必须拒绝（组播环路）")
	}
	if !strings.Contains(err.Error(), "组播") && !strings.Contains(err.Error(), "环路") {
		t.Fatalf("应说明组播环路后果, got: %v", err)
	}
	// 不完全相同则通过
	m.OutIfaces = []string{"eth2"}
	if err := ValidateMDNS(m); err != nil {
		t.Fatalf("in/out 不同应通过: %v", err)
	}
}

// TestValidateMDNSRejectsLo —— 含 lo 拒绝。
func TestValidateMDNSRejectsLo(t *testing.T) {
	m := MDNSRelay{InIfaces: []string{"lo"}, OutIfaces: []string{"eth1"}, Enabled: true}
	if err := ValidateMDNS(m); err == nil {
		t.Fatal("in 含 lo 必须拒绝")
	}
	m = MDNSRelay{InIfaces: []string{"eth1"}, OutIfaces: []string{"lo"}, Enabled: true}
	if err := ValidateMDNS(m); err == nil {
		t.Fatal("out 含 lo 必须拒绝")
	}
}

func TestValidateMDNSMissingIfaces(t *testing.T) {
	m := MDNSRelay{InIfaces: nil, OutIfaces: []string{"eth1"}, Enabled: true}
	if err := ValidateMDNS(m); err == nil {
		t.Fatal("缺 in 接口应拒绝")
	}
	m = MDNSRelay{InIfaces: []string{"eth1"}, OutIfaces: nil, Enabled: true}
	if err := ValidateMDNS(m); err == nil {
		t.Fatal("缺 out 接口应拒绝")
	}
}

// TestRenderAvahiContainsAllowInterfaces —— 含 allow-interfaces。
func TestRenderAvahiContainsAllowInterfaces(t *testing.T) {
	m := MDNSRelay{InIfaces: []string{"eth1"}, OutIfaces: []string{"eth2"}, Enabled: true}
	out := RenderAvahi(m)
	if !strings.Contains(out, "[server]") {
		t.Fatalf("应含 [server] 段:\n%s", out)
	}
	if !strings.Contains(out, "use-ipv4=yes") {
		t.Fatalf("应含 use-ipv4=yes:\n%s", out)
	}
	if !strings.Contains(out, "allow-interfaces=eth1,eth2") {
		t.Fatalf("应含 allow-interfaces=eth1,eth2:\n%s", out)
	}
	// 禁用时为空
	if RenderAvahi(MDNSRelay{Enabled: false}) != "" {
		t.Fatal("禁用时应返回空字符串")
	}
}
