package nettools2

import (
	"strings"
	"testing"
)

// TestValidateTargetRejectsInjection —— 命令注入防线，全部拒绝。
func TestValidateTargetRejectsInjection(t *testing.T) {
	bad := []string{
		"1.1.1.1; rm -rf /",         // 分号
		"1.1.1.1 | cat /etc/passwd", // 管道
		"1.1.1.1$(id)",              // 命令替换
		"1.1.1.1\nreboot",           // 换行
		"-c 100",                    // 以 - 开头，参数注入
		"1.1.1.1 & whoami",          // 后台 &
		"1.1.1.1`id`",               // 反引号
	}
	for _, s := range bad {
		if err := ValidateTarget(s); err == nil {
			t.Fatalf("命令注入目标必须被拒绝: %q", s)
		}
	}
}

// TestValidateTargetAcceptsValid —— 接受 IPv4、IPv6、域名。
func TestValidateTargetAcceptsValid(t *testing.T) {
	ok := []string{
		"1.1.1.1",
		"2001:db8::1",
		"router.local",
		"example.com",
		"my-router.example.com",
		"205.251.242.103",
	}
	for _, s := range ok {
		if err := ValidateTarget(s); err != nil {
			t.Fatalf("合法目标 %q 不应被拒绝: %v", s, err)
		}
	}
}

// TestValidatePort —— 拒绝 65536、-1；0 允许（表示任意）；具体端口通过。
func TestValidatePort(t *testing.T) {
	// 0 是合法哨兵（任意端口），ValidatePort 放行
	if err := ValidatePort(0); err != nil {
		t.Fatalf("ValidatePort(0) 应放行: %v", err)
	}
	// 1 和 65535 边界通过
	if err := ValidatePort(1); err != nil {
		t.Fatalf("ValidatePort(1) 应通过: %v", err)
	}
	if err := ValidatePort(65535); err != nil {
		t.Fatalf("ValidatePort(65535) 应通过: %v", err)
	}
	// 65536、-1、-123 拒绝
	for _, p := range []int{65536, -1, -123} {
		if err := ValidatePort(p); err == nil {
			t.Fatalf("非法端口 %d 应被拒绝", p)
		}
	}
}

// TestPingArgsReturnsSlice —— PingArgs 返回 []string 不是拼接的 shell 字符串。
func TestPingArgsReturnsSlice(t *testing.T) {
	args, err := PingArgs("1.1.1.1", 5)
	if err != nil {
		t.Fatalf("PingArgs error: %v", err)
	}
	// 断言是切片且结构正确：["-c","5","1.1.1.1"]
	if len(args) != 3 {
		t.Fatalf("len=%d (want 3): %v", len(args), args)
	}
	if args[0] != "-c" {
		t.Fatalf("args[0]=%q (want -c)", args[0])
	}
	if args[1] != "5" {
		t.Fatalf("args[1]=%q (want 5)", args[1])
	}
	if args[2] != "1.1.1.1" {
		t.Fatalf("args[2]=%q (want 1.1.1.1)", args[2])
	}
	// 绝不能是含空格的单一字符串
	single := strings.Join(args, " ")
	if single == "" || strings.Contains(single, ";") || strings.Contains(single, "|") {
		t.Fatalf("PingArgs 返回了可疑的拼接串: %q", single)
	}
	// count 越界拒绝
	if _, err := PingArgs("1.1.1.1", 0); err == nil {
		t.Fatal("count=0 应被拒绝")
	}
	if _, err := PingArgs("1.1.1.1", 21); err == nil {
		t.Fatal("count=21 应被拒绝")
	}
}

// TestSummarizePingReachable —— "0% packet loss" -> 可达。
func TestSummarizePingReachable(t *testing.T) {
	lines := []string{
		"PING 1.1.1.1 (1.1.1.1) 56(84) bytes of data.",
		"64 bytes from 1.1.1.1: icmp_seq=1 ttl=56 time=5.01 ms",
		"64 bytes from 1.1.1.1: icmp_seq=2 ttl=56 time=4.98 ms",
		"",
		"--- 1.1.1.1 ping statistics ---",
		"2 packets transmitted, 2 received, 0% packet loss, time 1000ms",
	}
	sum := Summarize("ping", lines)
	if !strings.Contains(sum, "可达") || !strings.Contains(sum, "0%") {
		t.Fatalf("应含'可达'与'0 percent', got: %q", sum)
	}
}

// TestSummarizePingUnreachable —— "100% packet loss" -> 不可达。
func TestSummarizePingUnreachable(t *testing.T) {
	lines := []string{
		"PING 10.255.255.1 (10.255.255.1) 56(84) bytes of data.",
		"",
		"--- 10.255.255.1 ping statistics ---",
		"3 packets transmitted, 0 received, 100% packet loss, time 2000ms",
	}
	sum := Summarize("ping", lines)
	if !strings.Contains(sum, "不可达") {
		t.Fatalf("应含'不可达', got: %q", sum)
	}
}

// TestSummarizeUnknownHonest —— 无法识别时如实说，不编造。
func TestSummarizeUnknownHonest(t *testing.T) {
	// 空输出 / 只有二进制噪音
	sum := Summarize("ping", []string{})
	if !strings.Contains(sum, "无法识别") {
		t.Fatalf("空输出应如实说无法识别, got: %q", sum)
	}
	sum2 := Summarize("ping", []string{"garbage, not ping output at all"})
	if !strings.Contains(sum2, "无法识别") {
		t.Fatalf("无法识别的输出应说无法识别, got: %q", sum2)
	}
	// 未知工具也一样
	sum3 := Summarize("some-unknown-tool", []string{"anything"})
	if !strings.Contains(sum3, "无法识别") {
		t.Fatalf("未知工具应说无法识别, got: %q", sum3)
	}
}

// TestTracerouteArgs —— maxHops 1-30，返回 ["-m","15",target]。
func TestTracerouteArgs(t *testing.T) {
	args, err := TracerouteArgs("example.com", 15)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(args) != 3 || args[0] != "-m" || args[1] != "15" || args[2] != "example.com" {
		t.Fatalf("traceroute args=%v", args)
	}
	if _, err := TracerouteArgs("example.com", 0); err == nil {
		t.Fatal("maxHops=0 应拒绝")
	}
	if _, err := TracerouteArgs("example.com", 31); err == nil {
		t.Fatal("maxHops=31 应拒绝")
	}
}

// TestPortProbeArgs —— timeout 1-10，返回 nc 风格参数；port=0 拒绝。
func TestPortProbeArgs(t *testing.T) {
	args, err := PortProbeArgs("example.com", 443, 3)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	// nc -z -w 3 example.com 443
	if len(args) != 5 || args[0] != "-z" || args[1] != "-w" || args[2] != "3" ||
		args[3] != "example.com" || args[4] != "443" {
		t.Fatalf("port probe args=%v", args)
	}
	// port=0 拒绝（探测需要具体端口）
	if _, err := PortProbeArgs("example.com", 0, 3); err == nil {
		t.Fatal("port=0 端口探测应拒绝")
	}
	// timeout 越界拒绝
	if _, err := PortProbeArgs("example.com", 80, 0); err == nil {
		t.Fatal("timeout=0 应拒绝")
	}
	if _, err := PortProbeArgs("example.com", 80, 11); err == nil {
		t.Fatal("timeout=11 应拒绝")
	}
}

// TestSummarizeTraceroute —— 统计非空行 -> "经过 N 跳"。
func TestSummarizeTraceroute(t *testing.T) {
	lines := []string{
		"traceroute to 8.8.8.8 (8.8.8.8), 30 hops max, 60 byte packets",
		" 1  192.168.1.1  1.0 ms  1.1 ms  1.2 ms",
		" 2  10.0.0.1  5.0 ms  5.1 ms  5.2 ms",
		" 3  8.8.4.4  12 ms  13 ms  14 ms",
	}
	sum := Summarize("traceroute", lines)
	if !strings.Contains(sum, "经过") || !strings.Contains(sum, "3") || !strings.Contains(sum, "跳") {
		t.Fatalf("应含'经过 3 跳', got: %q", sum)
	}
}
