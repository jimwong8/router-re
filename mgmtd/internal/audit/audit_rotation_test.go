package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 审计文件达到上限必须轮转到 .1：路由器是 flash 存储，无上限增长等于
// 让任何能反复触发被审计接口的人把盘填满。
func TestLogRotatesAtLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")
	l := New(path)

	// 用大 detail 快速把文件顶到上限之上。
	big := strings.Repeat("x", 4096)
	n := 0
	for {
		if err := l.Log(Event{Action: "test.fill", Detail: big}); err != nil {
			t.Fatalf("Log failed: %v", err)
		}
		n++
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if fi.Size() >= maxFileBytes {
			break
		}
		if n > 10000 {
			t.Fatal("文件始终没到上限，轮转阈值可能没生效")
		}
	}
	// 再写一条触发轮转
	if err := l.Log(Event{Action: "test.after"}); err != nil {
		t.Fatalf("Log failed: %v", err)
	}
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("应生成轮转文件 %s.1: %v", path, err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if fi.Size() >= maxFileBytes {
		t.Fatalf("轮转后当前文件应重新变小，实际 %d 字节", fi.Size())
	}
	// 轮转文件里应该有内容
	rfi, _ := os.Stat(path + ".1")
	if rfi.Size() == 0 {
		t.Fatal("轮转文件不应为空")
	}
}

// 查询必须只看尾部窗口，内存与文件大小无关；同时最新一条要在最前面。
func TestQueryReadsTailWindowOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")
	l := New(path)

	// 造一个超过查询窗口、但仍低于轮转上限的文件，全部是旧事件。
	// （超过 maxFileBytes 会先触发轮转，就测不到尾部窗口了。）
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	filler := `{"ts":1,"actor":"old","action":"old.event","target":"/x","result":"ok","detail":"","remote_ip":"1.1.1.1"}` + "\n"
	lineLen := int64(len(filler))
	want := int64(maxQueryBytes+1<<20)/lineLen + 10
	if maxQueryBytes+1<<20 >= maxFileBytes {
		t.Fatal("测试假设不成立：查询窗口加 1MB 必须仍小于轮转上限")
	}
	for i := int64(0); i < want; i++ {
		if _, err := f.WriteString(filler); err != nil {
			t.Fatal(err)
		}
	}
	f.Close()

	// 追加一条新事件作为最新的
	if err := l.Log(Event{Action: "newest.event", Actor: "me"}); err != nil {
		t.Fatal(err)
	}

	evs, err := l.Query(10, "")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(evs) != 10 {
		t.Fatalf("应返回 10 条，实际 %d", len(evs))
	}
	if evs[0].Action != "newest.event" {
		t.Fatalf("第一条应是最新的 newest.event，实际 %q", evs[0].Action)
	}
}

// 超长 detail（>64KB）不能让查询整体失败——旧的 bufio.Scanner 会返回
// ErrTooLong，审计页白屏。
func TestQueryToleratesLongLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")
	l := New(path)

	huge := strings.Repeat("A", 200*1024)
	if err := l.Log(Event{Action: "big.event", Detail: huge}); err != nil {
		t.Fatal(err)
	}
	if err := l.Log(Event{Action: "small.event"}); err != nil {
		t.Fatal(err)
	}

	evs, err := l.Query(10, "")
	if err != nil {
		t.Fatalf("超长行不应让查询失败: %v", err)
	}
	if len(evs) != 2 {
		t.Fatalf("应返回 2 条，实际 %d", len(evs))
	}
	if evs[0].Action != "small.event" {
		t.Fatalf("第一条应是最新事件，实际 %q", evs[0].Action)
	}
	if len(evs[1].Detail) != len(huge) {
		t.Fatalf("超长 detail 应完整保留，得到 %d 字节", len(evs[1].Detail))
	}
}

// 环形缓冲在“命中数远超 limit”时仍要给出正确的最后 limit 条与顺序。
func TestQueryRingOrder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")
	l := New(path)

	const total = 50
	for i := 0; i < total; i++ {
		b, _ := json.Marshal(Event{TS: int64(i), Action: "seq", Target: string(rune('a' + i%26))})
		f, _ := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		f.Write(append(b, '\n'))
		f.Close()
	}

	evs, err := l.Query(5, "seq")
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 5 {
		t.Fatalf("应返回 5 条，实际 %d", len(evs))
	}
	// 最新在前：ts 应是 49,48,47,46,45
	for i, want := range []int64{49, 48, 47, 46, 45} {
		if evs[i].TS != want {
			t.Fatalf("第 %d 条应 ts=%d，实际 %d（顺序错误）", i, want, evs[i].TS)
		}
	}
}
