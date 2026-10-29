package htb

import (
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
}
