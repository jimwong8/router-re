// Package easytier 管理 EasyTier（去中心化组网 VPN）节点的配置渲染与进程生命周期。
//
// 设计约束（都是踩过的坑）：
//   - 配置是 TOML，值会被原样写进文件：任何带换行的字段都等于配置注入，必须在 Validate 里拒绝。
//   - rpc_portal 是管理接口，默认只监听 127.0.0.1；暴露到公网等于把节点管理权交出去。
//   - ipv4 留空时 EasyTier 不创建 TUN，只做中继——这在没有 /dev/net/tun 的容器里是唯一可用形态，
//     界面上必须讲清楚，而不是让用户点了"启用"再看进程秒退。
package easytier

import (
	"fmt"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
)

const (
	// 默认监听端口与 EasyTier 官方一致
	defaultListenPort = 11010
	defaultRPCPortal  = "127.0.0.1:15888"
)

// Config 是一个 EasyTier 网络实例的配置。
type Config struct {
	Enabled       bool   `json:"enabled"`
	InstanceName  string `json:"instance_name"`
	NetworkName   string `json:"network_name"`
	NetworkSecret string `json:"network_secret"` // 只写不读：GET 返回空串 + SecretSet
	IPv4          string `json:"ipv4"`           // 10.144.0.1/24，留空=只中继不建 TUN
	RPCPortal     string `json:"rpc_portal"`
	Listeners     []string `json:"listeners"`
	Peers         []string `json:"peers"`
	ProxyNetworks []string `json:"proxy_networks"`
	ExternalNode  string `json:"external_node"`
	// 让本节点成为出口节点（其他节点可经它上网）
	EnableExitNode bool `json:"enable_exit_node"`
	// 中继其他节点的流量（无公网 IP 的节点也能帮别人转发）
	RelayAllPeerTraffic bool `json:"relay_all_peer_traffic"`
}

// Default 返回一份可直接落盘的默认配置。
func Default() Config {
	return Config{
		Enabled:      false,
		InstanceName: "default",
		RPCPortal:    defaultRPCPortal,
		Listeners:    []string{fmt.Sprintf("tcp://0.0.0.0:%d", defaultListenPort), fmt.Sprintf("udp://0.0.0.0:%d", defaultListenPort)},
		Peers:        []string{},
		ProxyNetworks: []string{},
	}
}

// 允许的传输协议（与 easytier-core 的 listeners/peers 语法一致）
var allowedSchemes = map[string]bool{
	"tcp": true, "udp": true, "ws": true, "wss": true,
	"quic": true, "wg": true, "ring": true, "faketcp": true,
}

var hostPortRe = regexp.MustCompile(`^(?:\[[0-9a-fA-F:]+\]|[A-Za-z0-9._-]+):([0-9]{1,5})$`)
var barePortRe = regexp.MustCompile(`^[0-9]{1,5}$`)

// hasControl 判定一个值是否含换行/控制字符——这些值会原样进 TOML，必须拒绝。
func hasControl(s string) bool {
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' || r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

func validPort(p string) bool {
	n, err := strconv.Atoi(p)
	return err == nil && n >= 1 && n <= 65535
}

// ValidateListener 接受 "tcp://0.0.0.0:11010"、"wg:11011" 或裸端口 "11010"。
func ValidateListener(v string) error {
	if v == "" {
		return fmt.Errorf("监听项不能为空")
	}
	if hasControl(v) {
		return fmt.Errorf("监听项含控制字符：%q", v)
	}
	if barePortRe.MatchString(v) {
		if !validPort(v) {
			return fmt.Errorf("端口超出范围：%s", v)
		}
		return nil
	}
	// proto:port 形式
	if i := strings.Index(v, ":"); i > 0 && !strings.Contains(v, "://") {
		scheme := v[:i]
		port := v[i+1:]
		if !allowedSchemes[scheme] {
			return fmt.Errorf("不支持的传输协议 %q（可用 tcp/udp/ws/wss/quic/wg/ring/faketcp）", scheme)
		}
		if !validPort(port) {
			return fmt.Errorf("端口不合法：%s", v)
		}
		return nil
	}
	i := strings.Index(v, "://")
	if i <= 0 {
		return fmt.Errorf("监听项格式应为 tcp://0.0.0.0:11010 或 11010：%s", v)
	}
	scheme := v[:i]
	if !allowedSchemes[scheme] {
		return fmt.Errorf("不支持的传输协议 %q（可用 tcp/udp/ws/wss/quic/wg/ring/faketcp）", scheme)
	}
	rest := strings.TrimPrefix(v[i+3:], "//")
	m := hostPortRe.FindStringSubmatch(rest)
	if m == nil {
		return fmt.Errorf("监听地址应形如 host:port：%s", v)
	}
	if !validPort(m[1]) {
		return fmt.Errorf("端口不合法：%s", v)
	}
	return nil
}

// ValidatePeer 接受 "tcp://1.2.3.4:11010"、"udp://host:11010"。
func ValidatePeer(v string) error {
	if v == "" {
		return fmt.Errorf("对等节点地址不能为空")
	}
	if hasControl(v) {
		return fmt.Errorf("对等节点地址含控制字符：%q", v)
	}
	i := strings.Index(v, "://")
	if i <= 0 {
		return fmt.Errorf("对等节点应形如 tcp://host:11010：%s", v)
	}
	if !allowedSchemes[v[:i]] {
		return fmt.Errorf("不支持的传输协议 %q：%s", v[:i], v)
	}
	rest := strings.TrimPrefix(v[i+3:], "//")
	m := hostPortRe.FindStringSubmatch(rest)
	if m == nil {
		return fmt.Errorf("对等节点地址应形如 host:port：%s", v)
	}
	if !validPort(m[1]) {
		return fmt.Errorf("端口不合法：%s", v)
	}
	return nil
}

// ValidateProxyNetwork 接受 "192.168.1.0/24" 或 "192.168.1.0/24->10.0.0.0/24"。
func ValidateProxyNetwork(v string) error {
	if v == "" {
		return fmt.Errorf("代理网段不能为空")
	}
	if hasControl(v) {
		return fmt.Errorf("代理网段含控制字符：%q", v)
	}
	parts := strings.SplitN(v, "->", 2)
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if !strings.Contains(p, "/") {
			return fmt.Errorf("代理网段必须是 CIDR：%s", v)
		}
		if _, _, err := net.ParseCIDR(p); err != nil {
			return fmt.Errorf("代理网段不合法：%s", p)
		}
	}
	return nil
}

// Validate 校验整份配置。只在 enabled 时才强制网络标识，便于"先存草稿再启用"。
func Validate(c Config) error {
	if hasControl(c.InstanceName) || hasControl(c.NetworkName) || hasControl(c.NetworkSecret) ||
		hasControl(c.IPv4) || hasControl(c.RPCPortal) || hasControl(c.ExternalNode) {
		return fmt.Errorf("配置项含换行或控制字符")
	}
	if c.InstanceName != "" && len(c.InstanceName) > 64 {
		return fmt.Errorf("实例名过长（<=64）")
	}
	if c.IPv4 != "" {
		if strings.Contains(c.IPv4, "/") {
			ip, _, err := net.ParseCIDR(c.IPv4)
			if err != nil || ip.To4() == nil {
				return fmt.Errorf("IPv4 必须是合法的 IPv4 CIDR，如 10.144.0.1/24：%s", c.IPv4)
			}
		} else if ip := net.ParseIP(c.IPv4); ip == nil || ip.To4() == nil {
			return fmt.Errorf("IPv4 不合法：%s", c.IPv4)
		}
	}
	if c.RPCPortal != "" {
		m := hostPortRe.FindStringSubmatch(c.RPCPortal)
		if m == nil || !validPort(m[1]) {
			return fmt.Errorf("RPC 管理地址应形如 127.0.0.1:15888：%s", c.RPCPortal)
		}
	}
	for _, l := range c.Listeners {
		if err := ValidateListener(l); err != nil {
			return err
		}
	}
	for _, p := range c.Peers {
		if err := ValidatePeer(p); err != nil {
			return err
		}
	}
	for _, n := range c.ProxyNetworks {
		if err := ValidateProxyNetwork(n); err != nil {
			return err
		}
	}
	if c.ExternalNode != "" {
		if err := ValidatePeer(c.ExternalNode); err != nil {
			return fmt.Errorf("公共中继节点：%w", err)
		}
	}
	if c.Enabled {
		if strings.TrimSpace(c.NetworkName) == "" {
			return fmt.Errorf("启用时必须填写网络名称")
		}
		if len(c.NetworkSecret) < 8 {
			return fmt.Errorf("网络密钥至少 8 个字符——空密钥等于任何人都能加入这个网络")
		}
	}
	return nil
}

// tomlString 转义成 TOML 基本字符串。
func tomlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// RenderConfig 生成 easytier-core 的 TOML 配置。
// 只输出配置里真正有值的段：空 listeners 会启用默认端口，空 peers 不生成 [[peer]]。
func RenderConfig(c Config) string {
	var b strings.Builder
	name := c.InstanceName
	if name == "" {
		name = "default"
	}
	b.WriteString("instance_name = " + tomlString(name) + "\n")
	if c.IPv4 != "" {
		b.WriteString("ipv4 = " + tomlString(c.IPv4) + "\n")
	}
	rpc := c.RPCPortal
	if rpc == "" {
		rpc = defaultRPCPortal
	}
	b.WriteString("rpc_portal = " + tomlString(rpc) + "\n")
	if len(c.Listeners) > 0 {
		b.WriteString("listeners = [")
		for i, l := range c.Listeners {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(tomlString(l))
		}
		b.WriteString("]\n")
	}
	if len(c.ProxyNetworks) > 0 {
		b.WriteString("proxy_networks = [")
		for i, n := range c.ProxyNetworks {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(tomlString(n))
		}
		b.WriteString("]\n")
	}
	if c.ExternalNode != "" {
		b.WriteString("external_node = " + tomlString(c.ExternalNode) + "\n")
	}
	if c.EnableExitNode {
		b.WriteString("enable_exit_node = true\n")
	}
	if c.RelayAllPeerTraffic {
		b.WriteString("relay_all_peer_traffic = true\n")
	}

	b.WriteString("\n[network_identity]\n")
	b.WriteString("network_name = " + tomlString(c.NetworkName) + "\n")
	b.WriteString("network_secret = " + tomlString(c.NetworkSecret) + "\n")

	for _, p := range c.Peers {
		b.WriteString("\n[[peer]]\n")
		b.WriteString("uri = " + tomlString(p) + "\n")
	}
	return b.String()
}

// ConfigPath 返回配置文件路径（可用 MGM_EASYTIER_CONFIG 覆盖）。
func ConfigPath() string {
	if p := os.Getenv("MGM_EASYTIER_CONFIG"); p != "" {
		return p
	}
	return "/etc/easytier/config.toml"
}

// BinaryPath 返回 easytier-core 路径（可用 MGM_EASYTIER_BINARY 覆盖）。
func BinaryPath() string {
	if p := os.Getenv("MGM_EASYTIER_BINARY"); p != "" {
		return p
	}
	return "easytier-core"
}

// CLIPath 返回 easytier-cli 路径（可用 MGM_EASYTIER_CLI 覆盖）。
func CLIPath() string {
	if p := os.Getenv("MGM_EASYTIER_CLI"); p != "" {
		return p
	}
	return "easytier-cli"
}
