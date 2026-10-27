package porttrigger

import "strings"

// ALGConfig 表示要加载哪些应用层网关（ALG）helper 内核模块。
type ALGConfig struct {
	FTP  bool `json:"ftp"`
	SIP  bool `json:"sip"`
	H323 bool `json:"h323"`
	TFTP bool `json:"tftp"`
	IRC  bool `json:"irc"`
	PPTP bool `json:"pptp"`
}

// RenderALG 生成加载 nf_conntrack_* helper 模块的 modprobe 脚本。
//
// ⚠ 安全提示：PPTP（nf_conntrack_pptp）与 SIP（nf_conntrack_sip）的 helper
// 有已知安全风险——它们会改写/放行穿越 NAT 的对应协议流量，可能被用来穿透
// 内网做探测。启用这两者时，脚本注释里必须写清风险，绝不静默启用。
//
// 全部 false 时返回空字符串（不生成任何加载脚本）。
func RenderALG(c ALGConfig) string {
	var b strings.Builder
	if c.FTP {
		b.WriteString("modprobe nf_conntrack_ftp\n")
	}
	if c.H323 {
		b.WriteString("modprobe nf_conntrack_h323\n")
	}
	if c.TFTP {
		b.WriteString("modprobe nf_conntrack_tftp\n")
	}
	if c.IRC {
		b.WriteString("modprobe nf_conntrack_irc\n")
	}
	if c.PPTP {
		b.WriteString("# WARNING: nf_conntrack_pptp 帮助器有已知安全风险，可被用于穿透 NAT 做内网探测。\n")
		b.WriteString("modprobe nf_conntrack_pptp\n")
	}
	if c.SIP {
		b.WriteString("# WARNING: nf_conntrack_sip 帮助器有已知安全风险，可被用于穿透 NAT 做内网探测。\n")
		b.WriteString("modprobe nf_conntrack_sip\n")
	}
	return b.String()
}

// AlgsEnabled 返回是否启用了至少一个 ALG（供上层判断需不需要加载脚本）。
func AlgsEnabled(c ALGConfig) bool {
	return c.FTP || c.SIP || c.H323 || c.TFTP || c.IRC || c.PPTP
}
