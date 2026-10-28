package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jimwong/router-re/mgmtd/internal/rbac"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// createUser registers a user via POST /api/v1/users.
func createUser(t *testing.T, s *Server, token, username, password string, role rbac.Role) {
	t.Helper()
	body := `{"username":"` + username + `","password":"` + password + `","role":"` + string(role) + `"}`
	req := httptest.NewRequest("POST", "/api/v1/users", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create %s: expected 201, got %d body=%s", username, rr.Code, rr.Body.String())
	}
}

// ── tests ────────────────────────────────────────────────────────────────────

// T1: 非法 role → 422
func TestUpdateUserRoleInvalid(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	token := getAdminToken(t, s)

	body := `{"role":"superadmin"}`
	req := httptest.NewRequest("PUT", "/api/v1/users/admin", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid role: expected 422, got %d body=%s", rr.Code, rr.Body.String())
	}
}

// T2: 降级最后一个 admin → 409
func TestUpdateRoleLastAdmin(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	token := getAdminToken(t, s)

	// 先建第二个 admin，再删掉，让 admin 成为唯一 admin。
	createUser(t, s, token, "admin2test", "pass12345", rbac.RoleAdmin)
	delReq := httptest.NewRequest("DELETE", "/api/v1/users/admin2test", nil)
	delReq.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, delReq)
	if rr.Code != http.StatusOK {
		t.Fatalf("delete admin2test: %d", rr.Code)
	}

	// 试图把唯一 admin 降级为 viewer → 409。
	body := `{"role":"viewer"}`
	req := httptest.NewRequest("PUT", "/api/v1/users/admin", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rr2 := httptest.NewRecorder()
	s.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusConflict {
		t.Fatalf("demote last admin: expected 409, got %d body=%s", rr2.Code, rr2.Body.String())
	}
}

// T3: 改 role 后 GET /users 能读到新值
func TestUpdateRolePersisted(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	token := getAdminToken(t, s)

	createUser(t, s, token, "testeroll", "pass12345", rbac.RoleViewer)

	body := `{"role":"operator"}`
	req := httptest.NewRequest("PUT", "/api/v1/users/testeroll", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("update role: expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		User struct{ Role string `json:"role"` } `json:"user"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.User.Role != "operator" {
		t.Fatalf("role not updated: got %q", resp.User.Role)
	}

	getReq := httptest.NewRequest("GET", "/api/v1/users", nil)
	getReq.Header.Set("Authorization", "Bearer "+token)
	getRR := httptest.NewRecorder()
	s.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("get users: %d", getRR.Code)
	}
	var usersResp struct{ Users []struct{ Username, Role string } `json:"users"` }
	if err := json.NewDecoder(getRR.Body).Decode(&usersResp); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, u := range usersResp.Users {
		if u.Username == "testeroll" && u.Role == "operator" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("role not persisted in GET /users")
	}
}

// T4: viewer 调用 PUT → 403
func TestUpdateViewerForbidden(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	adminToken := getAdminToken(t, s)
	createUser(t, s, adminToken, "vieweruser", "pass12345", rbac.RoleViewer)

	loginBody := `{"username":"vieweruser","password":"pass12345"}`
	loginReq := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(loginBody))
	loginRR := httptest.NewRecorder()
	s.ServeHTTP(loginRR, loginReq)
	var viewerToken struct{ Token string `json:"token"` }
	if err := json.NewDecoder(loginRR.Body).Decode(&viewerToken); err != nil || viewerToken.Token == "" {
		t.Fatalf("viewer login failed: %d", loginRR.Code)
	}

	body := `{"role":"operator"}`
	putReq := httptest.NewRequest("PUT", "/api/v1/users/vieweruser", strings.NewReader(body))
	putReq.Header.Set("Authorization", "Bearer "+viewerToken.Token)
	putRR := httptest.NewRecorder()
	s.ServeHTTP(putRR, putReq)
	if putRR.Code != http.StatusForbidden {
		t.Fatalf("viewer update: expected 403, got %d body=%s", putRR.Code, putRR.Body.String())
	}
}
