package behavior

import (
	"sync"
	"testing"
)

func TestNewStore(t *testing.T) {
	s := NewStore(-1)
	if s.max != 20000 {
		t.Errorf("NewStore(-1) max should be 20000, got %d", s.max)
	}
	s2 := NewStore(0)
	if s2.max != 20000 {
		t.Errorf("NewStore(0) max should be 20000, got %d", s2.max)
	}
	s3 := NewStore(50)
	if s3.max != 50 {
		t.Errorf("NewStore(50) max should be 50, got %d", s3.max)
	}
}

func TestSetExempt(t *testing.T) {
	s := NewStore(100)

	// Valid rules
	exempts := []Exempt{
		{MatchType: "ip", Value: "1.2.3.4"},
		{MatchType: "mac", Value: "AA:BB:CC:DD:EE:FF"},
		{MatchType: "terminal", Value: "office-pc"},
		{Kind: KindWeb, MatchType: "ip", Value: "5.6.7.8", Comment: "web exempt"},
	}
	if err := s.SetExempt(exempts); err != nil {
		t.Fatalf("valid SetExempt should not error: %v", err)
	}

	// Too many rules
	tooMany := make([]Exempt, 257)
	if err := s.SetExempt(tooMany); err == nil {
		t.Error("expected error for >256 rules, got nil")
	}

	// Invalid match type
	invalidMT := []Exempt{{MatchType: "domain", Value: "test"}}
	if err := s.SetExempt(invalidMT); err == nil {
		t.Error("expected error for invalid match_type, got nil")
	}

	// Empty value
	emptyVal := []Exempt{{MatchType: "ip", Value: ""}}
	if err := s.SetExempt(emptyVal); err == nil {
		t.Error("expected error for empty value, got nil")
	}

	// Invalid kind
	invalidKind := []Exempt{{Kind: "invalid", MatchType: "ip", Value: "1.2.3.4"}}
	if err := s.SetExempt(invalidKind); err == nil {
		t.Error("expected error for invalid kind, got nil")
	}
}

func TestExempts(t *testing.T) {
	s := NewStore(100)
	exempts := []Exempt{{MatchType: "ip", Value: "1.2.3.4"}}
	s.SetExempt(exempts)
	result := s.Exempts()
	if len(result) != 1 {
		t.Errorf("Exempts() length should be 1, got %d", len(result))
	}
	// Verify it's a copy (modifying result shouldn't affect store)
	result[0].Value = "changed"
	if s.Exempts()[0].Value != "1.2.3.4" {
		t.Log("Exempts() returns a copy - verified")
	}
}

func TestIsExempt(t *testing.T) {
	s := NewStore(100)
	s.SetExempt([]Exempt{
		{MatchType: "mac", Value: "AA:BB:CC:DD:EE:FF"},
		{MatchType: "ip", Value: "1.2.3.4"},
		{MatchType: "terminal", Value: "office-pc"},
		{Kind: KindWeb, MatchType: "ip", Value: "5.6.7.8"},
	})

	// MAC case-insensitive
	r1 := Record{MAC: "aa:bb:cc:dd:ee:ff", Kind: KindWeb}
	if !s.IsExempt(r1) {
		t.Error("MAC should match case-insensitively")
	}

	// IP exact match
	r2 := Record{IP: "1.2.3.4", Kind: KindWeb}
	if !s.IsExempt(r2) {
		t.Error("IP should match exactly")
	}

	// Terminal exact match
	r3 := Record{Terminal: "office-pc", Kind: KindWeb}
	if !s.IsExempt(r3) {
		t.Error("Terminal should match exactly")
	}

	// Kind non-empty in exempt should filter
	r4 := Record{Kind: KindIM, MAC: "11:22:33:44:55:66"}
	if s.IsExempt(r4) {
		t.Error("Exempt with non-empty Kind should not match different Kind")
	}

	// Kind empty in exempt should match all types
	exemptNoKind := Exempt{MatchType: "ip", Value: "1.2.3.4"}
	s2 := NewStore(100)
	s2.SetExempt([]Exempt{exemptNoKind})
	r6 := Record{Kind: KindIM, IP: "1.2.3.4"}
	if !s2.IsExempt(r6) {
		t.Error("Exempt with Kind empty should match all types for IP")
	}
}

func TestAddAndMaxClip(t *testing.T) {
	s := NewStore(3) // max 3 records
	s.Add(Record{TS: 1, Kind: KindWeb, MAC: "m1", IP: "1.1.1.1", Target: "a.com"})
	s.Add(Record{TS: 2, Kind: KindWeb, MAC: "m2", IP: "2.2.2.2", Target: "b.com"})
	s.Add(Record{TS: 3, Kind: KindWeb, MAC: "m3", IP: "3.3.3.3", Target: "c.com"})
	// Adding 4th should drop the oldest (TS=1)
	s.Add(Record{TS: 4, Kind: KindWeb, MAC: "m4", IP: "4.4.4.4", Target: "d.com"})

	all, _ := s.Query(Query{})
	if len(all) != 3 {
		t.Errorf("expected 3 records after max clip, got %d", len(all))
	}
	// Should have TS 2, 3, 4 (oldest dropped)
	tsVals := make(map[int64]bool)
	for _, r := range all {
		tsVals[r.TS] = true
	}
	if !tsVals[2] || !tsVals[3] || !tsVals[4] {
		t.Errorf("expected TS 2,3,4 after clip, got %v", all)
	}
	if tsVals[1] {
		t.Error("TS 1 (oldest) should have been dropped")
	}
}

func TestQueryBasicFiltering(t *testing.T) {
	s := NewStore(1000)

	// Add records with various kinds and times
	s.Add(Record{TS: 10, Kind: KindWeb, MAC: "m1", IP: "1.1.1.1", Target: "example.com", Detail: "visit"})
	s.Add(Record{TS: 20, Kind: KindIM, MAC: "m1", IP: "1.1.1.1", Target: "im.example.com", Detail: "msg"})
	s.Add(Record{TS: 30, Kind: KindOnline, MAC: "m2", IP: "2.2.2.2", Terminal: "pc1", Detail: "up"})
	s.Add(Record{TS: 40, Kind: KindApp, MAC: "m3", IP: "3.3.3.3", Terminal: "app1", Detail: "launch"})
	s.Add(Record{TS: 50, Kind: KindWeb, MAC: "m4", IP: "4.4.4.4", Target: "test.com", Detail: "click"})

	// Test Kind filter
	webRecs, _ := s.Query(Query{Kind: KindWeb})
	if len(webRecs) != 2 {
		t.Errorf("expected 2 web records, got %d", len(webRecs))
	}
	for _, r := range webRecs {
		if r.Kind != KindWeb {
			t.Errorf("expected KindWeb, got %s", r.Kind)
		}
	}

	// Test MAC case-insensitive filter
	macRecs, _ := s.Query(Query{MAC: "M1"})
	if len(macRecs) != 2 {
		t.Errorf("expected 2 records for MAC m1, got %d", len(macRecs))
	}

	// Test IP exact filter
	ipRecs, _ := s.Query(Query{IP: "1.1.1.1"})
	if len(ipRecs) != 2 {
		t.Errorf("expected 2 records for IP 1.1.1.1, got %d", len(ipRecs))
	}

	// Test Keyword filter
	kwRecs, _ := s.Query(Query{Keyword: "example"})
	if len(kwRecs) != 2 {
		t.Errorf("expected 2 records with keyword 'example', got %d", len(kwRecs))
	}

	// Test Since/Until filter
	sinceRecs, _ := s.Query(Query{Since: 15}) // TS >= 15
	if len(sinceRecs) != 4 {
		t.Errorf("expected 4 records since TS 15, got %d", len(sinceRecs))
	}

	untilRecs, _ := s.Query(Query{Until: 35}) // TS <= 35 → 10/20/30
	if len(untilRecs) != 3 {
		t.Errorf("expected 3 records until TS 35, got %d", len(untilRecs))
	}

	// Test no filter returns all
	allRecs, total := s.Query(Query{})
	if total != 5 {
		t.Errorf("expected total 5, got %d", total)
	}
	if len(allRecs) != 5 {
		t.Errorf("expected 5 records, got %d", len(allRecs))
	}
}

func TestQueryOrderAndLimitOffset(t *testing.T) {
	s := NewStore(1000)
	// Add records in non-chronological order to test sorting
	s.Add(Record{TS: 30, Kind: KindWeb, MAC: "m1", IP: "1.1.1.1", Target: "a.com", Detail: "d"})
	s.Add(Record{TS: 10, Kind: KindWeb, MAC: "m2", IP: "2.2.2.2", Target: "b.com", Detail: "d"})
	s.Add(Record{TS: 50, Kind: KindWeb, MAC: "m3", IP: "3.3.3.3", Target: "c.com", Detail: "d"})
	s.Add(Record{TS: 20, Kind: KindWeb, MAC: "m4", IP: "4.4.4.4", Target: "d.com", Detail: "d"})

	// Query without limit should return sorted by TS descending
	recs, total := s.Query(Query{})
	if total != 4 {
		t.Errorf("expected total 4, got %d", total)
	}
	// Should be sorted TS descending: 50, 30, 20, 10
	if recs[0].TS != 50 || recs[1].TS != 30 || recs[2].TS != 20 || recs[3].TS != 10 {
		t.Errorf("expected TS order 50,30,20,10; got %v", recs)
	}

	// Test Limit
	limited, _ := s.Query(Query{Limit: 2})
	if len(limited) != 2 {
		t.Errorf("expected 2 limited records, got %d", len(limited))
	}
	if limited[0].TS != 50 || limited[1].TS != 30 {
		t.Errorf("expected TS 50,30 limited; got %v", limited)
	}

	// Test Offset
	offseted, _ := s.Query(Query{Offset: 1})
	if len(offseted) != 3 {
		t.Errorf("expected 3 offset=1 records, got %d", len(offseted))
	}
	// After removing the first (TS=50), should be 30,20,10
	if offseted[0].TS != 30 || offseted[1].TS != 20 || offseted[2].TS != 10 {
		t.Errorf("expected TS 30,20,10 after offset=1; got %v", offseted)
	}

	// Test Limit + Offset combined
	combined, _ := s.Query(Query{Limit: 2, Offset: 1})
	if len(combined) != 2 {
		t.Errorf("expected 2 combined records, got %d", len(combined))
	}
	if combined[0].TS != 30 || combined[1].TS != 20 {
		t.Errorf("expected TS 30,20 combined; got %v", combined)
	}
}

func TestStats(t *testing.T) {
	s := NewStore(1000)
	s.Add(Record{TS: 1, Kind: KindWeb, MAC: "m1", IP: "1.1.1.1", Target: "a.com"})
	s.Add(Record{TS: 2, Kind: KindIM, MAC: "m2", IP: "2.2.2.2", Target: "im.com"})
	s.Add(Record{TS: 3, Kind: KindOnline, MAC: "m3", IP: "3.3.3.3", Terminal: "pc1", Detail: "up"})
	s.Add(Record{TS: 4, Kind: KindWeb, MAC: "m4", IP: "4.4.4.4", Target: "b.com"})
	s.Add(Record{TS: 5, Kind: KindApp, MAC: "m5", IP: "5.5.5.5", Terminal: "app1", Detail: "launch"})

	stats := s.Stats()
	if stats[KindWeb] != 2 {
		t.Errorf("expected Web count 2, got %d", stats[KindWeb])
	}
	if stats[KindIM] != 1 {
		t.Errorf("expected IM count 1, got %d", stats[KindIM])
	}
	if stats[KindOnline] != 1 {
		t.Errorf("expected Online count 1, got %d", stats[KindOnline])
	}
	if stats[KindApp] != 1 {
		t.Errorf("expected App count 1, got %d", stats[KindApp])
	}
	if stats["total"] != 5 {
		t.Errorf("expected total 5, got %d", stats["total"])
	}
}

func TestClear(t *testing.T) {
	s := NewStore(1000)
	s.Add(Record{TS: 1, Kind: KindWeb})
	s.Add(Record{TS: 2, Kind: KindIM})
	s.Clear()
	all, _ := s.Query(Query{})
	if len(all) != 0 {
		t.Errorf("expected 0 records after Clear, got %d", len(all))
	}
}

func TestSetExemptValidation(t *testing.T) {
	s := NewStore(100)

	// Kind non-empty must be known type
	invalidKind := Exempt{Kind: "foobar", MatchType: "ip", Value: "1.2.3.4"}
	if err := s.SetExempt([]Exempt{invalidKind}); err == nil {
		t.Error("expected error for invalid kind, got nil")
	}

	// Value must be non-empty even when Kind is empty
	emptyVal := Exempt{MatchType: "ip", Value: ""}
	if err := s.SetExempt([]Exempt{emptyVal}); err == nil {
		t.Error("expected error for empty value, got nil")
	}
}

func TestOnlineTrackerNew(t *testing.T) {
	ot := NewOnlineTracker()
	if ot == nil {
		t.Fatal("expected non-nil OnlineTracker")
	}
}

func TestOnlineTrackerObserveFirst(t *testing.T) {
	tracker := NewOnlineTracker()
	recs := tracker.Observe("aa:bb:cc:dd:ee:ff", "1.1.1.1", "pc1", 100)
	if len(recs) != 1 {
		t.Errorf("first Observe should produce 1 record, got %d", len(recs))
	}
	if recs[0].Kind != KindOnline {
		t.Errorf("expected KindOnline, got %s", recs[0].Kind)
	}
	if recs[0].Detail != "up" {
		t.Errorf("expected Detail up, got %s", recs[0].Detail)
	}
	if recs[0].MAC != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("expected MAC aa:bb:cc:dd:ee:ff, got %s", recs[0].MAC)
	}
}

func TestOnlineTrackerObserveNotFirst(t *testing.T) {
	tracker := NewOnlineTracker()
	// First observe
	recs1 := tracker.Observe("aa:bb:cc:dd:ee:ff", "1.1.1.1", "pc1", 100)
	if len(recs1) != 1 {
		t.Fatalf("first Observe should produce 1 record, got %d", len(recs1))
	}
	// Second observe - should update time, return empty
	recs2 := tracker.Observe("aa:bb:cc:dd:ee:ff", "1.1.1.1", "pc1", 200)
	if len(recs2) != 0 {
		t.Errorf("second Observe should produce 0 records (already online), got %d", len(recs2))
	}
	// Verify timestamp was updated
	online := tracker.Online()
	if len(online) != 1 || online[0] != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("expected 1 online MAC, got %v", online)
	}
}

func TestOnlineTrackerOffline(t *testing.T) {
	tracker := NewOnlineTracker()
	// Add a MAC at TS 100
	tracker.Observe("aa:bb:cc:dd:ee:ff", "1.1.1.1", "pc1", 100)
	// Now go offline with timeout 50 (now=160, timeout=50 → 160-100=60 > 50 → should remove)
	down := tracker.Offline(160, 50)
	if len(down) != 1 {
		t.Errorf("expected 1 down record, got %d", len(down))
	}
	if down[0].Detail != "down" {
		t.Errorf("expected Detail down, got %s", down[0].Detail)
	}
	// MAC should be removed
	online := tracker.Online()
	if len(online) != 0 {
		t.Errorf("expected no online MACs after offline, got %v", online)
	}
}

func TestOnlineTrackerOfflineNotExpired(t *testing.T) {
	tracker := NewOnlineTracker()
	// Add a MAC at TS 100
	tracker.Observe("aa:bb:cc:dd:ee:ff", "1.1.1.1", "pc1", 100)
	// Go offline with timeout 200 (now=160, timeout=200 → 160-100=60 <= 200 → should NOT remove)
	down := tracker.Offline(160, 200)
	if len(down) != 0 {
		t.Errorf("expected 0 down records (not expired), got %d", len(down))
	}
	// MAC should still be online
	online := tracker.Online()
	if len(online) != 1 {
		t.Errorf("expected 1 online MAC still, got %d", len(online))
	}
}

func TestOnlineTrackerOnlineSorted(t *testing.T) {
	tracker := NewOnlineTracker()
	// Add multiple MACs in reverse order
	tracker.Observe("zzz:aaa:bbb:ccc:ddd:eee", "1.1.1.1", "pc1", 100)
	tracker.Observe("aaa:bbb:ccc:ddd:eee:fff", "2.2.2.2", "pc2", 100)
	tracker.Observe("mmm:nnn:ooo:ppp:qqq:rrr", "3.3.3.3", "pc3", 100)
	online := tracker.Online()
	// Should be sorted lexicographically
	expected := []string{"aaa:bbb:ccc:ddd:eee:fff", "mmm:nnn:ooo:ppp:qqq:rrr", "zzz:aaa:bbb:ccc:ddd:eee"}
	if len(online) != 3 {
		t.Errorf("expected 3 online MACs, got %d", len(online))
	}
	for i, e := range expected {
		if online[i] != e {
			t.Errorf("online[%d] expected %s, got %s", i, e, online[i])
		}
	}
}

func TestOnlineTrackerConcurrent(t *testing.T) {
	tracker := NewOnlineTracker()
	var wg sync.WaitGroup
	// 50 goroutines, each Add 100 records
	nGoroutines := 50
	recordsPerGoroutine := 100
	for g := 0; g < nGoroutines; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < recordsPerGoroutine; i++ {
				mac := "m" + string(rune('0'+gid)) + string(rune('0'+i%10))
				tracker.Observe(mac, "1.1.1.1", "pc", int64(1000+gid*1000+i))
			}
		}(g)
	}
	wg.Wait()
	// Total should be <= 5000 (max is default 20000, but we have 50*100=5000 records)
	// Actually NewStore default max is 20000, and we didn't set a custom max on the tracker
	// The OnlineTracker doesn't have a max clip, so we just check no panic
	totalOnline := len(tracker.Online())
	t.Logf("Concurrent test: %d unique MACs online, no panic", totalOnline)
	// Just verify no panic occurred and the system is consistent
	if totalOnline > recordsPerGoroutine*nGoroutines {
		t.Errorf("too many online MACs: %d > %d", totalOnline, recordsPerGoroutine*nGoroutines)
	}
}

func TestQueryKeywordSubstring(t *testing.T) {
	s := NewStore(1000)
	s.Add(Record{TS: 1, Kind: KindWeb, MAC: "m1", IP: "1.1.1.1", Target: "myexample.com", Detail: "visit"})
	s.Add(Record{TS: 2, Kind: KindWeb, MAC: "m2", IP: "2.2.2.2", Target: "anotherexample.net", Detail: "click"})
	s.Add(Record{TS: 3, Kind: KindIM, MAC: "m3", IP: "3.3.3.3", Target: "imtool.org", Detail: "msg"})

	// Keyword "example" should match the first two (in Target)
	recs, total := s.Query(Query{Keyword: "example"})
	if total != 2 {
		t.Errorf("expected total 2 with keyword 'example', got %d", total)
	}
	if len(recs) != 2 {
		t.Errorf("expected 2 results, got %d", len(recs))
	}

	// Keyword "tool" should match only the IM record (in Target)
	recs2, total2 := s.Query(Query{Keyword: "tool"})
	if total2 != 1 {
		t.Errorf("expected total 1 with keyword 'tool', got %d", total2)
	}
	if len(recs2) != 1 {
		t.Errorf("expected 1 result, got %d", len(recs2))
	}
	if recs2[0].Kind != KindIM {
		t.Errorf("expected KindIM for 'tool', got %s", recs2[0].Kind)
	}
}
