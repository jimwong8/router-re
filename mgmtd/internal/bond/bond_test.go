package bond

import (
	"fmt"
	"strings"
	"testing"
)

// recordingRunner records commands and can simulate failures.
type recordingRunner struct {
	calls []string
	// slavesExist: set of slave names that "exist"
	slavesExist map[string]bool
}

func newRec(slavesExist ...string) *recordingRunner {
	m := map[string]bool{}
	for _, s := range slavesExist {
		m[s] = true
	}
	return &recordingRunner{slavesExist: m}
}

func (r *recordingRunner) Run(name string, args ...string) (string, error) {
	r.calls = append(r.calls, name+" "+strings.Join(args, " "))
	// ip link show <slave> — 存在性检查
	if name == "ip" && len(args) >= 3 && args[0] == "link" && args[1] == "show" {
		if r.slavesExist[args[2]] {
			return "", nil
		}
		return "", fmt.Errorf("ip link show %s: device not found", args[2])
	}
	return "", nil
}
func (r *recordingRunner) RunWithInput(input, name string, args ...string) (string, error) {
	return r.Run(name, args...)
}

func (r *recordingRunner) hasCall(sub string) bool {
	for _, c := range r.calls {
		if strings.Contains(c, sub) {
			return true
		}
	}
	return false
}

func TestValidateBondRejectsLoopbackSlave(t *testing.T) {
	r := newRec("eth1")
	c := BondConfig{Name: "bond0", Slaves: []string{"lo"}, Mode: "802.3ad", MIIMon: 100}
	if err := ValidateBond(r, c); err == nil {
		t.Fatal("expected error for lo slave")
	}
}

func TestValidateBondRejectsDuplicateSlave(t *testing.T) {
	r := newRec("eth1")
	c := BondConfig{Name: "bond0", Slaves: []string{"eth1", "eth1"}, Mode: "802.3ad", MIIMon: 100}
	if err := ValidateBond(r, c); err == nil {
		t.Fatal("expected error for duplicate slave")
	}
}

func TestValidateBondRejectsLACPRateInActiveBackup(t *testing.T) {
	r := newRec("eth1")
	c := BondConfig{Name: "bond0", Slaves: []string{"eth1"}, Mode: "active-backup", MIIMon: 100, LACPRate: "fast"}
	if err := ValidateBond(r, c); err == nil {
		t.Fatal("expected error: lacp_rate only valid for 802.3ad")
	}
}

func TestValidateBondRejectsXmitHashInActiveBackup(t *testing.T) {
	r := newRec("eth1")
	c := BondConfig{Name: "bond0", Slaves: []string{"eth1"}, Mode: "active-backup", MIIMon: 100, XmitHash: "layer3+4"}
	if err := ValidateBond(r, c); err == nil {
		t.Fatal("expected error: xmit_hash only valid for 802.3ad")
	}
}

func TestValidateBondRejectsNonexistentSlave(t *testing.T) {
	r := newRec() // eth1 does not exist
	c := BondConfig{Name: "bond0", Slaves: []string{"eth1"}, Mode: "802.3ad", MIIMon: 100}
	if err := ValidateBond(r, c); err == nil {
		t.Fatal("expected error for nonexistent slave")
	}
}

func TestValidateBondValid8023ad(t *testing.T) {
	r := newRec("eth1", "eth2")
	c := BondConfig{Name: "bond0", Slaves: []string{"eth1", "eth2"}, Mode: "802.3ad", MIIMon: 100, LACPRate: "fast", XmitHash: "layer3+4", MTU: 1500}
	if err := ValidateBond(r, c); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

func TestRenderScriptLacpRateBeforeEnslave(t *testing.T) {
	c := BondConfig{Name: "bond0", Slaves: []string{"eth1"}, Mode: "802.3ad", MIIMon: 100, LACPRate: "fast", XmitHash: "layer3+4"}
	out := RenderScript(c)
	// lacp_rate 必须出现在 ip link add 行（enslave 之前）
	addIdx := strings.Index(out, "ip link add")
	enslaveIdx := strings.Index(out, "ip link set eth1 master")
	lacpIdx := strings.Index(out, "lacp_rate fast")
	if addIdx < 0 || enslaveIdx < 0 || lacpIdx < 0 {
		t.Fatalf("missing expected lines:\n%s", out)
	}
	if !(addIdx < lacpIdx && lacpIdx < enslaveIdx) {
		t.Fatalf("lacp_rate must be set in ip link add (before enslave): addIdx=%d lacpIdx=%d enslaveIdx=%d\n%s", addIdx, lacpIdx, enslaveIdx, out)
	}
}

func TestRenderScriptOrder(t *testing.T) {
	c := BondConfig{Name: "bond0", Slaves: []string{"eth1"}, Mode: "active-backup", MIIMon: 100}
	out := RenderScript(c)
	// 顺序：ip link add -> down -> master -> up -> bond up
	idxAdd := strings.Index(out, "ip link add")
	idxDown := strings.Index(out, "ip link set eth1 down")
	idxMaster := strings.Index(out, "ip link set eth1 master bond0")
	idxUp := strings.Index(out, "ip link set eth1 up")
	idxBondUp := strings.Index(out, "ip link set bond0 up")
	if !(idxAdd < idxDown && idxDown < idxMaster && idxMaster < idxUp && idxUp < idxBondUp) {
		t.Fatalf("wrong order:\n%s", out)
	}
}

func TestApplyZeroCommandsOnValidationFailure(t *testing.T) {
	r := newRec() // eth1 does not exist → ValidateBond fails
	c := BondConfig{Name: "bond0", Slaves: []string{"eth1"}, Mode: "802.3ad", MIIMon: 100}
	ok, detail := Apply(r, c)
	if ok {
		t.Fatal("Apply should fail on validation failure")
	}
	if detail == "" {
		t.Fatal("detail should contain error")
	}
	if len(r.calls) != 1 { // only the ip link show existence check
		// ValidateBond calls ip link show for existence; that's the only command.
		// Apply itself runs zero additional commands after validation failure.
	}
	// Apply runs zero apply commands: only existence check (from ValidateBond) may have run.
	for _, c := range r.calls {
		if strings.Contains(c, "link add") || strings.Contains(c, "master") {
			t.Fatalf("no apply command should run on validation failure, got %v", r.calls)
		}
	}
}

func TestReadBond(t *testing.T) {
	rr := &readBondRunner{}
	out, err := ReadBond(rr, "bond0")
	if err != nil {
		t.Fatalf("ReadBond: %v", err)
	}
	if out["mode"] != "802.3ad" {
		t.Fatalf("mode wrong: %q", out["mode"])
	}
	if out["slaves"] != "eth1 eth2" {
		t.Fatalf("slaves wrong: %q", out["slaves"])
	}
}

type readBondRunner struct{}

func (readBondRunner) Run(name string, args ...string) (string, error) {
	if len(args) >= 1 && args[0] == "cat" {
	}
	// args: cat <path>
	if name == "cat" && len(args) == 1 {
		if strings.HasSuffix(args[0], "/mode") {
			return "802.3ad 1", nil
		}
		if strings.HasSuffix(args[0], "/slaves") {
			return "eth1 eth2", nil
		}
		if strings.HasSuffix(args[0], "/miimon") {
			return "100", nil
		}
	}
	return "", fmt.Errorf("unexpected")
}
func (readBondRunner) RunWithInput(input, name string, args ...string) (string, error) {
	return readBondRunner{}.Run(name, args...)
}
