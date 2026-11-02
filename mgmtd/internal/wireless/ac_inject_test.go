package wireless

import (
	"strings"
	"testing"
)

// hostapd.conf 以行为单位。密码里一个换行就能追加 "wpa=0"，
// 而 hostapd 对重复指令以最后一条为准 —— 加密配置在后面被直接关掉。
func TestRenderHostapdNoInjection(t *testing.T) {
	payloads := []string{
		"secret123\nwpa=0",
		"secret123\nwpa=0\n#",
		"secret123\r\nwpa=0",
		"secret123\ninterface=eth0",
	}
	for _, p := range payloads {
		out := RenderHostapd(SSID{Name: "Corp", Security: "wpa2", Password: p})
		for _, line := range strings.Split(out, "\n") {
			if strings.TrimSpace(line) == "wpa=0" {
				t.Fatalf("密码 %q 注入出了独占行的 wpa=0:\n%s", p, out)
			}
			if strings.TrimSpace(line) == "interface=eth0" {
				t.Fatalf("密码 %q 注入出了 interface 行:\n%s", p, out)
			}
		}
	}
}

func TestRenderHostapdNameNoInjection(t *testing.T) {
	out := RenderHostapd(SSID{Name: "Corp\nwpa=0", Security: "wpa2", Password: "secret123"})
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "wpa=0" {
			t.Fatalf("SSID 名称注入出了独占行 wpa=0:\n%s", out)
		}
	}
}

// 未知的 security 值以前落到 default 分支 = wpa=0。
// 传 "WPA2"（大写）就静默变成开放网络：管理端以为加密了，实际谁都能连。
func TestRenderHostapdUnknownSecurityFailsClosed(t *testing.T) {
	for _, sec := range []string{"WPA2", "wpa-psk", "WPA", "wep", "wpa2-psk"} {
		out := RenderHostapd(SSID{Name: "Corp", Security: sec, Password: "secret123"})
		for _, line := range strings.Split(out, "\n") {
			if strings.TrimSpace(line) == "wpa=0" {
				t.Fatalf("security=%q 回落成了开放网络:\n%s", sec, out)
			}
		}
		if !strings.Contains(out, "wpa=2") {
			t.Errorf("security=%q 应失败关闭为 wpa=2:\n%s", sec, out)
		}
	}
}

func TestRenderHostapdOpenIsExplicit(t *testing.T) {
	out := RenderHostapd(SSID{Name: "Guest", Security: "open"})
	if !strings.Contains(out, "wpa=0") {
		t.Errorf("显式 open 应渲染 wpa=0:\n%s", out)
	}
}

func TestValidateRejectsBadSecurity(t *testing.T) {
	base := SSID{Name: "Corp", Password: "secret123"}
	for _, sec := range []string{"WPA2", "wpa-psk", "wep", "WPA3-SAE"} {
		s := base
		s.Security = sec
		if err := Validate(s); err == nil {
			t.Errorf("security=%q 未被拒绝", sec)
		}
	}
}

func TestValidateRejectsControlCharsAndLength(t *testing.T) {
	cases := []SSID{
		{Name: "Corp\nwpa=0", Security: "wpa2", Password: "secret123"},
		{Name: "Corp", Security: "wpa2", Password: "secret\n123"},
		{Name: strings.Repeat("A", 33), Security: "wpa2", Password: "secret123"},
		{Name: "Corp", Security: "wpa2", Password: "short"},
		{Name: "Corp", Security: "wpa2", Password: strings.Repeat("A", 64)},
		{Name: "Corp", Security: "wpa2", Password: "secret123", VLAN: 5000},
	}
	for i, c := range cases {
		if err := Validate(c); err == nil {
			t.Errorf("用例 %d 未被拒绝: %+v", i, c)
		}
	}
}

func TestValidateAcceptsLegit(t *testing.T) {
	cases := []SSID{
		{Name: "Corp", Security: "wpa2", Password: "secret123"},
		{Name: "Corp-5G", Security: "wpa3", Password: "secret123", VLAN: 100},
		{Name: "Guest", Security: "open"},
		{Name: "Old", Security: "", Password: ""}, // 兼容：未指定视为 open
	}
	for i, c := range cases {
		if err := Validate(c); err != nil {
			t.Errorf("用例 %d 被误拒: %v", i, err)
		}
	}
}

// wpa_supplicant 用 ssid="..." / psk="..." 引号语法，
// 引号和反斜杠必须去掉，否则能闭合引号改写后面的键。
func TestRenderWpaSupplicantEscapesQuotes(t *testing.T) {
	out := RenderWpaSupplicant(SSID{Name: `Corp" key_mgmt=NONE "`, Security: "wpa2", Password: `p"ss\word`})
	if strings.Contains(out, `Corp"`) {
		t.Fatalf("名称里的引号未剔除:\n%s", out)
	}
	// 注入若成功，会多出一条以 key_mgmt= 开头的独立配置行
	var km int
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "key_mgmt=") {
			km++
		}
	}
	if km != 1 {
		t.Fatalf("应只有 1 条 key_mgmt 配置行，实际 %d:\n%s", km, out)
	}
}
