// Package authportal 提供上网认证门户模板：内置模板清单、配置校验、以及
// 门户 HTML 的渲染。纯校验/渲染，不接服务器。
//
// 设计取舍（沿用 internal/nat 与 internal/porttrigger 的注释风格）：
//   - **XSS 红线**：管理后台可以把任何文本填进 Title / Notice / LogoURL /
//     RedirectURL。这些字段会拼进 HTML，一旦被填入 <script>alert(1)</script>，
//     所有连上 WiFi 并打开门户页的人都会中招。因此 RenderHTML 必须用
//     html.EscapeString 转义一切用户可控字段，并把该攻击场景写进注释。
//   - **伪协议红线**：RedirectURL 是认证成功后浏览器跳转的地址，若允许
//     javascript: / data: / vbscript:，等于给攻击者一条 XSS 通道。Validate 与
//     ValidateRedirect 都必须拒绝这些伪协议。
//   - 非法颜色（BgColor/TextColor）会让整页 CSS 解析失败导致门户白屏，
//     用正则严格校验 #RGB / #RRGGBB。
package authportal

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

// Template 是一个上网认证门户模板。
type Template struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"` // wechat / sms / password / voucher / onepress
	Title       string `json:"title"`
	LogoURL     string `json:"logo_url"`
	BgColor     string `json:"bg_color"`
	TextColor   string `json:"text_color"`
	Notice      string `json:"notice"`
	RedirectURL string `json:"redirect_url"` // 认证成功后跳转
}

// kindWhitelist 是允许的认证方式白名单。
var kindWhitelist = map[string]bool{
	"wechat": true, "sms": true, "password": true, "voucher": true, "onepress": true,
}

// colorRe 匹配 #RGB 或 #RRGGBB。
var colorRe = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

// httpRe 匹配 http:// 或 https:// 开头的 URL。
var httpRe = regexp.MustCompile(`^(https?://)`)

// Builtin 返回内置模板。至少 4 个：password / voucher / sms / wechat。
func Builtin() []Template {
	return []Template{
		{
			ID: "password", Name: "密码认证", Kind: "password",
			Title: "欢迎使用本网络", LogoURL: "",
			BgColor: "#1a2b3c", TextColor: "#ffffff",
			Notice:      "请输入管理员分配的密码完成认证",
			RedirectURL: "",
		},
		{
			ID: "voucher", Name: "卡密认证", Kind: "voucher",
			Title: "卡密认证", LogoURL: "",
			BgColor: "#2e8b57", TextColor: "#ffffff",
			Notice:      "请输入购买的卡密（券码）完成认证",
			RedirectURL: "",
		},
		{
			ID: "sms", Name: "短信认证", Kind: "sms",
			Title: "手机短信认证", LogoURL: "",
			BgColor: "#0a84ff", TextColor: "#ffffff",
			Notice:      "请输入手机号并获取验证码完成认证",
			RedirectURL: "",
		},
		{
			ID: "wechat", Name: "微信认证", Kind: "wechat",
			Title: "微信扫码认证", LogoURL: "",
			BgColor: "#07c160", TextColor: "#ffffff",
			Notice:      "请使用微信扫码关注后完成认证",
			RedirectURL: "",
		},
	}
}

// Validate 校验一个模板。
func Validate(t Template) error {
	if !kindWhitelist[t.Kind] {
		return fmt.Errorf("kind 必须是 wechat/sms/password/voucher/onepress 之一, got %q", t.Kind)
	}
	if strings.TrimSpace(t.Title) == "" {
		return fmt.Errorf("title 不能为空")
	}
	if len([]rune(t.Title)) > 64 {
		return fmt.Errorf("title 不能超过 64 字符, got %d", len([]rune(t.Title)))
	}
	if !colorRe.MatchString(t.BgColor) {
		return fmt.Errorf("bg_color 必须是 #RGB 或 #RRGGBB, got %q（非法颜色会让整个门户白屏）", t.BgColor)
	}
	if !colorRe.MatchString(t.TextColor) {
		return fmt.Errorf("text_color 必须是 #RGB 或 #RRGGBB, got %q（非法颜色会让整个门户白屏）", t.TextColor)
	}
	if t.LogoURL != "" && !httpRe.MatchString(t.LogoURL) {
		return fmt.Errorf("logo_url 必须为空或 http(s):// 开头, got %q", t.LogoURL)
	}
	if err := ValidateRedirect(t.RedirectURL); err != nil {
		return fmt.Errorf("redirect_url: %w", err)
	}
	return nil
}

// ValidateRedirect 单独校验跳转地址，拒绝 javascript: / data: / vbscript:。
// 只允许空串或 http(s):// 开头。
func ValidateRedirect(u string) error {
	if u == "" {
		return nil
	}
	lower := strings.ToLower(strings.TrimSpace(u))
	if strings.HasPrefix(lower, "javascript:") ||
		strings.HasPrefix(lower, "data:") ||
		strings.HasPrefix(lower, "vbscript:") {
		return fmt.Errorf("非法跳转协议（拒绝 javascript:/data:/vbscript:，这是 XSS 入口）: %q", u)
	}
	if !httpRe.MatchString(lower) {
		return fmt.Errorf("redirect_url 必须为空或 http(s):// 开头, got %q", u)
	}
	return nil
}

// RenderHTML 生成门户 HTML。
//
// **XSS 攻击场景**：管理后台把 <script>alert(1)</script> 填进 Title（或 Notice /
// LogoURL / RedirectURL），所有连上 WiFi 并打开门户页的人都会执行这段脚本。
// 因此这里用 html.EscapeString 转义所有用户可控字段，绝不过滤后直接拼接。
//
// 输出的 HTML 含一个 POST 到 /portal/auth 的表单；字段由 Kind 决定：
//   - password / voucher：单输入（密码 / 卡密）+ 提交按钮
//   - sms：手机号 + 验证码
//   - wechat / onepress：说明文字 + 提交按钮（跳转触发认证）
func RenderHTML(t Template) string {
	title := html.EscapeString(t.Title)
	notice := html.EscapeString(t.Notice)
	logoURL := html.EscapeString(t.LogoURL)
	redirect := html.EscapeString(t.RedirectURL)
	bg := html.EscapeString(t.BgColor)
	tc := html.EscapeString(t.TextColor)

	var inputs string
	switch t.Kind {
	case "sms":
		inputs = `
      <label>手机号</label>
      <input type="text" name="phone" placeholder="请输入手机号" required>
      <label>验证码</label>
      <input type="text" name="code" placeholder="请输入验证码" required>`
	case "wechat", "onepress":
		inputs = `
      <p class="hint">点击下方按钮完成一键认证</p>`
	default: // password / voucher 共用卡密/密码输入框
		inputs = `
      <label>` + map[string]string{
			"password": "密码",
			"voucher":  "卡密",
		}[t.Kind] + `</label>
      <input type="text" name="credential" placeholder="请输入" required>`
	}

	// 认证后跳转地址：作为隐藏字段随表单提交；空则留空。
	redirectField := ""
	if redirect != "" {
		redirectField = `    <input type="hidden" name="redirect" value="` + redirect + `">`
	}

	return `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>` + title + `</title>
<style>
  body { margin:0; font-family:-apple-system,"PingFang SC","Microsoft YaHei",sans-serif;
         background:` + bg + `; color:` + tc + `;
         display:flex; min-height:100vh; align-items:center; justify-content:center; }
  .card { background:rgba(255,255,255,.12); border-radius:12px; padding:32px 28px;
          width:320px; box-shadow:0 8px 30px rgba(0,0,0,.25); }
  .logo { max-height:64px; margin-bottom:8px; }
  h1 { font-size:20px; margin:0 0 8px; }
  .notice { font-size:13px; opacity:.9; margin-bottom:16px; white-space:pre-wrap; }
  label { display:block; font-size:13px; margin:10px 0 4px; }
  input { width:100%; box-sizing:border-box; padding:10px; border:1px solid rgba(255,255,255,.4);
          border-radius:6px; background:rgba(255,255,255,.95); color:#111; font-size:14px; }
  button { width:100%; margin-top:18px; padding:12px; border:0; border-radius:6px;
           background:#fff; color:#333; font-size:15px; font-weight:600; cursor:pointer; }
  .hint { font-size:14px; opacity:.95; }
</style>
</head>
<body>
  <form class="card" method="post" action="/portal/auth">
    <img class="logo" src="` + logoURL + `" alt="logo" onerror="this.style.display='none'">
    <h1>` + title + `</h1>
    <div class="notice">` + notice + `</div>
` + inputs + `
` + redirectField + `
    <button type="submit">认证登录</button>
  </form>
</body>
</html>`
}
