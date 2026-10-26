package route

import (
	"strings"
	"testing"
)

func TestRenderBGP(t *testing.T) {
	cfg := BGPConfig{
		LocalAS:  65001,
		RouterID: "1.1.1.1",
		Peers: []BGPPeer{
			{Address: "10.0.0.2", RemoteAS: 65002},
		},
		Networks: []string{"192.168.1.0/24"},
	}
	out := RenderBGP(cfg)
	if !strings.Contains(out, "router bgp") {
		t.Fatalf("missing 'router bgp':\n%s", out)
	}
	if !strings.Contains(out, "neighbor") {
		t.Fatalf("missing 'neighbor':\n%s", out)
	}
	if !strings.Contains(out, "remote-as") {
		t.Fatalf("missing 'remote-as':\n%s", out)
	}
}

func TestRenderOSPF(t *testing.T) {
	cfg := OSPFConfig{
		RouterID: "2.2.2.2",
		Networks: []string{"192.168.2.0/24"},
		Area:     "0",
	}
	out := RenderOSPF(cfg)
	if !strings.Contains(out, "router ospf") {
		t.Fatalf("missing 'router ospf':\n%s", out)
	}
	if !strings.Contains(out, "area") {
		t.Fatalf("missing 'area':\n%s", out)
	}
}

func TestRenderFrrConfBoth(t *testing.T) {
	bgp := &BGPConfig{LocalAS: 65001, RouterID: "1.1.1.1"}
	ospf := &OSPFConfig{RouterID: "2.2.2.2", Area: "0"}
	out := RenderFrrConf(bgp, ospf)
	if !strings.Contains(out, "router bgp") {
		t.Fatalf("missing bgp section:\n%s", out)
	}
	if !strings.Contains(out, "router ospf") {
		t.Fatalf("missing ospf section:\n%s", out)
	}
}
