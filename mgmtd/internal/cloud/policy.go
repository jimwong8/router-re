package cloud

import (
	"strings"
)

// 设备侧与云端共用的权限策略。默认拒绝：只有下面列出的读接口和白名单动作能穿过隧道。
//
// 为什么要设备侧也校验一遍：云端一旦被拿下（或控制台账号被盗），攻击者能通过隧道
// 打到设备内部的 127.0.0.1:8080。设备侧再校验一次，等于把爆炸半径限制在"只读"。
// 爱快那套云管没有设备侧 ACL——云端能打到设备的整个 Web API，这是它最大的结构性缺陷。

// allowedReadPrefixes 是允许远端读取的路径前缀（仅 GET）。
var allowedReadPrefixes = []string{
	"/",
	"/assets",
	"/api/v1/health",
	"/api/v1/system/info",
	"/api/v1/system/disks",
	"/api/v1/system/services",
	"/api/v1/system/time",
	"/api/v1/system/processes",
	"/api/v1/wan/status",
	"/api/v1/wan/links",
	"/api/v1/interfaces",
	"/api/v1/interfaces/config",
	"/api/v1/dns/config",
	"/api/v1/qos/limits",
	"/api/v1/tailscale/config",
	"/api/v1/tailscale/status",
	"/api/v1/easytier/config",
	"/api/v1/easytier/status",
	"/api/v1/vpn/servers",
	"/api/v1/firewall/rules",
	"/api/v1/conntrack",
	"/api/v1/dhcp/leases",
	"/api/v1/stats/current",
	"/api/v1/stats/interfaces",
	"/api/v1/clients",
	"/api/v1/threat/list",
	"/api/v1/threat/stats",
	"/api/v1/l7/rules",
	"/api/v1/l7/stats",
	"/api/v1/dpi/apps",
	"/api/v1/dpi/flow-stats",
	"/api/v1/dpi/tracker-stats",
	"/api/v1/alerts",
	"/api/v1/nat/rules",
	"/api/v1/route/table",
	"/api/v1/wireless/clients",
	"/api/v1/behavior/stats",
	"/api/v1/appid/stats",
}

// allowedWrites 精确匹配 "METHOD PATH"，且必须带 confirm（控制台二次确认）。
// 刻意保持极小：能从远端"写"的东西越少越好，复杂变更走 SSH/本地控制台。
var allowedWrites = map[string]bool{
	"POST /api/v1/system/reboot":           true,
	"POST /api/v1/system/services/restart": true,
	"POST /api/v1/dns/cache/clear":         true,
	"POST /api/v1/system/upgrade":          true,
}

// PathAllowed 判断某次调用是否在白名单内。
func PathAllowed(method, path string) bool {
	method = strings.ToUpper(strings.TrimSpace(method))
	path = strings.TrimSpace(path)
	if path == "" || !strings.HasPrefix(path, "/") {
		return false
	}
	// 禁止路径穿越与查询串注入（白名单只匹配路径部分）。
	if strings.Contains(path, "..") || strings.ContainsAny(path, "\r\n\x00 ") {
		return false
	}
	switch method {
	case "GET":
		for _, p := range allowedReadPrefixes {
			if path == p || strings.HasPrefix(path, p+"/") || strings.HasPrefix(path, p+"?") {
				return true
			}
		}
		return false
	case "POST":
		return allowedWrites[method+" "+path]
	default:
		return false
	}
}

// NeedConfirm 判断该动作是否属于"写"，需要二次确认。
func NeedConfirm(method, path string) bool {
	return strings.ToUpper(method) != "GET" && PathAllowed(method, path)
}

// AllowedList 供控制台展示能力边界。
func AllowedList() (reads []string, writes []string) {
	reads = append(reads, allowedReadPrefixes...)
	for k := range allowedWrites {
		writes = append(writes, k)
	}
	return
}
