package server

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/dhcp"
	"github.com/jimwong/router-re/mgmtd/internal/online"
)

// ---------- 终端列表 ----------

// handleListTerminals GET /api/v1/terminals
// 只读聚合：ARP + DHCP 租约 + conntrack 会话，按 IP 去重合并。
func (s *Server) handleListTerminals(w http.ResponseWriter, r *http.Request) {
	arpData, err := os.ReadFile("/proc/net/arp")
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, 200, map[string]interface{}{
				"count":     0,
				"terminals": []map[string]interface{}{},
				"note":      "/proc/net/arp 不存在，可能非 Linux 或内核未开启 ARP 表",
			})
			return
		}
		writeJSON(w, 200, map[string]interface{}{
			"count":     0,
			"terminals": []map[string]interface{}{},
			"note":      fmt.Sprintf("读取 /proc/net/arp 失败: %v", err),
		})
		return
	}

	byIP := map[string]map[string]interface{}{}

	// ARP
	for _, line := range strings.Split(string(arpData), "\n")[1:] {
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		ip := fields[0]
		if net.ParseIP(ip) == nil {
			continue
		}
		mac := strings.ToLower(fields[3])
		iface := fields[5]
		if _, ok := byIP[ip]; !ok {
			byIP[ip] = map[string]interface{}{
				"ip": ip, "mac": mac, "hostname": "", "iface": iface, "source": "arp", "online": true,
			}
		}
	}

	// DHCP 租约 — hostname 优先取 dhcp
	leaseApplier := dhcp.NewApplier()
	if leases, lerr := leaseApplier.Leases(); lerr == nil {
		for _, l := range leases {
			if net.ParseIP(l.IP) == nil {
				continue
			}
			m, ok := byIP[l.IP]
			if !ok {
				m = map[string]interface{}{"ip": l.IP, "mac": "", "hostname": "", "iface": "", "source": "dhcp", "online": true}
				byIP[l.IP] = m
			}
			// dhcp 的 hostname 优先
			if l.Hostname != "" {
				m["hostname"] = l.Hostname
			}
			if l.MAC != "" {
				m["mac"] = strings.ToLower(l.MAC)
			}
			if !strings.Contains(m["source"].(string), "dhcp") {
				m["source"] = m["source"].(string) + ",dhcp"
			}
		}
	}

	// conntrack 会话
	if ctData, cerr := os.ReadFile("/proc/net/nf_conntrack"); cerr == nil {
		sessions := online.ParseConntrackSessions(string(ctData))
		for ip, sess := range sessions {
			m, ok := byIP[ip]
			if !ok {
				m = map[string]interface{}{"ip": ip, "mac": "", "hostname": "", "iface": "", "source": "conntrack", "online": true}
				byIP[ip] = m
			}
			if sess.MAC != "" && (m["mac"] == nil || m["mac"] == "") {
				m["mac"] = strings.ToLower(sess.MAC)
			}
			if !strings.Contains(m["source"].(string), "conntrack") {
				m["source"] = m["source"].(string) + ",conntrack"
			}
		}
	}

	terminals := make([]map[string]interface{}, 0, len(byIP))
	for _, m := range byIP {
		terminals = append(terminals, m)
	}

	// 分页（可选）：无 limit 参数时全量返回，保持向后兼容。
	total := len(terminals)
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")
	offset := 0
	if offsetStr != "" {
		if n, err := strconv.Atoi(offsetStr); err == nil && n >= 0 {
			offset = n
		}
	}
	hasMore := false
	if limitStr != "" {
		limit := 200
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
			limit = n
		}
		if limit > 1000 {
			limit = 1000
		}
		if offset > total {
			offset = total
		}
		end := offset + limit
		if end > total {
			end = total
		}
		hasMore = offset+limit < total
		terminals = terminals[offset:end]
	}
	if terminals == nil {
		terminals = []map[string]interface{}{}
	}

	writeJSON(w, 200, map[string]interface{}{
		"count":     len(terminals),
		"total":     total,
		"offset":    offset,
		"has_more":  hasMore,
		"terminals": terminals,
	})
}
