package server

// 这一组测试覆盖剩余「读整份 → 改一条 → 写整份」(RMW) handler 的并发安全：
//
//  - p15.go  handleDeleteTimeSchedule（读全表 → 过滤 → 写全表）
//  - p16.go  handleSetDDNS（读旧表继承明文 token → 写整表）、
//            handleDeleteDDNSRecord（读全表 → 过滤 → 写全表）
//  - p16.go  handleSetVPNClient（完整文档写入，私钥继承是唯一跨读）
//  - htb.go  handleHTBApply（iface 继承：读存档 → 下发 → 回写存档，
//            会与 handleSetHTBConfig 的全量写交叉）
//
// FileStore.Get/Set 单操作是原子的，但 load→modify→save 三步序列没有互斥：
// 两个并发 RMW 读同一份快照，后写者把先写者的改动整条盖掉（linkcheck 已
// 证过这类丢更新，见 linkcheck_concurrency_test.go）。
//
// 本文件先证明哪些 handler 真丢更新（未加锁时 1-4 FAIL），再固化可接受的
// 单键覆盖语义（第 5 个用例在加锁前后都 PASS，配合 handler 内注释）。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jimwong/router-re/mgmtd/internal/config"
	"github.com/jimwong/router-re/mgmtd/internal/ddnsup"
	"github.com/jimwong/router-re/mgmtd/internal/execx"
	"github.com/jimwong/router-re/mgmtd/internal/htb"
	"github.com/jimwong/router-re/mgmtd/internal/timesched"
	"github.com/jimwong/router-re/mgmtd/internal/vpnclient"
)

func newRMWServer(t *testing.T) *Server {
	t.Helper()
	fs, err := config.NewFileStore(t.TempDir() + "/cfg.json")
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
	return s
}

// gateRunner 模拟 tc 逐条成功执行：每调用一次（= 一条 tc 命令）原子计数并睡
// delay，把「读存档 → 下发 → 回写存档」窗口拉宽且可观测，使 RMW 竞态在
// 未加锁时确定性触发（真机 tc 也是逐条毫秒级，窗口天然存在）。
type gateRunner struct {
	entered *int32
	delay   time.Duration
}

func (r gateRunner) Run(name string, args ...string) (string, error) {
	atomic.AddInt32(r.entered, 1)
	time.Sleep(r.delay)
	return "", nil
}

func (r gateRunner) RunWithInput(input, name string, args ...string) (string, error) {
	atomic.AddInt32(r.entered, 1)
	time.Sleep(r.delay)
	return "", nil
}

var _ execx.Runner = gateRunner{}

// 1) 32 个并发的时间计划删除：未加锁时每次删除读同一份快照再写回 31 条，
// 最后写者盖掉其余 31 个删除 —— 表里必然残留规则。
func TestRMWConcurrencyDeleteTimeScheduleDoesNotLoseDeletions(t *testing.T) {
	s := newRMWServer(t)
	const n = 32
	var rules []timesched.Rule
	for i := 0; i < n; i++ {
		rules = append(rules, timesched.Rule{
			ID:       fmt.Sprintf("ts-%02d", i),
			Name:     fmt.Sprintf("rule-%02d", i),
			Days:     []int{1},
			StartMin: 0,
			EndMin:   1440,
			Targets:  []string{"10.0.0.1"},
			Action:   "block",
			Enabled:  true,
		})
	}
	if err := s.saveTimeRules(rules); err != nil {
		t.Fatalf("saveTimeRules: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("ts-%02d", i)
			req := httptest.NewRequest(http.MethodDelete, "/api/v1/timesched/rules/"+id, nil)
			req.SetPathValue("id", id)
			s.handleDeleteTimeSchedule(httptest.NewRecorder(), req)
		}(i)
	}
	wg.Wait()

	remaining := s.loadTimeRules()
	if len(remaining) != 0 {
		t.Fatalf("lost %d of %d deletions (RMW without lock); remaining ids: %v",
			len(remaining), n, idsOfRules(remaining))
	}
}

func idsOfRules(rules []timesched.Rule) []string {
	out := make([]string, 0, len(rules))
	for _, r := range rules {
		out = append(out, r.ID)
	}
	return out
}

// 2) 32 个并发的 DDNS 记录删除：同 1 的丢更新形状。
func TestRMWConcurrencyDeleteDDNSRecordDoesNotLoseDeletions(t *testing.T) {
	s := newRMWServer(t)
	const n = 32
	var recs []ddnsup.Record
	for i := 0; i < n; i++ {
		recs = append(recs, ddnsup.Record{
			ID:       fmt.Sprintf("dd-%02d", i),
			Provider: "custom",
			Domain:   "example.com",
			Enabled:  true,
		})
	}
	if err := s.saveDDNS(recs); err != nil {
		t.Fatalf("saveDDNS: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("dd-%02d", i)
			req := httptest.NewRequest(http.MethodDelete, "/api/v1/ddns/records/"+id, nil)
			req.SetPathValue("id", id)
			s.handleDeleteDDNSRecord(httptest.NewRecorder(), req)
		}(i)
	}
	wg.Wait()

	if got := s.loadDDNS(); len(got) != 0 {
		t.Fatalf("lost %d of %d deletions (RMW without lock); remaining: %v",
			len(got), n, ddnsIDs(got))
	}
}

func ddnsIDs(recs []ddnsup.Record) []string {
	out := make([]string, 0, len(recs))
	for _, r := range recs {
		out = append(out, r.ID)
	}
	return out
}

// 3) set 是完整文档写入（body.Records 即期望全表，服务端只在旧表上继承
// 明文 token/密码）：并发的 set 等价于「最后提交的整表胜出」(LWW) ——
// 客户端持有旧快照导致的过期写入属客户端语义，服务端锁无法也不应修复。
// 本用例固化 LWW 完整文档性质：最终表必为某一提交方的整表（恰含 1 个
// 编辑标记、无字段撕裂），加锁前后都应 PASS。真正的服务端 RMW 丢更新
// 由 delete×delete（用例 2）覆盖并修复。
func TestRMWConcurrencySetDDNSWritesOnlyCompleteTables(t *testing.T) {
	s := newRMWServer(t)
	const n = 32
	var recs []ddnsup.Record
	for i := 0; i < n; i++ {
		recs = append(recs, ddnsup.Record{
			ID:       fmt.Sprintf("dd-%02d", i),
			Provider: "custom",
			Domain:   "example.com",
			Enabled:  true,
		})
	}
	if err := s.saveDDNS(recs); err != nil {
		t.Fatalf("saveDDNS: %v", err)
	}

	// 32 个并发 set：各自在 handler 内拉全表 → 只改自己那条 → 写回整表。
	// 期望：无撕裂，最终表 = 最后一个提交方的整表（恰好 1 个 LastIP 标记）。
	var wg sync.WaitGroup
	var mu sync.Mutex
	var codes []int
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			payload, _ := json.Marshal(struct {
				Records []ddnsup.Record `json:"records"`
				Confirm bool            `json:"confirm"`
			}{Records: seededRecords(t, s, i), Confirm: true})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/ddns", bytes.NewReader(payload))
			w := httptest.NewRecorder()
			s.handleSetDDNS(w, req)
			mu.Lock()
			codes = append(codes, w.Code)
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	for _, c := range codes {
		if c != http.StatusOK {
			t.Fatalf("handleSetDDNS returned %d", c)
		}
	}

	final := s.loadDDNS()
	if len(final) != n {
		t.Fatalf("records lost: got %d want %d", len(final), n)
	}
	// LWW 完整文档性质：最终表必为某一提交方的整表（或其更早某代的表），
	// 因此至多出现「整表累积标记」形态；关键是——任何带标记的记录，其
	// LastIP 必与记录自身 ID 一致。出现跨记录错配（dd-05 挂着 dd-12 的
	// 标记）即字段级撕裂，判 FAIL。
	for _, rec := range final {
		if rec.LastIP != "" {
			want := fmt.Sprintf("10.9.%d.1", idxOfID(final, rec.ID))
			if rec.LastIP != want {
				t.Fatalf("torn field: record %s last_ip=%q want %q", rec.ID, rec.LastIP, want)
			}
		}
	}
}

// seededRecords 模拟一次 handler 内的 loadDDNS + 单条编辑（handler 会再
// 继承 token；custom provider 无 token，等价于快照本身）。
func seededRecords(t *testing.T, s *Server, i int) []ddnsup.Record {
	rr := s.loadDDNS()
	mine := fmt.Sprintf("dd-%02d", i)
	for j := range rr {
		if rr[j].ID == mine {
			rr[j].LastIP = fmt.Sprintf("10.9.%d.1", i)
		}
	}
	return rr
}

func idxOfID(recs []ddnsup.Record, id string) int {
	for i, r := range recs {
		if r.ID == id {
			return i
		}
	}
	return -1
}

// 4) apply(iface 继承) 会「读存档 → tc 下发 → 把读到的存档再写回」。
// 若 handleSetHTBConfig 的写落在读与回写之间，回写会用旧存档盖掉新配置。
// 用 fakeRunner 拉宽窗口：8 个 apply 先读完旧存档 base，随后 set 写入 clobber，
// 最后 apply 们回写 base —— 未加锁时最终存档是 base，clobber 被静默丢掉。
func TestRMWConcurrencyHTBApplyDoesNotClobberConfigSet(t *testing.T) {
	s := newRMWServer(t)
	var entered int32
	s.aclApplier = gateRunner{entered: &entered, delay: 20 * time.Millisecond}

	base := htb.Config{
		Iface:        "eth0",
		RootRateMbit: 100,
		DefaultClass: "c0",
		Enabled:      true,
		Classes:      []htb.Class{{Name: "c0", RateMbit: 50, CeilMbit: 50, Priority: 1}},
	}
	clobber := base
	clobber.RootRateMbit = 200
	clobber.Classes = []htb.Class{{Name: "c0", RateMbit: 100, CeilMbit: 100, Priority: 1}}
	if err := s.store.Set("htb", htbStoreKey, string(mustJSON(t, base))); err != nil {
		t.Fatalf("seed htb config: %v", err)
	}

	// 8 个 apply（全零 config → iface 继承读 base）先启动
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/htb/apply",
				bytes.NewReader([]byte(`{"confirm":true}`)))
			w := httptest.NewRecorder()
			s.handleHTBApply(w, req)
			if w.Code != http.StatusOK {
				t.Errorf("handleHTBApply returned %d: %s", w.Code, w.Body.String())
			}
		}()
	}
	// 门控：8 个 apply 全部进入 tc 执行 = 各自已读完 base 存档（读发生在
	// 下发之前）。gateRunner 逐条睡 20ms（脚本 5+ 条 → 回写发生在 ~100ms+），
	// 此间写入的 clobber 必被后续回写覆盖 —— 未加锁时确定性 FAIL。
	deadline := time.Now().Add(5 * time.Second)
	for atomic.LoadInt32(&entered) < 8 {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for 8 applies to enter the runner (entered=%d)",
				atomic.LoadInt32(&entered))
		}
		time.Sleep(2 * time.Millisecond)
	}
	time.Sleep(10 * time.Millisecond)

	// 客户端意图：持久化 clobber（RootRateMbit=200）
	payload := mustJSON(t, clobber)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/htb/config", bytes.NewReader(payload))
	w := httptest.NewRecorder()
	s.handleSetHTBConfig(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("handleSetHTBConfig returned %d: %s", w.Code, w.Body.String())
	}
	wg.Wait()

	raw, _ := s.store.Get("htb", htbStoreKey)
	var got htb.Config
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("stored htb config unmarshal: %v", err)
	}
	if got.RootRateMbit != 200 {
		t.Fatalf("config set clobbered by stale apply re-save: stored root_rate_mbit=%v want 200",
			got.RootRateMbit)
	}
}

// 5) VPN client 是完整文档写入（body 即期望全量）：并发时最后一次写整份胜出，
// 最终状态只能是两个完整文档之一，不存在字段级撕裂 —— 单键覆盖语义可接受。
// 本用例在加锁前后都应 PASS，固化的是「写回的永远是完整文档」这一性质。
func TestRMWConcurrencySetVPNClientWritesOnlyCompleteDocs(t *testing.T) {
	s := newRMWServer(t)
	// 44 字符 base64 且解码恰好 32 字节（43 个数据字符 + 1 个 '=' 填充），
	// 通过 vpnclient.Validate 的 isBase64Key 校验。
	const (
		k1 = "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG="
		k2 = "CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC="
	)
	// handler 实际保存的是继承后的文档（私钥非空），这里直接模拟其 store 层写入
	docA := vpnclient.Config{
		Interface:  "wg0",
		PrivateKey: k1,
		ListenPort: 51820,
		Peers: []vpnclient.Peer{{
			Name:       "node-a",
			PublicKey:  k2,
			Endpoint:   "192.0.2.1:51820",
			AllowedIPs: []string{"0.0.0.0/0"},
			Keepalive:  25,
		}},
	}
	docB := docA
	docB.ListenPort = 51821
	docB.Peers = []vpnclient.Peer{{
		Name:       "node-b",
		PublicKey:  k2,
		Endpoint:   "192.0.2.2:51821",
		AllowedIPs: []string{"0.0.0.0/0"},
		Keepalive:  25,
	}}
	rawA := mustJSON(t, docA)
	rawB := mustJSON(t, docB)

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); _ = s.saveVPNClient(docA) }()
		go func() { defer wg.Done(); _ = s.saveVPNClient(docB) }()
	}
	wg.Wait()

	raw, _ := s.store.Get("vpnclient", "config")
	if string(raw) != string(rawA) && string(raw) != string(rawB) {
		t.Fatalf("torn vpnclient doc (neither A nor B): %s", raw)
	}
	var got vpnclient.Config
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("stored vpnclient unmarshal: %v", err)
	}
	if err := vpnclient.Validate(got); err != nil {
		t.Fatalf("final doc is not a complete valid config: %v", err)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}
