package qos

import (
	"strings"
	"testing"
)

type fakeRunner struct{ cmds [][]string }

func (f *fakeRunner) Run(name string, args ...string) (string, error) {
	f.cmds = append(f.cmds, append([]string{name}, args...))
	return "", nil
}
func (f *fakeRunner) RunWithInput(input, name string, args ...string) (string, error) {
	f.cmds = append(f.cmds, append([]string{name}, args...))
	return "", nil
}

func sampleLimits() []ClientLimit {
	return []ClientLimit{{
		Selector:     "ip:192.168.1.10",
		DownloadKbps: 1000,
		UploadKbps:   500,
	}}
}

// 真实漏洞回归：handler 把请求里的 device 直接传给 ApplyClientLimits，
// 而 RenderClientTcCommands 把它拼进 "tc qdisc del dev <dev> root"，
// ApplyClientLimits 再逐行 strings.Fields 后 exec argv[0]。
// device 里一个换行就能注入 "sh -c id>/tmp/pwn" —— root 下任意命令执行。
func TestRenderClientTcCommandsRejectsBadDevice(t *testing.T) {
	for _, dev := range []string{
		"eth0\nsh -c id>/tmp/pwn",
		"eth0 ; id",
		"eth 0",
		"eth0\t",
		"eth0$(id)",
		strings.Repeat("a", 40),
	} {
		if _, err := RenderClientTcCommands(sampleLimits(), dev); err == nil {
			t.Errorf("device %q 未被拒绝", dev)
		}
	}
}

func TestRenderClientTcCommandsAcceptsLegitDevices(t *testing.T) {
	for _, dev := range []string{"eth0", "ens18", "pppoe-wan", "br-lan", "eth0.100", "wg0"} {
		cmds, err := RenderClientTcCommands(sampleLimits(), dev)
		if err != nil {
			t.Errorf("合法 device %q 被误拒: %v", dev, err)
		}
		for _, c := range cmds {
			if strings.Contains(c, "\n") {
				t.Errorf("device %q 渲染出多行: %q", dev, c)
			}
		}
	}
}

// 执行侧 fail-closed：即使 device 绕过渲染层，也绝不能被当作程序执行。
func TestApplyClientLimitsRefusesInjectedLine(t *testing.T) {
	f := &fakeRunner{}
	err := ApplyClientLimits(f, sampleLimits(), "eth0\nsh -c id>/tmp/pwn")
	if err == nil {
		t.Fatal("注入 device 应当报错")
	}
	for _, cmd := range f.cmds {
		if cmd[0] != "tc" && cmd[0] != "nft" {
			t.Fatalf("执行了非 tc/nft 命令: %q", cmd)
		}
	}
}

func TestApplyClientLimitsRunsLegit(t *testing.T) {
	f := &fakeRunner{}
	if err := ApplyClientLimits(f, sampleLimits(), "eth0"); err != nil {
		t.Fatalf("合法配置执行失败: %v", err)
	}
	sawTC := false
	for _, cmd := range f.cmds {
		if cmd[0] != "tc" && cmd[0] != "nft" {
			t.Fatalf("出现非预期命令: %q", cmd)
		}
		if cmd[0] == "tc" {
			sawTC = true
		}
	}
	if !sawTC {
		t.Fatal("没有执行任何 tc 命令")
	}
}
