package wechat

// 微信 OAuth 客户端的单元测试。
//
// 之前这个包一行测试都没有（验证器却给了 PASS）——因为上游地址硬编码在
// 函数体里，httptest 拦不住。BaseURL 注入是让这段代码可测的前提。

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	c := NewClient("wxappid", "wxsecret")
	c.BaseURL = srv.URL
	return c, srv
}

func TestCode2SessionSuccess(t *testing.T) {
	var gotQuery = map[string]string{}
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sns/oauth2/access_token" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		q := r.URL.Query()
		gotQuery["appid"] = q.Get("appid")
		gotQuery["grant_type"] = q.Get("grant_type")
		gotQuery["code"] = q.Get("code")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"openid":"o-abc","access_token":"at-123"}`))
	})
	defer srv.Close()

	s, err := c.Code2Session("CODE-1")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if s.OpenID != "o-abc" || s.AccessToken != "at-123" {
		t.Fatalf("解析错误: %+v", s)
	}
	if gotQuery["appid"] != "wxappid" || gotQuery["grant_type"] != "authorization_code" {
		t.Fatalf("参数没带上: %+v", gotQuery)
	}
}

// code 里出现 + / = 时必须原样送达，不能被 URL 拼接截断。
func TestCode2SessionEscapesCode(t *testing.T) {
	const tricky = "a+b/c=d&evil=1"
	var got string
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query().Get("code")
		if r.URL.Query().Get("evil") != "" {
			t.Errorf("code 里的 & 被当成了参数分隔符（注入）")
		}
		_, _ = w.Write([]byte(`{"openid":"o","access_token":"a"}`))
	})
	defer srv.Close()

	if _, err := c.Code2Session(tricky); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != tricky {
		t.Fatalf("code 转义丢失: want %q got %q", tricky, got)
	}
}

// 上游 errcode 必须转成 ErrCodeError，不能当成成功。
func TestCode2SessionUpstreamErrCode(t *testing.T) {
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"errcode":40029,"errmsg":"invalid code"}`))
	})
	defer srv.Close()

	_, err := c.Code2Session("bad")
	if err == nil {
		t.Fatal("errcode != 0 时必须报错")
	}
	ec, ok := err.(*ErrCodeError)
	if !ok {
		t.Fatalf("错误类型应为 *ErrCodeError，got %T: %v", err, err)
	}
	if ec.ErrCode != 40029 || !strings.Contains(ec.Error(), "40029") {
		t.Fatalf("errcode 丢失: %+v", ec)
	}
}

// 上游 5xx（例如微信侧故障返回 HTML）必须给出可读错误，不能伪装成"解析失败"。
func TestCode2SessionUpstream5xx(t *testing.T) {
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(502)
		_, _ = w.Write([]byte("<html>bad gateway</html>"))
	})
	defer srv.Close()

	_, err := c.Code2Session("x")
	if err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("5xx 应带状态码，got %v", err)
	}
}
