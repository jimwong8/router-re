package cfgsync

import (
	"errors"
	"sort"
	"strings"
)

// Plan 描述恢复时要做什么，不直接执行（安全：先给人看）。
type Plan struct {
	Sections   []string `json:"sections"` // 将被替换的
	Changes    []Change `json:"changes"`
	Dangerous  []string `json:"dangerous"` // 危险项说明
	ChecksumOK bool     `json:"checksum_ok"`
}

// BuildRestorePlan 对比当前配置与待恢复的 incoming 配置，生成恢复方案。
//
// 危险 section 识别：含 iface / interface / network / vlan / firewall / nat / route
// 的 section 会各加一句中文说明——这类配置恢复后如果与本机物理接口/拓扑不符，
// 很可能导致网络不可达从而失联。
func BuildRestorePlan(current, incoming Bundle) (Plan, error) {
	if err := ValidateBundle(incoming); err != nil {
		return Plan{}, err
	}
	changes := Diff(current, incoming)

	// 被替换的 section：incoming 里存在的所有 section。
	var sections []string
	for s := range incoming.Sections {
		sections = append(sections, s)
	}
	sort.Strings(sections)

	var dangerous []string
	lower := strings.ToLower
	for _, s := range sections {
		ls := lower(s)
		switch {
		case strings.Contains(ls, "iface"), strings.Contains(ls, "interface"):
			dangerous = append(dangerous, "将替换网络接口配置，恢复后如果与本机物理接口不符会导致失联")
		case strings.Contains(ls, "network"):
			dangerous = append(dangerous, "将替换网络配置，恢复后如果与本局域网/上游不符会断网失联")
		case strings.Contains(ls, "vlan"):
			dangerous = append(dangerous, "将替换 VLAN 配置，恢复后如果与物理交换机端口不符会导致广播域异常")
		case strings.Contains(ls, "firewall"):
			dangerous = append(dangerous, "将替换防火墙配置，恢复后如果规则与本机管理放行不符可能锁死远程管理")
		case strings.Contains(ls, "nat"), strings.Contains(ls, "route"):
			dangerous = append(dangerous, "将替换 NAT/路由配置，恢复后如果网关/转发路径不符会导致外网流量异常")
		}
	}

	return Plan{
		Sections:   sections,
		Changes:    changes,
		Dangerous:  dangerous,
		ChecksumOK: incoming.Verify(),
	}, nil
}

// ValidateApply 校验恢复方案是否可执行。confirm=false 时**必须**返回错误，
// 绝不静默执行——恢复配置是容易锁死设备的破坏性操作，必须先显式确认。
func ValidateApply(plan Plan, confirm bool) error {
	if len(plan.Sections) == 0 {
		return errors.New("restore plan has no sections to apply")
	}
	if !confirm {
		return errors.New("restore requires confirm=true")
	}
	return nil
}
