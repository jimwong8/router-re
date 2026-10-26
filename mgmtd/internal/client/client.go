// Package client 提供终端列表（从 `ip neigh show` 解析 IP/MAC/状态）与 DHCP 租约合并功能。
package client

import (
	"fmt"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/dhcp"
	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// Client 描述一个检测到的终端（来自网络邻居表），可与 DHCP 租约合并补全主机名。
type Client struct {
	IP        string `json:"ip"`
	MAC       string `json:"mac"`
	Hostname  string `json:"hostname"`
	State     string `json:"state"`
	LastSeen  int64  `json:"last_seen"`
}

// List 执行 "ip neigh show" 并解析输出，返回 Client 列表。
// 每条输出格式为 "<ip> dev <if> lladdr <mac> <STATE>"；跳过无 lladdr 的行。
func List(r execx.Runner) ([]Client, error) {
	out, err := r.Run("ip", "neigh", "show")
	if err != nil {
		return nil, fmt.Errorf("ip neigh show: %w", err)
	}
	var clients []Client
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// 解析格式：10.0.0.2 dev eth0 lladdr aa:bb:cc:dd:ee:ff REACHABLE
		// 先找 lladdr 关键字
		lladdrIdx := strings.Index(line, " lladdr ")
		if lladdrIdx < 0 {
			continue
		}
		before := strings.TrimSpace(line[:lladdrIdx])
		// 取第一列（IP）和第二列中的 MAC
		mac := ""
		fields := strings.Fields(line[lladdrIdx:])
		for i, f := range fields {
			if f == "lladdr" && i+1 < len(fields) {
				mac = fields[i+1]
				break
			}
		}
		if mac == "" {
			continue
		}
		// 取第一列的 IP（before 的第一个 token）
		beforeFields := strings.Fields(before)
		if len(beforeFields) == 0 {
			continue
		}
		ip := beforeFields[0]

		// STATE 是最后一个字段（如 REACHABLE、STALE、PERMANENT 等）；若没有则默认 REACHABLE
		state := "REACHABLE"
		afterMAC := line[lladdrIdx+len(" lladdr "):]
		afterFields := strings.Split(afterMAC, " ")
		if len(afterFields) > 1 {
			afterMACFields := strings.Fields(afterMAC)
			if len(afterMACFields) > 1 {
				state = afterMACFields[len(afterMACFields)-1]
			}
		}

		clients = append(clients, Client{
			IP: ip,
			MAC: mac,
			Hostname: "",
			State: state,
			LastSeen: 0,
		})
	}
	return clients, nil
}

// Merge 用 DHCP 租约信息（IP 匹配）补全 Client 的 Hostname。
// 对每个 Client，查找与其 IP 匹配的租约，取租约的 Hostname 补入 Client.Hostname。
func Merge(neigh []Client, leases []dhcp.Lease) []Client {
	merged := make([]Client, len(neigh))
	copy(merged, neigh)
	leaseMap := make(map[string]string)
	for _, l := range leases {
		if l.Hostname != "" {
			leaseMap[l.IP] = l.Hostname
		}
	}
	for i := range merged {
		if hn, ok := leaseMap[merged[i].IP]; ok {
			merged[i].Hostname = hn
		}
	}
	return merged
}