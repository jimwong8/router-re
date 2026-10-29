// Package htb 实现 HTB（Hierarchical Token Bucket）带宽保证的渲染与下发。
//
// 铁律（踩过的坑，务必遵守）：
//   - 绝不能对 lo 做任何 tc 操作。对 lo 挂 qdisc 会把本地回环限速，
//     本机 ssh/curl/本地 HTTP 全部卡死（曾实测本地响应从 3ms 涨到 20s）。
//     因此 ValidateIface 与 Apply 都会显式拦截 lo，双保险。
//   - tc 根 qdisc 只有 del + add 是幂等的：replace/add 在已有根 qdisc 时
//     会报 "Change operation not supported by specified qdisc" 或
//     "Exclusivity flag on, cannot modify"。所以脚本第一行永远是
//     `tc qdisc del ... 2>/dev/null || true`。
//   - 破坏性测试默认 SKIP，需 MGM_ALLOW_REAL_APPLY=1 才真下发。
package htb

import (
	"fmt"
	"net"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/dpi"
	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// Class 是一条 HTB 叶子类：保证速率 rate，可突发到 ceil，优先级 prio。
type Class struct {
	Name        string   `json:"name"`          // 类名，必须唯一
	RateMbit    float64  `json:"rate_mbit"`     // 保证速率（mbit）
	CeilMbit    float64  `json:"ceil_mbit"`     // 峰值速率（mbit），必须 >= RateMbit
	Priority    int      `json:"priority"`      // 1-7，数字越小越优先
	MatchPorts  []int    `json:"match_ports"`   // 匹配的层4目的端口
	MatchDstIPs []string `json:"match_dst_ips"` // 匹配的目的 IP/CIDR
	MatchMark   int      `json:"match_mark"`    // 匹配的 fwmark（nft 打标后用 fw 过滤）
}

// Config 是一份 HTB 整形配置：挂在某个出口接口上的根带宽 + 一组叶子类。
type Config struct {
	Iface        string  `json:"iface"`          // 出口接口，绝不能是 lo
	RootRateMbit float64 `json:"root_rate_mbit"` // 根带宽（mbit）
	DefaultClass string  `json:"default_class"`  // 默认类名（可空）
	Classes      []Class `json:"classes"`
	Enabled      bool    `json:"enabled"`
}

// ValidateIface 校验接口名。拒绝空串、含空格、以及 lo。
//
// lo 是本地回环：对它做 tc 会限速所有本机进程间通信（ssh、curl 到
// 127.0.0.1、本地数据库连接），等于把管理面锁死在自己门外。
func ValidateIface(iface string) error {
	if iface == "" {
		return fmt.Errorf("iface must not be empty")
	}
	if strings.ContainsAny(iface, " \t\n") {
		return fmt.Errorf("iface %q must not contain whitespace", iface)
	}
	if iface == "lo" {
		return fmt.Errorf("refusing to shape loopback interface %q: tc on lo throttles local traffic and locks out ssh/curl", iface)
	}
	return nil
}

// Validate 校验整份配置。
func Validate(c Config) error {
	if err := ValidateIface(c.Iface); err != nil {
		return err
	}
	if c.RootRateMbit <= 0 {
		return fmt.Errorf("root_rate_mbit must be > 0, got %g", c.RootRateMbit)
	}
	if c.RootRateMbit > 100000 {
		return fmt.Errorf("root_rate_mbit %g exceeds 100000", c.RootRateMbit)
	}
	seen := map[string]bool{}
	for i, cl := range c.Classes {
		if cl.Name == "" {
			return fmt.Errorf("class %d: name must not be empty", i)
		}
		if seen[cl.Name] {
			return fmt.Errorf("duplicate class name %q", cl.Name)
		}
		seen[cl.Name] = true
		if cl.RateMbit <= 0 {
			return fmt.Errorf("class %q: rate_mbit must be > 0, got %g", cl.Name, cl.RateMbit)
		}
		if cl.RateMbit > c.RootRateMbit {
			return fmt.Errorf("class %q: rate_mbit %g exceeds root_rate_mbit %g", cl.Name, cl.RateMbit, c.RootRateMbit)
		}
		if cl.CeilMbit < cl.RateMbit {
			return fmt.Errorf("class %q: ceil_mbit %g is below rate_mbit %g (ceil < rate is meaningless)", cl.Name, cl.CeilMbit, cl.RateMbit)
		}
		if cl.CeilMbit > c.RootRateMbit {
			return fmt.Errorf("class %q: ceil_mbit %g exceeds root_rate_mbit %g", cl.Name, cl.CeilMbit, c.RootRateMbit)
		}
		if cl.Priority < 1 || cl.Priority > 7 {
			return fmt.Errorf("class %q: priority must be 1-7, got %d", cl.Name, cl.Priority)
		}
		for _, p := range cl.MatchPorts {
			if p < 1 || p > 65535 {
				return fmt.Errorf("class %q: match port %d out of range 1-65535", cl.Name, p)
			}
		}
		for _, ip := range cl.MatchDstIPs {
			if net.ParseIP(ip) == nil {
				if _, _, err := net.ParseCIDR(ip); err != nil {
					return fmt.Errorf("class %q: invalid match dst %q (need IP or CIDR)", cl.Name, ip)
				}
			}
		}
	}
	if c.DefaultClass != "" && !seen[c.DefaultClass] {
		return fmt.Errorf("default_class %q not found in classes", c.DefaultClass)
	}
	return nil
}

// RenderScript 生成 HTB 整形的 tc 命令序列（每行一条，顺序即执行顺序）。
//
// 顺序至关重要：
//  1. `tc qdisc del dev <iface> root 2>/dev/null || true` —— 先删旧根 qdisc。
//     tc 根 qdisc 只有 del+add 是幂等的；replace/add 在已有根 qdisc 时会报
//     "Change operation not supported by specified qdisc" 或
//     "Exclusivity flag on, cannot modify"。del 在不存在时返回非零，故吞掉。
//  2. `tc qdisc add dev <iface> root handle 1: htb default 10` —— 建根 HTB。
//  3. `tc class add dev <iface> parent 1: classid 1:1 htb rate <root>mbit` —— 根类。
//  4. 每条 class：classid 从 1:10 开始编号，避开根类 1:1。
//  5. 每条 match port：u32 按 dport 分流到对应 classid。
//  6. 每条 match dst IP：u32 按 dst 分流（/32 掩码）。
func RenderScript(c Config) string {
	var b strings.Builder
	// 1) 幂等：先删旧根 qdisc（不存在时忽略）
	b.WriteString("# 幂等重建：先删除已存在的根 qdisc（tc 根 qdisc 只有 del+add 幂等）\n")
	b.WriteString(fmt.Sprintf("tc qdisc del dev %s root 2>/dev/null || true\n", c.Iface))
	// 2) 根 qdisc
	b.WriteString("# 建立根 HTB qdisc，默认类 1:10\n")
	b.WriteString(fmt.Sprintf("tc qdisc add dev %s root handle 1: htb default 10\n", c.Iface))
	// 3) 根类（承载总带宽）
	b.WriteString("# 根类：总带宽\n")
	b.WriteString(fmt.Sprintf("tc class add dev %s parent 1: classid 1:1 htb rate %gmbit\n", c.Iface, c.RootRateMbit))
	// 4) 叶子类：classid 从 1:10 开始
	for i, cl := range c.Classes {
		classID := 10 + i
		b.WriteString(fmt.Sprintf("# 叶子类 %s：保证 %gmbit，峰值 %gmbit，优先级 %d\n", cl.Name, cl.RateMbit, cl.CeilMbit, cl.Priority))
		b.WriteString(fmt.Sprintf("tc class add dev %s parent 1:1 classid 1:%d htb rate %gmbit ceil %gmbit prio %d\n",
			c.Iface, classID, cl.RateMbit, cl.CeilMbit, cl.Priority))
		// 5) 端口匹配
		for _, port := range cl.MatchPorts {
			b.WriteString(fmt.Sprintf("#   %s: 目的端口 %d → classid 1:%d\n", cl.Name, port, classID))
			b.WriteString(fmt.Sprintf("tc filter add dev %s protocol ip parent 1:0 prio %d u32 match ip dport %d 0xffff flowid 1:%d\n",
				c.Iface, cl.Priority, port, classID))
		}
		// 6) 目的 IP 匹配
		for _, ip := range cl.MatchDstIPs {
			mask := "/32"
			if _, _, err := net.ParseCIDR(ip); err == nil {
				// 已是 CIDR，直接用
			} else {
				ip = ip + mask
			}
			b.WriteString(fmt.Sprintf("#   %s: 目的地址 %s → classid 1:%d\n", cl.Name, ip, classID))
			b.WriteString(fmt.Sprintf("tc filter add dev %s protocol ip parent 1:0 prio %d u32 match ip dst %s flowid 1:%d\n",
				c.Iface, cl.Priority, ip, classID))
		}
		// 7) fwmark 匹配（DPI 按应用打标后在此整形）
		if cl.MatchMark > 0 {
			b.WriteString(fmt.Sprintf("#   %s: fwmark %d → classid 1:%d\n", cl.Name, cl.MatchMark, classID))
			b.WriteString(fmt.Sprintf("tc filter add dev %s protocol ip parent 1:0 prio %d handle %d fw flowid 1:%d\n",
				c.Iface, cl.Priority, cl.MatchMark, classID))
		}
	}
	return b.String()
}

// countClasses 从 `tc class show dev <iface>` 输出中数出 class 条数。
// tc 输出里每个 class 以 "class htb 1:X ..." 开头。
func countClasses(out string) int {
	n := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "class ") {
			n++
		}
	}
	return n
}

// Apply 校验并逐条执行 RenderScript 的命令，最后回读确认。
//
// 返回 (applied, detail)：
//   - 校验失败（含 iface=lo）→ (false, 错误说明)，此时一条命令都不会执行。
//   - 任何一条 tc 命令失败 → (false, 含该命令原文与 stderr 的说明)。
//   - 全部成功但回读 class 数与配置不一致 → (false, 数量不匹配说明)。
//   - 全部成功且数量一致 → (true, 成功说明)。
func Apply(r execx.Runner, c Config) (bool, string) {
	// 双保险：即使调用方绕过 Validate，也在这里再拦一次 lo。
	if err := Validate(c); err != nil {
		return false, "validate: " + err.Error()
	}
	if c.Iface == "lo" {
		return false, "refusing to shape loopback interface \"lo\": tc on lo throttles local traffic and locks out ssh/curl"
	}
	if !c.Enabled {
		return false, "htb is disabled in config"
	}

	lines := strings.Split(RenderScript(c), "\n")
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		fields := strings.Fields(t)
		if len(fields) == 0 {
			continue
		}
		// 渲染器只产出简单 tc 命令，无 shell 元字符，可直接按空白切分。
		if _, err := r.Run(fields[0], fields[1:]...); err != nil {
			// `tc qdisc del` 在根 qdisc 不存在时返回非零——这是预期行为，吞掉。
			if strings.Contains(t, "qdisc del") {
				continue
			}
			return false, fmt.Sprintf("command failed: %s → %v", t, err)
		}
	}

	// 回读确认：class 数量必须等于 len(c.Classes)+1（+1 是根类 1:1）
	out, err := r.Run("tc", "class", "show", "dev", c.Iface)
	if err != nil {
		return false, fmt.Sprintf("readback failed: tc class show dev %s → %v", c.Iface, err)
	}
	got := countClasses(out)
	want := len(c.Classes) + 1
	if got != want {
		return false, fmt.Sprintf("readback mismatch: tc class show dev %s reports %d classes, want %d (len(classes)+1)", c.Iface, got, want)
	}
	return true, fmt.Sprintf("applied %d classes on %s (root %gmbit)", len(c.Classes), c.Iface, c.RootRateMbit)
}

// Cleanup 删除接口上的根 qdisc。接口不存在或本来没有 qdisc 时不算错误。
func Cleanup(r execx.Runner, iface string) error {
	if err := ValidateIface(iface); err != nil {
		return err
	}
	if _, err := r.Run("tc", "qdisc", "del", "dev", iface, "root"); err != nil {
		// 接口不存在 / 没有 qdisc 都返回非零，属于正常情况。
		if strings.Contains(err.Error(), "Cannot find") || strings.Contains(err.Error(), "No such file") ||
			strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "Exclusivity") {
			return nil
		}
		return err
	}
	return nil
}

// Show 返回 `tc -s class show dev <iface>` 的原始输出（含统计计数器）。
func Show(r execx.Runner, iface string) (string, error) {
	if err := ValidateIface(iface); err != nil {
		return "", err
	}
	return r.Run("tc", "-s", "class", "show", "dev", iface)
}

// LimitRulesToHTB converts dpi limit rules into an HTB Config. Each limit rule
// becomes one leaf class with rate=ceil=RateMbit and priority 5, matched by
// the rule's fwmark.
func LimitRulesToHTB(rules []dpi.MarkRule, iface string, rootMbit float64) Config {
	classes := []Class{}
	for i, r := range rules {
		if r.Action != "limit" {
			continue
		}
		mark := r.Mark
		if mark <= 0 {
			mark = 200 + i
		}
		classes = append(classes, Class{
			Name:        r.App,
			RateMbit:    float64(r.RateMbit),
			CeilMbit:    float64(r.RateMbit),
			Priority:    5,
			MatchDstIPs: []string{},
			MatchMark:   mark,
		})
	}
	return Config{
		Iface:        iface,
		RootRateMbit: rootMbit,
		DefaultClass: "",
		Classes:      classes,
		Enabled:      true,
	}
}
