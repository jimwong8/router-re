package billing

import "fmt"

// Plan is a sellable package: price, validity days and traffic quota.
type Plan struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	PriceYuan  float64 `json:"price_yuan"`
	Days       int     `json:"days"`
	PeriodDays int     `json:"period_days"`
	QuotaMB    int     `json:"quota_mb"`
}

// ValidatePlan checks that a plan is well-formed (unified for both the
// legacy Days-based and the new PeriodDays-based billing flows).
func ValidatePlan(p Plan) error {
	if p.Name == "" {
		return fmt.Errorf("plan name must not be empty")
	}
	if p.PriceYuan < 0 || p.PriceYuan > 1000000 {
		return fmt.Errorf("plan price must be in [0, 1000000]")
	}
	if p.PeriodDays < 0 || p.PeriodDays > 365 {
		return fmt.Errorf("plan period_days must be in [0, 365]")
	}
	if p.PeriodDays > 0 && (p.PeriodDays < 1) {
		return fmt.Errorf("plan period_days must be >= 1")
	}
	// PeriodDays is the new authoritative field; Days is the legacy alias.
	// At least one day-counter (or a quota) must be present, otherwise the
	// plan has no defined validity window.
	if p.PeriodDays == 0 && p.Days == 0 && p.QuotaMB == 0 {
		return fmt.Errorf("plan must define period_days/days > 0 or quota_mb > 0")
	}
	// When PeriodDays is explicitly given it must be in 1..365.
	if p.PeriodDays != 0 && (p.PeriodDays < 1 || p.PeriodDays > 365) {
		return fmt.Errorf("plan period_days must be 1-365, got %d", p.PeriodDays)
	}
	if p.QuotaMB < 0 {
		return fmt.Errorf("plan quota_mb must be >= 0")
	}
	return nil
}

// TotalRevenue sums price * count for each named plan.
// Both empty inputs yield 0.
func TotalRevenue(plans []Plan, counts map[string]int) float64 {
	if len(plans) == 0 || len(counts) == 0 {
		return 0
	}
	total := 0.0
	for _, p := range plans {
		if n, ok := counts[p.Name]; ok && n > 0 {
			total += p.PriceYuan * float64(n)
		}
	}
	return total
}

// RemainingDays returns whole days left before expiry.
// Expired -> 0; never-expiring (expireAt == 0) -> -1.
func RemainingDays(expireAt, now int64) int {
	if expireAt == 0 {
		return -1
	}
	if now >= expireAt {
		return 0
	}
	return int((expireAt - now) / 86400)
}

// DefaultPlans returns the starter plan set used when no plans have been
// configured yet. It is intentionally small: operators replace it through the
// billing API as soon as they know their real pricing.
func DefaultPlans() []Plan {
	return []Plan{
		{Name: "basic", PriceYuan: 9.9, Days: 30, QuotaMB: 1024},
		{Name: "pro", PriceYuan: 29.9, Days: 30, QuotaMB: 4096},
	}
}
