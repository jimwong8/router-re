package server

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/jimwong/router-re/mgmtd/internal/config"
)

// TestConcurrentMultiWANProbeResultsDoNotLoseMembers 锁死多 WAN 探测结果的竞态。
//
// handleMultiWANProbeResult 是「读整份 states → 改自己那个 member → 写整份」。
// 多个成员的探测结果同时回来时，后写的把前一个成员的状态整条覆盖掉——
// 于是在故障切换判定里，那条链路的状态永远停留在旧值：
// 已经挂掉的链路显示 up（流量继续往死链路打），或者已经恢复的显示 down。
//
// 这不是显示问题，是切换决策用错数据。
func TestConcurrentMultiWANProbeResultsDoNotLoseMembers(t *testing.T) {
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
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			member := fmt.Sprintf("wan%d", i)
			if _, _, err := s.applyWANProbeResult(member, i%2 == 0, "", int64(1000+i)); err != nil {
				t.Errorf("applyWANProbeResult(%s): %v", member, err)
			}
		}(i)
	}
	wg.Wait()

	raw, _ := s.store.Get("multiwan", wanStatesKey)
	states := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &states); err != nil {
		t.Fatalf("解析 states 失败: %v（raw=%q）", err, raw)
	}
	if len(states) != n {
		missing := []string{}
		for i := 0; i < n; i++ {
			m := fmt.Sprintf("wan%d", i)
			if _, ok := states[m]; !ok {
				missing = append(missing, m)
			}
		}
		t.Fatalf("并发探测丢了 %d 个成员的状态（存下 %d/%d）：丢失 %v\n"+
			"故障切换会拿这些旧状态判断链路好坏",
			n-len(states), len(states), n, missing)
	}
}
