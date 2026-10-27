package vpnclient

import (
	"encoding/base64"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
)

// Peer 是一个 WireGuard 客户端对端（本机作为客户端连出去）。
type Peer struct {
	Name       string   `json:"name"`
	PublicKey  string   `json:"public_key"`
	Endpoint   string   `json:"endpoint"`    // host:port
	AllowedIPs []string `json:"allowed_ips"` // CIDR
	Keepalive  int      `json:"keepalive"`   // 秒，0=关
	Preshared  string   `json:"preshared,omitempty"`
}

// Config 是 WireGuard 客户端配置。
type Config struct {
	Interface  string `json:"interface"`   // wg0..wg99
	PrivateKey string `json:"private_key"`
	ListenPort int    `json:"listen_port"`
	Peers      []Peer `json:"peers"`
}

var ifaceRe = regexp.MustCompile(`^wg[0-9]{1,2}$`)

// isBase64Key 判断是否为 44 字符 base64（WireGuard 32 字节 key）。
func isBase64Key(s string) bool {
	if len(s) != 44 {
		return false
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return false
	}
	return len(b) == 32
}

// Validate 校验配置。
func Validate(c Config) error {
	if !ifaceRe.MatchString(c.Interface) {
		return fmt.Errorf("interface must match ^wg[0-9]{1,2}$ (wg0..wg99), got %q", c.Interface)
	}
	// PrivateKey：空允许（表示用已有的），但非空必须是合法 44 字符 base64 key。
	if c.PrivateKey != "" && !isBase64Key(c.PrivateKey) {
		return fmt.Errorf("private_key must be a 44-char base64 WireGuard key (32 bytes), got %d chars", len(c.PrivateKey))
	}
	if c.ListenPort < 0 || c.ListenPort > 65535 {
		return fmt.Errorf("listen_port must be 0-65535, got %d", c.ListenPort)
	}
	if len(c.Peers) == 0 {
		return fmt.Errorf("peers must not be empty")
	}
	names := map[string]bool{}
	for i, p := range c.Peers {
		if strings.TrimSpace(p.Name) == "" {
			return fmt.Errorf("peer[%d]: name required", i)
		}
		if names[p.Name] {
			return fmt.Errorf("peer name %q duplicated", p.Name)
		}
		names[p.Name] = true
		if !isBase64Key(p.PublicKey) {
			return fmt.Errorf("peer %q: public_key must be a 44-char base64 WireGuard key", p.Name)
		}
		// Preshared 不能等于 PrivateKey（配置错误但极难发现）。
		if p.Preshared != "" && c.PrivateKey != "" && p.Preshared == c.PrivateKey {
			return fmt.Errorf("peer %q: preshared key must not equal the interface private key", p.Name)
		}
		if p.Preshared != "" && !isBase64Key(p.Preshared) {
			return fmt.Errorf("peer %q: preshared must be a 44-char base64 key", p.Name)
		}
		// Endpoint host:port
		host, portStr, err := net.SplitHostPort(p.Endpoint)
		if err != nil || host == "" || portStr == "" {
			return fmt.Errorf("peer %q: endpoint must be host:port, got %q", p.Name, p.Endpoint)
		}
		port, err := strconv.Atoi(portStr)
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("peer %q: endpoint port must be 1-65535, got %q", p.Name, portStr)
		}
		if len(p.AllowedIPs) == 0 {
			return fmt.Errorf("peer %q: allowed_ips must not be empty", p.Name)
		}
		hasDefault := false
		for _, cidr := range p.AllowedIPs {
			if _, _, err := net.ParseCIDR(cidr); err != nil {
				return fmt.Errorf("peer %q: allowed_ips %q is not a valid CIDR", p.Name, cidr)
			}
			if cidr == "0.0.0.0/0" {
				hasDefault = true
			}
		}
		// 0.0.0.0/0 + Keepalive==0 => 全部流量走隧道但无保活，断线后静默黑洞。
		if hasDefault && p.Keepalive == 0 {
			return fmt.Errorf("peer %q: allowed_ips contains 0.0.0.0/0 but keepalive is 0 — all traffic would be routed into the tunnel with no keepalive, creating a silent black hole on disconnect (黑洞/保活)", p.Name)
		}
		if p.Keepalive < 0 || p.Keepalive > 65535 {
			return fmt.Errorf("peer %q: keepalive must be 0-65535, got %d", p.Name, p.Keepalive)
		}
	}
	return nil
}

// Render 生成 wg0.conf 格式（权限敏感：调用方要用 0600 写）。
func Render(c Config) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[Interface]\n")
	fmt.Fprintf(&b, "ListenPort = %d\n", c.ListenPort)
	if c.PrivateKey != "" {
		fmt.Fprintf(&b, "PrivateKey = %s\n", c.PrivateKey)
	} else {
		b.WriteString("# PrivateKey 由系统保留，不在此渲染\n")
	}
	for _, p := range c.Peers {
		fmt.Fprintf(&b, "\n[Peer]\n")
		fmt.Fprintf(&b, "PublicKey = %s\n", p.PublicKey)
		if p.Preshared != "" {
			fmt.Fprintf(&b, "PresharedKey = %s\n", p.Preshared)
		}
		fmt.Fprintf(&b, "Endpoint = %s\n", p.Endpoint)
		fmt.Fprintf(&b, "AllowedIPs = %s\n", strings.Join(p.AllowedIPs, ", "))
		if p.Keepalive > 0 {
			fmt.Fprintf(&b, "PersistentKeepalive = %d\n", p.Keepalive)
		}
	}
	return b.String()
}

// RenderCommands 返回要执行的命令序列（每条 [][]string，绝不拼成 shell 字符串）。
// 顺序：创建 interface -> 设地址 -> 设 private key -> 加 peers -> up。
// 地址从 AllowedIPs 推导，不硬编码。
func RenderCommands(c Config) [][]string {
	var cmds [][]string
	iface := c.Interface
	cmds = append(cmds, []string{"ip", "link", "add", "dev", iface, "type", "wireguard"})
	// 设地址：从 peers 的 AllowedIPs 推导（取第一个 peer 的第一个 AllowedIP 作为本端地址）。
	derivedAddr := ""
	if len(c.Peers) > 0 && len(c.Peers[0].AllowedIPs) > 0 {
		derivedAddr = c.Peers[0].AllowedIPs[0]
	}
	if derivedAddr != "" {
		cmds = append(cmds, []string{"ip", "address", "add", derivedAddr, "dev", iface})
	}
	if c.PrivateKey != "" {
		cmds = append(cmds, []string{"wg", "set", iface, "private-key", c.PrivateKey})
	}
	for _, p := range c.Peers {
		args := []string{"wg", "set", iface, "peer", p.PublicKey,
			"endpoint", p.Endpoint,
			"allowed-ips", strings.Join(p.AllowedIPs, ",")}
		if p.Keepalive > 0 {
			args = append(args, "persistent-keepalive", strconv.Itoa(p.Keepalive))
		}
		if p.Preshared != "" {
			args = append(args, "preshared-key", p.Preshared)
		}
		cmds = append(cmds, args)
	}
	cmds = append(cmds, []string{"ip", "link", "set", iface, "up"})
	return cmds
}

// PeerStatus 是 wg show 解析出的对端状态。
type PeerStatus struct {
	PublicKey      string   `json:"public_key"`
	Endpoint       string   `json:"endpoint"`
	AllowedIPs     []string `json:"allowed_ips"`
	LatestHandshake int64   `json:"latest_handshake"` // unix 秒，0=从未
	RxBytes        int64    `json:"rx_bytes"`
	TxBytes        int64    `json:"tx_bytes"`
}

// ParseWGShow 解析 `wg show <iface>` 输出。
func ParseWGShow(out string) []PeerStatus {
	var peers []PeerStatus
	var cur *PeerStatus
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "peer:") {
			if cur != nil {
				peers = append(peers, *cur)
			}
			cur = &PeerStatus{PublicKey: strings.TrimSpace(strings.TrimPrefix(trimmed, "peer:"))}
			continue
		}
		if cur == nil {
			continue
		}
		// key: value
		idx := strings.Index(trimmed, ":")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(trimmed[:idx])
		val := strings.TrimSpace(trimmed[idx+1:])
		switch key {
		case "endpoint":
			cur.Endpoint = val
		case "allowed ips":
			parts := strings.Split(val, ",")
			var ips []string
			for _, pp := range parts {
				pp = strings.TrimSpace(pp)
				if pp != "" {
					ips = append(ips, pp)
				}
			}
			cur.AllowedIPs = ips
		case "latest handshake":
			cur.LatestHandshake = parseHandshake(val)
		case "transfer":
			rx, tx := parseTransfer(val)
			cur.RxBytes = rx
			cur.TxBytes = tx
		}
	}
	if cur != nil {
		peers = append(peers, *cur)
	}
	return peers
}

func parseHandshake(val string) int64 {
	v := strings.ToLower(strings.TrimSpace(val))
	if v == "" || v == "never" || v == "(none)" || v == "none" || v == "0" {
		return 0
	}
	// "N seconds ago" 风格：无法还原 unix 时间，返回 0（视为未握手可识别）。
	if strings.Contains(v, "ago") {
		return 0
	}
	// 纯数字
	if n, err := strconv.ParseInt(v, 10, 64); err == nil {
		return n
	}
	return 0
}

func parseTransfer(val string) (int64, int64) {
	// "1.23 KiB received, 4.56 KiB sent" 或 "100 B received, 200 B sent"
	rx := parseBytesBefore(val, "received")
	tx := parseBytesBefore(val, "sent")
	return rx, tx
}

func parseBytesBefore(s, marker string) int64 {
	i := strings.Index(s, marker)
	if i < 0 {
		return 0
	}
	// 往回找数字+单位
	prefix := strings.TrimSpace(s[:i])
	// prefix like "1.23 KiB" or "100 B"
	parts := strings.Fields(prefix)
	if len(parts) == 0 {
		return 0
	}
	numStr := parts[len(parts)-2]
	unit := ""
	if len(parts) >= 2 {
		unit = parts[len(parts)-1]
	}
	// numStr 可能是最后一个字段前的数字
	f, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		// 尝试最后一个字段
		if len(parts) >= 1 {
			if f2, err2 := strconv.ParseFloat(parts[len(parts)-1], 64); err2 == nil {
				f = f2
				unit = ""
			} else {
				return 0
			}
		} else {
			return 0
		}
	}
	mult := int64(1)
	switch strings.ToLower(unit) {
	case "b":
		mult = 1
	case "kb":
		mult = 1000
	case "kib":
		mult = 1024
	case "mb":
		mult = 1000 * 1000
	case "mib":
		mult = 1024 * 1024
	case "gb":
		mult = 1000 * 1000 * 1000
	case "gib":
		mult = 1024 * 1024 * 1024
	}
	return int64(f * float64(mult))
}

// HandshakeAge 判断握手是否存活。
func HandshakeAge(now int64, last int64, timeoutSec int) (bool, string) {
	if last == 0 {
		return false, "从未握手"
	}
	age := now - last
	if age > int64(timeoutSec) {
		return false, fmt.Sprintf("最近握手在 %d 秒前，超过阈值 %d 秒", age, timeoutSec)
	}
	return true, "存活"
}