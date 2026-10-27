package firmware

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// testRunner records calls and lets tests simulate failures.
type testRunner struct {
	calls [][]string
	fail  bool
}

func (t *testRunner) Run(name string, args ...string) (string, error) {
	call := append([]string{name}, args...)
	t.calls = append(t.calls, call)
	if t.fail {
		return "", fmt.Errorf("%s failed", name)
	}
	return "", nil
}
func (t *testRunner) RunWithInput(input, name string, args ...string) (string, error) {
	return t.Run(name, args...)
}

func TestCompareVersion(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"24.10.10", "24.10.8", 1},
		{"24.10.8", "24.10.10", -1},
		{"24.10.8", "24.10.8", 0},
		{"23.05.4", "24.10.8", -1},
		{"1.2", "1.2.0", 0},
	}
	for _, c := range cases {
		got, err := CompareVersion(c.a, c.b)
		if err != nil {
			t.Fatalf("CompareVersion(%s,%s): %v", c.a, c.b, err)
		}
		if got != c.want {
			t.Errorf("CompareVersion(%s,%s) = %d want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestCompareVersionParseError(t *testing.T) {
	if _, err := CompareVersion("abc", "24.10.8"); err == nil {
		t.Fatal("expected parse error for abc")
	}
}

func TestInspectImageNotFound(t *testing.T) {
	_, err := InspectImage("/tmp/does-not-exist-firmware.bin")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "/tmp/does-not-exist-firmware.bin") {
		t.Fatalf("error should contain path, got %v", err)
	}
}

func TestInspectImageTarMagic(t *testing.T) {
	f, err := os.CreateTemp("", "fw-*.img")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	// 写 512 字节 ustar 头 + 数据
	buf := make([]byte, 512)
	copy(buf[257:263], []byte("ustar"))
	if _, err := f.Write(buf); err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("some firmware data"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	img, err := InspectImage(f.Name())
	if err != nil {
		t.Fatalf("InspectImage: %v", err)
	}
	if img.Magic != "tar" {
		t.Fatalf("expected magic tar, got %q", img.Magic)
	}
}

func TestValidateImageRejectsBoardMismatch(t *testing.T) {
	img := Image{Path: "/tmp/x", Size: 2 << 20, Magic: "tar", Board: "mt7621"}
	if err := ValidateImage(img, "mt7622"); err == nil {
		t.Fatal("expected board mismatch error")
	}
}

func TestValidateImageRejectsTooSmall(t *testing.T) {
	img := Image{Path: "/tmp/x", Size: 100, Magic: "tar", Board: "mt7621"}
	if err := ValidateImage(img, "mt7621"); err == nil {
		t.Fatal("expected size error")
	}
}

func TestBuildPlanNoBackupRisk(t *testing.T) {
	img := Image{Path: "/tmp/x", Size: 2 << 20, Magic: "tar", Board: "mt7621"}
	plan, err := BuildPlan(img, "mt7621", false)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	joined := strings.Join(plan.Risks, " ")
	if !strings.Contains(joined, "备份") {
		t.Fatalf("risks must mention backup when hasBackup=false, got %q", joined)
	}
}

func TestBuildPlanHasBackupNoBackupRisk(t *testing.T) {
	img := Image{Path: "/tmp/x", Size: 2 << 20, Magic: "tar", Board: "mt7621"}
	plan, err := BuildPlan(img, "mt7621", true)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	joined := strings.Join(plan.Risks, " ")
	// 有备份时仍应有升级风险提示
	if !strings.Contains(joined, "网络会中断") && !strings.Contains(joined, "ssh") {
		t.Fatalf("risks should contain ssh/network warning, got %q", joined)
	}
}

func TestApplyNoCommandOnValidationFailure(t *testing.T) {
	tr := &testRunner{}
	img := Image{Path: "/tmp/x", Size: 0, Magic: "tar", Board: "mt7621"} // 大小过小 → ValidateImage 失败
	ok, detail := Apply(tr, img, true)
	if ok {
		t.Fatal("Apply should fail on validation failure")
	}
	if detail == "" {
		t.Fatal("detail should contain error")
	}
	if len(tr.calls) != 0 {
		t.Fatalf("no command should run on validation failure, got %d calls", len(tr.calls))
	}
}

func TestApplyRunsSysupgradeOnValidImage(t *testing.T) {
	tr := &testRunner{}
	img := Image{Path: "/tmp/fw.bin", Size: 2 << 20, Magic: "tar", Board: "mt7621"}
	ok, detail := Apply(tr, img, true)
	if !ok {
		t.Fatalf("Apply should succeed on valid image, detail=%s", detail)
	}
	if len(tr.calls) != 1 {
		t.Fatalf("expected 1 sysupgrade call, got %d", len(tr.calls))
	}
	if tr.calls[0][0] != "sysupgrade" {
		t.Fatalf("expected sysupgrade, got %v", tr.calls[0])
	}
	// keepConfig=true → 不含 -n
	for _, a := range tr.calls[0] {
		if a == "-n" {
			t.Fatal("keepConfig=true must not use -n")
		}
	}
}

func TestApplyNoKeepConfig(t *testing.T) {
	tr := &testRunner{}
	img := Image{Path: "/tmp/fw.bin", Size: 2 << 20, Magic: "tar", Board: "mt7621"}
	ok, _ := Apply(tr, img, false)
	if !ok {
		t.Fatal("Apply should succeed")
	}
	found := false
	for _, a := range tr.calls[0] {
		if a == "-n" {
			found = true
		}
	}
	if !found {
		t.Fatal("keepConfig=false must use -n")
	}
}

func TestBackupConfig(t *testing.T) {
	// stub runner 模拟 sysupgrade -b 写出非空文件
	tr := &testBackupRunner{path: t.TempDir() + "/backup.tar.gz"}
	path, size, err := BackupConfig(tr, tr.path)
	if err != nil {
		t.Fatalf("BackupConfig: %v", err)
	}
	if path != tr.path || size <= 0 {
		t.Fatalf("bad result: path=%s size=%d", path, size)
	}
}

// testBackupRunner 模拟 sysupgrade -b 成功写出备份文件。
type testBackupRunner struct {
	path string
}

func (t *testBackupRunner) Run(name string, args ...string) (string, error) {
	if name == "sysupgrade" && len(args) >= 2 && args[0] == "-b" {
		// 写出一个非空文件
		os.WriteFile(args[1], []byte("backup data"), 0o644)
		return "", nil
	}
	return "", fmt.Errorf("unexpected command %s %v", name, args)
}
func (t *testBackupRunner) RunWithInput(input, name string, args ...string) (string, error) {
	return t.Run(name, args...)
}

func TestBackupConfigEmptyFileFails(t *testing.T) {
	tr := &testBackupRunnerEmpty{}
	_, _, err := BackupConfig(tr, t.TempDir()+"/empty.tar.gz")
	if err == nil {
		t.Fatal("expected error for empty backup file")
	}
}

type testBackupRunnerEmpty struct{}

func (testBackupRunnerEmpty) Run(name string, args ...string) (string, error) {
	if name == "sysupgrade" && len(args) >= 2 && args[0] == "-b" {
		os.WriteFile(args[1], []byte{}, 0o644)
		return "", nil
	}
	return "", fmt.Errorf("unexpected %s", name)
}
func (testBackupRunnerEmpty) RunWithInput(input, name string, args ...string) (string, error) {
	return testBackupRunnerEmpty{}.Run(name, args...)
}

func TestInspectImageSha256(t *testing.T) {
	f, err := os.CreateTemp("", "fw-*.img")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString("hello")
	f.Close()
	img, err := InspectImage(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	if img.Size != 5 {
		t.Fatalf("size=%d want 5", img.Size)
	}
	if img.SHA256 == "" {
		t.Fatal("sha256 should be computed")
	}
}
