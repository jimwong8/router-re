package firewall

import (
	"fmt"
	"os/exec"
	"strings"
)

// nftBin returns the nftables binary path; falls back to "nft" in PATH.
func nftBin() string {
	for _, p := range []string{"/usr/sbin/nft", "/usr/bin/nft", "/sbin/nft"} {
		if _, err := exec.LookPath(p); err == nil {
			return p
		}
	}
	return "nft"
}

// runNft executes nft with the given args, feeding ruleset to stdin.
// It first tries the binary directly; on permission errors it retries with
// `sudo -n` (non-interactive). A final permission failure is returned as an
// error containing the marker "permission" so callers/tests can distinguish
// it from real syntax errors.
func runNft(args []string, ruleset string) error {
	bin := nftBin()
	run := func(argv []string) (string, error) {
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Stdin = strings.NewReader(ruleset)
		out, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}

	out, err := run(append([]string{bin}, args...))
	if err == nil {
		return nil
	}
	lower := strings.ToLower(out + " " + err.Error())
	if strings.Contains(lower, "operation not permitted") ||
		strings.Contains(lower, "permission denied") {
		// Retry with sudo -n
		if sudoBin, e := exec.LookPath("sudo"); e == nil {
			out2, err2 := run(append([]string{sudoBin, "-n", bin}, args...))
			if err2 == nil {
				return nil
			}
			lower2 := strings.ToLower(out2 + " " + err2.Error())
			if strings.Contains(lower2, "operation not permitted") ||
				strings.Contains(lower2, "permission denied") ||
				strings.Contains(lower2, "password") {
				return fmt.Errorf("nft permission denied (need root): %s", out2)
			}
			return fmt.Errorf("nft failed: %s", out2)
		}
		return fmt.Errorf("nft permission denied (need root): %s", out)
	}
	if out == "" {
		out = err.Error()
	}
	return fmt.Errorf("nft failed: %s", out)
}

// Validate runs `nft -c -f -` on the rendered ruleset to check syntax
// without actually applying it. It returns the command's error if any.
func Validate(rules []Rule) error {
	return runNft([]string{"-c", "-f", "-"}, RenderNftables(rules))
}

// Apply validates the rendered ruleset and then applies it with `nft -f -`.
// Validate is always run first; if it fails, nothing is applied.
// The previous table is deleted first so repeated Apply calls replace the
// ruleset instead of appending duplicate rules (nft -f appends to an existing
// chain).
func Apply(rules []Rule) error {
	if err := Validate(rules); err != nil {
		return err
	}
	// Delete the old table so the apply is idempotent. A missing table makes
	// nft exit non-zero; that is expected on first apply, so ignore the error.
	_ = runNft([]string{"delete", "table", "inet", "router"}, "")
	return runNft([]string{"-f", "-"}, RenderNftables(rules))
}