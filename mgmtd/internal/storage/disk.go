package storage

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

// 磁盘与分区管理。
//
// 这一层只做"看得见 + 能挂载"的部分，破坏性操作（分区、格式化）单独走
// Plan 模式：先返回将要执行的命令，调用方确认后才真正执行。这不是出于
// 保守，而是因为路由器上唯一的系统盘一旦被格式化，设备就在现场变砖，
// 没有本地控制台的人只能返厂。
//
// 与 iKuai 的对应关系：接口是 system/disks；用户盘最多 8 个分区
// （DISK_USER_MAX_PARTITION_NUM=8），单个分区最小 1 GiB（PART_MIN_SIZE）。

const (
	// MaxUserPartitions 与 iKuai 的 DISK_USER_MAX_PARTITION_NUM 一致。
	MaxUserPartitions = 8
	// MinPartitionBytes 与 iKuai 的 PART_MIN_SIZE 一致：1 GiB。
	MinPartitionBytes = 1 << 30
	// MountRoot 是用户盘挂载点前缀（iKuai 用 /etc/disk/<UUID>）。
	MountRoot = "/etc/disk"
)

// Partition 是一个块设备分区。
type Partition struct {
	Name       string `json:"name"` // sda1
	Path       string `json:"path"` // /dev/sda1
	SizeBytes  uint64 `json:"size_bytes"`
	FSType     string `json:"fstype"` // ext4 / xfs / 空（未格式化）
	Label      string `json:"label"`
	UUID       string `json:"uuid"`
	MountPoint string `json:"mount_point"` // 已挂载则为挂载点
	IsSystem   bool   `json:"is_system"`   // 系统盘分区，禁止格式化
	IsMounted  bool   `json:"is_mounted"`
}

// Disk 是一块物理磁盘。
type Disk struct {
	Name       string      `json:"name"` // sda
	Path       string      `json:"path"` // /dev/sda
	SizeBytes  uint64      `json:"size_bytes"`
	Model      string      `json:"model"`
	Removable  bool        `json:"removable"`
	ReadOnly   bool        `json:"read_only"`
	IsSystem   bool        `json:"is_system"` // 承载 / 的磁盘
	Partitions []Partition `json:"partitions"`
	// TotalUsedBytes / TotalAvailBytes 只统计已挂载分区
	MountedCount int    `json:"mounted_count"`
	UsedBytes    uint64 `json:"used_bytes"`
	AvailBytes   uint64 `json:"avail_bytes"`
}

// systemMounts 是"绝不能格式化"的挂载点。
var systemMounts = map[string]bool{
	"/": true, "/boot": true, "/boot/efi": true, "/usr": true, "/var": true, "/opt": true,
}

// ListDisks 枚举块设备与其分区、文件系统、挂载状态。
// 只读：不执行任何会改盘的命令（不调用 blkid 的探测写入，不触发 udev）。
func ListDisks() ([]Disk, error) {
	entries, err := os.ReadDir("/sys/block")
	if err != nil {
		return nil, fmt.Errorf("读取 /sys/block 失败: %w", err)
	}
	devInfo := readBlkid()
	mounts := readMounts()

	systemDevs := systemDevices(mounts)

	var out []Disk
	for _, e := range entries {
		name := e.Name()
		// loop/ram/zram/dm- 是虚拟设备，不作为"磁盘"展示；
		// 但 nvme0n1 / mmcblk0 / sdX 都要
		if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") ||
			strings.HasPrefix(name, "zram") || strings.HasPrefix(name, "dm-") {
			continue
		}
		base := "/sys/block/" + name
		d := Disk{
			Name:      name,
			Path:      "/dev/" + name,
			SizeBytes: readSysUint(base+"/size") * 512,
			Removable: readSysUint(base+"/removable") == 1,
			ReadOnly:  readSysUint(base+"/ro") == 1,
			Model:     strings.TrimSpace(readSysString(base + "/device/model")),
		}
		if d.Model == "" {
			d.Model = strings.TrimSpace(readSysString(base + "/device/name"))
		}

		// 分区
		subs, _ := os.ReadDir(base)
		var parts []Partition
		for _, s := range subs {
			pn := s.Name()
			if !strings.HasPrefix(pn, name) || pn == name {
				continue
			}
			if _, err := os.Stat("/sys/block/" + name + "/" + pn + "/partition"); err != nil {
				continue // 不是分区
			}
			p := Partition{
				Name:      pn,
				Path:      "/dev/" + pn,
				SizeBytes: readSysUint("/sys/block/"+name+"/"+pn+"/size") * 512,
			}
			if info, ok := devInfo["/dev/"+pn]; ok {
				p.FSType, p.UUID, p.Label = info.fsType, info.uuid, info.label
			}
			if mp, ok := mounts["/dev/"+pn]; ok {
				p.MountPoint = mp
				p.IsMounted = true
				var st syscall.Statfs_t
				if err := syscall.Statfs(mp, &st); err == nil {
					total := st.Blocks * uint64(st.Bsize)
					avail := st.Bavail * uint64(st.Bsize)
					d.UsedBytes += total - avail
					d.AvailBytes += avail
					d.MountedCount++
				}
			}
			if systemDevs["/dev/"+pn] || systemDevs["/dev/"+name] {
				p.IsSystem = true
			}
			parts = append(parts, p)
		}
		sort.Slice(parts, func(i, j int) bool { return parts[i].Name < parts[j].Name })
		d.Partitions = parts
		if systemDevs["/dev/"+name] {
			d.IsSystem = true
		}
		for _, p := range parts {
			if p.IsSystem {
				d.IsSystem = true
			}
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

type blkidInfo struct{ fsType, uuid, label string }

// readBlkid 读文件系统信息。
//
// 未挂载的分区在内核里只体现为块设备，文件系统类型要靠 blkid 的缓存。
// 这里刻意不带 -p / -g 参数——那些会真的去读盘探测，在坏盘上可能卡住
// 几十秒，而路由器管理页面不能因为一块盘有问题就打不开。
func readBlkid() map[string]blkidInfo {
	if out := readBlkidTab(); len(out) > 0 {
		return out
	}
	return runBlkid()
}

func readBlkidTab() map[string]blkidInfo {
	out := map[string]blkidInfo{}
	data, err := os.ReadFile("/run/blkid/blkid.tab")
	if err != nil {
		data, err = os.ReadFile("/etc/blkid.tab")
		if err != nil {
			return out
		}
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		dev := line
		if i := strings.Index(line, " "); i > 0 {
			dev = line[:i]
		}
		dev = strings.TrimSuffix(dev, ":")
		info := blkidInfo{
			uuid:   extractAttr(line, "UUID"),
			fsType: extractAttr(line, "TYPE"),
			label:  extractAttr(line, "LABEL"),
		}
		if info.uuid != "" || info.fsType != "" {
			out[dev] = info
		}
	}
	return out
}

// runBlkid 调 blkid（不带参数即只读缓存），解析 /dev/sda1: UUID="..." TYPE="ext4"
func runBlkid() map[string]blkidInfo {
	out := map[string]blkidInfo{}
	cmd := exec.Command("blkid")
	raw, err := cmd.Output()
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if !strings.HasPrefix(line, "/dev/") {
			continue
		}
		dev := line
		if i := strings.Index(line, ":"); i > 0 {
			dev = line[:i]
		}
		info := blkidInfo{
			uuid:   extractAttr(line, "UUID"),
			fsType: extractAttr(line, "TYPE"),
			label:  extractAttr(line, "LABEL"),
		}
		out[dev] = info
	}
	return out
}

var attrRe = regexp.MustCompile(`(\w+)="([^"]*)"`)

func extractAttr(line, key string) string {
	for _, m := range attrRe.FindAllStringSubmatch(line, -1) {
		if m[1] == key {
			return m[2]
		}
	}
	return ""
}

// readMounts 返回 设备 → 挂载点。只取 /dev/ 开头的设备。
func readMounts() map[string]string {
	out := map[string]string{}
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fs := strings.Fields(sc.Text())
		if len(fs) < 2 || !strings.HasPrefix(fs[0], "/dev/") {
			continue
		}
		mp := strings.ReplaceAll(fs[1], `\040`, " ")
		out[fs[0]] = mp
	}
	return out
}

// systemDevices 找出承载系统关键挂载点的设备（含其父盘）。
func systemDevices(mounts map[string]string) map[string]bool {
	out := map[string]bool{}
	for dev, mp := range mounts {
		if !systemMounts[mp] {
			continue
		}
		out[dev] = true
		// /dev/sda1 → /dev/sda
		if parent := parentDevice(dev); parent != "" {
			out[parent] = true
		}
	}
	return out
}

// parentDevice 把 /dev/sda1 / /dev/nvme0n1p1 还原成父盘路径。
func parentDevice(dev string) string {
	base := filepath.Base(dev)
	if strings.HasPrefix(base, "nvme") || strings.HasPrefix(base, "mmcblk") {
		if i := strings.LastIndex(base, "p"); i > 0 {
			if _, err := strconv.Atoi(base[i+1:]); err == nil {
				return "/dev/" + base[:i]
			}
		}
		return ""
	}
	// sdX1 / vdX1 / hdX1
	i := len(base)
	for i > 0 && base[i-1] >= '0' && base[i-1] <= '9' {
		i--
	}
	if i == len(base) || i == 0 {
		return ""
	}
	return "/dev/" + base[:i]
}

func readSysUint(path string) uint64 {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	v, _ := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
	return v
}

func readSysString(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}
