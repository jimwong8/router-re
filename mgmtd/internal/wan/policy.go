package wan

import (
	"fmt"
	"sync"
)

// WANLink describes a single upstream WAN connection.
type WANLink struct {
	Name         string `json:"name"`
	Interface    string `json:"interface"`
	Gateway      string `json:"gateway"`
	Weight       int    `json:"weight"`
	Table        int    `json:"table"`
	HealthTarget string `json:"health_target"`
}

// Balancer performs weighted round-robin selection across WAN links.
type Balancer struct {
	mu     sync.Mutex
	cursor int
}

// NewBalancer creates a ready-to-use Balancer.
func NewBalancer() *Balancer { return &Balancer{} }

// PickLink selects one WAN link using weighted round-robin.
// Links with Weight <= 0 are treated as weight 1.
func (b *Balancer) PickLink(links []WANLink) WANLink {
	b.mu.Lock()
	defer b.mu.Unlock()

	total := 0
	for _, l := range links {
		w := l.Weight
		if w <= 0 {
			w = 1
		}
		total += w
	}
	if total <= 0 || len(links) == 0 {
		if len(links) > 0 {
			return links[0]
		}
		return WANLink{}
	}

	b.cursor = (b.cursor + 1) % total
	// find which link owns the cursor position
	pos := 0
	for _, l := range links {
		w := l.Weight
		if w <= 0 {
			w = 1
		}
		if b.cursor < pos+w {
			return l
		}
		pos += w
	}
	// fallback (shouldn't happen)
	return links[len(links)-1]
}

// RenderPolicyRouting generates the iproute2 commands needed to set up
// policy routing for the given WAN links. Each link gets its own routing
// table plus fwmark/from rules. The returned slice is directly executable.
func RenderPolicyRouting(links []WANLink) []string {
	cmds := []string{}
	for _, l := range links {
		tbl := l.Table
		if tbl <= 0 {
			tbl = 100 // sensible default table number
		}
		mark := tbl
		iface := l.Interface
		gw := l.Gateway
		if iface == "" {
			iface = l.Name
		}
		cmds = append(cmds, fmt.Sprintf("ip route add default via %s dev %s table %d", gw, iface, tbl))
		cmds = append(cmds, fmt.Sprintf("ip rule add fwmark %d lookup %d", mark, tbl))
		// NOTE: no "ip rule add from 0.0.0.0/0" here — that would match every
		// packet and black-hole all later links. Traffic is steered into a
		// table by setting fwmark (nftables/iptables), not by a catch-all rule.
	}
	return cmds
}
