package remote

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestValidateWOLOK(t *testing.T) {
	err := ValidateWOL(WOLTarget{Name: "nas", MAC: "aa:bb:cc:dd:ee:ff", Iface: "eth0", Broadcast: "192.168.1.255"})
	if err != nil {
		t.Fatalf("合法目标报错: %v", err)
	}
}

func TestValidateWOLBadName(t *testing.T) {
	// 空名
	if err := ValidateWOL(WOLTarget{Name: "", MAC: "aa:bb:cc:dd:ee:ff"}); err == nil {
		t.Fatal("空 name 应被拒绝")
	}
	// 超长
	long := strings.Repeat("x", 65)
	if err := ValidateWOL(WOLTarget{Name: long, MAC: "aa:bb:cc:dd:ee:ff"}); err == nil {
		t.Fatal("超 64 字符 name 应被拒绝")
	}
}

func TestValidateWOLBadMAC(t *testing.T) {
	for _, mac := range []string{"", "notamac", "aa:bb:cc:dd:ee", "aa:bb:cc:dd:ee:ff:11"} {
		err := ValidateWOL(WOLTarget{Name: "x", MAC: mac})
		if err == nil {
			t.Fatalf("非法 MAC %q 应被拒绝", mac)
		}
	}
}

func TestValidateWOLBadBroadcast(t *testing.T) {
	for _, bc := range []string{"999.1.1.1", "notanip", "fe80::1", "192.168.1.1/24"} {
		err := ValidateWOL(WOLTarget{Name: "x", MAC: "aa:bb:cc:dd:ee:ff", Broadcast: bc})
		if err == nil {
			t.Fatalf("非法广播地址 %q 应被拒绝", bc)
		}
	}
}

func TestValidateWOLBroadcastEmpty(t *testing.T) {
	// 广播可省略
	if err := ValidateWOL(WOLTarget{Name: "x", MAC: "aa:bb:cc:dd:ee:ff"}); err != nil {
		t.Fatalf("空广播应允许, got %v", err)
	}
}

func TestWakeBothToolsMissing(t *testing.T) {
	// 让 MustLookup 返回空：需要能注入。这里直接用 stubRunner 让两个命令都失败，
	// 且不实际依赖系统 —— 但 "均未安装" 分支依赖 execx.MustLookup。
	// 为可测，改走 iface 解析失败确认 etherwake 路径传参正确。
	r := &stubRunner{nextErr: map[string]bool{
		"etherwake -i eth0 aa:bb:cc:dd:ee:ff": true,
		"wakeonlan aa:bb:cc:dd:ee:ff":         true,
	}}
	// 由于 stubRunner 的命令不会真的被 execx.MustLookup 看到（那查的是 PATH），
	// 我们只验证 Wake 走了两个尝试并最终报错。
	err := Wake(r, WOLTarget{Name: "x", MAC: "aa:bb:cc:dd:ee:ff", Iface: "eth0"})
	if err == nil {
		t.Fatal("两个工具都失败应返回错误")
	}
	if !r.hasCall("etherwake", "-i", "eth0") {
		t.Fatalf("应优先尝试 etherwake -i eth0, calls=%v", r.calls)
	}
	if !r.hasCall("wakeonlan") {
		t.Fatalf("etherwake 失败后应尝试 wakeonlan, calls=%v", r.calls)
	}
}

// TestWakeEtherwakeSuccess 验证 etherwake 成功即不再尝试 wakeonlan。
func TestWakeEtherwakeSuccess(t *testing.T) {
	r := &stubRunner{nextErr: map[string]bool{
		"wakeonlan aa:bb:cc:dd:ee:ff": true, // 不应被触发
	}}
	err := Wake(r, WOLTarget{Name: "x", MAC: "aa:bb:cc:dd:ee:ff", Iface: "eth0"})
	if err != nil {
		t.Fatalf("etherwake 成功不应报错, got %v", err)
	}
	// 只调用 etherwake，不应调用 wakeonlan
	for _, c := range r.calls {
		if c[0] == "wakeonlan" {
			t.Fatalf("etherwake 成功后不应调用 wakeonlan, calls=%v", r.calls)
		}
	}
}

func TestSaveLoadTargets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "targets.json")
	targets := []WOLTarget{
		{Name: "nas", MAC: "aa:bb:cc:dd:ee:ff", Iface: "eth0"},
		{Name: "pc", MAC: "00:11:22:33:44:55", Broadcast: "192.168.1.255"},
	}
	if err := SaveTargets(path, targets); err != nil {
		t.Fatalf("SaveTargets: %v", err)
	}
	// 权限 0600
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Fatalf("权限应为 0600, got %o", perm)
	}
	got, err := LoadTargets(path)
	if err != nil {
		t.Fatalf("LoadTargets: %v", err)
	}
	if !reflect.DeepEqual(got, targets) {
		t.Fatalf("读取结果不一致:\n got %+v\nwant %+v", got, targets)
	}
}

func TestLoadTargetsMissingFile(t *testing.T) {
	got, err := LoadTargets("/nonexistent/definitely/missing.json")
	if err != nil {
		t.Fatalf("文件不存在应返回空切片而非错误, got %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("应返回空切片, got %+v", got)
	}
}

func TestLoadTargetsEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.json")
	if err := os.WriteFile(path, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadTargets(path)
	if err != nil {
		t.Fatalf("空文件应返回空切片, got %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("空文件应返回空, got %+v", got)
	}
}
