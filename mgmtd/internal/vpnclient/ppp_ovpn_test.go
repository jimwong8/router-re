package vpnclient

import (
	"strings"
	"testing"
)

func goodPPP(kind string) PPPSpec {
	return PPPSpec{
		Name: "office-vpn", Kind: kind, Server: "vpn.example.com",
		Username: "user1", Password: "s3cret!", RouteAll: true, Enabled: true,
	}
}

func TestValidatePPP(t *testing.T) {
	for _, kind := range []string{"pptp", "l2tp"} {
		if err := ValidatePPP(goodPPP(kind)); err != nil {
			t.Fatalf("%s 合法配置被拒: %v", kind, err)
		}
	}
	cases := []struct {
		name, kind string
		mod        func(*PPPSpec)
	}{
		{"类型非法", "sstp", func(s *PPPSpec) {}},
		{"名称为空", "pptp", func(s *PPPSpec) { s.Name = "" }},
		{"名称带斜杠", "pptp", func(s *PPPSpec) { s.Name = "a/b" }},
		{"名称带空格", "pptp", func(s *PPPSpec) { s.Name = "a b" }},
		{"服务器为空", "pptp", func(s *PPPSpec) { s.Server = "" }},
		{"服务器带空格", "pptp", func(s *PPPSpec) { s.Server = "1.2.3.4 --up" }},
		{"服务器含 ..", "pptp", func(s *PPPSpec) { s.Server = "a..b" }},
		{"用户名为空", "pptp", func(s *PPPSpec) { s.Username = "" }},
		{"用户名带换行", "pptp", func(s *PPPSpec) { s.Username = "u\n* * * *" }},
		{"用户名带空格", "pptp", func(s *PPPSpec) { s.Username = "u v" }},
		{"密码为空", "l2tp", func(s *PPPSpec) { s.Password = "" }},
		{"密码带换行", "l2tp", func(s *PPPSpec) { s.Password = "p\n* * * *" }},
		{"密码带引号", "l2tp", func(s *PPPSpec) { s.Password = `p"x` }},
		{"MTU 过小", "l2tp", func(s *PPPSpec) { s.MTU = 100 }},
		{"MTU 过大", "l2tp", func(s *PPPSpec) { s.MTU = 9000 }},
	}
	for _, c := range cases {
		s := goodPPP(c.kind)
		c.mod(&s)
		if err := ValidatePPP(s); err == nil {
			t.Fatalf("%s 必须被拒绝", c.name)
		}
	}
}

func TestChapSecretLine(t *testing.T) {
	s := goodPPP("pptp")
	line, err := ChapSecretLine(s)
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Split(line, "\t")
	if len(fields) != 4 {
		t.Fatalf("chap-secrets 必须是 4 个字段，实际 %d: %q", len(fields), line)
	}
	if fields[0] != "user1" || fields[1] != "PPTP" || fields[2] != "s3cret!" {
		t.Fatalf("字段内容错误: %q", line)
	}
	l2, _ := ChapSecretLine(goodPPP("l2tp"))
	if !strings.Contains(l2, "\tL2TP\t") {
		t.Fatalf("l2tp 的 remotename 错误: %q", l2)
	}
}

func TestChapSecretRejectsInjection(t *testing.T) {
	// 换行会让后面的内容变成一条新的通配凭据——等于给任何对端一个合法账号
	s := goodPPP("pptp")
	s.Password = "x\n* * * *"
	if _, err := ChapSecretLine(s); err == nil {
		t.Fatal("注入式密码必须被拒绝")
	}
}

func TestRenderPeers(t *testing.T) {
	s := goodPPP("pptp")
	out, err := RenderPeers(s)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`pty "pptp vpn.example.com --nolaunchpppd"`,
		"name user1",
		"remotename PPTP",
		"file /etc/ppp/options.pptp",
		"ipparam office-vpn",
		"persist",
		"maxfail 0",
		"defaultroute",
		"replacedefaultroute",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("peers 缺少 %q\n%s", want, out)
		}
	}
	// 密码绝不能出现在 peers 文件里（会被随手 cat 到工单）
	if strings.Contains(out, "s3cret!") {
		t.Fatalf("peers 文件不得含密码:\n%s", out)
	}
	// 不开全局路由时不该有 defaultroute
	s.RouteAll = false
	out2, _ := RenderPeers(s)
	if strings.Contains(out2, "defaultroute") {
		t.Fatalf("未开启全局走 VPN 时不该下发默认路由:\n%s", out2)
	}
	// MTU 透传
	s.MTU = 1400
	out3, _ := RenderPeers(s)
	if !strings.Contains(out3, "mtu 1400") || !strings.Contains(out3, "mru 1400") {
		t.Fatalf("MTU 未写入:\n%s", out3)
	}
}

func TestRenderPeersL2TP(t *testing.T) {
	out, err := RenderPeers(goodPPP("l2tp"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "remotename L2TP") || !strings.Contains(out, "options.l2tpd.client") {
		t.Fatalf("L2TP peers 内容错误:\n%s", out)
	}
	if strings.Contains(out, "pptp") {
		t.Fatalf("L2TP 配置里不该出现 pptp:\n%s", out)
	}
}

func TestRenderXl2tpd(t *testing.T) {
	s := goodPPP("l2tp")
	out, err := RenderXl2tpd(s)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"[lac office-vpn]", "lns = vpn.example.com", "name = user1",
		"pppoptfile = /etc/ppp/peers/office-vpn", "redial = yes"} {
		if !strings.Contains(out, want) {
			t.Fatalf("xl2tpd 缺少 %q\n%s", want, out)
		}
	}
	if _, err := RenderXl2tpd(goodPPP("pptp")); err == nil {
		t.Fatal("pptp 不该生成 xl2tpd 配置")
	}
}

func TestServiceName(t *testing.T) {
	if ServiceName(goodPPP("pptp")) != "rr-pptp-office-vpn" {
		t.Fatal("pptp 单元名错误")
	}
	if ServiceName(goodPPP("l2tp")) != "rr-l2tp-office-vpn" {
		t.Fatal("l2tp 单元名错误")
	}
}

// ---------- OpenVPN ----------

const goodOvpn = `client
dev tun
proto udp
remote vpn.example.com 1194
resolv-retry infinite
nobind
persist-key
persist-tun
verb 3
<ca>
-----BEGIN CERTIFICATE-----
MIIB
-----END CERTIFICATE-----
</ca>
`

func TestValidateOvpnAcceptsNormalConfig(t *testing.T) {
	if err := ValidateOvpn(goodOvpn); err != nil {
		t.Fatalf("正常客户端配置被拒: %v", err)
	}
}

func TestValidateOvpnRejectsScriptInjection(t *testing.T) {
	// 每一项都是"导入一份配置 → 连接时以 root 执行外部程序"的入口
	bad := map[string]string{
		"up":              goodOvpn + "up /tmp/evil.sh\n",
		"down":            goodOvpn + "down /tmp/evil.sh\n",
		"route-up":        goodOvpn + "route-up /tmp/evil.sh\n",
		"tls-verify":      goodOvpn + "tls-verify /tmp/evil.sh\n",
		"plugin":          goodOvpn + "plugin /tmp/evil.so\n",
		"ipchange":        goodOvpn + "ipchange /tmp/evil.sh\n",
		"learn-address":   goodOvpn + "learn-address /tmp/evil.sh\n",
		"client-connect":  goodOvpn + "client-connect /tmp/evil.sh\n",
		"带 -- 前缀的 up":     goodOvpn + "--up /tmp/evil.sh\n",
		"大写 UP":           goodOvpn + "UP /tmp/evil.sh\n",
		"script-security": goodOvpn + "script-security 3\n",
		"config 嵌套":       goodOvpn + "config /etc/shadow\n",
		"management 端口":   goodOvpn + "management 127.0.0.1 7505\n",
	}
	for name, text := range bad {
		if err := ValidateOvpn(text); err == nil {
			t.Fatalf("%s 必须被拒绝，否则导入配置即可执行任意命令", name)
		}
	}
}

func TestValidateOvpnInjectionErrorMentionsLine(t *testing.T) {
	text := "client\ndev tun\nremote a.example.com\nup /tmp/x.sh\n"
	err := ValidateOvpn(text)
	if err == nil {
		t.Fatal("应被拒绝")
	}
	if !strings.Contains(err.Error(), "第 4 行") {
		t.Fatalf("错误应指出行号，实际: %v", err)
	}
}

func TestValidateOvpnStructure(t *testing.T) {
	cases := []struct{ name, text string }{
		{"空内容", "  \n\n"},
		{"没有 remote", "client\ndev tun\n"},
		{"没有 dev", "client\nremote a.example.com\n"},
		{"只有服务端配置", "dev tun\nremote a.example.com\n<ca>\nxx\n</ca>\n"},
		{"remote 参数非法", "client\ndev tun\nremote a.example.com 1194 udp; rm -rf /\n"},
		{"dev 名字任意", "client\ndev eth0\nremote a.example.com\n"},
		{"dev 带路径", "client\ndev /dev/../../etc\nremote a.example.com\n"},
	}
	for _, c := range cases {
		if err := ValidateOvpn(c.text); err == nil {
			t.Fatalf("%s 必须被拒绝", c.name)
		}
	}
}

func TestValidateOvpnDevForms(t *testing.T) {
	for _, dev := range []string{"tun", "tap", "tun0", "tap12"} {
		text := "client\ndev " + dev + "\nremote a.example.com\n"
		if err := ValidateOvpn(text); err != nil {
			t.Fatalf("dev %s 应被接受: %v", dev, err)
		}
	}
}

func TestParseOvpnSkipsBlocksAndComments(t *testing.T) {
	text := "# 注释\n\n<ca>\nup /tmp/evil.sh\n</ca>\nclient\ndev tun\n"
	dirs := ParseOvpn(text)
	for _, d := range dirs {
		if d.Name == "up" {
			t.Fatal("内联证书块里的内容不该被当作指令解析")
		}
	}
	if len(dirs) != 2 {
		t.Fatalf("应解析出 2 条指令，实际 %d: %+v", len(dirs), dirs)
	}
}

func TestOvpnRemote(t *testing.T) {
	h, p := OvpnRemote(goodOvpn)
	if h != "vpn.example.com" || p != 1194 {
		t.Fatalf("remote 解析错误: %s:%d", h, p)
	}
	h2, p2 := OvpnRemote("client\ndev tun\nremote 1.2.3.4 443 tcp\n")
	if h2 != "1.2.3.4" || p2 != 443 {
		t.Fatalf("带端口 remote 解析错误: %s:%d", h2, p2)
	}
	if h3, _ := OvpnRemote("client\ndev tun\n"); h3 != "" {
		t.Fatal("没有 remote 时应返回空")
	}
}

func TestRenderOvpnUnit(t *testing.T) {
	unit, err := RenderOvpnUnit("office", "/etc/router-re/openvpn/office.ovpn")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(unit, "ExecStart=/usr/sbin/openvpn --config /etc/router-re/openvpn/office.ovpn") {
		t.Fatalf("ExecStart 错误:\n%s", unit)
	}
	// 必须做权限收敛：VPN 客户端不需要完整 root
	for _, want := range []string{"CAP_NET_ADMIN", "NoNewPrivileges=true", "ProtectSystem=strict"} {
		if !strings.Contains(unit, want) {
			t.Fatalf("单元缺少 %q", want)
		}
	}
	// 单元里不能出现任何来自配置的拼接内容
	if strings.Contains(unit, "up ") {
		t.Fatal("单元不该包含脚本指令")
	}
	if _, err := RenderOvpnUnit("../etc", "/x"); err == nil {
		t.Fatal("非法连接名应被拒绝")
	}
}
