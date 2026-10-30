package vpn

import (
	"strings"
	"testing"
)

func TestParseAddrPool(t *testing.T) {
	cases := []struct {
		raw     string
		wantN   int
		wantErr bool
	}{
		{"192.168.20.100-192.168.20.200", 1, false},
		{"192.168.20.100-200", 1, false},
		{"192.168.20.100-192.168.20.110,192.168.21.100-192.168.21.110", 2, false},
		{"", 0, false},
		{"abc", 0, true},
		{"192.168.20.200-192.168.20.100", 0, true},
		{"192.168.20.1-192.168.20.255", 1, false},
	}
	for _, c := range cases {
		p, err := ParseAddrPool(c.raw)
		if (err != nil) != c.wantErr {
			t.Fatalf("ParseAddrPool(%q) err=%v wantErr=%v", c.raw, err, c.wantErr)
		}
		if !c.wantErr && len(p.Ranges) != c.wantN {
			t.Fatalf("ParseAddrPool(%q) ranges=%d want %d", c.raw, len(p.Ranges), c.wantN)
		}
	}
}

func TestAddrPoolContains(t *testing.T) {
	p, err := ParseAddrPool("192.168.20.100-192.168.20.110")
	if err != nil {
		t.Fatal(err)
	}
	if !p.Contains("192.168.20.105") {
		t.Fatal("应包含 192.168.20.105")
	}
	if p.Contains("192.168.20.111") {
		t.Fatal("不应包含 192.168.20.111")
	}
	if p.Contains("not-an-ip") {
		t.Fatal("非法 IP 不应命中")
	}
	if p.Total() != 11 {
		t.Fatalf("Total=%d want 11", p.Total())
	}
}

func TestAddrPoolRejectsNetworkAndBroadcast(t *testing.T) {
	p, _ := ParseAddrPool("192.168.20.0-192.168.20.50")
	if err := p.Validate(true); err == nil {
		t.Fatal("包含 .0 网络号应报错")
	}
	p2, _ := ParseAddrPool("192.168.20.200-192.168.20.255")
	if err := p2.Validate(true); err == nil {
		t.Fatal("包含 .255 广播号应报错")
	}
}

func validPPTP() PPTPConfig {
	return PPTPConfig{
		Enabled:    true,
		ServerIP:   "192.168.20.1",
		ServerPort: 1723,
		AddrPool:   "192.168.20.100-192.168.20.150",
		DNS1:       "223.5.5.5",
		DNS2:       "119.29.29.29",
		OpenMPPE:   2,
		MTU:        1400,
		MRU:        1400,
		Users: []VPNUser{
			{Username: "alice", Password: "secret123"},
			{Username: "bob", Password: "secret456", IP: "192.168.20.101"},
		},
	}
}

func TestValidatePPTP(t *testing.T) {
	if err := ValidatePPTP(validPPTP()); err != nil {
		t.Fatalf("合法配置被拒: %v", err)
	}

	// 服务端地址落在地址池内
	c := validPPTP()
	c.ServerIP = "192.168.20.120"
	if err := ValidatePPTP(c); err == nil {
		t.Fatal("server_ip 在池内应报错")
	}

	// MTU 越界（iKuai 上限 1492）
	c = validPPTP()
	c.MTU = 1500
	if err := ValidatePPTP(c); err == nil {
		t.Fatal("MTU=1500 应报错（上限 1492）")
	}

	// open_mppe 越界
	c = validPPTP()
	c.OpenMPPE = 3
	if err := ValidatePPTP(c); err == nil {
		t.Fatal("open_mppe=3 应报错")
	}

	// 弱口令
	c = validPPTP()
	c.Users[0].Password = "123"
	if err := ValidatePPTP(c); err == nil {
		t.Fatal("弱口令应报错")
	}

	// 重复账号
	c = validPPTP()
	c.Users[1].Username = "ALICE"
	if err := ValidatePPTP(c); err == nil {
		t.Fatal("大小写不同的同名账号应报错")
	}

	// 固定地址不在池内
	c = validPPTP()
	c.Users[1].IP = "192.168.99.5"
	if err := ValidatePPTP(c); err == nil {
		t.Fatal("固定地址越池应报错")
	}

	// 非法 DNS
	c = validPPTP()
	c.DNS1 = "300.1.1.1"
	if err := ValidatePPTP(c); err == nil {
		t.Fatal("非法 DNS 应报错")
	}
}

func TestRenderPPTP(t *testing.T) {
	c := validPPTP()
	conf := RenderPptpdConf(c)
	for _, want := range []string{"localip 192.168.20.1", "remoteip 192.168.20.100-192.168.20.150"} {
		if !strings.Contains(conf, want) {
			t.Fatalf("pptpd.conf 缺少 %q:\n%s", want, conf)
		}
	}
	opt := RenderPptpdOptions(c)
	for _, want := range []string{"require-mschap-v2", "require-mppe-128", "ms-dns 223.5.5.5", "mtu 1400"} {
		if !strings.Contains(opt, want) {
			t.Fatalf("pptpd-options 缺少 %q:\n%s", want, opt)
		}
	}
	sec := RenderChapSecrets(c.Users)
	if !strings.Contains(sec, "alice\t*\tsecret123\t*") {
		t.Fatalf("chap-secrets 格式不对:\n%s", sec)
	}
	if !strings.Contains(sec, "bob\t*\tsecret456\t192.168.20.101") {
		t.Fatalf("固定地址账号第4列应为该地址:\n%s", sec)
	}
	// 顺序稳定
	if RenderChapSecrets(c.Users) != sec {
		t.Fatal("chap-secrets 输出应稳定")
	}
}

func validL2TP() L2TPConfig {
	return L2TPConfig{
		Enabled:     true,
		ServerIP:    "192.168.30.1",
		AddrPool:    "192.168.30.100-192.168.30.150",
		IPSecSecret: "VeryStrongPSK2026",
		ForceIPSec:  1,
		DNS1:        "223.5.5.5",
		MTU:         1400,
		Users:       []VPNUser{{Username: "l2tpuser", Password: "secret123"}},
	}
}

func TestValidateL2TP(t *testing.T) {
	if err := ValidateL2TP(validL2TP()); err != nil {
		t.Fatalf("合法配置被拒: %v", err)
	}

	c := validL2TP()
	c.IPSecSecret = ""
	if err := ValidateL2TP(c); err == nil {
		t.Fatal("force_ipsec=1 且无 PSK 应报错")
	}

	c = validL2TP()
	c.IPSecSecret = "short"
	if err := ValidateL2TP(c); err == nil {
		t.Fatal("PSK 少于 8 位应报错")
	}

	// 显式允许纯 L2TP 时无 PSK 可以
	c = validL2TP()
	c.IPSecSecret = ""
	c.ForceIPSec = 0
	if err := ValidateL2TP(c); err != nil {
		t.Fatalf("force_ipsec=0 无 PSK 应放行: %v", err)
	}

	c = validL2TP()
	c.MTU = 1500
	if err := ValidateL2TP(c); err == nil {
		t.Fatal("MTU=1500 应报错")
	}
}

func TestRenderL2TP(t *testing.T) {
	c := validL2TP()
	conf := RenderXl2tpdConf(c)
	for _, want := range []string{"port = 1701", "ip range = 192.168.30.100-192.168.30.150", "local ip = 192.168.30.1"} {
		if !strings.Contains(conf, want) {
			t.Fatalf("xl2tpd.conf 缺少 %q:\n%s", want, conf)
		}
	}
	ipsec := RenderIPSecConf(c)
	for _, want := range []string{"conn L2TP-PSK", "leftid=192.168.30.1", "rightprotoport=17/1701"} {
		if !strings.Contains(ipsec, want) {
			t.Fatalf("ipsec 配置缺少 %q:\n%s", want, ipsec)
		}
	}
	sec := RenderIPSecSecrets(c)
	if !strings.Contains(sec, `PSK "VeryStrongPSK2026"`) {
		t.Fatalf("PSK 行不对:\n%s", sec)
	}
}

func validOpenVPN() OpenVPNConfig {
	return OpenVPNConfig{
		Enabled:    true,
		Subnet:     "10.8.0.0",
		Mask:       "255.255.255.0",
		Proto:      "udp",
		Port:       1194,
		TunMTU:     1420,
		Cipher:     "AES-256-GCM",
		DevType:    "tun",
		Topology:   "subnet",
		Method:     0,
		DNS1:       "223.5.5.5",
		PushRoutes: []string{"192.168.1.0/24"},
	}
}

func TestValidateOpenVPN(t *testing.T) {
	if err := ValidateOpenVPN(validOpenVPN()); err != nil {
		t.Fatalf("合法配置被拒: %v", err)
	}

	// 拒绝已不安全的算法
	c := validOpenVPN()
	c.Cipher = "DES-CBC"
	if err := ValidateOpenVPN(c); err == nil {
		t.Fatal("DES-CBC 应被拒绝")
	}
	c.Cipher = "BF-CBC"
	if err := ValidateOpenVPN(c); err == nil {
		t.Fatal("BF-CBC 应被拒绝")
	}

	c = validOpenVPN()
	c.Mask = "255.255.0.0" // /16 允许
	if err := ValidateOpenVPN(c); err != nil {
		t.Fatalf("/16 应允许: %v", err)
	}
	c.Mask = "255.255.255.252" // /30 允许
	if err := ValidateOpenVPN(c); err != nil {
		t.Fatalf("/30 应允许: %v", err)
	}
	c.Mask = "255.255.255.254" // /31 拒绝
	if err := ValidateOpenVPN(c); err == nil {
		t.Fatal("/31 应拒绝")
	}

	c = validOpenVPN()
	c.Method = 1
	if err := ValidateOpenVPN(c); err == nil {
		t.Fatal("method=1 无账号应报错")
	}
	c.Users = []VPNUser{{Username: "u1", Password: "secret123"}}
	if err := ValidateOpenVPN(c); err != nil {
		t.Fatalf("method=1 有账号应通过: %v", err)
	}

	c = validOpenVPN()
	c.Proto = "sctp"
	if err := ValidateOpenVPN(c); err == nil {
		t.Fatal("非法协议应报错")
	}

	c = validOpenVPN()
	c.PushRoutes = []string{"not-a-cidr"}
	if err := ValidateOpenVPN(c); err == nil {
		t.Fatal("非法推送路由应报错")
	}
}

func TestRenderOpenVPN(t *testing.T) {
	c := validOpenVPN()
	conf := RenderOpenVPNServerConf(c)
	for _, want := range []string{
		"port 1194", "proto udp", "dev tun", "topology subnet",
		"server 10.8.0.0 255.255.255.0", "tun-mtu 1420",
		"data-ciphers AES-256-GCM", `push "dhcp-option DNS 223.5.5.5"`,
		`push "route 192.168.1.0/24"`, "allow-compression no",
	} {
		if !strings.Contains(conf, want) {
			t.Fatalf("server.conf 缺少 %q:\n%s", want, conf)
		}
	}
	// CBC 算法走 cipher 而不是 data-ciphers
	c2 := validOpenVPN()
	c2.Cipher = "AES-256-CBC"
	if got := RenderOpenVPNServerConf(c2); !strings.Contains(got, "cipher AES-256-CBC") {
		t.Fatalf("CBC 应输出 cipher 指令:\n%s", got)
	}
	// 客户端配置内联证书
	client := RenderOpenVPNClientConf(c, "phone", "CA-BODY", "CERT-BODY", "KEY-BODY", "TLS-BODY", "vpn.example.com")
	for _, want := range []string{"client", "remote vpn.example.com 1194", "<ca>", "</ca>", "<key>", "# client: phone"} {
		if !strings.Contains(client, want) {
			t.Fatalf("客户端配置缺少 %q:\n%s", want, client)
		}
	}
}
