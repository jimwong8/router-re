package kvm

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeRunner struct {
	out map[string]string
	err map[string]error
	got []string
}

func (f *fakeRunner) Run(name string, args ...string) (string, error) {
	key := name + " " + strings.Join(args, " ")
	f.got = append(f.got, key)
	if e, ok := f.err[key]; ok {
		return f.out[key], e
	}
	if v, ok := f.out[key]; ok {
		return v, nil
	}
	// 前缀匹配：dominfo vm1 / virsh dominfo 之类
	for k, v := range f.out {
		if strings.HasPrefix(key, k) {
			return v, nil
		}
	}
	return "", fmt.Errorf("unexpected command: %s", key)
}

func TestDetectOnThisHost(t *testing.T) {
	c := Detect()
	// 只要不 panic 且字段自洽即可（CI 环境可能没有 KVM）
	if c.Available && !c.HasDevice {
		t.Fatal("可用状态必须建立在 /dev/kvm 存在之上")
	}
	if !c.Available && c.Reason == "" {
		t.Fatal("不可用时必须给出原因，界面上要能解释为什么点不了")
	}
	t.Logf("可用=%v 设备=%v 可写=%v qemu=%v virsh=%v libvirt=%v 原因=%q",
		c.Available, c.HasDevice, c.DeviceWritable, c.HasQemu, c.HasVirsh, c.LibvirtRunning, c.Reason)
}

func TestParseVMNames(t *testing.T) {
	out := "vm1\nvm-two\n\nvm_3\n"
	got := parseVMNames(out)
	if len(got) != 3 {
		t.Fatalf("应解析出 3 个名字，实际 %v", got)
	}
	if got[1] != "vm-two" {
		t.Fatalf("第二个名字错: %v", got)
	}
	// 错误输出不能被当成虚机名
	if got := parseVMNames("error: failed to connect\n"); len(got) != 0 {
		t.Fatalf("error 行应被过滤: %v", got)
	}
}

func TestParseDominfo(t *testing.T) {
	out := `Id:             3
Name:           web
UUID:           1234-abcd
OS Type:        hvm
State:          running
CPU(s):         4
CPU time:       120.5s
Max memory:     2097152 KiB
Used memory:    1048576 KiB
Autostart:      enable
`
	var vm VM
	parseDominfo(out, &vm)
	if vm.State != "running" || vm.VCPU != 4 || vm.MemoryMiB != 2048 || vm.UUID != "1234-abcd" {
		t.Fatalf("解析结果错误: %+v", vm)
	}
	if !vm.Autostart {
		t.Fatal("Autostart 应为 true")
	}
	if stateText(vm.State) != "运行中" {
		t.Fatalf("状态文案错误: %s", stateText(vm.State))
	}
}

func TestParseDominfoShutoff(t *testing.T) {
	var vm VM
	parseDominfo("State:          shut off\nCPU(s):         2\nMax memory:     524288 KiB\n", &vm)
	if vm.StateText != "" && stateText(vm.State) != "已关机" {
		t.Fatalf("关机状态文案错误: %s", stateText(vm.State))
	}
	if vm.MemoryMiB != 512 {
		t.Fatalf("内存换算错误: %d", vm.MemoryMiB)
	}
}

func TestParseFirstDisk(t *testing.T) {
	out := ` Target   Source
--------------------------------
 vda      /vm/web.qcow2
 hda      /iso/install.iso
`
	if got := parseFirstDisk(out); got != "/vm/web.qcow2" {
		t.Fatalf("应取第一个真实磁盘，实际 %q", got)
	}
	// 没有磁盘时返回空
	if got := parseFirstDisk(" Target   Source\n--------------------------------\n\n"); got != "" {
		t.Fatalf("无磁盘应返回空，实际 %q", got)
	}
}

func TestListViaFakeVirsh(t *testing.T) {
	r := &fakeRunner{
		out: map[string]string{
			"virsh list --all --name": "web\ndb\n",
			"virsh dominfo web":       "Name: web\nState: running\nCPU(s): 2\nMax memory: 1048576 KiB\n",
			"virsh dominfo db":        "Name: db\nState: shut off\nCPU(s): 4\nMax memory: 2097152 KiB\n",
			"virsh domblklist":        " Target   Source\n vda /vm/a.qcow2\n",
		},
	}
	m := &Manager{R: r}
	vms, err := m.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(vms) != 2 {
		t.Fatalf("应返回 2 台，实际 %d", len(vms))
	}
	if vms[0].Name != "web" || vms[0].StateText != "运行中" || vms[0].VCPU != 2 {
		t.Fatalf("web 解析错误: %+v", vms[0])
	}
	if vms[1].StateText != "已关机" || vms[1].MemoryMiB != 2048 {
		t.Fatalf("db 解析错误: %+v", vms[1])
	}
	if vms[0].DiskPath != "/vm/a.qcow2" {
		t.Fatalf("磁盘解析错误: %q", vms[0].DiskPath)
	}
}

func TestValidName(t *testing.T) {
	ok := []string{"web", "vm-1", "vm_2", "a.b.c", strings.Repeat("x", 63)}
	for _, n := range ok {
		if err := validName(n); err != nil {
			t.Fatalf("%q 应通过: %v", n, err)
		}
	}
	bad := []string{"", "-web", ".web", "web vm", "web;rm", "web/1", strings.Repeat("x", 64), "web$(id)"}
	for _, n := range bad {
		if err := validName(n); err == nil {
			t.Fatalf("%q 必须被拒绝", n)
		}
	}
}

func TestValidateCreate(t *testing.T) {
	dir := t.TempDir()
	disk := filepath.Join(dir, "vm.qcow2")
	if err := os.WriteFile(disk, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	good := CreateSpec{Name: "vm1", VCPU: 2, MemoryMiB: 1024, DiskPath: disk}
	if err := ValidateCreate(good); err != nil {
		t.Fatalf("合法参数被拒: %v", err)
	}

	cases := []struct {
		name string
		s    CreateSpec
	}{
		{"名称为空", CreateSpec{VCPU: 2, MemoryMiB: 512, DiskPath: disk}},
		{"名称带斜杠", CreateSpec{Name: "a/b", VCPU: 2, MemoryMiB: 512, DiskPath: disk}},
		{"vCPU 为 0", CreateSpec{Name: "v", VCPU: 0, MemoryMiB: 512, DiskPath: disk}},
		{"vCPU 过大", CreateSpec{Name: "v", VCPU: 128, MemoryMiB: 512, DiskPath: disk}},
		{"内存过小", CreateSpec{Name: "v", VCPU: 2, MemoryMiB: 64, DiskPath: disk}},
		{"内存过大", CreateSpec{Name: "v", VCPU: 2, MemoryMiB: 2097152, DiskPath: disk}},
		{"磁盘空", CreateSpec{Name: "v", VCPU: 2, MemoryMiB: 512}},
		{"磁盘相对路径", CreateSpec{Name: "v", VCPU: 2, MemoryMiB: 512, DiskPath: "vm.qcow2"}},
		{"磁盘含 ..", CreateSpec{Name: "v", VCPU: 2, MemoryMiB: 512, DiskPath: "/etc/../etc/passwd"}},
		{"磁盘带引号", CreateSpec{Name: "v", VCPU: 2, MemoryMiB: 512, DiskPath: `/tmp/a'b`}},
		{"磁盘不存在", CreateSpec{Name: "v", VCPU: 2, MemoryMiB: 512, DiskPath: "/nonexistent/x.qcow2"}},
	}
	for _, c := range cases {
		if err := ValidateCreate(c.s); err == nil {
			t.Fatalf("%s 必须被拒绝", c.name)
		}
	}
}

func TestRenderXML(t *testing.T) {
	s := CreateSpec{Name: "web", VCPU: 4, MemoryMiB: 2048, DiskPath: "/vm/web.qcow2", ISO: "/iso/x.iso", Network: "br0"}
	x := RenderXML(s)
	for _, want := range []string{
		"<name>web</name>", "<vcpu>4</vcpu>", "<memory unit='MiB'>2048</memory>",
		"<source file='/vm/web.qcow2'/>", "<source file='/iso/x.iso'/>",
		"<source network='br0'/>", "device='disk'", "device='cdrom'", "<readonly/>",
		"listen='127.0.0.1'", // VNC 只监听本机，别默认开到公网
	} {
		if !strings.Contains(x, want) {
			t.Fatalf("XML 缺少 %q\n%s", want, x)
		}
	}
	// 不带 ISO 时不该有 cdrom
	s.ISO = ""
	if strings.Contains(RenderXML(s), "cdrom") {
		t.Fatal("没有 ISO 时不该出现 cdrom")
	}
}

func TestStartValidatesName(t *testing.T) {
	r := &fakeRunner{out: map[string]string{}}
	m := &Manager{R: r}
	if err := m.Start("-bad"); err == nil {
		t.Fatal("非法名称应在调用 virsh 前被拒")
	}
	if len(r.got) != 0 {
		t.Fatalf("不该执行任何命令，实际 %v", r.got)
	}
}

func TestStartReportsVirshError(t *testing.T) {
	r := &fakeRunner{
		out: map[string]string{"virsh start web": "error: Domain is already running"},
		err: map[string]error{"virsh start web": fmt.Errorf("exit 1")},
	}
	m := &Manager{R: r}
	err := m.Start("web")
	if err == nil {
		t.Fatal("virsh 报错时应返回错误")
	}
	if !strings.Contains(err.Error(), "already running") {
		t.Fatalf("错误信息应带上 virsh 的输出: %v", err)
	}
}
