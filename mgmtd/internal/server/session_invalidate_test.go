package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 改密必须让此前签发的 token 失效——包括内置 env 管理员。
//
// 内置管理员在启动时就会被种进 RBAC 表（NewManagerWithFile），所以旧逻辑
// "actor == AdminUser 恒真"让最重要的账号永远踢不掉：实测改密后旧 token
// 访问 /api/v1/users 仍返回 200。
func TestPasswordChangeInvalidatesBuiltinAdminToken(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	oldToken := loginTokenAs(t, s, "admin", "admin123")

	// 改密前旧 token 可用
	if code := getWithToken(s, "/api/v1/users", oldToken); code != http.StatusOK {
		t.Fatalf("改密前应 200，得 %d", code)
	}
	if code := postChangePassword(s, oldToken, "admin123", "NewPass12345"); code != http.StatusOK {
		t.Fatalf("改密应 200，得 %d", code)
	}

	// 改密后旧 token 必须被拒（401）
	if code := getWithToken(s, "/api/v1/users", oldToken); code != http.StatusUnauthorized {
		t.Fatalf("改密后旧 token 应 401，实际 %d —— 旧会话未被踢掉", code)
	}

	// 用新密码登录，新 token 可用
	newToken := loginTokenAs(t, s, "admin", "NewPass12345")
	if code := getWithToken(s, "/api/v1/users", newToken); code != http.StatusOK {
		t.Fatalf("新 token 应 200，得 %d", code)
	}
}

// 改密后旧的环境变量密码必须失效：否则"改密"只是多了一个能用的新密码，
// 泄露的旧密码照样能登进来（实测旧密码改密后仍返回 200）。
func TestPasswordChangeInvalidatesEnvFallbackPassword(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	tok := loginTokenAs(t, s, "admin", "admin123")
	if code := postChangePassword(s, tok, "admin123", "NewPass12345"); code != http.StatusOK {
		t.Fatalf("改密应 200，得 %d", code)
	}

	// 旧 env 密码必须登不进来
	body := `{"username":"admin","password":"admin123"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("改密后旧密码应 401，实际 %d —— env 兜底密码仍然可用", rr.Code)
	}

	// 新密码可以登录
	loginTokenAs(t, s, "admin", "NewPass12345")
}

// 未改过密码时，env 兜底仍要工作：单管理员部署与 .env 轮换都依赖它。
func TestEnvFallbackWorksBeforeAnyPasswordChange(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	// 种进表的密码与 env 一致，未改过密 → 两条路径都放行
	loginTokenAs(t, s, "admin", "admin123")
	if s.rbacMgr != nil {
		u, ok := s.rbacMgr.Get("admin")
		if !ok {
			t.Fatal("内置管理员应已种入 RBAC 表")
		}
		if u.PasswordChangedAt != 0 {
			t.Fatalf("刚启动时 PasswordChangedAt 应为 0，得 %d", u.PasswordChangedAt)
		}
	}
}

func getWithToken(s *Server, path, token string) int {
	req := httptest.NewRequest("GET", path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	return rr.Code
}
