package sysctl

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// 内核参数页。
//
// 字段名与取值范围逐字对齐 iKuai 的 system/kernel-params 接口
// （bbr / syn_send_timeout / established_timeout / udp_stream_timeout…），
// 这样从爱快迁过来的运维习惯不用改。
//
// 落点是 11 个 net.netfilter.nf_conntrack_* timeout 加 BBR 拥塞控制。
// 这些参数决定了「路由器同时能撑多少条连接、半开连接占多少内存」，
// 调错的表现是连不上或者内存被 conntrack 表吃光，所以每一项都给区间，
// 超范围直接拒绝写入，不做"尽力而为"的钳位。

// KernelParams 是一组内核网络参数。
type KernelParams struct {
	BBR                int `json:"bbr"`                 // 0/1
	SynSendTimeout     int `json:"syn_send_timeout"`    // 5-60
	SynRecvTimeout     int `json:"syn_recv_timeout"`    // 5-60
	EstablishedTimeout int `json:"established_timeout"` // 600-86400
	FinWaitTimeout     int `json:"fin_wait_timeout"`    // 5-60
	CloseWaitTimeout   int `json:"close_wait_timeout"`  // 5-60
	LastAckTimeout     int `json:"last_ack_timeout"`    // 5-60
	TimeWaitTimeout    int `json:"time_wait_timeout"`   // 5-60
	CloseTimeout       int `json:"close_timeout"`       // 5-60
	UDPTimeout         int `json:"udp_timeout"`         // 5-60
	UDPStreamTimeout   int `json:"udp_stream_timeout"`  // 30-1800
	ICMPTimeout        int `json:"icmp_timeout"`        // 5-100
}

// KernelParamSpec 描述一个参数的 sysctl 键、区间与说明。
type KernelParamSpec struct {
	Field string
	Key   string
	Min   int
	Max   int
	Def   int
	Unit  string
	Desc  string
}

// KernelSpecs 是全部可调参数的定义，顺序即界面展示顺序。
var KernelSpecs = []KernelParamSpec{
	{"syn_send_timeout", "net.netfilter.nf_conntrack_tcp_timeout_syn_sent", 5, 60, 120, "秒", "SYN 发出后等待握手的超时"},
	{"syn_recv_timeout", "net.netfilter.nf_conntrack_tcp_timeout_syn_recv", 5, 60, 60, "秒", "收到 SYN 后等待 ACK 的超时"},
	{"established_timeout", "net.netfilter.nf_conntrack_tcp_timeout_established", 600, 86400, 432000, "秒", "已建立连接的空闲保持时间"},
	{"fin_wait_timeout", "net.netfilter.nf_conntrack_tcp_timeout_fin_wait", 5, 60, 120, "秒", "FIN_WAIT 状态超时"},
	{"close_wait_timeout", "net.netfilter.nf_conntrack_tcp_timeout_close_wait", 5, 60, 60, "秒", "CLOSE_WAIT 状态超时"},
	{"last_ack_timeout", "net.netfilter.nf_conntrack_tcp_timeout_last_ack", 5, 60, 30, "秒", "LAST_ACK 状态超时"},
	{"time_wait_timeout", "net.netfilter.nf_conntrack_tcp_timeout_time_wait", 5, 60, 120, "秒", "TIME_WAIT 状态超时"},
	{"close_timeout", "net.netfilter.nf_conntrack_tcp_timeout_close", 5, 60, 10, "秒", "CLOSE 状态超时"},
	{"udp_timeout", "net.netfilter.nf_conntrack_udp_timeout", 5, 60, 30, "秒", "单向 UDP 流超时"},
	{"udp_stream_timeout", "net.netfilter.nf_conntrack_udp_timeout_stream", 30, 1800, 180, "秒", "双向 UDP 流超时"},
	{"icmp_timeout", "net.netfilter.nf_conntrack_icmp_timeout", 5, 100, 30, "秒", "ICMP 连接跟踪超时"},
}

// BBRKeys 是开启 BBR 需要同时设置的两个键。
// 只设 tcp_congestion_control=bbr 而不把 qdisc 换成 fq，BBR 的 pacing
// 会退化成 noqueue 下的近似行为，实测吞吐明显低于预期。
var BBRKeys = []struct {
	Key string
	On  string
	Off string
}{
	{"net.core.default_qdisc", "fq", "fq_codel"},
	{"net.ipv4.tcp_congestion_control", "bbr", "cubic"},
}

// KernelParamsPath 是持久化文件。
func KernelParamsPath() string { return "/etc/sysctl.d/91-router-re-kernel.conf" }

// DefaultKernelParams 返回建站默认值（已把 established 从内核默认的 5 天
// 降到 12 小时：家宽/小企业场景下 5 天不回收的连接只会白占 conntrack 表）。
func DefaultKernelParams() KernelParams {
	return KernelParams{
		BBR:                1,
		SynSendTimeout:     30,
		SynRecvTimeout:     30,
		EstablishedTimeout: 43200,
		FinWaitTimeout:     30,
		CloseWaitTimeout:   30,
		LastAckTimeout:     30,
		TimeWaitTimeout:    30,
		CloseTimeout:       10,
		UDPTimeout:         30,
		UDPStreamTimeout:   180,
		ICMPTimeout:        30,
	}
}

// get 按字段名取值。
func (p KernelParams) get(field string) int {
	switch field {
	case "syn_send_timeout":
		return p.SynSendTimeout
	case "syn_recv_timeout":
		return p.SynRecvTimeout
	case "established_timeout":
		return p.EstablishedTimeout
	case "fin_wait_timeout":
		return p.FinWaitTimeout
	case "close_wait_timeout":
		return p.CloseWaitTimeout
	case "last_ack_timeout":
		return p.LastAckTimeout
	case "time_wait_timeout":
		return p.TimeWaitTimeout
	case "close_timeout":
		return p.CloseTimeout
	case "udp_timeout":
		return p.UDPTimeout
	case "udp_stream_timeout":
		return p.UDPStreamTimeout
	case "icmp_timeout":
		return p.ICMPTimeout
	}
	return -1
}

// Set 按字段名赋值，返回是否命中已知字段。
func (p *KernelParams) Set(field string, v int) bool {
	switch field {
	case "syn_send_timeout":
		p.SynSendTimeout = v
	case "syn_recv_timeout":
		p.SynRecvTimeout = v
	case "established_timeout":
		p.EstablishedTimeout = v
	case "fin_wait_timeout":
		p.FinWaitTimeout = v
	case "close_wait_timeout":
		p.CloseWaitTimeout = v
	case "last_ack_timeout":
		p.LastAckTimeout = v
	case "time_wait_timeout":
		p.TimeWaitTimeout = v
	case "close_timeout":
		p.CloseTimeout = v
	case "udp_timeout":
		p.UDPTimeout = v
	case "udp_stream_timeout":
		p.UDPStreamTimeout = v
	case "icmp_timeout":
		p.ICMPTimeout = v
	default:
		return false
	}
	return true
}

// ValidateKernelParams 校验全部参数。
//
// 注意 established 的区间下限是 600：比 10 分钟还短的 established 超时
// 会让长时间空闲的 SSH / 数据库连接被中途掐断，这类"能连上但过一会儿断"
// 的问题极难排查，宁可在保存时挡住。
func ValidateKernelParams(p KernelParams) error {
	if p.BBR != 0 && p.BBR != 1 {
		return fmt.Errorf("bbr 只能是 0 或 1，当前 %d", p.BBR)
	}
	for _, s := range KernelSpecs {
		v := p.get(s.Field)
		if v < s.Min || v > s.Max {
			return fmt.Errorf("%s 必须在 %d-%d 之间（当前 %d）", s.Field, s.Min, s.Max, v)
		}
	}
	return nil
}

// RenderKernelConf 生成 sysctl.d 片段。顺序稳定，便于 diff。
func RenderKernelConf(p KernelParams) string {
	var b strings.Builder
	b.WriteString("# router-re kernel network params\n")
	b.WriteString("# 由 mgmtd 生成，手工修改会在下次保存时被覆盖\n\n")
	if p.BBR == 1 {
		b.WriteString("# BBR 拥塞控制\n")
		for _, k := range BBRKeys {
			fmt.Fprintf(&b, "%s=%s\n", k.Key, k.On)
		}
	} else {
		b.WriteString("# 使用默认拥塞控制\n")
		for _, k := range BBRKeys {
			fmt.Fprintf(&b, "%s=%s\n", k.Key, k.Off)
		}
	}
	b.WriteString("\n# 连接跟踪超时（秒）\n")
	for _, s := range KernelSpecs {
		fmt.Fprintf(&b, "%s=%d\n", s.Key, p.get(s.Field))
	}
	return b.String()
}

// ReadCurrent 读取每个键当前的内核生效值。读不到（例如模块未加载）时
// 返回 0 并在 missing 里列出键名，前端据此提示"该参数在当前内核不可用"，
// 而不是显示一个 0 让人以为已经设成 0。
func ReadCurrent() (KernelParams, []string) {
	p := DefaultKernelParams()
	var missing []string
	for _, s := range KernelSpecs {
		raw, err := os.ReadFile("/proc/sys/" + strings.ReplaceAll(s.Key, ".", "/"))
		if err != nil {
			missing = append(missing, s.Key)
			continue
		}
		v, err := strconv.Atoi(strings.TrimSpace(string(raw)))
		if err != nil {
			missing = append(missing, s.Key)
			continue
		}
		p.Set(s.Field, v)
	}
	// BBR 是否已在生效
	if raw, err := os.ReadFile("/proc/sys/net/ipv4/tcp_congestion_control"); err == nil {
		if strings.TrimSpace(string(raw)) == "bbr" {
			p.BBR = 1
		} else {
			p.BBR = 0
		}
	} else {
		missing = append(missing, "net.ipv4.tcp_congestion_control")
	}
	sort.Strings(missing)
	return p, missing
}

// ParseSysctlConf 解析 sysctl.d 片段回结构体（读取已保存的配置用）。
func ParseSysctlConf(text string) KernelParams {
	p := DefaultKernelParams()
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		kv := strings.SplitN(line, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		val := strings.TrimSpace(kv[1])
		if key == "net.ipv4.tcp_congestion_control" {
			if val == "bbr" {
				p.BBR = 1
			} else {
				p.BBR = 0
			}
			continue
		}
		for _, s := range KernelSpecs {
			if s.Key == key {
				if n, err := strconv.Atoi(val); err == nil {
					p.Set(s.Field, n)
				}
			}
		}
	}
	return p
}

// ParseApplyOutput 解析 `sysctl -p` 的输出。
//
// sysctl -p 遇到个别键不可写（容器里 /proc/sys 部分只读、模块未加载）
// 会以非 0 退出，但其余键**已经生效**。把整体当成失败会误导用户去回滚，
// 而把配置文件删掉更糟——那些生效的值下次开机就没了。
// 所以这里逐行判断：写成 "key = value" 的是成功，sysctl: 开头的是失败。
func ParseApplyOutput(out string) (okKeys []string, failedKeys []string) {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "sysctl:") {
			// sysctl: cannot stat /proc/sys/net/core/default_qdisc: No such file
			// sysctl: permission denied on key 'net.core.default_qdisc'
			field := strings.TrimPrefix(line, "sysctl:")
			field = strings.TrimSpace(field)
			if i := strings.Index(field, " on key '"); i >= 0 {
				field = strings.Trim(field[i+len(" on key '"):], "'")
			} else if i := strings.Index(field, " "); i > 0 {
				field = field[:i]
			}
			failedKeys = append(failedKeys, strings.TrimSpace(field))
			continue
		}
		if i := strings.Index(line, "="); i > 0 {
			okKeys = append(okKeys, strings.TrimSpace(line[:i]))
		}
	}
	return okKeys, failedKeys
}
