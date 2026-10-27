// Package billing 提供套餐与账单的校验与金额计算。
package billing

import "fmt"

// Bill 是一个账号在一个计费周期的账单。
type Bill struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	PlanID      string  `json:"plan_id"`
	PeriodStart int64   `json:"period_start"`
	PeriodEnd   int64   `json:"period_end"`
	UsedMB      int     `json:"used_mb"`
	QuotaMB     int     `json:"quota_mb"`
	Amount      float64 `json:"amount"`
	Status      string  `json:"status"` // pending / paid / overdue / waived
	CreatedAt   int64   `json:"created_at"`
}

var validBillStatuses = map[string]bool{
	"pending": true, "paid": true, "overdue": true, "waived": true,
}

// ValidateBill 校验账单。
func ValidateBill(b Bill) error {
	if b.Username == "" {
		return fmt.Errorf("username must not be empty")
	}
	if b.PlanID == "" {
		return fmt.Errorf("plan_id must not be empty")
	}
	if b.PeriodEnd <= b.PeriodStart {
		return fmt.Errorf("period_end must be > period_start")
	}
	if b.UsedMB < 0 {
		return fmt.Errorf("used_mb must be >= 0")
	}
	if b.QuotaMB < 0 {
		return fmt.Errorf("quota_mb must be >= 0")
	}
	if b.Amount < 0 || b.Amount > 1000000 {
		return fmt.Errorf("amount must be in [0, 1000000]")
	}
	if !validBillStatuses[b.Status] {
		return fmt.Errorf("status must be pending/paid/overdue/waived, got %q", b.Status)
	}
	return nil
}

// effectivePeriodDays 取套餐的计费周期天数（PeriodDays 优先，Days 为旧别名）。
func effectivePeriodDays(p Plan) int {
	if p.PeriodDays > 0 {
		return p.PeriodDays
	}
	return p.Days
}

// AmountFor 计算账单金额：套餐价 + 超量费。
//
// 超量部分 = max(0, usedMB - quotaMB)；每超出 1GB 收套餐价的 5%。
// 但总超量费不超过套餐价的 3 倍（防止天价账单，这是计费系统的硬保护）。
// quotaMB==0 表示不限量，不产生超量费。
// 结果保留两位小数（四舍五入）。
func AmountFor(p Plan, usedMB int) float64 {
	base := p.PriceYuan
	if p.QuotaMB == 0 {
		// 不限量，无超量费
		return round2(base)
	}
	overMB := usedMB - p.QuotaMB
	if overMB < 0 {
		overMB = 0
	}
	// 每超出 1GB 收 5%
	overageFee := (float64(overMB) / 1024.0) * base * 0.05
	// 硬保护：总超量费不超过套餐价的 3 倍
	if overageFee > base*3 {
		overageFee = base * 3
	}
	return round2(base + overageFee)
}

func round2(v float64) float64 {
	// 四舍五入到两位小数
	return float64(int(v*100+0.5)) / 100.0
}

// NewBill 按套餐和用量生成账单。
func NewBill(id, username string, p Plan, usedMB int, periodStart, now int64) Bill {
	days := effectivePeriodDays(p)
	if days <= 0 {
		days = 30
	}
	return Bill{
		ID:          id,
		Username:    username,
		PlanID:      p.ID,
		PeriodStart: periodStart,
		PeriodEnd:   periodStart + int64(days)*86400,
		UsedMB:      usedMB,
		QuotaMB:     p.QuotaMB,
		Amount:      AmountFor(p, usedMB),
		Status:      "pending",
		CreatedAt:   now,
	}
}

// Overdue 判断账单是否逾期（pending 且 now > PeriodEnd + 7 天宽限）。
func Overdue(b Bill, now int64) bool {
	if b.Status != "pending" {
		return false
	}
	return now > b.PeriodEnd+7*86400
}
