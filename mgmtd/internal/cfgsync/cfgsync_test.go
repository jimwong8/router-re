package cfgsync

import (
	"encoding/json"
	"strings"
	"testing"
)

func raw(s string) json.RawMessage { return json.RawMessage(s) }

// TestComputeChecksumOrderIndependent —— 两个 Section 插入顺序不同的 bundle，
// checksum 必须相同（map 遍历顺序无关，最容易错的地方）。
func TestComputeChecksumOrderIndependent(t *testing.T) {
	secA := map[string]json.RawMessage{
		"firewall": raw(`{"default":{"input":"drop"}}`),
		"network":  raw(`{"lan":{"ipaddr":"192.168.1.1"}}`),
	}
	secB := map[string]json.RawMessage{
		"network":  raw(`{"lan":{"ipaddr":"192.168.1.1"}}`),
		"firewall": raw(`{"default":{"input":"drop"}}`),
	}
	b1 := NewBundle("router1", 1000, secA)
	b2 := NewBundle("router1", 1000, secB)
	if b1.ComputeChecksum() != b2.ComputeChecksum() {
		t.Fatalf("插入顺序不同导致 checksum 不同:\n%s\nvs\n%s", b1.ComputeChecksum(), b2.ComputeChecksum())
	}
}

// TestVerifyRejectsTampering —— 篡改任一 section 后 Verify 返回 false。
func TestVerifyRejectsTampering(t *testing.T) {
	b := NewBundle("router1", 1000, map[string]json.RawMessage{
		"network": raw(`{"lan":{"ipaddr":"192.168.1.1"}}`),
	})
	// 深拷贝 Sections map（浅拷贝会共享 map，篡改会连带改掉原 bundle）
	tampered := b
	tampered.Sections = make(map[string]json.RawMessage, len(b.Sections))
	for k, v := range b.Sections {
		tampered.Sections[k] = v
	}
	// 篡改 section 内容（改一个字段值）
	tampered.Sections["network"] = raw(`{"lan":{"ipaddr":"10.0.0.1"}}`)
	if tampered.Verify() {
		t.Fatal("篡改 section 后 Verify 应返回 false")
	}
	// 篡改 hostname
	tampered2 := b
	tampered2.Hostname = "evil"
	if tampered2.Verify() {
		t.Fatal("篡改 hostname 后 Verify 应返回 false")
	}
	// 原 bundle 仍应验证通过
	if !b.Verify() {
		t.Fatal("未篡改的 bundle 应验证通过")
	}
}

// TestValidateBundleRejectsVersion —— 拒绝 Version=2。
func TestValidateBundleRejectsVersion(t *testing.T) {
	b := NewBundle("router1", 1000, map[string]json.RawMessage{
		"network": raw(`{"lan":{"ipaddr":"192.168.1.1"}}`),
	})
	b.Version = 2
	if err := ValidateBundle(b); err == nil {
		t.Fatal("Version=2 应被拒绝")
	} else if !strings.Contains(err.Error(), "version") {
		t.Fatalf("拒绝理由应说明版本: %v", err)
	}
}

// TestValidateBundleRejectsEmptySections —— 拒绝空 Sections。
func TestValidateBundleRejectsEmptySections(t *testing.T) {
	b := NewBundle("router1", 1000, map[string]json.RawMessage{})
	if err := ValidateBundle(b); err == nil {
		t.Fatal("空 Sections 应被拒绝")
	}
}

// TestValidateBundleRejectsChecksumMismatch —— 拒绝 checksum 不匹配（篡改后导入）。
func TestValidateBundleRejectsChecksumMismatch(t *testing.T) {
	b := NewBundle("router1", 1000, map[string]json.RawMessage{
		"network": raw(`{"lan":{"ipaddr":"192.168.1.1"}}`),
	})
	b.Sections["network"] = raw(`{"lan":{"ipaddr":"10.0.0.1"}}`) // 篡改后 checksum 不再匹配
	if err := ValidateBundle(b); err == nil {
		t.Fatal("checksum 不匹配应被拒绝")
	} else if !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("拒绝理由应说明 checksum: %v", err)
	}
}

// TestValidateBundleOK —— 合法 bundle 应通过。
func TestValidateBundleOK(t *testing.T) {
	b := NewBundle("router1", 1000, map[string]json.RawMessage{
		"network": raw(`{"lan":{"ipaddr":"192.168.1.1"}}`),
	})
	if err := ValidateBundle(b); err != nil {
		t.Fatalf("合法 bundle 不应报错: %v", err)
	}
}

// TestFlattenNested —— 嵌套 JSON 扁平化：{"a":{"b":1}} -> {"a.b":"1"}。
func TestFlattenNested(t *testing.T) {
	flat, err := Flatten(raw(`{"a":{"b":1,"c":{"d":true}}} `), "")
	if err != nil {
		t.Fatalf("flatten error: %v", err)
	}
	if flat["a.b"] != "1" {
		t.Fatalf("a.b=%q (want 1)", flat["a.b"])
	}
	if flat["a.c.d"] != "true" {
		t.Fatalf("a.c.d=%q (want true)", flat["a.c.d"])
	}
}

// TestDiffKinds —— Diff 识别 added / removed / changed 三种。
func TestDiffKinds(t *testing.T) {
	oldB := NewBundle("r", 1, map[string]json.RawMessage{
		"network":  raw(`{"lan":{"ipaddr":"192.168.1.1","gateway":"192.168.1.254"}}`),
		"firewall": raw(`{"default":{"input":"drop"}}`),
	})
	newB := NewBundle("r", 1, map[string]json.RawMessage{
		"network": raw(`{"lan":{"ipaddr":"10.0.0.1","dns":"8.8.8.8"}}`),
		"dhcp":    raw(`{"lan":{"start":"100"}}`),
	})
	changes := Diff(oldB, newB)
	got := map[string]string{}
	for _, c := range changes {
		got[c.Section+"."+c.Key] = c.Kind
	}
	// lan.ipaddr changed
	if got["network.lan.ipaddr"] != "changed" {
		t.Fatalf("network.lan.ipaddr 应为 changed, got %q", got["network.lan.ipaddr"])
	}
	// lan.gateway removed（new 无）
	if got["network.lan.gateway"] != "removed" {
		t.Fatalf("network.lan.gateway 应为 removed, got %q", got["network.lan.gateway"])
	}
	// lan.dns added（new 新增）
	if got["network.lan.dns"] != "added" {
		t.Fatalf("network.lan.dns 应为 added, got %q", got["network.lan.dns"])
	}
	// firewall 整个移除 -> 字段 removed
	if got["firewall.default.input"] != "removed" {
		t.Fatalf("firewall.default.input 应为 removed, got %q", got["firewall.default.input"])
	}
	// dhcp 整个新增 -> 字段 added
	if got["dhcp.lan.start"] != "added" {
		t.Fatalf("dhcp.lan.start 应为 added, got %q", got["dhcp.lan.start"])
	}
}

// TestDiffSortedStable —— 输出按 Section+Key 排序，两次调用结果相同。
func TestDiffSortedStable(t *testing.T) {
	oldB := NewBundle("r", 1, map[string]json.RawMessage{
		"network":  raw(`{"lan":{"ipaddr":"1.1.1.1"},"wan":{"ipaddr":"2.2.2.2"}}`),
		"firewall": raw(`{"default":{"input":"drop"}}`),
	})
	newB := NewBundle("r", 1, map[string]json.RawMessage{
		"network":  raw(`{"wan":{"ipaddr":"3.3.3.3"},"lan":{"ipaddr":"1.1.1.1"}}`),
		"firewall": raw(`{"default":{"input":"accept"}}`),
	})
	c1 := Diff(oldB, newB)
	c2 := Diff(oldB, newB)
	// 两次调用长度一致
	if len(c1) != len(c2) {
		t.Fatalf("两次 diff 长度不同: %d vs %d", len(c1), len(c2))
	}
	// 验证排序：Section 升序，同 Section 按 Key 升序
	for i := 1; i < len(c1); i++ {
		a, b := c1[i-1], c1[i]
		if a.Section > b.Section || (a.Section == b.Section && a.Key > b.Key) {
			t.Fatalf("diff 未排序: %+v 在 %+v 之前", a, b)
		}
	}
	// 内容一致（逐字段）
	for i := range c1 {
		if c1[i] != c2[i] {
			t.Fatalf("两次 diff 内容不同: %+v vs %+v", c1[i], c2[i])
		}
	}
}

// TestBuildRestorePlanDangerous —— 网络相关 section 产出 Dangerous 说明。
func TestBuildRestorePlanDangerous(t *testing.T) {
	current := NewBundle("r", 1, map[string]json.RawMessage{
		"network": raw(`{"lan":{"ipaddr":"192.168.1.1"}}`),
		"system":  raw(`{"hostname":"old"}`),
	})
	incoming := NewBundle("r", 2, map[string]json.RawMessage{
		"network":   raw(`{"lan":{"ipaddr":"10.0.0.1"}}`),
		"interface": raw(`{"wan":{"proto":"pppoe"}}`),
		"system":    raw(`{"hostname":"new"}`),
	})
	plan, err := BuildRestorePlan(current, incoming)
	if err != nil {
		t.Fatalf("build plan error: %v", err)
	}
	foundNetwork := false
	foundInterface := false
	for _, d := range plan.Dangerous {
		if strings.Contains(d, "网络") && (strings.Contains(d, "失联") || strings.Contains(d, "不符")) {
			foundNetwork = true
		}
		if strings.Contains(d, "接口") && strings.Contains(d, "失联") {
			foundInterface = true
		}
	}
	if !foundNetwork {
		t.Fatalf("network section 应产出网络危险说明, got %v", plan.Dangerous)
	}
	if !foundInterface {
		t.Fatalf("interface section 应产出接口失联说明, got %v", plan.Dangerous)
	}
	if !plan.ChecksumOK {
		t.Fatal("incoming 是合法 bundle，ChecksumOK 应为 true")
	}
}

// TestBuildRestorePlanDangerousFirewallNatRoute —— 其余危险类也覆盖。
func TestBuildRestorePlanDangerousFirewallNatRoute(t *testing.T) {
	current := NewBundle("r", 1, map[string]json.RawMessage{})
	incoming := NewBundle("r", 2, map[string]json.RawMessage{
		"firewall": raw(`{"default":{"input":"drop"}}`),
		"nat":      raw(`{"wan":{"masq":"1"}}`),
		"route":    raw(`{"static":{"dst":"10.0.0.0/8"}}`),
		"vlan":     raw(`{"lan":{"id":"5"}}`),
	})
	plan, err := BuildRestorePlan(current, incoming)
	if err != nil {
		t.Fatalf("build plan error: %v", err)
	}
	var all string
	for _, d := range plan.Dangerous {
		all += d + "\n"
	}
	if !strings.Contains(all, "防火墙") || !strings.Contains(all, "NAT") ||
		!strings.Contains(all, "路由") || !strings.Contains(all, "VLAN") {
		t.Fatalf("危险 section 覆盖不全: %s", all)
	}
}

// TestValidateApplyConfirmRequired —— confirm=false 必须报错，绝不静默执行。
func TestValidateApplyConfirmRequired(t *testing.T) {
	plan := Plan{
		Sections:   []string{"network"},
		Changes:    []Change{{Section: "network", Key: "lan.ipaddr", Kind: "changed"}},
		Dangerous:  []string{"将替换网络接口配置"},
		ChecksumOK: true,
	}
	err := ValidateApply(plan, false)
	if err == nil {
		t.Fatal("confirm=false 必须报错（不能静默执行）")
	}
	if !strings.Contains(err.Error(), "confirm=true") {
		t.Fatalf("错误应说明需要 confirm=true, got %v", err)
	}
	// confirm=true 且 plan 合法时通过
	if err := ValidateApply(plan, true); err != nil {
		t.Fatalf("confirm=true 且 plan 有 section 应通过: %v", err)
	}
}
