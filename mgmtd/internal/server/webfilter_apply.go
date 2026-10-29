package server

// webfilter apply 的可写性预检与状态记录。
//
// 为什么单独一个文件：handleWebfilterApply 原本直接用包级全局变量记录
// 上一次结果（webfilterLastOK / LastErr / LastScript），没有锁。面板上
// 双击"应用"就是两个 goroutine 同时写这三个变量——go test -race 会直接
// 报数据竞争，而生产上表现为 status 接口返回错乱的上一次结果。

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	webfilterStateMu     sync.Mutex
	webfilterLastOK      bool
	webfilterLastErr     string
	webfilterLastScript  string
	webfilterLastApplied int64
)

// recordWebfilterResult 线程安全地记录上一次下发结果。
func recordWebfilterResult(ok bool, script, errMsg string) {
	webfilterStateMu.Lock()
	defer webfilterStateMu.Unlock()
	webfilterLastOK = ok
	webfilterLastScript = script
	webfilterLastErr = errMsg
	webfilterLastApplied = time.Now().Unix()
}

// webfilterResult 线程安全地读回上一次下发结果。
func webfilterResult() (ok bool, script, errMsg string, at int64) {
	webfilterStateMu.Lock()
	defer webfilterStateMu.Unlock()
	return webfilterLastOK, webfilterLastScript, webfilterLastErr, webfilterLastApplied
}

// webfilterConfPath 返回 dnsmasq 配置文件路径，可用 MGM_WEBFILTER_CONF 覆盖
// （测试与非 root 开发实例用）。
func webfilterConfPath() string {
	if p := os.Getenv("MGM_WEBFILTER_CONF"); p != "" {
		return p
	}
	return "/etc/dnsmasq.d/router-re-webfilter.conf"
}

// checkConfWritable 在真正下发前判断目标目录能不能写。
//
// 没有这道预检时，非 root 运行会一路走到写文件才失败，返回 500
// "apply failed: permission denied"——500 暗示服务端有 bug，运维会去翻代码，
// 而真正的答案是"这个进程没有 /etc/dnsmasq.d 的写权限"。改为 403 并说清。
func checkConfWritable(path string) error {
	dir := filepath.Dir(path)
	fi, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("配置目录 %s 不可用: %w", dir, err)
	}
	if !fi.IsDir() {
		return fmt.Errorf("%s 不是目录", dir)
	}
	probe := filepath.Join(dir, fmt.Sprintf(".wfc-probe-%d", os.Getpid()))
	f, err := os.OpenFile(probe, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("没有 %s 的写权限（需要 root 运行或将 MGM_WEBFILTER_CONF 指向可写路径）: %w", dir, err)
	}
	_ = f.Close()
	_ = os.Remove(probe)
	// 目标文件若已存在且不可写，同样是必然失败
	if fi2, err := os.Stat(path); err == nil && fi2.Mode().Perm()&0o222 == 0 {
		return fmt.Errorf("%s 存在但不可写（权限 %v）", path, fi2.Mode().Perm())
	}
	return nil
}
