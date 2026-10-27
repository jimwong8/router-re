// Package nettools2 提供网络诊断工具（ping / traceroute / 端口探测）的安全入参
// 构造与输出总结。它与既有的 diag 工具互补，但特意做成了纯参数构造 / 纯文本总结，
// 不直接执行命令——执行层由调用方经 execx.Runner 完成。
//
// 设计取舍（沿用 internal/conntrack 与 internal/nat 的注释风格）：
//   - **命令注入防线**：任何最终要拼进命令行的输入都必须先过 ValidateTarget。
//     只放行 IPv4/IPv6 字面量与白名单域名，其余（空格 / 分号 / 管道 / & / $ /
//     反引号 / 重定向 / 换行 / 以 - 开头）一律拒绝。这里绝不做"黑名单"，而是
//     白名单放行，缺省拒绝。
//   - **返回 []string 参数切片而不是拼 shell 字符串**：交给 exec.Command 直接
//     执行，避免 shell 二次解释注入。
//   - Summarize 只做有把握的解析，解析不出来就如实说"无法识别"，**绝不编造结论**。
package nettools2

import (
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"strings"
)

// Result 是一次网络工具的原始结果。
type Result struct {
	OK      bool     `json:"ok"`
	Command string   `json:"command"` // 实际执行的命令（回显给用户，便于判断）
	Lines   []string `json:"lines"`
	Summary string   `json:"summary"` // 一句话结论
	Error   string   `json:"error"`
}

// targetRe 是白名单域名字符集（字母数字、点、连字符），不含空格等危险字符。
var targetRe = regexp.MustCompile(`^[a-zA-Z0-9.-]+$`)

// ValidateTarget 校验目标地址。
//
// 只允许：IPv4/IPv6 字面量、域名（[a-zA-Z0-9.-]，长度 1-253）。
// **必须拒绝**：含空格、分号、|、&、$、`、>、<、换行、以 - 开头（参数注入）。
//
// 这是命令注入防线：任何要拼接进命令行的输入都必须先过这里。策略是白名单放行、
// 缺省拒绝——不试图枚举危险字符，而是只放行安全字符，天然挡住所有注入手法。
func ValidateTarget(target string) error {
	if target == "" {
		return fmt.Errorf("target 不能为空")
	}
	if strings.HasPrefix(target, "-") {
		return fmt.Errorf("target 不能以 - 开头（可能是参数注入）: %q", target)
	}
	if strings.ContainsAny(target, " \t;|&$`><\"'\\\n\r") {
		return fmt.Errorf("target 含非法字符（空格/分号/管道/&/$/反引号/重定向/换行均拒绝，参数注入防线）: %q", target)
	}
	// IPv4/IPv6 字面量
	if ip, err := netip.ParseAddr(target); err == nil {
		if ip.IsValid() {
			return nil
		}
	}
	if ip := net.ParseIP(target); ip != nil {
		return nil
	}
	// 域名白名单
	if len(target) > 253 {
		return fmt.Errorf("target 域名长度超限（最长 253）: %q", target)
	}
	if len(target) < 1 {
		return fmt.Errorf("target 过短")
	}
	if !targetRe.MatchString(target) {
		return fmt.Errorf("target 只能含字母数字、点、连字符（IPv4/IPv6/域名）: %q", target)
	}
	if strings.HasSuffix(target, ".") {
		// 容忍末尾点？域名末尾的点合法但很少见；这里放行（它不含危险字符）。
		_ = target
	}
	return nil
}

// ValidatePort 校验端口：0 或 1-65535。0 表示"任意/不指定端口"。
func ValidatePort(p int) error {
	if p == 0 {
		return nil
	}
	if p < 1 || p > 65535 {
		return fmt.Errorf("port 必须在 1-65535（或 0），got %d", p)
	}
	return nil
}

// ValidateCount 校验 count 在 1-20。
func ValidateCount(count int) error {
	if count < 1 || count > 20 {
		return fmt.Errorf("count 必须在 1-20，got %d", count)
	}
	return nil
}

// PingArgs 生成 ping 命令参数（返回 []string，不拼 shell 字符串）。
// count 1-20；参数注入已在 ValidateTarget 挡掉。
// 返回的参数形如 ["-c", "5", "1.1.1.1"]，供 exec.Command("ping", args...) 使用。
func PingArgs(target string, count int) ([]string, error) {
	if err := ValidateTarget(target); err != nil {
		return nil, err
	}
	if err := ValidateCount(count); err != nil {
		return nil, err
	}
	return []string{"-c", fmt.Sprintf("%d", count), target}, nil
}

// TracerouteArgs 生成 traceroute（或 mtr）参数。
// maxHops 1-30。返回 ["-m", "15", target]。
func TracerouteArgs(target string, maxHops int) ([]string, error) {
	if err := ValidateTarget(target); err != nil {
		return nil, err
	}
	if maxHops < 1 || maxHops > 30 {
		return nil, fmt.Errorf("maxHops 必须在 1-30，got %d", maxHops)
	}
	return []string{"-m", fmt.Sprintf("%d", maxHops), target}, nil
}

// PortProbeArgs 生成端口探测参数。
// 优先用 nc -z -w <timeout>；timeout 1-10。返回 ["-z", "-w", "3" , target, "port"]。
func PortProbeArgs(target string, port, timeoutSec int) ([]string, error) {
	if err := ValidateTarget(target); err != nil {
		return nil, err
	}
	if err := ValidatePort(port); err != nil {
		return nil, err
	}
	if port == 0 {
		return nil, fmt.Errorf("port 探测需要一个具体端口（1-65535），got 0")
	}
	if timeoutSec < 1 || timeoutSec > 10 {
		return nil, fmt.Errorf("timeoutSec 必须在 1-10，got %d", timeoutSec)
	}
	return []string{"-z", "-w", fmt.Sprintf("%d", timeoutSec), target, fmt.Sprintf("%d", port)}, nil
}

// Summarize 把原始输出变成一句话结论。
//   - ping: 解析 "0% packet loss" -> "可达，0% 丢包"；"100% packet loss" -> "不可达"；
//     其它丢包率如实给出。
//   - traceroute: 统计非空行数 -> "经过 N 跳"。
//   - 解析不出来就如实返回 "输出格式无法识别（可能命令不可用）"，**不要猜**。
func Summarize(tool string, lines []string) string {
	switch strings.ToLower(strings.TrimSpace(tool)) {
	case "ping":
		// 在最后一行（统计行）里找 x% packet loss
		joined := strings.Join(lines, "\n")
		if strings.Contains(joined, "% packet loss") {
			// 提取最接近的 x%
			loss := extractPacketLossPct(joined)
			if loss >= 0 {
				if loss == 0 {
					return "可达，0% 丢包"
				}
				if loss == 100 {
					return "不可达（" + itoa(loss) + "% 丢包）"
				}
				return fmt.Sprintf("可达但丢包 %d%%", loss)
			}
		}
		if strings.Contains(joined, "100% packet loss") {
			return "不可达"
		}
		return "输出格式无法识别（可能命令不可用）"
	case "traceroute", "mtr", "tracepath", "tracert":
		n := 0
		for _, l := range lines {
			l = strings.TrimSpace(l)
			if l == "" {
				continue
			}
			// 跳过表头
			if strings.HasPrefix(strings.ToLower(l), "traceroute to") ||
				strings.HasPrefix(strings.ToLower(l), "hostname") ||
				strings.HasPrefix(strings.ToLower(l), "traceroute") {
				continue
			}
			n++
		}
		if n == 0 {
			return "输出格式无法识别（可能命令不可用）"
		}
		return fmt.Sprintf("经过 %d 跳", n)
	default:
		return "输出格式无法识别（可能命令不可用）"
	}
}

// extractPacketLossPct 从 ping 输出中提取 x% packet loss 的百分比；找不到返回 -1。
func extractPacketLossPct(out string) int {
	var pct int
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		// 形如: "5 packets transmitted, 5 received, 0% packet loss, time 4002ms"
		idx := strings.Index(line, "% packet loss")
		if idx < 0 {
			continue
		}
		head := line[:idx]
		// 往回找到数字
		start := idx
		for start > 0 && head[start-1] >= '0' && head[start-1] <= '9' {
			start--
		}
		num := strings.TrimSpace(head[start:idx])
		if num == "" {
			continue
		}
		var v int
		if _, err := fmt.Sscanf(num, "%d", &v); err == nil {
			pct = v
			return pct
		}
	}
	return -1
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
