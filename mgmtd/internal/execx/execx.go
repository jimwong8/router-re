// Package execx 提供统一的命令执行抽象，供各配置模块的"执行层"复用。
//
// 设计目标：
//   - 可 mock：单元测试用 FakeRunner，不依赖真实系统
//   - 绝对路径解析：ip/tc/nft/dnsmasq 都在 /usr/sbin，非 root 进程 PATH 常不含它，
//     直接用 exec.Command("ip") 会 "executable file not found"
//   - 错误不吞：stderr 为空时回退 err.Error()，避免只看到 "command failed: "
package execx

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Runner 抽象命令执行。实现必须把 stderr 合并进返回的错误信息。
type Runner interface {
	// Run 执行命令并返回 stdout。
	Run(name string, args ...string) (string, error)
	// RunWithInput 执行命令并把 input 写到 stdin（用于 nft -f -、wg pubkey 等）。
	RunWithInput(input, name string, args ...string) (string, error)
}

// DefaultRunner 是真实执行器。
type DefaultRunner struct{}

// Run 执行命令，stdout 剥离首尾空白返回；失败时错误里带 stderr。
func (DefaultRunner) Run(name string, args ...string) (string, error) {
	return run("", name, args...)
}

// RunWithInput 把 input 写到 stdin 后执行。
func (DefaultRunner) RunWithInput(input, name string, args ...string) (string, error) {
	return run(input, name, args...)
}

func run(input, name string, args ...string) (string, error) {
	// Resolve bare command names to absolute paths. Daemons started by
	// systemd/procd often have a PATH without /usr/sbin, which turns a healthy
	// system into `exec: "tc": executable file not found in $PATH`. Doing this
	// inside DefaultRunner (not in each module) keeps FakeRunner-based tests
	// asserting plain command names.
	if !strings.ContainsRune(name, '/') {
		if p, err := Lookup(name); err == nil {
			name = p
		}
	}
	cmd := exec.Command(name, args...)
	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg == "" {
			msg = err.Error()
		}
		return stdout.String(), fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), msg)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// commonDirs 是系统管理命令常见的绝对路径目录（PATH 之外）。
var commonDirs = []string{"/usr/sbin", "/sbin", "/usr/bin", "/bin", "/usr/local/sbin"}

// Lookup 在 PATH 与常见 sbin 目录中查找可执行文件的绝对路径。
// 找不到返回错误；调用方应把返回的绝对路径用于后续执行。
func Lookup(name string) (string, error) {
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	for _, dir := range commonDirs {
		candidate := dir + "/" + name
		if isExecutable(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%q not found in PATH or %s", name, strings.Join(commonDirs, ", "))
}

// MustLookup 是 Lookup 的便捷包装：找不到时返回空串（调用方自行报错）。
func MustLookup(name string) string {
	p, _ := Lookup(name)
	return p
}

// ErrNotFound 表示环境中缺少必需的命令/服务。
var ErrNotFound = errors.New("required command not found")

// isExecutable 判断 path 是否为可执行普通文件。
func isExecutable(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !fi.IsDir() && fi.Mode()&0o111 != 0
}
