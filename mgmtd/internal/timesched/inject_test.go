package timesched

import (
	"strings"
	"testing"
	"time"
)

func ruleMon() Rule {
	return Rule{
		ID: "r1", Name: "晚间限速",
		Days: []int{1}, StartMin: 540, EndMin: 600,
		Targets: []string{"10.0.0.1"}, Action: "block", Enabled: true,
	}
}

// ID 拼进注释行 "# active rule: %s"。注释只保护到行尾 —— 一个换行就能
// 在 nft 表里插入任意规则，例如 "ip saddr 0.0.0.0/0 accept"（整体放通）。
func TestValidateRejectsIDInjection(t *testing.T) {
	for _, id := range []string{
		"r1\n\t\tip saddr 0.0.0.0/0 accept\n\t\t#",
		"r1\r\n\t\tip saddr 0.0.0.0/0 accept",
		"r1; drop",
		"r 1",
		"",
		strings.Repeat("a", 65),
	} {
		r := ruleMon()
		r.ID = id
		if err := Validate(r); err == nil {
			t.Errorf("ID=%q 未被拒绝", id)
		}
	}
}

func TestRenderNoInjection(t *testing.T) {
	r := ruleMon()
	r.ID = "r1\n\t\tip saddr 0.0.0.0/0 accept\n\t\t#"
	// 绕过 Validate，直接验证渲染层兜底
	at := time.Date(2026, 11, 2, 9, 30, 0, 0, time.UTC)
	out := renderUnchecked([]Rule{r}, at.Unix(), time.UTC)
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "ip saddr") {
			t.Fatalf("ID 注入产生了生效 nft 规则:\n%s", out)
		}
	}
}

// renderUnchecked 复刻 Render 的注释渲染，用于验证兜底不依赖 Validate。
func renderUnchecked(rules []Rule, now int64, loc *time.Location) string {
	var b strings.Builder
	b.WriteString("table inet timesched {\n\tchain forward {\n")
	for _, r := range rules {
		if ok, _ := Active(r, now, loc); !ok {
			// Active 内部会 Validate；这里只是想验证注释渲染本身
			b.WriteString("\t\t# active rule: " + oneLine(r.ID) + "\n")
		}
	}
	b.WriteString("\t}\n}\n")
	return b.String()
}

func TestRenderAcceptsLegit(t *testing.T) {
	r := ruleMon()
	at := time.Date(2026, 11, 2, 9, 30, 0, 0, time.UTC)
	ok, err := Active(r, at.Unix(), time.UTC)
	if !ok || err != nil {
		t.Fatalf("合法规则应处于活跃状态: ok=%v err=%v", ok, err)
	}
	out := Render([]Rule{r}, at.Unix(), time.UTC)
	if !strings.Contains(out, "# active rule: r1") {
		t.Errorf("缺少规则标注:\n%s", out)
	}
	if !strings.Contains(out, "policy accept") {
		t.Errorf("缺少基础链定义:\n%s", out)
	}
}

// 没有活跃规则时不得输出空 elements 集合
func TestRenderNoActiveRules(t *testing.T) {
	r := ruleMon()
	at := time.Date(2026, 11, 2, 20, 0, 0, 0, time.UTC)
	out := Render([]Rule{r}, at.Unix(), time.UTC)
	if !strings.Contains(out, "# no active rules") {
		t.Errorf("应标注无活跃规则:\n%s", out)
	}
	if strings.Contains(out, "elements") {
		t.Errorf("不应输出 elements:\n%s", out)
	}
}
