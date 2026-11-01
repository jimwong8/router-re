package threat

import (
	"strings"
	"testing"
)

// hosts.deny 按行解析。注释里的 IP 带换行就能逃出 "# ..." 变成生效规则
// （例如 "ALL: ALL" 直接拒绝所有连接）。
func TestRenderHostsDenyNoInjection(t *testing.T) {
	payloads := []Entry{
		{IP: "1.2.3.4\nALL: ALL"},
		{IP: "1.2.3.4\r\nALL: ALL"},
		{IP: "10.0.0.0/8\nALL: ALL"},
		{IP: "1.2.3.4", Reason: "r\nALL: ALL"},
		{IP: "1.2.3.4", Reason: "r\r\nALL: ALL"},
	}
	for i, e := range payloads {
		out := RenderHostsDeny([]Entry{e})
		for _, line := range strings.Split(out, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			// 任何生效行都必须是 "ALL: <合法IPv4>" 形态
			if !strings.HasPrefix(trimmed, "ALL: 1.2.3.4") {
				t.Errorf("用例 %d 逃逸出非法生效行: %q\n完整输出:\n%s", i, line, out)
			}
		}
	}
}

func TestRenderHostsDenyAcceptsLegit(t *testing.T) {
	out := RenderHostsDeny([]Entry{
		{IP: "1.2.3.4", Reason: "手工封禁"},
		{IP: "5.6.7.8/32", Reason: "情报源"},
		{IP: "10.0.0.0/8", Reason: "CIDR 不支持"},
		{IP: "2001:db8::1", Reason: "IPv6 跳过"},
	})
	if !strings.Contains(out, "ALL: 1.2.3.4 # 手工封禁") {
		t.Errorf("合法单 IP 未渲染:\n%s", out)
	}
	if !strings.Contains(out, "ALL: 5.6.7.8 # 情报源") {
		t.Errorf("/32 未规整为单 IP:\n%s", out)
	}
	if !strings.Contains(out, "CIDR") {
		t.Errorf("CIDR 应留下说明注释:\n%s", out)
	}
}

// Reason 里的 '#' 必须去掉，否则会提前结束注释、让后半段成为规则的一部分。
func TestRenderHostsDenyStripsHashInReason(t *testing.T) {
	out := RenderHostsDeny([]Entry{{IP: "1.2.3.4", Reason: "# ALL: ALL"}})
	if strings.Count(out, "#") < 2 {
		t.Fatalf("输出异常:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "ALL: ALL") {
			t.Fatalf("Reason 里的 # 逃逸成了新规则: %q", line)
		}
	}
}
