package easytier

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Status 是节点运行态。字段名与前端/接口保持一致。
type Status struct {
	Running      bool   `json:"running"`
	PID          int    `json:"pid"`
	Since        string `json:"since"`
	Config       string `json:"config"`
	Binary       string `json:"binary"`
	BinaryExists bool   `json:"binary_exists"`
	Version      string `json:"version"`
	LastError    string `json:"last_error"`
	LogPath      string `json:"log_path"`
}

type runningProc struct {
	cmd     *exec.Cmd
	cancel  context.CancelFunc
	started time.Time
	logPath string
}

var (
	procMu   sync.Mutex
	current  *runningProc
	lastErr  string
	verOnce  sync.Once
	verCache string
)

// LogPath 返回进程日志路径（启动失败时要能从接口里看到原因）。
func LogPath() string {
	if p := os.Getenv("MGM_EASYTIER_LOG"); p != "" {
		return p
	}
	return "/var/log/router-re/easytier.log"
}

// Version 返回 easytier-core 版本（取不到就返回空串，不当作错误）。
func Version() string {
	verOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, BinaryPath(), "--version").Output()
		if err == nil {
			verCache = strings.TrimSpace(string(out))
		}
	})
	return verCache
}

// BinaryAvailable 报告二进制是否存在（写死 true 会让界面在没装二进制时照样给"启用"按钮）。
func BinaryAvailable() bool {
	if strings.ContainsRune(BinaryPath(), os.PathSeparator) {
		_, err := os.Stat(BinaryPath())
		return err == nil
	}
	_, err := exec.LookPath(BinaryPath())
	return err == nil
}

// CurrentStatus 返回当前运行态。
func CurrentStatus() Status {
	procMu.Lock()
	p := current
	errMsg := lastErr
	procMu.Unlock()

	st := Status{
		Config:       ConfigPath(),
		Binary:       BinaryPath(),
		BinaryExists: BinaryAvailable(),
		Version:      Version(),
		LogPath:      LogPath(),
		LastError:    errMsg,
	}
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return st
	}
	// ProcessState 只在进程已退出时才非 nil；不能用 Signal(nil) 判断（返回 EINVAL）。
	if p.cmd.ProcessState != nil && p.cmd.ProcessState.Exited() {
		return st
	}
	st.Running = true
	st.PID = p.cmd.Process.Pid
	st.Since = p.started.Format(time.RFC3339)
	return st
}

// Stop 停止当前进程；没有在跑时返回 nil。
func Stop() error {
	procMu.Lock()
	p := current
	current = nil
	procMu.Unlock()
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	if p.cancel != nil {
		p.cancel()
	}
	done := make(chan error, 1)
	go func() {
		_, err := p.cmd.Process.Wait()
		done <- err
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		if err := p.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return fmt.Errorf("进程未能停止：%w", err)
		}
	}
	return nil
}

// Apply 校验 → 落盘 → 启停。enabled=false 时等价 Stop（配置保留）。
func Apply(c Config) error {
	if err := Validate(c); err != nil {
		return err
	}
	if !c.Enabled {
		if err := Stop(); err != nil {
			return err
		}
		return nil
	}
	if !BinaryAvailable() {
		return fmt.Errorf("找不到 easytier-core（%s），请先安装 EasyTier", BinaryPath())
	}
	path := ConfigPath()
	rpc := c.RPCPortal
	if rpc == "" {
		rpc = defaultRPCPortal
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建配置目录失败：%w", err)
	}
	// 配置里有网络密钥，权限 0600；先写 tmp 再 rename，避免读到半截文件。
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(RenderConfig(c)), 0o600); err != nil {
		return fmt.Errorf("写入配置失败：%w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("替换配置失败：%w", err)
	}

	if err := Stop(); err != nil {
		return err
	}

	logPath := LogPath()
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return fmt.Errorf("创建日志目录失败：%w", err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("打开日志文件失败：%w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	// 2.6.4 的配置文件不认 rpc_portal（写进 TOML 会被静默丢弃，节点仍监听默认 15888），
	// 必须用 -r 显式传；否则 easytier-cli 连不上管理端口，状态页永远是 0 个节点。
	cmd := exec.CommandContext(ctx, BinaryPath(), "-c", path, "-r", rpc)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		cancel()
		_ = logFile.Close()
		setLastError(err.Error())
		return fmt.Errorf("启动 easytier-core 失败：%w", err)
	}

	p := &runningProc{cmd: cmd, cancel: cancel, started: time.Now(), logPath: logPath}
	procMu.Lock()
	current = p
	lastErr = ""
	procMu.Unlock()

	go func() {
		err := cmd.Wait()
		_ = logFile.Close()
		procMu.Lock()
		if current == p {
			current = nil
			if err != nil {
				lastErr = tailLog(logPath, 400)
			}
		}
		procMu.Unlock()
	}()

	// 只等 2 秒：EasyTier 起 TUN 很快，秒退的情况这段时间就能看出来。
	time.Sleep(2 * time.Second)
	if st := CurrentStatus(); !st.Running {
		return fmt.Errorf("easytier-core 启动后立即退出：%s", strings.TrimSpace(tailLog(logPath, 300)))
	}
	return nil
}

func setLastError(msg string) {
	procMu.Lock()
	lastErr = msg
	procMu.Unlock()
}

// tailLog 取日志尾部若干字节，用于把启动失败的真实原因带回接口。
func tailLog(path string, n int64) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return ""
	}
	offset := st.Size() - n
	if offset < 0 {
		offset = 0
	}
	if _, err := f.Seek(offset, 0); err != nil {
		return ""
	}
	buf := make([]byte, n)
	read, _ := f.Read(buf)
	return string(buf[:read])
}

// Peers 通过 easytier-cli 拉取对等节点列表。
// 未运行时返回 (nil, false, nil)：调用方据此给空列表而不是 500。
func Peers(rpcPortal string) ([]Peer, bool, error) {
	st := CurrentStatus()
	if !st.Running {
		return nil, false, nil
	}
	if !BinaryAvailable() {
		return nil, false, fmt.Errorf("找不到 easytier-cli")
	}
	rpc := rpcPortal
	if rpc == "" {
		rpc = defaultRPCPortal
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, CLIPath(), "-p", rpc, "peer").CombinedOutput()
	if err != nil {
		return nil, false, fmt.Errorf("执行 easytier-cli 失败：%v: %s", err, strings.TrimSpace(string(out)))
	}
	peers := ParsePeerTable(string(out))
	if peers == nil {
		peers = []Peer{}
	}
	return peers, true, nil
}
