package route

import (
	"strings"
	"testing"
)

// frr.conf 以行为单位。任何自由字段里一个换行就能插入任意 FRR 指令。
// 最危险的是 password：注入一条 neighbor 就等于和攻击者的路由器建立
// BGP peering —— 路由劫持。
func TestRenderBGPNoInjection(t *testing.T) {
	cases := []struct {
		name     string
		cfg      BGPConfig
		forbidden string
	}{
		{
			"address",
			BGPConfig{LocalAS: 65001, Peers: []BGPPeer{{Address: "10.0.0.2\nlog file /etc/passwd", RemoteAS: 65002}}},
			"log file",
		},
		{
			"password",
			BGPConfig{LocalAS: 65001, Peers: []BGPPeer{{Address: "10.0.0.2", RemoteAS: 65002, Password: "pw\n neighbor 6.6.6.6 remote-as 1"}}},
			"neighbor 6.6.6.6",
		},
		{
			"network",
			BGPConfig{LocalAS: 65001, Networks: []string{"10.1.0.0/24\n address-family ipv6 unicast"}},
			"address-family ipv6",
		},
	}
	for _, c := range cases {
		out := RenderFrrConf(&c.cfg, nil)
		for _, line := range strings.Split(out, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), c.forbidden) {
				t.Fatalf("%s 注入出了独立指令行 %q:\n%s", c.name, line, out)
			}
		}
	}
}

func TestRenderOSPFNoInjection(t *testing.T) {
	cfg := OSPFConfig{
		RouterID: "10.0.0.1",
		Networks: []string{"10.2.0.0/24"},
		Area:     "0\n default-information originate",
	}
	out := RenderFrrConf(nil, &cfg)
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "default-information originate") {
			t.Fatalf("area 注入出了 default-information 行:\n%s", out)
		}
	}
}

func TestValidateBGPRejectsInjection(t *testing.T) {
	cases := []BGPConfig{
		{LocalAS: 65001, Peers: []BGPPeer{{Address: "10.0.0.2\nlog file /x", RemoteAS: 1}}},
		{LocalAS: 65001, Peers: []BGPPeer{{Address: "not-an-ip", RemoteAS: 1}}},
		{LocalAS: 65001, Peers: []BGPPeer{{Address: "10.0.0.2", RemoteAS: 0}}},
		{LocalAS: 65001, Peers: []BGPPeer{{Address: "10.0.0.2", RemoteAS: 1, Password: "pw\nneighbor 6.6.6.6 remote-as 1"}}},
		{LocalAS: 65001, Peers: []BGPPeer{{Address: "10.0.0.2", RemoteAS: 1, Password: "pw with space"}}},
		{LocalAS: 0},
		{LocalAS: 65001, Networks: []string{"10.1.0.0/24\naddress-family ipv6"}},
		{LocalAS: 65001, Networks: []string{"not-a-cidr"}},
		{LocalAS: 65001, RouterID: "10.0.0.1\nx"},
	}
	for i, c := range cases {
		if err := ValidateBGP(c); err == nil {
			t.Errorf("用例 %d 未被拒绝: %+v", i, c)
		}
	}
}

func TestValidateOSPFRejectsInjection(t *testing.T) {
	cases := []OSPFConfig{
		{Area: "0\n default-information originate", Networks: []string{"10.2.0.0/24"}},
		{Area: "0.0.0.1.5"},
		{Area: "abc"},
		{Area: "4294967296"},
		{Networks: []string{"10.2.0.0/24\nx"}},
		{RouterID: "bad-id"},
	}
	for i, c := range cases {
		if err := ValidateOSPF(c); err == nil {
			t.Errorf("用例 %d 未被拒绝: %+v", i, c)
		}
	}
}

func TestValidateAcceptsLegit(t *testing.T) {
	bgp := BGPConfig{
		LocalAS: 65001, RouterID: "10.0.0.1",
		Peers: []BGPPeer{
			{Address: "192.0.2.1", RemoteAS: 65002, Password: "s3cret", MultiHop: true},
			{Address: "2001:db8::1", RemoteAS: 65003},
		},
		Networks: []string{"10.1.0.0/24", "192.0.2.0/24"},
	}
	if err := ValidateBGP(bgp); err != nil {
		t.Errorf("合法 BGP 被误拒: %v", err)
	}
	ospf := OSPFConfig{RouterID: "10.0.0.1", Networks: []string{"10.2.0.0/24"}, Area: "0.0.0.1"}
	if err := ValidateOSPF(ospf); err != nil {
		t.Errorf("合法 OSPF 被误拒: %v", err)
	}
	if err := ValidateOSPF(OSPFConfig{Area: "0"}); err != nil {
		t.Errorf("area=0 被误拒: %v", err)
	}
}

func TestRenderBGPKeepsLegit(t *testing.T) {
	cfg := BGPConfig{LocalAS: 65001, RouterID: "10.0.0.1",
		Peers:    []BGPPeer{{Address: "192.0.2.1", RemoteAS: 65002, Password: "s3cret"}},
		Networks: []string{"10.1.0.0/24"}}
	out := RenderFrrConf(&cfg, nil)
	for _, want := range []string{
		"router bgp 65001",
		" bgp router-id 10.0.0.1",
		" neighbor 192.0.2.1 remote-as 65002",
		" neighbor 192.0.2.1 password s3cret",
		"  network 10.1.0.0/24",
		"  neighbor 192.0.2.1 activate",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("缺少 %q:\n%s", want, out)
		}
	}
}
