// Package sms 实现短信验证码网关：
//   - Provider：可插拔的短信发送抽象，便于对接不同网关或单元测试造假；
//   - GenericHTTPProvider：通过 HTTP 调用通用短信网关，可配置 url/method/body 模板/头；
//   - MemoryStore：基于内存的验证码存储与频率限制（6 位码、5 分钟过期、同号 60 秒限发一条、每小时最多 5 次）。
package sms

import (
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Provider 是短信发送的抽象接口。
type Provider interface {
	// Send 向手机号发送短信内容，失败返回错误。
	// 若上游返回非 2xx，返回携带 HTTP 状态码的 *HTTPError，供调用方透传。
	Send(phone, content string) error
}

// HTTPError 携带 HTTP 状态码的错误。
type HTTPError struct {
	StatusCode int
	Err        error
}

func (e *HTTPError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return fmt.Sprintf("http status %d", e.StatusCode)
}

func (e *HTTPError) Unwrap() error { return e.Err }

// GenericHTTPProvider 通过 HTTP 调用通用短信网关。
// body 模板支持占位符 {phone}、{content}；headers 为额外请求头。
// http.Client 带 5s 超时，避免上游卡死拖垮调用方。
type GenericHTTPProvider struct {
	URL      string
	Method   string
	BodyTmpl string
	Headers  map[string]string
	client   *http.Client
}

// NewGenericHTTPProvider 创建通用 HTTP 短信提供者。
func NewGenericHTTPProvider(url, method, bodyTmpl string, headers map[string]string) *GenericHTTPProvider {
	if method == "" {
		method = http.MethodPost
	}
	return &GenericHTTPProvider{
		URL:      url,
		Method:   method,
		BodyTmpl: bodyTmpl,
		Headers:  headers,
		client:   &http.Client{Timeout: 5 * time.Second},
	}
}

// Send 把 body 模板里的 {phone}/{content} 替换后，发送给上游网关。
func (p *GenericHTTPProvider) Send(phone, content string) error {
	body := strings.ReplaceAll(p.BodyTmpl, "{phone}", phone)
	body = strings.ReplaceAll(body, "{content}", content)

	req, err := http.NewRequest(p.Method, p.URL, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", detectContentType(body))
	for k, v := range p.Headers {
		req.Header.Set(k, v)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		io.Copy(io.Discard, resp.Body)
		return &HTTPError{StatusCode: resp.StatusCode, Err: errors.New("gateway returned " + resp.Status)}
	}
	io.Copy(io.Discard, resp.Body)
	return nil
}

func detectContentType(body string) string {
	trimmed := strings.TrimSpace(body)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return "application/json"
	}
	return "application/x-www-form-urlencoded"
}

// ---------------------------------------------------------------------------
// 验证码内存存储与频率限制
// ---------------------------------------------------------------------------

const (
	smsSection     = "sms"
	smsOption      = "config"
	codeLen        = 6
	codeTTL        = 5 * time.Minute
	resendInterval = 60 * time.Second
	maxPerHour     = 5
	maxWrong       = 5
)

// Section 返回短信配置在 config.Store 中的 section。
func Section() string { return smsSection }

// Option 返回短信配置在 config.Store 中的 option。
func Option() string { return smsOption }

type record struct {
	code      string
	expireAt  time.Time
	sendCount int       // 当前小时窗口内已发送次数
	hourStart time.Time // 当前小时窗口起点
	lastSend  time.Time
	wrong     int // 连续错码次数
}

// MemoryStore 是基于内存的验证码存储，并发安全。
type MemoryStore struct {
	mu  sync.Mutex
	m   map[string]*record
	Now func() time.Time // 可注入，默认 time.Now；单元测试用于模拟过期
}

// NewMemoryStore 创建内存验证码存储。
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		m:   make(map[string]*record),
		Now: time.Now,
	}
}

// generateCode 生成 6 位数字验证码。
func (s *MemoryStore) GenerateCode() int {
	return rand.Intn(1000000)
}

// CanSend 检查该手机号是否可发送；不可发送返回需等待时间。
// 规则：同手机号 60 秒内限发一条；每小时最多发送 5 次。
func (s *MemoryStore) CanSend(phone string) (retryAfter time.Duration, allowed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.Now()
	rec, ok := s.m[phone]
	if !ok {
		return 0, true
	}
	// 小时窗口滚动
	if rec.hourStart.IsZero() || now.Sub(rec.hourStart) >= time.Hour {
		rec.hourStart = now
		rec.sendCount = 0
	}
	if rec.sendCount >= maxPerHour {
		return rec.hourStart.Add(time.Hour).Sub(now), false
	}
	if !rec.lastSend.IsZero() && now.Sub(rec.lastSend) < resendInterval {
		return resendInterval - now.Sub(rec.lastSend), false
	}
	return 0, true
}

// SaveCode 保存新生成的验证码并更新发送计数。
func (s *MemoryStore) SaveCode(phone, code string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.Now()
	rec, ok := s.m[phone]
	if !ok {
		rec = &record{}
		s.m[phone] = rec
	}
	if rec.hourStart.IsZero() || now.Sub(rec.hourStart) >= time.Hour {
		rec.hourStart = now
		rec.sendCount = 0
	}
	rec.code = code
	rec.expireAt = now.Add(codeTTL)
	rec.sendCount++
	rec.lastSend = now
	rec.wrong = 0
}

// Verify 验证验证码。
// 成功返回 (true, "") 并作废该验证码；
// 失败返回 (false, reason)，reason 为 "expired"、"mismatch"、"not_found"。
// 连续错码达到 maxWrong 后作废验证码。
func (s *MemoryStore) Verify(phone, code string) (bool, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.m[phone]
	if !ok {
		return false, "not_found"
	}
	now := s.Now()
	if now.After(rec.expireAt) {
		delete(s.m, phone)
		return false, "expired"
	}
	if !strings.EqualFold(rec.code, code) {
		rec.wrong++
		if rec.wrong >= maxWrong {
			delete(s.m, phone)
		}
		return false, "mismatch"
	}
	delete(s.m, phone) // 验证成功即作废
	return true, ""
}
