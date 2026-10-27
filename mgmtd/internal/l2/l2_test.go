package l2

import (
	"fmt"
	"strings"
	"testing"
)

type fakeRunner struct {
	calls []string
}

func (f *fakeRunner) Run(name string, args ...string) (string, error) {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	// simulate ip link show: not found for new names (return error)
	if name == "ip" && len(args) >= 3 && args[0] == "link" && args[1] == "show" {
		return "", fmt.Errorf("not found")
	}
	// ListVLAN: return empty array
	if name == "ip" && len(args) >= 4 && args[0] == "-j" && args[1] == "-d" {
		return "[]", nil
	}
	if name == "ip" && len(args) >= 4 && args[0] == "-j" && args[1] == "link" {
		return "[]", nil
	}
	if name == "bridge" {
		return "", nil
	}
	return "", nil
}

func (f *fakeRunner) RunWithInput(input, name string, args ...string) (string, error) {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	return "", nil
}

func TestValidateVLAN(t *testing.T) {
	if err := ValidateVLAN(VLAN{Parent: "eth0", VID: 0, Name: "eth0.0"}); err == nil {
		t.Fatal("expected error for VID 0")
	}
	if err := ValidateVLAN(VLAN{Parent: "eth0", VID: 4095, Name: "eth0.4095"}); err == nil {
		t.Fatal("expected error for VID 4095")
	}
	if err := ValidateVLAN(VLAN{Parent: "eth0", VID: 100, Name: "has space"}); err == nil {
		t.Fatal("expected error for name with space")
	}
	if err := ValidateVLAN(VLAN{Parent: "eth0", VID: 100, Name: "eth0.100"}); err != nil {
		t.Fatalf("valid vlan rejected: %v", err)
	}
}

func TestApplyVLANCmds(t *testing.T) {
	fr := &fakeRunner{}
	v := VLAN{Parent: "eth0", VID: 100, Name: "eth0.100", Address: "10.0.0.1/24"}
	if err := ApplyVLAN(fr, v); err != nil {
		t.Fatalf("ApplyVLAN failed: %v", err)
	}
	joined := strings.Join(fr.calls, "\n")
	if !strings.Contains(joined, "link add link eth0 name eth0.100 type vlan id 100") {
		t.Errorf("missing vlan create cmd; got:\n%s", joined)
	}
	if !strings.Contains(joined, "ip addr replace 10.0.0.1/24 dev eth0.100") {
		t.Errorf("missing addr replace; got:\n%s", joined)
	}
	if !strings.Contains(joined, "ip link set eth0.100 up") {
		t.Errorf("missing link up; got:\n%s", joined)
	}
}

func TestValidateBridge(t *testing.T) {
	members := make([]string, 17)
	for i := range members {
		members[i] = "eth0"
	}
	if err := ValidateBridge(Bridge{Name: "br0", Members: members}); err == nil {
		t.Fatal("expected error for 17 members")
	}
	if err := ValidateBridge(Bridge{Name: "has space", Members: []string{"eth0"}}); err == nil {
		t.Fatal("expected error for name with space")
	}
	if err := ValidateBridge(Bridge{Name: "br0", Members: []string{"eth0"}}); err != nil {
		t.Fatalf("valid bridge rejected: %v", err)
	}
}

func TestApplyBridgeCmds(t *testing.T) {
	fr := &fakeRunner{}
	b := Bridge{Name: "br0", Members: []string{"eth0", "eth1"}, STP: true, Address: "192.168.1.1/24"}
	if err := ApplyBridge(fr, b); err != nil {
		t.Fatalf("ApplyBridge failed: %v", err)
	}
	joined := strings.Join(fr.calls, "\n")
	if !strings.Contains(joined, "master br0") {
		t.Errorf("missing 'master br0'; got:\n%s", joined)
	}
	if !strings.Contains(joined, "ip link add br0 type bridge") {
		t.Errorf("missing bridge create; got:\n%s", joined)
	}
	if !strings.Contains(joined, "stp_state 1") {
		t.Errorf("missing stp; got:\n%s", joined)
	}
}

// stubRunner 按命令返回预置 JSON，用来验证 List* 的解析路径。
type stubRunner struct{ stdout map[string]string }

func (s *stubRunner) RunWithInput(input, name string, args ...string) (string, error) {
	return s.Run(name, args...)
}

func (s *stubRunner) Run(name string, args ...string) (string, error) {
	key := name + " " + strings.Join(args, " ")
	for pat, out := range s.stdout {
		if strings.Contains(key, pat) {
			return out, nil
		}
	}
	return "", nil
}

// TestListVLANParsesRealIPJSON 锁死两个真实踩到的解析 bug：
//  1. VID 在 linkinfo.info_data.id，不是顶层 vlan_info（写错静默得到 0）；
//  2. IPv4 只在 `ip -j addr show` 的 addr_info 里，link show 只有 MAC。
//
// JSON 取自本机 `ip -j -d link show type vlan` 真实输出（截断）。
func TestListVLANParsesRealIPJSON(t *testing.T) {
	r := &stubRunner{stdout: map[string]string{
		"-j -d link show type vlan": `[{"ifindex":6059,"link":"vtest0","ifname":"vtest0.100","address":"d2:9a:f6:aa:88:71","linkinfo":{"info_kind":"vlan","info_data":{"protocol":"802.1Q","id":100}}}]`,
		"-j addr show":              `[{"ifname":"vtest0.100","addr_info":[{"family":"inet","local":"10.123.100.1","prefixlen":24}]}]`,
	}}
	got, err := ListVLAN(r)
	if err != nil {
		t.Fatalf("ListVLAN: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 vlan, got %d", len(got))
	}
	v := got[0]
	if v.VID != 100 {
		t.Errorf("VID = %d, want 100 (linkinfo.info_data.id)", v.VID)
	}
	if v.Parent != "vtest0" {
		t.Errorf("Parent = %q, want vtest0", v.Parent)
	}
	if v.Address != "10.123.100.1/24" {
		t.Errorf("Address = %q, want 10.123.100.1/24 (from addr show)", v.Address)
	}
}

// TestListBridgeParsesRealIPJSON 锁死：必须带 -d 才有 stp_state；
// 成员要从各 link 的 master 反查，不能解析 `bridge link show` 文本
// （那行开头是序号，会得到 members=["6059:"]）。
func TestListBridgeParsesRealIPJSON(t *testing.T) {
	r := &stubRunner{stdout: map[string]string{
		"-j -d link show type bridge": `[{"ifname":"btest0","linkinfo":{"info_kind":"bridge","info_data":{"stp_state":1,"forward_delay":1500}}}]`,
		"-j link show":                `[{"ifname":"btest0"},{"ifname":"vtest0.100","master":"btest0"}]`,
		"-j addr show":                `[{"ifname":"btest0","addr_info":[{"family":"inet","local":"10.123.200.1","prefixlen":24}]}]`,
	}}
	got, err := ListBridge(r)
	if err != nil {
		t.Fatalf("ListBridge: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 bridge, got %d", len(got))
	}
	b := got[0]
	if !b.STP {
		t.Error("STP = false, want true (linkinfo.info_data.stp_state=1)")
	}
	if len(b.Members) != 1 || b.Members[0] != "vtest0.100" {
		t.Errorf("Members = %v, want [vtest0.100]", b.Members)
	}
	if b.Address != "10.123.200.1/24" {
		t.Errorf("Address = %q, want 10.123.200.1/24", b.Address)
	}
}
