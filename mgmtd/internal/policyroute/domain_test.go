package policyroute

import (
	"strings"
	"testing"
)

func TestValidateDomainRulesNormal(t *testing.T) {
	rs := []DomainRule{
		{ID: "1", Domain: "example.com", Action: "route", Table: 1, Fwmark: 100, Priority: 1, Enabled: true},
		{ID: "2", Domain: "*.example.com", Action: "block", Table: 2, Fwmark: 0, Priority: 2, Enabled: false},
	}
	if err := ValidateDomainRules(rs); err != nil {
		t.Fatalf("normal rules should pass: %v", err)
	}
}

func TestValidateDomainRulesEmptyDomainEnabled(t *testing.T) {
	rs := []DomainRule{
		{Domain: "", Action: "route", Table: 1, Enabled: true},
	}
	if err := ValidateDomainRules(rs); err == nil {
		t.Fatal("empty domain should be rejected")
	}
}

func TestValidateDomainRulesSpaceDomain(t *testing.T) {
	rs := []DomainRule{
		{Domain: " example.com", Action: "route", Table: 1, Enabled: true},
	}
	if err := ValidateDomainRules(rs); err == nil {
		t.Fatal("domain with leading space should be rejected")
	}
}

func TestValidateDomainRulesLongDomain(t *testing.T) {
	longDomain := "a." + string(make([]byte, 245)) // 247 chars
	// generate exactly 254 chars
	var sb []byte
	sb = append(sb, "a."...)
	for i := 0; i < 252; i++ {
		sb = append(sb, byte('a'))
	}
	longDomain = string(sb) // 254 chars
	rs := []DomainRule{
		{Domain: longDomain, Action: "route", Table: 1, Enabled: true},
	}
	if err := ValidateDomainRules(rs); err == nil {
		t.Fatal("domain longer than 253 should be rejected")
	}
}

func TestValidateDomainRulesInvalidAction(t *testing.T) {
	rs := []DomainRule{
		{Domain: "example.com", Action: "invalid", Table: 1, Enabled: true},
	}
	if err := ValidateDomainRules(rs); err == nil {
		t.Fatal("invalid action should be rejected")
	}
}

func TestValidateDomainRulesRouteMissingTable(t *testing.T) {
	rs := []DomainRule{
		{Domain: "example.com", Action: "route", Table: 0, Enabled: true},
	}
	if err := ValidateDomainRules(rs); err == nil {
		t.Fatal("route action must have table")
	}
}

func TestValidateDomainRulesTableOutOfRange(t *testing.T) {
	rs := []DomainRule{
		{Domain: "example.com", Action: "route", Table: 253, Enabled: true},
	}
	if err := ValidateDomainRules(rs); err == nil {
		t.Fatal("table out of range should be rejected")
	}
	rs2 := []DomainRule{
		{Domain: "example.com", Action: "route", Table: 300, Enabled: true},
	}
	if err := ValidateDomainRules(rs2); err == nil {
		t.Fatal("table 300 out of range should be rejected")
	}
}

func TestValidateDomainRulesDuplicateEnabledDomain(t *testing.T) {
	rs := []DomainRule{
		{Domain: "example.com", Action: "route", Table: 1, Enabled: true},
		{Domain: "example.com", Action: "block", Table: 2, Enabled: true},
	}
	if err := ValidateDomainRules(rs); err == nil {
		t.Fatal("two enabled rules for same domain should be rejected")
	}
}

func TestValidateDomainRulesDuplicateEnabledWithDisabledAllowed(t *testing.T) {
	// 一条 Enabled 同域名 + 一条 Disabled 同域名，应该允许
	rs := []DomainRule{
		{Domain: "example.com", Action: "route", Table: 1, Enabled: true},
		{Domain: "example.com", Action: "block", Table: 2, Enabled: false},
	}
	if err := ValidateDomainRules(rs); err != nil {
		t.Fatalf("one enabled + one disabled for same domain should be allowed, got: %s", err.Error())
	}
}

func TestValidateDomainRulesMaxCount(t *testing.T) {
	rs := make([]DomainRule, 513)
	for i := range rs {
		rs[i] = DomainRule{
			ID:       "",
			Domain:   "example.com",
			Action:   "route",
			Table:    1,
			Fwmark:   0,
			Priority: i,
			Enabled:  true,
		}
	}
	if err := ValidateDomainRules(rs); err == nil {
		t.Fatal("more than 512 rules should be rejected")
	}
}

func TestSetName(t *testing.T) {
	tests := []struct {
		idx  int
		want string
	}{
		{0, "rr_dom_0"},
		{1, "rr_dom_1"},
		{123, "rr_dom_123"},
	}
	for _, tt := range tests {
		got := SetName(tt.idx)
		if got != tt.want {
			t.Errorf("SetName(%d) = %q, want %q", tt.idx, got, tt.want)
		}
	}
}

func TestRenderDnsmasqIpset(t *testing.T) {
	rs := []DomainRule{
		{Domain: "example.com", Action: "route", Table: 1, Enabled: true},
		{Domain: "*.example.com", Action: "block", Table: 2, Enabled: true},
		{Domain: "test.org", Action: "route", Table: 3, Enabled: false},
	}
	out := RenderDnsmasqIpset(rs)
	lines := 0
	for _, line := range strings.Split(out, "\n") {
		if line != "" {
			lines++
		}
	}
	if lines != 2 {
		t.Fatalf("RenderDnsmasqIpset should output 2 lines for enabled rules, got %d", lines)
	}
	// *.example.com 渲染成 example.com：dnsmasq 的 /domain/ 本身覆盖子域，
	// 带星号或前导点是无效语法
	if !strings.Contains(out, "ipset=/example.com/") {
		t.Fatalf("RenderDnsmasqIpset for star prefix should contain ipset=/example.com/, got: %s", out)
	}
	if strings.Contains(out, "*") || strings.Contains(out, "/.example.com/") {
		t.Fatalf("星号或前导点不应出现在 dnsmasq 片段里: %s", out)
	}
	// 未启用的 test.org 不应输出
	if strings.Contains(out, "ipset=//test.org") || strings.Contains(out, "ipset=/test.org") {
		t.Fatalf("disabled rule should not appear in output, got: %s", out)
	}
}

func TestRenderDnsmasqIpsetDisabledOnly(t *testing.T) {
	rs := []DomainRule{
		{Domain: "example.com", Action: "route", Table: 1, Enabled: false},
	}
	out := RenderDnsmasqIpset(rs)
	if out != "" {
		t.Fatalf("no enabled rules should return empty string, got: %s", out)
	}
}

func TestRenderNft(t *testing.T) {
	rs := AssignMarks([]DomainRule{
		{Domain: "example.com", Action: "route", Table: 1, Fwmark: 100, Priority: 1, Enabled: true},
		{Domain: "test.org", Action: "block", Table: 2, Fwmark: 200, Priority: 1, Enabled: true},
		{Domain: "aaa.com", Action: "route", Table: 3, Fwmark: 0, Priority: 2, Enabled: true},
	})
	tableByName := func(tableID int) string {
		if tableID == 1 {
			return "wan1table"
		}
		return ""
	}
	out, err := RenderNft(rs, tableByName)
	if err != nil {
		t.Fatalf("RenderNft should not error: %v", err)
	}
	// 必须是能直接 nft -f 的完整配置，而不是裸规则行
	for _, want := range []string{
		"table inet rr_dom {",
		"set rr_dom_0 {", "type ipv4_addr", "flags timeout",
		"chain prerouting {", "type filter hook prerouting priority mangle",
		"meta mark set", "drop", "# table wan1table",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("RenderNft 缺少 %q:\n%s", want, out)
		}
	}
	// 大括号必须配平，否则 nft -f 会语法错误
	if strings.Count(out, "{") != strings.Count(out, "}") {
		t.Fatalf("nft 配置括号不配平:\n%s", out)
	}
}


func TestRenderNftOrderByPriority(t *testing.T) {
	rs := AssignMarks([]DomainRule{
		{Domain: "zzz.com", Action: "route", Table: 1, Priority: 30, Enabled: true},
		{Domain: "aaa.com", Action: "route", Table: 1, Priority: 10, Enabled: true},
		{Domain: "mmm.com", Action: "route", Table: 1, Priority: 20, Enabled: true},
	})
	out, err := RenderNft(rs, nil)
	if err != nil {
		t.Fatalf("RenderNft should not error: %v", err)
	}
	iA := strings.Index(out, "aaa.com") // 只出现在注释里没有域名，改按 set 顺序判断
	_ = iA
	// 规则按 priority 升序：aaa(10) → mmm(20) → zzz(30)，按集合名对应
	names := collectionNames(rs)
	order := []string{names["aaa.com"], names["mmm.com"], names["zzz.com"]}
	last := -1
	for _, n := range order {
		i := strings.Index(out, "ip daddr @"+n+" ")
		if i < 0 {
			t.Fatalf("规则里找不到集合 %s:\n%s", n, out)
		}
		if i < last {
			t.Fatalf("规则顺序未按优先级升序:\n%s", out)
		}
		last = i
	}
}


func TestRenderIpRules(t *testing.T) {
	rs := []DomainRule{
		{Domain: "example.com", Action: "route", Table: 1, Fwmark: 100, Priority: 1, Enabled: true},
		{Domain: "test.org", Action: "route", Table: 2, Fwmark: 200, Priority: 2, Enabled: true},
		{Domain: "block.com", Action: "block", Table: 3, Fwmark: 0, Priority: 1, Enabled: true},
	}
	out := RenderIpRules(rs)
	if len(out) != 2 {
		t.Fatalf("RenderIpRules should output 2 rules for Enabled+route, got %d", len(out))
	}
	// 检查包含 lookup 和 priority
	if !strings.Contains(out[0], "lookup") || !strings.Contains(out[0], "priority") {
		t.Fatalf("RenderIpRules should contain lookup and priority, got: %s", out[0])
	}
}

func TestAssignMarks(t *testing.T) {
	rs := []DomainRule{
		{Domain: "a.com", Action: "route", Table: 1, Fwmark: 0, Priority: 1, Enabled: true},
		{Domain: "b.com", Action: "route", Table: 2, Fwmark: 50, Priority: 2, Enabled: true},
		{Domain: "c.com", Action: "route", Table: 3, Fwmark: 0, Priority: 3, Enabled: true},
	}
	original := make([]DomainRule, len(rs))
	copy(original, rs)
	result := AssignMarks(rs)
	// 检查入参未修改
	for i := range original {
		if original[i].Fwmark != rs[i].Fwmark {
			t.Fatalf("AssignMarks modified input parameter at index %d", i)
		}
	}
	// Fwmark=0 的规则按位置分配 100+index（zeroMarkIndices 中的 idx）
	// a.com (position 0) 是第一个 Fwmark=0 → 100
	// c.com (position 2) 是第二个 Fwmark=0 → 101
	if result[0].Fwmark != 100 {
		t.Fatalf("result[0].Fwmark should be 100, got %d", result[0].Fwmark)
	}
	if result[2].Fwmark != 101 {
		t.Fatalf("result[2].Fwmark should be 101, got %d", result[2].Fwmark)
	}
	// b.com 原本有 Fwmark=50，不应变
	if result[1].Fwmark != 50 {
		t.Fatalf("result[1].Fwmark should remain 50, got %d", result[1].Fwmark)
	}
	// mark 必须全局唯一（nft 与 ip rule 靠它对应），不要求整体单调：
	// 用户手填的 50 有权保留原值，自动分配从 100 起跳过已占用值。
	seen := map[int]bool{}
	for i := range result {
		if seen[result[i].Fwmark] {
			t.Fatalf("mark %d 重复出现在索引 %d", result[i].Fwmark, i)
		}
		seen[result[i].Fwmark] = true
	}
}
