package acl

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// RenderSysctl renders the `sysctl -w` lines that enable ARP spoofing
// protection on the given interface: strict unicast reverse ARP
// (arp_ignore=1) and local-only announcement (arp_announce=2).
func RenderSysctl(iface string) string {
	return fmt.Sprintf(
		"sysctl -w net.ipv4.conf.%s.arp_ignore=1\n"+
			"sysctl -w net.ipv4.conf.%s.arp_announce=2\n", iface, iface)
}

// netDevices 列出本机网卡名。抽成变量是为了测试能替换，
// 同时避免为了拿一个目录列表就去 shell 里跑 ls。
var netDevices = func() []string {
	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

// validIfaceName 校验网卡名。Linux 的 IFNAMSIZ 是 16，名字里只可能出现
// 字母数字和 -_.: 这几种字符。
//
// 这个校验是安全边界，不是格式美化：iface 会拼进 sysctl 的键名再交给 shell，
// 一个带 `;` 的名字就是一条任意命令（实测 `all; touch /tmp/x; echo` 能落地）。
var validIfaceName = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,32}$`)

// ValidateIface 校验传入的 iface 参数（"all" 是合法特殊值）。
func ValidateIface(iface string) error {
	if iface == "all" {
		return nil
	}
	if !validIfaceName.MatchString(iface) {
		return fmt.Errorf("网卡名 %q 非法：只允许字母数字和 - _ . : 且不超过 32 字符", iface)
	}
	return nil
}

// ApplyArpGuard applies ARP anti-spoof sysctls and verifies by re-reading.
//
// For a concrete interface it writes net.ipv4.conf.all.*, net.ipv4.conf.default.*
// and net.ipv4.conf.<iface>.*. For scope "all" it writes the all/default scopes
// plus every concrete interface enumerated from /sys/class/net.
//
// Each sysctl is applied, then re-read back from /proc/sys/net/ipv4/conf/<scope>/<k>
// and its value confirmed; any failure lists the exact scope/param that failed.
func ApplyArpGuard(r execx.Runner, iface string) error {
	if strings.TrimSpace(iface) == "" {
		return fmt.Errorf("iface required")
	}
	if err := ValidateIface(iface); err != nil {
		return err
	}
	scopes := []string{"all", "default"}
	if iface != "all" {
		scopes = append(scopes, iface)
	} else {
		// 直接读目录，不必再借 shell 跑 ls
		for _, n := range netDevices() {
			if n != "" && n != "lo" && ValidateIface(n) == nil {
				scopes = append(scopes, n)
			}
		}
	}
	var failed []string
	pairs := []struct{ k, val string }{
		{"arp_ignore", "1"},
		{"arp_announce", "2"},
	}
	for _, scope := range scopes {
		for _, p := range pairs {
			key := "net.ipv4.conf." + scope + "." + p.k
			// 参数分开传给 sysctl，不经过 shell：即便 scope 里混进元字符，
			// 也只会被当成一个普通参数，不会变成第二条命令。
			if _, err := r.Run("sysctl", "-w", key+"="+p.val); err != nil {
				failed = append(failed, fmt.Sprintf("set %s: %v", key, err))
				continue
			}
			cur, err := r.Run("sysctl", "-n", key)
			if err != nil {
				failed = append(failed, fmt.Sprintf("verify-read %s: %v", key, err))
				continue
			}
			if strings.TrimSpace(cur) != p.val {
				failed = append(failed, fmt.Sprintf("verify %s: got %q want %q", key, cur, p.val))
			}
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("ARP guard apply failed (%s): %s", strings.Join(scopes, ","), strings.Join(failed, "; "))
	}
	return nil
}

// NeighEntry is a parsed row from `ip -j neigh show`.
type NeighEntry struct {
	If    string `json:"if"`
	IP    string `json:"ip"`
	MAC   string `json:"mac"`
	State string `json:"state"`
}

// StaticBindings reads the current neighbor table (static entries) using the
// JSON output of `ip neigh show`.
func StaticBindings(r execx.Runner) ([]map[string]string, error) {
	out, err := r.RunWithInput("", "ip", "-j", "neigh", "show")
	if err != nil {
		return nil, fmt.Errorf("ip neigh show: %w", err)
	}
	var raw []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, fmt.Errorf("parse neighbor json: %w", err)
	}
	result := make([]map[string]string, 0, len(raw))
	for _, m := range raw {
		row := make(map[string]string)
		for k, v := range m {
			switch vv := v.(type) {
			case string:
				row[k] = vv
			case float64:
				row[k] = fmt.Sprintf("%v", vv)
			default:
				row[k] = fmt.Sprint(v)
			}
		}
		result = append(result, row)
	}
	return result, nil
}
