package vpnclient

import (
	"strings"
	"testing"
)

func validCfg() Config {
	return Config{
		Interface:  "wg0",
		PrivateKey: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="[:43] + "A", // placeholder replaced below
		ListenPort: 51820,
		Peers: []Peer{
			{
				Name:       "home",
				PublicKey:  "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB="[:43] + "B",
				Endpoint:   "1.2.3.4:51820",
				AllowedIPs: []string{"10.0.0.0/24"},
				Keepalive:  25,
			},
		},
	}
}

// 44-char base64 keys (32 bytes) for tests.
var (
	privKey = "yAnz5TF+lXXJte14tji3zlMNq+hd2rYUIgJBgB3fBmk="
	pubKey  = "xTIBA5rboUvnH4htodjb6eVLH4hZIB3j2oFbG4hFJmk="
	pskKey  = "F1t+3T1x2y3z4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c="
)

func TestValidateRejectsBadInterface(t *testing.T) {
	for _, bad := range []string{"eth0", "wg100", "wg", "wgabc"} {
		c := validCfg()
		c.PrivateKey = privKey
		c.Peers[0].PublicKey = pubKey
		c.Interface = bad
		if err := Validate(c); err == nil {
			t.Fatalf("expected rejection for interface %q", bad)
		}
	}
}

func TestValidateRejectsBadPublicKey(t *testing.T) {
	c := validCfg()
	c.PrivateKey = privKey
	c.Peers[0].PublicKey = "short"
	c.Peers[0].Keepalive = 25
	if err := Validate(c); err == nil {
		t.Fatal("expected rejection for bad public key length")
	}
}

func TestValidateRejectsPresharedEqualsPrivate(t *testing.T) {
	c := validCfg()
	c.PrivateKey = privKey
	c.Peers[0].PublicKey = pubKey
	c.Peers[0].Preshared = privKey // same as private
	c.Peers[0].Keepalive = 25
	if err := Validate(c); err == nil {
		t.Fatal("expected rejection when preshared == private key")
	}
}

func TestValidateRejectsDefaultRouteNoKeepalive(t *testing.T) {
	c := validCfg()
	c.PrivateKey = privKey
	c.Peers[0].PublicKey = pubKey
	c.Peers[0].AllowedIPs = []string{"0.0.0.0/0"}
	c.Peers[0].Keepalive = 0
	err := Validate(c)
	if err == nil {
		t.Fatal("expected rejection for 0.0.0.0/0 + keepalive=0")
	}
	msg := err.Error()
	if !strings.Contains(msg, "黑洞") && !strings.Contains(msg, "保活") {
		t.Fatalf("error must mention 黑洞 or 保活, got: %s", msg)
	}
}

func TestValidateAcceptsValid(t *testing.T) {
	c := validCfg()
	c.PrivateKey = privKey
	c.Peers[0].PublicKey = pubKey
	c.Peers[0].Keepalive = 25
	if err := Validate(c); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

func TestRenderNoPrivateKeyWhenEmpty(t *testing.T) {
	c := validCfg()
	c.PrivateKey = ""
	c.Peers[0].PublicKey = pubKey
	c.Peers[0].Keepalive = 25
	out := Render(c)
	if strings.Contains(out, privKey) {
		t.Fatal("render must not contain private key when empty")
	}
	if !strings.Contains(out, "PrivateKey 由系统保留，不在此渲染") {
		t.Fatalf("render must contain placeholder comment, got:\n%s", out)
	}
}

func TestRenderCommandsFirstElementIsCommandName(t *testing.T) {
	c := validCfg()
	c.PrivateKey = privKey
	c.Peers[0].PublicKey = pubKey
	c.Peers[0].Keepalive = 25
	cmds := RenderCommands(c)
	if len(cmds) == 0 {
		t.Fatal("expected commands")
	}
	for i, cmd := range cmds {
		if len(cmd) == 0 {
			t.Fatalf("command %d is empty", i)
		}
		name := cmd[0]
		if name != "ip" && name != "wg" {
			t.Fatalf("command %d first element should be command name (ip/wg), got %q", i, name)
		}
	}
}

func TestParseWGShow(t *testing.T) {
	out := `interface: wg0
  public key: abc
  private key: (hidden)
  listening port: 51820

peer: AAAApubkeyAAAA
  endpoint: 1.2.3.4:51820
  allowed ips: 10.0.0.0/24
  latest handshake: 10 seconds ago
  transfer: 1.50 KiB received, 2.00 KiB sent

peer: BBBBpubkeyBBBB
  endpoint: 5.6.7.8:51820
  allowed ips: 192.168.1.0/24
  latest handshake: never
  transfer: 0 B received, 0 B sent
`
	peers := ParseWGShow(out)
	if len(peers) != 2 {
		t.Fatalf("expected 2 peers, got %d", len(peers))
	}
	if peers[1].PublicKey != "BBBBpubkeyBBBB" {
		t.Fatalf("second peer pubkey wrong: %q", peers[1].PublicKey)
	}
	if peers[1].LatestHandshake != 0 {
		t.Fatalf("never-handshaken peer must have LatestHandshake=0, got %d", peers[1].LatestHandshake)
	}
	if peers[1].Endpoint != "5.6.7.8:51820" {
		t.Fatalf("endpoint wrong: %q", peers[1].Endpoint)
	}
	if len(peers[1].AllowedIPs) != 1 || peers[1].AllowedIPs[0] != "192.168.1.0/24" {
		t.Fatalf("allowed ips wrong: %v", peers[1].AllowedIPs)
	}
}

func TestHandshakeAgeNever(t *testing.T) {
	ok, msg := HandshakeAge(1000, 0, 300)
	if ok {
		t.Fatal("expected not alive for last=0")
	}
	if msg != "从未握手" {
		t.Fatalf("expected '从未握手', got %q", msg)
	}
}

func TestHandshakeAgeTimeout(t *testing.T) {
	ok, msg := HandshakeAge(1000, 100, 300)
	if ok {
		t.Fatal("expected not alive on timeout")
	}
	if !strings.Contains(msg, "超过阈值") {
		t.Fatalf("expected timeout message, got %q", msg)
	}
}

func TestHandshakeAgeAlive(t *testing.T) {
	ok, _ := HandshakeAge(1000, 900, 300)
	if !ok {
		t.Fatal("expected alive")
	}
}
