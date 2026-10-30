package sysctl

import (
	"strings"
	"testing"
)

func TestDefaultKernelParamsValid(t *testing.T) {
	if err := ValidateKernelParams(DefaultKernelParams()); err != nil {
		t.Fatalf("默认值应通过校验: %v", err)
	}
}

func TestValidateKernelParamsBounds(t *testing.T) {
	cases := []struct {
		name  string
		mut   func(*KernelParams)
		field string
	}{
		{"bbr 越界", func(p *KernelParams) { p.BBR = 2 }, "bbr"},
		{"syn_send 太小", func(p *KernelParams) { p.SynSendTimeout = 1 }, "syn_send_timeout"},
		{"syn_recv 太大", func(p *KernelParams) { p.SynRecvTimeout = 61 }, "syn_recv_timeout"},
		{"established 太小", func(p *KernelParams) { p.EstablishedTimeout = 599 }, "established_timeout"},
		{"established 太大", func(p *KernelParams) { p.EstablishedTimeout = 86401 }, "established_timeout"},
		{"udp_stream 太小", func(p *KernelParams) { p.UDPStreamTimeout = 29 }, "udp_stream_timeout"},
		{"icmp 太大", func(p *KernelParams) { p.ICMPTimeout = 101 }, "icmp_timeout"},
	}
	for _, c := range cases {
		p := DefaultKernelParams()
		c.mut(&p)
		err := ValidateKernelParams(p)
		if err == nil {
			t.Fatalf("%s 应报错", c.name)
		}
		if !strings.Contains(err.Error(), c.field) {
			t.Fatalf("%s 的错误信息应包含字段名 %s，实际: %v", c.name, c.field, err)
		}
	}
}

func TestValidateKernelParamsEdges(t *testing.T) {
	// 边界值应当通过：区间是闭区间
	p := KernelParams{
		BBR: 0, SynSendTimeout: 5, SynRecvTimeout: 60, EstablishedTimeout: 600,
		FinWaitTimeout: 5, CloseWaitTimeout: 60, LastAckTimeout: 5,
		TimeWaitTimeout: 60, CloseTimeout: 5, UDPTimeout: 60,
		UDPStreamTimeout: 30, ICMPTimeout: 100,
	}
	if err := ValidateKernelParams(p); err != nil {
		t.Fatalf("边界值应通过: %v", err)
	}
	p.UDPStreamTimeout = 1800
	if err := ValidateKernelParams(p); err != nil {
		t.Fatalf("udp_stream_timeout=1800 应通过: %v", err)
	}
	p.EstablishedTimeout = 86400
	if err := ValidateKernelParams(p); err != nil {
		t.Fatalf("established_timeout=86400 应通过: %v", err)
	}
}

func TestRenderKernelConfBBROn(t *testing.T) {
	p := DefaultKernelParams()
	out := RenderKernelConf(p)
	for _, want := range []string{
		"net.core.default_qdisc=fq",
		"net.ipv4.tcp_congestion_control=bbr",
		"net.netfilter.nf_conntrack_tcp_timeout_syn_sent=30",
		"net.netfilter.nf_conntrack_tcp_timeout_established=43200",
		"net.netfilter.nf_conntrack_udp_timeout_stream=180",
		"net.netfilter.nf_conntrack_icmp_timeout=30",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("配置缺少 %q:\n%s", want, out)
		}
	}
	// 11 个 conntrack 键都要出现
	if n := strings.Count(out, "nf_conntrack_"); n != len(KernelSpecs) {
		t.Fatalf("conntrack 键数量 = %d，应为 %d", n, len(KernelSpecs))
	}
}

func TestRenderKernelConfBBROff(t *testing.T) {
	p := DefaultKernelParams()
	p.BBR = 0
	out := RenderKernelConf(p)
	if !strings.Contains(out, "tcp_congestion_control=cubic") {
		t.Fatalf("关闭 BBR 应写回 cubic:\n%s", out)
	}
	if strings.Contains(out, "=bbr") {
		t.Fatalf("关闭 BBR 不应残留 =bbr:\n%s", out)
	}
}

func TestRenderKernelConfStable(t *testing.T) {
	p := DefaultKernelParams()
	if RenderKernelConf(p) != RenderKernelConf(p) {
		t.Fatal("两次渲染应完全一致")
	}
}

func TestParseSysctlConfRoundTrip(t *testing.T) {
	p := DefaultKernelParams()
	p.BBR = 1
	p.EstablishedTimeout = 7200
	p.UDPStreamTimeout = 600
	p.ICMPTimeout = 45
	got := ParseSysctlConf(RenderKernelConf(p))
	if got != p {
		t.Fatalf("往返不一致:\nwant %+v\ngot  %+v", p, got)
	}
}

func TestParseSysctlConfIgnoresGarbage(t *testing.T) {
	got := ParseSysctlConf("# comment\n\nnot a kv line\nnet.ipv4.tcp_congestion_control=bbr\n")
	if got.BBR != 1 {
		t.Fatalf("应识别 bbr=1，实际 %d", got.BBR)
	}
	// 垃圾行不应把其它字段改成 0
	if got.EstablishedTimeout != DefaultKernelParams().EstablishedTimeout {
		t.Fatalf("未出现的键应保留默认值，实际 %d", got.EstablishedTimeout)
	}
}

func TestSetGet(t *testing.T) {
	p := KernelParams{}
	for _, s := range KernelSpecs {
		if !p.Set(s.Field, 42) {
			t.Fatalf("Set(%s) 应命中", s.Field)
		}
		if p.get(s.Field) != 42 {
			t.Fatalf("get(%s) 应为 42", s.Field)
		}
	}
	if p.Set("not_a_field", 1) {
		t.Fatal("未知字段应返回 false")
	}
	if p.get("not_a_field") != -1 {
		t.Fatal("未知字段 get 应返回 -1")
	}
}

func TestParseApplyOutputAllOK(t *testing.T) {
	out := "net.ipv4.tcp_congestion_control = bbr\nnet.netfilter.nf_conntrack_udp_timeout = 30\n"
	ok, failed := ParseApplyOutput(out)
	if len(ok) != 2 || len(failed) != 0 {
		t.Fatalf("ok=%v failed=%v", ok, failed)
	}
	if ok[0] != "net.ipv4.tcp_congestion_control" {
		t.Fatalf("键名解析错误: %q", ok[0])
	}
}

func TestParseApplyOutputPartialFail(t *testing.T) {
	// 真实场景：容器里 qdisc 不可写，但 conntrack 键生效了
	out := `net.core.default_qdisc = fq
sysctl: permission denied on key 'net.core.default_qdisc'
net.netfilter.nf_conntrack_udp_timeout_stream = 240`
	ok, failed := ParseApplyOutput(out)
	if len(ok) != 2 {
		t.Fatalf("应有 2 个成功键，实际 %v", ok)
	}
	if len(failed) != 1 || failed[0] != "net.core.default_qdisc" {
		t.Fatalf("失败键解析错误: %v", failed)
	}
}

func TestParseApplyOutputCannotStat(t *testing.T) {
	out := "sysctl: cannot stat /proc/sys/net/netfilter/nf_conntrack_icmp_timeout: No such file or directory\n"
	ok, failed := ParseApplyOutput(out)
	if len(ok) != 0 || len(failed) != 1 {
		t.Fatalf("ok=%v failed=%v", ok, failed)
	}
	if !strings.Contains(failed[0], "cannot") && !strings.Contains(failed[0], "nf_conntrack") {
		t.Fatalf("失败原因应可辨认: %q", failed[0])
	}
}

func TestParseApplyOutputEmpty(t *testing.T) {
	ok, failed := ParseApplyOutput("")
	if len(ok) != 0 || len(failed) != 0 {
		t.Fatalf("空输出不应产生任何键: %v %v", ok, failed)
	}
}
