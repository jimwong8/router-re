package htb

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jimwong/router-re/mgmtd/internal/dpi"
)

func TestLimitRulesToHTB(t *testing.T) {
	rules := []dpi.MarkRule{
		{App: "netflix", Action: "limit", RateMbit: 10},
		{App: "youtube", Action: "limit", RateMbit: 20},
		{App: "other", Action: "mark", RateMbit: 0},
	}
	cfg := LimitRulesToHTB(rules, "eth0", 100)
	if len(cfg.Classes) != 2 {
		t.Fatalf("expected 2 classes, got %d", len(cfg.Classes))
	}
	if cfg.Classes[0].Name != "netflix" || cfg.Classes[0].RateMbit != 10 {
		t.Fatalf("unexpected class0: %+v", cfg.Classes[0])
	}
	if cfg.Classes[1].Name != "youtube" || cfg.Classes[1].RateMbit != 20 {
		t.Fatalf("unexpected class1: %+v", cfg.Classes[1])
	}
	// 每个 limit 类必须带 fwmark，并在脚本里生成 fw 过滤器；否则类存在但流量
	// 永远命中不到，等于配了个假限速。
	for i, cl := range cfg.Classes {
		if cl.MatchMark <= 0 {
			t.Fatalf("class %d (%s) 缺少 MatchMark", i, cl.Name)
		}
	}
	script := RenderScript(cfg)
	for _, cl := range cfg.Classes {
		need := fmt.Sprintf("fwmark %d", cl.MatchMark)
		if !strings.Contains(script, need) {
			t.Fatalf("脚本缺少 %q 的 fw 过滤器:\n%s", need, script)
		}
	}
}
