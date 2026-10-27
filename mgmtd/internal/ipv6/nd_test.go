package ipv6

import (
	"fmt"
	"strings"
	"testing"
)

// recordingRunner records commands and simulates /proc output.
type recordingRunner struct {
	seen   []string          // recorded commands (name + args joined)
	catMap map[string]string // path -> /proc simulation
	ipOut  string            // simulated `ip link show` output
}

func (r *recordingRunner) Run(name string, args ...string) (string, error) {
	r.seen = append(r.seen, name+" "+strings.Join(args, " "))
	if name == "cat" && len(args) == 1 {
		if v, ok := r.catMap[args[0]]; ok {
			return v, nil
		}
		return "", fmt.Errorf("cat: no simulation for %s", args[0])
	}
	if name == "ip" && containsStr(args, "link") && containsStr(args, "show") {
		return r.ipOut, nil
	}
	return "0", nil
}

func (r *recordingRunner) RunWithInput(input, name string, args ...string) (string, error) {
	return r.Run(name, args...)
}

func containsCmd(r *recordingRunner, sub string) bool {
	for _, c := range r.seen {
		if strings.Contains(c, sub) {
			return true
		}
	}
	return false
}

func containsStr(a []string, s string) bool {
	for _, v := range a {
		if v == s {
			return true
		}
	}
	return false
}

func TestValidateND(t *testing.T) {
	cases := []struct {
		name string
		c    NDConfig
	}{
		{"lo iface", NDConfig{Iface: "lo", RaIntervalMin: 3, RaIntervalMax: 10}},
		{"RaIntervalMax < min", NDConfig{Iface: "eth0", RaIntervalMin: 5, RaIntervalMax: 4}},
		{"RaIntervalMax > 1800", NDConfig{Iface: "eth0", RaIntervalMin: 3, RaIntervalMax: 2000}},
		{"Mtu too small", NDConfig{Iface: "eth0", RaIntervalMin: 3, RaIntervalMax: 10, Mtu: 100}},
		{"HopLimit out of range", NDConfig{Iface: "eth0", RaIntervalMin: 3, RaIntervalMax: 10, HopLimit: 300}},
		{"valid min", NDConfig{Iface: "eth0", RaIntervalMin: 3, RaIntervalMax: 10}},
	}
	for _, tc := range cases {
		err := ValidateND(tc.c)
		if tc.name == "valid min" {
			if err != nil {
				t.Errorf("expected no error for valid, got %v", err)
			}
			continue
		}
		if err == nil {
			t.Errorf("expected error for %s", tc.name)
		}
	}
}

// ApplyND: stub runner records writes and reads back — 回读值不一致时返回错误。
func TestApplyNDWritesAndReadsBack(t *testing.T) {
	rr := &recordingRunner{
		catMap: map[string]string{},
		ipOut:  "3: eth0: <...> mtu 1500 qdisc",
	}
	c := NDConfig{
		Iface:           "eth0",
		RaIntervalMin:   200,
		RaIntervalMax:   600,
		HopLimit:        64,
		ReachableTimeMs: 30000,
		DadTransmits:    1,
		AcceptRa:        false,
		Forwarding:      true,
	}
	// 模拟回读值匹配。
	// 注入 writer：真实实现会去写 /proc/sys（测试环境里 eth0 不存在，
	// 而且绝不该让单元测试去改内核参数）。
	writes := map[string]string{}
	oldWriter := sysctlWriter
	sysctlWriter = func(path, val string) error { writes[path] = val; return nil }
	defer func() { sysctlWriter = oldWriter }()

	rr.catMap["/proc/sys/net/ipv6/conf/eth0/hop_limit"] = "64"
	rr.catMap["/proc/sys/net/ipv6/conf/eth0/reachable_time"] = "30000"
	rr.catMap["/proc/sys/net/ipv6/conf/eth0/retrans_time"] = "0"
	rr.catMap["/proc/sys/net/ipv6/conf/eth0/dad_transmits"] = "1"
	rr.catMap["/proc/sys/net/ipv6/conf/eth0/accept_ra"] = "0"
	rr.catMap["/proc/sys/net/ipv6/conf/eth0/forwarding"] = "1"

	if err := ApplyND(rr, c); err != nil {
		t.Fatalf("expected ApplyND ok, got %v", err)
	}
	if writes["/proc/sys/net/ipv6/conf/eth0/hop_limit"] != "64" {
		t.Fatalf("hop_limit 应被写成 64，实际写入: %v", writes)
	}
	if writes["/proc/sys/net/ipv6/conf/eth0/forwarding"] != "1" {
		t.Fatalf("forwarding 应被写成 1，实际写入: %v", writes)
	}
	if !containsCmd(rr, "cat /proc/sys/net/ipv6/conf/eth0/hop_limit") {
		t.Fatal("expected readback via cat")
	}
}

// 回读值不一致时必须返回错误。
func TestApplyNDReadbackMismatch(t *testing.T) {
	rr := &recordingRunner{
		catMap: map[string]string{},
		ipOut:  "3: eth0: <...> mtu 1500",
	}
	// Hopfälle 回读返回 0， aber wir setzen 64 →  mismatch。
	rr.catMap["/proc/sys/net/ipv6/conf/eth0/hop_limit"] = "0"
	rr.catMap["/proc/sys/net/ipv6/conf/eth0/reachable_time"] = "0"
	rr.catMap["/proc/sys/net/ipv6/conf/eth0/retrans_time"] = "0"
	rr.catMap["/proc/sys/net/ipv6/conf/eth0/dad_transmits"] = "0"
	rr.catMap["/proc/sys/net/ipv6/conf/eth0/accept_ra"] = "1"
	rr.catMap["/proc/sys/net/ipv6/conf/eth0/forwarding"] = "1"
	c := NDConfig{
		Iface:         "eth0",
		RaIntervalMin: 200,
		RaIntervalMax: 600,
		HopLimit:      64,
		AcceptRa:      true,
		Forwarding:    true,
	}
	err := ApplyND(rr, c)
	if err == nil {
		t.Fatal("expected readback mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "hop_limit") && !strings.Contains(err.Error(), "reachable_time") {
		t.Fatalf("error should mention readback mismatch: %v", err)
	}
}

// MTU 通过 ip link set 写，回读 via ip link show。
func TestApplyNDMtu(t *testing.T) {
	rr := &recordingRunner{
		catMap: map[string]string{},
		ipOut:  "3: eth0: <BROADCAST,MULTICAST> mtu 1280",
	}
	c := NDConfig{
		Iface:         "eth0",
		RaIntervalMin: 200,
		RaIntervalMax: 600,
		HopLimit:      64,
		Mtu:           1280,
		Forwarding:    true,
	}
	oldWriter := sysctlWriter
	sysctlWriter = func(path, val string) error { return nil }
	defer func() { sysctlWriter = oldWriter }()
	rr.catMap["/proc/sys/net/ipv6/conf/eth0/hop_limit"] = "64"
	rr.catMap["/proc/sys/net/ipv6/conf/eth0/reachable_time"] = "0"
	rr.catMap["/proc/sys/net/ipv6/conf/eth0/retrans_time"] = "0"
	rr.catMap["/proc/sys/net/ipv6/conf/eth0/dad_transmits"] = "0"
	rr.catMap["/proc/sys/net/ipv6/conf/eth0/accept_ra"] = "0"
	rr.catMap["/proc/sys/net/ipv6/conf/eth0/forwarding"] = "1"

	if err := ApplyND(rr, c); err != nil {
		t.Fatalf("expected MTU apply ok, got %v", err)
	}
	if !containsCmd(rr, "ip link set dev eth0 mtu 1280") {
		t.Fatal("expected ip link set mtu")
	}
	if !containsCmd(rr, "ip link show dev eth0") {
		t.Fatal("expected ip link show for mtu readback")
	}
}

// ReadND reads back from /proc.
func TestReadND(t *testing.T) {
	rr := &recordingRunner{
		catMap: map[string]string{
			"/proc/sys/net/ipv6/conf/eth0/hop_limit":      "64",
			"/proc/sys/net/ipv6/conf/eth0/reachable_time": "0",
			"/proc/sys/net/ipv6/conf/eth0/retrans_time":   "1000",
			"/proc/sys/net/ipv6/conf/eth0/dad_transmits":  "1",
			"/proc/sys/net/ipv6/conf/eth0/accept_ra":      "0",
			"/proc/sys/net/ipv6/conf/eth0/forwarding":     "1",
		},
		ipOut: "3: eth0: <BROADCAST> mtu 1280 qdisc",
	}
	c, err := ReadND(rr, "eth0")
	if err != nil {
		t.Fatalf("ReadND: %v", err)
	}
	if c.HopLimit != 64 || c.RetransTimerMs != 1000 || c.DadTransmits != 1 {
		t.Fatalf("ReadND values wrong: %+v", c)
	}
	if c.AcceptRa != false || c.Forwarding != true {
		t.Fatalf("bool sysctls wrong: accept_ra=%v forwarding=%v", c.AcceptRa, c.Forwarding)
	}
	if c.Mtu != 1280 {
		t.Fatalf("mtu wrong: %d", c.Mtu)
	}
}

func TestReadNDLoopback(t *testing.T) {
	rr := &recordingRunner{catMap: map[string]string{}}
	if _, err := ReadND(rr, "lo"); err == nil {
		t.Fatal("expected error for lo iface")
	}
}
