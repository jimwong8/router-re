package frp

import (
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	cfg := Config{ServerAddr: "1.2.3.4", ServerPort: 7000, Tunnels: []Tunnel{{LocalPort: 80, RemotePort: 8080}}}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (Config{ServerAddr: "", ServerPort: 7000}).Validate(); err == nil {
		t.Fatal("expected error")
	}
}

func TestRenderConfig(t *testing.T) {
	cfg := Config{ServerAddr: "1.2.3.4", ServerPort: 7000, Token: "tok", Tunnels: []Tunnel{{Name: "web", Type: "tcp", LocalPort: 80, RemotePort: 8080}}}
	out := RenderConfig(cfg)
	if !strings.Contains(out, "server_addr = 1.2.3.4") {
		t.Fatalf("missing server_addr: %s", out)
	}
	if !strings.Contains(out, "[web]") || !strings.Contains(out, "local_port = 80") || !strings.Contains(out, "remote_port = 8080") {
		t.Fatalf("missing tunnel: %s", out)
	}
}
