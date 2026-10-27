package procmon

import (
	"fmt"
	"regexp"
	"strings"
)

// Unit 描述一个 systemd 单元的状态。
type Unit struct {
	Name        string `json:"name"`
	Load        string `json:"load"` // loaded / not-found / masked
	Active      string `json:"active"` // active / inactive / failed
	Sub         string `json:"sub"`   // running / dead / exited
	Description string `json:"description"`
}

// AllowedAction 是允许执行的 systemctl 动作白名单。
type AllowedAction string

const (
	ActionStart   AllowedAction = "start"
	ActionStop    AllowedAction = "stop"
	ActionRestart AllowedAction = "restart"
	ActionReload  AllowedAction = "reload"
)

// validUnitSuffixes 是允许的单元文件后缀。
var validUnitSuffixes = []string{".service", ".socket", ".timer", ".target"}

// unitNameRe 仅允许安全字符（无空格/分号/管道/$()/反引号）。
var unitNameRe = regexp.MustCompile(`^[a-zA-Z0-9@._-]+$`)

// ParseUnits 解析 `systemctl list-units --all --no-pager --plain --no-legend` 输出。
//
// 典型行：
//   ssh.service loaded active running OpenSSH server daemon
//   nginx.service loaded inactive dead A high performance web server
func ParseUnits(out string) []Unit {
	var units []Unit
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// systemctl --plain 输出以 name load active sub description... 组成
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		u := Unit{
			Name:        fields[0],
			Load:        fields[1],
			Active:      fields[2],
			Sub:         fields[3],
			Description: strings.Join(fields[4:], " "),
		}
		units = append(units, u)
	}
	return units
}

// ValidateUnitName 校验 unit 名。
//
// 安全边界：只允许 [a-zA-Z0-9@._-]，必须以 .service/.socket/.timer/.target 结尾。
// **必须拒绝含空格、分号、管道、$()、反引号的输入** —— 否则 systemctl 调用会变成
// 命令注入（攻击者传 "sshd; rm -rf /" 时，如果拼成 shell 字符串会执行 rm）。
// 这里直接拒绝非白名单字符，从源头阻断注入。
func ValidateUnitName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("unit name required")
	}
	if !unitNameRe.MatchString(name) {
		return fmt.Errorf("unit name %q contains invalid characters (only [a-zA-Z0-9@._-] allowed; no spaces, semicolons, pipes, $(), backticks)", name)
	}
	ok := false
	for _, suf := range validUnitSuffixes {
		if strings.HasSuffix(name, suf) {
			ok = true
			break
		}
	}
	if !ok {
		return fmt.Errorf("unit name %q must end with one of .service/.socket/.timer/.target", name)
	}
	return nil
}

// ValidateAction 只允许白名单里的四个动作。**绝不能拼接任意字符串给 systemctl**。
func ValidateAction(a AllowedAction) error {
	switch a {
	case ActionStart, ActionStop, ActionRestart, ActionReload:
		return nil
	default:
		return fmt.Errorf("action %q not allowed (only start/stop/restart/reload)", a)
	}
}

// RenderCmd 生成 systemctl 命令参数（返回 slice，绝不拼成 shell 字符串）。
// 调用方应使用 execx.Runner.Run("systemctl", args...) 的 argv 形式。
func RenderCmd(action AllowedAction, unit string) ([]string, error) {
	if err := ValidateAction(action); err != nil {
		return nil, err
	}
	if err := ValidateUnitName(unit); err != nil {
		return nil, err
	}
	return []string{string(action), unit}, nil
}

// ProtectedUnits 返回绝对不能被 stop 的关键服务清单。
// **停掉这些会立刻失去远程管理能力**，Stop 前必须检查。
func ProtectedUnits() map[string]bool {
	return map[string]bool{
		"sshd.service":             true,
		"sshd@.service":            true,
		"systemd-networkd.service": true,
		"networking.service":       true,
		"netifd.service":           true,
		"mgmtd.service":            true,
	}
}

// IsProtected 判断 unit 是否为关键服务。
func IsProtected(unit string) bool {
	return ProtectedUnits()[unit]
}