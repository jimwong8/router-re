package ha

import (
	"testing"

	"github.com/jimwong/router-re/mgmtd/internal/config"
)

func TestExportImportMergeReplace(t *testing.T) {
	st := config.NewMockStore()
	st.Set("sec", "a", "1")
	st.Set("sec", "b", "2")
	data, err := Export(st, []string{"sec.a", "sec.b"})
	if err != nil {
		t.Fatal(err)
	}
	if data["sec.a"] != "1" {
		t.Fatalf("export mismatch")
	}
	// merge: add c, keep b
	err = Import(st, map[string]string{"sec.c": "3"}, "merge", []string{"sec.a", "sec.b"})
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := st.Get("sec", "b"); v != "2" {
		t.Fatalf("merge deleted b")
	}
	if v, _ := st.Get("sec", "c"); v != "3" {
		t.Fatalf("merge not set c")
	}
	// replace: remove b
	err = Import(st, map[string]string{"sec.a": "1"}, "replace", []string{"sec.a", "sec.b", "sec.c"})
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := st.Get("sec", "b"); v != "" {
		t.Fatalf("replace did not delete b")
	}
}
