package server

import (
	"testing"

	"github.com/jimwong/router-re/mgmtd/internal/nettools2"
)

// TestNetToolArgsDoNotContainBinary 锁死一个真实 bug：
// PingArgs/TracerouteArgs/PortProbeArgs 返回的是**参数**，不含命令名。
// 把它们直接当 argv 传给 runner（argv[0] 当命令）会让系统去执行 "-c"，
// 于是无论目标是什么都报"未安装"—— 功能完全不可用，而错误信息看起来
// 像是环境问题，极难定位。
func TestNetToolArgsDoNotContainBinary(t *testing.T) {
	ping, err := nettools2.PingArgs("1.1.1.1", 4)
	if err != nil {
		t.Fatalf("PingArgs: %v", err)
	}
	if len(ping) == 0 {
		t.Fatal("PingArgs 返回空")
	}
	if ping[0] == "ping" {
		t.Fatalf("约定变了：PingArgs 现在包含命令名，handler 需同步修改")
	}
	if isBinaryName(ping[0]) {
		t.Fatalf("PingArgs 的第一个元素 %q 看起来是命令名，不是参数", ping[0])
	}

	tp, err := nettools2.TracerouteArgs("1.1.1.1", 15)
	if err != nil {
		t.Fatalf("TracerouteArgs: %v", err)
	}
	if len(tp) == 0 || isBinaryName(tp[0]) {
		t.Fatalf("TracerouteArgs 第一个元素 %q 不应是命令名", tp[0])
	}

	pp, err := nettools2.PortProbeArgs("1.1.1.1", 80, 3)
	if err != nil {
		t.Fatalf("PortProbeArgs: %v", err)
	}
	if len(pp) == 0 || isBinaryName(pp[0]) {
		t.Fatalf("PortProbeArgs 第一个元素 %q 不应是命令名", pp[0])
	}
}

func isBinaryName(s string) bool {
	switch s {
	case "ping", "traceroute", "nc", "mtr", "ping6":
		return true
	}
	return false
}
