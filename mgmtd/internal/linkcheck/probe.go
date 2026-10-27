package linkcheck

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
)

// Target 描述一个链路探测目标。
type Target struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Iface            string `json:"iface"`
	Kind             string `json:"kind"` // icmp / dns / http
	Host             string `json:"host"`
	URL              string `json:"url"`
	Interval         int    `json:"interval"`
	Timeout          int    `json:"timeout"`
	FailThreshold    int    `json:"fail_threshold"`
	RecoverThreshold int    `json:"recover_threshold"`
	Enabled          bool   `json:"enabled"`
}

// State 描述链路状态（迟滞）。
type State struct {
	Up         bool   `json:"up"`
	ConsecFail int    `json:"consec_fail"`
	ConsecOK   int    `json:"consec_ok"`
	LastChange int64  `json:"last_change"`
	LastError  string `json:"last_error"`
	Reason     string `json:"reason"`
}

var ifaceRe = regexp.MustCompile(`^[a-zA-Z0-9._@-]+$`)

// Validate 校验探测目标。
func Validate(t Target) error {
	if strings.TrimSpace(t.Iface) == "" {
		return fmt.Errorf("iface required")
	}
	if !ifaceRe.MatchString(t.Iface) {
		return fmt.Errorf("iface %q contains invalid characters (only [a-zA-Z0-9._@-] allowed)", t.Iface)
	}
	switch t.Kind {
	case "icmp", "dns", "http":
	default:
		return fmt.Errorf("kind must be icmp/dns/http, got %q", t.Kind)
	}
	if t.Kind == "icmp" || t.Kind == "dns" {
		if strings.TrimSpace(t.Host) == "" {
			return fmt.Errorf("host required for %s", t.Kind)
		}
		// 合法 IP 或域名
		if net.ParseIP(t.Host) == nil {
			// 域名：至少含一个点或字母，且不含空格
			if strings.ContainsAny(t.Host, " \t/\\") {
				return fmt.Errorf("host %q is not a valid IP or domain", t.Host)
			}
		}
	}
	if t.Kind == "http" {
		if !strings.HasPrefix(t.URL, "http://") && !strings.HasPrefix(t.URL, "https://") {
			return fmt.Errorf("url must start with http:// or https://, got %q", t.URL)
		}
	}
	if t.Interval < 5 || t.Interval > 300 {
		return fmt.Errorf("interval must be 5-300, got %d", t.Interval)
	}
	if t.Timeout < 1 || t.Timeout > 30 {
		return fmt.Errorf("timeout must be 1-30, got %d", t.Timeout)
	}
	if t.FailThreshold < 1 || t.FailThreshold > 10 {
		return fmt.Errorf("fail_threshold must be 1-10, got %d", t.FailThreshold)
	}
	if t.RecoverThreshold < 1 || t.RecoverThreshold > 10 {
		return fmt.Errorf("recover_threshold must be 1-10, got %d", t.RecoverThreshold)
	}
	return nil
}

// Step 迟滞状态机：连续 FailThreshold 次失败才 Down；连续 RecoverThreshold 次成功才 Up。
// 返回 (新状态, 是否刚改变)。
func Step(st State, ok bool, now int64, t Target) (State, bool) {
	nst := st
	changed := false
	if ok {
		nst.ConsecOK++
		nst.ConsecFail = 0
		nst.LastError = ""
		if !nst.Up && nst.ConsecOK >= t.RecoverThreshold {
			nst.Up = true
			nst.LastChange = now
			nst.Reason = "recovered"
			changed = true
		}
	} else {
		nst.ConsecFail++
		nst.ConsecOK = 0
		nst.LastError = "probe failed"
		if nst.Up && nst.ConsecFail >= t.FailThreshold {
			nst.Up = false
			nst.LastChange = now
			nst.Reason = "down"
			changed = true
		}
	}
	// 初始状态：Up 默认 false，但第一次探测... 保持语义：Up 零值 false。
	// 若从未设置过且首次成功达到 recover threshold 才 up（已处理）。
	return nst, changed
}

// ProbeArgs 返回参数（不含命令名）。
func ProbeArgs(t Target) ([]string, error) {
	switch t.Kind {
	case "icmp":
		return []string{"-c", "1", "-W", strconv.Itoa(t.Timeout), t.Host}, nil
	case "dns":
		return []string{t.Host}, nil
	case "http":
		return []string{"-s", "-o", "/dev/null", "-w", "%{http_code}", "-m", strconv.Itoa(t.Timeout), t.URL}, nil
	default:
		return nil, fmt.Errorf("unknown kind %q", t.Kind)
	}
}

// ParseProbeOK 解析探测输出。
func ParseProbeOK(t Target, out string, err error) (bool, string) {
	switch t.Kind {
	case "icmp":
		if strings.Contains(out, "1 received") || strings.Contains(out, "bytes from") {
			return true, "icmp ok"
		}
		return false, "icmp failed: " + strings.TrimSpace(out)
	case "http":
		code := strings.TrimSpace(out)
		n, perr := strconv.Atoi(code)
		if perr != nil {
			if err != nil {
				return false, fmt.Sprintf("http error: %v", err)
			}
			return false, "http: cannot parse status code " + code
		}
		// 200-399 算链路通：4xx/5xx 说明服务器有问题但链路是通的，按链路检测语义算通。
		if n >= 200 && n <= 399 {
			return true, fmt.Sprintf("http %d", n)
		}
		// 链路层：只要拿到 HTTP 响应（包括 4xx/5xx），链路是通的
		if n >= 400 && n <= 599 {
			return true, fmt.Sprintf("http %d (server error but link up)", n)
		}
		return false, fmt.Sprintf("http %d", n)
	case "dns":
		if err == nil && strings.TrimSpace(out) != "" {
			return true, "dns ok"
		}
		return false, "dns failed"
	default:
		return false, "unknown kind"
	}
}
