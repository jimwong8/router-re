package sms

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// captureGateway 记录上一次请求并按预设状态码响应，用于单元测试。
type captureGateway struct {
	mu       sync.Mutex
	lastReq  *http.Request
	lastBody string
	status   int
	failNext bool // 下一次请求返回 500 后自恢复
}

func (g *captureGateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	g.lastReq = r
	body := make([]byte, r.ContentLength)
	if r.ContentLength > 0 {
		r.Body.Read(body)
	}
	g.lastBody = string(body)
	status := g.status
	g.mu.Unlock()

	if g.failNext {
		g.failNext = false
		status = http.StatusInternalServerError
	}
	w.WriteHeader(status)
	w.Write([]byte(`{"ok":true}`))
}

func (g *captureGateway) lastPhone() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return strings.TrimPrefix(strings.TrimSpace(g.lastBody), "phone=")
}

func TestGenericHTTPProviderSendSuccess(t *testing.T) {
	gw := &captureGateway{status: http.StatusOK}
	srv := httptest.NewServer(gw)
	defer srv.Close()

	p := NewGenericHTTPProvider(srv.URL, "POST", `{"phone":"{phone}","content":"{content}"}`, nil)
	if err := p.Send("13800138000", "code 123456"); err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if !strings.Contains(gw.lastBody, "13800138000") || !strings.Contains(gw.lastBody, "code 123456") {
		t.Errorf("body missing phone/content: %q", gw.lastBody)
	}
	if got := gw.lastReq.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("unexpected content-type %q", got)
	}
}

func TestGenericHTTPProviderSendGateway500(t *testing.T) {
	gw := &captureGateway{status: http.StatusOK, failNext: true}
	srv := httptest.NewServer(gw)
	defer srv.Close()

	p := NewGenericHTTPProvider(srv.URL, "POST", `{"phone":"{phone}","content":"{content}"}`, nil)
	err := p.Send("13800138000", "code 123456")
	if err == nil {
		t.Fatal("expected error for gateway 500")
	}
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected HTTPError 500, got %v", err)
	}
}

func TestGenericHTTPProviderDefaultMethodAndTimeout(t *testing.T) {
	gw := &captureGateway{status: http.StatusOK}
	srv := httptest.NewServer(gw)
	defer srv.Close()

	p := NewGenericHTTPProvider(srv.URL, "", `{"phone":"{phone}"}`, nil)
	if p.Method != http.MethodPost {
		t.Errorf("expected default POST, got %q", p.Method)
	}
	if p.client.Timeout != 5*time.Second {
		t.Errorf("expected 5s timeout, got %v", p.client.Timeout)
	}
}

func TestMemoryStoreCanSendRateLimit(t *testing.T) {
	s := NewMemoryStore()
	s.Now = func() time.Time { return time.Unix(1000, 0) }

	if _, ok := s.CanSend("13800138000"); !ok {
		t.Fatal("first send should be allowed")
	}
	s.SaveCode("13800138000", "123456")

	// 60 秒内不得再发
	retry, ok := s.CanSend("13800138000")
	if ok {
		t.Fatal("should be rate limited within 60s")
	}
	if retry <= 0 || retry > time.Minute {
		t.Errorf("unexpected retryAfter %v", retry)
	}

	// 超过 60 秒可再发
	s.Now = func() time.Time { return time.Unix(1061, 0) }
	if _, ok := s.CanSend("13800138000"); !ok {
		t.Fatal("after 60s should be allowed again")
	}
}

func TestMemoryStoreMaxPerHour(t *testing.T) {
	s := NewMemoryStore()
	base := time.Unix(2000, 0)
	s.Now = func() time.Time { return base }

	// 5 次在同一小时内允许（相邻发送间隔拉超过 60s，避开 60s 限发规则）
	for i := 0; i < maxPerHour; i++ {
		s.Now = func() time.Time { return base.Add(time.Duration(i)*61*time.Second + time.Second) }
		if _, ok := s.CanSend("13800138000"); !ok {
			t.Fatalf("send %d should be allowed", i+1)
		}
		s.SaveCode("13800138000", "000000")
	}
	// 第 6 次应被每小时限流（同一小时内，且已超过 60s 限发间隔）
	s.Now = func() time.Time { return base.Add(time.Duration(maxPerHour-1)*61*time.Second + 2*time.Second) }
	_, ok := s.CanSend("13800138000")
	if ok {
		t.Fatal("6th send in same hour should be rate limited")
	}

	// 跨小时恢复
	s.Now = func() time.Time { return base.Add(time.Hour + time.Second) }
	if _, ok := s.CanSend("13800138000"); !ok {
		t.Fatal("after new hour should be allowed again")
	}
}

func TestMemoryStoreVerifySuccessInvalidates(t *testing.T) {
	s := NewMemoryStore()
	s.Now = func() time.Time { return time.Unix(3000, 0) }
	s.SaveCode("13800138000", "123456")

	ok, _ := s.Verify("13800138000", "123456")
	if !ok {
		t.Fatal("valid code should pass")
	}
	// 验证成功即作废
	ok, reason := s.Verify("13800138000", "123456")
	if ok || reason != "not_found" {
		t.Errorf("code should be invalidated after use, got ok=%v reason=%q", ok, reason)
	}
}

func TestMemoryStoreVerifyWrongCount(t *testing.T) {
	s := NewMemoryStore()
	s.Now = func() time.Time { return time.Unix(4000, 0) }
	s.SaveCode("13800138000", "123456")

	// 错一次：提示 mismatch，但正确码仍然可用（错码仅计数，未达到上限）
	ok, reason := s.Verify("13800138000", "000000")
	if ok || reason != "mismatch" {
		t.Errorf("expected mismatch, got ok=%v reason=%q", ok, reason)
	}
	ok, _ = s.Verify("13800138000", "123456")
	if !ok {
		t.Fatal("right code after one wrong should still pass (wrong counted but under limit)")
	}
	// 重新存一码，再连续错 maxWrong 次达到上限，验证码被作废
	s.SaveCode("13800138000", "123456")
	for i := 0; i < maxWrong; i++ {
		s.Verify("13800138000", "000000")
	}
	ok, reason = s.Verify("13800138000", "123456")
	if ok || reason != "not_found" {
		t.Errorf("after max wrong, code should be invalidated, got ok=%v reason=%q", ok, reason)
	}
}

func TestMemoryStoreVerifyExpired(t *testing.T) {
	s := NewMemoryStore()
	s.Now = func() time.Time { return time.Unix(5000, 0) }
	s.SaveCode("13800138000", "123456")

	// 时钟推过 5 分钟 TTL
	s.Now = func() time.Time { return time.Unix(5000+301, 0) }
	ok, reason := s.Verify("13800138000", "123456")
	if ok || reason != "expired" {
		t.Errorf("expected expired, got ok=%v reason=%q", ok, reason)
	}
}

func TestMemoryStoreCodeFormat(t *testing.T) {
	s := NewMemoryStore()
	seen := make(map[int]bool)
	for i := 0; i < 1000; i++ {
		c := s.GenerateCode()
		if c < 0 || c >= 1000000 {
			t.Errorf("code out of range: %d", c)
		}
		seen[c] = true
	}
	if len(seen) < 10 {
		t.Errorf("generateCode too repetitive, distinct=%d", len(seen))
	}
}
