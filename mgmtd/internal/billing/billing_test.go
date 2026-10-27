package billing

import (
	"math"
	"testing"
)

func TestValidatePlan(t *testing.T) {
	cases := []struct {
		name string
		p    Plan
		want string
	}{
		{"ok days", Plan{Name: "basic", PriceYuan: 10, Days: 30}, ""},
		{"ok quota", Plan{Name: "basic", PriceYuan: 10, QuotaMB: 1024}, ""},
		{"empty name", Plan{Name: "", PriceYuan: 10, Days: 30}, "name"},
		{"neg price", Plan{Name: "basic", PriceYuan: -1, Days: 30}, "price"},
		{"no days no quota", Plan{Name: "basic", PriceYuan: 10}, "days"},
		{"zero price ok", Plan{Name: "free", PriceYuan: 0, Days: 7}, ""},
	}
	for _, c := range cases {
		err := ValidatePlan(c.p)
		if c.want == "" {
			if err != nil {
				t.Errorf("%s: unexpected error %v", c.name, err)
			}
			continue
		}
		if err == nil {
			t.Errorf("%s: want error, got nil", c.name)
		}
	}
}

func TestTotalRevenue(t *testing.T) {
	plans := []Plan{
		{Name: "basic", PriceYuan: 9.9, Days: 30},
		{Name: "pro", PriceYuan: 29.9, Days: 30},
	}
	counts := map[string]int{"basic": 3, "pro": 2}
	got := TotalRevenue(plans, counts)
	want := 9.9*3 + 29.9*2
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("TotalRevenue = %v, want %v", got, want)
	}
	// empty inputs
	if TotalRevenue(nil, counts) != 0 {
		t.Error("nil plans should be 0")
	}
	if TotalRevenue(plans, nil) != 0 {
		t.Error("nil counts should be 0")
	}
	if TotalRevenue(nil, nil) != 0 {
		t.Error("both nil should be 0")
	}
	// unknown plan name ignored
	if TotalRevenue(plans, map[string]int{"ghost": 5}) != 0 {
		t.Error("unknown plan name should contribute 0")
	}
	// floating point accumulation
	p2 := []Plan{{Name: "a", PriceYuan: 0.1, Days: 1}, {Name: "b", PriceYuan: 0.2, Days: 1}}
	got2 := TotalRevenue(p2, map[string]int{"a": 3, "b": 3})
	if math.Abs(got2-0.9) > 1e-9 {
		t.Errorf("float accumulation = %v, want 0.9", got2)
	}
}

func TestRemainingDays(t *testing.T) {
	now := int64(1_000_000)
	// expired
	if got := RemainingDays(now-100, now); got != 0 {
		t.Errorf("expired: got %d, want 0", got)
	}
	// never expires
	if got := RemainingDays(0, now); got != -1 {
		t.Errorf("never: got %d, want -1", got)
	}
	// 30 days left
	if got := RemainingDays(now+30*86400, now); got != 30 {
		t.Errorf("30d: got %d, want 30", got)
	}
	// partial day rounds down
	if got := RemainingDays(now+86400+3600, now); got != 1 {
		t.Errorf("partial: got %d, want 1", got)
	}
	// exactly now -> expired
	if got := RemainingDays(now, now); got != 0 {
		t.Errorf("exact now: got %d, want 0", got)
	}
}
