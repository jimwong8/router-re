package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// loginToken 走真实登录接口拿 token，避免测试绕过鉴权层。
func loginToken(t *testing.T, s *Server) string {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/v1/auth/login",
		strings.NewReader(`{"username":"admin","password":"admin123"}`))
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil || body.Token == "" {
		t.Fatalf("login 响应没拿到 token: %s", rr.Body.String())
	}
	return body.Token
}

// 索引式持久化的覆盖写必须把旧的更高索引清掉。
//
// 这个 bug 是被实测抓出来的：先提交 3 条防火墙规则（22/80/443），再提交 1 条
// （只有 22），GET 仍然返回 3 条——80 和 443 早已被用户删掉，却还躺在 store 里。
// 影响不只是界面显示错：
//   - GET /firewall/rules 会把已删规则渲染进 nft 文本，运维照着核对会被误导；
//   - 任何按 store 重放的路径（重启恢复、配置导出/恢复）都会让删掉的规则复活，
//     若那是条 accept 规则，等于开了个自己以为已经关掉的洞。
//
// 根因：写入只覆盖 0..len-1，而 GET 用 `for i:=0;;i++ { if 空 break }` 枚举，
// 高位残留自然被当成仍然存在。
func TestSetFirewallRulesDropsStaleIndices(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	token := loginToken(t, s)

	put := func(rulesJSON string) {
		t.Helper()
		req := httptest.NewRequest("POST", "/api/v1/firewall/rules", strings.NewReader(rulesJSON))
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		s.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("set rules failed: %d %s", rr.Code, rr.Body.String())
		}
	}
	get := func() []map[string]interface{} {
		t.Helper()
		req := httptest.NewRequest("GET", "/api/v1/firewall/rules", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		s.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("get rules failed: %d %s", rr.Code, rr.Body.String())
		}
		var body struct {
			Rules []map[string]interface{} `json:"rules"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatalf("parse get rules: %v", err)
		}
		return body.Rules
	}

	three := `{"rules":[
		{"chain":"input","proto":"tcp","dport":"22","action":"accept"},
		{"chain":"input","proto":"tcp","dport":"80","action":"accept"},
		{"chain":"input","proto":"tcp","dport":"443","action":"accept"}]}`
	put(three)
	if got := get(); len(got) != 3 {
		t.Fatalf("写入 3 条后应读回 3 条，实际 %d", len(got))
	}

	one := `{"rules":[{"chain":"input","proto":"tcp","dport":"22","action":"accept"}]}`
	put(one)
	got := get()
	if len(got) != 1 {
		t.Fatalf("收敛到 1 条后应读回 1 条，实际 %d 条：%v（幽灵规则未清除）", len(got), got)
	}
	if dport := fmt.Sprint(got[0]["DPort"]); dport != "22" {
		t.Errorf("剩下的规则应是 22，实际 dport=%s", dport)
	}

	// 全清空也要真的清空
	put(`{"rules":[]}`)
	if got := get(); len(got) != 0 {
		t.Fatalf("清空后应读回 0 条，实际 %d：%v", len(got), got)
	}
}

// NAT 用的是同一套索引式存储，同一个 bug，一起钉死。
func TestSetNATRulesDropsStaleIndices(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	token := loginToken(t, s)

	post := func(rulesJSON string) {
		t.Helper()
		req := httptest.NewRequest("POST", "/api/v1/nat/rules", strings.NewReader(rulesJSON))
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		s.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("set nat failed: %d %s", rr.Code, rr.Body.String())
		}
	}
	count := func() int {
		t.Helper()
		req := httptest.NewRequest("GET", "/api/v1/nat/rules", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		s.ServeHTTP(rr, req)
		var body struct {
			Rules []map[string]interface{} `json:"rules"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatalf("parse nat rules: %v", err)
		}
		return len(body.Rules)
	}

	post(`{"rules":[{"type":"snat","interface":"wan","proto":"tcp","srcport":"","dstaddr":"","dstport":"","toport":""},
	               {"type":"dnat","interface":"wan","proto":"tcp","srcport":"8080","dstaddr":"10.0.0.5","dstport":"80","toport":"80"}]}`)
	if n := count(); n != 2 {
		t.Fatalf("写入 2 条后应读回 2 条，实际 %d", n)
	}
	post(`{"rules":[{"type":"snat","interface":"wan","proto":"tcp","srcport":"","dstaddr":"","dstport":"","toport":""}]}`)
	if n := count(); n != 1 {
		t.Fatalf("收敛到 1 条后应读回 1 条，实际 %d（幽灵 NAT 规则未清除）", n)
	}
}
