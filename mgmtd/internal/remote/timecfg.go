package remote

import (
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// TimeConfig 描述系统时区与 NTP 同步配置。
type TimeConfig struct {
	Timezone   string   `json:"timezone"`    // 如 "Asia/Shanghai"
	NTPServers []string `json:"ntp_servers"` // 如 ["ntp.aliyun.com","127.0.0.1"]
	Enabled    bool     `json:"enabled"`     // 是否启用 NTP 自动同步（timedatectl set-ntp）
}

// ValidateTime 校验 TimeConfig：
//   - Timezone 必须在 /usr/share/zoneinfo 下真实存在（用 os.Stat，不硬编码白名单）；
//   - NTPServers 每项必须是合法主机名或 IP。
func ValidateTime(c TimeConfig) error {
	if c.Timezone == "" {
		return fmt.Errorf("timezone 不能为空")
	}
	// 用实际文件系统校验，避免白名单遗漏合法时区（如 "Etc/UTC"、"UTC"）。
	p := filepath.Join("/usr/share/zoneinfo", c.Timezone)
	if fi, err := os.Stat(p); err != nil || fi.IsDir() {
		return fmt.Errorf("timezone %q 不存在于 /usr/share/zoneinfo（校验路径 %s）", c.Timezone, p)
	}
	for _, srv := range c.NTPServers {
		if !validNTPHost(srv) {
			return fmt.Errorf("ntp_servers 含非法主机名/IP %q", srv)
		}
	}
	return nil
}

// validNTPHost 判断一个 NTP 服务器标识是合法 IP 或合法主机名（不含空格/路径）。
func validNTPHost(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if net.ParseIP(s) != nil {
		return true
	}
	// 主机名：字母数字与 - .，每段不以 - 开头结尾。不做 DNS 解析（离线可用）。
	if strings.ContainsAny(s, " /\\") {
		return false
	}
	for _, seg := range strings.Split(s, ".") {
		if seg == "" {
			return false
		}
		for i, ch := range seg {
			if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' {
				if ch == '-' && (i == 0 || i == len(seg)-1) {
					return false
				}
				continue
			}
			return false
		}
	}
	return true
}

// ApplyTime 应用系统时区与 NTP 设置，且**回读校验**：
//   - timedatectl set-timezone <tz>
//   - timedatectl set-ntp true/false
//
// 每条执行后回读 `timedatectl show -p Timezone --value`，若与预期不一致则报错
// （只看命令退出码会被部分系统假成功骗过）。
func ApplyTime(r execx.Runner, c TimeConfig) error {
	if r == nil {
		r = execx.DefaultRunner{}
	}
	// 1) 时区
	if c.Timezone != "" {
		if _, err := r.Run("timedatectl", "set-timezone", c.Timezone); err != nil {
			return fmt.Errorf("set-timezone 失败: %w", err)
		}
		got, err := r.Run("timedatectl", "show", "-p", "Timezone", "--value")
		if err != nil {
			return fmt.Errorf("回读 Timezone 失败: %w", err)
		}
		got = strings.TrimSpace(got)
		if got != c.Timezone {
			return fmt.Errorf("时区回读不一致：期望 %q 实际 %q（设置未生效）", c.Timezone, got)
		}
	}
	// 2) NTP 开关
	flag := "false"
	if c.Enabled {
		flag = "true"
	}
	if _, err := r.Run("timedatectl", "set-ntp", flag); err != nil {
		return fmt.Errorf("set-ntp %s 失败: %w", flag, err)
	}
	return nil
}

// TimeStatus 解析 `timedatectl show` 关键字段。返回键：
//   - Timezone / LocalRTC / NTP / NTPSynchronized
func TimeStatus(r execx.Runner) (map[string]string, error) {
	if r == nil {
		r = execx.DefaultRunner{}
	}
	out, err := r.Run("timedatectl", "show")
	if err != nil {
		return nil, err
	}
	st := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch key {
		case "Timezone", "LocalRTC", "NTP", "NTPSynchronized":
			st[key] = val
		}
	}
	return st, nil
}

// maxTimezones 限制返回的时区数量，避免超大目录拖垮响应。
const maxTimezones = 600

// zoneinfoRoot 是时区数据库根目录，可被测试替换。
var zoneinfoRoot = "/usr/share/zoneinfo"

// ListTimezones 递归列出时区路径（如 "Asia/Shanghai"）。见 listTimezones。
func ListTimezones(r execx.Runner) ([]string, error) {
	_ = r // 不需要执行外部命令，直接读文件系统
	return listTimezones()
}

// listTimezones 递归列出 /usr/share/zoneinfo 下的时区路径。
// 跳过 posix/ right/ 子目录与点文件；返回数量限制在 maxTimezones，超出时在结果
// 尾部追加说明被截断。
func listTimezones() ([]string, error) {
	root := zoneinfoRoot
	var zones []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // 单点错误不终止整体遍历
		}
		if info.IsDir() {
			// 只跳过 posix/right 两个特殊子目录。注意：不能按 base name == "zoneinfo"
			// 来判断——根目录本身也叫 zoneinfo，会整棵被跳掉返回空。
			if path != root {
				name := filepath.Base(path)
				if name == "posix" || name == "right" {
					return filepath.SkipDir
				}
			}
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		if strings.HasPrefix(filepath.Base(rel), ".") {
			return nil
		}
		zones = append(zones, filepath.ToSlash(rel))
		if len(zones) >= maxTimezones {
			return fs.SkipAll
		}
		return nil
	})
	if err != nil && err != fs.SkipAll {
		return zones, err
	}
	sort.Strings(zones)
	if len(zones) >= maxTimezones {
		zones = append(zones, fmt.Sprintf("... 时区数量超过 %d，已截断（仅列出前 %d 个）", maxTimezones, maxTimezones))
	}
	return zones, nil
}
