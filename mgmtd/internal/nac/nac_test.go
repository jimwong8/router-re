package nac

import (
	"fmt"
	"strings"
	"testing"
)

type stubRunner struct {
	out string
	err error
}

func (s *stubRunner) Run(name string, args ...string) (string, error) { return s.out, s.err }
func (s *stubRunner) RunWithInput(input, name string, args ...string) (string, error) {
	return s.out, s.err
}

func TestValidateRejectsEUI64(t *testing.T) {
	// 8-byte MAC must be rejected
	c := Config{Mode: "whitelist", Devices: []Device{{MAC: "00:11:22:33:44:55:66:77", Policy: "allow"}}}
	if err := Validate(c); err == nil {
		t.Fatal("8-byte MAC must be rejected")
	}
}

func TestValidateDuplicateMAC(t *testing.T) {
	c := Config{Mode: "blacklist", Devices: []Device{
		{MAC: "aa:bb:cc:dd:ee:ff", Name: "a", Policy: "deny"},
		{MAC: "AA:BB:CC:DD:EE:FF", Name: "b", Policy: "deny"},
	}}
	if err := Validate(c); err == nil {
		t.Fatal("duplicate MAC must be rejected")
	} else if !strings.Contains(err.Error(), "duplicate") || !strings.Contains(err.Error(), "a") || !strings.Contains(err.Error(), "b") {
		t.Errorf("error should name both conflicting devices: %v", err)
	}
}

func TestValidateWhitelistEmpty(t *testing.T) {
	c := Config{Mode: "whitelist", Devices: []Device{{MAC: "aa:bb:cc:dd:ee:ff", Policy: "deny"}}}
	if err := Validate(c); err == nil {
		t.Fatal("whitelist with no allow device must be rejected")
	} else if !strings.Contains(err.Error(), "block everyone") && !strings.Contains(err.Error(), "self-lockout") {
		t.Errorf("error should warn about self-lockout: %v", err)
	}
}

func TestValidateBadPolicy(t *testing.T) {
	c := Config{Mode: "blacklist", Devices: []Device{{MAC: "aa:bb:cc:dd:ee:ff", Policy: "banned"}}}
	if err := Validate(c); err == nil {
		t.Fatal("bad policy must be rejected")
	}
}

func TestValidateMaxConnsZero(t *testing.T) {
	if err := ValidateLimit(Limit{Name: "x", SrcIP: "1.2.3.4", MaxConns: 0}); err == nil {
		t.Fatal("max_conns=0 must be rejected")
	}
}

func TestValidateLimitOK(t *testing.T) {
	if err := ValidateLimit(Limit{Name: "x", SrcIP: "192.168.1.0/24", MaxConns: 100, MaxPerSec: 50}); err != nil {
		t.Fatalf("valid limit rejected: %v", err)
	}
}

func TestRenderNftUsesEtherSaddr(t *testing.T) {
	c := Config{Mode: "whitelist", Devices: []Device{{MAC: "aa:bb:cc:dd:ee:ff", Policy: "allow"}}}
	out := RenderNft(c)
	if !strings.Contains(out, "ether saddr") {
		t.Errorf("MAC filter must use ether saddr, got:\n%s", out)
	}
	if strings.Contains(out, "flush ruleset") {
		t.Fatal("must never flush ruleset")
	}
	if strings.Contains(out, "policy drop") {
		t.Fatal("must never set policy drop")
	}
}

func TestRenderNftWhitelistDropsNonAllow(t *testing.T) {
	c := Config{Mode: "whitelist", Devices: []Device{{MAC: "aa:bb:cc:dd:ee:ff", Policy: "allow"}}}
	out := RenderNft(c)
	if !strings.Contains(out, "ether saddr AA:BB:CC:DD:EE:FF accept") {
		t.Errorf("whitelist must accept listed MAC:\n%s", out)
	}
	if !strings.Contains(out, "drop") {
		t.Errorf("whitelist must drop non-listed:\n%s", out)
	}
}

func TestRenderNftBlacklistDropsListed(t *testing.T) {
	c := Config{Mode: "blacklist", Devices: []Device{{MAC: "aa:bb:cc:dd:ee:ff", Policy: "deny"}}}
	out := RenderNft(c)
	if strings.Contains(out, "ether saddr AA:BB:CC:DD:EE:FF accept") {
		t.Errorf("blacklist must not accept listed MAC:\n%s", out)
	}
	if !strings.Contains(out, "ether saddr AA:BB:CC:DD:EE:FF drop") {
		t.Errorf("blacklist must drop listed MAC:\n%s", out)
	}
}

func TestRenderConnlimitUsesCtCount(t *testing.T) {
	ls := []Limit{{Name: "x", SrcIP: "1.2.3.4", MaxConns: 10, MaxPerSec: 5, Enabled: true}}
	out := RenderConnlimit(ls)
	if !strings.Contains(out, "ct count over 10") {
		t.Errorf("connlimit must use ct count over, got:\n%s", out)
	}
	if strings.Contains(out, "flush ruleset") {
		t.Fatal("must never flush ruleset")
	}
	if strings.Contains(out, "policy drop") {
		t.Fatal("must never set policy drop")
	}
}

func TestNeighborsJSON(t *testing.T) {
	// ⚠ state 必须是**数组**：这是 `ip -j neigh show` 的真实格式。
	// 之前这里写成字符串 "REACHABLE"，测试绿了但真实调用 500 —— 测试捏造了
	// 输入格式，等于没测。
	jsonStr := `[{"dev":"ens18","dst":"10.0.0.1","lladdr":"aa:bb:cc:dd:ee:ff","state":["REACHABLE"]},{"dev":"ens18","dst":"10.0.0.2","lladdr":"11:22:33:44:55:66","state":["STALE"]}]`
	r := &stubRunner{out: jsonStr}
	devs, err := Neighbors(r)
	if err != nil {
		t.Fatalf("Neighbors error: %v", err)
	}
	if len(devs) != 2 {
		t.Fatalf("expected 2 neighbors, got %d", len(devs))
	}
	if devs[0].MAC != "aa:bb:cc:dd:ee:ff" || devs[0].IP != "10.0.0.1" {
		t.Errorf("unexpected first neighbor: %+v", devs[0])
	}
}

// TestNeighborsParsesStateArray 锁死一个真实踩到的解析错误：
// iproute2 的 `ip -j neigh show` 里 state 是数组（["REACHABLE"]）而不是字符串。
// 写成 string 会让整个邻居表 Unmarshal 失败 -> HTTP 500。
func TestNeighborsParsesStateArray(t *testing.T) {
	const realJSON = `[{"dst":"10.100.1.168","dev":"ens18","lladdr":"bc:24:11:76:6f:e2","state":["STALE"]},` +
		`{"dst":"10.250.17.2","dev":"vgh772002093","lladdr":"1a:12:d0:e8:d4:58","state":["REACHABLE","DELAY"]}]`
	r := &stubRunner{out: realJSON}
	got, err := Neighbors(r)
	if err != nil {
		t.Fatalf("Neighbors must accept array state, got error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 neighbors, got %d", len(got))
	}
	if got[0].Note != "STALE" {
		t.Fatalf("want STALE, got %q", got[0].Note)
	}
	if got[1].Note != "REACHABLE,DELAY" {
		t.Fatalf("multi-state should join with comma, got %q", got[1].Note)
	}
	if got[0].MAC != "bc:24:11:76:6f:e2" || got[0].IP != "10.100.1.168" {
		t.Fatalf("field mapping wrong: %+v", got[0])
	}
}

// mapRunner 按 "命令 参数" 返回不同输出，用来模拟 BusyBox 的 -j 失败。
type mapRunner struct {
	outs map[string]string
	errs map[string]error
}

func (m *mapRunner) Run(name string, args ...string) (string, error) {
	k := name
	for _, a := range args {
		k += " " + a
	}
	if e, ok := m.errs[k]; ok {
		return m.outs[k], e
	}
	return m.outs[k], nil
}
func (m *mapRunner) RunWithInput(input, name string, args ...string) (string, error) {
	return m.Run(name, args...)
}

// TestNeighborsFromTextBusyBox 锁死真机兼容性：
// OpenWrt 的 BusyBox ip（v1.36.1）不支持 `-j`，`ip -j neigh` 只打印 usage 并返回 1。
// 没有文本 fallback，真机上的邻居表永远是空的 —— 而"空列表"会被误读成
// "网络里没有设备"，比报错更具误导性。
func TestNeighborsFromTextBusyBox(t *testing.T) {
	r := &mapRunner{
		outs: map[string]string{
			"ip -j neigh show": "BusyBox v1.36.1 (2026-07-24 07:21:50 UTC) multi-call binary.\n\nUsage: ip [OPTIONS] address|route|link|neigh|rule",
			"ip neigh show":    "10.100.1.1 dev br-lan lladdr aa:bb:cc:dd:ee:ff REACHABLE\n10.100.1.50 dev br-lan FAILED\n",
		},
		errs: map[string]error{
			"ip -j neigh show": fmt.Errorf("exit status 1"),
		},
	}
	devs, err := Neighbors(r)
	if err != nil {
		t.Fatalf("BusyBox 环境应走文本 fallback 而不是报错: %v", err)
	}
	if len(devs) != 2 {
		t.Fatalf("应解析出 2 条邻居，got %d: %+v", len(devs), devs)
	}
	if devs[0].IP != "10.100.1.1" || devs[0].MAC != "aa:bb:cc:dd:ee:ff" || devs[0].Iface != "br-lan" {
		t.Fatalf("第一条解析错误: %+v", devs[0])
	}
	if devs[0].State != "REACHABLE" {
		t.Fatalf("状态解析错误: %q", devs[0].State)
	}
	// FAILED 行没有 lladdr：MAC 为空但设备必须出现（"有设备但不可达"是有用信息）
	if devs[1].IP != "10.100.1.50" || devs[1].MAC != "" || devs[1].State != "FAILED" {
		t.Fatalf("FAILED 行解析错误（MAC 应为空但设备要保留）: %+v", devs[1])
	}
}

// Name 拼进 nft 注释行，带换行就等于注入任意 nft 规则（如 flush ruleset）。
func TestValidateLimitRejectsNewlineInName(t *testing.T) {
	bad := []string{
		"x\nflush ruleset",
		"x\r\nip saddr 0.0.0.0/0 accept",
		"tab\there",
		strings.Repeat("a", 65),
	}
	for _, name := range bad {
		l := Limit{Name: name, SrcIP: "10.0.0.1", MaxConns: 10}
		if err := ValidateLimit(l); err == nil {
			t.Fatalf("非法 Name %q 通过了校验", name)
		}
	}
	ok := Limit{Name: "guest-limit", SrcIP: "10.0.0.1", MaxConns: 10}
	if err := ValidateLimit(ok); err != nil {
		t.Fatalf("合法 Name 被拒: %v", err)
	}
}
