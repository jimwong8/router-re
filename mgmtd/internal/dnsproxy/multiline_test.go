package dnsproxy

import (
	"strings"
	"testing"
)

func mlBase() MultiLineConfig {
	return MultiLineConfig{
		Enabled: true,
		Lines: []LineUpstream{
			{Line: "telecom", Interface: "pppoe-wan1", Servers: []string{"202.96.128.86"}, Enabled: true},
			{Line: "unicom", Interface: "pppoe-wan2", Servers: []string{"221.6.4.66", "221.6.4.67"}, Enabled: true},
		},
		DefaultLine: "telecom",
	}
}

func TestValidateMultiLineOK(t *testing.T) {
	if err := ValidateMultiLine(mlBase()); err != nil {
		t.Fatalf("合法配置被拒: %v", err)
	}
}

func TestValidateMultiLineErrors(t *testing.T) {
	c := mlBase()
	c.Lines = nil
	if err := ValidateMultiLine(c); err == nil {
		t.Fatal("启用但无线路应报错")
	}

	c = mlBase()
	c.Lines[1].Line = "telecom"
	if err := ValidateMultiLine(c); err == nil {
		t.Fatal("线路名重复应报错")
	}

	c = mlBase()
	c.Lines[0].Servers = []string{"abc"}
	if err := ValidateMultiLine(c); err == nil {
		t.Fatal("坏上游应报错")
	}

	c = mlBase()
	c.Lines[0].Domains = []string{"bad domain.com"}
	if err := ValidateMultiLine(c); err == nil {
		t.Fatal("含空格的域名应报错")
	}

	c = mlBase()
	c.DefaultLine = "mobile"
	if err := ValidateMultiLine(c); err == nil {
		t.Fatal("默认线路不存在应报错")
	}

	c = mlBase()
	c.Lines = append(c.Lines, LineUpstream{Line: "a"}, LineUpstream{Line: "b"},
		LineUpstream{Line: "c"}, LineUpstream{Line: "d"}, LineUpstream{Line: "e"},
		LineUpstream{Line: "f"}, LineUpstream{Line: "g"})
	if err := ValidateMultiLine(c); err == nil {
		t.Fatal("超过 8 条线路应报错")
	}
}

func TestRenderMultiLineGlobal(t *testing.T) {
	c := MultiLineConfig{Enabled: true, Lines: []LineUpstream{
		{Line: "wan1", Interface: "eth0", Servers: []string{"1.1.1.1"}, Enabled: true},
	}}
	out, err := RenderMultiLine(c)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "server=1.1.1.1@eth0") {
		t.Fatalf("无域名时应输出全局 server 且带接口:\n%s", out)
	}
	if !strings.Contains(out, "# router-re multi-line dns") {
		t.Fatalf("缺少头部注释:\n%s", out)
	}
	if !strings.Contains(out, "# line wan1") {
		t.Fatalf("缺少线路注释:\n%s", out)
	}
}

func TestRenderMultiLineDomains(t *testing.T) {
	c := MultiLineConfig{Enabled: true, Lines: []LineUpstream{
		{Line: "wan1", Servers: []string{"1.1.1.1"}, Domains: []string{"a.com", "*.b.com"}, Enabled: true},
	}}
	out, err := RenderMultiLine(c)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "server=/a.com/1.1.1.1") {
		t.Fatalf("缺少域名上游行:\n%s", out)
	}
	if !strings.Contains(out, "server=/*.b.com/1.1.1.1") {
		t.Fatalf("通配域名应原样输出:\n%s", out)
	}
	if strings.Contains(out, "\nserver=1.1.1.1\n") {
		t.Fatalf("有域名时不应再输出全局 server:\n%s", out)
	}
}

func TestRenderMultiLineDisabled(t *testing.T) {
	out, err := RenderMultiLine(MultiLineConfig{Enabled: false, Lines: mlBase().Lines})
	if err != nil || out != "" {
		t.Fatalf("未启用应返回空串: out=%q err=%v", out, err)
	}
}

func TestRenderMultiLineStable(t *testing.T) {
	c := mlBase()
	a, _ := RenderMultiLine(c)
	b, _ := RenderMultiLine(c)
	if a != b {
		t.Fatal("两次渲染结果应完全相同（顺序稳定）")
	}
}

func TestRenderParseRoundTrip(t *testing.T) {
	c := mlBase()
	c.Lines[0].Domains = []string{"corp.example.com", "intra.example.com"}
	out, err := RenderMultiLine(c)
	if err != nil {
		t.Fatal(err)
	}
	got, perr := ParseMultiLine(out)
	if perr != nil {
		t.Fatalf("解析自己的输出不应出错: %v", perr)
	}
	if len(got.Lines) != len(c.Lines) {
		t.Fatalf("线路数不一致: got %d want %d", len(got.Lines), len(c.Lines))
	}
	for i := range got.Lines {
		if got.Lines[i].Line != c.Lines[i].Line {
			t.Fatalf("第 %d 条线路名不一致: %s vs %s", i, got.Lines[i].Line, c.Lines[i].Line)
		}
		if got.Lines[i].Interface != c.Lines[i].Interface {
			t.Fatalf("第 %d 条接口不一致: %q vs %q", i, got.Lines[i].Interface, c.Lines[i].Interface)
		}
		if len(got.Lines[i].Domains) != len(c.Lines[i].Domains) {
			t.Fatalf("第 %d 条域名数不一致: %d vs %d", i, len(got.Lines[i].Domains), len(c.Lines[i].Domains))
		}
		// 同一上游被多个域名重复引用，解析后应去重成一份
		if len(got.Lines[i].Servers) != len(c.Lines[i].Servers) {
			t.Fatalf("第 %d 条上游数不一致: %d vs %d", i, len(got.Lines[i].Servers), len(c.Lines[i].Servers))
		}
	}
}

func TestParseMultiLineBadLine(t *testing.T) {
	text := "# router-re multi-line dns\n# line wan1\nserver=1.1.1.1\nthis is garbage\n"
	c, err := ParseMultiLine(text)
	if err == nil {
		t.Fatal("非法行应返回错误")
	}
	if len(c.Lines) != 1 {
		t.Fatalf("已解析部分应保留: %d", len(c.Lines))
	}
}
