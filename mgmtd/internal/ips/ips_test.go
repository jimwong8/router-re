package ips

import (
	"strings"
	"testing"
)

const sqlLine = `alert tcp any any -> any 80 (msg:"ET WEB SQL Injection"; content:"union select"; nocase; sid:1000001; rev:1; classification:web-application-attack; priority:2;)`

func TestParseRuleRealSuricata(t *testing.T) {
	r, err := ParseRule(sqlLine)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if r.Action != "alert" {
		t.Errorf("action=%q", r.Action)
	}
	if r.Proto != "tcp" {
		t.Errorf("proto=%q", r.Proto)
	}
	if r.DstPort != "80" {
		t.Errorf("dst_port=%q", r.DstPort)
	}
	if r.SID != 1000001 {
		t.Errorf("sid=%d", r.SID)
	}
	if r.Severity != 2 {
		t.Errorf("severity=%d", r.Severity)
	}
	if r.Msg != "ET WEB SQL Injection" {
		t.Errorf("msg=%q", r.Msg)
	}
	if r.PayloadAscii == "" {
		t.Error("expected payload_ascii set")
	}
}

func TestParseRuleHexContent(t *testing.T) {
	line := `alert tcp any any -> any 80 (msg:"hex"; content:"|27 20 6f 72 20 31 3d 31|"; sid:2;)`
	r, err := ParseRule(line)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// |27 20 6f 72 20 31 3d 31| => "' or 1=1"
	want := "' or 1=1"
	if r.PayloadHex != want {
		t.Fatalf("payload_hex=%q want=%q", r.PayloadHex, want)
	}
	if r.PayloadAscii != "" {
		t.Errorf("payload_ascii should be empty for pure-hex, got %q", r.PayloadAscii)
	}
}

func TestParseRuleEscapedQuoteNotTerminator(t *testing.T) {
	line := `alert tcp any any -> any 80 (msg:"q"; content:"a\"b\"c"; sid:3;)`
	r, err := ParseRule(line)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if r.PayloadAscii != `a"b"c` {
		t.Fatalf("payload_ascii=%q want %q", r.PayloadAscii, `a"b"c`)
	}
}

func TestParseRuleMixedAsciiAndHex(t *testing.T) {
	// "abc" + |20| +  "def" => ascii "abcdef", hex " "
	line := `alert tcp any any -> any 80 (msg:"m"; content:"abc|20|def"; sid:4;)`
	r, err := ParseRule(line)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if r.PayloadAscii != "abcdef" {
		t.Errorf("payload_ascii=%q", r.PayloadAscii)
	}
	if r.PayloadHex != " " {
		t.Errorf("payload_hex=%q", r.PayloadHex)
	}
}

// --- Validate 失败分支 ---

func validRule() Rule {
	return Rule{Action: "alert", Proto: "tcp", SrcPort: "any", DstPort: "80", Msg: "x", SID: 1, Severity: 2, PayloadAscii: "abc"}
}

func TestValidateSID(t *testing.T) {
	r := validRule()
	r.SID = 0
	if err := r.Validate(); err == nil || !strings.Contains(err.Error(), "sid") {
		t.Fatalf("got %v", err)
	}
}

func TestValidateBadAction(t *testing.T) {
	r := validRule()
	r.Action = "reject"
	if err := r.Validate(); err == nil || !strings.Contains(err.Error(), "action") {
		t.Fatalf("got %v", err)
	}
}

func TestValidateBadPort(t *testing.T) {
	r := validRule()
	r.DstPort = "80:40"
	if err := r.Validate(); err == nil || !strings.Contains(err.Error(), "dst_port") {
		t.Fatalf("got %v", err)
	}
}

func TestValidateEmptyMsg(t *testing.T) {
	r := validRule()
	r.Msg = ""
	if err := r.Validate(); err == nil || !strings.Contains(err.Error(), "msg") {
		t.Fatalf("got %v", err)
	}
}

func TestValidateEmptyContent(t *testing.T) {
	r := validRule()
	r.PayloadAscii = ""
	r.PayloadHex = ""
	if err := r.Validate(); err == nil || !strings.Contains(err.Error(), "payload") {
		t.Fatalf("got %v", err)
	}
}

func TestValidateOK(t *testing.T) {
	if err := validRule().Validate(); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

// --- Match ---

func TestMatchHitCaseInsensitive(t *testing.T) {
	r := validRule() // PayloadAscii="abc"
	if !r.Match("tcp", 80, []byte("XX UNION SELECT ABCdef")) {
		t.Fatal("expected case-insensitive hit")
	}
}

func TestMatchMiss(t *testing.T) {
	r := validRule()
	if r.Match("tcp", 80, []byte("nothing here")) {
		t.Fatal("expected miss")
	}
}

func TestMatchProtoMismatch(t *testing.T) {
	r := validRule()
	if r.Match("udp", 80, []byte("abc")) {
		t.Fatal("proto mismatch should not match")
	}
}

func TestMatchPortRange(t *testing.T) {
	r := validRule()
	r.DstPort = "80:90"
	if !r.Match("tcp", 85, []byte("abc")) {
		t.Fatal("85 should be in 80:90")
	}
	if r.Match("tcp", 91, []byte("abc")) {
		t.Fatal("91 should not match 80:90")
	}
}

func TestMatchHexPayload(t *testing.T) {
	r := validRule()
	r.PayloadAscii = ""
	r.PayloadHex = "' or 1=1"
	if !r.Match("tcp", 80, []byte("union select ' or 1=1 from")) {
		t.Fatal("expected hex payload hit")
	}
	if r.Match("tcp", 80, []byte("union select from")) {
		t.Fatal("expected hex payload miss")
	}
}

// --- ParseRulesFile: bad line recorded, others parsed ---

func TestParseRulesFileSkipsBad(t *testing.T) {
	text := "# comment\n\n" + sqlLine + "\nthis is not a rule\nalert tcp any any -> any 53 (msg:\"x\"; content:\"a\"; sid:5;)"
	rules, err := ParseRulesFile(text)
	if err == nil {
		t.Fatal("expected aggregate error for bad line")
	}
	if !strings.Contains(err.Error(), "line 4") {
		t.Fatalf("error should mention failing line number, got %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 parsed rules, got %d", len(rules))
	}
}

func TestParseRulesFileBadLineStillContinues(t *testing.T) {
	text := "garbage line\nalert tcp any any -> any 80 (msg:\"x\"; content:\"abc\"; sid:7;)"
	rules, err := ParseRulesFile(text)
	if err == nil {
		t.Fatal("expected error for garbage line")
	}
	if !strings.Contains(err.Error(), "line 1") {
		t.Fatalf("expected line 1 in error, got %v", err)
	}
	if len(rules) != 1 || rules[0].SID != 7 {
		t.Fatalf("expected the good rule to still parse, got %+v", rules)
	}
}

// --- RenderNft: never flush ruleset, multiline chain ---

func TestRenderNftNoFlush(t *testing.T) {
	out := RenderNft(map[string]bool{"1.2.3.4": true, "5.6.7.8": true})
	if strings.Contains(out, "flush ruleset") {
		t.Fatalf("must not emit flush ruleset:\n%s", out)
	}
	if !strings.Contains(out, "table inet router_ips {") {
		t.Fatal("missing table inet router_ips")
	}
	if !strings.Contains(out, "set blackhole {") {
		t.Fatal("missing set blackhole")
	}
	if !strings.Contains(out, "elements = {") {
		t.Fatalf("missing elements line:\n%s", out)
	}
	if !strings.Contains(out, "1.2.3.4") || !strings.Contains(out, "5.6.7.8") {
		t.Fatalf("missing blocked ips:\n%s", out)
	}
}

func TestRenderNftEmptyNoElements(t *testing.T) {
	out := RenderNft(nil)
	if strings.Contains(out, "elements =") {
		t.Fatalf("empty blackhole must not emit elements line (nft syntax error):\n%s", out)
	}
}
