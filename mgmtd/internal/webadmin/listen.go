package webadmin

import (
	"fmt"
	"net"
	"strings"
)

// ListenConfig 描述 Web 管理端的监听配置。
type ListenConfig struct {
	HTTPPort       int    `json:"http_port"`        // 80 或 0(关闭)
	HTTPSPort      int    `json:"https_port"`       // 443 或 0(关闭)
	BindAddr       string `json:"bind_addr"`        // 0.0.0.0 或具体内网 IP
	RedirectHTTP   bool   `json:"redirect_http"`    // HTTP 是否 301 到 HTTPS
	IdleTimeoutSec int    `json:"idle_timeout_sec"` // 60-86400
}

// ValidateListen 校验监听配置。
func ValidateListen(c ListenConfig) error {
	if c.HTTPPort < 0 || c.HTTPPort > 65535 {
		return fmt.Errorf("http_port must be 0 or 1-65535, got %d", c.HTTPPort)
	}
	if c.HTTPSPort < 0 || c.HTTPSPort > 65535 {
		return fmt.Errorf("https_port must be 0 or 1-65535, got %d", c.HTTPSPort)
	}
	if c.HTTPPort == 0 && c.HTTPSPort == 0 {
		return fmt.Errorf("http_port and https_port cannot both be 0 — that would lock you out of the management interface entirely")
	}
	if c.HTTPPort != 0 && c.HTTPSPort != 0 && c.HTTPPort == c.HTTPSPort {
		return fmt.Errorf("http_port and https_port must differ, both are %d", c.HTTPPort)
	}
	bind := strings.TrimSpace(c.BindAddr)
	if bind == "" {
		bind = "0.0.0.0"
	}
	ip := net.ParseIP(bind)
	if ip == nil {
		return fmt.Errorf("bind_addr %q is not a valid IP", bind)
	}
	// 明文 HTTP 暴露到所有接口 + 无 HTTPS = 密码/SSO 明文传输风险。
	if bind == "0.0.0.0" && c.HTTPSPort == 0 {
		return fmt.Errorf("refusing bind_addr=0.0.0.0 with https_port=0: plaintext HTTP would be exposed on every interface, leaking credentials (SSO/passwords) in clear text; enable HTTPS or bind to a specific internal IP")
	}
	if c.IdleTimeoutSec != 0 && (c.IdleTimeoutSec < 60 || c.IdleTimeoutSec > 86400) {
		return fmt.Errorf("idle_timeout_sec must be 0 or in [60,86400], got %d", c.IdleTimeoutSec)
	}
	return nil
}

// RenderNginx 生成 nginx server 块。
//
// HTTPS 时引用 /etc/ssl/router-re/{cert,key}.pem。
// redirect_http 时 HTTP server 块只做 return 301。
func RenderNginx(c ListenConfig) string {
	bind := strings.TrimSpace(c.BindAddr)
	if bind == "" {
		bind = "0.0.0.0"
	}
	var b strings.Builder
	if c.HTTPPort > 0 {
		b.WriteString("server {\n")
		fmt.Fprintf(&b, "    listen %s:%d;\n", bind, c.HTTPPort)
		if c.RedirectHTTP && c.HTTPSPort > 0 {
			fmt.Fprintf(&b, "    return 301 https://$host:%d$request_uri;\n", c.HTTPSPort)
		} else {
			b.WriteString("    location / {\n")
			b.WriteString("        proxy_pass http://127.0.0.1:8080;\n")
			b.WriteString("    }\n")
		}
		b.WriteString("}\n")
	}
	if c.HTTPSPort > 0 {
		b.WriteString("server {\n")
		fmt.Fprintf(&b, "    listen %s:%d ssl;\n", bind, c.HTTPSPort)
		b.WriteString("    ssl_certificate /etc/ssl/router-re/cert.pem;\n")
		b.WriteString("    ssl_certificate_key /etc/ssl/router-re/key.pem;\n")
		b.WriteString("    location / {\n")
		b.WriteString("        proxy_pass http://127.0.0.1:8080;\n")
		b.WriteString("    }\n")
		b.WriteString("}\n")
	}
	return b.String()
}
