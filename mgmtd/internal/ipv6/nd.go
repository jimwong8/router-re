package ipv6

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// NDConfig holds neighbor discovery parameters for one interface.
// Values mirror /proc/sys/net/ipv6/conf/<iface>/ tunables.
type NDConfig struct {
	Iface           string `json:"iface"`
	RaIntervalMin   int    `json:"ra_interval_min"`   // 3-1350 秒
	RaIntervalMax   int    `json:"ra_interval_max"`   // >= min 且 <= 1800
	HopLimit        int    `json:"hop_limit"`         // 0-255, 0=未设置
	Mtu             int    `json:"mtu"`               // 0 或 1280-65535
	ReachableTimeMs int    `json:"reachable_time_ms"` // 0-3600000
	RetransTimerMs  int    `json:"retrans_timer_ms"`  // 0-3600000
	DadTransmits    int    `json:"dad_transmits"`     // 0-10
	AcceptRa        bool   `json:"accept_ra"`         // 路由器一般关
	Forwarding      bool   `json:"forwarding"`
}

// ValidateND checks NDConfig fields against their documented ranges.
// Iface must be non-empty and not loopback ("lo").
func ValidateND(c NDConfig) error {
	iface := strings.TrimSpace(c.Iface)
	if iface == "" {
		return fmt.Errorf("iface required")
	}
	if iface == "lo" {
		return fmt.Errorf("iface must not be loopback (lo)")
	}
	// ⚠ RA 间隔 0 表示"不改这项"，必须允许。
	// 用户常常只想调 hop_limit / mtu / dad_transmits，不想碰 RA 参数；
	// 强行要求 ra_interval_min 必填会让这种最普通的用法被拒（实测踩到）。
	// 只有两个都非 0 时才校验取值范围和大小关系。
	if c.RaIntervalMin != 0 && (c.RaIntervalMin < 3 || c.RaIntervalMin > 1350) {
		return fmt.Errorf("ra_interval_min must be 0 (leave unchanged) or in [3,1350], got %d", c.RaIntervalMin)
	}
	if c.RaIntervalMax != 0 {
		if c.RaIntervalMax > 1800 {
			return fmt.Errorf("ra_interval_max must be <= 1800, got %d", c.RaIntervalMax)
		}
		if c.RaIntervalMin != 0 && c.RaIntervalMax < c.RaIntervalMin {
			return fmt.Errorf("ra_interval_max (%d) must be >= ra_interval_min (%d)", c.RaIntervalMax, c.RaIntervalMin)
		}
	}
	if c.HopLimit < 0 || c.HopLimit > 255 {
		return fmt.Errorf("hop_limit must be in [0,255], got %d", c.HopLimit)
	}
	if c.Mtu != 0 && (c.Mtu < 1280 || c.Mtu > 65535) {
		return fmt.Errorf("mtu must be 0 or in [1280,65535], got %d", c.Mtu)
	}
	if c.ReachableTimeMs < 0 || c.ReachableTimeMs > 3600000 {
		return fmt.Errorf("reachable_time_ms must be in [0,3600000], got %d", c.ReachableTimeMs)
	}
	if c.RetransTimerMs < 0 || c.RetransTimerMs > 3600000 {
		return fmt.Errorf("retrans_timer_ms must be in [0,3600000], got %d", c.RetransTimerMs)
	}
	if c.DadTransmits < 0 || c.DadTransmits > 10 {
		return fmt.Errorf("dad_transmits must be in [0,10], got %d", c.DadTransmits)
	}
	return nil
}

// ndSysctl returns path to a per-interface sysctl.
func ndSysctl(iface, key string) string {
	return "/proc/sys/net/ipv6/conf/" + iface + "/" + key
}

// writeSysctl 直接把值写进 /proc/sys 文件。
//
// ⚠ 不要用 `sysctl -w /proc/sys/xxx=y`：sysctl 的 -w 参数期望的是**点号 key**
// （net.ipv6.conf.ens18.hop_limit），传文件路径时它会自己再拼一次 /proc/sys 前缀，
// 变成 `sysctl: 无法获取/proc/sys//proc/sys/net/ipv6/...`，写入必然失败。
// 直接写文件既不依赖 sysctl 的 key 规则，也不需要外部进程。
func writeSysctlReal(path, val string) error {
	if err := os.WriteFile(path, []byte(val+"\n"), 0o644); err != nil {
		return fmt.Errorf("write %s=%s: %w", path, val, err)
	}
	return nil
}

// sysctlWriter 是可替换的写入口，测试用它避免真的去改内核参数。
// 生产路径始终是 writeSysctlReal。
var sysctlWriter = writeSysctlReal

func writeSysctl(path, val string) error { return sysctlWriter(path, val) }

// ipv6Bool writes a boolean sysctl (1/0).
func ipv6Bool(r execx.Runner, path string, v bool) error {
	val := "0"
	if v {
		val = "1"
	}
	return writeSysctl(path, val)
}

// ipv6Int writes an integer sysctl.
func ipv6Int(r execx.Runner, path string, v int) error {
	return writeSysctl(path, strconv.Itoa(v))
}

// ApplyND applies ND parameters via sysctl (and `ip link set` for MTU),
// then reads back each value to confirm it stuck. Mismatches are errors.
func ApplyND(r execx.Runner, c NDConfig) error {
	if err := ValidateND(c); err != nil {
		return err
	}
	writes := []struct {
		key string
		val interface{}
	}{
		{"hop_limit", c.HopLimit},
		{"reachable_time", c.ReachableTimeMs},
		{"retrans_time", c.RetransTimerMs},
		{"dad_transmits", c.DadTransmits},
	}
	if c.AcceptRa {
		writes = append(writes, struct {
			key string
			val interface{}
		}{"accept_ra", 1})
	} else {
		writes = append(writes, struct {
			key string
			val interface{}
		}{"accept_ra", 0})
	}
	if c.Forwarding {
		writes = append(writes, struct {
			key string
			val interface{}
		}{"forwarding", 1})
	} else {
		writes = append(writes, struct {
			key string
			val interface{}
		}{"forwarding", 0})
	}
	for _, w := range writes {
		path := ndSysctl(c.Iface, w.key)
		iv := w.val.(int)
		if iv == 0 {
			continue
		}
		if err := ipv6Int(r, path, iv); err != nil {
			return err
		}
		// 回读确认
		got, err := r.Run("cat", path)
		if err != nil {
			return fmt.Errorf("read back %s: %w", path, err)
		}
		if strings.TrimSpace(got) != strconv.Itoa(iv) {
			return fmt.Errorf("readback mismatch %s: want %d got %q", path, iv, got)
		}
	}
	// MTU isn't a sysctl — use ip link set, then read back via ip.
	if c.Mtu > 0 {
		if _, err := r.Run("ip", "link", "set", "dev", c.Iface, "mtu", strconv.Itoa(c.Mtu)); err != nil {
			return fmt.Errorf("ip link set dev %s mtu %d: %w", c.Iface, c.Mtu, err)
		}
		out, err := r.Run("ip", "link", "show", "dev", c.Iface)
		if err != nil {
			return fmt.Errorf("read back mtu via ip link: %w", err)
		}
		if !strings.Contains(out, "mtu "+strconv.Itoa(c.Mtu)) {
			return fmt.Errorf("readback mismatch mtu: want %d, ip output does not contain it (%q)", c.Mtu, out)
		}
	}
	return nil
}

// ReadND reads back the real ND parameters for an interface from /proc/sys.
// ra_interval_min/max are radvd-level, not sysctl, so they're left as
// sentinel 0/0 (not applicable) here.
func ReadND(r execx.Runner, iface string) (NDConfig, error) {
	iface = strings.TrimSpace(iface)
	if iface == "" {
		return NDConfig{}, fmt.Errorf("iface required")
	}
	if iface == "lo" {
		return NDConfig{}, fmt.Errorf("iface must not be loopback (lo)")
	}
	readInt := func(key string) (int, error) {
		out, err := r.Run("cat", ndSysctl(iface, key))
		if err != nil {
			return 0, err
		}
		return strconv.Atoi(strings.TrimSpace(out))
	}
	var c NDConfig
	c.Iface = iface
	if v, err := readInt("hop_limit"); err == nil {
		c.HopLimit = v
	}
	if v, err := readInt("reachable_time"); err == nil {
		c.ReachableTimeMs = v
	}
	if v, err := readInt("retrans_time"); err == nil {
		c.RetransTimerMs = v
	}
	if v, err := readInt("dad_transmits"); err == nil {
		c.DadTransmits = v
	}
	if v, err := readInt("accept_ra"); err == nil {
		c.AcceptRa = v == 1
	}
	if v, err := readInt("forwarding"); err == nil {
		c.Forwarding = v == 1
	}
	// mtu: from ip link show
	if out, err := r.Run("ip", "-6", "link", "show", "dev", iface); err == nil {
		// e.g. "mtu 1500"
		if i := strings.Index(out, "mtu "); i >= 0 {
			rest := out[i+4:]
			sp := strings.IndexAny(rest, " \n")
			if sp > 0 {
				rest = rest[:sp]
			}
			if v, err := strconv.Atoi(rest); err == nil {
				c.Mtu = v
			}
		}
	}
	return c, nil
}
