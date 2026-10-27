package stats

import (
	"testing"
	"time"
)

func TestRingOverwriteAndOrder(t *testing.T) {
	r := NewRing(3)
	now := time.Now().Unix()
	r.Add(Sample{TS: now - 300})
	r.Add(Sample{TS: now - 200})
	r.Add(Sample{TS: now - 100})
	r.Add(Sample{TS: now}) // overwrites the oldest (now-300)
	since := r.Since(60)
	if len(since) != 3 {
		t.Fatalf("expected 3 samples after overwrite, got %d", len(since))
	}
	if since[0].TS != now-200 || since[2].TS != now {
		t.Fatalf("samples should be in order [now-200, now-100, now], got %v", since)
	}
}

func TestLatestReturnsMostRecent(t *testing.T) {
	r := NewRing(5)
	r.Add(Sample{TS: 100, MemUsedMB: 50})
	s, ok := r.Latest()
	if !ok {
		t.Fatal("expected a sample")
	}
	if s.TS != 100 || s.MemUsedMB != 50 {
		t.Fatalf("unexpected latest: %+v", s)
	}
}

func TestSinceFiltersByTime(t *testing.T) {
	r := NewRing(5)
	r.Add(Sample{TS: time.Now().Unix() - 300, MemUsedMB: 10}) // 5 min ago
	r.Add(Sample{TS: time.Now().Unix(), MemUsedMB: 20})       // now
	since := r.Since(2)
	if len(since) != 1 {
		t.Fatalf("expected 1 recent sample, got %d", len(since))
	}
	if since[0].MemUsedMB != 20 {
		t.Fatal("since should only include recent (<=2min) sample")
	}
}

func TestCollectHasRealData(t *testing.T) {
	s, err := Collect()
	if err != nil {
		t.Skipf("collect error (system may not have /proc): %v", err)
	}
	if s.MemTotalMB <= 0 {
		t.Fatal("expected MemTotalMB > 0")
	}
	if s.Load1 < 0 {
		t.Fatal("expected Load1 >= 0")
	}
}
