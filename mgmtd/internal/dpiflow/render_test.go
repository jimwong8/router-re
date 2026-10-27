package dpiflow

import (
	"strings"
	"testing"
)

func TestRenderNftMarkRules_noFlush(t *testing.T) {
	out := RenderNftMarkRules(map[string][]int{"http": {80}, "mysql": {3306}})
	if strings.Contains(out, "flush ruleset") {
		t.Fatalf("must not contain flush ruleset:\n%s", out)
	}
}

func TestRenderNftMarkRules_noPolicyDrop(t *testing.T) {
	out := RenderNftMarkRules(map[string][]int{"ssh": {22}})
	if strings.Contains(out, "policy drop") || strings.Contains(out, "drop") {
		t.Fatalf("must not contain policy drop / drop:\n%s", out)
	}
	if !strings.Contains(out, "policy accept") {
		t.Fatalf("must have policy accept:\n%s", out)
	}
}

func TestRenderNftMarkRules_marksPorts(t *testing.T) {
	out := RenderNftMarkRules(map[string][]int{
		"http":  {80, 8080},
		"mysql": {3306},
	})
	// 多行 chain 声明
	if !strings.Contains(out, "chain classify {\n") {
		t.Fatalf("chain must be declared multiline:\n%s", out)
	}
	if !strings.Contains(out, "table inet router_dpi {\n") {
		t.Fatalf("missing table inet router_dpi:\n%s", out)
	}
	if !strings.Contains(out, "tcp dport { 80, 8080 } meta mark set 0x20") {
		t.Fatalf("missing http mark 0x20:\n%s", out)
	}
	if !strings.Contains(out, "tcp dport { 3306 } meta mark set 0x21") {
		t.Fatalf("missing mysql mark 0x21:\n%s", out)
	}
}

func TestRenderNftMarkRules_sorted(t *testing.T) {
	// 应用名不直接出现在输出（只出 mark/端口），用 mark 顺序验证排序：
	// aapp(10) → 0x20 在前，zapp(90) → 0x21 在后。
	out := RenderNftMarkRules(map[string][]int{"zapp": {90}, "aapp": {10}})
	i20 := strings.Index(out, "{ 10 } meta mark set 0x20")
	i21 := strings.Index(out, "{ 90 } meta mark set 0x21")
	if i20 < 0 || i21 < 0 || i20 > i21 {
		t.Fatalf("mark 0x20 (aapp) should come before 0x21 (zapp):\n%s", out)
	}
}

func TestRenderNfqueueRule_bypass(t *testing.T) {
	out := RenderNfqueueRule("classify", 7)
	if !strings.Contains(out, "queue num 7 bypass") {
		t.Fatalf("NFQUEUE must have bypass (else all traffic stalls when userspace dies):\n%s", out)
	}
	if strings.Contains(out, "flush ruleset") {
		t.Fatalf("must not flush ruleset:\n%s", out)
	}
	if !strings.Contains(out, "policy accept") {
		t.Fatalf("nfq chain must be accept (not drop):\n%s", out)
	}
}

func TestRenderNfqueueRule_defaultChain(t *testing.T) {
	out := RenderNfqueueRule("", 3)
	if !strings.Contains(out, "chain classify {") {
		t.Fatalf("default chain should be classify:\n%s", out)
	}
	if !strings.Contains(out, "queue num 3 bypass") {
		t.Fatalf("missing bypass:\n%s", out)
	}
}
