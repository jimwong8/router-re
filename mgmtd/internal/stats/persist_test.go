package stats

import (
	"os"
	"path/filepath"
	"testing"
)

func mkSample(ts int64, cpu float64) Sample {
	return Sample{TS: ts, CPUPercent: cpu, MemUsedMB: 100}
}

func TestSnapshotChronologicalAfterWrap(t *testing.T) {
	r := NewRing(3)
	for i := int64(1); i <= 5; i++ {
		r.Add(mkSample(i, float64(i)))
	}
	snap := r.Snapshot()
	if len(snap) != 3 {
		t.Fatalf("应保留 3 条，实际 %d", len(snap))
	}
	// 环形回绕后必须仍按时间升序，否则前端折线会画成来回折返
	for i := 1; i < len(snap); i++ {
		if snap[i].TS <= snap[i-1].TS {
			t.Fatalf("顺序错误: %+v", snap)
		}
	}
	if snap[0].TS != 3 || snap[2].TS != 5 {
		t.Fatalf("保留的区间错误: %d..%d", snap[0].TS, snap[2].TS)
	}
}

func TestSnapshotNotYetFull(t *testing.T) {
	r := NewRing(10)
	r.Add(mkSample(1, 1))
	r.Add(mkSample(2, 2))
	if got := r.Snapshot(); len(got) != 2 || got[0].TS != 1 {
		t.Fatalf("未满时顺序错误: %+v", got)
	}
}

func TestLoadSortsAndTrims(t *testing.T) {
	r := NewRing(3)
	// 故意乱序输入
	r.Load([]Sample{mkSample(3, 3), mkSample(1, 1), mkSample(2, 2), mkSample(4, 4)})
	snap := r.Snapshot()
	if len(snap) != 3 {
		t.Fatalf("应只保留 3 条，实际 %d", len(snap))
	}
	if snap[0].TS != 2 || snap[2].TS != 4 {
		t.Fatalf("应保留最新的 3 条: %+v", snap)
	}
}

func TestLoadDedupsSameTimestamp(t *testing.T) {
	r := NewRing(10)
	r.Load([]Sample{mkSample(1, 1), mkSample(1, 2), mkSample(2, 3)})
	snap := r.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("重复时间戳应去重，实际 %d: %+v", len(snap), snap)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "history.json")
	r := NewRing(5)
	for i := int64(1); i <= 3; i++ {
		r.Add(mkSample(i*10, float64(i)))
	}
	if err := r.SaveFile(path); err != nil {
		t.Fatal(err)
	}

	r2, n, err := LoadFile(path, 5)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("应恢复 3 条，实际 %d", n)
	}
	snap := r2.Snapshot()
	if len(snap) != 3 || snap[0].TS != 10 || snap[2].TS != 30 {
		t.Fatalf("恢复内容错误: %+v", snap)
	}
}

func TestLoadFileMissingIsNotError(t *testing.T) {
	// 首次启动没有历史文件是正常状态
	r, n, err := LoadFile(filepath.Join(t.TempDir(), "nope.json"), 10)
	if err != nil {
		t.Fatalf("文件不存在不该报错: %v", err)
	}
	if n != 0 || len(r.Snapshot()) != 0 {
		t.Fatal("应返回空 ring")
	}
}

func TestLoadFileCorruptDoesNotCrash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	os.WriteFile(path, []byte("{not json"), 0o644)
	r, n, err := LoadFile(path, 10)
	if err == nil {
		t.Fatal("损坏文件应给出错误提示（但不应导致启动失败）")
	}
	if r == nil || n != 0 {
		t.Fatal("应返回可用的空 ring")
	}
}

func TestSaveFileAtomicNoTempLeft(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "h.json")
	r := NewRing(2)
	r.Add(mkSample(1, 1))
	if err := r.SaveFile(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("临时文件应已被 rename，不该残留")
	}
}

func TestPersistPreservesOrderAcrossRestart(t *testing.T) {
	// 模拟：跑满环绕 → 落盘 → 重启恢复，时间顺序必须仍然正确
	dir := t.TempDir()
	path := filepath.Join(dir, "h.json")
	r := NewRing(4)
	for i := int64(1); i <= 7; i++ {
		r.Add(mkSample(i, float64(i)))
	}
	if err := r.SaveFile(path); err != nil {
		t.Fatal(err)
	}
	r2, _, err := LoadFile(path, 4)
	if err != nil {
		t.Fatal(err)
	}
	snap := r2.Snapshot()
	want := []int64{4, 5, 6, 7}
	if len(snap) != len(want) {
		t.Fatalf("条数错误: %d", len(snap))
	}
	for i, ts := range want {
		if snap[i].TS != ts {
			t.Fatalf("第 %d 条应为 %d，实际 %d (%+v)", i, ts, snap[i].TS, snap)
		}
	}
}
