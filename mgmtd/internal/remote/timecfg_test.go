package remote

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateTimeOK(t *testing.T) {
	// "Asia/Shanghai" 在标准系统可用；若极端环境缺 zoneinfo 则跳过
	_, err := os.Stat(filepath.Join("/usr/share/zoneinfo", "Asia/Shanghai"))
	if err != nil {
		t.Skip("系统无 /usr/share/zoneinfo/Asia/Shanghai，跳过")
	}
	err = ValidateTime(TimeConfig{Timezone: "Asia/Shanghai", NTPServers: []string{"ntp.aliyun.com", "127.0.0.1"}})
	if err != nil {
		t.Fatalf("合法时区应通过, got %v", err)
	}
}

func TestValidateTimeNotExistingZone(t *testing.T) {
	err := ValidateTime(TimeConfig{Timezone: "Not/AZone"})
	if err == nil {
		t.Fatal("不存在的时区必须被拒绝")
	}
	if !strings.Contains(err.Error(), "不存在") {
		t.Fatalf("错误信息应说明时区不存在, got %v", err)
	}
}

func TestValidateTimeEmptyZone(t *testing.T) {
	err := ValidateTime(TimeConfig{})
	if err == nil {
		t.Fatal("空时区应被拒绝")
	}
}

func TestValidateTimeBadNTPServer(t *testing.T) {
	for _, bad := range []string{"", "has space", "bad///host", "x_y"} {
		err := ValidateTime(TimeConfig{Timezone: "UTC", NTPServers: []string{bad}})
		if err == nil {
			t.Fatalf("非法 NTP 主机 %q 应被拒绝", bad)
		}
	}
}

func TestValidateTimeGoodNTPServer(t *testing.T) {
	err := ValidateTime(TimeConfig{Timezone: "UTC", NTPServers: []string{"ntp.aliyun.com", "2001:4860:4860::8888", "192.168.1.1"}})
	if err != nil {
		t.Fatalf("合法 NTP 服务器应通过, got %v", err)
	}
}

// runnableRunner 用 stubRunner 模拟 ApplyTime 的回读行为。
func TestApplyTimeRereadInconsistent(t *testing.T) {
	r := &stubRunner{results: map[string]string{
		"timedatectl set-timezone Asia/Shanghai": "",
		// 回读故意返回另一个时区 —— 必须报错
		"timedatectl show -p Timezone --value": "UTC",
	}}
	err := ApplyTime(r, TimeConfig{Timezone: "Asia/Shanghai", Enabled: true})
	if err == nil {
		t.Fatal("时区回读不一致必须报错")
	}
	if !strings.Contains(err.Error(), "回读不一致") {
		t.Fatalf("错误信息应说明回读不一致, got %v", err)
	}
}

func TestApplyTimeRereadConsistent(t *testing.T) {
	r := &stubRunner{results: map[string]string{
		"timedatectl set-timezone Asia/Shanghai": "",
		"timedatectl show -p Timezone --value":   "Asia/Shanghai",
		"timedatectl set-ntp true":               "",
	}}
	err := ApplyTime(r, TimeConfig{Timezone: "Asia/Shanghai", Enabled: true})
	if err != nil {
		t.Fatalf("回读一致的场景应成功, got %v", err)
	}
	if !r.hasCall("timedatectl", "set-ntp", "true") {
		t.Fatalf("应执行 set-ntp true, calls=%v", r.calls)
	}
}

func TestApplyTimeSetTimeoutError(t *testing.T) {
	r := &stubRunner{nextErr: map[string]bool{"timedatectl set-timezone Asia/Shanghai": true}}
	err := ApplyTime(r, TimeConfig{Timezone: "Asia/Shanghai"})
	if err == nil {
		t.Fatal("set-timezone 失败应报错")
	}
}

func TestTimeStatusParse(t *testing.T) {
	r := &stubRunner{results: map[string]string{
		"timedatectl show": "Timezone=Asia/Shanghai\nLocalRTC=no\nNTP=yes\nNTPSynchronized=yes\nCanNTP=yes",
	}}
	st, err := TimeStatus(r)
	if err != nil {
		t.Fatalf("TimeStatus: %v", err)
	}
	if st["Timezone"] != "Asia/Shanghai" || st["NTP"] != "yes" || st["NTPSynchronized"] != "yes" {
		t.Fatalf("解析 timedatectl show 失败: %+v", st)
	}
}

func TestListTimezonesRootNotSkipped(t *testing.T) {
	dir := t.TempDir()
	// 构建 <area>/<zone> 结构，根目录 base name 与 "../zoneinfo" 不同名。
	if err := os.MkdirAll(filepath.Join(dir, "Asia"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Asia", "Shanghai"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldRoot := zoneinfoRoot
	zoneinfoRoot = dir
	defer func() { zoneinfoRoot = oldRoot }()
	zones, err := listTimezones()
	if err != nil {
		t.Fatalf("listTimezones: %v", err)
	}
	found := false
	for _, z := range zones {
		if z == "Asia/Shanghai" {
			found = true
		}
	}
	if !found {
		t.Fatalf("应包含 Asia/Shanghai, got %v", zones)
	}
}

func TestListTimezonesTruncation(t *testing.T) {
	// 模拟一个含 >600 时区文件的目录来验证截断提示。
	dir := t.TempDir()
	// 先放 610 个分区目录，每个一个文件
	count := 610
	for i := 0; i < count; i++ {
		sub := filepath.Join(dir, fmt.Sprintf("Area%d", i/100))
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		f := filepath.Join(sub, fmt.Sprintf("Zone%d", i))
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	oldRoot := zoneinfoRoot
	zoneinfoRoot = dir
	defer func() { zoneinfoRoot = oldRoot }()

	zones, err := listTimezones()
	if err != nil {
		t.Fatalf("listTimezones: %v", err)
	}
	if len(zones) == 0 {
		t.Fatal("应至少返回若干时区")
	}
	last := zones[len(zones)-1]
	if !strings.Contains(last, "截断") {
		t.Fatalf("超过上限应出现截断说明, 但最后一项=%q", last)
	}
	// 去掉说明后剩余 ≤上限
	real := zones[:len(zones)-1]
	if len(real) > maxTimezones {
		t.Fatalf("实际返回 %d 超过上限 %d", len(real), maxTimezones)
	}
}
