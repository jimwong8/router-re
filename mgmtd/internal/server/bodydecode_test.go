package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 整表替换型接口在请求体缺少数组字段时，绝不能把线上配置清空。
// 修复前：POST {"x":1} → 未知字段被忽略 → nil 切片 → Apply(nil) 清空 → 200。
// 修复后：字段缺失 → 422/10099；显式 {"rules":[]} 仍允许清空。
func TestReplaceListRejectsMissingField(t *testing.T) {
	cases := []struct {
		name   string
		path   string
		field  string
		action string
	}{
		{"firewall", "/api/v1/firewall/rules", "rules", "firewall.rules.set"},
		{"nat", "/api/v1/nat/rules", "rules", "nat.rules.set"},
		{"qos", "/api/v1/qos/limits", "limits", "qos.limits.set"},
	}
	bodies := []string{
		`{"x":1}`,         // 字段名写错
		`{}`,              // 空对象
		`{"rules":null}`,  // 显式 null 也不该被当成"清空"以外的语义
		`{"limits":null}`, // 同上
	}

	for _, c := range cases {
		for _, body := range bodies {
			// 只测与本端点字段相关的 body，避免 rules/limits 混用
			if c.field == "rules" && strings.Contains(body, "limits") {
				continue
			}
			if c.field == "limits" && strings.Contains(body, "rules") {
				continue
			}
			s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
			tok := loginTokenAs(t, s, "admin", "admin123")
			req := httptest.NewRequest("POST", c.path, strings.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+tok)
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			s.ServeHTTP(rr, req)

			// 字段缺失必须是 422（10099），绝不能是 200
			if rr.Code == http.StatusOK {
				t.Fatalf("%s %s: body=%s 不应返回 200（会静默清空配置），实际 200: %s",
					c.name, c.action, body, rr.Body.String())
			}
			if !strings.Contains(rr.Body.String(), "10099") && !strings.Contains(rr.Body.String(), "missing list field") {
				t.Fatalf("%s %s: body=%s 应返回 10099 missing list field，实际 %d: %s",
					c.name, c.action, body, rr.Code, rr.Body.String())
			}
		}
	}
}

// 显式空数组必须仍然允许（表示清空全部规则），不能因为加了校验就挡住合法清空。
func TestReplaceListAllowsExplicitEmpty(t *testing.T) {
	cases := []struct{ path, body string }{
		{"/api/v1/firewall/rules", `{"rules":[]}`},
		{"/api/v1/nat/rules", `{"rules":[]}`},
		{"/api/v1/qos/limits", `{"limits":[]}`},
	}
	for _, c := range cases {
		s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
		tok := loginTokenAs(t, s, "admin", "admin123")
		req := httptest.NewRequest("POST", c.path, strings.NewReader(c.body))
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		s.ServeHTTP(rr, req)
		// 非 root 环境下 nft 应用会失败（500/10022 等），但绝不应是 10099（缺字段）
		if strings.Contains(rr.Body.String(), "10099") {
			t.Fatalf("%s body=%s 显式空数组不应被判为缺字段: %d %s", c.path, c.body, rr.Code, rr.Body.String())
		}
	}
}

// 畸形 JSON 仍应是 400/10007，不应误报缺字段。
func TestReplaceListMalformedJSON(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	tok := loginTokenAs(t, s, "admin", "admin123")
	req := httptest.NewRequest("POST", "/api/v1/firewall/rules", strings.NewReader(`{not json`))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("畸形 JSON 应 400，得 %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "10007") {
		t.Fatalf("畸形 JSON 应返回 10007，实际: %s", rr.Body.String())
	}
}
