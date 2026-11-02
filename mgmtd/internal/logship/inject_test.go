package logship

import (
	"strings"
	"testing"
)

// rsyslog 动作行以 "localX.* @host:port" 的形式写进配置。
// Address 里一个换行就能追加任意动作模块，例如
//   action(type="omprog" binary="/bin/sh")  → 远程命令执行。
func TestRenderRsyslogNoInjection(t *testing.T) {
	cases := []string{
		"10.0.0.1:514\naction(type=\"omprog\" binary=\"/bin/sh\")",
		"10.0.0.1:514\r\naction(type=\"omprog\")",
		"10.0.0.1:514;RSYSLOG_TraditionalForwardFormat",
	}
	for _, addr := range cases {
		out := RenderRsyslog(SyslogTarget{
			Name: "s", Address: addr, Protocol: "udp",
			Format: "rfc5424", Facility: "local0",
		})
		for _, line := range strings.Split(out, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "action(") {
				t.Fatalf("address=%q 注入出了独立 action 行:\n%s", addr, out)
			}
		}
	}
}

func TestValidateTargetRejectsInjection(t *testing.T) {
	cases := []SyslogTarget{
		{Name: "s", Address: "10.0.0.1:514\naction(type=\"omprog\")", Protocol: "udp", Format: "rfc5424", Facility: "local0"},
		{Name: "s", Address: "10.0.0.1", Protocol: "udp", Format: "rfc5424", Facility: "local0"},
		{Name: "s", Address: "10.0.0.1:99999", Protocol: "udp", Format: "rfc5424", Facility: "local0"},
		{Name: "s", Address: "10.0.0.1:514", Protocol: "sctp", Format: "rfc5424", Facility: "local0"},
		{Name: "s", Address: "10.0.0.1:514", Protocol: "udp", Format: "raw", Facility: "local0"},
		{Name: "s", Address: "10.0.0.1:514", Protocol: "udp", Format: "rfc5424", Facility: "kern"},
		{Name: "", Address: "10.0.0.1:514", Protocol: "udp", Format: "rfc5424", Facility: "local0"},
	}
	for i, c := range cases {
		if err := ValidateTarget(c); err == nil {
			t.Errorf("用例 %d 未被拒绝: %+v", i, c)
		}
	}
}

func TestRenderRsyslogAcceptsLegit(t *testing.T) {
	udp := RenderRsyslog(SyslogTarget{Name: "s", Address: "10.0.0.1:514",
		Protocol: "udp", Format: "rfc3164", Facility: "local0"})
	if !strings.Contains(udp, "@10.0.0.1:514") || strings.Contains(udp, "@@") {
		t.Errorf("UDP 应为单个 @，实际 %q", udp)
	}
	tcp := RenderRsyslog(SyslogTarget{Name: "s", Address: "10.0.0.1:514",
		Protocol: "tcp", Format: "rfc5424", Facility: "local3"})
	if !strings.Contains(tcp, "@@10.0.0.1:514") {
		t.Errorf("TCP 应为 @@，实际 %q", tcp)
	}
	if !strings.HasPrefix(tcp, "local3.*") {
		t.Errorf("facility 前缀缺失，实际 %q", tcp)
	}
}
