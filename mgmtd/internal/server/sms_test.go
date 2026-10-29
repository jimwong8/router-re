package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jimwong/router-re/mgmtd/internal/sms"
)

// smsMockGateway 记录上一次请求并按预设状态码响应，用于单元测试。
type smsMockGateway struct {
	mu       sync.Mutex
	status   int
	lastReq  *http.Request
	lastBody string
}

func (g *smsMockGateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	g.lastReq = r
	buf := make([]byte, r.ContentLength)
	if r.ContentLength > 0 {
		r.Body.Read(buf)
	}
	g.lastBody = string(buf)
	status := g.status
	g.mu.Unlock()
	w.WriteHeader(status)
	w.Write([]byte(`{"ok":true}`))
}

func newSMSTestServer(t *testing.T) (*Server, *smsMockGateway, string) {
	t.Helper()
	s := New(Config{Addr: ":0", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	gw := &smsMockGateway{status: http.StatusOK}
	srv := httptest.NewServer(gw)
	t.Cleanup(srv.Close)
	cfg := smsConfig{
		URL:      srv.URL,
		Method:   "POST",
		BodyTmpl: `{"phone":"{phone}","content":"{content}"}`,
		Headers:  nil,
	}
	b, _ := json.Marshal(cfg)
	if err := s.store.Set(sms.Section(), sms.Option(), string(b)); err != nil {
		t.Fatalf("store sms config: %v", err)
	}
	return s, gw, srv.URL
}

func smsLogin(t *testing.T, s *Server) string {
	t.Helper()
	lr := httptest.NewRecorder()
	s.ServeHTTP(lr, httptest.NewRequest("POST", "/api/v1/auth/login",
		strings.NewReader(`{"username":"admin","password":"admin123"}`)))
	if lr.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", lr.Code, lr.Body.String())
	}
	var resp struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(lr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	return resp.Token
}

func smsSend(t *testing.T, s *Server, token, phone string) *httptest.ResponseRecorder {
	t.Helper()
	wr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/auth/sms/send",
		strings.NewReader(`{"phone":"`+phone+`"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	s.ServeHTTP(wr, req)
	return wr
}

func smsVerify(t *testing.T, s *Server, token, phone, code string) *httptest.ResponseRecorder {
	t.Helper()
	wr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/auth/sms/verify",
		strings.NewReader(`{"phone":"`+phone+`","code":"`+code+`"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	s.ServeHTTP(wr, req)
	return wr
}

func TestSMSSendSuccess(t *testing.T) {
	s, gw, _ := newSMSTestServer(t)
	token := smsLogin(t, s)

	wr := smsSend(t, s, token, "13800138000")
	if wr.Code != http.StatusOK {
		t.Fatalf("send success expected 200, got %d %s", wr.Code, wr.Body.String())
	}
	gw.mu.Lock()
	body := gw.lastBody
	gw.mu.Unlock()
	if !strings.Contains(body, "13800138000") || !strings.Contains(body, "您的验证码是") {
		t.Errorf("gateway did not receive phone/content: %q", body)
	}
}

func TestSMSSendGateway500Passthrough(t *testing.T) {
	s, gw, _ := newSMSTestServer(t)
	token := smsLogin(t, s)

	gw.status = http.StatusInternalServerError
	wr := smsSend(t, s, token, "13800138000")
	if wr.Code != http.StatusInternalServerError {
		t.Fatalf("gateway 500 expected passthrough 500, got %d %s", wr.Code, wr.Body.String())
	}
}

func TestSMSSendNotConfigured(t *testing.T) {
	s := New(Config{Addr: ":0", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	token := smsLogin(t, s)

	wr := smsSend(t, s, token, "13800138000")
	if wr.Code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured sms expected 503, got %d %s", wr.Code, wr.Body.String())
	}
}

func TestSMSSendRateLimit(t *testing.T) {
	s, _, _ := newSMSTestServer(t)
	token := smsLogin(t, s)

	if wr := smsSend(t, s, token, "13800138000"); wr.Code != http.StatusOK {
		t.Fatalf("first send expected 200, got %d", wr.Code)
	}
	if wr := smsSend(t, s, token, "13800138000"); wr.Code != http.StatusTooManyRequests {
		t.Fatalf("second send within 60s expected 429, got %d %s", wr.Code, wr.Body.String())
	}
	// 不同手机号不受影响
	if wr := smsSend(t, s, token, "13900139000"); wr.Code != http.StatusOK {
		t.Fatalf("different phone send expected 200, got %d", wr.Code)
	}
}

func TestSMSVerifyKnownCode(t *testing.T) {
	s, _, _ := newSMSTestServer(t)
	token := smsLogin(t, s)
	s.smsStore.SaveCode("13800138000", "123456")

	if wr := smsVerify(t, s, token, "13800138000", "123456"); wr.Code != http.StatusOK {
		t.Fatalf("verify correct code expected 200, got %d %s", wr.Code, wr.Body.String())
	}
	// 验证成功即作废
	if wr := smsVerify(t, s, token, "13800138000", "123456"); wr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("verify used code expected 422 not_found, got %d %s", wr.Code, wr.Body.String())
	}
}

func TestSMSVerifyWrongCode(t *testing.T) {
	s, _, _ := newSMSTestServer(t)
	token := smsLogin(t, s)
	s.smsStore.SaveCode("13800138000", "123456")

	// 错一次：422 mismatch，正确码仍可用（未达上限）
	if wr := smsVerify(t, s, token, "13800138000", "000000"); wr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("verify wrong code expected 422 mismatch, got %d", wr.Code)
	}
	if wr := smsVerify(t, s, token, "13800138000", "123456"); wr.Code != http.StatusOK {
		t.Fatalf("verify right code after one wrong expected 200, got %d", wr.Code)
	}

	// 重新存码，连续错 maxWrong 次后作废
	s.smsStore.SaveCode("13800138000", "123456")
	for i := 0; i < 5; i++ {
		smsVerify(t, s, token, "13800138000", "000000")
	}
	if wr := smsVerify(t, s, token, "13800138000", "123456"); wr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("verify after max wrong expected 422 not_found, got %d %s", wr.Code, wr.Body.String())
	}
}

func TestSMSVerifyExpired(t *testing.T) {
	s, _, _ := newSMSTestServer(t)
	token := smsLogin(t, s)

	fake := time.Unix(100000, 0)
	s.smsStore.Now = func() time.Time { return fake }
	s.smsStore.SaveCode("13800138000", "123456")

	// 时钟推过 5 分钟 TTL
	s.smsStore.Now = func() time.Time { return fake.Add(5*time.Minute + time.Second) }
	if wr := smsVerify(t, s, token, "13800138000", "123456"); wr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("verify expired code expected 422 expired, got %d %s", wr.Code, wr.Body.String())
	}
}

func TestSMSSendToVerifyEndToEnd(t *testing.T) {
	s, gw, _ := newSMSTestServer(t)
	token := smsLogin(t, s)

	if wr := smsSend(t, s, token, "13800138000"); wr.Code != http.StatusOK {
		t.Fatalf("send expected 200, got %d", wr.Code)
	}
	gw.mu.Lock()
	body := gw.lastBody
	gw.mu.Unlock()
	re := regexp.MustCompile(`验证码是 (\d{6})`)
	m := re.FindStringSubmatch(body)
	if len(m) < 2 {
		t.Fatalf("code not found in gateway body: %q", body)
	}
	code := m[1]
	if wr := smsVerify(t, s, token, "13800138000", code); wr.Code != http.StatusOK {
		t.Fatalf("verify extracted code expected 200, got %d %s", wr.Code, wr.Body.String())
	}
}
