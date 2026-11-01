package bond

import (
	"fmt"
	"strings"
	"testing"
)

// recordingTuneRunner records commands and simulates /sys reads.
type recordingTuneRunner struct {
	calls    []string
	sysFiles map[string]string
	failPath string // path that should fail on write
}

func newTuneRunner() *recordingTuneRunner {
	return &recordingTuneRunner{sysFiles: map[string]string{}}
}

func (r *recordingTuneRunner) Run(name string, args ...string) (string, error) {
	r.calls = append(r.calls, name+" "+strings.Join(args, " "))
	if name == "cat" && len(args) == 1 {
		if v, ok := r.sysFiles[args[0]]; ok {
			return v, nil
		}
		return "", fmt.Errorf("cat: no sim for %s", args[0])
	}
	// ip link set ... -> success
	// sh -c 'echo X > path' -> record write, simulate readback
	if name == "sh" && len(args) >= 2 && args[0] == "-c" {
		cmd := args[1]
		// parse "echo MASK > PATH"
		if strings.Contains(cmd, " > ") {
			parts := strings.SplitN(cmd, " > ", 2)
			path := strings.TrimSpace(parts[1])
			mask := strings.TrimPrefix(strings.TrimSpace(parts[0]), "echo ")
			mask = strings.TrimSpace(mask)
			if r.failPath == path {
				return "", fmt.Errorf("write failed on %s", path)
			}
			r.sysFiles[path] = mask
		}
		return "", nil
	}
	return "", nil
}
func (r *recordingTuneRunner) RunWithInput(input, name string, args ...string) (string, error) {
	return r.Run(name, args...)
}

func (r *recordingTuneRunner) hasCall(sub string) bool {
	for _, c := range r.calls {
		if strings.Contains(c, sub) {
			return true
		}
	}
	return false
}

// withSysfs 把 sysfs 读写接缝换成内存实现，并返回还原函数。
// applyTune 已经不再经过 shell，所以写入必须在这里拦截而不是在 Runner 里。
func (r *recordingTuneRunner) withSysfs(t *testing.T) {
	t.Helper()
	oldW, oldR := sysfsWrite, sysfsRead
	sysfsWrite = func(path, value string) error {
		r.calls = append(r.calls, "write "+path+" = "+strings.TrimSpace(value))
		if r.failPath == path {
			return fmt.Errorf("write failed on %s", path)
		}
		r.sysFiles[path] = strings.TrimSpace(value)
		return nil
	}
	sysfsRead = func(path string) (string, error) {
		r.calls = append(r.calls, "read "+path)
		if v, ok := r.sysFiles[path]; ok {
			return v, nil
		}
		return "", fmt.Errorf("no sim for %s", path)
	}
	t.Cleanup(func() { sysfsWrite, sysfsRead = oldW, oldR })
}

func TestValidateTuneRejectsLoopback(t *testing.T) {
	c := TuneConfig{Iface: "lo", MTU: 1500}
	if err := ValidateTune(c); err == nil {
		t.Fatal("expected error for lo")
	}
}

func TestValidateTuneRejectsBadMTU(t *testing.T) {
	c := TuneConfig{Iface: "eth0", MTU: 100}
	if err := ValidateTune(c); err == nil {
		t.Fatal("expected error for MTU=100")
	}
}

func TestValidateTuneValid(t *testing.T) {
	c := TuneConfig{Iface: "eth0", MTU: 1500, TxQueueLen: 1000, GroMaxSize: 65536, RPS: []int{3}}
	if err := ValidateTune(c); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

func TestApplyTuneRPSReadback(t *testing.T) {
	rr := newTuneRunner()
	rr.withSysfs(t)
	c := TuneConfig{Iface: "eth0", RPS: []int{3, 7}}
	ok, detail := ApplyTune(rr, c)
	if !ok {
		t.Fatalf("ApplyTune should succeed, detail=%s", detail)
	}
	path0 := "/sys/class/net/eth0/queues/rx-0/rps_cpus"
	path1 := "/sys/class/net/eth0/queues/rx-1/rps_cpus"
	if rr.sysFiles[path0] != "3" {
		t.Fatalf("rx-0 rps not written: got %q", rr.sysFiles[path0])
	}
	if rr.sysFiles[path1] != "7" {
		t.Fatalf("rx-1 rps not written: got %q", rr.sysFiles[path1])
	}
	// 回读动作
	if !rr.hasCall("read " + path0) {
		t.Fatalf("missing readback for rx-0: %v", rr.calls)
	}
}

// 接口名字符白名单：这些 payload 以前能拼进 sh -c 以 root 执行任意命令。
// 实测 iface="eth0; touch /tmp/pwn_rce_test #" 会真的创建文件。
func TestValidateTuneRejectsShellInjection(t *testing.T) {
	payloads := []string{
		"eth0; touch /tmp/pwn_rce_test #",
		"eth0 && id",
		"eth0 | id",
		"$(id)",
		"`id`",
		"eth0\nid",
		"../../etc/passwd",
		"-f",
		"eth0 x",
		"eth0'",
	}
	for _, p := range payloads {
		if err := ValidateTune(TuneConfig{Iface: p, GroMaxSize: 4096}); err == nil {
			t.Errorf("接口名 %q 应被拒绝，实际通过校验", p)
		}
	}
}

// 合法接口名不能误伤：bond/vlan/别名都要能过。
func TestValidateTuneAcceptsRealIfaceNames(t *testing.T) {
	for _, name := range []string{"eth0", "br-lan", "bond0", "eth0.100", "wlan0", "pppoe-wan", "ens18", "eth0:1"} {
		if err := ValidateTune(TuneConfig{Iface: name, MTU: 1500}); err != nil {
			t.Errorf("合法接口名 %q 被拒绝: %v", name, err)
		}
	}
}

func TestApplyTuneRPSReadbackMismatch(t *testing.T) {
	rr := newTuneRunner()
	// failPath 让写入失败（模拟内核拒绝）
	rr.failPath = "/sys/class/net/eth0/queues/rx-0/rps_cpus"
	c := TuneConfig{Iface: "eth0", RPS: []int{3}}
	ok, detail := ApplyTune(rr, c)
	if ok {
		t.Fatal("ApplyTune should fail on readback mismatch")
	}
	if detail == "" {
		t.Fatal("detail should contain error")
	}
}

func TestApplyTuneMTU(t *testing.T) {
	rr := newTuneRunner()
	c := TuneConfig{Iface: "eth0", MTU: 9000}
	ok, detail := ApplyTune(rr, c)
	if !ok {
		t.Fatalf("ApplyTune: %v", detail)
	}
	if !rr.hasCall("ip link set dev eth0 mtu 9000") {
		t.Fatalf("missing mtu command: %v", rr.calls)
	}
}

func TestApplyTuneValidationFailure(t *testing.T) {
	rr := newTuneRunner()
	c := TuneConfig{Iface: "lo", MTU: 1500}
	ok, detail := ApplyTune(rr, c)
	if ok {
		t.Fatal("ApplyTune should fail on validation")
	}
	if detail == "" {
		t.Fatal("detail should contain error")
	}
	if len(rr.calls) != 0 {
		t.Fatalf("no command should run on validation failure, got %v", rr.calls)
	}
}
