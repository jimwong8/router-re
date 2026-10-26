package qos

import (
    "fmt"
    "strings"
)

type RateLimit struct {
	Interface string `json:"interface"` // "eth0", "lan", etc.
	RateMbps  int    `json:"rate_mbps"` // rate in Mbps
	BurstKB   int    `json:"burst_kb"`  // burst size in KB
	Direction string `json:"direction"` // "egress" or "ingress"
	Priority  int    `json:"priority"`  // 0-65535 (optional traffic-shaping priority)
}

// RenderTcCommands generates `tc` command strings for the given limits.
// Each limit produces either an egress TBF (tbf) rule or an ingress policing
// (police) rule depending on Direction.
func RenderTcCommands(limits []RateLimit) []string {
    cmds := make([]string, 0, len(limits)*2)
    for _, lim := range limits {
        iface := lim.Interface
        if iface == "" {
            iface = "eth0"
        }
        rateStr := fmt.Sprintf("%dmbit", lim.RateMbps)
        burstStr := fmt.Sprintf("%dkb", lim.BurstKB)
        if lim.Direction == "ingress" {
            cmds = append(cmds,
                fmt.Sprintf("tc qdisc add dev %s handle ffff: ingress", iface),
                fmt.Sprintf("tc filter add dev %s parent ffff: protocol ip prio 1 u32 match ip src 0.0.0.0/0 police rate %s burst %s drop flowid :1", iface, rateStr, burstStr),
            )
        } else {
        	cmds = append(cmds,
        		fmt.Sprintf("tc qdisc replace dev %s root tbf rate %s burst %s latency 50ms", iface, rateStr, burstStr),
        	)
        }
    }
    return cmds
}

// RenderNftLimit renders the given limits as nftables rules text
// (alternative to tc). The output contains the "limit rate" phrase.
func RenderNftLimit(limits []RateLimit) string {
    var b strings.Builder
    b.WriteString("table inet qos_limit {\n")
    for _, lim := range limits {
        iface := lim.Interface
        if iface == "" {
            iface = "eth0"
        }
        dir := lim.Direction
        if dir == "" || dir == "egress" {
            b.WriteString(fmt.Sprintf("  chain output { limit rate %d mbytes/second; }\n", lim.RateMbps))
        } else if dir == "ingress" {
            b.WriteString(fmt.Sprintf("  chain input { limit rate %d mbytes/second; }\n", lim.RateMbps))
        }
    }
    b.WriteString("}\n")
    return b.String()
}
