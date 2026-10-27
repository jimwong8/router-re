package alerting

import (
	"strings"
	"testing"
)

func validRule() Rule {
	// Pattern 必须和测试里用的消息一致：下面所有测试的 message 都是
	// "failed login"，所以 Pattern 用 "failed"。
	// （这里原本写的是 "error"，与测试消息完全不相交，导致三个测试全部
	// 因为"规则不命中"而失败 —— 看起来像引擎 bug，其实是 fixture 自相矛盾。）
	return Rule{ID: "r1", Name: "test", Pattern: "failed", IsRegex: false, Severity: "warning", Cooldown: 60, Enabled: true}
}

func TestValidateRejectsEmptyPattern(t *testing.T) {
	r := validRule()
	r.Pattern = ""
	if err := Validate(r); err == nil || !strings.Contains(err.Error(), "pattern") {
		t.Fatalf("expected pattern error, got %v", err)
	}
}

func TestValidateRejectsBadSeverity(t *testing.T) {
	r := validRule()
	r.Severity = "fatal"
	if err := Validate(r); err == nil || !strings.Contains(err.Error(), "severity") {
		t.Fatalf("expected severity error, got %v", err)
	}
}

func TestValidateRejectsCooldownOutOfRange(t *testing.T) {
	r := validRule()
	r.Cooldown = 999999
	if err := Validate(r); err == nil || !strings.Contains(err.Error(), "cooldown") {
		t.Fatalf("expected cooldown error, got %v", err)
	}
	// 负值
	r.Cooldown = -5
	if err := Validate(r); err == nil {
		t.Fatal("expected error for negative cooldown")
	}
}

// 非法正则返回错误且错误里含正则编译原文。
func TestValidateRegexErrorContainsPattern(t *testing.T) {
	r := validRule()
	r.IsRegex = true
	r.Pattern = "([unclosed" // 非法正则
	err := Validate(r)
	if err == nil {
		t.Fatal("expected error for invalid regex")
	}
	if !strings.Contains(err.Error(), "([unclosed") {
		t.Fatalf("error should contain the regex text, got %v", err)
	}
}

func TestValidateOK(t *testing.T) {
	if err := Validate(validRule()); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

func TestMatchContainsCaseInsensitive(t *testing.T) {
	r := validRule()
	if !Match(r, "sshd.service", "User root FAILED login attempt") {
		t.Fatal("contains should match case-insensitive (FAILED vs failed)")
	}
	if Match(r, "sshd.service", "clean message") {
		t.Fatal("should not match without pattern")
	}
}

func TestMatchRegex(t *testing.T) {
	r := validRule()
	r.IsRegex = true
	r.Pattern = `\d{4}-\d{2}-\d{2}`
	if !Match(r, "app.service", "disk full at 2026-10-27") {
		t.Fatal("regex should match date")
	}
	if Match(r, "app.service", "no date here") {
		t.Fatal("regex should not match")
	}
}

func TestMatchUnitsRestrict(t *testing.T) {
	r := validRule()
	r.Units = []string{"sshd.service"}
	// 消息必须包含 validRule 的 Pattern（"failed"），否则测的是"不命中"而不是"unit 过滤"
	if !Match(r, "sshd.service", "failed login") {
		t.Fatal("should match when unit is in list")
	}
	if Match(r, "nginx.service", "failed login") {
		t.Fatal("should NOT match when unit not in list")
	}
}

// cooldown 抑制：同一规则连续两次命中，第二次 suppressed 且不返回 alert。
func TestEngineCooldownSuppression(t *testing.T) {
	e := NewEngine()
	r := validRule() // cooldown=60
	now := int64(1000)
	alerts, suppressed := e.Evaluate([]Rule{r}, "sshd.service", "failed login", now)
	if len(alerts) != 1 || suppressed != 0 {
		t.Fatalf("first hit: want 1 alert/0 suppressed, got %d/%d", len(alerts), suppressed)
	}
	// 60s 内再次命中 → 抑制
	alerts, suppressed = e.Evaluate([]Rule{r}, "sshd.service", "failed login", now+30)
	if len(alerts) != 0 || suppressed != 1 {
		t.Fatalf("second hit within cooldown: want 0 alert/1 suppressed, got %d/%d", len(alerts), suppressed)
	}
	// 超过 cooldown → 再次告警
	alerts, suppressed = e.Evaluate([]Rule{r}, "sshd.service", "failed login", now+61)
	if len(alerts) != 1 || suppressed != 0 {
		t.Fatalf("third hit after cooldown: want 1 alert/0 suppressed, got %d/%d", len(alerts), suppressed)
	}
}

// 不同规则各自的 cooldown 互不影响。
func TestEngineCooldownIndependentPerRule(t *testing.T) {
	e := NewEngine()
	r1 := validRule()
	r1.ID = "r1"
	r2 := validRule()
	r2.ID = "r2"
	now := int64(1000)
	// 第一次只让 r1 命中（单独传 r1）—— 这样 r2 才是真正的"首次"。
	// 如果两条一起 Evaluate，r2 在第一次就告警过并记了时间，第二次当然也被抑制，
	// 那就测不出"互不影响"了（这是测试设计错误，不是引擎问题）。
	e.Evaluate([]Rule{r1}, "sshd.service", "failed login", now)
	alerts, suppressed := e.Evaluate([]Rule{r1, r2}, "sshd.service", "failed login", now+30)
	// r1 被抑制，r2 仍告警
	if len(alerts) != 1 || alerts[0].RuleID != "r2" {
		t.Fatalf("want r2 alert, got %+v", alerts)
	}
	if suppressed != 1 {
		t.Fatalf("want 1 suppressed (r1), got %d", suppressed)
	}
}

func TestEngineDisabledRuleSkipped(t *testing.T) {
	e := NewEngine()
	r := validRule()
	r.Enabled = false
	alerts, suppressed := e.Evaluate([]Rule{r}, "sshd.service", "failed login", 1000)
	if len(alerts) != 0 || suppressed != 0 {
		t.Fatalf("disabled rule must be skipped, got %d/%d", len(alerts), suppressed)
	}
}

func TestEngineReset(t *testing.T) {
	e := NewEngine()
	r := validRule()
	e.Evaluate([]Rule{r}, "u", "failed login", 1000)
	e.Reset()
	alerts, _ := e.Evaluate([]Rule{r}, "u", "failed login", 1000+10) // 仍在 cooldown 但已 Reset
	if len(alerts) != 1 {
		t.Fatalf("after Reset cooldown cleared, want 1 alert, got %d", len(alerts))
	}
}

// --- RenderCSV 转义 ---
func TestRenderCSVEscapeComma(t *testing.T) {
	out := RenderCSV([][]string{{"a,b", "x"}})
	if !strings.Contains(out, `"a,b",x`) {
		t.Fatalf("comma field should be quoted: %q", out)
	}
}

func TestRenderCSVEscapeQuote(t *testing.T) {
	out := RenderCSV([][]string{{`say "hi"`}})
	if out != `"say ""hi"""` {
		t.Fatalf("quoted field with doubled quotes: %q", out)
	}
}

func TestRenderCSVEscapeNewline(t *testing.T) {
	out := RenderCSV([][]string{{"line1\nline2", "col2"}})
	if !strings.Contains(out, `"line1`) || !strings.Contains(out, `line2"`) {
		t.Fatalf("newline field should be quoted: %q", out)
	}
	// 完整：第一字段含换行被包裹，第二字段 col2 无特殊字符不外层
	if !strings.Contains(out, "col2") {
		t.Fatalf("missing col2: %q", out)
	}
}

func TestRenderCSVPlain(t *testing.T) {
	out := RenderCSV([][]string{{"a", "b", "c"}})
	if out != "a,b,c" {
		t.Fatalf("plain fields not wrapped: %q", out)
	}
}

// --- RenderJSONL ---
func TestRenderJSONL(t *testing.T) {
	out := RenderJSONL([]string{"hello", `quote " x`, "line\nbreak"})
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("want 3 JSONL lines, got %d", len(lines))
	}
	if !strings.Contains(lines[0], `"message":"hello"`) {
		t.Fatalf("bad line0: %q", lines[0])
	}
	if !strings.Contains(lines[1], `\"`) {
		t.Fatalf("quote should be json-escaped: %q", lines[1])
	}
}

// --- ParseTimeRange ---
func TestParseTimeRangeFormats(t *testing.T) {
	now := int64(1_000_000_000_000) // 任意 ms 基准
	cases := []struct {
		spec string
		want int64 // since = now - want
	}{
		{"1h", 3_600_000},
		{"30m", 30 * 60_000},
		{"24h", 24 * 3_600_000},
		{"7d", 7 * 24 * 3_600_000},
		{"90s", 90_000},
	}
	for _, c := range cases {
		since, until, err := ParseTimeRange(c.spec, now)
		if err != nil {
			t.Fatalf("%s: %v", c.spec, err)
		}
		if until != now {
			t.Fatalf("%s: until=%d want %d", c.spec, until, now)
		}
		if since != now-c.want {
			t.Fatalf("%s: since=%d want %d", c.spec, since, now-c.want)
		}
	}
}

func TestParseTimeRangeInvalid(t *testing.T) {
	now := int64(1_000_000_000_000)
	for _, spec := range []string{"", "abc", "1x", "h", "0h", "10z"} {
		if _, _, err := ParseTimeRange(spec, now); err == nil {
			t.Fatalf("expected error for %q", spec)
		}
	}
}
