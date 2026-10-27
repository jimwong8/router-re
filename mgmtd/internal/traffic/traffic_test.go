package traffic

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// sampleNetDev 模拟 /proc/net/dev：两行表头 + 带空格的接口名 + lo + 字段不全的行。
const sampleNetDev = `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo: 1000       10    0    0    0     0          0         0     1000       10    0    0    0     0       0          0
  ens18: 5000000    4000    1    0    0     0          0         0  3000000    3000    2    0    0     0       0          0
eth0.10:     123       4    0    0    0     0          0         0      456       5    0    0    0     0       0          0
   broken: 1 2 3
`

func TestParseNetDevSkipsHeadersAndLoopback(t *testing.T) {
	got, err := parseNetDev(strings.NewReader(sampleNetDev))
	if err != nil {
		t.Fatalf("parseNetDev: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 interfaces (lo broken skipped), got %d: %+v", len(got), got)
	}
	if got[0].Iface != "ens18" {
		t.Fatalf("first iface = %q, want ens18", got[0].Iface)
	}
	want := Sample{
		Iface:     "ens18",
		RXBytes:   5000000,
		TXBytes:   3000000,
		RXPackets: 4000,
		TXPackets: 3000,
		RXErrors:  1,
		TXErrors:  2,
	}
	if got[0] != want {
		t.Fatalf("ens18 = %+v, want %+v", got[0], want)
	}
	if got[1].Iface != "eth0.10" || got[1].RXBytes != 123 || got[1].TXBytes != 456 {
		t.Fatalf("vlan iface parsed wrong: %+v", got[1])
	}
}

func TestReadCountersFromTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dev")
	if err := os.WriteFile(path, []byte(sampleNetDev), 0o644); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	got, err := readCountersFrom(path, execx.DefaultRunner{})
	if err != nil {
		t.Fatalf("readCountersFrom: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("no counters parsed")
	}
	for _, s := range got {
		if s.Iface == "lo" {
			t.Fatal("loopback must be excluded")
		}
	}
}

// TestReadCountersRealProc 直读真实 /proc/net/dev：非空，且本地测试机存在 ens18。
func TestReadCountersRealProc(t *testing.T) {
	if _, err := os.Stat(procNetDev); err != nil {
		t.Skipf("/proc/net/dev 不可用: %v", err)
	}
	got, err := ReadCounters(execx.DefaultRunner{})
	if err != nil {
		t.Fatalf("ReadCounters: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("真实 /proc/net/dev 解析结果为空")
	}
	found := false
	for _, s := range got {
		if s.Iface == "ens18" {
			found = true
		}
		if s.Iface == "lo" {
			t.Fatal("lo 不应出现在结果里")
		}
	}
	if !found {
		t.Logf("当前机器无 ens18（容器/其它网卡命名），已解析接口: %v", ifaceNames(got))
		// 不硬失败：CI 容器里的网卡名可能不是 ens18，但解析必须非空（上面已断言）。
	}
}

func TestComputeRatesNormal(t *testing.T) {
	prev := []Sample{
		{Iface: "ens18", RXBytes: 1000, TXBytes: 2000},
		{Iface: "eth1", RXBytes: 0, TXBytes: 0},
	}
	cur := []Sample{
		{Iface: "ens18", RXBytes: 1000 + 5000, TXBytes: 2000 + 1000},
		{Iface: "eth1", RXBytes: 100, TXBytes: 0},
	}
	got := ComputeRates(prev, cur, 2)
	if len(got) != 2 {
		t.Fatalf("want 2 rates, got %d", len(got))
	}
	if got[0].Iface != "ens18" || got[0].RXBps != 2500 || got[0].TXBps != 500 {
		t.Fatalf("ens18 rate = %+v, want rx=2500 tx=500", got[0])
	}
	if got[1].RXBps != 50 || got[1].TXBps != 0 {
		t.Fatalf("eth1 rate = %+v, want rx=50 tx=0", got[1])
	}
}

// TestComputeRatesCounterWrap 计数器回绕/接口重启时必须返回 0，不能出现负数。
func TestComputeRatesCounterWrap(t *testing.T) {
	prev := []Sample{{Iface: "ens18", RXBytes: 9000, TXBytes: 12000}}
	cur := []Sample{{Iface: "ens18", RXBytes: 10, TXBytes: 500}}
	got := ComputeRates(prev, cur, 5)
	if len(got) != 1 {
		t.Fatalf("want 1 rate, got %d", len(got))
	}
	if got[0].RXBps < 0 || got[0].TXBps < 0 {
		t.Fatalf("回绕时速率必须为 0，实际为负: %+v", got[0])
	}
	if got[0].RXBps != 0 || got[0].TXBps != 0 {
		t.Fatalf("回绕时速率必须为 0，got %+v", got[0])
	}
}

// TestComputeRatesNonPositiveElapsed elapsedSec=0（或负）时必须全部为 0，不能除零出 Inf/NaN。
func TestComputeRatesNonPositiveElapsed(t *testing.T) {
	prev := []Sample{{Iface: "ens18", RXBytes: 100}}
	cur := []Sample{{Iface: "ens18", RXBytes: 900}}
	for _, elapsed := range []float64{0, -1, -0.5} {
		got := ComputeRates(prev, cur, elapsed)
		if len(got) != 1 {
			t.Fatalf("elapsed=%v: want 1 rate, got %d", elapsed, len(got))
		}
		if got[0].RXBps != 0 || got[0].TXBps != 0 {
			t.Fatalf("elapsed=%v: 速率必须为 0，got %+v", elapsed, got[0])
		}
	}
}

// TestComputeRatesMissingPrev 新接口没有上一次采样时速率按 0 处理。
func TestComputeRatesMissingPrev(t *testing.T) {
	cur := []Sample{{Iface: "new0", RXBytes: 777, TXBytes: 999}}
	got := ComputeRates(nil, cur, 1)
	if len(got) != 1 || got[0].RXBps != 0 || got[0].TXBps != 0 {
		t.Fatalf("无 prev 时应为 0 速率, got %+v", got)
	}
}

func TestTopNOrderAndTruncate(t *testing.T) {
	rates := []Rate{
		{Iface: "a", RXBps: 10, TXBps: 5},  // 15
		{Iface: "b", RXBps: 100, TXBps: 0}, // 100
		{Iface: "c", RXBps: 50, TXBps: 60}, // 110
		{Iface: "d", RXBps: 0, TXBps: 1},   // 1
	}
	got := TopN(rates, 2)
	if len(got) != 2 {
		t.Fatalf("want 2, got %d", len(got))
	}
	if got[0].Iface != "c" || got[1].Iface != "b" {
		t.Fatalf("TopN(2) = %+v, want c then b", got)
	}
	// 入参不被改动
	if rates[0].Iface != "a" {
		t.Fatalf("TopN 修改了入参切片: %+v", rates)
	}
	all := TopN(rates, 0)
	if len(all) != 4 || all[0].Iface != "c" || all[3].Iface != "d" {
		t.Fatalf("TopN(0) 应返回全部并降序, got %+v", all)
	}
	if n := TopN(rates, -3); len(n) != 4 {
		t.Fatalf("n<0 应返回全部, got %d", len(n))
	}
	if n := TopN(rates, 99); len(n) != 4 {
		t.Fatalf("n 超过长度应返回全部, got %d", len(n))
	}
	if n := TopN(nil, 5); len(n) != 0 {
		t.Fatalf("空输入应返回空, got %d", len(n))
	}
}

func ifaceNames(s []Sample) []string {
	out := make([]string, 0, len(s))
	for _, v := range s {
		out = append(out, v.Iface)
	}
	return out
}
