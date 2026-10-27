package timesched

import (
	"strings"
	"testing"
	"time"
)

func shanghai() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		panic(err)
	}
	return loc
}

func baseRule() Rule {
	return Rule{
		ID: "r1", Name: "test", Days: []int{1}, StartMin: 480, EndMin: 1080,
		Targets: []string{"10.0.0.1"}, Action: "block", Enabled: true,
	}
}

func TestValidateRejectsCrossMidnight(t *testing.T) {
	r := baseRule()
	r.StartMin = 1380
	r.EndMin = 120
	if err := Validate(r); err == nil {
		t.Fatal("cross-midnight rule must be rejected")
	} else if !contains(err.Error(), "cross-midnight") && !contains(err.Error(), "split") {
		t.Errorf("error should explain cross-midnight: %v", err)
	}
}

func TestValidateRejectsDuplicateDays(t *testing.T) {
	r := baseRule()
	r.Days = []int{1, 1}
	if err := Validate(r); err == nil {
		t.Fatal("duplicate days must be rejected")
	}
}

func TestValidateRejectsEndLEStart(t *testing.T) {
	r := baseRule()
	r.StartMin = 600
	r.EndMin = 600
	if err := Validate(r); err == nil {
		t.Fatal("end<=start must be rejected")
	}
}

func TestValidateRejectsBadTarget(t *testing.T) {
	r := baseRule()
	r.Targets = []string{"not-an-ip"}
	if err := Validate(r); err == nil {
		t.Fatal("bad target must be rejected")
	}
}

func TestValidateAcceptsValid(t *testing.T) {
	r := baseRule()
	if err := Validate(r); err != nil {
		t.Fatalf("valid rule rejected: %v", err)
	}
}

func TestActiveTimezone(t *testing.T) {
	loc := shanghai()
	// 2026-10-26 is a Monday in Shanghai. Construct a UTC instant that is
	// Monday 08:00 in Shanghai (UTC+8 → 00:00 UTC the same day).
	// Monday 2026-10-26 08:00 CST = 2026-10-26 00:00 UTC
	monday8am := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC).Unix()
	r := baseRule()
	r.Days = []int{1} // Monday
	r.StartMin = 480  // 08:00
	r.EndMin = 540    // 09:00
	ok, err := Active(r, monday8am, loc)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("should be active at Monday 08:00 Shanghai time")
	}
	// A naive UTC modulo would have shifted the day; verify the local weekday
	// is Monday by re-checking the time.
	tl := time.Unix(monday8am, 0).In(loc)
	if tl.Weekday() != time.Monday {
		t.Fatalf("expected Monday, got %v", tl.Weekday())
	}
}

func TestActiveDayNotIncluded(t *testing.T) {
	loc := shanghai()
	monday8am := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC).Unix()
	r := baseRule()
	r.Days = []int{2} // Tuesday only
	r.StartMin = 480
	r.EndMin = 540
	ok, err := Active(r, monday8am, loc)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("should not be active on a day not in Days")
	}
}

func TestActiveBoundaryStartMin(t *testing.T) {
	loc := shanghai()
	// Monday 08:00 exactly
	monday8am := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC).Unix()
	r := baseRule()
	r.Days = []int{1}
	r.StartMin = 480
	r.EndMin = 540
	ok, err := Active(r, monday8am, loc)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("should be active at exact StartMin boundary")
	}
}

func TestActiveDisabled(t *testing.T) {
	loc := shanghai()
	monday8am := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC).Unix()
	r := baseRule()
	r.Enabled = false
	ok, err := Active(r, monday8am, loc)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("disabled rule must not be active")
	}
}

func TestMergeBlockOverridesRatelimit(t *testing.T) {
	loc := shanghai()
	monday8am := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC).Unix()
	r1 := baseRule()
	r1.ID = "block1"
	r1.Action = "block"
	r1.Days = []int{1}
	r1.StartMin = 480
	r1.EndMin = 540
	r1.Targets = []string{"10.0.0.1"}
	r2 := baseRule()
	r2.ID = "rate1"
	r2.Action = "ratelimit"
	r2.RateKbps = 1000
	r2.Days = []int{1}
	r2.StartMin = 480
	r2.EndMin = 540
	r2.Targets = []string{"10.0.0.1"}
	d := Merge([]Rule{r1, r2}, "10.0.0.1", monday8am, loc)
	if !d.Blocked {
		t.Fatal("block must override ratelimit")
	}
	if d.RateKbps != 0 {
		t.Errorf("blocked decision should have rate 0, got %d", d.RateKbps)
	}
}

func TestMergeNoMatch(t *testing.T) {
	loc := shanghai()
	monday8am := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC).Unix()
	r := baseRule()
	r.Targets = []string{"10.0.0.2"}
	d := Merge([]Rule{r}, "10.0.0.1", monday8am, loc)
	if d.Blocked {
		t.Fatal("no matching target should not block")
	}
}

func TestRenderNoPolicyDropNoEmptyElements(t *testing.T) {
	loc := shanghai()
	monday8am := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC).Unix()
	r := baseRule()
	out := Render([]Rule{r}, monday8am, loc)
	if contains(out, "policy drop") {
		t.Fatal("Render must not contain policy drop")
	}
	if contains(out, "elements = {}") {
		t.Fatal("Render must not contain empty elements set")
	}
}

func TestRenderEmptyRules(t *testing.T) {
	loc := shanghai()
	monday8am := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC).Unix()
	out := Render([]Rule{}, monday8am, loc)
	if contains(out, "policy drop") {
		t.Fatal("Render must not contain policy drop")
	}
	if contains(out, "elements = {}") {
		t.Fatal("Render must not contain empty elements set")
	}
}

func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}
