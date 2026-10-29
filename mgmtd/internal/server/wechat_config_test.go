package server

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"
)

// 前端「GET 出来 → 改点别的 → 原样 POST 回去」不能让掩码覆盖真 secret。
// 这是最阴的一类配置 bug：secret_set 仍为 true，登录却永远失败。
func TestWechatConfigMaskedSecretNotOverwritten(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	token := loginToken(t, s)

	const realSecret = "abcdef1234567890abcdef1234567890"
	body, _ := json.Marshal(map[string]string{"appid": "wx1234567890abcdef", "appsecret": realSecret})
	req := httptest.NewRequest("POST", "/api/v1/auth/wechat/config", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("保存配置应 200，got %d %s", rr.Code, rr.Body.String())
	}

	// 模拟前端：读回（拿到掩码），改 appid，再整份存回
	req = httptest.NewRequest("GET", "/api/v1/auth/wechat/config", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr = httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	var got map[string]interface{}
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	if got["secret_set"] != true {
		t.Fatalf("secret 应已设置: %v", got)
	}
	masked, _ := got["appsecret"].(string)
	if masked == realSecret || masked == "" {
		t.Fatalf("GET 必须返回掩码，got %q", masked)
	}

	body, _ = json.Marshal(map[string]string{
		"appid": "wx9999999999abcdef", "appsecret": masked,
	})
	req = httptest.NewRequest("POST", "/api/v1/auth/wechat/config", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rr = httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("回存应 200，got %d %s", rr.Code, rr.Body.String())
	}

	// 真 secret 必须还在
	var cfg wechatConfig
	if !s.loadJSON("wechat", "config", &cfg) {
		t.Fatal("配置读不出来")
	}
	if cfg.AppSecret != realSecret {
		t.Fatalf("真 secret 被掩码覆盖了: %q", cfg.AppSecret)
	}
	if cfg.AppID != "wx9999999999abcdef" {
		t.Fatalf("appid 应已更新，got %q", cfg.AppID)
	}
}

func TestWechatConfigRejectsBadAppID(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	token := loginToken(t, s)
	for _, bad := range []string{"abc", "wx123", "123456789012345678"} {
		body, _ := json.Marshal(map[string]string{"appid": bad, "appsecret": "x"})
		req := httptest.NewRequest("POST", "/api/v1/auth/wechat/config", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		s.ServeHTTP(rr, req)
		if rr.Code != 422 {
			t.Fatalf("appid %q 应 422，got %d", bad, rr.Code)
		}
	}
}

func TestWechatLoginWithoutConfig(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	token := loginToken(t, s)
	req := httptest.NewRequest("POST", "/api/v1/auth/wechat/login", bytes.NewReader([]byte(`{"code":"x"}`)))
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if rr.Code != 409 {
		t.Fatalf("未配置应 409（不是 503，503 会诱导客户端重试），got %d %s", rr.Code, rr.Body.String())
	}
}
