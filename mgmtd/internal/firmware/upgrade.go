package firmware

import (
	"fmt"
	"os"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// Plan 是一次固件升级的计划。
type Plan struct {
	Image      Image    `json:"image"`
	Steps      []string `json:"steps"`
	Risks      []string `json:"risks"`
	KeepConfig bool     `json:"keep_config"`
	BackupPath string   `json:"backup_path"`
}

// BuildPlan 生成升级计划。
//
// 必须包含的风险提示（中文）：
//   - 升级期间网络中断，ssh 远程操作升级后连接会断开
//   - 配置默认保留，但跨大版本（如 23.x->24.x）可能不兼容
//   - 未做配置备份时，risk 明确要求先备份
func BuildPlan(img Image, localBoard string, hasBackup bool) (Plan, error) {
	if err := ValidateImage(img, localBoard); err != nil {
		return Plan{}, err
	}
	p := Plan{
		Image:      img,
		KeepConfig: true,
		BackupPath: "",
		Steps: []string{
			"1. 校验镜像 SHA256 与板型（已在 Inspect/Validate 完成）",
			"2. 生成并核对升级计划",
			"3. 执行 sysupgrade（配置默认保留）",
			"4. 设备重启，等待上线",
		},
	}
	p.Risks = append(p.Risks,
		"升级期间网络会中断；如果通过 ssh 远程操作，升级后连接会断开，需要重新连接",
		"配置默认保留，但跨大版本升级（如 23.x->24.x）可能不兼容",
	)
	if !hasBackup {
		p.Risks = append(p.Risks, "尚未备份配置：请先用 sysupgrade -b 备份配置再升级，防止配置丢失无法恢复")
	}
	return p, nil
}

// BackupConfig 用 sysupgrade -b 生成配置备份，返回备份文件路径和大小。
// 文件不存在或大小为 0 视为失败。
func BackupConfig(r execx.Runner, dest string) (string, int64, error) {
	if _, err := r.Run("sysupgrade", "-b", dest); err != nil {
		return "", 0, fmt.Errorf("sysupgrade -b %s: %w", dest, err)
	}
	fi, err := os.Stat(dest)
	if err != nil {
		return "", 0, fmt.Errorf("backup not found at %s: %w", dest, err)
	}
	if fi.Size() == 0 {
		return "", 0, fmt.Errorf("backup at %s is empty (0 bytes) — backup failed", dest)
	}
	return dest, fi.Size(), nil
}

// Apply 执行升级。必须先 ValidateImage + BuildPlan 成功。
//
// 命令：sysupgrade <image>（保留配置）或 sysupgrade -n <image>（不保留）。
// 只用 -q（安静）不用 -f（强制）。
// 注意：本函数立即返回，因为系统会重启 —— detail 里说明这一点。
func Apply(r execx.Runner, img Image, keepConfig bool) (bool, string) {
	// 必须先 ValidateImage（BuildPlan 也调它，但这里显式保证顺序）。
	if err := ValidateImage(img, img.Board); err != nil {
		return false, err.Error()
	}
	args := []string{"-q"}
	if !keepConfig {
		args = append(args, "-n")
	}
	args = append(args, img.Path)
	if _, err := r.Run("sysupgrade", args...); err != nil {
		return false, fmt.Sprintf("sysupgrade failed: %v", err)
	}
	return true, "升级已提交；系统即将重启，ssh 连接会断开，请稍后重新连接以确认升级结果"
}
