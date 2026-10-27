package billing

import "fmt"

// Plan is a sellable package: price, validity days and traffic quota.
type Plan struct {
	Name      string  `json:"name"`
	PriceYuan float64 `json:"price_yuan"`
	Days      int     `json:"days"`
	QuotaMB   int     `json:"quota_mb"`
}

// ValidatePlan checks that a plan has a name and at least one of days/quota.
func ValidatePlan(p Plan) error {
	if p.Name == "" {
		return fmt.Errorf("plan name must not be empty")
	}
	if p.PriceYuan < 0 {
		return fmt.Errorf("plan price must be >= 0")
	}
	if p.Days <= 0 && p.QuotaMB <= 0 {
		return fmt.Errorf("plan must define days > 0 or quota_mb > 0")
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
