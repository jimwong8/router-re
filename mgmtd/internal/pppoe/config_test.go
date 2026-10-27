package pppoe

import (
	"strings"
	"testing"
)

func TestConfigValidate(t *testing.T) {
	good := Config{
		Interface: "eth0", LocalIP: "10.0.0.1",
		PoolStart: "10.0.0.100", PoolEnd: "10.0.0.200",
		DNS: []string{"8.8.8.8"}, ServiceName: "mypppoe",
	}
	if err := good.Validate(); err != nil {
		t.Errorf("good config validate error: %v", err)
	}

	bad := []Config{
		{Interface: "", LocalIP: "10.0.0.1", PoolStart: "10.0.0.10", PoolEnd: "10.0.0.20", ServiceName: "x"},
		{Interface: "eth0", LocalIP: "badip", PoolStart: "10.0.0.1", PoolEnd: "10.0.0.2", ServiceName: "x"},
		{Interface: "eth0", LocalIP: "10.0.0.1", PoolStart: "10.0.0.2", PoolEnd: "10.0.0.1", ServiceName: "x"},
		{Interface: "eth0", LocalIP: "10.0.0.1", PoolStart: "bad", PoolEnd: "10.0.0.1", ServiceName: "x"},
		{Interface: "eth0", LocalIP: "10.0.0.1", PoolStart: "10.0.0.1", PoolEnd: "10.0.0.2", ServiceName: ""},
	}
	for i, b := range bad {
		if err := b.Validate(); err == nil {
			t.Errorf("bad config %d expected error: %+v", i, b)
		}
	}
}

func TestRenderOptions(t *testing.T) {
	cfg := Config{
		Interface: "eth0", LocalIP: "10.0.0.1",
		PoolStart: "10.0.0.100", PoolEnd: "10.0.0.200",
		DNS: []string{"8.8.8.8", "223.5.5.5"}, ServiceName: "mypppoe",
	}
	out, err := RenderOptions(cfg)
	if err != nil {
		t.Fatalf("RenderOptions error: %v", err)
	}
	mustContain := []string{
		"# PPPoE server options",
		"require-chap",
		"login",
		"lcp-echo-interval 10",
		"lcp-echo-failure 2",
		"ms-dns 8.8.8.8",
		"ms-dns 223.5.5.5",
		"-I eth0",
		"service: mypppoe",
	}
	for _, s := range mustContain {
		if !strings.Contains(out, s) {
			t.Errorf("RenderOptions output missing %q:\n%s", s, out)
		}
	}
}

func TestRenderChapSecrets(t *testing.T) {
	users := []User{
		{Name: "alice", Password: "secret1", IP: "10.0.0.101"},
		{Name: "bob", Password: "secret2", IP: "10.0.0.102"},
	}
	out := RenderChapSecrets(users)
	if !strings.Contains(out, "\"alice\" * \"secret1\" \"10.0.0.101\"") {
		t.Errorf("missing alice line:\n%s", out)
	}
	if !strings.Contains(out, "\"bob\" * \"secret2\" \"10.0.0.102\"") {
		t.Errorf("missing bob line:\n%s", out)
	}
}
