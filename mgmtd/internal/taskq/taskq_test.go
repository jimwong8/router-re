package taskq

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"
)

func newQ(t *testing.T) *Queue {
	t.Helper()
	q := New("", 2, 5)
	q.Register("echo", func(ctx context.Context, params json.RawMessage, progress func(int)) (any, error) {
		return params, nil
	})
	q.Register("sleep", func(ctx context.Context, params json.RawMessage, progress func(int)) (any, error) {
		var p struct {
			Seconds int `json:"seconds"`
		}
		_ = json.Unmarshal(params, &p)
		if p.Seconds <= 0 {
			p.Seconds = 1
		}
		for i := 0; i < p.Seconds; i++ {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(50 * time.Millisecond):
			}
			progress((i + 1) * 100 / p.Seconds)
		}
		return map[string]any{"slept": p.Seconds}, nil
	})
	q.Register("fail", func(ctx context.Context, params json.RawMessage, progress func(int)) (any, error) {
		return nil, fmt.Errorf("boom")
	})
	return q
}

func TestSubmitAndGet(t *testing.T) {
	q := newQ(t)
	task, err := q.Submit("echo", json.RawMessage(`{"msg":"hi"}`))
	if err != nil {
		t.Fatal(err)
	}
	if task.ID == "" || task.State != StatePending {
		t.Fatalf("bad task: %+v", task)
	}
	// wait for completion
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		got, ok := q.Get(task.ID)
		if ok && (got.State == StateDone || got.State == StateFailed) {
			if got.State != StateDone {
				t.Fatalf("state = %s, want done", got.State)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("task did not complete")
}

func TestListFilter(t *testing.T) {
	q := newQ(t)
	t1, _ := q.Submit("echo", json.RawMessage(`{}`))
	t2, _ := q.Submit("echo", json.RawMessage(`{}`))
	_ = t1
	_ = t2
	// wait
	time.Sleep(200 * time.Millisecond)
	tasks, total := q.List("", "", 0, 50)
	_ = tasks
	if total != 2 {
		t.Fatalf("total = %d, want 2", total)
	}
	tasks2, total2 := q.List("done", "", 0, 50)
	if total2 != 2 {
		t.Fatalf("done total = %d, want 2 (got %d tasks)", total2, len(tasks2))
	}
}

func TestCancel(t *testing.T) {
	q := newQ(t)
	task, _ := q.Submit("sleep", json.RawMessage(`{"seconds":10}`))
	// cancel quickly
	time.Sleep(30 * time.Millisecond)
	if err := q.Cancel(task.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	got, _ := q.Get(task.ID)
	if got.State != StateCancelled {
		t.Fatalf("state = %s, want cancelled", got.State)
	}
	// cancel again should error
	if err := q.Cancel(task.ID); err == nil {
		t.Fatal("expected error on double cancel")
	}
}

func TestUnknownType(t *testing.T) {
	q := newQ(t)
	if _, err := q.Submit("nope", json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected error for unknown type")
	}
}

func TestConcurrencyLimit(t *testing.T) {
	q := New("", 1, 100) // 1 worker
	q.Register("sleep", func(ctx context.Context, params json.RawMessage, progress func(int)) (any, error) {
		time.Sleep(200 * time.Millisecond)
		return nil, nil
	})
	q.Submit("sleep", json.RawMessage(`{}`))
	q.Submit("sleep", json.RawMessage(`{}`))
	time.Sleep(50 * time.Millisecond)
	// with 1 worker, second should still be pending
	tasks, _ := q.List("pending", "", 0, 50)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 pending (concurrency=1), got %d", len(tasks))
	}
	time.Sleep(400 * time.Millisecond)
	tasks2, _ := q.List("done", "", 0, 50)
	if len(tasks2) != 2 {
		t.Fatalf("expected 2 done, got %d", len(tasks2))
	}
}

func TestRetention(t *testing.T) {
	q := New("", 4, 2) // max 2 tasks
	q.Register("echo", func(ctx context.Context, params json.RawMessage, progress func(int)) (any, error) {
		return params, nil
	})
	q.Submit("echo", json.RawMessage(`{}`))
	time.Sleep(50 * time.Millisecond)
	q.Submit("echo", json.RawMessage(`{}`))
	time.Sleep(50 * time.Millisecond)
	q.Submit("echo", json.RawMessage(`{}`))
	time.Sleep(100 * time.Millisecond)
	tasks, total := q.List("", "", 0, 50)
	if total > 2 {
		t.Fatalf("retention violated: total=%d, want <=2", total)
	}
	_ = tasks
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/tasks.json"
	q := New(path, 2, 10)
	q.Register("echo", func(ctx context.Context, params json.RawMessage, progress func(int)) (any, error) {
		return params, nil
	})
	q.Submit("echo", json.RawMessage(`{"x":1}`))
	time.Sleep(100 * time.Millisecond)
	// reload
	q2 := New(path, 2, 10)
	q2.Register("echo", func(ctx context.Context, params json.RawMessage, progress func(int)) (any, error) {
		return params, nil
	})
	tasks, total := q2.List("", "", 0, 50)
	if total != 1 {
		t.Fatalf("persistence: expected 1 task after reload, got %d", total)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
}

var _ = time.Second

func TestPersistenceAtomicNoTempLeftover(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/tasks.json"
	q := New(path, 2, 10)
	q.Register("echo", func(ctx context.Context, params json.RawMessage, progress func(int)) (any, error) {
		return params, nil
	})
	if _, err := q.Submit("echo", json.RawMessage(`{"x":1}`)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	// 原子写：不该留下 .tmp 残file
	if _, err := os.Stat(path + ".tmp"); err == nil {
		t.Fatal("原子写不应留下 .tmp 文件")
	}
	// 落盘内容必须是完整 JSON
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("持久化文件缺失: %v", err)
	}
	var tasks []map[string]any
	if err := json.Unmarshal(data, &tasks); err != nil {
		t.Fatalf("持久化文件不是完整 JSON（可能被截断）: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("应有 1 个任务，实际 %d", len(tasks))
	}
}

func TestCorruptStoreNotOverwritten(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/tasks.json"
	bad := []byte(`[{"id":"task-1","type":"echo"`) // 截断的 JSON
	if err := os.WriteFile(path, bad, 0o600); err != nil {
		t.Fatal(err)
	}
	q := New(path, 2, 10)
	q.Register("echo", func(ctx context.Context, params json.RawMessage, progress func(int)) (any, error) {
		return nil, nil
	})
	// 解析失败：列表为空，这是可以接受的降级
	if _, total := q.List("", "", 0, 10); total != 0 {
		t.Fatalf("损坏文件应表现为空队列，实际 %d", total)
	}
	// 但绝不能用空队列覆盖原文件
	if _, err := q.Submit("echo", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	now, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(now) != string(bad) {
		t.Fatalf("损坏的持久化文件被覆盖了（数据丢失）：%s", string(now))
	}
	// 应保留一份 .corrupt 供人工恢复
	if _, err := os.Stat(path + ".corrupt"); err != nil {
		t.Fatalf("应备份 .corrupt: %v", err)
	}
}
