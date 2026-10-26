package route

import (
	"fmt"
	"strings"
)

// BGPPeer describes a single BGP neighbor.
type BGPPeer struct {
	Address  string `json:"address"`
	RemoteAS int    `json:"remote_as"`
	Password string `json:"password,omitempty"`
	MultiHop bool   `json:"multi_hop,omitempty"`
}

// BGPConfig describes a BGP instance for FRR.
type BGPConfig struct {
	LocalAS  int       `json:"local_as"`
	RouterID string    `json:"router_id"`
	Peers    []BGPPeer `json:"peers"`
	Networks []string  `json:"networks"`
}

// OSPFConfig describes an OSPF instance for FRR.
type OSPFConfig struct {
	RouterID string   `json:"router_id"`
	Networks []string `json:"networks"`
	Area     string   `json:"area"`
}

// RenderBGP renders the BGP section of an FRR configuration.
func RenderBGP(cfg BGPConfig) string {
	var b strings.Builder
	if cfg.LocalAS <= 0 {
		return ""
	}
	b.WriteString(fmt.Sprintf("router bgp %d\n", cfg.LocalAS))
	if cfg.RouterID != "" {
		b.WriteString(fmt.Sprintf(" bgp router-id %s\n", cfg.RouterID))
	}
	for _, p := range cfg.Peers {
		if p.Address == "" {
			continue
		}
		b.WriteString(fmt.Sprintf(" neighbor %s remote-as %d\n", p.Address, p.RemoteAS))
		if p.Password != "" {
			b.WriteString(fmt.Sprintf(" neighbor %s password %s\n", p.Address, p.Password))
		}
		if p.MultiHop {
			b.WriteString(fmt.Sprintf(" neighbor %s ebgp-multihop 2\n", p.Address))
		}
	}
	b.WriteString(" address-family ipv4 unicast\n")
	for _, n := range cfg.Networks {
		b.WriteString(fmt.Sprintf("  network %s\n", n))
	}
	for _, p := range cfg.Peers {
		if p.Address == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("  neighbor %s activate\n", p.Address))
	}
	b.WriteString(" exit-address-family\n")
	return b.String()
}

// RenderOSPF renders the OSPF section of an FRR configuration.
func RenderOSPF(cfg OSPFConfig) string {
	var b strings.Builder
	b.WriteString("router ospf\n")
	if cfg.RouterID != "" {
		b.WriteString(fmt.Sprintf(" ospf router-id %s\n", cfg.RouterID))
	}
	area := cfg.Area
	if area == "" {
		area = "0"
	}
	for _, n := range cfg.Networks {
		b.WriteString(fmt.Sprintf(" network %s area %s\n", n, area))
	}
	return b.String()
}

// RenderFrrConf combines BGP and OSPF configs into a complete frr.conf.
func RenderFrrConf(bgp *BGPConfig, ospf *OSPFConfig) string {
	var b strings.Builder
	b.WriteString("frr version 8.4\n")
	b.WriteString("frr defaults traditional\n")
	b.WriteString("hostname router\n")
	b.WriteString("log syslog informational\n")
	if bgp != nil {
		b.WriteString("!\n")
		b.WriteString(RenderBGP(*bgp))
	}
	if ospf != nil {
		b.WriteString("!\n")
		b.WriteString(RenderOSPF(*ospf))
	}
	b.WriteString("!\n")
	b.WriteString("line vty\n")
	return b.String()
}
