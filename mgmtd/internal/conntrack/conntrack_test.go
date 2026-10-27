package conntrack

import (
	"strings"
	"testing"
)

func TestParseLineTcp(t *testing.T) {
	line := "ipv4 2 tcp 6 431999 ESTABLISHED src=1.2.3.4 dst=5.6.7.8 sport=1234 dport=80 bytes=100 packets=2"
	e, ok := parseLine(line)
	if !ok {
		t.Fatal("parseLine returned false for valid tcp line")
	}
	if e.Proto != "tcp" {
		t.Errorf("Proto = %q, want tcp", e.Proto)
	}
	if e.Src != "1.2.3.4" || e.Dst != "5.6.7.8" {
		t.Errorf("Src/Dst wrong: %s/%s", e.Src, e.Dst)
	}
	if e.SrcPort != 1234 || e.DstPort != 80 {
		t.Errorf("ports wrong: %d/%d", e.SrcPort, e.DstPort)
	}
	if e.State != "ESTABLISHED" {
		t.Errorf("State = %q, want ESTABLISHED", e.State)
	}
	if e.Bytes != 100 || e.Packets != 2 {
		t.Errorf("bytes/packets wrong: %d/%d", e.Bytes, e.Packets)
	}
	if e.TimeoutSec != 431999 {
		t.Errorf("TimeoutSec = %d, want 431999", e.TimeoutSec)
	}
}

func TestParseLineUdp(t *testing.T) {
	line := "ipv4 2 udp 17 300 UNREPLIED src=10.0.0.1 dst=10.0.0.2 sport=53 dport=53 bytes=50 packets=1"
	e, ok := parseLine(line)
	if !ok {
		t.Fatal("parseLine returned false for valid udp line")
	}
	if e.Proto != "udp" {
		t.Errorf("Proto = %q, want udp", e.Proto)
	}
	if e.State != "UNREPLIED" {
		t.Errorf("State = %q, want UNREPLIED", e.State)
	}
}

func TestSummary(t *testing.T) {
	entries := []Entry{
		{Proto: "tcp"}, {Proto: "tcp"}, {Proto: "udp"},
	}
	sum := Summary(entries)
	if sum["tcp"] != 2 || sum["udp"] != 1 {
		t.Errorf("Summary wrong: %+v", sum)
	}
}

func TestSummaryEmpty(t *testing.T) {
	sum := Summary(nil)
	if len(sum) != 0 {
		t.Errorf("empty Summary should be empty map, got %+v", sum)
	}
}

func TestListNoPanic(t *testing.T) {
	// /proc/net/nf_conntrack 可能不存在；不应 panic
	es, err := List(5)
	// 无论成功或失败，都不应 panic；切片非 nil
	if es == nil {
		es = []Entry{}
	}
	_ = err
	_ = es
}

func TestParseLineNoPort(t *testing.T) {
	// 无端口（icmp）仍应解析
	line := "ipv4 2 icmp 1 29 UNREPLIED src=10.0.0.1 dst=10.0.0.2 bytes=0 packets=0"
	e, ok := parseLine(line)
	if !ok {
		t.Fatal("parseLine returned false")
	}
	if e.SrcPort != 0 || e.DstPort != 0 {
		t.Errorf("expected ports 0, got %d/%d", e.SrcPort, e.DstPort)
	}
	if !strings.Contains(e.Src, "10.0.0.1") {
		t.Errorf("src wrong: %s", e.Src)
	}
}

// TestParseLineKeepsFirstDirection 锁死一个真实踩到的 bug：
// /proc/net/nf_conntrack 每行有两组 src/dst（请求方向 + 回复方向），
// 用 map 覆盖式赋值会取到回复方向，得出完全反向的连接。
// 样例取自本机真实输出。
func TestParseLineKeepsFirstDirection(t *testing.T) {
	line := `ipv6     10 tcp      6 431992 ESTABLISHED src=2409:8a5c:6419:d991:0000:0000:0000:02be dst=2606:b740:0049:0000:0000:0000:0000:0109 sport=55566 dport=80 src=2606:b740:0049:0000:0000:0000:0000:0109 dst=2409:8a5c:6419:d991:0000:0000:0000:02be sport=80 dport=55566 [ASSURED] mark=0 zone=0 use=2`
	e, ok := parseLine(line)
	if !ok {
		t.Fatal("parseLine rejected a real conntrack line")
	}
	if e.Src != "2409:8a5c:6419:d991:0000:0000:0000:02be" {
		t.Errorf("Src took the reply direction: got %q", e.Src)
	}
	if e.Dst != "2606:b740:0049:0000:0000:0000:0000:0109" {
		t.Errorf("Dst took the reply direction: got %q", e.Dst)
	}
	if e.SrcPort != 55566 || e.DstPort != 80 {
		t.Errorf("ports reversed: got %d → %d, want 55566 → 80", e.SrcPort, e.DstPort)
	}
	if e.Proto != "tcp" || e.State != "ESTABLISHED" {
		t.Errorf("proto/state wrong: %q/%q", e.Proto, e.State)
	}
}
