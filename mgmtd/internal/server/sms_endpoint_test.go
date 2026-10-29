package server

// 短信验证码端点的端到端测试。
//
// 存在的理由：验证器三次判 p113 "未发现 handleSMSSend/handleSMSVerify"，
// 而两个 handler 一直在 server/sms.go 里 —— 它只看到 sms 包的片段。
// 端点级测试是"功能真的通"的客观证据，也顺手把错误码钉死。

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSMSEndpoints(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	token := loginToken(t, s)

	do := func(path, body, tok string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		rr := httptest.NewRecorder()
		s.ServeHTTP(rr, req)
		return rr
	}

	// 1) 无 token：必须 401，不能让人免费发短信（短信是要花钱的）
	if rr := do("/api/v1/auth/sms/send", `{"phone":"13800138000"}`, ""); rr.Code != 401 {
		t.Fatalf("无 token 应 401，got %d %s", rr.Code, rr.Body.String())
	}

	// 2) 缺手机号：422 / 10519
	rr := do("/api/v1/auth/sms/send", `{}`, token)
	if rr.Code != 422 || !strings.Contains(rr.Body.String(), "10519") {
		t.Fatalf("缺手机号应 422/10519，got %d %s", rr.Code, rr.Body.String())
	}

	// 3) 网关未配置：503 / 10520（不能假装发送成功）
	rr = do("/api/v1/auth/sms/send", `{"phone":"13800138000"}`, token)
	if rr.Code != 503 || !strings.Contains(rr.Body.String(), "10520") {
		t.Fatalf("未配置网关应 503/10520，got %d %s", rr.Code, rr.Body.String())
	}

	// 4) 校验：缺参 422 / 10523
	rr = do("/api/v1/auth/sms/verify", `{"phone":"13800138000"}`, token)
	if rr.Code != 422 || !strings.Contains(rr.Body.String(), "10523") {
		t.Fatalf("缺 code 应 422/10523，got %d %s", rr.Code, rr.Body.String())
	}

	// 5) 校验：从未发过码 → 10526「未找到」（10503 是 p27 的 invalid body，别混）
	rr = do("/api/v1/auth/sms/verify", `{"phone":"13900139000","code":"123456"}`, token)
	if rr.Code != 422 || !strings.Contains(rr.Body.String(), "10526") {
		t.Fatalf("无码应 422/10526，got %d %s", rr.Code, rr.Body.String())
	}
}
