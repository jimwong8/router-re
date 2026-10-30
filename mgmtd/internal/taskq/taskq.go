// Package taskq is a single-process in-memory task queue with disk persistence.
//
// It limits concurrency (semaphore) and retention (max tasks), supports
// cancel, and persists state to a JSON file so it survives restarts.
package taskq

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// State is the lifecycle state of a task.
type State string

const (
	StatePending   State = "pending"
	StateRunning   State = "running"
	StateDone      State = "done"
	StateFailed    State = "failed"
	StateCancelled State = "cancelled"
)

// Task is one queued job.
type Task struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	Params     json.RawMessage `json:"params"`
	State      State           `json:"state"`
	Progress   int             `json:"progress"`
	Result     any             `json:"result,omitempty"`
	Error      string          `json:"error,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
	StartedAt  *time.Time      `json:"started_at,omitempty"`
	FinishedAt *time.Time      `json:"finished_at,omitempty"`
}

// Handler executes a task.
type Handler func(ctx context.Context, params json.RawMessage, progress func(int)) (any, error)

// Queue is the task queue.
type Queue struct {
	mu         sync.Mutex
	tasks      map[string]*Task
	order      []string
	handlers   map[string]Handler
	maxWorkers int
	maxTasks   int
	sem        chan struct{}
	storePath  string
	cancelFns  map[string]context.CancelFunc
	seq        int64
	// loadFailed 表示启动时持久化文件读不出来。为 true 时拒绝回写，
	// 防止"读失败 + 回写空"把队列真的清空。
	loadFailed bool
}

// New creates a queue. storePath may be "" (no persistence).
func New(storePath string, maxWorkers, maxTasks int) *Queue {
	if maxWorkers < 1 {
		maxWorkers = 1
	}
	if maxTasks < 1 {
		maxTasks = 100
	}
	q := &Queue{
		tasks:      map[string]*Task{},
		handlers:   map[string]Handler{},
		maxWorkers: maxWorkers,
		maxTasks:   maxTasks,
		sem:        make(chan struct{}, maxWorkers),
		storePath:  storePath,
		cancelFns:  map[string]context.CancelFunc{},
	}
	q.load()
	return q
}

// Register adds a handler for a task type.
func (q *Queue) Register(typ string, h Handler) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.handlers[typ] = h
}

// Submit enqueues a task and starts it if capacity allows.
func (q *Queue) Submit(typ string, params json.RawMessage) (*Task, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if _, ok := q.handlers[typ]; !ok {
		return nil, fmt.Errorf("unknown task type %q", typ)
	}
	q.seq++
	id := fmt.Sprintf("task-%d", q.seq)
	now := time.Now()
	t := &Task{
		ID:        id,
		Type:      typ,
		Params:    params,
		State:     StatePending,
		CreatedAt: now,
	}
	q.tasks[id] = t
	q.order = append(q.order, id)
	q.evictLocked()
	q.saveLocked()
	go q.run(id)
	return cloneTask(t), nil
}

func (q *Queue) run(id string) {
	// acquire semaphore first (blocks until a worker slot is free)
	q.sem <- struct{}{}
	defer func() { <-q.sem }()

	q.mu.Lock()
	t, ok := q.tasks[id]
	if !ok {
		q.mu.Unlock()
		return
	}
	if t.State == StateCancelled {
		q.mu.Unlock()
		return
	}
	t.State = StateRunning
	now := time.Now()
	t.StartedAt = &now
	h := q.handlers[t.Type]
	ctx, cancel := context.WithCancel(context.Background())
	q.cancelFns[id] = cancel
	q.saveLocked()
	q.mu.Unlock()

	progress := func(p int) {
		if p < 0 {
			p = 0
		}
		if p > 100 {
			p = 100
		}
		q.mu.Lock()
		if tt, ok := q.tasks[id]; ok && tt.State == StateRunning {
			tt.Progress = p
			q.saveLocked()
		}
		q.mu.Unlock()
	}

	res, err := h(ctx, t.Params, progress)

	q.mu.Lock()
	defer q.mu.Unlock()
	t, ok = q.tasks[id]
	if !ok {
		return
	}
	delete(q.cancelFns, id)
	cancel()
	now2 := time.Now()
	t.FinishedAt = &now2
	if err != nil {
		if ctx.Err() != nil {
			t.State = StateCancelled
		} else {
			t.State = StateFailed
			t.Error = err.Error()
		}
	} else {
		t.State = StateDone
		t.Result = res
		t.Progress = 100
	}
	q.saveLocked()
}

// Get returns a copy of a task.
func (q *Queue) Get(id string) (Task, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	t, ok := q.tasks[id]
	if !ok {
		return Task{}, false
	}
	return *cloneTask(t), true
}

// List returns tasks filtered by state/type, with pagination.
func (q *Queue) List(state, typ string, offset, limit int) ([]Task, int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	var ids []string
	for _, id := range q.order {
		t := q.tasks[id]
		if t == nil {
			continue
		}
		if state != "" && string(t.State) != state {
			continue
		}
		if typ != "" && t.Type != typ {
			continue
		}
		ids = append(ids, id)
	}
	total := len(ids)
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	if limit <= 0 {
		limit = 50
	}
	end := offset + limit
	if end > total {
		end = total
	}
	out := make([]Task, 0, end-offset)
	for _, id := range ids[offset:end] {
		out = append(out, *cloneTask(q.tasks[id]))
	}
	return out, total
}

// Cancel marks a task cancelled.
func (q *Queue) Cancel(id string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	t, ok := q.tasks[id]
	if !ok {
		return fmt.Errorf("task %q not found", id)
	}
	if t.State == StateDone || t.State == StateFailed || t.State == StateCancelled {
		return fmt.Errorf("task %q already %s", id, t.State)
	}
	t.State = StateCancelled
	if cancel, ok := q.cancelFns[id]; ok {
		cancel()
	}
	now := time.Now()
	t.FinishedAt = &now
	q.saveLocked()
	return nil
}

func (q *Queue) evictLocked() {
	for len(q.order) > q.maxTasks {
		// remove oldest finished task
		evicted := false
		for i, id := range q.order {
			t := q.tasks[id]
			if t == nil {
				continue
			}
			if t.State == StateDone || t.State == StateFailed || t.State == StateCancelled {
				delete(q.tasks, id)
				q.order = append(q.order[:i], q.order[i+1:]...)
				evicted = true
				break
			}
		}
		if !evicted {
			// all pending/running; drop oldest anyway
			id := q.order[0]
			delete(q.tasks, id)
			q.order = q.order[1:]
		}
	}
}

func (q *Queue) saveLocked() {
	if q.storePath == "" {
		return
	}
	// 持久化文件读不出来时绝不回写：那会把"可能是好文件、只是这次没读到"
	// 变成"确定被空队列覆盖"，真的丢任务。宁可这次不保存，等人来修。
	if q.loadFailed {
		return
	}
	var tasks []*Task
	for _, id := range q.order {
		if t, ok := q.tasks[id]; ok {
			cp := *t
			tasks = append(tasks, &cp)
		}
	}
	data, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(q.storePath), 0o755)
	// 原子替换：先写临时文件再 rename。
	//
	// 直接 WriteFile 会先 truncate 再写，另一进程（或崩溃后的重启）在这个
	// 窗口里读到的是半截 JSON —— 解析失败就会被当成"没有任务"，等于把
	// 队列悄悄清空。rename 在 POSIX 上是原子的，读到的永远是完整版本。
	tmp := q.storePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return
	}
	if err := os.Rename(tmp, q.storePath); err != nil {
		_ = os.Remove(tmp)
	}
}

func (q *Queue) load() {
	if q.storePath == "" {
		return
	}
	data, err := os.ReadFile(q.storePath)
	if err != nil {
		return
	}
	var tasks []*Task
	if err := json.Unmarshal(data, &tasks); err != nil {
		// 文件存在但解析不了：备份一份再标记 loadFailed，后续 saveLocked 不写，
		// 避免用空队列覆盖掉可能还能人工恢复的内容。
		q.loadFailed = true
		if len(data) > 0 {
			_ = os.WriteFile(q.storePath+".corrupt", data, 0o600)
		}
		fmt.Fprintf(os.Stderr, "taskq: 持久化文件解析失败，本次不加载也不回写: %s: %v\n", q.storePath, err)
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, t := range tasks {
		q.tasks[t.ID] = t
		q.order = append(q.order, t.ID)
		// restore seq
		var n int64
		fmt.Sscanf(t.ID, "task-%d", &n)
		if n > q.seq {
			q.seq = n
		}
	}
	// sort order by created_at to be safe
	sort.SliceStable(q.order, func(i, j int) bool {
		return q.tasks[q.order[i]].CreatedAt.Before(q.tasks[q.order[j]].CreatedAt)
	})
}

func cloneTask(t *Task) *Task {
	cp := *t
	if t.Params != nil {
		cp.Params = append(json.RawMessage(nil), t.Params...)
	}
	if t.StartedAt != nil {
		v := *t.StartedAt
		cp.StartedAt = &v
	}
	if t.FinishedAt != nil {
		v := *t.FinishedAt
		cp.FinishedAt = &v
	}
	if t.Result != nil {
		// shallow copy is fine for JSON-serializable results
		cp.Result = t.Result
	}
	return &cp
}
