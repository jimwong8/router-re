package bond

import (
	"fmt"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// BondConfig 描述一条链路聚合。
type BondConfig struct {
	Name     string   `json:"name"`
	Slaves   []string `json:"slaves"`
	Mode     string   `json:"mode"`      // 802.3ad / active-backup / balance-rr / balance-xor
	MIIMon   int      `json:"miimon"`    // 0 或 100-10000 ms
	LACPRate string   `json:"lacp_rate"` // slow / fast（仅 802.3ad 有效）
	XmitHash string   `json:"xmit_hash"` // layer2 / layer2+3 / layer3+4（仅 802.3ad 有效）
	MTU      int      `json:"mtu"`       // 0 或 1500-9000
}

var validModes = map[string]bool{
	"802.3ad":       true,
	"active-backup": true,
	"balance-rr":    true,
	"balance-xor":   true,
}

var validLACPRates = map[string]bool{"slow": true, "fast": true}
var validXmitHash = map[string]bool{"layer2": true, "layer2+3": true, "layer3+4": true}

// ValidateBond 校验 bond 配置。
// - Name 非空且不是 lo/eth*（避免覆盖已有接口）
// - Slaves 至少 1 个，不含 lo、不含 Name 自身、无重复
// - Mode 白名单；MIIMon 0 或 100-10000
// - LACP rate/xmit_hash 只在 mode=802.3ad 时允许设置
// - 每个 slave 必须存在（ip link show <s>）
func ValidateBond(r execx.Runner, c BondConfig) error {
	name := strings.TrimSpace(c.Name)
	if name == "" {
		return fmt.Errorf("name required")
	}
	if name == "lo" {
		return fmt.Errorf("name must not be loopback (lo)")
	}
	if strings.HasPrefix(name, "eth") {
		return fmt.Errorf("name %q looks like an existing physical interface; use bond0/bond1 to avoid overwriting", name)
	}
	if len(c.Slaves) == 0 {
		return fmt.Errorf("slaves must not be empty (at least one slave)")
	}
	seen := map[string]bool{}
	for _, s := range c.Slaves {
		s = strings.TrimSpace(s)
		if s == "" {
			return fmt.Errorf("slave must not be empty")
		}
		if s == "lo" {
			return fmt.Errorf("slave must not be loopback (lo)")
		}
		if s == name {
			return fmt.Errorf("slave %q must not be the bond itself", s)
		}
		if seen[s] {
			return fmt.Errorf("duplicate slave %q", s)
		}
		seen[s] = true
	}
	if !validModes[c.Mode] {
		return fmt.Errorf("mode must be one of 802.3ad/active-backup/balance-rr/balance-xor, got %q", c.Mode)
	}
	if c.MIIMon != 0 && (c.MIIMon < 100 || c.MIIMon > 10000) {
		return fmt.Errorf("miimon must be 0 or in [100,10000], got %d", c.MIIMon)
	}
	if c.MTU != 0 && (c.MTU < 1500 || c.MTU > 9000) {
		return fmt.Errorf("mtu must be 0 or in [1500,9000], got %d", c.MTU)
	}
	// LACP rate/xmit_hash 只在 802.3ad 时允许设置。
	if c.Mode != "802.3ad" {
		if strings.TrimSpace(c.LACPRate) != "" {
			return fmt.Errorf("lacp_rate is only valid for mode=802.3ad, but mode is %q", c.Mode)
		}
		if strings.TrimSpace(c.XmitHash) != "" {
			return fmt.Errorf("xmit_hash is only valid for mode=802.3ad, but mode is %q", c.Mode)
		}
	} else {
		if c.LACPRate != "" && !validLACPRates[c.LACPRate] {
			return fmt.Errorf("lacp_rate must be slow/fast, got %q", c.LACPRate)
		}
		if c.XmitHash != "" && !validXmitHash[c.XmitHash] {
			return fmt.Errorf("xmit_hash must be layer2/layer2+3/layer3+4, got %q", c.XmitHash)
		}
	}
	// 每个 slave 必须存在。
	for _, s := range c.Slaves {
		if _, err := r.Run("ip", "link", "show", strings.TrimSpace(s)); err != nil {
			return fmt.Errorf("slave %q does not exist: %w", s, err)
		}
	}
	return nil
}

// RenderScript 生成 ip 命令脚本（每行一条）。
//
// ⚠ 802.3ad 模式下内核要求**先设好 lacp_rate 再 enslave** slave；
// 否则 lacp_rate 会被静默忽略。因此 RenderScript 在 ip link add 时就把
// lacp_rate 写入 bond master， enslaving 时就无需再设。
func RenderScript(c BondConfig) string {
	var b strings.Builder
	mode := c.Mode
	if mode == "" {
		mode = "active-backup"
	}
	line := "ip link add " + c.Name + " type bond mode " + mode
	if c.MIIMon > 0 {
		line += " miimon " + fmt.Sprintf("%d", c.MIIMon)
	}
	if c.Mode == "802.3ad" {
		if c.LACPRate != "" {
			line += " lacp_rate " + c.LACPRate
		}
		if c.XmitHash != "" {
			line += " xmit_hash_policy " + c.XmitHash
		}
	}
	b.WriteString(line + "\n")
	for _, s := range c.Slaves {
		s = strings.TrimSpace(s)
		b.WriteString("ip link set " + s + " down\n")
		b.WriteString("ip link set " + s + " master " + c.Name + "\n")
		b.WriteString("ip link set " + s + " up\n")
	}
	b.WriteString("ip link set " + c.Name + " up\n")
	if c.MTU > 0 {
		b.WriteString("ip link set dev " + c.Name + " mtu " + fmt.Sprintf("%d", c.MTU) + "\n")
	}
	return b.String()
}

// Apply 执行 bond 配置。必须先 ValidateBond 成功。
// 返回 (ok, detail)。
func Apply(r execx.Runner, c BondConfig) (bool, string) {
	if err := ValidateBond(r, c); err != nil {
		return false, err.Error()
	}
	script := RenderScript(c)
	lines := strings.Split(strings.TrimSpace(script), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		args := strings.Fields(line)
		// args[0] 是 "ip"，args[1:] 是 ip 的参数
		if len(args) < 2 || args[0] != "ip" {
			continue
		}
		if _, err := r.Run("ip", args[1:]...); err != nil {
			return false, fmt.Sprintf("bond apply failed at %q: %v", line, err)
		}
	}
	return true, "bond configured; link aggregation is up"
}

// ReadBond 从 /sys/class/net/<name>/bonding/ 读回真实状态。
func ReadBond(r execx.Runner, name string) (map[string]string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("name required")
	}
	base := "/sys/class/net/" + name + "/bonding"
	out := map[string]string{}
	// mode
	if v, err := r.Run("cat", base+"/mode"); err == nil {
		fields := strings.Fields(strings.TrimSpace(v))
		if len(fields) > 0 {
			out["mode"] = fields[0]
		} else {
			out["mode"] = strings.TrimSpace(v)
		}
	}
	// slaves
	if v, err := r.Run("cat", base+"/slaves"); err == nil {
		out["slaves"] = strings.TrimSpace(v)
	}
	// miimon
	if v, err := r.Run("cat", base+"/miimon"); err == nil {
		out["miimon"] = strings.TrimSpace(v)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("read bond %s: no data from %s", name, base)
	}
	return out, nil
}
