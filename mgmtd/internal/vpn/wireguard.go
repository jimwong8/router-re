// Package vpn 提供 VPN（WireGuard）配置模型、密钥生成与配置渲染。
//
// 密钥对通过 exec.Command("wg", "genkey"/"pubkey") 生成，
// 配置渲染输出可直接写入 /etc/wireguard/wg0.conf 的文本。
package vpn

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Peer 描述一个 WireGuard 对端（隧道节点）。
type Peer struct {
	PublicKey           string   `json:"public_key"`                     // 对端公钥
	AllowedIPs          []string `json:"allowed_ips,omitempty"`          // 允许路由到该对端的网段
	Endpoint            string   `json:"endpoint,omitempty"`             // 对端地址:端口
	PersistentKeepalive int      `json:"persistent_keepalive,omitempty"` // 保活间隔（秒），0 表示不设置
}

// WireGuardConfig 描述一个完整的 WireGuard 隧道配置。
type WireGuardConfig struct {
	Interface  string `json:"interface"`       // 网卡名，如 "wg0"
	Address    string `json:"address"`         // 本端隧道地址，如 "10.0.0.1/24"
	ListenPort int    `json:"listen_port"`     // 监听端口
	PrivateKey string `json:"private_key"`     // 本端私钥
	Peers      []Peer `json:"peers,omitempty"` // 对端列表
}

// GenerateKeypair 用 wg 工具生成一对 WireGuard 密钥。
// 返回私钥（base64）与公钥（base64）。本机无 wg 命令时报错。
func GenerateKeypair() (privateKey, publicKey string, err error) {
	priv, err := exec.Command("wg", "genkey").Output()
	if err != nil {
		return "", "", fmt.Errorf("wg genkey: %w", err)
	}
	priv = bytes.TrimSpace(priv)

	cmd := exec.Command("wg", "pubkey")
	cmd.Stdin = bytes.NewReader(priv)
	pub, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("wg pubkey: %w", err)
	}
	return string(priv), strings.TrimSpace(string(pub)), nil
}

// RenderConfig 将 WireGuardConfig 渲染为 wg0.conf 文本。
//
// 生成的配置包含：
//   - [Interface] 段：Address、ListenPort、PrivateKey
//   - [Peer] 段（每 Peer 一个）：PublicKey、AllowedIPs、Endpoint、PersistentKeepalive
func RenderConfig(cfg WireGuardConfig) string {
	var b strings.Builder

	b.WriteString("[Interface]\n")
	if cfg.Address != "" {
		b.WriteString("Address = " + cfg.Address + "\n")
	}
	if cfg.ListenPort != 0 {
		b.WriteString(fmt.Sprintf("ListenPort = %d\n", cfg.ListenPort))
	}
	if cfg.PrivateKey != "" {
		b.WriteString("PrivateKey = " + cfg.PrivateKey + "\n")
	}

	for _, p := range cfg.Peers {
		b.WriteString("\n[Peer]\n")
		if p.PublicKey != "" {
			b.WriteString("PublicKey = " + p.PublicKey + "\n")
		}
		if len(p.AllowedIPs) > 0 {
			b.WriteString("AllowedIPs = " + strings.Join(p.AllowedIPs, ", ") + "\n")
		}
		if p.Endpoint != "" {
			b.WriteString("Endpoint = " + p.Endpoint + "\n")
		}
		if p.PersistentKeepalive != 0 {
			b.WriteString(fmt.Sprintf("PersistentKeepalive = %d\n", p.PersistentKeepalive))
		}
	}

	return b.String()
}
