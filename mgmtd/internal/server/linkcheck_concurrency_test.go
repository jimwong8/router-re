package server

import (
	"fmt"
	"sync"
	"testing"

	"github.com/jimwong/router-re/mgmtd/internal/config"
	"github.com/jimwong/router-re/mgmtd/internal/linkcheck"
)

// TestConcurrentLinkCheckStateUpdatesDoNotLoseEntries 锁死一个真实的读-改-写竞态。
//
// handleProbeLink 原来是：
//
//	states := s.loadLinkCheckStates()   // 读出整个 map
//	states[body.ID] = newState          // 改自己那一条
//	s.saveLinkCheckStates(states)       // 整个 map 写回去
//
// 两个目标同时探测时，两者读到的是同一份快照，后写的那个把前一个刚写进去的
// 整条覆盖掉——面板上某个目标的状态停在旧值，看起来像"这条链路没在探测"。
// 单目标探测、手动点一次，永远复现不出来；只有并发才出现。
//
// 这里用 32 个并发写模拟，任何一条状态丢失都算失败。
func TestConcurrentLinkCheckStateUpdatesDoNotLoseEntries(t *testing.T) {
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

	const n = 32
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("t%02d", i)
			// 走 handleProbeLink 真正用的入口
			if _, err := s.applyLinkCheckProbe(id, true, int64(1000+i), linkcheck.Target{
				ID: id, Name: id, Kind: "icmp", Host: "127.0.0.1",
			}); err != nil {
				t.Errorf("applyLinkCheckProbe(%s): %v", id, err)
			}
		}(i)
	}
	wg.Wait()

	got := s.loadLinkCheckStates()
	if len(got) != n {
		missing := []string{}
		for i := 0; i < n; i++ {
			id := fmt.Sprintf("t%02d", i)
			if _, ok := got[id]; !ok {
				missing = append(missing, id)
			}
		}
		t.Fatalf("并发探测丢了 %d 条状态（存下 %d/%d）：丢失 %v\n"+
			"读-改-写之间没有互斥，后写者把前写者的整条状态覆盖了",
			n-len(got), len(got), n, missing)
	}
}
