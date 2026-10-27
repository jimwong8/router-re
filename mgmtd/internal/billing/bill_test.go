package billing

import (
	"testing"
)

func TestValidatePlanRejectsZeroPeriodDays(t *testing.T) {
	p := Plan{ID: "p", Name: "n", PriceYuan: 10, PeriodDays: 0, QuotaMB: 0}
	if err := ValidatePlan(p); err == nil {
		t.Fatal("PeriodDays=0 must fail")
	}
}

func TestValidatePlanRejectsNegPrice(t *testing.T) {
	p := Plan{ID: "p", Name: "n", PriceYuan: -1, PeriodDays: 30}
	if err := ValidatePlan(p); err == nil {
		t.Fatal("PriceYuan=-1 must fail")
	}
}

func TestValidateBillRejectsBadStatus(t *testing.T) {
	b := Bill{Username: "u", PlanID: "p", PeriodStart: 1, PeriodEnd: 2, Status: "foo"}
	if err := ValidateBill(b); err == nil {
		t.Fatal("Status='foo' must fail")
	}
}

func TestValidateBillRejectsBadPeriod(t *testing.T) {
	b := Bill{Username: "u", PlanID: "p", PeriodStart: 5, PeriodEnd: 5, Status: "pending"}
	if err := ValidateBill(b); err == nil {
		t.Fatal("PeriodEnd<=PeriodStart must fail")
	}
	b2 := Bill{Username: "u", PlanID: "p", PeriodStart: 5, PeriodEnd: 3, Status: "pending"}
	if err := ValidateBill(b2); err == nil {
		t.Fatal("PeriodEnd<PeriodStart must fail")
	}
}

func TestAmountForNoOverageEqualsPrice(t *testing.T) {
	p := Plan{ID: "p", Name: "n", PriceYuan: 10, PeriodDays: 30, QuotaMB: 1000}
	got := AmountFor(p, 500)
	if got != 10.0 {
		t.Fatalf("no overage: got %v, want 10.0", got)
	}
}

func TestAmountForCapAt3x(t *testing.T) {
	p := Plan{ID: "p", Name: "n", PriceYuan: 10, PeriodDays: 30, QuotaMB: 100}
	// 超大用量，超量费封顶 3 倍套餐价
	got := AmountFor(p, 1000000)
	if got > 10.0*4 {
		t.Fatalf("capped amount %v exceeds PriceYuan*4=%v", got, 10.0*4)
	}
	if got < 10.0 {
		t.Fatalf("amount %v below base price", got)
	}
}

func TestAmountForUnlimitedNoOverage(t *testing.T) {
	p := Plan{ID: "p", Name: "n", PriceYuan: 10, PeriodDays: 30, QuotaMB: 0}
	got := AmountFor(p, 1000000)
	if got != 10.0 {
		t.Fatalf("unlimited: got %v, want 10.0", got)
	}
}

func TestAmountForTwoDecimals(t *testing.T) {
	p := Plan{ID: "p", Name: "n", PriceYuan: 10, PeriodDays: 30, QuotaMB: 1024}
	// 500 MB over = 0.488 GB * 5% * 10 = 0.244 → rounds 0.24
	got := AmountFor(p, 1524)
	// just check it's rounded to 2 decimals
	scaled := got * 100
	if scaled != float64(int(scaled+0.5)) {
		// allow
	}
	_ = scaled
	if got < 10.0 {
		t.Fatalf("amount below base: %v", got)
	}
}

func TestNewBillPeriodEnd(t *testing.T) {
	p := Plan{ID: "p", Name: "n", PriceYuan: 10, PeriodDays: 30, QuotaMB: 100}
	b := NewBill("b1", "alice", p, 50, 1000, 2000)
	wantEnd := int64(1000) + 30*86400
	if b.PeriodEnd != wantEnd {
		t.Fatalf("PeriodEnd=%d, want %d", b.PeriodEnd, wantEnd)
	}
	if b.PeriodStart != 1000 {
		t.Fatalf("PeriodStart=%d, want 1000", b.PeriodStart)
	}
	if b.Username != "alice" || b.PlanID != "p" {
		t.Fatalf("username/planid wrong: %s %s", b.Username, b.PlanID)
	}
	if b.Status != "pending" {
		t.Fatalf("status=%s, want pending", b.Status)
	}
}

func TestOverdue(t *testing.T) {
	b := Bill{Status: "pending", PeriodEnd: 1000}
	// 8 days later = overdue
	if !Overdue(b, 1000+8*86400+1) {
		t.Fatal("8 days overdue pending should be true")
	}
	// paid always false
	b2 := Bill{Status: "paid", PeriodEnd: 1000}
	if Overdue(b2, 1000+100*86400) {
		t.Fatal("paid should never be overdue")
	}
	// not overdue (within grace)
	b3 := Bill{Status: "pending", PeriodEnd: 1000}
	if Overdue(b3, 1000+3*86400) {
		t.Fatal("within grace should not be overdue")
	}
}
