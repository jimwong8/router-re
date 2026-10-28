package sysctl

import (
	"os"
	"strings"
)

// AcctPath 返回 conntrack 字节统计的 proc 路径。
func AcctPath() string {
	return "/proc/sys/net/netfilter/nf_conntrack_acct"
}

// IsAcctOn 读取当前是否开启 conntrack 字节统计（1=开）。
func IsAcctOn() (bool, error) {
	b, err := os.ReadFile(AcctPath())
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(b)) == "1", nil
}

// SetAcct 写 "1"/"0" 到 AcctPath（立即生效，非持久化）。
func SetAcct(on bool) error {
	v := "0"
	if on {
		v = "1"
	}
	return os.WriteFile(AcctPath(), []byte(v), 0o644)
}

// RenderAcctConf 返回 sysctl.d 持久化片段文本。
func RenderAcctConf(on bool) string {
	v := "0"
	if on {
		v = "1"
	}
	return "net.netfilter.nf_conntrack_acct=" + v + "\n"
}

// PersistAcctConf 把配置写入 sysctl.d 文件（默认 /etc/sysctl.d/90-router-re-conntrack.conf）。
func PersistAcctConf(path string, on bool) error {
	if path == "" {
		path = "/etc/sysctl.d/90-router-re-conntrack.conf"
	}
	return os.WriteFile(path, []byte(RenderAcctConf(on)), 0o644)
}