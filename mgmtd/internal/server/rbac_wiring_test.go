package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 策略函数 rbac.Can 早就写好了，规则也对，但从来没有任何请求走到过它——
// 单测绿着，生产零校验。实测过：viewer 账号 POST /api/v1/firewall/rules
// 返回 200，能改防火墙；只有 /api/v1/users 那几个端点自己单独查了 role。
//
// 这个测试盯的是「策略有没有真的接到请求链上」，而不是策略内容本身
// （策略内容由 internal/rbac 的单测负责）。
func TestRBACIsEnforcedOnRequests(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	admin := loginToken(t, s)

	createUser := func(username, password, role string) {
		t.Helper()
		body := `{"username":"` + username + `","password":"` + password + `","role":"` + role + `"}`
		req := httptest.NewRequest("POST", "/api/v1/users", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+admin)
		rr := httptest.NewRecorder()
		s.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated && rr.Code != http.StatusOK {
			t.Fatalf("create %s(%s) failed: %d %s", username, role, rr.Code, rr.Body.String())
		}
	}
	login := func(username, password string) string {
		t.Helper()
		req := httptest.NewRequest("POST", "/api/v1/auth/login",
			strings.NewReader(`{"username":"`+username+`","password":"`+password+`"}`))
		rr := httptest.NewRecorder()
		s.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("login %s failed: %d %s", username, rr.Code, rr.Body.String())
		}
		var body struct {
			Token string `json:"token"`
		}
		_ = json.Unmarshal(rr.Body.Bytes(), &body)
		return body.Token
	}
	do := func(token, method, path, body string) int {
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
		return rr.Code
	}

	createUser("vt1", "Vtest12345!", "viewer")
	createUser("op1", "Otest12345!", "operator")
	viewer := login("vt1", "Vtest12345!")
	operator := login("op1", "Otest12345!")

	const ruleBody = `{"rules":[{"chain":"input","proto":"tcp","dport":"8080","action":"accept"}]}`

	t.Run("viewer 可以读", func(t *testing.T) {
		if code := do(viewer, "GET", "/api/v1/system/info", ""); code != http.StatusOK {
			t.Errorf("viewer GET /system/info = %d，应 200", code)
		}
	})

	t.Run("viewer 不能写", func(t *testing.T) {
		if code := do(viewer, "POST", "/api/v1/firewall/rules", ruleBody); code != http.StatusForbidden {
			t.Errorf("viewer POST /firewall/rules = %d，应 403（修复前是 200，等于提权）", code)
		}
	})

	t.Run("viewer 不能恢复配置", func(t *testing.T) {
		if code := do(viewer, "POST", "/api/v1/config/restore", `{"confirm":true}`); code != http.StatusForbidden {
			t.Errorf("viewer POST /config/restore = %d，应 403", code)
		}
	})

	t.Run("operator 能写业务配置", func(t *testing.T) {
		if code := do(operator, "POST", "/api/v1/firewall/rules", ruleBody); code != http.StatusOK {
			t.Errorf("operator POST /firewall/rules = %d，应 200", code)
		}
	})

	t.Run("operator 不能管用户", func(t *testing.T) {
		if code := do(operator, "GET", "/api/v1/users", ""); code != http.StatusForbidden {
			t.Errorf("operator GET /users = %d，应 403", code)
		}
	})

	t.Run("admin 全通", func(t *testing.T) {
		if code := do(admin, "GET", "/api/v1/users", ""); code != http.StatusOK {
			t.Errorf("admin GET /users = %d，应 200", code)
		}
		if code := do(admin, "POST", "/api/v1/firewall/rules", ruleBody); code != http.StatusOK {
			t.Errorf("admin POST /firewall/rules = %d，应 200", code)
		}
	})

	t.Run("删掉的用户 token 立即失效", func(t *testing.T) {
		// roleAllows 每次请求都查一次角色，用户被删后拿旧 token 继续用必须被拒。
		if code := do(admin, "DELETE", "/api/v1/users/vt1", ""); code != http.StatusOK && code != http.StatusNoContent {
			t.Fatalf("delete vtest failed: %d", code)
		}
		if code := do(viewer, "GET", "/api/v1/system/info", ""); code != http.StatusForbidden {
			t.Errorf("已删除用户的旧 token 仍然可用：GET /system/info = %d，应 403", code)
		}
	})
}
