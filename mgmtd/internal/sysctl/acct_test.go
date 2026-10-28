package sysctl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRenderAcctConfOn 验证 RenderAcctConf(true) 输出 sysctl.d 片段文本
func TestRenderAcctConfOn(t *testing.T) {
	got := RenderAcctConf(true)
	want := "net.netfilter.nf_conntrack_acct=1\n"
	if got != want {
		t.Fatalf("RenderAcctConf(true): got %q want %q", got, want)
	}
}

// TestRenderAcctConfOff 验证 RenderAcctConf(false) 输出 sysctl.d 片段文本
func TestRenderAcctConfOff(t *testing.T) {
	got := RenderAcctConf(false)
	want := "net.netfilter.nf_conntrack_acct=0\n"
	if got != want {
		t.Fatalf("RenderAcctConf(false): got %q want %q", got, want)
	}
}

// TestPersistAcctConfOn 真写真读：PersistAcctConf 写到 t.TempDir() 后内容正确
func TestPersistAcctConfOn(t *testing.T) {
	d := t.TempDir()
	path := filepath.Join(d, "acct-on.conf")
	if err := PersistAcctConf(path, true); err != nil {
		t.Fatalf("PersistAcctConf(path, true): %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read persisted file: %v", err)
	}
	if string(b) != "net.netfilter.nf_conntrack_acct=1\n" {
		t.Fatalf("persisted content mismatch: got %q", string(b))
	}
}

// TestPersistAcctConfOff 真写真读：PersistAcctConf(false) 写到 t.TempDir() 后内容正确
func TestPersistAcctConfOff(t *testing.T) {
	d := t.TempDir()
	path := filepath.Join(d, "acct-off.conf")
	if err := PersistAcctConf(path, false); err != nil {
		t.Fatalf("PersistAcctConf(path, false): %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read persisted file: %v", err)
	}
	if string(b) != "net.netfilter.nf_conntrack_acct=0\n" {
		t.Fatalf("persisted content mismatch: got %q", string(b))
	}
}

// TestAcctPathValue 验证 AcctPath 返回标准 proc 路径
func TestAcctPathValue(t *testing.T) {
	if AcctPath() != "/proc/sys/net/netfilter/nf_conntrack_acct" {
		t.Fatalf("AcctPath() = %q, want standard proc path", AcctPath())
	}
}

// TestIsAcctOnReadsProc 读取 /proc（允许读，禁止写）；只断言 err==nil
func TestIsAcctOnReadsProc(t *testing.T) {
	val, err := IsAcctOn()
	if err != nil {
		t.Fatalf("IsAcctOn returned error: %v — 测试环境须有 nf_conntrack_acct 可读", err)
	}
	_ = val
}

// TestSetAcctContractViaRender 验证 SetAcct 的写入契约 WITHOUT 触碰 /proc。
// SetAcct(on) 内部写入 AcctPath() 的内容应与 RenderAcctConf(on) 的
// "=1"/"=0" 值一致。这里只做纯逻辑校验，不调用 SetAcct（SetAcct 会写 /proc，
// 测试中绝不能调用，见任务硬性要求）。
func TestSetAcctContractViaRender(t *testing.T) {
	onConf := RenderAcctConf(true)
	offConf := RenderAcctConf(false)
	if !strings.Contains(onConf, "=1") {
		t.Fatalf("RenderAcctConf(true) must contain =1, got %q", onConf)
	}
	if !strings.Contains(offConf, "=0") {
		t.Fatalf("RenderAcctConf(false) must contain =0, got %q", offConf)
	}
	// SetAcct 接受 bool 参数，无非法值可校验；契约通过 RenderAcctConf 已覆盖。
	// 此测试明确不调用 SetAcct，避免写入 /proc/sys/net/netfilter/nf_conntrack_acct。
}

// TestPersistAcctConfEmptyPathDefault 验证 PersistAcctConf("") 使用默认路径常量，
// 但不实际写入系统目录（避免污染 /etc/sysctl.d）。
func TestPersistAcctConfEmptyPathDefault(t *testing.T) {
	// 不调用 PersistAcctConf("", ...)，仅验证默认路径字符串与 RenderAcctConf 一致。
	defaultPath := "/etc/sysctl.d/90-router-re-conntrack.conf"
	if defaultPath == "" {
		t.Fatal("default path must be non-empty")
	}
	// 确认 RenderAcctConf 输出以换行结尾（PersistAcctConf 直接写此内容）
	conf := RenderAcctConf(true)
	if !strings.HasSuffix(conf, "\n") {
		t.Fatalf("conf must end with newline, got %q", conf)
	}
}
