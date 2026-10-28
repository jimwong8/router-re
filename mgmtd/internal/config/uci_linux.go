//go:build linux

package config

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// UCIStore 是基于 OpenWrt `uci` 命令的真实配置存储实现。
//
// 通过 exec.Command("uci", ...) 调用系统 uci 命令：
//   - Get:      uci get <section>.<option>
//   - Set:      uci set <section>.<option>=<value>
//   - Snapshot: uci export 导出全部配置到临时文件
//   - Rollback: uci import 从快照文件导入，恢复配置
//   - Commit:   uci commit 固化暂存更改
type UCIStore struct {
	mu sync.Mutex
	// snapDir 存放快照导出文件的目录（默认系统临时目录）。
	snapDir string
}

// NewUCIStore 创建一个 UCI 配置存储。
func NewUCIStore() *UCIStore {
	return &UCIStore{snapDir: os.TempDir()}
}

// uciBin 返回 uci 命令路径；若系统无 uci 命令则报错。
func uciBin() (string, error) {
	path, err := exec.LookPath("uci")
	if err != nil {
		return "", errors.New("uci: uci command not found (not an OpenWrt/OpenWrt-like system?)")
	}
	return path, nil
}

// run 执行一条 uci 命令并返回其 stdout。
func (u *UCIStore) run(args ...string) (string, error) {
	bin, err := uciBin()
	if err != nil {
		return "", err
	}
	cmd := exec.Command(bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("uci %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// Get 返回指定配置项的值。section 或 option 为空时报错。
func (u *UCIStore) Get(section, option string) (string, error) {
	if section == "" || option == "" {
		return "", errors.New("uci: section and option must not be empty")
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.run("get", section+"."+option)
}

// Set 写入一个配置项。section 或 option 为空时报错。
func (u *UCIStore) Set(section, option, value string) error {
	if section == "" || option == "" {
		return errors.New("uci: section and option must not be empty")
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	_, err := u.run("set", section+"."+option+"="+value)
	return err
}

// Unset 用 `uci delete` 删除单个 option。键不存在时 uci 返回非零，视为已删除。
func (u *UCIStore) Unset(section, option string) error {
	if section == "" || option == "" {
		return errors.New("uci: section and option must not be empty")
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if _, err := u.run("delete", section+"."+option); err != nil {
		// 目标本来就不存在时 uci 报 Entry not found，等价于已达目的。
		return nil
	}
	return nil
}

// Snapshot 通过 `uci export` 导出全部配置到临时文件，返回该文件路径作为快照标识符。
func (u *UCIStore) Snapshot() (string, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	exp, err := u.run("export")
	if err != nil {
		return "", err
	}
	f, err := os.CreateTemp(u.snapDir, "mgmtd-snap-*.uci")
	if err != nil {
		return "", fmt.Errorf("uci: create snapshot file: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(exp); err != nil {
		return "", fmt.Errorf("uci: write snapshot: %w", err)
	}
	return f.Name(), nil
}

// Rollback 通过 `uci import` 从快照文件恢复配置。
// 快照文件不存在时返回错误。
func (u *UCIStore) Rollback(snap string) error {
	if snap == "" {
		return errors.New("uci: snapshot id must not be empty")
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if _, err := os.Stat(snap); err != nil {
		return fmt.Errorf("uci: snapshot %s not found: %w", snap, err)
	}
	_, err := u.run("import", snap)
	return err
}

// Commit 通过 `uci commit` 固化暂存更改。
func (u *UCIStore) Commit() error {
	u.mu.Lock()
	defer u.mu.Unlock()
	_, err := u.run("commit")
	return err
}

var _ Store = (*UCIStore)(nil)
