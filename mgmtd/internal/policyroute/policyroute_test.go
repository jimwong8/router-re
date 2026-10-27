package policyroute

import (
	"strings"
	"testing"
)

func validTable() Table {
	return Table{
		ID:   100,
		Name: "wan1table",
		Routes: []Route{
			{Dst: "10.0.0.0/8", Via: "192.168.1.1", Dev: "eth1", Metric: 10},
		},
	}
}

// TestValidateTableRejectsReservedID —— 拒绝内核保留表 253/254/255，错误要说明后果。
func TestValidateTableRejectsReservedID(t *testing.T) {
	for _, id := range []int{253, 254, 255} {
		tb := validTable()
		tb.ID = id
		err := ValidateTable(tb)
		if err == nil {
			t.Fatalf("table id=%d 必须被拒绝", id)
		}
		if !strings.Contains(err.Error(), "253") || !strings.Contains(err.Error(), "保留") {
			t.Logf("错误信息: %v", err)
		}
		if !strings.Contains(err.Error(), "失联") && !strings.Contains(err.Error(), "全乱") {
			t.Fatalf("拒绝 id=%d 应说明内核保留表的后果, got: %v", id, err)
		}
	}
}

// TestValidateTableRejectsOutOfRange —— 拒绝 ID=0、ID=300。
func TestValidateTableRejectsOutOfRange(t *testing.T) {
	for _, id := range []int{0, 300, -1} {
		tb := validTable()
		tb.ID = id
		if err := ValidateTable(tb); err == nil {
			t.Fatalf("table id=%d 必须被拒绝", id)
		}
	}
}

func TestValidateTableBadName(t *testing.T) {
	tb := validTable()
	tb.Name = "bad name!"
	if err := ValidateTable(tb); err == nil {
		t.Fatal("含非法字符的表名应被拒绝")
	}
}

func TestValidateTableOK(t *testing.T) {
	if err := ValidateTable(validTable()); err != nil {
		t.Fatalf("合法表不应报错: %v", err)
	}
}

// TestValidateRuleRejectsPriorityZero —— priority=0 抢占 local 表。
func TestValidateRuleRejectsPriorityZero(t *testing.T) {
	// from 非空让其余条件满足，只测 priority 红线
	r := Rule{Priority: 0, From: "10.0.0.0/8", Table: 100}
	err := ValidateRule(r)
	if err == nil {
		t.Fatal("priority=0 必须被拒绝")
	}
	if !strings.Contains(err.Error(), "local") && !strings.Contains(err.Error(), "priority 0") {
		t.Fatalf("应说明 priority 0 抢占 local 表, got: %v", err)
	}
	r.Priority = 5
	if err := ValidateRule(r); err != nil {
		t.Fatalf("priority>0 且条件非空应通过: %v", err)
	}
}

// TestValidateRuleRejectsEmptyConditions —— 全条件为空 = 劫持全部流量。
func TestValidateRuleRejectsEmptyConditions(t *testing.T) {
	r := Rule{Priority: 10, Table: 100} // from/to/fwmark/iif 全空
	err := ValidateRule(r)
	if err == nil {
		t.Fatal("全条件为空必须被拒绝")
	}
	if !strings.Contains(err.Error(), "劫持") {
		t.Fatalf("应说明全空规则劫持默认路由, got: %v", err)
	}
	// 任一条件非空即通过
	r.FWMark = 3
	if err := ValidateRule(r); err != nil {
		t.Fatalf("有 fwmark 应通过: %v", err)
	}
}

func TestValidateRuleBadFromTo(t *testing.T) {
	// 非法 from
	r := Rule{Priority: 10, From: "not-an-ip", Table: 100}
	if err := ValidateRule(r); err == nil {
		t.Fatalf("非法 from 应被拒绝: %v", r)
	}
	// 非法 to
	r = Rule{Priority: 10, To: "999.1.2.3", Table: 100}
	if err := ValidateRule(r); err == nil {
		t.Fatal("非法 to 应被拒绝")
	}
	// 合法 CIDR to + fwmark 组合通过
	r = Rule{Priority: 10, To: "10.0.0.0/8", FWMark: 5, Table: 100}
	if err := ValidateRule(r); err != nil {
		t.Fatalf("合法 to+fwmark 应通过: %v", err)
	}
}

// TestValidateRouteBadCIDRAndIP —— 拒绝非法 CIDR 与非法 IP。
func TestValidateRouteBadCIDRAndIP(t *testing.T) {
	// 非法 CIDR /33
	r := Route{Dst: "10.0.0.0/33", Via: "192.168.1.1"}
	if err := ValidateRoute(r); err == nil {
		t.Fatal("非法 CIDR /33 应被拒绝")
	}
	// 非法 via IP
	r = Route{Dst: "10.0.0.0/8", Via: "999.1.1.1"}
	if err := ValidateRoute(r); err == nil {
		t.Fatal("非法 via IP 应被拒绝")
	}
	// 直连路由（Via 空）合法
	r = Route{Dst: "10.0.0.0/8", Dev: "eth1"}
	if err := ValidateRoute(r); err != nil {
		t.Fatalf("直连路由应通过: %v", err)
	}
}

// TestValidateRouteAcceptsDefaultAndCIDR —— 接受 default 与合法 CIDR。
func TestValidateRouteAcceptsDefaultAndCIDR(t *testing.T) {
	if err := ValidateRoute(Route{Dst: "default", Via: "192.168.1.1", Dev: "eth1"}); err != nil {
		t.Fatalf("default 应被接受: %v", err)
	}
	if err := ValidateRoute(Route{Dst: "2001:db8::/32", Via: "2001:db8::1", Dev: "eth1"}); err != nil {
		t.Fatalf("合法 v6 CIDR 应被接受: %v", err)
	}
	if err := ValidateRoute(Route{Dst: "10.0.0.0/8", Via: "192.168.1.1", Dev: "eth1", Metric: 5}); err != nil {
		t.Fatalf("合法 CIDR 应被接受: %v", err)
	}
}

func TestValidateRouteRejectsLo(t *testing.T) {
	if err := ValidateRoute(Route{Dst: "default", Dev: "lo"}); err == nil {
		t.Fatal("dev=lo 应被拒绝")
	}
}

// TestRenderRuleOnlyNonEmptyFields —— from 为空时不含 "from"。
func TestRenderRuleOnlyNonEmptyFields(t *testing.T) {
	r := Rule{Priority: 100, To: "10.0.0.0/8", FWMark: 7, Table: 100, Iif: "eth2"}
	cmd := RenderRule(r)
	// to/fwmark/iif/table 都在
	for _, want := range []string{"ip rule add", "priority 100", "to 10.0.0.0/8", "fwmark 7", "iif eth2", "table 100"} {
		if !strings.Contains(cmd, want) {
			t.Fatalf("应含 %q, got: %s", want, cmd)
		}
	}
	if strings.Contains(cmd, "from") {
		t.Fatalf("from 为空不应含 'from', got: %s", cmd)
	}
	// 顺序：priority 在前
	if strings.Index(cmd, "priority") > strings.Index(cmd, "to ") {
		t.Fatalf("priority 应位于 from/to 之前, got: %s", cmd)
	}
}

// TestRenderRuleFromPresent —— from 非空时输出。
func TestRenderRuleFromPresent(t *testing.T) {
	r := Rule{Priority: 10, From: "10.0.0.0/8", Table: 50}
	cmd := RenderRule(r)
	if !strings.Contains(cmd, "from 10.0.0.0/8") {
		t.Fatalf("应含 from, got: %s", cmd)
	}
}

// TestRenderTableRoutesDirectNoVia —— 直连路由（Via 空）不含 "via"。
func TestRenderTableRoutesDirectNoVia(t *testing.T) {
	tb := Table{
		ID:   100,
		Name: "wan1",
		Routes: []Route{
			{Dst: "10.0.0.0/8", Dev: "eth1", Metric: 10},      // 直连，无 via
			{Dst: "default", Via: "192.168.1.1", Dev: "eth1"}, // 有 via
		},
	}
	cmds := RenderTableRoutes(tb)
	if len(cmds) != 2 {
		t.Fatalf("len=%d (want 2), got %v", len(cmds), cmds)
	}
	direct := cmds[0]
	if strings.Contains(direct, "via") {
		t.Fatalf("直连路由不应含 via, got: %s", direct)
	}
	if !strings.Contains(direct, "10.0.0.0/8") || !strings.Contains(direct, "dev eth1") || !strings.Contains(direct, "table 100") {
		t.Fatalf("直连路由字段缺失, got: %s", direct)
	}
	if !strings.Contains(cmds[1], "via 192.168.1.1") {
		t.Fatalf("有网关路由应含 via, got: %s", cmds[1])
	}
	// metric 只对 >0 输出
	if strings.Contains(direct, "metric") == false && tb.Routes[0].Metric > 0 {
		t.Fatalf("metric>0 应输出 metric, got: %s", direct)
	}
}
