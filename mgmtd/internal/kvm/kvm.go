// Package kvm 管理 KVM 虚拟机（经由 libvirt/virsh）。
//
// 路由器上跑 KVM 的前提是三条同时成立：CPU 有虚拟化扩展、/dev/kvm 可访问、
// libvirt 在跑。少任何一条，界面上就该显示"不可用 + 具体缺什么"，
// 而不是给一堆点了没反应的按钮。
//
// 因此这个包的第一件事是 Detect()——把能力探测做成一等公民。
package kvm

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Capability 是当前环境对 KVM 的支持情况。
type Capability struct {
	HasDevice      bool   `json:"has_device"`      // /dev/kvm 存在
	DeviceWritable bool   `json:"device_writable"` // 当前进程能读写它
	HasQemu        bool   `json:"has_qemu"`        // qemu-system-* 存在
	QemuBinary     string `json:"qemu_binary,omitempty"`
	HasVirsh       bool   `json:"has_virsh"`        // virsh 存在
	LibvirtRunning bool   `json:"libvirt_running"`  // libvirtd/virtqemud 在跑
	Available      bool   `json:"available"`        // 能否管理虚机
	Reason         string `json:"reason,omitempty"` // 不可用的原因（给用户看）
	CPUVirtFlags   bool   `json:"cpu_virt_flags"`   // /proc/cpuinfo 里有 vmx/svm
}

// Detect 探测当前环境的虚拟化能力。
func Detect() Capability {
	var c Capability

	if _, err := os.Stat("/dev/kvm"); err == nil {
		c.HasDevice = true
		// 能打开才算真可用：设备节点存在但权限不对是最常见的坑
		f, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0)
		if err == nil {
			c.DeviceWritable = true
			f.Close()
		}
	}

	if data, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		s := string(data)
		c.CPUVirtFlags = strings.Contains(s, " vmx") || strings.Contains(s, " svm")
	}

	for _, bin := range []string{"qemu-system-x86_64", "qemu-system-aarch64"} {
		if p, err := exec.LookPath(bin); err == nil {
			c.HasQemu = true
			c.QemuBinary = p
			break
		}
	}

	if p, err := exec.LookPath("virsh"); err == nil {
		c.HasVirsh = true
		_ = p
	}
	if c.HasVirsh {
		// virsh list 能跑通就说明有可连上的 libvirtd
		ctx, cancel := timeoutCtx(3 * time.Second)
		defer cancel()
		if err := exec.CommandContext(ctx, "virsh", "list", "--all").Run(); err == nil {
			c.LibvirtRunning = true
		}
	}

	// 逐条给出不可用的原因，顺序按"最先卡住的那一步"
	switch {
	case !c.CPUVirtFlags && !c.HasDevice:
		c.Reason = "CPU 未启用虚拟化扩展（BIOS 里的 VT-x/AMD-V 或内核参数），且没有 /dev/kvm"
	case !c.HasDevice:
		c.Reason = "没有 /dev/kvm：容器内通常看不到宿主机的 KVM 设备，需要在宿主机上开或把设备透传进来"
	case !c.DeviceWritable:
		c.Reason = "/dev/kvm 存在但当前用户无读写权限（需要 root 或加入 kvm 组）"
	case !c.HasVirsh:
		c.Reason = "KVM 设备可用，但未安装 libvirt（virsh），无法管理虚机"
	case !c.LibvirtRunning:
		c.Reason = "已安装 libvirt，但 libvirtd 没有运行"
	default:
		c.Available = true
	}
	return c
}

// VM 是一台虚拟机。
type VM struct {
	Name      string `json:"name"`
	State     string `json:"state"`      // running / shut off / paused ...
	StateText string `json:"state_text"` // 中文状态
	VCPU      int    `json:"vcpu"`
	MemoryMiB int    `json:"memory_mib"`
	DiskPath  string `json:"disk_path,omitempty"`
	Autostart bool   `json:"autostart"`
	UUID      string `json:"uuid,omitempty"`
}

// timeoutCtx 是所有外部命令的统一超时入口。
func timeoutCtx(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}

// runner 抽象命令执行，便于测试。
type runner interface {
	Run(name string, args ...string) (string, error)
}

type realRunner struct{}

func (realRunner) Run(name string, args ...string) (string, error) {
	ctx, cancel := timeoutCtx(8 * time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = 2 * time.Second
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// Manager 通过 virsh 操作虚机。
type Manager struct {
	R runner
}

// NewManager 用真实命令执行器。
func NewManager() *Manager { return &Manager{R: realRunner{}} }

// List 列出全部虚机。没有 libvirt 时返回空列表（调用方先看 Detect）。
func (m *Manager) List() ([]VM, error) {
	out, err := m.R.Run("virsh", "list", "--all", "--name")
	if err != nil {
		return nil, fmt.Errorf("virsh list 失败: %s", strings.TrimSpace(out))
	}
	names := parseVMNames(out)
	vms := make([]VM, 0, len(names))
	for _, name := range names {
		vms = append(vms, m.info(name))
	}
	return vms, nil
}

// info 取单台虚机的详细信息。解析失败不整体报错，字段留空即可。
func (m *Manager) info(name string) VM {
	vm := VM{Name: name}
	if out, err := m.R.Run("virsh", "dominfo", name); err == nil {
		parseDominfo(out, &vm)
	}
	if out, err := m.R.Run("virsh", "domblklist", name); err == nil {
		vm.DiskPath = parseFirstDisk(out)
	}
	vm.StateText = stateText(vm.State)
	return vm
}

// parseVMNames 清理 virsh list --name 的输出。
func parseVMNames(out string) []string {
	var names []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "error:") || strings.HasPrefix(line, "Id ") {
			continue
		}
		names = append(names, line)
	}
	return names
}

// parseDominfo 解析 `virsh dominfo` 的 key: value 输出。
func parseDominfo(out string, vm *VM) {
	for _, line := range strings.Split(out, "\n") {
		i := strings.Index(line, ":")
		if i < 0 {
			continue
		}
		key := strings.TrimSpace(line[:i])
		val := strings.TrimSpace(line[i+1:])
		switch key {
		case "State":
			vm.State = val
		case "CPU(s)":
			vm.VCPU, _ = strconv.Atoi(val)
		case "Max memory":
			// 单位是 KiB，如 "2097152 KiB"
			fields := strings.Fields(val)
			if len(fields) > 0 {
				if n, err := strconv.Atoi(fields[0]); err == nil {
					vm.MemoryMiB = n / 1024
				}
			}
		case "UUID":
			vm.UUID = val
		case "Autostart":
			vm.Autostart = strings.EqualFold(val, "enable") || strings.EqualFold(val, "是")
		}
	}
}

// parseFirstDisk 从 domblklist 里取第一个真实磁盘（跳过 cdrom 和快照）。
func parseFirstDisk(out string) string {
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		if f[0] == "Target" || f[0] == "-----" || strings.HasPrefix(f[0], "-") {
			continue
		}
		path := f[len(f)-1]
		if strings.HasPrefix(path, "/") {
			return path
		}
	}
	return ""
}

func stateText(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "running":
		return "运行中"
	case "shut off", "shutoff":
		return "已关机"
	case "paused":
		return "已暂停"
	case "crashed":
		return "已崩溃"
	case "pmsuspended":
		return "休眠"
	case "":
		return "未知"
	}
	return s
}

// Start 启动虚机。
func (m *Manager) Start(name string) error {
	return m.runFor(name, "start", "启动")
}

// Shutdown 请求虚机正常关机（ACPI）。它可能因为客户机没响应而超时。
func (m *Manager) Shutdown(name string) error {
	return m.runFor(name, "shutdown", "关机")
}

// Destroy 强制断电。用于客户机卡死时。
func (m *Manager) Destroy(name string) error {
	return m.runFor(name, "destroy", "强制断电")
}

// SetAutostart 设置开机自启。
func (m *Manager) SetAutostart(name string, on bool) error {
	arg := "--disable"
	if on {
		arg = "--enable"
	}
	return m.runFor(name, "autostart", "设置自启", arg)
}

func (m *Manager) runFor(name, verb, verbCN string, extra ...string) error {
	if err := validName(name); err != nil {
		return err
	}
	args := append([]string{verb}, name)
	args = append(args, extra...)
	out, err := m.R.Run("virsh", args...)
	if err != nil {
		return fmt.Errorf("%s失败: %s", verbCN, strings.TrimSpace(out))
	}
	return nil
}

var vmNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$`)

// netNameRe 是 libvirt 网络名的保守子集（与 vmNameRe 同形）。
var netNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$`)

// validNetworkName 校验 Network 字段。
//
// 这个字段以前完全没被校验过：ValidateCreate 只查 Name/VCPU/Memory/DiskPath/ISO，
// 而 RenderXML 把它原样插进 <source network='...'/>。传
//   default'/><console type='file' path='/etc/passwd
// 就能在 <devices> 里凭空多出一个任意 XML 元素——libvirt 接受 <hostdev>
// 就会把宿主机 PCI 设备直通给 guest，直接越出虚拟化边界。
func validNetworkName(name string) error {
	if name == "" {
		return nil // 空表示用默认网络
	}
	if !netNameRe.MatchString(name) {
		return fmt.Errorf("网络名只能用字母、数字、点、下划线、短横线，且以字母或数字开头（最长 63）: %q", name)
	}
	return nil
}

// xmlEscape 转义 XML 文本/属性里的五个特殊字符，并剔除 XML 1.0 不允许的
// 控制字符。
//
// 渲染层必须自己转义，不能依赖调用方跑过校验：即使将来新增字段忘了校验，
// 转义也能保证注入的值只会是"一段字符串"，改不了文档结构。
//
// 控制字符（除 	 \n \r 外）在 XML 1.0 里连转义都不允许出现，原样写进去
// 会让 libvirt 报解析错误——等于一次请求就能让这台虚机永远定义不出来。
func xmlEscape(s string) string {
	var clean strings.Builder
	for _, r := range s {
		switch {
		case r == '	' || r == '\n' || r == '\r':
			clean.WriteByte(' ')
		case r < 0x20 || r == 0x7f:
			// 丢弃
		default:
			clean.WriteRune(r)
		}
	}
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return r.Replace(clean.String())
}

// validName 校验虚机名：libvirt 允许的保守子集，且禁止以 - 开头（会被当成参数）。
func validName(name string) error {
	if name == "" {
		return fmt.Errorf("虚机名不能为空")
	}
	if !vmNameRe.MatchString(name) {
		return fmt.Errorf("虚机名只能用字母、数字、点、下划线、短横线，且以字母或数字开头（最长 63）")
	}
	return nil
}

// CreateSpec 是创建虚机的参数。
type CreateSpec struct {
	Name      string `json:"name"`
	VCPU      int    `json:"vcpu"`
	MemoryMiB int    `json:"memory_mib"`
	DiskPath  string `json:"disk_path"` // 必须是已存在的块设备或镜像文件
	ISO       string `json:"iso,omitempty"`
	Network   string `json:"network"` // 默认 default
	Autostart bool   `json:"autostart"`
}

// ValidateCreate 校验创建参数。
//
// 这里把"能在别处造成破坏"的输入挡在渲染 XML 之前：名称进的是 libvirt
// 的标识符，磁盘路径进的是 qemu 的 -drive，任何一处带引号或 .. 都可能
// 变成意料之外的路径。
func ValidateCreate(s CreateSpec) error {
	if err := validName(s.Name); err != nil {
		return err
	}
	// Network 以前完全没校验。它直接进 <source network='...'/>，带引号就能
	// 闭合属性并在 <devices> 里插入任意 XML 元素（如 <hostdev> 直通宿主机
	// PCI 设备）。这里与虚机名同一套白名单。
	if err := validNetworkName(s.Network); err != nil {
		return fmt.Errorf("网络: %w", err)
	}
	if s.VCPU < 1 || s.VCPU > 64 {
		return fmt.Errorf("vCPU 必须在 1-64 之间")
	}
	if s.MemoryMiB < 128 || s.MemoryMiB > 1048576 {
		return fmt.Errorf("内存必须在 128 MiB - 1 TiB 之间")
	}
	if s.DiskPath == "" {
		return fmt.Errorf("必须指定磁盘")
	}
	if err := validAbsPath(s.DiskPath); err != nil {
		return fmt.Errorf("磁盘路径: %w", err)
	}
	if s.ISO != "" {
		if err := validAbsPath(s.ISO); err != nil {
			return fmt.Errorf("ISO 路径: %w", err)
		}
	}
	// 磁盘必须真实存在：让 virsh define 成功后才发现路径写错，
	// 用户会得到一台起不来的虚机，而不是一条明确的报错。
	if _, err := os.Stat(s.DiskPath); err != nil {
		return fmt.Errorf("磁盘不存在: %s", s.DiskPath)
	}
	return nil
}

// validAbsPath 拒绝相对路径、.. 和会破坏 XML/命令行语义的字符。
func validAbsPath(p string) error {
	if !strings.HasPrefix(p, "/") {
		return fmt.Errorf("必须是绝对路径")
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return fmt.Errorf("不能包含 ..")
		}
	}
	if strings.ContainsAny(p, "\n\r\t\"'`$") {
		return fmt.Errorf("路径包含非法字符")
	}
	return nil
}

// RenderXML 生成 libvirt domain XML。只包含必要元素，避免生成一堆看不懂的默认值。
func RenderXML(s CreateSpec) string {
	net := s.Network
	if net == "" {
		net = "default"
	}
	// 渲染层兜底：所有插值一律转义。即使调用方漏了 ValidateCreate，
	// 注入的值也只能是"一段字符串"，无法闭合属性或插入新元素。
	// Network 额外做白名单过滤——它不是路径，非法值直接回落到 default。
	if !netNameRe.MatchString(net) {
		net = "default"
	}
	name := xmlEscape(s.Name)
	diskPath := xmlEscape(s.DiskPath)
	iso := xmlEscape(s.ISO)
	net = xmlEscape(net)
	var b strings.Builder
	b.WriteString("<domain type='kvm'>\n")
	fmt.Fprintf(&b, "  <name>%s</name>\n", name)
	b.WriteString("  <memory unit='MiB'>" + strconv.Itoa(s.MemoryMiB) + "</memory>\n")
	b.WriteString("  <vcpu>" + strconv.Itoa(s.VCPU) + "</vcpu>\n")
	b.WriteString("  <os>\n    <type arch='x86_64'>hvm</type>\n    <boot dev='hd'/>\n")
	if s.ISO != "" {
		b.WriteString("    <boot dev='cdrom'/>\n")
	}
	b.WriteString("  </os>\n")
	b.WriteString("  <features><acpi/><apic/></features>\n")
	b.WriteString("  <devices>\n")
	b.WriteString("    <disk type='file' device='disk'>\n      <driver name='qemu' type='qcow2'/>\n")
	fmt.Fprintf(&b, "      <source file='%s'/>\n", diskPath)
	b.WriteString("      <target dev='vda' bus='virtio'/>\n    </disk>\n")
	if s.ISO != "" {
		b.WriteString("    <disk type='file' device='cdrom'>\n")
		fmt.Fprintf(&b, "      <source file='%s'/>\n", iso)
		b.WriteString("      <target dev='hda' bus='ide'/>\n      <readonly/>\n    </disk>\n")
	}
	fmt.Fprintf(&b, "    <interface type='network'><source network='%s'/><model type='virtio'/></interface>\n", net)
	b.WriteString("    <graphics type='vnc' port='-1' listen='127.0.0.1'/>\n")
	b.WriteString("    <console type='pty'/>\n")
	b.WriteString("  </devices>\n</domain>\n")
	return b.String()
}

// Create 定义并（可选）启动虚机。
func (m *Manager) Create(s CreateSpec, start bool) error {
	if err := ValidateCreate(s); err != nil {
		return err
	}
	dir := "/etc/router-re/vms"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	xmlPath := dir + "/" + s.Name + ".xml"
	if err := os.WriteFile(xmlPath, []byte(RenderXML(s)), 0o644); err != nil {
		return fmt.Errorf("写 XML 失败: %w", err)
	}
	if out, err := m.R.Run("virsh", "define", xmlPath); err != nil {
		return fmt.Errorf("virsh define 失败: %s", strings.TrimSpace(out))
	}
	if s.Autostart {
		_ = m.SetAutostart(s.Name, true)
	}
	if start {
		if err := m.Start(s.Name); err != nil {
			return fmt.Errorf("已定义但启动失败: %w", err)
		}
	}
	return nil
}
