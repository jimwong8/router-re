package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 第二批 6 个"全量替换"端点的空数组保护（与第一批 l7/alerts/tables 同一病灶）：
// 空数组 + 无 confirm -> 422；带 confirm:true -> 正常 200。
func TestEmptyArrayRequiresConfirmBatch2(t *testing.T) {
	s := New(Config{
		Addr:          ":0",
		SecretKey:     []byte("0123456789abcdef0123456789abcdef"),
		AdminUser:     "admin",
		AdminPassword: "pw",
	})

	cases := []struct {
		name    string
		handler http.HandlerFunc
		body    string
		wantOK  bool
		code    string
	}{
		{"pppoe 空数组被拒", s.handleSetPPPoEUsers, `{"users":[]}`, false, "10052"},
		{"pppoe confirm 放行", s.handleSetPPPoEUsers, `{"users":[],"confirm":true}`, true, ""},
		{"porttrigger 空数组被拒", s.handleSetPortTrigger, `{"rules":[]}`, false, "10347"},
		{"porttrigger confirm 放行", s.handleSetPortTrigger, `{"rules":[],"confirm":true}`, true, ""},
		{"policyrules 空数组被拒", s.handleSetPolicyRules, `{"rules":[]}`, false, "10320"},
		{"policyrules confirm 放行", s.handleSetPolicyRules, `{"rules":[],"confirm":true}`, true, ""},
		{"quota 空数组被拒", s.handleSetQuotaPolicies, `{"policies":[]}`, false, "10173"},
		{"quota confirm 放行", s.handleSetQuotaPolicies, `{"policies":[],"confirm":true}`, true, ""},
		{"ddns 空数组被拒", s.handleSetDDNS, `{"records":[]}`, false, "10425"},
		{"ddns confirm 放行", s.handleSetDDNS, `{"records":[],"confirm":true}`, true, ""},
		{"threat 空文本被拒", s.handleThreatImport, `{"text":""}`, false, "10166"},
		{"threat 空白文本被拒", s.handleThreatImport, `{"text":"  \n "}`, false, "10166"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(c.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c.handler(rec, req)

			if c.wantOK {
				if rec.Code != http.StatusOK {
					t.Fatalf("应放行，实际 %d body=%s", rec.Code, rec.Body.String())
				}
				return
			}
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("应返回 422，实际 %d body=%s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), c.code) {
				t.Fatalf("错误码应含 %s，实际 %s", c.code, rec.Body.String())
			}
		})
	}
}

func TestIPSBlocklistEmptyRequiresConfirm(t *testing.T) {
	s := New(Config{
		Addr:          ":0",
		SecretKey:     []byte("0123456789abcdef0123456789abcdef"),
		AdminUser:     "admin",
		AdminPassword: "pw",
	})

	// 空数组 = 全网解封，必须 422；带 confirm 才能放行
	for _, tc := range []struct {
		name   string
		body   string
		wantOK bool
		code   string
	}{
		{"空数组被拒", `{"ips":[]}`, false, "10043"},
		{"confirm 放行", `{"ips":[],"confirm":true}`, true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			s.handleSetIPSBlocklist(rec, req)
			if tc.wantOK {
				// 无 aclApplier 的单测环境会 500（exec runner 未初始化），
				// 关键是不能 422 —— 说明空数组检查已通过
				if rec.Code == http.StatusUnprocessableEntity {
					t.Fatalf("带 confirm 不应再报 422，实际 body=%s", rec.Body.String())
				}
				return
			}
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("应返回 422，实际 %d body=%s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tc.code) {
				t.Fatalf("错误码应含 %s，实际 %s", tc.code, rec.Body.String())
			}
		})
	}
}
