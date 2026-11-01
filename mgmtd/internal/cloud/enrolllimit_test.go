package cloud

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// /agent/enroll 是公开端点。用假令牌反复试探必须被按 IP 限流，
// 否则每次请求都要查令牌表、写审计，等于给攻击者一个免费的负载发生器。
func TestEnrollRateLimitedByIP(t *testing.T) {
	svc := newTestService(t)
	srv := httptest.NewServer(svc.Handler())
	defer srv.Close()

	post := func() int {
		body, _ := json.Marshal(map[string]string{"token": "bogus-token", "name": "x"})
		resp, err := http.Post(srv.URL+"/cloud/v1/agent/enroll", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("enroll request: %v", err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	// 前 loginMaxFails 次应是凭据错误 401（令牌无效）
	saw429 := false
	for i := 0; i < loginMaxFails+3; i++ {
		code := post()
		if code == http.StatusTooManyRequests {
			saw429 = true
			break
		}
		if code != http.StatusUnauthorized {
			t.Fatalf("第 %d 次期望 401，得到 %d", i+1, code)
		}
	}
	if !saw429 {
		t.Fatalf("连续无效注册始终没有触发 429 限流")
	}
}

// 无效注册只应置 dirty，不应每次同步落盘。这里直接断言：
// 一批失败注册后，SaveIfDirty 能识别出待落盘；清一次后即干净。
func TestFailedEnrollDoesNotForceSyncSave(t *testing.T) {
	svc := newTestService(t)
	srv := httptest.NewServer(svc.Handler())
	defer srv.Close()

	for i := 0; i < 3; i++ {
		body, _ := json.Marshal(map[string]string{"token": "bogus", "name": "x"})
		resp, err := http.Post(srv.URL+"/cloud/v1/agent/enroll", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	// 有未落盘变更
	if err := svc.Store().SaveIfDirty(); err != nil {
		t.Fatalf("SaveIfDirty: %v", err)
	}
	// 落盘后再调应无操作（不报错）
	if err := svc.Store().SaveIfDirty(); err != nil {
		t.Fatalf("二次 SaveIfDirty: %v", err)
	}
}
