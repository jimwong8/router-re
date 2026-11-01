package cloud

import (
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// Frame 是云端 ↔ 设备之间的一帧。连接上是 NDJSON（一行一帧）。
type Frame struct {
	Type   string          `json:"type"`             // exec | ping | result | hello
	ID     string          `json:"id,omitempty"`     // exec/result 关联用
	Method string          `json:"method,omitempty"` // exec
	Path   string          `json:"path,omitempty"`
	Body   string          `json:"body,omitempty"`  // exec: 原始请求体（base64）
	Proxy  bool            `json:"proxy,omitempty"` // exec: 代理模式，跳过设备侧策略检查
	Status int             `json:"status,omitempty"`
	Data   string          `json:"data,omitempty"` // result: 原始响应体（base64）
	Error  string          `json:"error,omitempty"`
	TS     int64           `json:"ts,omitempty"`
	Meta   json.RawMessage `json:"meta,omitempty"` // hello: 设备元数据
}

// ErrOffline 表示设备当前没有隧道连接。
var ErrOffline = errors.New("device offline")

// ErrTimeout 表示设备在超时时间内没有回结果。
var ErrTimeout = errors.New("device timeout")

type conn struct {
	deviceID string
	out      chan Frame
	done     chan struct{}
	closeFn  func()
	once     sync.Once
	// lastActive 是最近一次成功写出/收到结果的 unix 纳秒时间。
	// 只有"连接对象存在"不足以证明设备还活着：TCP 半开时对端已经消失，
	// 但本地 socket 还能把数据收进发送缓冲区，于是 Online() 会长时间返回 true，
	// 运维看到的是"设备在线，命令全超时"。用它做活性判定。
	lastActive atomic.Int64
}

// touch 记录一次活性。
func (c *conn) touch() { c.lastActive.Store(time.Now().UnixNano()) }

// aliveFor 返回距上次活性的时长。
func (c *conn) aliveFor() time.Duration {
	ns := c.lastActive.Load()
	if ns == 0 {
		return 0
	}
	return time.Since(time.Unix(0, ns))
}

// close 保证 done 只关一次。设备重连时 Attach 关旧连接、旧连接的 handler 退出时
// Detach 又关一次——不加 once 就是 close of closed channel 的 panic。
func (c *conn) close() {
	c.once.Do(func() {
		close(c.done)
		if c.closeFn != nil {
			c.closeFn()
		}
	})
}

// Hub 维护所有在线设备的隧道连接，并做请求/响应关联。
type Hub struct {
	mu    sync.RWMutex
	conns map[string]*conn
	pend  map[string]chan Frame
}

// NewHub 创建 Hub。
func NewHub() *Hub {
	return &Hub{
		conns: make(map[string]*conn),
		pend:  make(map[string]chan Frame),
	}
}

// Attach 挂上一条设备隧道。同一设备重复连接时，旧连接会被踢掉
// （否则设备重连后云端还往死连接发请求，表现为"设备在线但命令超时"）。
func (h *Hub) Attach(deviceID string, closeFn func()) *conn {
	c := &conn{
		deviceID: deviceID,
		out:      make(chan Frame, 32),
		done:     make(chan struct{}),
		closeFn:  closeFn,
	}
	c.touch()
	h.mu.Lock()
	old := h.conns[deviceID]
	h.conns[deviceID] = c
	h.mu.Unlock()
	if old != nil {
		old.close()
	}
	return c
}

// Detach 摘掉连接（只有当前连接才生效，避免旧连接误删新连接）。
func (h *Hub) Detach(c *conn) {
	h.mu.Lock()
	if h.conns[c.deviceID] == c {
		delete(h.conns, c.deviceID)
	}
	h.mu.Unlock()
	c.close()
}

// connStaleAfter 是判定连接"僵死"的阈值。
// 云端每 20s 发一次 ping，设备收到也会回 report；超过这个窗口没有任何
// 活性就认为连接不可用，宁可报离线让用户重连，也不要让命令卡满超时。
const connStaleAfter = 75 * time.Second

// Online 判断设备是否有可用连接（存在且近期有活性）。
func (h *Hub) Online(deviceID string) bool {
	h.mu.RLock()
	c := h.conns[deviceID]
	h.mu.RUnlock()
	if c == nil {
		return false
	}
	return c.aliveFor() <= connStaleAfter
}

// OnlineIDs 返回所有在线设备 ID。
func (h *Hub) OnlineIDs() map[string]bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make(map[string]bool, len(h.conns))
	for id, c := range h.conns {
		if c.aliveFor() <= connStaleAfter {
			out[id] = true
		}
	}
	return out
}

// Send 向设备推一帧，带超时（设备卡住时不能把云端 goroutine 拖死）。
func (h *Hub) Send(deviceID string, f Frame, timeout time.Duration) error {
	h.mu.RLock()
	c := h.conns[deviceID]
	h.mu.RUnlock()
	if c == nil {
		return ErrOffline
	}
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case c.out <- f:
		return nil
	case <-t.C:
		return ErrTimeout
	case <-c.done:
		return ErrOffline
	}
}

// Call 下发一条 exec 并等待结果。
func (h *Hub) Call(deviceID string, f Frame, timeout time.Duration) (Frame, error) {
	if f.ID == "" {
		f.ID = randHex(8)
	}
	ch := make(chan Frame, 1)
	h.mu.Lock()
	h.pend[f.ID] = ch
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.pend, f.ID)
		h.mu.Unlock()
	}()

	if err := h.Send(deviceID, f, 3*time.Second); err != nil {
		return Frame{}, err
	}
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case res := <-ch:
		return res, nil
	case <-t.C:
		return Frame{}, ErrTimeout
	}
}

// Deliver 把设备回来的结果投递给等待方。没人等就丢弃（设备重试/超时后的迟到结果）。
func (h *Hub) Deliver(f Frame) bool {
	h.mu.RLock()
	ch := h.pend[f.ID]
	h.mu.RUnlock()
	if ch == nil {
		return false
	}
	select {
	case ch <- f:
		return true
	default:
		return false
	}
}

// Touch 由隧道 handler 在成功写出一帧后调用，刷新该连接的活性时间。
func (h *Hub) Touch(c *conn) {
	if c != nil {
		c.touch()
	}
}

// TouchID 按设备 ID 刷新活性（设备回传执行结果时用）。
func (h *Hub) TouchID(deviceID string) {
	h.mu.RLock()
	c := h.conns[deviceID]
	h.mu.RUnlock()
	if c != nil {
		c.touch()
	}
}
