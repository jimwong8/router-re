package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// POST /api/v1/wan/links 空列表必须显式 confirm:true 才放行。
//
// 这是 wan-access 冒烟测试抓出来的真缺陷：空 body 直接 200 applied=true。
// 危害不只是"少了一层校验"——handleSetWANLinks 会调用 wan.Applier.Apply(nil)，
// 按"零条策略路由"下发，等于把现存的 WAN 策略路由规则整片抽掉，然后回 200
// 让人以为一切正常。
//
// 约定与 alerts / l7 / tables 三个"全量替换"端点保持一致（见 empty_arrays_test.go），
// 不另造一套语义。
func TestWanLinksEmptyRequiresConfirm(t *testing.T) {
	s := New(Config{
		Addr:          ":0",
		SecretKey:     []byte("0123456789abcdef0123456789abcdef"),
		AdminUser:     "admin",
		AdminPassword: "pw",
	})

	cases := []struct {
		name   string
		body   string
		want   int
		codeIn string
	}{
		{"空 body 被拒", `{}`, http.StatusUnprocessableEntity, "10484"},
		{"空数组被拒", `{"links":[]}`, http.StatusUnprocessableEntity, "10484"},
		{"空数组 confirm 放行", `{"links":[],"confirm":true}`, http.StatusOK, ""},
		{"缺接口名被拒", `{"links":[{"name":"wan1"}]}`, http.StatusUnprocessableEntity, "10485"},
		{"坏 JSON 400", `{`, http.StatusBadRequest, "10007"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/wan/links", strings.NewReader(c.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			s.handleSetWANLinks(rec, req)

			if rec.Code != c.want {
				t.Fatalf("HTTP %d，期望 %d，body=%s", rec.Code, c.want, rec.Body.String())
			}
			if c.codeIn != "" && !strings.Contains(rec.Body.String(), c.codeIn) {
				t.Fatalf("响应里应含错误码 %s，实际 %s", c.codeIn, rec.Body.String())
			}
		})
	}
}
