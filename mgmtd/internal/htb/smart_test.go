package htb

import "testing"

func TestPlanSmart(t *testing.T) {
	cfg, err := PlanSmart(100, "eth0", []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Classes) != 3 {
		t.Fatalf("classes=%d", len(cfg.Classes))
	}
	for _, c := range cfg.Classes {
		if c.RateMbit != 100.0/3.0 {
			t.Fatalf("rate=%v", c.RateMbit)
		}
	}
	_, err = PlanSmart(100, "eth0", []string{})
	if err == nil {
		t.Fatal("expected error for empty terminals")
	}
}
