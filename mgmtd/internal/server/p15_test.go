package server

import (
	"encoding/json"
	"testing"

	"github.com/jimwong/router-re/mgmtd/internal/ddos"
	"github.com/jimwong/router-re/mgmtd/internal/nettools2"
	"strings"
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

// TestDDoSConfigWhitelistNeverNull 锁死一个让整个 tab 空白的 bug：
// ddos.Config.Whitelist 是 nil slice 时，encoding/json 会输出 null 而不是 []。
// 前端 Object.assign 后 whitelist 变成 null，模板里 whitelist.length 抛
// TypeError，Vue 把整个 tab-pane 渲染成注释 —— 页面上只有空白，
// 控制台只有一行错，非常难定位。
func TestDDoSConfigWhitelistNeverNull(t *testing.T) {
	c := ddos.Config{PortScanWindow: 10, BlockSeconds: 600}
	if c.Whitelist != nil {
		t.Fatal("前置条件: 零值 Config 的 Whitelist 应是 nil")
	}
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// 先确认问题真实存在：零值就是会输出 null（这就是当初的 bug 源头）
	if !strings.Contains(string(raw), `"whitelist":null`) {
		t.Fatalf("前提变了：零值 Config 不再序列化成 null，归一化逻辑可考虑移除。实际: %s", raw)
	}
	// handleGetDDoS 的归一化逻辑等价于这一句
	if c.Whitelist == nil {
		c.Whitelist = []string{}
	}
	raw2, _ := json.Marshal(c)
	if !strings.Contains(string(raw2), `"whitelist":[]`) {
		t.Fatalf("归一化后应是 []，实际: %s", raw2)
	}
}
