package portmap

import (
	"strings"
	"testing"
)

func TestValidateValid(t *testing.T) {
	rs := []Rule{
		{ID: 1, Name: "web", Proto: "tcp", ExtPort: 8080, IntIP: "192.168.1.100", IntPort: 80, Enabled: true},
		{ID: 2, Name: "udp-test", Proto: "udp", ExtPort: 5353, IntIP: "10.0.0.5", IntPort: 53, Enabled: true},
	}
	if err := Validate(rs); err != nil {
		t.Fatalf("Validate valid rules error: %v", err)
	}
}

func TestValidateDuplicatePort(t *testing.T) {
	rs := []Rule{
		{ID: 1, Name: "a", Proto: "tcp", ExtPort: 8080, IntIP: "192.168.1.100", IntPort: 80},
		{ID: 2, Name: "b", Proto: "tcp", ExtPort: 8080, IntIP: "192.168.1.101", IntPort: 81},
	}
	if err := Validate(rs); err == nil {
		t.Fatal("expected error for duplicate proto+ext_port")
	}
	// 不同 proto 同端口应允许
	rs2 := []Rule{
		{ID: 1, Name: "a", Proto: "tcp", ExtPort: 8080, IntIP: "192.168.1.100", IntPort: 80},
		{ID: 2, Name: "b", Proto: "udp", ExtPort: 8080, IntIP: "192.168.1.101", IntPort: 81},
	}
	if err := Validate(rs2); err != nil {
		t.Fatalf("tcp+udp same port should be valid: %v", err)
	}
}

func TestValidatePublicIP(t *testing.T) {
	rs := []Rule{
		{ID: 1, Name: "a", Proto: "tcp", ExtPort: 8080, IntIP: "8.8.8.8", IntPort: 80},
	}
	if err := Validate(rs); err == nil {
		t.Fatal("expected error for public int_ip")
	}
}

func TestValidateBadPort(t *testing.T) {
	bad := []Rule{
		{ID: 1, Name: "a", Proto: "tcp", ExtPort: 0, IntIP: "192.168.1.1", IntPort: 80},
		{ID: 2, Name: "b", Proto: "tcp", ExtPort: 70000, IntIP: "192.168.1.1", IntPort: 80},
		{ID: 3, Name: "c", Proto: "tcp", ExtPort: 80, IntIP: "192.168.1.1", IntPort: 0},
		{ID: 4, Name: "d", Proto: "foo", ExtPort: 80, IntIP: "192.168.1.1", IntPort: 80},
	}
	for i, r := range bad {
		if err := Validate([]Rule{r}); err == nil {
			t.Errorf("bad rule %d expected error", i)
		}
	}
}

func TestRenderNftContains(t *testing.T) {
	rs := []Rule{
		{ID: 1, Name: "web", Proto: "tcp", ExtPort: 8080, IntIP: "192.168.1.100", IntPort: 80, Enabled: true},
	}
	out := RenderNft(rs)
	for _, want := range []string{
		"table inet router_portmap",
		"chain prerouting {",
		"type nat hook prerouting priority dstnat;",
		"tcp dport 8080 dnat ip to 192.168.1.100:80",
		"chain forward {",
		"type filter hook forward priority filter;",
		"ip daddr 192.168.1.100 tcp dport 80 accept",
		"chain postrouting {",
		"type nat hook postrouting priority srcnat;",
		"ip saddr 192.168.0.0/16 ip daddr 192.168.1.100 tcp dport 80 masquerade",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("RenderNft missing %q:\n%s", want, out)
		}
	}
	// chain 声明必须多行：不允许 "chain prerouting { type"
	if strings.Contains(out, "chain prerouting { type") {
		t.Errorf("chain declaration must be multi-line:\n%s", out)
	}
	if strings.Contains(out, "chain forward { type") {
		t.Errorf("chain forward declaration must be multi-line:\n%s", out)
	}
	if strings.Contains(out, "chain postrouting { type") {
		t.Errorf("chain postrouting declaration must be multi-line:\n%s", out)
	}
}

func TestRenderCommands(t *testing.T) {
	ps := []Policy{
		{ID: 1, Name: "p1", SrcCIDR: "192.168.1.0/24", DstCIDR: "", Table: 100, Priority: 100, Enabled: true},
		{ID: 2, Name: "p2", SrcCIDR: "", DstCIDR: "10.0.0.0/8", Table: 200, Priority: 200, Enabled: true},
		{ID: 3, Name: "disabled", SrcCIDR: "0.0.0.0/0", Table: 300, Priority: 300, Enabled: false},
	}
	cmds := RenderCommands(ps, map[int]string{100: "wan1"})
	joined := strings.Join(cmds, "\n")
	if !strings.Contains(joined, "ip rule add from 192.168.1.0/24 lookup 100 priority 100") {
		t.Errorf("missing from rule:\n%s", joined)
	}
	if !strings.Contains(joined, "ip rule add to 10.0.0.0/8 lookup 200 priority 200") {
		t.Errorf("missing to rule:\n%s", joined)
	}
	if !strings.Contains(joined, "ip route replace default dev wan1 table 100") {
		t.Errorf("missing route for table 100:\n%s", joined)
	}
	if strings.Contains(joined, "lookup 300") {
		t.Errorf("disabled policy should not render:\n%s", joined)
	}
}

func TestValidatePolicyDuplicatePriority(t *testing.T) {
	ps := []Policy{
		{ID: 1, Name: "a", Table: 100, Priority: 100},
		{ID: 2, Name: "b", Table: 200, Priority: 100},
	}
	if err := ValidatePolicy(ps); err == nil {
		t.Fatal("expected error for duplicate priority")
	}
}

func TestValidatePolicyBadCIDR(t *testing.T) {
	ps := []Policy{
		{ID: 1, Name: "a", SrcCIDR: "not-a-cidr", Table: 100, Priority: 100},
	}
	if err := ValidatePolicy(ps); err == nil {
		t.Fatal("expected error for bad src_cidr")
	}
}

// TestRenderNftUsesDnatIP 锁死 inet 表里 dnat 必须带 ip 族别：
// 裸 `dnat to` 会被 nft 拒绝（specify `dnat ip' or `dnat ip6'）。
func TestRenderNftUsesDnatIP(t *testing.T) {
	text := RenderNft([]Rule{{ID: 1, Proto: "tcp", ExtPort: 8080, IntIP: "10.0.0.5", IntPort: 80, Enabled: true}})
	if !strings.Contains(text, "dnat ip to 10.0.0.5:80") {
		t.Fatalf("want 'dnat ip to', got:\n%s", text)
	}
	if strings.Contains(text, " dnat to ") {
		t.Fatalf("bare 'dnat to' rejected by nft in inet table; got:\n%s", text)
	}
}
