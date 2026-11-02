package qos

import (
	"fmt"
	"net"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// ClientMarkBase 是 per-client 防火墙 mark 的起始值。
// 每条 ClientLimit 分配 mark = ClientMarkBase + i（i 是规则索引）。
const ClientMarkBase = 0x100

// ClientLimit 描述对单个 IP 或 MAC 的上/下行速率限制。
type ClientLimit struct {
	Selector     string `json:"selector"`      // "ip:1.2.3.4" 或 "mac:aa:bb:cc:dd:ee:ff"
	UploadKbps   int    `json:"upload_kbps"`   // 上传限速（kbps），<=0 表示不限制
	DownloadKbps int    `json:"download_kbps"` // 下载限速（kbps），<=0 表示不限制
	Comment      string `json:"comment"`       // 备注
}

// ParseSelector 校验并拆分解 Selector 为 (kind, value)。
// kind 为 "ip" 或 "mac"，value 为去掉前缀后的真实值。
func ParseSelector(s string) (kind, value string, err error) {
	s = strings.TrimSpace(s)
	kind, val, ok := strings.Cut(s, ":")
	if !ok {
		return "", "", fmt.Errorf("invalid selector %q: expected \"ip:1.2.3.4\" or \"mac:aa:bb:cc:dd:ee:ff\"", s)
	}
	val = strings.TrimSpace(val)
	switch kind {
	case "ip":
		if ip := net.ParseIP(val); ip == nil {
			return "", "", fmt.Errorf("invalid IP %q", val)
		}
	case "mac":
		if _, e := net.ParseMAC(val); e != nil {
			return "", "", fmt.Errorf("invalid MAC %q: %v", val, e)
		}
	default:
		return "", "", fmt.Errorf("invalid selector kind %q: must be \"ip\" or \"mac\"", kind)
	}
	return kind, val, nil
}

// RenderClientMarkNft 生成 per-client mark 的 nftables 规则文本。
//
// 生成 table inet router_client_qos，含两个 chain：
//   - mark_prerouting  (hook prerouting,  priority mangle)：按 ip saddr / ether saddr 匹配并打 mark
//   - mark_postrouting (hook postrouting, priority mangle)：按 ip daddr / ether daddr 匹配并打 mark（上传方向）
//
// 每条 limit 分配一个 mark = ClientMarkBase + i。
func RenderClientMarkNft(limits []ClientLimit) (string, error) {
	if len(limits) == 0 {
		return "", fmt.Errorf("no client limits")
	}
	var b strings.Builder
	b.WriteString("table inet router_client_qos {\n")

	// prerouting chain：匹配下行（download），按 IP 源地址或 MAC 源地址
	b.WriteString("  chain mark_prerouting {\n")
	b.WriteString("    type filter hook prerouting priority mangle; policy accept;\n")
	for i, lim := range limits {
		kind, val, err := ParseSelector(lim.Selector)
		if err != nil {
			return "", err
		}
		if lim.DownloadKbps <= 0 {
			continue
		}
		mark := ClientMarkBase + i
		if kind == "mac" {
			b.WriteString(fmt.Sprintf("    ether saddr %s meta mark set %#x\n", val, mark))
		} else {
			b.WriteString(fmt.Sprintf("    ip saddr %s meta mark set %#x\n", val, mark))
		}
	}
	b.WriteString("  }\n")

	// postrouting chain：匹配上行（upload），按 IP 目的地址或 MAC 目的地址
	b.WriteString("  chain mark_postrouting {\n")
	b.WriteString("    type filter hook postrouting priority mangle; policy accept;\n")
	for i, lim := range limits {
		kind, val, err := ParseSelector(lim.Selector)
		if err != nil {
			return "", err
		}
		if lim.UploadKbps <= 0 {
			continue
		}
		mark := ClientMarkBase + i
		if kind == "mac" {
			b.WriteString(fmt.Sprintf("    ether daddr %s meta mark set %#x\n", val, mark))
		} else {
			b.WriteString(fmt.Sprintf("    ip daddr %s meta mark set %#x\n", val, mark))
		}
	}
	b.WriteString("  }\n")

	b.WriteString("}\n")
	return b.String(), nil
}

// RenderClientTcCommands 生成 per-client 的 tc HTB 整形命令。
//
// 每个 ClientLimit 生成 4 条命令（针对 Download 方向）：
//  1. tc qdisc replace dev <dev> root handle 1: htb default 9999（幂等重建根qdisc）
//  2. tc class replace dev <dev> parent 1: classid 1:<i+1> htb rate <down>kbit（每客户端一个下行类）
//  3. tc filter replace dev <dev> parent 1: protocol ip handle <mark> fw classid 1:<i+1>（按 mark 分流）
//
// 注意：本实现只做 download 方向（htb 挂在 root，match fw mark 到下行类）。
// Upload 方向需要 ifb（Intermediate Functional Block）镜像到虚拟接口——简化版用注释说明。
func RenderClientTcCommands(limits []ClientLimit, dev string) ([]string, error) {
	if dev == "" {
		dev = "eth0"
	}
	// 渲染层兜底：dev 直接进 tc 命令行，而 ApplyClientLimits 会把每行
	// 按空白切分后 exec —— dev 里的换行等于凭空多出一条被执行的命令。
	// 本包 apply.go 已有同一套 ifaceNameRe，这里再拦一次。
	if err := validateIface(dev); err != nil {
		return nil, err
	}
	if len(limits) == 0 {
		return nil, fmt.Errorf("no client limits")
	}
	cmds := make([]string, 0, len(limits)*3+2)
	// 根 qdisc：先 del 再 add。`tc qdisc replace` 在 root 已是 htb 时会报
	// "Change operation not supported by specified qdisc"，而 `add` 在已存在
	// 时报 "Exclusivity flag on, cannot modify" —— 只有 del+add 幂等可靠。
	// del 在不存在时返回非零，调用方需忽略这条命令的错误。
	cmds = append(cmds, fmt.Sprintf("tc qdisc del dev %s root", dev))
	cmds = append(cmds, fmt.Sprintf("tc qdisc add dev %s root handle 1: htb default 9999", dev))
	for i, lim := range limits {
		if _, _, err := ParseSelector(lim.Selector); err != nil {
			return nil, err
		}
		if lim.DownloadKbps <= 0 {
			continue // 只对 downloadKbps > 0 的下发下行类
		}
		classID := i + 1
		// 下载类
		cmds = append(cmds,
			fmt.Sprintf("tc class replace dev %s parent 1: classid 1:%d htb rate %dkbit", dev, classID, lim.DownloadKbps),
		)
		// 按 mark 分流
		mark := ClientMarkBase + i
		cmds = append(cmds,
			fmt.Sprintf("tc filter replace dev %s parent 1: protocol ip handle %#x fw classid 1:%d", dev, mark, classID),
		)
	}
	return cmds, nil
}

// ApplyClientLimits 真实下发 per-client 限速：
//
//  1. nft delete table inet router_client_qos（幂等，忽略\"不存在\"错误）
//  2. nft -f - 下发 mark 规则（stdin 喂渲染文本）
//  3. 逐条执行 tc 命令
//
// 使用传入的 execx.Runner（测试可注入 FakeRunner 断言命令序列）。
func ApplyClientLimits(r execx.Runner, limits []ClientLimit, dev string) error {
	if len(limits) == 0 {
		return fmt.Errorf("no client limits")
	}
	// 1. 幂等删除旧 table（首次不存在报错，忽略——nft delete 不存在的表返回非零）
	if _, err := r.Run("nft", "delete", "table", "inet", "router_client_qos"); err != nil {
		_ = err // 表不存在或删除失败都继续：管理面会被重复调用，幂等靠"先删再建"
	}
	// 2. 下发 mark 规则
	markNft, err := RenderClientMarkNft(limits)
	if err != nil {
		return err
	}
	if _, err := r.RunWithInput(markNft, "nft", "-f", "-"); err != nil {
		return fmt.Errorf("nft -f - (mark rules): %w", err)
	}
	// 3. 逐条执行 tc 命令
	tcCmds, err := RenderClientTcCommands(limits, dev)
	if err != nil {
		return err
	}
	for _, cmd := range tcCmds {
		fields := strings.Fields(cmd)
		if len(fields) == 0 {
			continue
		}
		// 执行侧白名单：渲染文本只有 tc 命令，其余形态一律拒绝。
		if fields[0] != "tc" {
			return fmt.Errorf("refusing to execute unexpected line %q", cmd)
		}
		if _, err := r.Run(fields[0], fields[1:]...); err != nil {
			// `tc qdisc del` fails when no qdisc exists — expected on a fresh
			// interface, must not abort the rest of the apply.
			if strings.Contains(cmd, "qdisc del") {
				continue
			}
			return fmt.Errorf("tc command %q: %w", cmd, err)
		}
	}
	return nil
}
