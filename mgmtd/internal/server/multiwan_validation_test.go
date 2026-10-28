package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// handleSetMultiWAN 需要在集合级拦截以下坏输入（单成员 ValidateMember
// 无法发现交叉问题）：
//
//	① members 为空数组          → 422, 10369  (需 confirm:true 放行)
//	② 同名成员重复              → 422, 10370
//	③ 同一 interface 重复       → 422, 10371
//	④ weight 为负数或超过 100   → 422, 10372
//
// 校验顺序：空数组 → 集合级交叉校验 → 单成员 ValidateMember → ValidateProbe。
// 权重在集合级提前拦截，赋予 10372，区别于单成员 10365。
func TestHandleSetMultiWANValidation(t *testing.T) {
	s := New(Config{
		Addr:          ":0",
		SecretKey:     []byte("0123456789abcdef0123456789abcdef"),
		AdminUser:     "admin",
		AdminPassword: "pw",
	})

	// 合法 probe —— 后续坏输入用例都会在成员校验之前失败，
	// 但 confirm 放行和正常路径需要它通过 ValidateProbe。
	const validProbe = `"probe":{"type":"icmp","target":"1.1.1.1","interval_sec":5,"timeout_sec":3,"fail_threshold":2,"recover_threshold":2}`

	cases := []struct {
		name    string
		body    string
		want    int    // 期望 HTTP 状态码
		codeIn  string // 期望错误码 ("" 表示不检查)
	}{
		// ① members 为空数组
		{
			"空成员被拒（无 confirm）",
			`{"members":[],` + validProbe + `}`,
			http.StatusUnprocessableEntity, "10369",
		},
		{
			"空成员 confirm 放行",
			`{"members":[],` + validProbe + `,"confirm":true}`,
			http.StatusOK, "",
		},
		// ② 同名成员重复
		{
			"同名成员重复被拒",
			`{"members":[{"name":"wan1","iface":"eth0","gateway":"192.168.1.1","weight":50},{"name":"wan1","iface":"eth1","gateway":"192.168.2.1","weight":50}],` + validProbe + `}`,
			http.StatusUnprocessableEntity, "10370",
		},
		// ③ 同一 interface 出现在两个成员里
		{
			"重复接口被拒",
			`{"members":[{"name":"wan1","iface":"eth0","gateway":"192.168.1.1","weight":50},{"name":"wan2","iface":"eth0","gateway":"192.168.2.1","weight":50}],` + validProbe + `}`,
			http.StatusUnprocessableEntity, "10371",
		},
		// ④ weight 为负数
		{
			"负数权重被拒",
			`{"members":[{"name":"wan1","iface":"eth0","gateway":"192.168.1.1","weight":-5}],` + validProbe + `}`,
			http.StatusUnprocessableEntity, "10372",
		},
		// ④ weight 超过 100
		{
			"超限权重被拒",
			`{"members":[{"name":"wan1","iface":"eth0","gateway":"192.168.1.1","weight":101}],` + validProbe + `}`,
			http.StatusUnprocessableEntity, "10372",
		},
		// 1 个正常路径
		{
			"正常成员放行",
			`{"members":[{"name":"wan1","iface":"eth0","gateway":"192.168.1.1","weight":50,"enabled":true},{"name":"wan2","iface":"eth1","gateway":"192.168.2.1","weight":50,"enabled":true}],` + validProbe + `}`,
			http.StatusOK, "",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/multiwan", strings.NewReader(c.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			s.handleSetMultiWAN(rec, req)

			if rec.Code != c.want {
				t.Fatalf("HTTP %d，期望 %d，body=%s", rec.Code, c.want, rec.Body.String())
			}
			if c.codeIn != "" && !strings.Contains(rec.Body.String(), c.codeIn) {
				t.Fatalf("响应应含错误码 %s，实际 body=%s", c.codeIn, rec.Body.String())
			}
		})
	}
}
