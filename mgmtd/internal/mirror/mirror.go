// Package mirror 配置端口镜像：把某个接口的流量复制一份到监控口。
//
// 实现方式是 tc mirred（不占用 ebtables，也不需要内核模块）。
// 铁律：绝不能对 lo 做 tc（会把本地回环卡死，ssh/curl 全挂）。
package mirror

import (
	"fmt"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// Config 描述一份端口镜像配置。
type Config struct {
	SourceIface string `json:"source_iface"` // 被镜像的口
	TargetIface string `json:"target_iface"` // 监控口
	Direction   string `json:"direction"`    // ingress / egress / both
	Enabled     bool   `json:"enabled"`
}

// validIfaceName 校验 Linux 网卡名。
//
// 注：这些字段当前都经 execx.DefaultRunner 走 exec.Command 的 argv（全仓
// 没有任何 sh -c/bash -c），所以 `;` `|` `$()` 不会被 shell 解释 —— 这里
// 不是命令注入。加白名单是纵深防御：渲染出的 tc/nft 文本会被返回给前端、
// 也可能被运维复制到 shell 里手工执行，非法字符应尽早失败而不是留下畸形命令。
func validIfaceName(field, s string) error {
	if s == "" {
		return fmt.Errorf("%s must not be empty", field)
	}
	if len(s) > 15 {
		return fmt.Errorf("%s 名不能超过 15 字节（IFNAMSIZ），收到 %q", field, s)
	}
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' || r == '@' || r == ':'
		if !ok {
			return fmt.Errorf("%s 只能包含字母数字和 _-.:@（发现 %q）", field, r)
		}
	}
	return nil
}

// oneLine 把任意字符串压成单行，渲染层兜底用。
func oneLine(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\t' || r == '\n' || r == '\r' || r == '\v' || r == '\f':
			b.WriteByte(' ')
		case r < 0x20 || r == 0x7f:
			// 丢弃
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Validate 校验镜像配置。
// 规则：
//   - 两个接口都非空，且不能相同（镜像到自己没意义，还会环回放大流量）
//   - 都不能是 "lo"（对 lo 做 tc 会把本地回环卡死）
//   - Direction 只能是 ingress/egress/both
func Validate(c Config) error {
	if c.SourceIface == "" {
		return fmt.Errorf("source_iface must not be empty")
	}
	if c.TargetIface == "" {
		return fmt.Errorf("target_iface must not be empty")
	}
	if err := validIfaceName("source_iface", c.SourceIface); err != nil {
		return err
	}
	if err := validIfaceName("target_iface", c.TargetIface); err != nil {
		return err
	}
	if c.SourceIface == c.TargetIface {
		return fmt.Errorf("source_iface and target_iface must differ (mirroring an interface onto itself is meaningless and would loop-amplify traffic): both %q", c.SourceIface)
	}
	if c.SourceIface == "lo" {
		return fmt.Errorf("refusing to mirror loopback source %q: tc on lo throttles local traffic and locks out ssh/curl", c.SourceIface)
	}
	if c.TargetIface == "lo" {
		return fmt.Errorf("refusing to mirror onto loopback target %q: tc on lo throttles local traffic and locks out ssh/curl", c.TargetIface)
	}
	switch c.Direction {
	case "ingress", "egress", "both":
	default:
		return fmt.Errorf("direction must be ingress/egress/both, got %q", c.Direction)
	}
	return nil
}

// ingressScript 生成 ingress 方向的镜像命令段。
func ingressScript(src, tgt string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("tc qdisc del dev %s ingress 2>/dev/null || true\n", src))
	b.WriteString(fmt.Sprintf("tc qdisc add dev %s ingress\n", src))
	b.WriteString(fmt.Sprintf("tc filter add dev %s parent ffff: protocol all u32 match u32 0 0 action mirred egress mirror dev %s\n", src, tgt))
	return b.String()
}

// egressScript 生成 egress 方向的镜像命令段。
func egressScript(src, tgt string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("tc qdisc del dev %s root 2>/dev/null || true\n", src))
	b.WriteString(fmt.Sprintf("tc qdisc add dev %s root handle 1: prio\n", src))
	b.WriteString(fmt.Sprintf("tc filter add dev %s parent 1: protocol all u32 match u32 0 0 action mirred egress mirror dev %s\n", src, tgt))
	return b.String()
}

// RenderScript 生成 tc 命令，每行一条。
// ingress 用 ingressScript，egress 用 egressScript，both 两段都生成。
// 禁用时返回空字符串。
func RenderScript(c Config) string {
	if !c.Enabled {
		return ""
	}
	var b strings.Builder
	// 渲染层兜底：tc 命令以行为单位，接口名里的换行会多出一条命令
	src := oneLine(c.SourceIface)
	tgt := oneLine(c.TargetIface)
	switch c.Direction {
	case "ingress":
		b.WriteString(ingressScript(src, tgt))
	case "egress":
		b.WriteString(egressScript(src, tgt))
	case "both":
		b.WriteString(ingressScript(src, tgt))
		b.WriteString(egressScript(src, tgt))
	}
	return b.String()
}

// Cleanup 生成清理命令，把 ingress 和 root 上的镜像规则都删掉。
func Cleanup(c Config) string {
	src := oneLine(c.SourceIface)
	return fmt.Sprintf("tc qdisc del dev %s ingress 2>/dev/null || true\n", src) +
		fmt.Sprintf("tc qdisc del dev %s root 2>/dev/null || true\n", src)
}

// Apply 执行 RenderScript 里的每条命令。
// 返回 (applied, detail)。校验失败（含 lo）时一条命令都不执行。
// 任何一条失败，detail 里包含该命令原文和 stderr。
func Apply(r execx.Runner, c Config) (bool, string) {
	if err := Validate(c); err != nil {
		return false, "validate: " + err.Error()
	}
	if !c.Enabled {
		return false, "mirror disabled in config"
	}
	for _, line := range strings.Split(RenderScript(c), "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		// ⚠ 必须先剥掉 shell 语法。RenderScript 生成的是给人看的 shell 行
		// （末尾带 `2>/dev/null || true`），而这里走的是 exec.Command 的 argv，
		// 不经 shell —— 直接 Fields 拆分会把 "2>/dev/null"、"||"、"true"
		// 当成 tc 的参数传进去，tc 报错被下面的 qdisc del 分支吞掉，
		// 结果「旧 qdisc 实际没删」→ 下一次 qdisc add 报 File exists，
		// 整个镜像配置在第二次应用时必然失败。
		t = strings.ReplaceAll(t, "2>/dev/null || true", "")
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		fields := strings.Fields(t)
		if len(fields) == 0 {
			continue
		}
		// 执行侧白名单：渲染文本只有 tc 命令；接口名已在 Validate 限制字符集，
		// 这里再做一层 fail-closed，保证任何渲染层缺陷都不会变成任意 argv 执行。
		if fields[0] != "tc" {
			return false, fmt.Sprintf("refusing to execute unexpected line %q", t)
		}
		if _, err := r.Run(fields[0], fields[1:]...); err != nil {
			// `tc qdisc del` 在无 qdisc 时返回非零——预期行为，吞掉。
			if strings.Contains(t, "qdisc del") {
				continue
			}
			return false, fmt.Sprintf("command failed: %s → %v", t, err)
		}
	}
	return true, fmt.Sprintf("mirror %s → %s (%s) applied", c.SourceIface, c.TargetIface, c.Direction)
}
