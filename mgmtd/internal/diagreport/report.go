// Package diagreport 收集并渲染路由器诊断报告。
package diagreport

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// Section 是诊断报告的一个章节。
type Section struct {
	Name   string   `json:"name"`
	Title  string   `json:"title"`
	OK     bool     `json:"ok"`
	Items  []string `json:"items"`
	Errors []string `json:"errors"`
}

// Summary 是报告的汇总。
type Summary struct {
	Total  int `json:"total"`
	Failed int `json:"failed"`
}

// Report 是完整诊断报告。
type Report struct {
	GeneratedAt int64     `json:"generated_at"`
	Hostname    string    `json:"hostname"`
	Sections    []Section `json:"sections"`
	Summary     Summary   `json:"summary"`
}

// titleFor maps a command name to a human-readable Chinese title.
func titleFor(name string) string {
	switch name {
	case "interfaces":
		return "网络接口"
	case "routes":
		return "路由表"
	case "neighbors":
		return "邻居表"
	case "nft":
		return "nftables 规则集"
	case "disk":
		return "磁盘使用"
	case "memory":
		return "内存使用"
	case "loadavg":
		return "系统负载"
	case "failed_units":
		return "失败的服务单元"
	default:
		return name
	}
}

// Collect 收集各类诊断信息。
//
// commands 是 name -> 命令参数 的映射，按名称排序执行并收集输出行。
// 任何一条失败都要记在 Section.Errors 里，但不能让整份报告失败——
// 诊断报告的价值恰恰在于"某条命令跑不动"这件事本身。
func Collect(hostname string, now int64, runner execx.Runner, commands map[string][]string) Report {
	r := Report{
		GeneratedAt: now,
		Hostname:    hostname,
		Sections:    []Section{},
	}
	// 稳定顺序：按名称排序
	names := make([]string, 0, len(commands))
	for n := range commands {
		names = append(names, n)
	}
	sort.Strings(names)

	failed := 0
	for _, name := range names {
		args := commands[name]
		sec := Section{
			Name:   name,
			Title:  titleFor(name),
			Items:  []string{},
			Errors: []string{},
		}
		if len(args) == 0 {
			sec.OK = false
			sec.Errors = append(sec.Errors, "empty command")
			failed++
			r.Sections = append(r.Sections, sec)
			continue
		}
		out, err := runner.Run(args[0], args[1:]...)
		if err != nil {
			sec.OK = false
			sec.Errors = append(sec.Errors, err.Error())
			failed++
		} else {
			sec.OK = true
			for _, line := range strings.Split(out, "\n") {
				line = strings.TrimRight(line, "\r")
				if strings.TrimSpace(line) == "" {
					continue
				}
				sec.Items = append(sec.Items, line)
			}
		}
		r.Sections = append(r.Sections, sec)
	}
	r.Summary = Summary{Total: len(r.Sections), Failed: failed}
	return r
}

// DefaultCommands 返回默认要跑的命令清单的 Section 骨架（Name+Title，Items 待填）。
func DefaultCommands() []Section {
	names := []string{"interfaces", "routes", "neighbors", "nft", "disk", "memory", "loadavg", "failed_units"}
	out := make([]Section, 0, len(names))
	for _, n := range names {
		out = append(out, Section{Name: n, Title: titleFor(n), Items: []string{}, Errors: []string{}})
	}
	return out
}

// RenderText 把报告渲染成人类可读的纯文本。
func RenderText(r Report) string {
	var b strings.Builder
	ts := time.Unix(r.GeneratedAt, 0).Format("2006-01-02 15:04:05")
	fmt.Fprintf(&b, "Router-RE 诊断报告  %s\n", ts)
	fmt.Fprintf(&b, "Host: %s\n", r.Hostname)
	fmt.Fprintf(&b, "================================================\n")
	for _, s := range r.Sections {
		mark := "[OK]"
		if !s.OK {
			mark = "[FAIL]"
		}
		fmt.Fprintf(&b, "%s %s (%s)\n", mark, s.Title, s.Name)
		if s.OK {
			for _, it := range s.Items {
				fmt.Fprintf(&b, "  %s\n", it)
			}
		} else {
			for _, e := range s.Errors {
				fmt.Fprintf(&b, "  错误: %s\n", e)
			}
		}
	}
	return b.String()
}
