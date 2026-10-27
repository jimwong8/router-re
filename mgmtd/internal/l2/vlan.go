package l2

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// VLAN describes a VLAN sub-interface.
type VLAN struct {
	Parent  string `json:"parent"`
	VID     int    `json:"vid"`
	Name    string `json:"name"`
	Address string `json:"address,omitempty"`
}

func ValidateVLAN(v VLAN) error {
	if v.Parent == "" {
		return fmt.Errorf("parent interface required")
	}
	if v.VID < 1 || v.VID > 4094 {
		return fmt.Errorf("vid must be in [1,4094], got %d", v.VID)
	}
	if v.Name == "" {
		return fmt.Errorf("name required")
	}
	if strings.ContainsAny(v.Name, " /") {
		return fmt.Errorf("name must not contain space or slash")
	}
	return nil
}

// ApplyVLAN creates/configures a VLAN sub-interface idempotently.
func ApplyVLAN(r execx.Runner, v VLAN) error {
	if err := ValidateVLAN(v); err != nil {
		return err
	}
	// probe existence
	if _, err := r.Run("ip", "link", "show", v.Name); err != nil {
		// not exist → create
		if _, err := r.Run("ip", "link", "add", "link", v.Parent, "name", v.Name, "type", "vlan", "id", fmt.Sprintf("%d", v.VID)); err != nil {
			return err
		}
	}
	if v.Address != "" {
		if _, err := r.Run("ip", "addr", "replace", v.Address, "dev", v.Name); err != nil {
			return err
		}
	}
	if _, err := r.Run("ip", "link", "set", v.Name, "up"); err != nil {
		return err
	}
	return nil
}

// DeleteVLAN removes a VLAN sub-interface.
func DeleteVLAN(r execx.Runner, name string) error {
	if name == "" {
		return fmt.Errorf("name required")
	}
	_, err := r.Run("ip", "link", "del", name)
	return err
}

// ListVLAN returns VLANs present on the system.
func ListVLAN(r execx.Runner) ([]VLAN, error) {
	out, _ := r.Run("ip", "-j", "-d", "link", "show", "type", "vlan")
	out = strings.TrimSpace(out)
	if out == "" || out == "[]" {
		return []VLAN{}, nil
	}
	var raw []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, fmt.Errorf("parse vlan json: %w", err)
	}
	// `ip -j link show` 只给 MAC（address），IPv4 只在 `ip -j addr show` 的
	// addr_info 里。少了这一步回读出来的 Address 恒为空。
	addrMap := ipv4ByIface(r)
	res := []VLAN{}
	for _, m := range raw {
		name, _ := m["ifname"].(string)
		parent, _ := m["link"].(string)
		// VID 在 linkinfo.info_data.id —— 不是顶层 vlan_info。
		// 写错路径会静默得到 vid=0（真实踩到：创建 id 100 成功后回读显示 0）。
		vid := 0
		if li, ok := m["linkinfo"].(map[string]interface{}); ok {
			if idata, ok := li["info_data"].(map[string]interface{}); ok {
				if id, ok := idata["id"].(float64); ok {
					vid = int(id)
				}
			}
		}
		res = append(res, VLAN{Parent: parent, VID: vid, Name: name, Address: addrMap[name]})
	}
	return res, nil
}

// ipv4ByIface 返回 ifname → 第一个 IPv4 地址（含前缀），数据来自 `ip -j addr show`。
func ipv4ByIface(r execx.Runner) map[string]string {
	res := map[string]string{}
	out, err := r.Run("ip", "-j", "addr", "show")
	if err != nil {
		return res
	}
	var links []map[string]interface{}
	if json.Unmarshal([]byte(strings.TrimSpace(out)), &links) != nil {
		return res
	}
	for _, l := range links {
		ifn, _ := l["ifname"].(string)
		if ifn == "" {
			continue
		}
		if v := firstIPv4(l); v != "" {
			res[ifn] = v
		}
	}
	return res
}

// firstIPv4 从 `ip -j` 的 addr_info 里取第一个 inet 地址（含前缀）。
func firstIPv4(m map[string]interface{}) string {
	ai, ok := m["addr_info"].([]interface{})
	if !ok {
		return ""
	}
	for _, a := range ai {
		am, ok := a.(map[string]interface{})
		if !ok {
			continue
		}
		if fam, _ := am["family"].(string); fam != "inet" {
			continue
		}
		local, _ := am["local"].(string)
		if local == "" {
			continue
		}
		if pl, ok := am["prefixlen"].(float64); ok {
			return fmt.Sprintf("%s/%d", local, int(pl))
		}
		return local
	}
	return ""
}
