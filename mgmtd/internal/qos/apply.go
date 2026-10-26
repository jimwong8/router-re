package qos

import (
	"fmt"
	"os/exec"
	"strings"
)

// tcBin returns the tc binary path, falling back to "tc" in PATH.
func tcBin() string {
	for _, p := range []string{"/usr/sbin/tc", "/sbin/tc", "/usr/bin/tc"} {
		if _, err := exec.LookPath(p); err == nil {
			return p
		}
	}
	return "tc"
}

// runCmd executes a shell word list, retrying with `sudo -n` when the first
// attempt fails on a permission error.
func runCmd(argv []string) error {
	if len(argv) == 0 {
		return nil
	}
	out, err := exec.Command(argv[0], argv[1:]...).CombinedOutput()
	if err == nil {
		return nil
	}
	lower := strings.ToLower(string(out) + " " + err.Error())
	if !strings.Contains(lower, "operation not permitted") &&
		!strings.Contains(lower, "permission denied") {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("command failed: %s", msg)
	}
	// Retry through sudo -n (non-interactive) when available.
	if sudoBin, e := exec.LookPath("sudo"); e == nil {
		out2, err2 := exec.Command(sudoBin, append([]string{"-n", argv[0]}, argv[1:]...)...).CombinedOutput()
		if err2 == nil {
			return nil
		}
		lower2 := strings.ToLower(string(out2) + " " + err2.Error())
		if strings.Contains(lower2, "permission") || strings.Contains(lower2, "password") {
			return fmt.Errorf("permission denied (need root): %s", strings.TrimSpace(string(out2)))
		}
		return fmt.Errorf("command failed: %s", strings.TrimSpace(string(out2)))
	}
	return fmt.Errorf("permission denied (need root): %s", strings.TrimSpace(string(out)))
}

// Apply executes the rendered tc commands for the given rate limits.
// Each command string is split into argv (the renderer only produces simple
// `tc qdisc ...` / `tc filter ...` forms with no shell metacharacters).
// Requires root/CAP_NET_ADMIN; when unavailable it retries with `sudo -n` and
// returns a permission error if that also fails.
func Apply(limits []RateLimit) error {
	for _, l := range limits {
		if l.Interface == "" {
			return fmt.Errorf("interface required")
		}
		if l.RateMbps <= 0 {
			return fmt.Errorf("invalid rate: %d", l.RateMbps)
		}
	}
	for _, cmd := range RenderTcCommands(limits) {
		fields := strings.Fields(cmd)
		if len(fields) == 0 {
			continue
		}
		// The renderer emits commands starting with "tc"; resolve it to an
		// absolute path because /usr/sbin is not always in the process PATH.
		if fields[0] == "tc" {
			fields[0] = tcBin()
		}
		if err := runCmd(fields); err != nil {
			return err
		}
	}
	return nil
}
