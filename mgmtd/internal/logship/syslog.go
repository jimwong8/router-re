package logship

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// SyslogTarget is a remote syslog destination.
type SyslogTarget struct {
	Name     string `json:"name"`
	Address  string `json:"address"`
	Protocol string `json:"protocol"`
	Format   string `json:"format"`
	Facility string `json:"facility"`
	Enabled  bool   `json:"enabled"`
}

// ValidateTarget checks a SyslogTarget.
func ValidateTarget(t SyslogTarget) error {
	if strings.TrimSpace(t.Name) == "" {
		return fmt.Errorf("name required")
	}
	host, portStr, err := net.SplitHostPort(t.Address)
	if err != nil {
		return fmt.Errorf("address must be host:port, got %q: %w", t.Address, err)
	}
	if host == "" {
		return fmt.Errorf("address host must not be empty: %q", t.Address)
	}
	port, perr := strconv.Atoi(portStr)
	if perr != nil {
		return fmt.Errorf("port must be numeric, got %q", portStr)
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("port must be in [1,65535], got %d", port)
	}
	switch t.Protocol {
	case "udp", "tcp":
	default:
		return fmt.Errorf("protocol must be udp or tcp, got %q", t.Protocol)
	}
	switch t.Format {
	case "rfc5424", "rfc3164":
	default:
		return fmt.Errorf("format must be rfc5424 or rfc3164, got %q", t.Format)
	}
	switch t.Facility {
	case "local0", "local1", "local2", "local3", "local4", "local5", "local6", "local7":
	default:
		return fmt.Errorf("facility must be local0-local7, got %q", t.Facility)
	}
	return nil
}

// RenderRsyslog emits an rsyslog action line. @ is UDP, @@ is TCP — swapping
// them silently drops logs and is hard to diagnose, hence the comment.
func RenderRsyslog(t SyslogTarget) string {
	host, port, _ := net.SplitHostPort(t.Address)
	// 渲染层兜底：Address 里一个换行就能追加 rsyslog 动作模块
	// （action(type="omprog" binary="/bin/sh")）—— 等于远程命令执行。
	addr := oneLine(host) + ":" + oneLine(port)
	var b strings.Builder
	if t.Facility != "" && strings.HasPrefix(t.Facility, "local") {
		fmt.Fprintf(&b, "local%s.* ", t.Facility[len("local"):])
	}
	if t.Protocol == "tcp" {
		b.WriteString("@@" + addr + ";RSYSLOG_SyslogProtocol23Format")
	} else {
		b.WriteString("@" + addr)
	}
	return b.String()
}

// TestTarget sends one test message to the target. If logger does not
// support -T (TCP), it falls back to nc -zv which only probes port
// reachability — it does NOT claim a log was actually delivered.
func TestTarget(r execx.Runner, t SyslogTarget) (bool, string) {
	host, port, err := net.SplitHostPort(t.Address)
	if err != nil {
		return false, fmt.Sprintf("address parse: %v", err)
	}
	msg := fmt.Sprintf("mgmtd logship test from %s", t.Name)
	if t.Protocol == "udp" {
		out, err := r.Run("logger", "-n", host, "-P", port, "-d", msg)
		if err != nil {
			return false, fmt.Sprintf("logger udp: %v", err)
		}
		return true, "udp test message sent: " + strings.TrimSpace(out)
	}
	// tcp: try logger -T first
	_, err = r.Run("logger", "-n", host, "-P", port, "-T", msg)
	if err == nil {
		return true, "tcp test message sent via logger -T"
	}
	// logger does not support -T (or failed) — probe port only, do not pretend
	if strings.Contains(err.Error(), "invalid option") || strings.Contains(err.Error(), "unrecognized") || strings.Contains(err.Error(), "unknown option") {
		out, perr := r.Run("nc", "-zv", "-w", "2", host, port)
		if perr != nil {
			return false, fmt.Sprintf("logger -T unsupported; nc port probe failed: %v", perr)
		}
		return true, "logger -T unsupported; nc port probe ok: " + strings.TrimSpace(out)
	}
	return false, fmt.Sprintf("logger -T failed: %v", err)
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
