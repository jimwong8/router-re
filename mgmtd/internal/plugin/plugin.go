// Package plugin 管理可插拔的应用（应用商店）与 Docker 容器。
//
// 这一层的现实约束必须写在代码里而不是文档里：
// 路由器上装插件等于往设备里放一个会长期运行的进程，出问题就是断网。
// 所以这里不做"一键下载安装"——在没有包管理器、没有外网的现场，
// 那只会变成"点了没反应"或者"装了个跑不起来的空壳"。
//
// 这里做的是三件确定的事：
//  1. 一份内置应用清单 + 现场可用性探测（二进制在不在、版本号）
//  2. 启停控制与开机自启标记（写 /etc/router-re/plugins/<id>.enabled）
//  3. 状态如实上报：missing / stopped / running / disabled
package plugin

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

// StateDir 存放插件的启停标记与配置。
const StateDir = "/etc/router-re/plugins"

// Plugin 是应用商店里的一个条目。
type Plugin struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Category string   `json:"category"` // 网络 / 存储 / 监控 / 代理 / 容器
	Desc     string   `json:"desc"`
	Binary   string   `json:"binary"`   // 用来探测是否已安装
	Argv     []string `json:"argv"`     // 启动命令（binary 之后的参数）
	Ports    []int    `json:"ports"`    // 占用的端口，用于冲突提示
	Homepage string   `json:"homepage"` // 项目地址，现场装的时候照着装
	Note     string   `json:"note,omitempty"`
}

// Catalog 是内置清单。
//
// 选的都是路由器上真正常用的：DDNS、内网穿透、DNS 加速、文件管理、
// 下载、远程访问。没有放"看起来很酷但路由器跑不动"的东西。
var Catalog = []Plugin{
	{ID: "ddns-go", Name: "DDNS-GO", Category: "网络", Binary: "ddns-go", Argv: []string{"-l", ":9876"},
		Desc: "多平台动态域名解析，支持阿里云/腾讯云/Cloudflare", Ports: []int{9876}, Homepage: "https://github.com/jeessy2/ddns-go"},
	{ID: "frps", Name: "frp 服务端", Category: "代理", Binary: "frps", Argv: []string{"-c", StateDir + "/frps.toml"},
		Desc: "内网穿透服务端，把内网端口映射到公网", Ports: []int{7000}, Homepage: "https://github.com/fatedier/frp"},
	{ID: "frpc", Name: "frp 客户端", Category: "代理", Binary: "frpc", Argv: []string{"-c", StateDir + "/frpc.toml"},
		Desc: "内网穿透客户端，主动连到公网服务端", Homepage: "https://github.com/fatedier/frp"},
	{ID: "smartdns", Name: "SmartDNS", Category: "网络", Binary: "smartdns", Argv: []string{"-f", "-c", StateDir + "/smartdns.conf"},
		Desc: "本地 DNS 加速与分流，多上游并发择优", Ports: []int{6353}, Homepage: "https://github.com/pymumu/smartdns"},
	{ID: "adguardhome", Name: "AdGuard Home", Category: "网络", Binary: "AdGuardHome", Argv: []string{"--no-check-update", "-c", StateDir + "/AdGuardHome.yaml"},
		Desc: "DNS 广告拦截与家庭防护，带 Web 面板", Ports: []int{3000, 53}, Homepage: "https://github.com/AdguardTeam/AdGuardHome"},
	{ID: "tailscale", Name: "Tailscale", Category: "网络", Binary: "tailscaled",
		Desc: "基于 WireGuard 的零配置组网，跨 NAT 直连", Ports: []int{41641}, Homepage: "https://tailscale.com/"},
	{ID: "zerotier", Name: "ZeroTier", Category: "网络", Binary: "zerotier-one",
		Desc: "虚拟局域网组网，适合多层 NAT 环境", Ports: []int{9993}, Homepage: "https://www.zerotier.com/"},
	{ID: "filebrowser", Name: "File Browser", Category: "存储", Binary: "filebrowser", Argv: []string{"-a", "0.0.0.0", "-p", "8080", "-r", "/etc/disk_user"},
		Desc: "网页版文件管理器，可直接管理存储目录", Ports: []int{8080}, Homepage: "https://github.com/filebrowser/filebrowser"},
	{ID: "alist", Name: "AList", Category: "存储", Binary: "alist", Argv: []string{"server"},
		Desc: "多网盘聚合挂载，统一浏览与直链", Ports: []int{5244}, Homepage: "https://github.com/alist-org/alist"},
	{ID: "aria2", Name: "Aria2", Category: "存储", Binary: "aria2c", Argv: []string{"--enable-rpc", "--rpc-listen-all=false", "--rpc-listen-port=6800"},
		Desc: "多协议下载器，支持 BT/磁力/HTTP", Ports: []int{6800}, Homepage: "https://aria2.github.io/"},
	{ID: "cloudflared", Name: "Cloudflare Tunnel", Category: "代理", Binary: "cloudflared", Argv: []string{"tunnel", "run"},
		Desc: "无需公网 IP 把内网服务发布到 Cloudflare", Homepage: "https://github.com/cloudflare/cloudflared"},
	{ID: "node-exporter", Name: "Node Exporter", Category: "监控", Binary: "node_exporter", Argv: []string{"--web.listen-address=:9100"},
		Desc: "Prometheus 指标采集端点", Ports: []int{9100}, Homepage: "https://github.com/prometheus/node_exporter"},
	{ID: "speedtest", Name: "Speedtest CLI", Category: "监控", Binary: "speedtest",
		Desc: "命令行测速，用于宽带质量巡检", Homepage: "https://www.speedtest.net/apps/cli"},
	{ID: "docker", Name: "Docker", Category: "容器", Binary: "dockerd",
		Desc: "容器运行时。注意：宿主容器里通常没有 NET_ADMIN，跑不了网络类容器", Homepage: "https://www.docker.com/"},
}

// Status 是一个插件的现场状态。
type Status struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Category   string `json:"category"`
	Desc       string `json:"desc"`
	Homepage   string `json:"homepage"`
	Binary     string `json:"binary"`
	Installed  bool   `json:"installed"` // 二进制是否存在
	Path       string `json:"path,omitempty"`
	Version    string `json:"version,omitempty"`
	Enabled    bool   `json:"enabled"` // 开机自启标记
	Running    bool   `json:"running"`
	PID        int    `json:"pid,omitempty"`
	Ports      []int  `json:"ports,omitempty"`
	Note       string `json:"note,omitempty"`
	StateText  string `json:"state_text"` // 中文状态，界面直接用
	ConfigPath string `json:"config_path,omitempty"`
}

// Manager 负责探测与启停。
type Manager struct {
	Dir string // 状态目录，默认 StateDir
}

// NewManager 返回绑定到 dir 的管理器。
func NewManager(dir string) *Manager {
	if dir == "" {
		dir = StateDir
	}
	return &Manager{Dir: dir}
}

// enabledFlag 返回插件的自启标记路径。
func (m *Manager) enabledFlag(id string) string {
	return filepath.Join(m.Dir, id+".enabled")
}

// StatusOf 探测单个插件。
func (m *Manager) StatusOf(p Plugin) Status {
	st := Status{
		ID: p.ID, Name: p.Name, Category: p.Category, Desc: p.Desc,
		Homepage: p.Homepage, Binary: p.Binary, Ports: p.Ports,
		Note: p.Note, ConfigPath: filepath.Join(m.Dir, p.ID+".conf"),
	}
	if path, err := exec.LookPath(p.Binary); err == nil {
		st.Installed = true
		st.Path = path
		// 版本号刻意不在列表里探测：任何一条外部命令都可能把接口拖死，
		// 需要版本时走 Version() 单独取（见下面的说明）。
	}
	if _, err := os.Stat(m.enabledFlag(p.ID)); err == nil {
		st.Enabled = true
	}
	if st.Installed {
		st.Running, st.PID = probeRunning(p.Binary)
	}

	switch {
	case !st.Installed:
		st.StateText = "未安装"
	case st.Running:
		st.StateText = "运行中"
	case st.Enabled:
		st.StateText = "已启用但未运行"
	default:
		st.StateText = "已停止"
	}
	return st
}

// List 返回全部插件的状态，按分类再按名称排序。
//
// 并发探测：每个插件的状态要靠 LookPath + pgrep + 版本命令，串行跑
// 十几个插件会把接口拖到几十秒——而任何一条版本命令都可能卡住
// （有程序把 `version` 当成"启动服务"的别名），所以既并发也必须带超时。
func (m *Manager) List() []Status {
	out := make([]Status, len(Catalog))
	var wg sync.WaitGroup
	for i := range Catalog {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			out[i] = m.StatusOf(Catalog[i])
		}(i)
	}
	wg.Wait()
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Category != out[j].Category {
			return out[i].Category < out[j].Category
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Find 按 ID 查清单条目。
func Find(id string) (Plugin, bool) {
	for _, p := range Catalog {
		if p.ID == id {
			return p, true
		}
	}
	return Plugin{}, false
}

// SetEnabled 写/删自启标记。它只表达"重启后要不要起来"，不启动进程本身。
func (m *Manager) SetEnabled(id string, on bool) error {
	if _, ok := Find(id); !ok {
		return fmt.Errorf("未知插件 %q", id)
	}
	if err := os.MkdirAll(m.Dir, 0o755); err != nil {
		return err
	}
	flag := m.enabledFlag(id)
	if on {
		return os.WriteFile(flag, []byte("1\n"), 0o644)
	}
	if err := os.Remove(flag); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Start 启动插件进程。
//
// 用 setsid 脱离当前会话：mgmtd 重启时不能把插件一起带走，
// 也不能让插件占着 mgmtd 的管道（那会让 mgmtd 的日志读不到 EOF）。
func (m *Manager) Start(id string) (int, error) {
	p, ok := Find(id)
	if !ok {
		return 0, fmt.Errorf("未知插件 %q", id)
	}
	path, err := exec.LookPath(p.Binary)
	if err != nil {
		return 0, fmt.Errorf("%s 未安装，无法启动（请先把二进制放到 PATH 里）", p.Binary)
	}
	if running, pid := probeRunning(p.Binary); running {
		return pid, fmt.Errorf("%s 已在运行（PID %d）", p.Binary, pid)
	}
	if err := os.MkdirAll(m.Dir, 0o755); err != nil {
		return 0, err
	}
	logPath := filepath.Join(m.Dir, id+".log")
	logf, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, fmt.Errorf("打开日志失败: %w", err)
	}
	defer logf.Close()

	args := append([]string{}, p.Argv...)
	cmd := exec.Command("setsid", append([]string{path}, args...)...)
	cmd.Stdout = logf
	cmd.Stderr = logf
	cmd.Dir = m.Dir
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("启动失败: %w", err)
	}
	pid := cmd.Process.Pid
	// 必须 Wait，否则插件退出后留下僵尸进程（父进程是 mgmtd，没人回收）。
	// 放在 goroutine 里等，不阻塞 HTTP 响应；setsid 已让它脱离会话，
	// mgmtd 重启不会把它带走。
	go func() { _ = cmd.Wait() }()
	return pid, nil
}

// Stop 停止插件进程。
//
// 关键点是杀**整个进程组**而不是单个 PID：插件是用 setsid 启动的，
// 所以它的 PGID 等于自己的 PID。如果只 kill 父进程，脚本型插件里的
// 子进程（sleep、实际的守护进程）会活下来，界面上就会出现
// "点了停止，状态还是运行中"。
func (m *Manager) Stop(id string) error {
	p, ok := Find(id)
	if !ok {
		return fmt.Errorf("未知插件 %q", id)
	}
	running, pid := probeRunning(p.Binary)
	if !running {
		return fmt.Errorf("%s 没有在运行", p.Binary)
	}
	// 先温和地终止整组
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
		// PGID 不等于 PID 时（外部启动的进程）退回单进程
		if err2 := syscall.Kill(pid, syscall.SIGTERM); err2 != nil {
			return fmt.Errorf("停止失败（PID %d）: %w", pid, err)
		}
	}
	// 给它 3 秒体面退出
	for i := 0; i < 30; i++ {
		time.Sleep(100 * time.Millisecond)
		if alive, _ := probeRunning(p.Binary); !alive {
			return nil
		}
	}
	// 还不走就强杀整组
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	_ = syscall.Kill(pid, syscall.SIGKILL)
	time.Sleep(300 * time.Millisecond)
	if alive, stillPID := probeRunning(p.Binary); alive {
		return fmt.Errorf("停止失败：%s 仍在运行（PID %d）", p.Binary, stillPID)
	}
	return nil
}

// probeRunning 用 pgrep -x 精确匹配进程名，并**跳过僵尸进程**。
//
// pgrep 把僵尸也算作命中（它还在进程表里），但僵尸既不能 kill 也不干活。
// 不排除的话，插件退出后会永远显示"运行中"，而"停止"按钮永远失败。
func probeRunning(name string) (bool, int) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "pgrep", "-x", name).Output()
	if err != nil {
		return false, 0
	}
	for _, f := range strings.Fields(string(out)) {
		var pid int
		if _, err := fmt.Sscanf(f, "%d", &pid); err != nil || pid <= 0 {
			continue
		}
		if isZombie(pid) {
			continue
		}
		return true, pid
	}
	return false, 0
}

// isZombie 通过 /proc/<pid>/stat 的进程状态判断是否僵尸（Z）或已死（X）。
func isZombie(pid int) bool {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return true // 进程已经没了
	}
	s := string(data)
	i := strings.LastIndexByte(s, ')')
	if i < 0 || i+2 >= len(s) {
		return false
	}
	switch s[i+2] {
	case 'Z', 'X':
		return true
	}
	return false
}

// Version 探测插件的版本号，带硬超时。
//
// 两个坑都要防：
//  1. 程序把 `version` 当成"启动服务"的别名，直接阻塞；
//  2. 程序 fork 出守护进程并让它继承 stdout —— 这时即使 CommandContext
//     杀掉直接子进程，ReadAll 仍会等管道 EOF，接口就永久挂住。
//     WaitDelay 会在 ctx 到期后强制关闭管道，是这类场景的正解。
func (m *Manager) Version(id string) (string, error) {
	p, ok := Find(id)
	if !ok {
		return "", fmt.Errorf("未知插件 %q", id)
	}
	bin, err := exec.LookPath(p.Binary)
	if err != nil {
		return "", fmt.Errorf("%s 未安装", p.Binary)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "--version")
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("读取版本失败: %w", err)
	}
	line := strings.TrimSpace(string(out))
	if i := strings.IndexByte(line, '\n'); i > 0 {
		line = line[:i]
	}
	if len(line) > 80 {
		line = line[:80]
	}
	return line, nil
}

// PortConflicts 找出两两冲突的端口，用于界面上提前提醒。
func PortConflicts() map[int][]string {
	owner := map[int][]string{}
	for _, p := range Catalog {
		for _, port := range p.Ports {
			owner[port] = append(owner[port], p.Name)
		}
	}
	out := map[int][]string{}
	for port, names := range owner {
		if len(names) > 1 {
			out[port] = names
		}
	}
	return out
}
