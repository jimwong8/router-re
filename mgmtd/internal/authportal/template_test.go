package authportal

import (
	"strings"
	"testing"
)

// TestBuiltinAtLeast4 —— 至少 4 个，ID 唯一，Kind 都在白名单。
func TestBuiltinAtLeast4(t *testing.T) {
	bs := Builtin()
	if len(bs) < 4 {
		t.Fatalf("Builtin 应至少 4 个模板, got %d", len(bs))
	}
	seen := map[string]bool{}
	for _, b := range bs {
		if seen[b.ID] {
			t.Fatalf("内置模板 ID 重复: %q", b.ID)
		}
		seen[b.ID] = true
		if !kindWhitelist[b.Kind] {
			t.Fatalf("内置模板 kind 不在白名单: %q", b.Kind)
		}
		if err := Validate(b); err != nil {
			t.Fatalf("内置模板 %q 校验失败: %v", b.ID, err)
		}
	}
}

// TestValidateRejectBadColor —— 非法颜色 "red" 拒绝。
func TestValidateRejectBadColor(t *testing.T) {
	tmpl := Template{
		Kind: "password", Title: "测试", BgColor: "red", TextColor: "#ffffff",
	}
	if err := Validate(tmpl); err == nil {
		t.Fatal("非法颜色 red 应被拒绝")
	}
	tmpl.BgColor = "#ffffff"
	tmpl.TextColor = "red"
	if err := Validate(tmpl); err == nil {
		t.Fatal("非法 text_color red 应被拒绝")
	}
}

// TestValidateAcceptColors —— 接受 "#fff" 和 "#1a2b3c"。
func TestValidateAcceptColors(t *testing.T) {
	base := Template{Kind: "password", Title: "测试", TextColor: "#fff"}
	for _, c := range []string{"#fff", "#1a2b3c", "#FFF", "#a1b2c3"} {
		base.BgColor = c
		if err := Validate(base); err != nil {
			t.Fatalf("合法颜色 %q 应通过: %v", c, err)
		}
	}
}

// TestValidateRejectJavascriptRedirect —— 拒绝 javascript: 伪协议。
func TestValidateRejectJavascriptRedirect(t *testing.T) {
	tmpl := Template{
		Kind: "password", Title: "测试", BgColor: "#fff", TextColor: "#fff",
		RedirectURL: "javascript:alert(1)",
	}
	if err := Validate(tmpl); err == nil {
		t.Fatal("javascript: 伪协议的 redirect_url 必须被拒绝")
	}
	// ValidateRedirect 单独测
	if err := ValidateRedirect("javascript:alert(1)"); err == nil {
		t.Fatal("ValidateRedirect 应拒绝 javascript:")
	}
}

// TestRenderHTMLEscapesInjection —— 标题注入 <script> 必须被转义。
func TestRenderHTMLEscapesInjection(t *testing.T) {
	tmpl := Template{
		ID: "x", Kind: "password", Title: "<script>alert(1)</script>",
		BgColor: "#fff", TextColor: "#000",
		Notice: "<script>alert(2)</script>",
	}
	out := RenderHTML(tmpl)
	if strings.Contains(out, "<script>alert(1)</script>") {
		t.Fatalf("RenderHTML 输出绝不能含未转义的注入脚本:\n%s", out)
	}
	if !strings.Contains(out, "&lt;script&gt;") {
		t.Fatalf("RenderHTML 应含转义后的 &lt;script&gt;:\n%s", out)
	}
}

// TestRenderHTMLFormAction —— 含 POST 到 /portal/auth 的表单。
func TestRenderHTMLFormAction(t *testing.T) {
	tmpl := Template{Kind: "voucher", Title: "卡密", BgColor: "#fff", TextColor: "#000"}
	out := RenderHTML(tmpl)
	if !strings.Contains(out, `action="/portal/auth"`) || !strings.Contains(out, `method="post"`) {
		t.Fatalf("应含 POST 到 /portal/auth 的表单:\n%s", out)
	}
}

// TestValidateRedirectRejectsDataVbscript —— 拒绝 data: 和 vbscript:。
func TestValidateRedirectRejectsDataVbscript(t *testing.T) {
	for _, u := range []string{"data:text/html,<script>alert(1)</script>", "vbscript:msgbox(1)"} {
		if err := ValidateRedirect(u); err == nil {
			t.Fatalf("ValidateRedirect 应拒绝 %q", u)
		}
	}
}

// TestValidateRedirectAcceptsHTTPS —— 接受 https://example.com。
func TestValidateRedirectAcceptsHTTPS(t *testing.T) {
	if err := ValidateRedirect("https://example.com"); err != nil {
		t.Fatalf("https URL 应通过: %v", err)
	}
	if err := ValidateRedirect("http://portal.example.com/welcome"); err != nil {
		t.Fatalf("http URL 应通过: %v", err)
	}
	// 空串也应通过（未配置跳转）
	if err := ValidateRedirect(""); err != nil {
		t.Fatalf("空串应通过: %v", err)
	}
}
