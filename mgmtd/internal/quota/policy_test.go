package quota

import (
	"strings"
	"testing"
)

func validPolicy() Policy {
	return Policy{
		ID:         "p1",
		Name:       "test",
		Target:     "203.0.113.5",
		TargetType: TargetIP,
		RateMbit:   10,
		MaxConns:   100,
		Enabled:    true,
	}
}

func TestValidateIPOK(t *testing.T) {
	if err := Validate(validPolicy()); err != nil {
		t.Fatalf("合法 ip 策略报错: %v", err)
	}
}

func TestValidateCIDROK(t *testing.T) {
	p := validPolicy()
	p.TargetType = TargetCIDR
	p.Target = "10.0.0.0/8"
	if err := Validate(p); err != nil {
		t.Fatalf("合法 cidr 策略报错: %v", err)
	}
	// IPv6 CIDR 非 /0 也应合法
	p.Target = "2001:db8::/32"
	if err := Validate(p); err != nil {
		t.Fatalf("合法 v6 cidr 报错: %v", err)
	}
}

// TestValidateRejectsCIDRZero —— 最重要的红线测试：拒绝 cidr /0。
func TestValidateRejectsCIDRZero(t *testing.T) {
	for _, cidr := range []string{"0.0.0.0/0", "::/0"} {
		p := validPolicy()
		p.TargetType = TargetCIDR
		p.Target = cidr
		err := Validate(p)
		if err == nil {
			t.Fatalf("cidr %q 必须被拒绝", cidr)
		}
		if !strings.Contains(err.Error(), "限住") && !strings.Contains(err.Error(), "失联") {
			t.Fatalf("拒绝理由需说明为何危险: ip=%s err=%v", cidr, err)
		}
	}
}

func TestValidateMAC(t *testing.T) {
	p := validPolicy()
	p.TargetType = TargetMAC
	p.Target = "aa:bb:cc:dd:ee:ff"
	if err := Validate(p); err != nil {
		t.Fatalf("合法 mac 报错: %v", err)
	}
	p.Target = "not-a-mac"
	if err := Validate(p); err == nil {
		t.Fatal("非法 mac 应被拒绝")
	}
	p.Target = "aa:bb:cc:dd:ee:ff:11" // 7 字节
	if err := Validate(p); err == nil {
		t.Fatal("7 字节 mac 应被拒绝")
	}
}

func TestValidateApp(t *testing.T) {
	p := validPolicy()
	p.TargetType = TargetApp
	p.Target = "-" // app 维度 target 可为空占位，真正用的是 App
	p.App = "wechat"
	if err := Validate(p); err != nil {
		t.Fatalf("合法 app 报错: %v", err)
	}
	// app 名只能字母数字和 -_
	p.App = "bad app!"
	if err := Validate(p); err == nil {
		t.Fatal("含非法字符的 app 名应被拒绝")
	}
}

// app 字段只对 app 维度有意义：其它维度非空即报错。
func TestValidateAppFieldOnlyForAppTarget(t *testing.T) {
	p := validPolicy() // ip 维度
	p.App = "wechat"
	if err := Validate(p); err == nil {
		t.Fatal("ip 维度不应允许 app 字段")
	}
}

func TestValidateBadTargetType(t *testing.T) {
	p := validPolicy()
	p.TargetType = "vlan"
	if err := Validate(p); err == nil {
		t.Fatal("非法 target_type 应被拒绝")
	}
}

func TestValidateEmptyIDName(t *testing.T) {
	p := validPolicy()
	p.ID = ""
	if err := Validate(p); err == nil {
		t.Fatal("缺 id 应报错")
	}
	p = validPolicy()
	p.Name = ""
	if err := Validate(p); err == nil {
		t.Fatal("缺 name 应报错")
	}
}

func TestValidateRates(t *testing.T) {
	p := validPolicy()
	p.RateMbit = 200000 // >100000
	if err := Validate(p); err == nil {
		t.Fatal("rate>100000 应拒绝")
	}
	p = validPolicy()
	p.BurstKB = 99999 // >65535
	if err := Validate(p); err == nil {
		t.Fatal("burst>65535 应拒绝")
	}
	p = validPolicy()
	p.MaxConns = 200000
	if err := Validate(p); err == nil {
		t.Fatal("max_conns>100000 应拒绝")
	}
	p = validPolicy()
	p.MaxConnsPerSec = 20000
	if err := Validate(p); err == nil {
		t.Fatal("max_conns_per_sec>10000 应拒绝")
	}
}

// TestValidateRejectsEmptyPolicy —— 空策略（三项全 0）是假配置，必须拒绝。
func TestValidateRejectsEmptyPolicy(t *testing.T) {
	p := validPolicy()
	p.RateMbit = 0
	p.MaxConns = 0
	p.MaxConnsPerSec = 0
	if err := Validate(p); err == nil {
		t.Fatal("空策略（三项全 0）必须拒绝")
	}
}

// rate=0 但 max_conns>0 合法（只限连接数）。
func TestValidateRateZeroAllowedWithConns(t *testing.T) {
	p := validPolicy()
	p.RateMbit = 0
	p.MaxConns = 50
	p.MaxConnsPerSec = 0
	if err := Validate(p); err != nil {
		t.Fatalf("rate=0 但有连接数上限应合法: %v", err)
	}
}
