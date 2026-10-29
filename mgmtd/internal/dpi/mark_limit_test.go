package dpi

import (
	"strings"
	"testing"
)

func TestRenderMarkNftLimit(t *testing.T) {
	out, err := RenderMarkNft([]MarkRule{{App: "x", Protocol: "tcp", DPorts: []int{80}, Action: "limit", RateMbit: 20}})
	if err != nil { t.Fatal(err) }
	if !strings.Contains(out, "limit rate 20 mbytes/second") {
		t.Fatalf("missing limit rate: %s", out)
	}
	// mark/drop unchanged
	o2,_ := RenderMarkNft([]MarkRule{{App:"y",Protocol:"tcp",DPorts:[]int{443},Action:"mark",Mark:7}})
	if !strings.Contains(o2,"meta mark set 7") { t.Fatal("mark broken") }
}
