package diagreport

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

type stubRunner struct {
	outs  map[string]string
	errs  map[string]error
	calls int
}

func (s *stubRunner) Run(name string, args ...string) (string, error) {
	s.calls++
	k := name + " " + strings.Join(args, " ")
	if e, ok := s.errs[k]; ok {
		return "", e
	}
	return s.outs[k], nil
}

func (s *stubRunner) RunWithInput(input, name string, args ...string) (string, error) {
	return s.Run(name, args...)
}

var _ execx.Runner = (*stubRunner)(nil)

func baseCommands() map[string][]string {
	return map[string][]string{
		"interfaces": {"ip", "-br", "addr"},
		"routes":     {"ip", "route"},
	}
}

func TestCollectSurvivesFailure(t *testing.T) {
	r := &stubRunner{
		outs: map[string]string{
			"ip -br addr": "eth0 UP 10.0.0.1/24",
		},
		errs: map[string]error{
			"ip route": fmt.Errorf("permission denied"),
		},
	}
	cmds := baseCommands()
	rep := Collect("host1", 1000, r, cmds)
	// interfaces 有内容
	var ifaceSec, routeSec *Section
	for i := range rep.Sections {
		if rep.Sections[i].Name == "interfaces" {
			ifaceSec = &rep.Sections[i]
		}
		if rep.Sections[i].Name == "routes" {
			routeSec = &rep.Sections[i]
		}
	}
	if ifaceSec == nil || len(ifaceSec.Items) == 0 {
		t.Fatal("interfaces section should have content")
	}
	if routeSec == nil || len(routeSec.Errors) == 0 {
		t.Fatal("routes section should have errors")
	}
	if rep.Summary.Failed != 1 {
		t.Fatalf("Summary.Failed=%d, want 1", rep.Summary.Failed)
	}
}

func TestCollectSplitsLines(t *testing.T) {
	r := &stubRunner{
		outs: map[string]string{
			"ip -br addr": "line1\nline2\nline3",
			"ip route":    "r1",
		},
		errs: map[string]error{},
	}
	rep := Collect("h", 1, r, baseCommands())
	var sec *Section
	for i := range rep.Sections {
		if rep.Sections[i].Name == "interfaces" {
			sec = &rep.Sections[i]
		}
	}
	if sec == nil {
		t.Fatal("interfaces section missing")
	}
	if len(sec.Items) != 3 {
		t.Fatalf("expected 3 items, got %d: %v", len(sec.Items), sec.Items)
	}
	if sec.Items[0] != "line1" || sec.Items[1] != "line2" || sec.Items[2] != "line3" {
		t.Fatalf("items wrong: %v", sec.Items)
	}
}

func TestRenderTextHasMarkers(t *testing.T) {
	r := &stubRunner{
		outs: map[string]string{"ip -br addr": "ok line"},
		errs: map[string]error{"ip route": fmt.Errorf("boom")},
	}
	rep := Collect("h", 1, r, baseCommands())
	txt := RenderText(rep)
	if !strings.Contains(txt, "[OK]") {
		t.Fatal("RenderText must contain [OK]")
	}
	if !strings.Contains(txt, "[FAIL]") {
		t.Fatal("RenderText must contain [FAIL]")
	}
}

func TestRenderTextFailureShowsError(t *testing.T) {
	r := &stubRunner{
		outs: map[string]string{"ip -br addr": "ok line"},
		errs: map[string]error{"ip route": fmt.Errorf("permission denied")},
	}
	rep := Collect("h", 1, r, baseCommands())
	txt := RenderText(rep)
	if !strings.Contains(txt, "错误: permission denied") {
		t.Fatalf("RenderText must show error line, got:\n%s", txt)
	}
}

func TestSummaryTotalEqualsSections(t *testing.T) {
	r := &stubRunner{outs: map[string]string{}, errs: map[string]error{}}
	cmds := map[string][]string{
		"a": {"echo", "a"},
		"b": {"echo", "b"},
		"c": {"echo", "c"},
	}
	rep := Collect("h", 1, r, cmds)
	if rep.Summary.Total != len(rep.Sections) {
		t.Fatalf("Total=%d, sections=%d", rep.Summary.Total, len(rep.Sections))
	}
	if rep.Summary.Total != 3 {
		t.Fatalf("Total=%d, want 3", rep.Summary.Total)
	}
}

func TestEmptyOutputNoEmptyItems(t *testing.T) {
	r := &stubRunner{
		outs: map[string]string{"ip -br addr": ""},
		errs: map[string]error{},
	}
	cmds := map[string][]string{"interfaces": {"ip", "-br", "addr"}}
	rep := Collect("h", 1, r, cmds)
	if len(rep.Sections) != 1 {
		t.Fatal("expected 1 section")
	}
	if rep.Sections[0].Items == nil {
		// nil is acceptable? spec says empty slice; check not [""]
	}
	if len(rep.Sections[0].Items) != 0 {
		t.Fatalf("empty output must produce 0 items, got %d: %v", len(rep.Sections[0].Items), rep.Sections[0].Items)
	}
	// ensure not [""]
	for _, it := range rep.Sections[0].Items {
		if it == "" {
			t.Fatal("items must not contain empty string")
		}
	}
}

func TestDefaultCommands(t *testing.T) {
	secs := DefaultCommands()
	if len(secs) == 0 {
		t.Fatal("DefaultCommands must not be empty")
	}
	for _, s := range secs {
		if s.Name == "" || s.Title == "" {
			t.Fatalf("section missing name/title: %+v", s)
		}
	}
}
