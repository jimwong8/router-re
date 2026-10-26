package vpn

import (
	"os/exec"
	"strings"
	"testing"
)

// wgAvailable 检测系统是否有 wg 命令。
func wgAvailable() bool {
	_, err := exec.LookPath("wg")
	return err == nil
}

// TestGenerateKeypairReal 仅当 wg 存在时验证真实密钥对生成：
// 公私钥都非空、私钥经 wg pubkey 重算能得到同一公钥。
func TestGenerateKeypairReal(t *testing.T) {
	if !wgAvailable() {
		t.Skip("wg command not found; skipping real keypair test")
	}
	priv, pub, err := GenerateKeypair()
	if err != nil {
		t.Fatalf("GenerateKeypair failed: %v", err)
	}
	if priv == "" {
		t.Fatal("empty private key")
	}
	if pub == "" {
		t.Fatal("empty public key")
	}
	if priv == pub {
		t.Fatal("private key equals public key (impossible)")
	}
	// 用 wg pubkey 重算，验证公钥一致
	cmd := exec.Command("wg", "pubkey")
	cmd.Stdin = strings.NewReader(priv)
	recalc, err := cmd.Output()
	if err != nil {
		t.Fatalf("recalc pubkey failed: %v", err)
	}
	if got := strings.TrimSpace(string(recalc)); got != pub {
		t.Fatalf("derived pubkey mismatch: got %q want %q", got, pub)
	}
}

// TestRenderConfig 验证渲染输出包含 [Interface]/[Peer]/AllowedIPs。
func TestRenderConfig(t *testing.T) {
	cfg := WireGuardConfig{
		Interface:  "wg0",
		Address:    "10.0.0.1/24",
		ListenPort: 51820,
		PrivateKey: "SECRET_PRIVKEY",
		Peers: []Peer{
			{
				PublicKey:           "PEER_PUBKEY",
				AllowedIPs:          []string{"10.0.0.2/32", "192.168.2.0/24"},
				Endpoint:            "203.0.113.1:51820",
				PersistentKeepalive: 25,
			},
		},
	}
	out := RenderConfig(cfg)
	for _, want := range []string{
		"[Interface]", "Address = 10.0.0.1/24", "ListenPort = 51820", "PrivateKey = SECRET_PRIVKEY",
		"[Peer]", "PublicKey = PEER_PUBKEY",
		"AllowedIPs = 10.0.0.2/32, 192.168.2.0/24",
		"Endpoint = 203.0.113.1:51820", "PersistentKeepalive = 25",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

// TestRenderConfigEmpty 验证空配置渲染不崩且无 [Peer]。
func TestRenderConfigEmpty(t *testing.T) {
	out := RenderConfig(WireGuardConfig{})
	if !strings.Contains(out, "[Interface]") {
		t.Fatalf("empty cfg should still render [Interface]:\n%s", out)
	}
	if strings.Contains(out, "[Peer]") {
		t.Fatalf("empty cfg should not render [Peer]:\n%s", out)
	}
}
