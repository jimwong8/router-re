package qos

import (
	"strings"
	"testing"
)

// Apply 把渲染出的 tc 命令用 strings.Fields 重新分词后执行，所以接口名里
// 出现空格就等于追加了额外的 tc 参数；出现 ; | $ 等字符在别的执行路径上
// 也可能被解释。字符白名单必须挡住全部这些形态。
func TestApplyRejectsInjectedIface(t *testing.T) {
	payloads := []string{
		"eth0 root handle 1: htb",          // 追加 tc 参数
		"eth0; touch /tmp/pwn",             // shell 元字符
		"eth0 | id",                        // 管道
		"$(id)",                            // 命令替换
		"`id`",                             // 反引号
		"eth0\neth1",                       // 换行
		"../../etc/passwd",                 // 路径穿越
		"-f",                               // 以 - 开头会被当选项
		"eth0'",                            // 引号
	}
	for _, p := range payloads {
		err := Apply([]RateLimit{{Interface: p, RateMbps: 10, BurstKB: 32, Direction: "egress"}})
		if err == nil {
			t.Errorf("接口名 %q 应被拒绝", p)
			continue
		}
		if !strings.Contains(err.Error(), "invalid interface name") {
			t.Errorf("接口名 %q 的报错应指明非法接口名，实际: %v", p, err)
		}
	}
}

// 合法接口名必须放行（不能误伤 bond/vlan/别名）。
func TestValidateIfaceAcceptsRealNames(t *testing.T) {
	for _, name := range []string{"eth0", "br-lan", "bond0", "eth0.100", "wlan0", "pppoe-wan", "ens18", "eth0:1"} {
		if err := validateIface(name); err != nil {
			t.Errorf("合法接口名 %q 被拒绝: %v", name, err)
		}
	}
}

// 空接口名仍走原有的 "interface required" 分支，报错文案不能变——
// 前端按这段文案提示用户。
func TestApplyEmptyIfaceKeepsOriginalError(t *testing.T) {
	err := Apply([]RateLimit{{Interface: "", RateMbps: 10, BurstKB: 32}})
	if err == nil || !strings.Contains(err.Error(), "interface required") {
		t.Fatalf("空接口名应报 interface required，实际: %v", err)
	}
}
