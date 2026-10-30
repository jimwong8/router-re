package vpnclient

import (
	"fmt"
	"regexp"
	"strings"
)

// OpenVPN 客户端配置的导入与校验。
//
// 这里最重要的不是"能不能连上"，而是**不要因为导入一份配置文件就把
// 路由器变成执行任意命令的入口**。OpenVPN 支持 up/down/plugin/tls-verify
// 等指令，它们都指向外部程序；一份恶意 .ovpn 上传后一旦连接，
// up 脚本就以 root 跑起来了——而用户以为自己只是"导入了一个 VPN 配置"。
//
// 所以策略是白名单化地拒绝：任何会让 OpenVPN 调起外部程序的指令一律拒绝，
// 并且连 script-security 都锁在 1（只允许内置的最小脚本能力）。

// forbiddenOvpn 是会调起外部程序或改变执行语义的指令。
//
// 键是去掉前导 -- 后的指令名，值告诉用户为什么被拒。
var forbiddenOvpn = map[string]string{
	"up":                    "会执行外部脚本",
	"down":                  "会执行外部脚本",
	"route-up":              "会执行外部脚本",
	"route-pre-down":        "会执行外部脚本",
	"ipchange":              "会执行外部脚本",
	"tls-verify":            "会执行外部脚本",
	"client-connect":        "会执行外部脚本",
	"client-disconnect":     "会执行外部脚本",
	"auth-user-pass-verify": "会执行外部脚本",
	"learn-address":         "会执行外部脚本",
	"plugin":                "会加载外部动态库",
	"tmp-dir":               "可让别人写入脚本目录",
	"script-security":       "本系统固定使用 1（只允许内置能力）",
	"setenv opt":            "可用于藏匿任意指令",
	"writepid":              "可覆盖系统文件路径",
	"log":                   "会写任意路径",
	"log-append":            "会写任意路径",
	"status":                "会写任意路径",
	"auth-user-pass":        "密码需由本页单独填写，不从配置里读文件",
	"auth-nocache":          "",
	"management":            "会开本地管理端口",
	"config":                "不允许嵌套加载其它配置",
}

// maxOvpnBytes 限制配置体积：正常的 .ovpn（含内联证书）几十 KB 足够。
const maxOvpnBytes = 512 * 1024

// OvpnDirective 是一行解析后的指令。
type OvpnDirective struct {
	Line int
	Name string
	Args string
	Raw  string
}

// ParseOvpn 把配置拆成指令行（跳过注释、空行、内联块标记）。
func ParseOvpn(text string) []OvpnDirective {
	var out []OvpnDirective
	inBlock := false
	for i, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		// 内联证书块：<ca> ... </ca>，块内内容按数据对待，不当指令解析
		if strings.HasPrefix(line, "<") && strings.HasSuffix(line, ">") {
			inBlock = !strings.HasPrefix(line, "</")
			continue
		}
		if inBlock {
			continue
		}
		fields := strings.Fields(strings.TrimPrefix(line, "--"))
		if len(fields) == 0 {
			continue
		}
		name := strings.ToLower(fields[0])
		args := ""
		if len(fields) > 1 {
			args = strings.Join(fields[1:], " ")
		}
		out = append(out, OvpnDirective{Line: i + 1, Name: name, Args: args, Raw: line})
	}
	return out
}

// remoteRe 匹配 `remote host [port] [proto]`。
var remoteRe = regexp.MustCompile(`^[A-Za-z0-9.-]{1,253}( [0-9]{1,5})?( (udp|tcp|udp4|tcp4|udp6|tcp6))?$`)

// ValidateOvpn 校验一份客户端配置文本。
//
// 返回的错误带行号，因为用户是从别处复制来的，得让他知道删哪一行。
func ValidateOvpn(text string) error {
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("配置内容为空")
	}
	if len(text) > maxOvpnBytes {
		return fmt.Errorf("配置超过 %d KiB，正常客户端配置不该这么大", maxOvpnBytes/1024)
	}

	dirs := ParseOvpn(text)
	if len(dirs) == 0 {
		return fmt.Errorf("没有解析到任何有效指令——检查是不是贴错了内容")
	}

	var hasClient, hasRemote, hasDev bool
	for _, d := range dirs {
		if reason, bad := forbiddenOvpn[d.Name]; bad {
			if reason == "" {
				return fmt.Errorf("第 %d 行 %q 不被接受（本系统统一由页面管理凭据）", d.Line, d.Name)
			}
			return fmt.Errorf("第 %d 行 %q 被拒绝：%s", d.Line, d.Name, reason)
		}
		switch d.Name {
		case "client", "tls-client", "pull":
			hasClient = true
		case "remote":
			hasRemote = true
			if !remoteRe.MatchString(d.Args) {
				return fmt.Errorf("第 %d 行 remote 参数 %q 不合法（形如 remote vpn.example.com 1194 udp）", d.Line, d.Args)
			}
		case "dev":
			hasDev = true
			// 只接受 tun/tap，或 tunN/tapN；不接受任意名字（会撞已有接口）
			if !regexp.MustCompile(`^(tun|tap)[0-9]{0,3}$`).MatchString(d.Args) {
				return fmt.Errorf("第 %d 行 dev %q 不被接受：只支持 tun/tap（可带编号）", d.Line, d.Args)
			}
		}
	}
	if !hasRemote {
		return fmt.Errorf("缺少 remote 指令，不知道该连哪台服务器")
	}
	if !hasDev {
		return fmt.Errorf("缺少 dev 指令（通常写 dev tun）")
	}
	if !hasClient {
		return fmt.Errorf("缺少 client 指令：这看起来不是客户端配置（服务端配置不能导入到客户端页）")
	}
	return nil
}

// OvpnRemote 取出 remote 的 host 与 port，供界面显示"要连到哪里"。
func OvpnRemote(text string) (host string, port int) {
	port = 1194
	for _, d := range ParseOvpn(text) {
		if d.Name != "remote" {
			continue
		}
		f := strings.Fields(d.Args)
		if len(f) == 0 {
			return "", 0
		}
		host = f[0]
		if len(f) > 1 {
			if p, err := parsePort(f[1]); err == nil {
				port = p
			}
		}
		return host, port
	}
	return "", 0
}

func parsePort(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("bad port")
		}
		n = n*10 + int(c-'0')
	}
	if n < 1 || n > 65535 {
		return 0, fmt.Errorf("port out of range")
	}
	return n, nil
}

// RenderOvpnUnit 生成 systemd 单元。配置文件路径固定，避免指令注入。
//
// 刻意不在 ExecStart 里拼接任何来自配置的内容：只写死 --config <路径>。
func RenderOvpnUnit(name, confPath string) (string, error) {
	if !pppNameRe.MatchString(name) {
		return "", fmt.Errorf("连接名不合法")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[Unit]\nDescription=router-re OpenVPN client (%s)\n", name)
	b.WriteString("After=network-online.target\nWants=network-online.target\n\n")
	b.WriteString("[Service]\nType=simple\n")
	fmt.Fprintf(&b, "ExecStart=/usr/sbin/openvpn --config %s\n", confPath)
	b.WriteString("Restart=on-failure\nRestartSec=10\n")
	// openvpn 需要有创建 tun 设备与改路由的权限，给最小集
	b.WriteString("CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW\n")
	b.WriteString("AmbientCapabilities=CAP_NET_ADMIN CAP_NET_RAW\n")
	b.WriteString("NoNewPrivileges=true\n")
	b.WriteString("ProtectSystem=strict\n")
	b.WriteString("ProtectHome=true\n")
	b.WriteString("PrivateTmp=true\n\n")
	b.WriteString("[Install]\nWantedBy=multi-user.target\n")
	return b.String(), nil
}
