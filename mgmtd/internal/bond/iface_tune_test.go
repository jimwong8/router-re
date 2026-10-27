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
	// 回读动作：calls 应含 cat
	if !rr.hasCall("cat " + path0) {
		t.Fatalf("missing readback cat for rx-0: %v", rr.calls)
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
