package easytier

import (
	"strings"
	"testing"
)

func TestRenderConfigContainsIdentityAndPeers(t *testing.T) {
	c := Default()
	c.NetworkName = "office-net"
	c.NetworkSecret = "s3cret-pass"
	c.IPv4 = "10.144.0.1/24"
	c.Peers = []string{"tcp://1.2.3.4:11010", "udp://vpn.example.com:11010"}
	c.ProxyNetworks = []string{"192.168.1.0/24", "10.0.0.0/24->192.168.50.0/24"}

	out := RenderConfig(c)
	for _, want := range []string{
		`network_name = "office-net"`,
		`network_secret = "s3cret-pass"`,
		`ipv4 = "10.144.0.1/24"`,
		`rpc_portal = "127.0.0.1:15888"`,
		"[[peer]]",
		`uri = "tcp://1.2.3.4:11010"`,
		`uri = "udp://vpn.example.com:11010"`,
		`"10.0.0.0/24->192.168.50.0/24"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("渲染结果缺少 %q\n%s", want, out)
		}
	}
}

func TestRenderConfigOmitsEmptyIPv4AndEscapes(t *testing.T) {
	c := Default()
	c.NetworkName = `net"quote`
	c.NetworkSecret = `back\slash`
	out := RenderConfig(c)
	if strings.Contains(out, "ipv4 =") {
		t.Fatalf("IPv4 为空时不应渲染 ipv4 行（否则 EasyTier 会去建 TUN）\n%s", out)
	}
	if !strings.Contains(out, `network_name = "net\"quote"`) {
		t.Fatalf("引号未转义\n%s", out)
	}
	if !strings.Contains(out, `network_secret = "back\\slash"`) {
		t.Fatalf("反斜杠未转义\n%s", out)
	}
}

func TestValidateRejectsInjectionAndBadValues(t *testing.T) {
	base := Default()
	base.Enabled = true
	base.NetworkName = "n"
	base.NetworkSecret = "12345678"

	cases := []struct {
		name string
		mut  func(*Config)
	}{
		{"换行注入网络名", func(c *Config) { c.NetworkName = "net\n[[peer]]\nuri=\"tcp://evil:1\"" }},
		{"换行注入密钥", func(c *Config) { c.NetworkSecret = "12345678\nipv4=\"1.2.3.4\"" }},
		{"密钥过短", func(c *Config) { c.NetworkSecret = "1234" }},
		{"IPv4 非法", func(c *Config) { c.IPv4 = "10.144.0.999/24" }},
		{"IPv6 混入 ipv4 字段", func(c *Config) { c.IPv4 = "fd00::1/64" }},
		{"监听协议非法", func(c *Config) { c.Listeners = []string{"http://0.0.0.0:80"} }},
		{"监听端口越界", func(c *Config) { c.Listeners = []string{"tcp://0.0.0.0:70000"} }},
		{"对等节点缺协议", func(c *Config) { c.Peers = []string{"1.2.3.4:11010"} }},
		{"代理网段非 CIDR", func(c *Config) { c.ProxyNetworks = []string{"192.168.1.1"} }},
		{"RPC 地址非法", func(c *Config) { c.RPCPortal = "0.0.0.0" }},
	}
	for _, tc := range cases {
		c := base
		tc.mut(&c)
		if err := Validate(c); err == nil {
			t.Fatalf("%s：应当被拒绝，实际通过", tc.name)
		}
	}
}

func TestValidateAllowsDisabledDraft(t *testing.T) {
	c := Default() // enabled=false，无网络名/密钥
	if err := Validate(c); err != nil {
		t.Fatalf("未启用时应允许保存草稿：%v", err)
	}
}

func TestValidateAcceptsTypicalConfig(t *testing.T) {
	c := Default()
	c.Enabled = true
	c.NetworkName = "home"
	c.NetworkSecret = "supersecret"
	c.IPv4 = "10.144.0.1/24"
	c.Peers = []string{"tcp://public.easytier.cn:11010"}
	c.Listeners = []string{"tcp://0.0.0.0:11010", "udp://0.0.0.0:11010", "wg:11011", "11012"}
	if err := Validate(c); err != nil {
		t.Fatalf("合法配置被拒：%v", err)
	}
}

// 真实 easytier-cli 输出（v2.6.4，两节点 p2p 场景）
const realPeerOutput = `| ipv4          | hostname           | cost  | lat(ms) | loss | rx    | tx      | tunnel | NAT     | version        |
|---------------|--------------------|-------|---------|------|-------|---------|--------|---------|----------------|
| 10.144.1.1/24 | jimwong-standardpc | Local | -       | -    | -     | -       | -      | Unknown | 2.6.4-8428a89d |
| 10.144.1.2/24 | jimwong-standardpc | p2p   | 0.95    | 0.0% | 940 B | 1.09 kB | tcp    | Unknown | 2.6.4-8428a89d |
`

func TestParsePeerTableRealOutput(t *testing.T) {
	peers := ParsePeerTable(realPeerOutput)
	if len(peers) != 2 {
		t.Fatalf("应解析出 2 个节点，实际 %d：%+v", len(peers), peers)
	}
	local := peers[0]
	if local.IPv4 != "10.144.1.1/24" || !local.Local || local.Cost != "Local" {
		t.Fatalf("本机行解析错误：%+v", local)
	}
	if local.LatencyOK {
		t.Fatalf("本机延迟为 '-'，不应被当成 0")
	}
	remote := peers[1]
	if remote.IPv4 != "10.144.1.2/24" || remote.Local {
		t.Fatalf("对端行解析错误：%+v", remote)
	}
	if !remote.LatencyOK || remote.LatencyMS != 0.95 {
		t.Fatalf("对端延迟应解析为 0.95，实际 %+v", remote)
	}
	if remote.Tunnel != "tcp" || remote.Rx != "940 B" || remote.Tx != "1.09 kB" {
		t.Fatalf("列错位：%+v", remote)
	}
}

func TestParsePeerTableIgnoresGarbage(t *testing.T) {
	if got := ParsePeerTable("Error: failed to get manage client"); len(got) != 0 {
		t.Fatalf("非表格输出应返回空列表，实际 %+v", got)
	}
	if got := ParsePeerTable(""); len(got) != 0 {
		t.Fatalf("空输出应返回空列表，实际 %+v", got)
	}
}

func TestParsePeerTableStripsANSI(t *testing.T) {
	out := "| ipv4 | hostname | cost | lat(ms) | loss | rx | tx | tunnel | NAT | version |\n" +
		"|---|---|---|---|---|---|---|---|---|---|\n" +
		"| 10.144.1.2/24 | h | \x1b[32mp2p\x1b[0m | 1.5 | 0.0% | 1 B | 2 B | tcp | PortRestricted | 2.6.4 |\n"
	peers := ParsePeerTable(out)
	if len(peers) != 1 || peers[0].Cost != "p2p" {
		t.Fatalf("ANSI 颜色码应被剥离：%+v", peers)
	}
	if peers[0].LatencyMS != 1.5 || !peers[0].LatencyOK {
		t.Fatalf("延迟解析错误：%+v", peers[0])
	}
}
