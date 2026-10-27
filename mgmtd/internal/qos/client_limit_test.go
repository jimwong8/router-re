package qos

import (
	"strings"
	"testing"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// --- ParseSelector ---

func TestParseSelectorValid(t *testing.T) {
	cases := []struct {
		in       string
		wantKind string
		wantVal  string
	}{
		{"ip:192.168.1.100", "ip", "192.168.1.100"},
		{"ip:10.0.0.1", "ip", "10.0.0.1"},
		{"mac:aa:bb:cc:dd:ee:ff", "mac", "aa:bb:cc:dd:ee:ff"},
		{"mac:AA:BB:CC:DD:EE:FF", "mac", "AA:BB:CC:DD:EE:FF"},
	}
	for _, c := range cases {
		kind, val, err := ParseSelector(c.in)
		if err != nil {
			t.Errorf("ParseSelector(%q) error: %v", c.in, err)
			continue
		}
		if kind != c.wantKind || val != c.wantVal {
			t.Errorf("ParseSelector(%q) = (%q,%q), want (%q,%q)", c.in, kind, val, c.wantKind, c.wantVal)
		}
	}
}

func TestParseSelectorInvalid(t *testing.T) {
	bad := []string{
		"",            // empty
		"192.168.1.1", // no prefix
		"ip:notanip",  // bad IP
		"mac:zz:zz",   // bad MAC
		"foo:1.2.3.4", // bad kind
		"ip:",         // empty value
	}
	for _, s := range bad {
		if _, _, err := ParseSelector(s); err == nil {
			t.Errorf("ParseSelector(%q) expected error, got nil", s)
		}
	}
}

// --- RenderClientMarkNft ---

func TestRenderClientMarkNft(t *testing.T) {
	limits := []ClientLimit{
		{Selector: "ip:192.168.1.100", DownloadKbps: 5000, UploadKbps: 1000},
		{Selector: "mac:aa:bb:cc:dd:ee:ff", DownloadKbps: 3000, UploadKbps: 0},
	}
	out, err := RenderClientMarkNft(limits)
	if err != nil {
		t.Fatalf("RenderClientMarkNft error: %v", err)
	}
	// 表结构
	if !strings.Contains(out, "table inet router_client_qos") {
		t.Fatalf("missing table declaration:\n%s", out)
	}
	if !strings.Contains(out, "chain mark_prerouting") {
		t.Fatalf("missing prerouting chain:\n%s", out)
	}
	if !strings.Contains(out, "chain mark_postrouting") {
		t.Fatalf("missing postrouting chain:\n%s", out)
	}
	// IP 选择的下载方向（prerouting saddr）
	if !strings.Contains(out, "ip saddr 192.168.1.100") {
		t.Fatalf("missing ip saddr rule:\n%s", out)
	}
	// MAC 选择的下载方向（prerouting ether saddr）
	if !strings.Contains(out, "ether saddr aa:bb:cc:dd:ee:ff") {
		t.Fatalf("missing ether saddr rule:\n%s", out)
	}
	// 上传方向（postrouting daddr）—只有第一条有 UploadKbps>0
	if !strings.Contains(out, "ip daddr 192.168.1.100") {
		t.Fatalf("missing postrouting ip daddr rule:\n%s", out)
	}
	// mark 值 = ClientMarkBase + 0 = 0x100
	if !strings.Contains(out, "meta mark set 0x100") {
		t.Fatalf("missing mark 0x100:\n%s", out)
	}
	// 第二条 mark = 0x101
	if !strings.Contains(out, "meta mark set 0x101") {
		t.Fatalf("missing mark 0x101:\n%s", out)
	}
}

func TestRenderClientMarkNftBadSelector(t *testing.T) {
	limits := []ClientLimit{{Selector: "bad:nope", DownloadKbps: 1000}}
	if _, err := RenderClientMarkNft(limits); err == nil {
		t.Fatal("expected error for bad selector")
	}
}

// --- RenderClientTcCommands ---

func TestRenderClientTcCommands(t *testing.T) {
	limits := []ClientLimit{
		{Selector: "ip:192.168.1.100", DownloadKbps: 5000, UploadKbps: 1000, Comment: "laptop"},
		{Selector: "mac:aa:bb:cc:dd:ee:ff", DownloadKbps: 3000, UploadKbps: 0},
	}
	cmds, err := RenderClientTcCommands(limits, "eth0")
	if err != nil {
		t.Fatalf("RenderClientTcCommands error: %v", err)
	}
	if len(cmds) == 0 {
		t.Fatal("empty commands")
	}
	joined := strings.Join(cmds, "\n")
	// 根 qdisc
	if !strings.Contains(joined, "tc qdisc add dev eth0 root handle 1: htb default 9999") {
		t.Errorf("missing htb root qdisc add; got:\n%s", joined)
	}
	// del+add is required: `replace` fails on an existing htb root qdisc and
	// `add` fails when it already exists, so only del+add is idempotent.
	if !strings.Contains(joined, "tc qdisc del dev eth0 root") {
		t.Errorf("missing qdisc del before add; got:\n%s", joined)
	}
	// htb class（第一条 download=5000）
	if !strings.Contains(joined, "tc class replace dev eth0 parent 1: classid 1:1 htb rate 5000kbit") {
		t.Fatalf("missing class 1:1:\n%s", joined)
	}
	// fw filter（第一条 mark=0x100 → classid 1:1）
	if !strings.Contains(joined, "tc filter replace dev eth0 parent 1: protocol ip handle 0x100 fw classid 1:1") {
		t.Fatalf("missing fw filter 0x100:\n%s", joined)
	}
	// 第二条 classid 1:2
	if !strings.Contains(joined, "classid 1:2 htb rate 3000kbit") {
		t.Fatalf("missing class 1:2:\n%s", joined)
	}
}

// --- ApplyClientLimits ---

// recordingRunner 记录所有调用供断言命令序列。
type recordingRunner struct {
	calls []recCall
}

type recCall struct {
	input string
	name  string
	args  []string
}

func (r *recordingRunner) Run(name string, args ...string) (string, error) {
	r.calls = append(r.calls, recCall{name: name, args: append([]string(nil), args...)})
	return "", nil
}

func (r *recordingRunner) RunWithInput(input, name string, args ...string) (string, error) {
	r.calls = append(r.calls, recCall{input: input, name: name, args: append([]string(nil), args...)})
	return "", nil
}

func TestApplyClientLimitsOrder(t *testing.T) {
	r := &recordingRunner{}
	limits := []ClientLimit{
		{Selector: "ip:192.168.1.100", DownloadKbps: 5000},
	}
	if err := ApplyClientLimits(r, limits, "eth0"); err != nil {
		t.Fatalf("ApplyClientLimits error: %v", err)
	}
	if len(r.calls) < 3 {
		t.Fatalf("expected >= 3 calls, got %d", len(r.calls))
	}
	// 第一条必须是 nft delete table
	first := r.calls[0]
	if first.name != "nft" || first.args[0] != "delete" {
		t.Fatalf("first call should be 'nft delete table', got name=%q args=%v", first.name, first.args)
	}
	// 第二条是 nft -f -（带 stdin 输入）
	second := r.calls[1]
	if second.name != "nft" || second.args[0] != "-f" || second.args[1] != "-" {
		t.Fatalf("second call should be 'nft -f -', got name=%q args=%v", second.name, second.args)
	}
	if second.input == "" {
		t.Fatal("nft -f - should have non-empty stdin input")
	}
	if !strings.Contains(second.input, "table inet router_client_qos") {
		t.Fatalf("nft stdin should contain table declaration:\n%s", second.input)
	}
	// 后续是 tc 命令（至少一条）
	hasTc := false
	for _, c := range r.calls[2:] {
		if c.name == "tc" {
			hasTc = true
			break
		}
	}
	if !hasTc {
		t.Fatal("expected at least one tc command after nft calls")
	}
}

func TestApplyClientLimitsEmpty(t *testing.T) {
	r := &recordingRunner{}
	if err := ApplyClientLimits(r, nil, "eth0"); err == nil {
		t.Fatal("expected error for empty limits")
	}
}

// 确保实现了 execx.Runner 接口（编译期检查）
var _ execx.Runner = (*recordingRunner)(nil)
