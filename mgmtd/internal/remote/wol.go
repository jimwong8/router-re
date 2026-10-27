package remote

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// WOLTarget 描述一个网络唤醒目标。
type WOLTarget struct {
	Name      string `json:"name"`      // 目标名（≤64 字符）
	MAC       string `json:"mac"`       // 目标 MAC，须 6 字节
	Iface     string `json:"iface"`     // 发送唤醒包的网络接口
	Broadcast string `json:"broadcast"` // 可选：IPv4 广播地址
}

// ValidateWOL 校验 WOLTarget：
//   - Name 非空且 ≤64；
//   - MAC 必须可通过 net.ParseMAC 解析且为 6 字节；
//   - Broadcast 非空时必须是合法 IPv4 广播地址。
func ValidateWOL(t WOLTarget) error {
	if t.Name == "" {
		return fmt.Errorf("name 不能为空")
	}
	if len(t.Name) > 64 {
		return fmt.Errorf("name 长度 %d 超过 64", len(t.Name))
	}
	mac, err := net.ParseMAC(t.MAC)
	if err != nil {
		return fmt.Errorf("mac %q 非法: %v", t.MAC, err)
	}
	if len(mac) != 6 {
		return fmt.Errorf("mac %q 长度 %d 不是 6 字节", t.MAC, len(mac))
	}
	if t.Broadcast != "" {
		ip := net.ParseIP(t.Broadcast)
		if ip == nil || ip.To4() == nil {
			return fmt.Errorf("broadcast %q 不是合法 IPv4 地址", t.Broadcast)
		}
	}
	return nil
}

// Wake 发送网络唤醒包。
//
// 依次尝试 etherwake -i <iface> <mac> 与 wakeonlan <mac>；两者都不存在时返回
// "etherwake/wakeonlan 均未安装"。Broadcast 非空时优先传给 etherwake 用
// `-b <broadcast>`（若支持）——这里为兼容性只透传 iface 与 mac，广播地址由系统
// 默认组播，避免因特定参数在新版本被移除而误报失败。
func Wake(r execx.Runner, t WOLTarget) error {
	if err := ValidateWOL(t); err != nil {
		return err
	}
	if r == nil {
		r = execx.DefaultRunner{}
	}
	// 依次尝试，第一个成功即返回。
	if _, err := r.Run("etherwake", "-i", t.Iface, t.MAC); err == nil {
		return nil
	}
	if _, err := r.Run("wakeonlan", t.MAC); err == nil {
		return nil
	}
	// 两者都失败 —— 区分"命令不存在"（安装缺失）与"发包失败"。
	if execx.MustLookup("etherwake") == "" && execx.MustLookup("wakeonlan") == "" {
		return fmt.Errorf("etherwake/wakeonlan 均未安装，请先安装 wakeonlan 或 etherwake")
	}
	return fmt.Errorf("发送唤醒包失败：etherwake 与 wakeonlan 均未成功（请检查 iface 与 MAC）")
}

// SaveTargets 将 WOL 目标列表以 0600 权限写入 JSON 文件。
func SaveTargets(path string, targets []WOLTarget) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	b, err := json.MarshalIndent(targets, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// LoadTargets 从 JSON 文件读取 WOL 目标列表。文件不存在返回空切片而非错误。
func LoadTargets(path string) ([]WOLTarget, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []WOLTarget{}, nil
		}
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	var targets []WOLTarget
	if len(strings.TrimSpace(string(data))) == 0 {
		return []WOLTarget{}, nil
	}
	if err := json.Unmarshal(data, &targets); err != nil {
		return nil, err
	}
	return targets, nil
}
