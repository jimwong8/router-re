// appidtool 构建与查询应用特征库。
//
// 用途：
//
//	appidtool build  -src data/ -out /var/lib/router-re/appid.json
//	appidtool lookup -file /var/lib/router-re/appid.json www.bilibili.com www.qq.com
//	appidtool stats  -file /var/lib/router-re/appid.json
//
// build 的 -src 指向 v2ray domain-list 风格的数据目录（一个文件一组，
// 文件名即应用名），也可以直接指向单个文件。
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jimwong/router-re/mgmtd/internal/appid"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "build":
		os.Exit(cmdBuild(os.Args[2:]))
	case "lookup":
		os.Exit(cmdLookup(os.Args[2:]))
	case "stats":
		os.Exit(cmdStats(os.Args[2:]))
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `用法:
  appidtool build  -src <目录或文件> -out <快照文件> [-version <标记>]
  appidtool lookup -file <快照文件> <域名...>
  appidtool stats  -file <快照文件>
`)
	os.Exit(2)
}

func cmdBuild(args []string) int {
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	src := fs.String("src", "", "domain-list 数据目录或单文件")
	out := fs.String("out", "", "输出快照路径")
	version := fs.String("version", time.Now().Format("20060102"), "版本标记")
	fs.Parse(args)

	if *src == "" || *out == "" {
		usage()
	}

	groups := map[string][]byte{}
	meta := map[string]appid.App{}

	info, err := os.Stat(*src)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取来源失败: %v\n", err)
		return 1
	}
	if info.IsDir() {
		entries, err := os.ReadDir(*src)
		if err != nil {
			fmt.Fprintf(os.Stderr, "列目录失败: %v\n", err)
			return 1
		}
		for _, e := range entries {
			if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			b, err := os.ReadFile(filepath.Join(*src, e.Name()))
			if err != nil {
				fmt.Fprintf(os.Stderr, "跳过 %s: %v\n", e.Name(), err)
				continue
			}
			name := e.Name()
			groups[name] = b
			meta[name] = appid.App{ID: name, Name: name, Category: appid.CategoryForGroup(name)}
		}
	} else {
		b, err := os.ReadFile(*src)
		if err != nil {
			fmt.Fprintf(os.Stderr, "读取失败: %v\n", err)
			return 1
		}
		base := strings.TrimSuffix(filepath.Base(*src), filepath.Ext(*src))
		groups[base] = b
		meta[base] = appid.App{ID: base, Name: base, Category: appid.CategoryForGroup(base)}
	}

	start := time.Now()
	lib := appid.New()
	rej, err := lib.ImportDomainList(groups, meta)
	if err != nil {
		fmt.Fprintf(os.Stderr, "导入失败: %v\n", err)
		return 1
	}
	lib.Version = *version
	lib.UpdatedAt = time.Now()
	lib.Source = *src

	snap, err := lib.Snapshot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "序列化失败: %v\n", err)
		return 1
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "创建目录失败: %v\n", err)
		return 1
	}
	tmp := *out + ".tmp"
	if err := os.WriteFile(tmp, snap, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "写文件失败: %v\n", err)
		return 1
	}
	if err := os.Rename(tmp, *out); err != nil {
		os.Remove(tmp)
		fmt.Fprintf(os.Stderr, "落盘失败: %v\n", err)
		return 1
	}

	s := lib.Stats()
	fmt.Printf("已生成 %s\n", *out)
	fmt.Printf("  应用 %d 个，域名 %d 条，耗时 %s，体积 %d KB\n",
		s.Apps, s.Domains, time.Since(start).Round(time.Millisecond), len(snap)/1024)
	if rej.Regexp+rej.Keyword+rej.Invalid+rej.Missing > 0 {
		fmt.Printf("  跳过：正则 %d、关键字 %d、非法 %d、缺失引用 %d\n",
			rej.Regexp, rej.Keyword, rej.Invalid, rej.Missing)
	}
	printCategories(s)
	return 0
}

func printCategories(s appid.Stats) {
	type kv struct {
		k appid.Category
		v int
	}
	list := make([]kv, 0, len(s.ByCategory))
	for k, v := range s.ByCategory {
		list = append(list, kv{k, v})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].v != list[j].v {
			return list[i].v > list[j].v
		}
		return string(list[i].k) < string(list[j].k)
	})
	parts := make([]string, 0, len(list))
	for _, x := range list {
		parts = append(parts, fmt.Sprintf("%s %d", x.k, x.v))
	}
	fmt.Printf("  分类分布：%s\n", strings.Join(parts, "，"))
}

func loadLib(path string) (*appid.Library, int) {
	b, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取快照失败: %v\n", err)
		return nil, 1
	}
	lib, err := appid.LoadSnapshot(b)
	if err != nil {
		fmt.Fprintf(os.Stderr, "解析快照失败: %v\n", err)
		return nil, 1
	}
	return lib, 0
}

func cmdLookup(args []string) int {
	fs := flag.NewFlagSet("lookup", flag.ExitOnError)
	file := fs.String("file", "", "快照文件")
	fs.Parse(args)
	hosts := fs.Args()
	if *file == "" || len(hosts) == 0 {
		usage()
	}
	lib, rc := loadLib(*file)
	if rc != 0 {
		return rc
	}
	hit := 0
	for _, h := range hosts {
		if a, ok := lib.Lookup(h); ok {
			hit++
			fmt.Printf("%-32s -> %s（%s）\n", h, a.Name, a.Category)
		} else {
			fmt.Printf("%-32s -> 未识别\n", h)
		}
	}
	fmt.Printf("命中 %d/%d\n", hit, len(hosts))
	return 0
}

func cmdStats(args []string) int {
	fs := flag.NewFlagSet("stats", flag.ExitOnError)
	file := fs.String("file", "", "快照文件")
	fs.Parse(args)
	if *file == "" {
		usage()
	}
	lib, rc := loadLib(*file)
	if rc != 0 {
		return rc
	}
	s := lib.Stats()
	fmt.Printf("版本 %s（%s 生成，来源 %s）\n", s.Version, s.UpdatedAt.Format("2006-01-02 15:04"), s.Source)
	fmt.Printf("应用 %d 个，域名 %d 条\n", s.Apps, s.Domains)
	printCategories(s)
	fmt.Println("域名最多的应用：")
	for _, a := range s.TopApps {
		fmt.Printf("  %-24s %5d 条  %s\n", a.Name, a.Domains, a.Category)
	}
	return 0
}
