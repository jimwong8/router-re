package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// 挂载、分区、格式化。
//
// 设计原则：所有破坏性操作都先出"计划"（要执行的命令 + 影响面），
// 计划里带着是否触碰系统盘、目标是否已挂载等判断，调用方确认后才执行。
// 路由器上格式错盘是不可逆事故，dry-run 不是可选项。

// Plan 是一次待执行的磁盘操作。
type Plan struct {
	Action  string   `json:"action"` // format / partition / mount / umount
	Device  string   `json:"device"`
	Command []string `json:"command"`
	// Danger 说明这条命令会造成什么后果，前端必须原样展示
	Danger string `json:"danger"`
	// 已确认可执行的标记（由调用方在用户确认后置位）
	Confirmed bool `json:"confirmed"`
}

// errSystemDisk 是"碰了系统盘"的统一错误。
var errSystemDisk = fmt.Errorf("拒绝操作：该设备承载系统挂载点")

// errUnknownSystem 在无法判定系统盘时拒绝破坏性操作。
//
// 典型场景是容器（LXC/容器内 / 是 loop 或 overlay，没有 /dev/xxx 挂载根）：
// 这时所有磁盘的 IsSystem 都是 false，如果照常放行，用户格式化的可能是
// 宿主机透传进来的数据盘。判不出来就等于判成"不安全"。
var errUnknownSystem = fmt.Errorf("拒绝操作：当前环境无法判定哪块盘是系统盘（容器/overlay 根），为避免误格式化为宿主机磁盘，已禁用破坏性操作")

// systemKnown 报告是否能明确识别系统盘。
func systemKnown(disks []Disk) bool {
	for _, d := range disks {
		if d.IsSystem {
			return true
		}
	}
	return false
}

// findDisk 在只读枚举结果里定位设备，防止调用方传入 /dev/ 下的任意路径。
func findDisk(disks []Disk, dev string) (*Disk, *Partition, error) {
	dev = strings.TrimSpace(dev)
	if !strings.HasPrefix(dev, "/dev/") {
		return nil, nil, fmt.Errorf("设备路径必须以 /dev/ 开头")
	}
	if strings.Contains(dev, "..") || strings.ContainsAny(dev, " \t\n") {
		return nil, nil, fmt.Errorf("设备路径非法")
	}
	for i := range disks {
		if disks[i].Path == dev {
			return &disks[i], nil, nil
		}
		for j := range disks[i].Partitions {
			if disks[i].Partitions[j].Path == dev {
				return &disks[i], &disks[i].Partitions[j], nil
			}
		}
	}
	return nil, nil, fmt.Errorf("设备不存在或不可管理：%s", dev)
}

// PlanFormat 生成格式化计划。fsType 只接受 ext4 / xfs / vfat（FAT32）。
func PlanFormat(dev, fsType, label string) (Plan, error) {
	disks, err := ListDisks()
	if err != nil {
		return Plan{}, err
	}
	if !systemKnown(disks) {
		return Plan{}, errUnknownSystem
	}
	disk, part, err := findDisk(disks, dev)
	if err != nil {
		return Plan{}, err
	}
	if part == nil {
		// 整盘格式化：必须有分区表保护意识，直接拒
		return Plan{}, fmt.Errorf("请选择分区而不是整块磁盘（%s 不是分区）", dev)
	}
	if part.IsSystem || disk.IsSystem {
		return Plan{}, errSystemDisk
	}
	if part.IsMounted {
		return Plan{}, fmt.Errorf("分区已挂载到 %s，请先卸载再格式化", part.MountPoint)
	}
	switch fsType {
	case "ext4":
	case "xfs":
	case "vfat":
	default:
		return Plan{}, fmt.Errorf("不支持的文件系统 %q（只接受 ext4 / xfs / vfat）", fsType)
	}
	if len(label) > 16 {
		return Plan{}, fmt.Errorf("卷标最多 16 字节")
	}
	for _, r := range label {
		if r < 0x20 || r == '/' || r == ':' {
			return Plan{}, fmt.Errorf("卷标包含非法字符")
		}
	}

	var cmd []string
	switch fsType {
	case "xfs":
		cmd = []string{"mkfs.xfs", "-f", "-L", label, dev}
		if label == "" {
			cmd = []string{"mkfs.xfs", "-f", dev}
		}
	case "vfat":
		cmd = []string{"mkfs.vfat", "-n", strings.ToUpper(label), dev}
		if label == "" {
			cmd = []string{"mkfs.vfat", dev}
		}
	default:
		cmd = []string{"mkfs.ext4", "-F", "-L", label, dev}
		if label == "" {
			cmd = []string{"mkfs.ext4", "-F", dev}
		}
	}
	danger := fmt.Sprintf("格式化 %s（%s，%s）会清空该分区上的全部数据，且不可恢复",
		dev, humanSize(part.SizeBytes), fsType)
	if part.FSType != "" {
		danger += fmt.Sprintf("；该分区当前是 %s", part.FSType)
	}
	return Plan{Action: "format", Device: dev, Command: cmd, Danger: danger}, nil
}

// PlanPartition 生成新建分区计划。
// 起始/结束单位是 MiB，结束为 0 表示用到剩余全部空间。
func PlanPartition(dev string, startMiB, endMiB int) (Plan, error) {
	disks, err := ListDisks()
	if err != nil {
		return Plan{}, err
	}
	if !systemKnown(disks) {
		return Plan{}, errUnknownSystem
	}
	disk, part, err := findDisk(disks, dev)
	if err != nil {
		return Plan{}, err
	}
	if part != nil {
		return Plan{}, fmt.Errorf("请选择整块磁盘作为分区对象")
	}
	if disk.IsSystem {
		return Plan{}, errSystemDisk
	}
	if len(disk.Partitions) >= MaxUserPartitions {
		return Plan{}, fmt.Errorf("该磁盘已有 %d 个分区，达到上限 %d", len(disk.Partitions), MaxUserPartitions)
	}
	if startMiB < 1 {
		return Plan{}, fmt.Errorf("起始位置必须 >= 1 MiB（前面要给分区表留空间）")
	}
	diskMiB := int(disk.SizeBytes / (1 << 20))
	if startMiB >= diskMiB {
		return Plan{}, fmt.Errorf("起始位置 %d MiB 超过磁盘容量 %d MiB", startMiB, diskMiB)
	}
	if endMiB == 0 {
		endMiB = diskMiB - 1
	}
	if endMiB <= startMiB {
		return Plan{}, fmt.Errorf("结束位置必须大于起始位置")
	}
	if endMiB > diskMiB {
		return Plan{}, fmt.Errorf("结束位置 %d MiB 超过磁盘容量 %d MiB", endMiB, diskMiB)
	}
	if int64(endMiB-startMiB)<<20 < MinPartitionBytes {
		return Plan{}, fmt.Errorf("分区太小：至少 1 GiB")
	}
	// 已挂载分区不能在其所在盘上改分区表
	for _, p := range disk.Partitions {
		if p.IsMounted {
			return Plan{}, fmt.Errorf("分区 %s 正在挂载（%s），请先全部卸载再改分区表", p.Path, p.MountPoint)
		}
	}
	cmd := []string{"parted", "-s", dev, "mkpart", "primary", fmt.Sprintf("%dMiB", startMiB), fmt.Sprintf("%dMiB", endMiB)}
	return Plan{
		Action:  "partition",
		Device:  dev,
		Command: cmd,
		Danger:  fmt.Sprintf("在 %s 上新建分区会改写分区表，现有分区编号可能变化", dev),
	}, nil
}

// MountPointFor 返回按 UUID 生成的挂载点（与 iKuai 的 /etc/disk/<UUID> 一致）。
func MountPointFor(uuid, dev string) string {
	key := uuid
	if key == "" {
		key = strings.TrimPrefix(dev, "/dev/")
	}
	key = strings.ReplaceAll(key, "/", "_")
	return filepath.Join(MountRoot, key)
}

// Mount 挂载分区。挂载点按 UUID 生成，避免设备名变化后挂错。
func Mount(dev string, readOnly bool) (string, error) {
	disks, err := ListDisks()
	if err != nil {
		return "", err
	}
	_, part, err := findDisk(disks, dev)
	if err != nil {
		return "", err
	}
	if part == nil {
		return "", fmt.Errorf("只能挂载分区")
	}
	if part.IsMounted {
		return part.MountPoint, fmt.Errorf("分区已挂载到 %s", part.MountPoint)
	}
	if part.FSType == "" {
		return "", fmt.Errorf("分区没有可识别的文件系统，请先格式化")
	}
	mp := MountPointFor(part.UUID, dev)
	if err := os.MkdirAll(mp, 0o755); err != nil {
		return "", fmt.Errorf("创建挂载点失败: %w", err)
	}
	opts := "noatime"
	if readOnly {
		opts = "ro,noatime"
	}
	runner := execx.DefaultRunner{}
	if out, err := runner.Run("mount", "-o", opts, dev, mp); err != nil {
		return "", fmt.Errorf("挂载失败: %s", strings.TrimSpace(out))
	}
	return mp, nil
}

// Unmount 卸载。busy 时先尝试懒卸载，仍失败则如实报错（不强制 -f，
// 那会丢正在写的数据）。
func Unmount(mountPoint string) error {
	if !strings.HasPrefix(filepath.Clean(mountPoint), MountRoot) {
		return fmt.Errorf("只能卸载 %s 下的挂载点", MountRoot)
	}
	runner := execx.DefaultRunner{}
	if out, err := runner.Run("umount", mountPoint); err != nil {
		if strings.Contains(strings.ToLower(out), "busy") {
			return fmt.Errorf("挂载点正被占用（可能有进程在读写），请先停止使用：%s", strings.TrimSpace(out))
		}
		return fmt.Errorf("卸载失败: %s", strings.TrimSpace(out))
	}
	return nil
}

// Execute 执行计划。没有 Confirmed 标记一律拒绝——避免任何调用路径
// "顺手"把格式化跑掉。
func Execute(p Plan, runner execx.Runner) (string, error) {
	if !p.Confirmed {
		return "", fmt.Errorf("计划未确认，拒绝执行")
	}
	if len(p.Command) == 0 {
		return "", fmt.Errorf("空命令")
	}
	switch p.Action {
	case "format", "partition":
	default:
		return "", fmt.Errorf("不支持的破坏性操作: %s", p.Action)
	}
	out, err := runner.Run(p.Command[0], p.Command[1:]...)
	if err != nil {
		return out, fmt.Errorf("%s 失败: %s", p.Command[0], strings.TrimSpace(out))
	}
	return out, nil
}

// humanSize 把字节数转成便于阅读的字符串（界面提示用）。
func humanSize(b uint64) string {
	const unit = 1024
	if b < unit {
		return strconv.FormatUint(b, 10) + " B"
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
