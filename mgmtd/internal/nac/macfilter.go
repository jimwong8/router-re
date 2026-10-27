package nac

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// Device is one MAC access-control entry.
type Device struct {
	MAC    string `json:"mac"`
	Name   string `json:"name"`
	IP     string `json:"ip"`
	Policy string `json:"policy"` // allow | deny | quarantine
	Note   string `json:"note"`
	// 以下两个字段来自邻居表回读，配置侧的 Device 里为空（omitempty）
	Iface string `json:"iface,omitempty"`
	State string `json:"state,omitempty"`
}

// Config holds MAC access-control mode and devices.
type Config struct {
	Mode    string   `json:"mode"` // off | whitelist | blacklist
	Devices []Device `json:"devices"`
}

// Validate checks the MAC filter config.
func Validate(c Config) error {
	switch c.Mode {
	case "off", "whitelist", "blacklist":
	default:
		return fmt.Errorf("mode must be off, whitelist or blacklist, got %q", c.Mode)
	}
	if c.Mode == "off" {
		return nil
	}
	seen := map[string]string{}
	for _, d := range c.Devices {
		mac, err := net.ParseMAC(strings.TrimSpace(d.MAC))
		if err != nil {
			return fmt.Errorf("invalid MAC %q: %v", d.MAC, err)
		}
		if len(mac) != 6 {
			return fmt.Errorf("MAC %q is %d bytes: only 48-bit MACs are supported (EUI-64 is rejected)", d.MAC, len(mac))
		}
		if d.Policy != "allow" && d.Policy != "deny" && d.Policy != "quarantine" {
			return fmt.Errorf("device %q: policy must be allow/deny/quarantine, got %q", d.MAC, d.Policy)
		}
		if d.IP != "" {
			if net.ParseIP(strings.TrimSpace(d.IP)) == nil {
				return fmt.Errorf("device %q: invalid IP %q", d.MAC, d.IP)
			}
		}
		upper := strings.ToUpper(mac.String())
		if prev, ok := seen[upper]; ok {
			return fmt.Errorf("duplicate MAC %q: conflicts between %q and %q", mac.String(), prev, d.Name)
		}
		seen[upper] = d.Name
	}
	if c.Mode == "whitelist" {
		allowed := 0
		for _, d := range c.Devices {
			if d.Policy == "allow" {
				allowed++
			}
		}
		if allowed == 0 {
			return fmt.Errorf("whitelist mode with zero allow devices would block everyone — this is a self-lockout: add at least one allow device")
		}
	}
	return nil
}

// RenderNft emits the router_nac table. The forward chain never sets a
// policy drop (default accept); only matched devices are dropped. Chain
// declarations are multi-line. Never flushes the ruleset.
func RenderNft(c Config) string {
	var b strings.Builder
	b.WriteString("table inet router_nac {\n")
	b.WriteString("\tchain forward {\n")
	b.WriteString("\t\ttype filter hook forward priority filter;\n")
	b.WriteString("\t\t# NAC: default accept; only matched devices are dropped.\n")
	b.WriteString("\t\tct state established,related accept\n")
	if c.Mode == "off" {
		b.WriteString("\t\t# mode=off: no rules\n")
	} else if c.Mode == "whitelist" {
		b.WriteString("\t\t# whitelist: drop everything not on the allow list\n")
		for _, d := range c.Devices {
			if d.Policy != "allow" {
				continue
			}
			upper := strings.ToUpper(strings.TrimSpace(d.MAC))
			b.WriteString("\t\tether saddr " + upper + " accept\n")
		}
		b.WriteString("\t\tdrop\n")
	} else { // blacklist
		b.WriteString("\t\t# blacklist: drop devices on the deny/quarantine list\n")
		for _, d := range c.Devices {
			if d.Policy != "deny" && d.Policy != "quarantine" {
				continue
			}
			upper := strings.ToUpper(strings.TrimSpace(d.MAC))
			b.WriteString("\t\tether saddr " + upper + " drop\n")
		}
	}
	b.WriteString("\t}\n")
	b.WriteString("}\n")
	return b.String()
}

// ApplyNAC validates, replaces the router_nac table and confirms via nft list.
func ApplyNAC(r execx.Runner, c Config) error {
	if err := Validate(c); err != nil {
		return err
	}
	if _, err := r.Run("nft", "delete", "table", "inet", "router_nac"); err != nil {
		_ = err
	}
	nft := RenderNft(c)
	if _, err := r.RunWithInput(nft, "nft", "-f", "-"); err != nil {
		return err
	}
	return nil
}

// Neighbors reads the neighbor (ARP/ND) table as JSON.
func Neighbors(r execx.Runner) ([]Device, error) {
	out, err := r.Run("ip", "-j", "neigh", "show")
	if err != nil || strings.Contains(out, "BusyBox") || strings.HasPrefix(strings.TrimSpace(out), "Usage:") {
		// OpenWrt 的 BusyBox ip 不支持 -j（实测 v1.36.1 会打印 usage 并返回 1）。
		// 退回文本解析，否则整个邻居表在真机上就是空的 —— 而空列表会让人以为
		// "网络里没有设备"，比报错更误导。
		return neighborsFromText(r)
	}
	out = strings.TrimSpace(out)
	if out == "" || out == "[]" {
		return []Device{}, nil
	}
	// ⚠ iproute2 的 neigh JSON 里 state 是**数组**（如 ["REACHABLE"]），不是字符串。
	// 写成 string 会让 Unmarshal 直接报 "cannot unmarshal array into Go struct
	// field .state of type string"，整个邻居表 500。
	var raw []struct {
		Dev   string   `json:"dev"`
		IP    string   `json:"dst"`
		MAC   string   `json:"lladdr"`
		State []string `json:"state"`
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, fmt.Errorf("parse ip -j neigh show: %w", err)
	}
	res := []Device{}
	for _, e := range raw {
		res = append(res, Device{
			MAC:    e.MAC,
			IP:     e.IP,
			Name:   e.Dev,
			Note:   strings.Join(e.State, ","),
			Policy: "",
		})
	}
	return res, nil
}

// neighborsFromText 解析 `ip neigh show` 的纯文本输出。
//
// 为什么要这个：OpenWrt 的 BusyBox ip（v1.36.1）不支持 `-j`，`ip -j neigh`
// 只打印 usage 并返回 1。没有这个 fallback，真机上的邻居表永远是空的。
//
// 真实输出格式（每行一条）：
//
//	10.100.1.1 dev br-lan lladdr aa:bb:cc:dd:ee:ff REACHABLE
//	10.100.1.50 dev br-lan FAILED
//	10.100.1.60 dev br-lan lladdr 11:22:33:44:55:66 STALE
//
// 注意 FAILED/INCOMPLETE 这类行**没有 lladdr**，MAC 为空但设备仍要出现
// （这恰恰是"有设备但不可达"，是有用的信息）。
func neighborsFromText(r execx.Runner) ([]Device, error) {
	out, err := r.Run("ip", "neigh", "show")
	if err != nil {
		return nil, fmt.Errorf("ip neigh show: %w", err)
	}
	res := []Device{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		d := Device{IP: f[0]}
		// 字段顺序不固定（lladdr/dev 谁先都可能），逐段扫
		for i := 1; i < len(f)-1; i++ {
			switch f[i] {
			case "dev":
				d.Iface = f[i+1]
			case "lladdr":
				d.MAC = f[i+1]
			}
		}
		// 最后一个全大写单词通常是状态
		last := f[len(f)-1]
		if last == strings.ToUpper(last) && !strings.Contains(last, ":") {
			d.State = last
		}
		if d.IP != "" {
			res = append(res, d)
		}
	}
	return res, nil
}
