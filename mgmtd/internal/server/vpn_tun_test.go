package server

import (
	"os"
	"testing"
)

// TestTunUsableMatchesSyscall 保证 tunUsable 不是写死的常量：
// 它的结果必须与"直接 open /dev/net/tun 能不能成功"一致。
//
// 132 那种容器上 open 返回 EPERM，函数必须回 false，界面才会提前告警；
// 在能建 tun 的机器上必须回 true，否则会把可用环境误报成不可用。
func TestTunUsableMatchesSyscall(t *testing.T) {
	f, err := os.OpenFile("/dev/net/tun", os.O_RDWR, 0)
	expect := err == nil
	if f != nil {
		_ = f.Close()
	}
	if got := tunUsable(); got != expect {
		t.Fatalf("tunUsable() = %v, 直接 open 的结果是 %v", got, expect)
	}
	t.Logf("本机 tun 可用 = %v", expect)
}
