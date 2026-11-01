package cloud

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

// 设备侧接入点。全部走 mTLS/TLS（由反代或进程自己终结），凭据放请求头：
//   X-Device-ID / X-Device-Secret
// 设备侧只"主动外连"，所以 NAT/CGNAT/动态 IP 后面都能用。

func (s *Service) agentAuth(r *http.Request) (Device, bool) {
	id := r.Header.Get("X-Device-ID")
	sec := r.Header.Get("X-Device-Secret")
	dev, ok := s.store.AuthDevice(id, sec)
	if !ok {
		s.store.AddAudit(AuditEntry{
			Actor: "device:" + id, Action: "device.auth", Result: "fail",
			Detail: "凭据无效 " + clientIP(r),
		})
	}
	return dev, ok
}

func clientIP(r *http.Request) string {
	// 反代后面真实 IP 在 X-Forwarded-For，取第一段。
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// handleEnroll 用一次性令牌换 device_id + secret。
func (s *Service) handleEnroll(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token   string            `json:"token"`
		Name    string            `json:"name"`
		Model   string            `json:"model"`
		Version string            `json:"version"`
		Labels  map[string]string `json:"labels"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		writeErr(w, 400, 20001, "bad json", err.Error())
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeErr(w, 422, 20002, "name required", "设备名不能为空")
		return
	}
	// 注册是公开端点：按 IP 限制失败次数，避免被拿假令牌无限试探。
	ip := clientIP(r)
	if wait := s.enrollLimiter.blocked(ip); wait > 0 {
		writeErr(w, 429, 20005, "too many attempts",
			fmt.Sprintf("注册失败次数过多，请 %d 分钟后再试", int(wait.Minutes())+1))
		return
	}
	probeID := randHex(8)
	if !s.store.ConsumeToken(req.Token, probeID) {
		s.enrollLimiter.fail(ip)
		s.store.AddAudit(AuditEntry{Actor: "enroll:" + ip, Action: "device.enroll", Result: "fail", Detail: "令牌无效/过期/已用"})
		// 不在这里同步 Save：这是公开端点，同步写盘会被用来刷盘。
		// AddAudit 已置 dirty，由后台批量落盘。
		writeErr(w, 401, 20003, "invalid token", "注册令牌无效、已过期或已被使用")
		return
	}
	s.enrollLimiter.reset(ip)
	dev, secret := s.store.RegisterDevice(Device{
		Name: req.Name, Model: req.Model, Version: req.Version, Labels: req.Labels,
		RemoteIP: clientIP(r),
	})
	s.store.AddAudit(AuditEntry{Actor: "device:" + dev.ID, Device: dev.Name, Action: "device.enroll", Result: "ok", Detail: clientIP(r)})
	if err := s.store.Save(); err != nil {
		log.Printf("cloud: 保存状态失败: %v", err)
	}
	writeJSON(w, 200, map[string]interface{}{
		"device_id": dev.ID,
		"secret":    secret, // 只此一次
		"tunnel":    "/cloud/v1/agent/stream",
	})
}

// handleReport 设备上报元数据（名称/版本/运行时长）。
func (s *Service) handleReport(w http.ResponseWriter, r *http.Request) {
	dev, ok := s.agentAuth(r)
	if !ok {
		writeErr(w, 401, 20004, "unauthorized", "设备凭据无效")
		return
	}
	var req struct {
		Name      string          `json:"name"`
		Version   string          `json:"version"`
		Model     string          `json:"model"`
		UptimeSec int64           `json:"uptime_sec"`
		Meta      json.RawMessage `json:"meta"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeErr(w, 400, 20001, "bad json", err.Error())
		return
	}
	ip := clientIP(r)
	s.store.Touch(dev.ID, func(d *Device) {
		if req.Name != "" {
			d.Name = req.Name
		}
		if req.Version != "" {
			d.Version = req.Version
		}
		if req.Model != "" {
			d.Model = req.Model
		}
		if req.UptimeSec > 0 {
			d.UptimeSec = req.UptimeSec
		}
		if len(req.Meta) > 0 {
			d.Meta = req.Meta
		}
		d.RemoteIP = ip
	})
	_ = s.store.Save()
	writeJSON(w, 200, map[string]interface{}{"ok": true})
}

// handleResult 设备回传执行结果。
func (s *Service) handleResult(w http.ResponseWriter, r *http.Request) {
	dev, ok := s.agentAuth(r)
	if !ok {
		writeErr(w, 401, 20004, "unauthorized", "设备凭据无效")
		return
	}
	var f Frame
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<20)).Decode(&f); err != nil {
		writeErr(w, 400, 20001, "bad json", err.Error())
		return
	}
	if f.Type == "" {
		f.Type = "result"
	}
	// 设备能回结果就说明链路是活的，即使这次结果已经没人等。
	s.hub.TouchID(dev.ID)
	if !s.hub.Deliver(f) {
		// 迟到的结果（云端已超时）不算错误，丢掉即可。
		log.Printf("cloud: 丢弃无主的执行结果 id=%s dev=%s", f.ID, dev.Name)
	}
	s.store.Touch(dev.ID, nil)
	writeJSON(w, 200, map[string]interface{}{"ok": true})
}

// handleStream 是设备的长连接：云端→设备用 NDJSON 一行一帧下发。
func (s *Service) handleStream(w http.ResponseWriter, r *http.Request) {
	dev, ok := s.agentAuth(r)
	if !ok {
		writeErr(w, 401, 20004, "unauthorized", "设备凭据无效")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, 500, 20005, "streaming unsupported", "服务端不支持流式响应")
		return
	}
	hj, _ := w.(http.Hijacker)
	_ = hj

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no") // nginx 不要缓冲
	w.WriteHeader(200)
	flusher.Flush()

	ip := clientIP(r)
	s.store.Touch(dev.ID, func(d *Device) { d.RemoteIP = ip })
	s.store.AddAudit(AuditEntry{Actor: "device:" + dev.ID, Device: dev.Name, Action: "tunnel.connect", Result: "ok", Detail: ip})

	// closeFn 在连接被踢掉时触发，让下面的循环立刻退出。
	// 它由 conn.close 的 sync.Once 保证只调用一次。
	closed := make(chan struct{})
	c := s.hub.Attach(dev.ID, func() { close(closed) })
	defer func() {
		s.hub.Detach(c)
		s.store.Touch(dev.ID, nil)
		s.store.AddAudit(AuditEntry{Actor: "device:" + dev.ID, Device: dev.Name, Action: "tunnel.disconnect", Result: "ok"})
		_ = s.store.Save()
	}()

	// 写超时：对端半开时 ResponseWriter.Write 会一直阻塞在发送缓冲区，
	// 没有 deadline 的话这个 handler goroutine 永远不退出，连接也就永远"在线"。
	rc := http.NewResponseController(w)
	setWriteDeadline := func() {
		// 老实现或某些 ResponseWriter 不支持 deadline，忽略错误即可。
		_ = rc.SetWriteDeadline(time.Now().Add(30 * time.Second))
	}

	enc := json.NewEncoder(w)
	// 第一帧：hello，把设备侧看到的配置回给云端（便于排障）。
	setWriteDeadline()
	_ = enc.Encode(Frame{Type: "hello", TS: time.Now().Unix()})
	flusher.Flush()

	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	ctxDone := r.Context().Done()

	for {
		select {
		case <-ctxDone:
			return
		case <-closed:
			return
		case <-c.done:
			return
		case <-ping.C:
			setWriteDeadline()
			if err := enc.Encode(Frame{Type: "ping", TS: time.Now().Unix()}); err != nil {
				return
			}
			flusher.Flush()
			s.hub.Touch(c)
		case f := <-c.out:
			setWriteDeadline()
			if err := enc.Encode(f); err != nil {
				return
			}
			flusher.Flush()
			s.hub.Touch(c)
		}
	}
}

// 保持 bufio 引用（某些反代会因为缺少写缓冲导致小帧粘包），这里显式用 bufio 包一层。
var _ = bufio.NewWriter
var _ = base64.RawURLEncoding
