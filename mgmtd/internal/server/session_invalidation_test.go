package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// 密码泄漏时的第一反应是改密。但 token 是无状态的，改密本身不会让已签发的
// token 失效——旧代码下攻击者手上那个 token 能一直用到 TTL 到期（15 分钟）。
// 也就是说：改密看起来生效了，实际没把任何人踢出去。
//
// 这个用例盯的是「改密之后旧会话还灵不灵」，而不是密码哈希本身
// （哈希由 internal/rbac 的单测负责）。
func TestPasswordChangeInvalidatesOldTokens(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})

	login := func(username, password string) string {
		t.Helper()
		req := httptest.NewRequest("POST", "/api/v1/auth/login",
			strings.NewReader(`{"username":"`+username+`","password":"`+password+`"}`))
		rr := httptest.NewRecorder()
		s.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("登录 %s 失败: %d %s", username, rr.Code, rr.Body.String())
		}
		var body struct {
			Token string `json:"token"`
		}
		_ = json.Unmarshal(rr.Body.Bytes(), &body)
		return body.Token
	}
	do := func(token, method, path, body string) (int, string) {
		t.Helper()
		var r *http.Request
		if body == "" {
			r = httptest.NewRequest(method, path, nil)
		} else {
			r = httptest.NewRequest(method, path, strings.NewReader(body))
		}
		r.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		s.ServeHTTP(rr, r)
		return rr.Code, rr.Body.String()
	}

	admin := login("admin", "admin123")

	// 建一个 operator：能写业务接口，便于证明"旧 token 改密前确实可用"。
	rule := `{"rules":[{"chain":"input","proto":"tcp","dport":"9","action":"accept"}]}`
	if code, body := do(admin, "POST", "/api/v1/users",
		`{"username":"stale_op","password":"Oldpass12345!","role":"operator"}`); code != http.StatusCreated && code != http.StatusOK {
		t.Fatalf("建用户失败: %d %s", code, body)
	}

	oldToken := login("stale_op", "Oldpass12345!")

	// 前提：改密前这个 token 确实能用。不然下面的 401 可能本来就没权限，
	// 测出来的"通过"是假的。
	if code, body := do(oldToken, "POST", "/api/v1/firewall/rules", rule); code != http.StatusOK {
		t.Fatalf("改密前旧 token 应可用: %d %s", code, body)
	}

	// iat 是秒级，改密必须晚于签发时间那一秒，否则同一秒内 iat == 改密时间，
	// 按设计仍然有效（避免刚改完密重新登录被自己的新 token 踢掉）。
	time.Sleep(1100 * time.Millisecond)

	if code, body := do(admin, "POST", "/api/v1/users/stale_op/password",
		`{"password":"Newpass12345!"}`); code != http.StatusOK {
		t.Fatalf("改密失败: %d %s", code, body)
	}

	// 核心断言：旧 token 立即失效，且是 401（会话失效）而不是 403（权限不足）——
	// 403 会让人以为"再提个权就行"，而这里需要的是重新登录。
	code, body := do(oldToken, "POST", "/api/v1/firewall/rules", rule)
	if code != http.StatusUnauthorized {
		t.Errorf("改密后旧 token 仍可用: %d %s（应 401，实际让它执行了 %d 次写操作）",
			code, body, 1)
	}
	if !strings.Contains(body, "重新登录") {
		t.Errorf("401 的提示应当告诉用户重新登录，实际: %s", body)
	}

	// 新密码能登进来，且新会话可用——别把人锁在门外。
	newToken := login("stale_op", "Newpass12345!")
	if code, body := do(newToken, "POST", "/api/v1/firewall/rules", rule); code != http.StatusOK {
		t.Errorf("改密后新登录应当可用: %d %s", code, body)
	}

	// 内置 admin 没有用户记录，改密踢人不能误伤它。
	if code, body := do(admin, "GET", "/api/v1/system/info", ""); code != http.StatusOK {
		t.Errorf("内置 admin 不受影响: %d %s", code, body)
	}
}
