package fwlog

import (
	"encoding/json"
	"strings"
	"testing"
)

func sampleLines() []string {
	return []string{
		"IN=eth0 OUT= MAC=aa SRC=10.0.0.5 DST=8.8.8.8 LEN=60 PROTO=TCP SPT=12345 DPT=443 DROP",
		"IN=eth0 OUT= MAC=bb SRC=10.0.0.5 DST=1.1.1.1 LEN=40 PROTO=UDP SPT=53 DPT=53 ACCEPT",
		"garbage line with no src",
		"",
		"SRC=192.168.1.9 PROTO=ICMP DROP",
	}
}

func TestParseLinesReal(t *testing.T) {
	evts := ParseLines(sampleLines())
	if len(evts) != 3 {
		t.Fatalf("expected 3 parsed events, got %d", len(evts))
	}
	if evts[0].SrcIP != "10.0.0.5" || evts[0].DstIP != "8.8.8.8" {
		t.Fatalf("src/dst wrong: %+v", evts[0])
	}
	if evts[0].Proto != "TCP" {
		t.Fatalf("proto wrong: %s", evts[0].Proto)
	}
	if evts[0].DstPort != 443 {
		t.Fatalf("dport wrong: %d", evts[0].DstPort)
	}
	if evts[0].Action != "drop" {
		t.Fatalf("action wrong: %s", evts[0].Action)
	}
}

func TestParseLinesGarbageNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic on garbage: %v", r)
		}
	}()
	evts := ParseLines([]string{"", "!!!", "random stuff", "SRC without equals"})
	if len(evts) != 0 {
		t.Fatalf("expected 0 events from garbage, got %d", len(evts))
	}
}

func TestTopTalkersOrder(t *testing.T) {
	evts := []Event{
		{SrcIP: "a", Action: "drop"},
		{SrcIP: "a", Action: "drop"},
		{SrcIP: "b", Action: "drop"},
		{SrcIP: "b", Action: "drop"},
		{SrcIP: "b", Action: "drop"},
		{SrcIP: "c", Action: "accept"},
	}
	got := TopTalkers(evts, 10)
	if len(got) != 3 {
		t.Fatalf("expected 3 talkers, got %d", len(got))
	}
	if got[0].IP != "b" {
		t.Fatalf("first should be b (most drops), got %s", got[0].IP)
	}
	if got[0].Dropped != 3 {
		t.Fatalf("b dropped=%d, want 3", got[0].Dropped)
	}
}

func TestTopTalkersCap(t *testing.T) {
	evts := []Event{{SrcIP: "a", Action: "drop"}}
	got := TopTalkers(evts, 100)
	if len(got) != 1 {
		t.Fatalf("cap must not over-read: got %d", len(got))
	}
	got2 := TopTalkers(evts, 0)
	if len(got2) != 1 {
		t.Fatalf("n=0 should return all, got %d", len(got2))
	}
}

func TestSummarizeCounts(t *testing.T) {
	evts := []Event{
		{SrcIP: "a", Proto: "TCP", Action: "drop", Chain: "INPUT"},
		{SrcIP: "b", Proto: "UDP", Action: "accept", Chain: "INPUT"},
		{SrcIP: "c", Proto: "TCP", Action: "drop", Chain: "FORWARD"},
	}
	s := Summarize(evts)
	if s["total"] != 3 {
		t.Fatalf("total=%v, want 3", s["total"])
	}
	ba := s["by_action"].(map[string]int)
	if ba["drop"] != 2 || ba["accept"] != 1 {
		t.Fatalf("by_action wrong: %v", ba)
	}
}

func TestSuspiciousHighFreq(t *testing.T) {
	now := int64(1000)
	evts := []Event{}
	for i := 0; i < 60; i++ {
		evts = append(evts, Event{SrcIP: "1.2.3.4", Action: "drop", Time: now + int64(i)})
	}
	// 低频 IP
	evts = append(evts, Event{SrcIP: "5.6.7.8", Action: "drop", Time: now})
	got := Suspicious(evts, 60)
	found := false
	for _, ip := range got {
		if ip == "1.2.3.4" {
			found = true
		}
		if ip == "5.6.7.8" {
			t.Fatal("low-freq IP must not be suspicious")
		}
	}
	if !found {
		t.Fatalf("high-freq IP must be suspicious, got %v", got)
	}
}

func TestSuggestBlockThreshold(t *testing.T) {
	evts := []Event{}
	for i := 0; i < 10; i++ {
		evts = append(evts, Event{SrcIP: "9.9.9.9", Action: "drop"})
	}
	for i := 0; i < 3; i++ {
		evts = append(evts, Event{SrcIP: "1.1.1.1", Action: "drop"})
	}
	rules := SuggestBlock(evts, 5)
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d: %v", len(rules), rules)
	}
	if rules[0].SrcIP != "9.9.9.9" {
		t.Fatalf("expected 9.9.9.9, got %s", rules[0].SrcIP)
	}
	if rules[0].Count != 10 {
		t.Fatalf("count=%d, want 10", rules[0].Count)
	}
}

func TestSuggestBlockNoExecField(t *testing.T) {
	evts := []Event{{SrcIP: "9.9.9.9", Action: "drop"}}
	for i := 0; i < 50; i++ {
		evts = append(evts, Event{SrcIP: "9.9.9.9", Action: "drop"})
	}
	rules := SuggestBlock(evts, 5)
	b, _ := json.Marshal(rules)
	s := string(b)
	for _, forbidden := range []string{"command", "exec", "shell", "iptables", "nft"} {
		if strings.Contains(strings.ToLower(s), forbidden) {
			t.Fatalf("Rule JSON must not contain %q (suggestion only): %s", forbidden, s)
		}
	}
}
