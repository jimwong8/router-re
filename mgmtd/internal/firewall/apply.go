package firewall

import (
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// nftBin returns the nftables binary path; falls back to "nft" in PATH.
func nftBin() string {
	for _, p := range []string{"/usr/sbin/nft", "/usr/bin/nft", "/sbin/nft"} {
		if _, err := exec.LookPath(p); err == nil {
			return p
		}
	}
	return "nft"
}

// runNft executes nft with the given args, feeding ruleset to stdin.
// It first tries the binary directly; on permission errors it retries with
// `sudo -n` (non-interactive). A final permission failure is returned as an
// error containing the marker "permission" so callers/tests can distinguish
// it from real syntax errors.
func runNft(args []string, ruleset string) error {
	bin := nftBin()
	run := func(argv []string) (string, error) {
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Stdin = strings.NewReader(ruleset)
		out, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}

	out, err := run(append([]string{bin}, args...))
	if err == nil {
		return nil
	}
	lower := strings.ToLower(out + " " + err.Error())
	if strings.Contains(lower, "operation not permitted") ||
		strings.Contains(lower, "permission denied") {
		// Retry with sudo -n
		if sudoBin, e := exec.LookPath("sudo"); e == nil {
			out2, err2 := run(append([]string{sudoBin, "-n", bin}, args...))
			if err2 == nil {
				return nil
			}
			lower2 := strings.ToLower(out2 + " " + err2.Error())
			if strings.Contains(lower2, "operation not permitted") ||
				strings.Contains(lower2, "permission denied") ||
				strings.Contains(lower2, "password") {
				return fmt.Errorf("nft permission denied (need root): %s", out2)
			}
			return fmt.Errorf("nft failed: %s", out2)
		}
		return fmt.Errorf("nft permission denied (need root): %s", out)
	}
	if out == "" {
		out = err.Error()
	}
	return fmt.Errorf("nft failed: %s", out)
}

// Validate 先做字段级白名单校验，再用 `nft -c -f -` 做语法检查。
//
// 白名单是必需的，不能只靠 nft -c：解释器能挡住"语法错误"的注入，但挡不住
// **语法合法**的注入。例如 Dst="1.1.1.1 counter accept; ip daddr 2.2.2.2"
// 渲染出来仍是一段合法的 nft 脚本——等于让调用方追加任意规则，把防火墙
// 改成自己想要的形状。先把每个字段限制成"看起来只能是它本来的东西"。
func Validate(rules []Rule) error {
	if err := validateFields(rules); err != nil {
		return err
	}
	return runNft([]string{"-c", "-f", "-"}, RenderNftables(rules))
}

var (
	// 地址：IPv4/IPv6 字面量或 CIDR，允许通配 0.0.0.0/0 与 ::/0
	addrRe = regexp.MustCompile(`^[0-9A-Fa-f:.]+(/[0-9]{1,3})?$`)
	// 协议：nft 允许的少数几个名字
	protoAllow = map[string]bool{"tcp": true, "udp": true, "icmp": true, "icmpv6": true, "sctp": true, "gre": true, "esp": true, "ah": true}
	// 端口：单个端口或 a-b 范围
	portRe = regexp.MustCompile(`^[0-9]{1,5}(-[0-9]{1,5})?$`)
	// 动作：只允许这几个终结符。nft 里其它动作（jump/goto/log/...）会把
	// 渲染出的行结构改掉，不在本接口的语义范围内。
	actionAllow = map[string]bool{"accept": true, "drop": true, "reject": true, "return": true}
	// 链名：自定义链也只允许标识符形态
	chainRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]{0,31}$`)
)

func validateFields(rules []Rule) error {
	if len(rules) > 1000 {
		return fmt.Errorf("too many rules: %d (max 1000)", len(rules))
	}
	for i, r := range rules {
		if r.Chain != "" && !chainRe.MatchString(r.Chain) {
			return fmt.Errorf("rule %d: invalid chain %q", i, r.Chain)
		}
		for name, v := range map[string]string{"src": r.Src, "dst": r.Dst} {
			if v == "" {
				continue
			}
			if !addrRe.MatchString(v) {
				return fmt.Errorf("rule %d: invalid %s address %q", i, name, v)
			}
			if _, _, err := net.ParseCIDR(normalizeCIDR(v)); err != nil {
				if net.ParseIP(v) == nil {
					return fmt.Errorf("rule %d: invalid %s address %q", i, name, v)
				}
			}
		}
		if r.Proto != "" && !protoAllow[strings.ToLower(r.Proto)] {
			return fmt.Errorf("rule %d: unsupported proto %q", i, r.Proto)
		}
		if r.DPort != "" {
			if !portRe.MatchString(r.DPort) {
				return fmt.Errorf("rule %d: invalid dport %q", i, r.DPort)
			}
			if !portInRange(r.DPort) {
				return fmt.Errorf("rule %d: dport out of range %q", i, r.DPort)
			}
		}
		if !actionAllow[r.Action] {
			return fmt.Errorf("rule %d: unsupported action %q", i, r.Action)
		}
	}
	return nil
}

// normalizeCIDR 把裸 IP 补成 /32 或 /128，供 ParseCIDR 校验。
func normalizeCIDR(v string) string {
	if strings.Contains(v, "/") {
		return v
	}
	if strings.Contains(v, ":") {
		return v + "/128"
	}
	return v + "/32"
}

// portInRange 校验端口（或端口段的两端）都在 1-65535 内。
func portInRange(v string) bool {
	for _, p := range strings.Split(v, "-") {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return false
		}
	}
	return true
}

// Apply validates the rendered ruleset and then applies it with `nft -f -`.
// Validate is always run first; if it fails, nothing is applied.
// The previous table is deleted first so repeated Apply calls replace the
// ruleset instead of appending duplicate rules (nft -f appends to an existing
// chain).
func Apply(rules []Rule) error {
	if err := Validate(rules); err != nil {
		return err
	}
	// Delete the old table so the apply is idempotent. A missing table makes
	// nft exit non-zero; that is expected on first apply, so ignore the error.
	_ = runNft([]string{"delete", "table", "inet", "router"}, "")
	return runNft([]string{"-f", "-"}, RenderNftables(rules))
}
