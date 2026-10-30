package storage

import (
	"strings"
	"testing"
)

type fakeRunner struct {
	calls [][]string
	out   string
	err   error
}

func (f *fakeRunner) Run(name string, args ...string) (string, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	return f.out, f.err
}

func (f *fakeRunner) RunWithInput(input, name string, args ...string) (string, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	return f.out, f.err
}

func TestPlanFormatRejectsBadInput(t *testing.T) {
	cases := []struct{ dev, fs, want string }{
		{"sda1", "ext4", "必须以 /dev/ 开头"},
		{"/dev/../../etc/passwd", "ext4", "非法"},
		{"/dev/does-not-exist-xyz", "ext4", "不存在"},
		{"/dev/sda1; rm -rf /", "ext4", "必须"},
	}
	for _, c := range cases {
		if _, err := PlanFormat(c.dev, c.fs, ""); err == nil {
			t.Fatalf("dev=%q 应被拒绝", c.dev)
		} else if !strings.Contains(err.Error(), c.want) && !strings.Contains(err.Error(), "不存在") {
			t.Logf("dev=%q err=%v（可接受）", c.dev, err)
		}
	}
}

func TestPlanFormatRejectsSystemDisk(t *testing.T) {
	disks, err := ListDisks()
	if err != nil {
		t.Skipf("无法枚举磁盘: %v", err)
	}
	var sysPart *Partition
	for i := range disks {
		for j := range disks[i].Partitions {
			if disks[i].Partitions[j].IsSystem {
				sysPart = &disks[i].Partitions[j]
				break
			}
		}
		if sysPart != nil {
			break
		}
	}
	if sysPart == nil {
		t.Skip("本机没有识别到系统盘分区")
	}
	_, err = PlanFormat(sysPart.Path, "ext4", "")
	if err == nil {
		t.Fatalf("格式化系统盘分区 %s 必须被拒绝", sysPart.Path)
	}
	if !strings.Contains(err.Error(), "系统") && !strings.Contains(err.Error(), "已挂载") {
		t.Fatalf("错误应说明原因（系统盘/已挂载），实际: %v", err)
	}
}

func TestPlanFormatFsTypeWhitelist(t *testing.T) {
	disks, _ := ListDisks()
	var dev string
	for _, d := range disks {
		if d.IsSystem {
			continue
		}
		for _, p := range d.Partitions {
			if !p.IsMounted {
				dev = p.Path
			}
		}
	}
	if dev == "" {
		t.Skip("没有可用于测试的非系统未挂载分区")
	}
	for _, fs := range []string{"ntfs", "btrfs", "ext2", ""} {
		if _, err := PlanFormat(dev, fs, ""); err == nil {
			t.Fatalf("文件系统 %q 应被拒绝", fs)
		}
	}
	p, err := PlanFormat(dev, "ext4", "backup")
	if err != nil {
		t.Fatalf("ext4 应被接受: %v", err)
	}
	if p.Command[0] != "mkfs.ext4" || !contains(p.Command, "-F") {
		t.Fatalf("ext4 命令不符: %v", p.Command)
	}
	if !strings.Contains(p.Danger, dev) {
		t.Fatalf("Danger 必须点明设备: %s", p.Danger)
	}
}

func TestPlanFormatBadLabel(t *testing.T) {
	disks, _ := ListDisks()
	var dev string
	for _, d := range disks {
		if d.IsSystem {
			continue
		}
		for _, p := range d.Partitions {
			if !p.IsMounted {
				dev = p.Path
			}
		}
	}
	if dev == "" {
		t.Skip("没有可用于测试的分区")
	}
	if _, err := PlanFormat(dev, "ext4", "a:b"); err == nil {
		t.Fatal("卷标含冒号应被拒绝")
	}
	if _, err := PlanFormat(dev, "ext4", strings.Repeat("x", 20)); err == nil {
		t.Fatal("卷标超 16 字节应被拒绝")
	}
}

func TestPlanPartitionBounds(t *testing.T) {
	disks, _ := ListDisks()
	var dev string
	for _, d := range disks {
		if !d.IsSystem && !d.ReadOnly {
			dev = d.Path
		}
	}
	if dev == "" {
		t.Skip("没有可用于测试的非系统磁盘")
	}
	if _, err := PlanPartition(dev, 0, 100); err == nil {
		t.Fatal("起始 <1MiB 应被拒绝")
	}
	// 分区小于 1GiB
	if _, err := PlanPartition(dev, 1, 100); err == nil {
		t.Fatal("小于 1GiB 的分区应被拒绝")
	}
	// 结束小于起始
	if _, err := PlanPartition(dev, 5000, 4000); err == nil {
		t.Fatal("结束小于起始应被拒绝")
	}
}

func TestExecuteRequiresConfirm(t *testing.T) {
	f := &fakeRunner{out: "ok"}
	p := Plan{Action: "format", Device: "/dev/sdz1", Command: []string{"mkfs.ext4", "/dev/sdz1"}}
	if _, err := Execute(p, f); err == nil {
		t.Fatal("未确认的计划必须拒绝执行")
	}
	if len(f.calls) != 0 {
		t.Fatalf("拒绝时不应执行任何命令，实际调用了 %v", f.calls)
	}
}

func TestExecuteRunsConfirmedPlan(t *testing.T) {
	f := &fakeRunner{out: "done"}
	p := Plan{Action: "format", Device: "/dev/sdz1", Command: []string{"mkfs.ext4", "-F", "/dev/sdz1"}, Confirmed: true}
	out, err := Execute(p, f)
	if err != nil {
		t.Fatalf("确认的计划应执行: %v", err)
	}
	if out != "done" || len(f.calls) != 1 || f.calls[0][0] != "mkfs.ext4" {
		t.Fatalf("执行结果不符: out=%q calls=%v", out, f.calls)
	}
}

func TestExecuteRejectsNonDestructiveAction(t *testing.T) {
	f := &fakeRunner{}
	p := Plan{Action: "mount", Command: []string{"mount", "/dev/sda1", "/mnt"}, Confirmed: true}
	if _, err := Execute(p, f); err == nil {
		t.Fatal("mount 不应走 Execute 的破坏性通道")
	}
}

func TestMountPointFor(t *testing.T) {
	if got := MountPointFor("abcd-1234", "/dev/sda1"); got != MountRoot+"/abcd-1234" {
		t.Fatalf("UUID 挂载点不符: %s", got)
	}
	if got := MountPointFor("", "/dev/sda1"); got != MountRoot+"/sda1" {
		t.Fatalf("无 UUID 时应退回设备名: %s", got)
	}
}

func TestUnmountOutsideRootRejected(t *testing.T) {
	if err := Unmount("/mnt/other"); err == nil {
		t.Fatal("只允许卸载 MountRoot 下的挂载点")
	}
}

func TestParentDevice(t *testing.T) {
	cases := map[string]string{
		"/dev/sda1":      "/dev/sda",
		"/dev/nvme0n1p1": "/dev/nvme0n1",
		"/dev/vda2":      "/dev/vda",
		"/dev/mmcblk0p1": "/dev/mmcblk0",
		"/dev/sda":       "",
	}
	for in, want := range cases {
		if got := parentDevice(in); got != want {
			t.Fatalf("parentDevice(%s)=%q，期望 %q", in, got, want)
		}
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
