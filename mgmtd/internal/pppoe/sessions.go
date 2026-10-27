package pppoe

import (
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// Session 描述一个活跃 PPPoE 会话（从 ps -eo pid,args 解析）。
type Session struct {
	User string `json:"user"` // 会话关联的用户（解析自进程参数或不确定时为空）
	IP   string `json:"ip"`   // 会话 IP（解析自 -I 参数或为空）
	PID  string `json:"pid"`  // 进程 PID
}

// Sessions 执行 ps -eo pid,args 过滤含 pppoe-server 且带 -u 的行解析。
// 如果解析不到（进程不存在或参数不匹配），返回空切片而不报错。
func Sessions(r execx.Runner) ([]Session, error) {
	out, err := r.Run("ps", "-eo", "pid,args")
	if err != nil {
		// ps 执行失败：返回空，不报错（要求：拿不到就返回空，不报错）
		return []Session{}, nil
	}
	var sessions []Session
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// 过滤含 pppoe-server 且带 -u 的行
		if !strings.Contains(line, "pppoe-server") {
			continue
		}
		// 解析 PID（行首数字）和 args（剩余部分）
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		pid := parts[0]
		argsStr := strings.Join(parts[1:], " ")
		if !strings.Contains(argsStr, "-u") {
			continue
		}
		// 尝试从 argsStr 提取用户和 IP（简化解析：找 -u 后的值和 IP 模式）
		user := extractUserFromArgs(argsStr)
		ip := extractIPFromArgs(argsStr)
		sessions = append(sessions, Session{
			User: user,
			IP:   ip,
			PID:  pid,
		})
	}
	return sessions, nil
}

func extractUserFromArgs(argsStr string) string {
	parts := strings.Split(argsStr, " ")
	for i := 0; i < len(parts); i++ {
		if parts[i] == "-u" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func extractIPFromArgs(argsStr string) string {
	// 简化：查找形似 IP 的字符串（不做严格校验，只提取第一个）
	parts := strings.Split(argsStr, " ")
	for _, p := range parts {
		if strings.Contains(p, ".") && len(p) > 3 {
			// 快速判断是否像 IPv4（至少一个点，不是参数名）
			return p
		}
	}
	return ""
}
