package server

import (
	"fmt"
	"sync"
	"testing"

	"github.com/jimwong/router-re/mgmtd/internal/config"
	"github.com/jimwong/router-re/mgmtd/internal/voucher"
)

// TestConcurrentVoucherRedeemDoesNotResurrectUsedCards 锁死一个「白用上网」的竞态。
//
// handleRedeemVoucher 是：
//
//	list := s.loadVouchers()        // 读出整份卡密表
//	list[i] = v（标记已用）          // 改自己那一张
//	s.saveVouchers(list)            // 整份写回去
//
// 两个人同时核销两张不同的卡时，两者读到同一份快照，后写的把前一张的 used_at
// 抹回 0——那张卡看上去没用过，可以再核销一次。这不是显示问题，是少收钱。
//
// 单张卡、一个人点一次，永远复现不出来。
func TestConcurrentVoucherRedeemDoesNotResurrectUsedCards(t *testing.T) {
	dir := t.TempDir()
	fs, err := config.NewFileStore(dir + "/cfg.json")
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	s := New(Config{
		Addr:          ":0",
		SecretKey:     []byte("0123456789abcdef0123456789abcdef"),
		AdminUser:     "admin",
		AdminPassword: "pw",
	})
	s.store = fs

	const n = 16
	list := make([]voucher.Voucher, n)
	for i := 0; i < n; i++ {
		list[i] = voucher.Voucher{
			Code:     fmt.Sprintf("CARD%02d", i),
			Password: "pw123456",
			Minutes:  60,
		}
	}
	if err := s.saveVouchers(list); err != nil {
		t.Fatalf("saveVouchers: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// 走 handleRedeemVoucher 真正用的入口
			if _, _, _, err := s.redeemVoucherLocked(fmt.Sprintf("CARD%02d", i), "pw123456", 1000); err != nil {
				t.Errorf("核销 CARD%02d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	after := s.loadVouchers()
	unused := []string{}
	for _, v := range after {
		if !v.Used || v.UsedAt == 0 {
			unused = append(unused, v.Code)
		}
	}
	if len(unused) > 0 {
		t.Fatalf("%d/%d 张卡核销后 used 仍为 false：%v\n"+
			"并发核销互相覆盖了整份列表，这些卡可以再白用一次",
			len(unused), n, unused)
	}
}
