package l2

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// Bridge describes a Linux bridge.
type Bridge struct {
	Name    string   `json:"name"`
	Members []string `json:"members"`
	Address string   `json:"address,omitempty"`
	STP     bool     `json:"stp"`
}

func ValidateBridge(b Bridge) error {
	if b.Name == "" {
		return fmt.Errorf("name required")
	}
	if strings.ContainsAny(b.Name, " /") {
		return fmt.Errorf("name must not contain space or slash")
	}
	if len(b.Members) == 0 || len(b.Members) > 16 {
		return fmt.Errorf("members count must be in [1,16], got %d", len(b.Members))
	}
	for _, m := range b.Members {
		if strings.ContainsAny(m, " ") {
			return fmt.Errorf("member name must not contain space: %q", m)
		}
	}
	return nil
}

// ApplyBridge creates/configures a bridge idempotently.
func ApplyBridge(r execx.Runner, b Bridge) error {
	if err := ValidateBridge(b); err != nil {
		return err
	}
	// probe existence
	if _, err := r.Run("ip", "link", "show", b.Name); err != nil {
		if _, err := r.Run("ip", "link", "add", b.Name, "type", "bridge"); err != nil {
			return err
		}
	}
	if b.STP {
		if _, err := r.Run("ip", "link", "set", b.Name, "type", "bridge", "stp_state", "1"); err != nil {
			return err
		}
	}
	for _, m := range b.Members {
		if _, err := r.Run("ip", "link", "set", m, "master", b.Name); err != nil {
			return err
		}
		if _, err := r.Run("ip", "link", "set", m, "up"); err != nil {
			return err
		}
	}
	if b.Address != "" {
		if _, err := r.Run("ip", "addr", "replace", b.Address, "dev", b.Name); err != nil {
			return err
		}
	}
	if _, err := r.Run("ip", "link", "set", b.Name, "up"); err != nil {
		return err
	}
	return nil
}

// DeleteBridge unbinds members then deletes the bridge.
func DeleteBridge(r execx.Runner, name string, members []string) error {
	if name == "" {
		return fmt.Errorf("name required")
	}
	for _, m := range members {
		_, _ = r.Run("ip", "link", "set", m, "nomaster")
	}
	_, err := r.Run("ip", "link", "del", name)
	return err
}

// ListBridge returns bridges present on the system.
func ListBridge(r execx.Runner) ([]Bridge, error) {
	// 必须带 -d：不带 -d 时 linkinfo 是空的，stp_state 取不到（实测踩坑）。
	out, _ := r.Run("ip", "-j", "-d", "link", "show", "type", "bridge")
	out = strings.TrimSpace(out)
	if out == "" || out == "[]" {
		return []Bridge{}, nil
	}
	var raw []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, fmt.Errorf("parse bridge json: %w", err)
	}

	// 成员从「所有 link 的 master 字段」反查，不要解析 `bridge link show`
	// 的文本 —— 它每行是 "6059: vtest0.100@vtest0: ..."，首个字段是序号，
	// 会得到 members=["6059:"] 这种垃圾。
	members := map[string][]string{}
	if allOut, err := r.Run("ip", "-j", "link", "show"); err == nil {
		var links []map[string]interface{}
		if json.Unmarshal([]byte(strings.TrimSpace(allOut)), &links) == nil {
			for _, l := range links {
				master, _ := l["master"].(string)
				ifn, _ := l["ifname"].(string)
				if master != "" && ifn != "" {
					members[master] = append(members[master], ifn)
				}
			}
		}
	}

	addrMap := ipv4ByIface(r)
	res := []Bridge{}
	for _, m := range raw {
		name, _ := m["ifname"].(string)
		b := Bridge{Name: name, Members: members[name], STP: false}
		if b.Members == nil {
			b.Members = []string{}
		}
		if li, ok := m["linkinfo"].(map[string]interface{}); ok {
			if idata, ok := li["info_data"].(map[string]interface{}); ok {
				if stp, ok := idata["stp_state"].(float64); ok {
					b.STP = stp != 0
				}
			}
		}
		b.Address = addrMap[name]
		res = append(res, b)
	}
	return res, nil
}
