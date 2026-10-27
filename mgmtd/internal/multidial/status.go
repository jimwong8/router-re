package multidial

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Status is the runtime snapshot of one PPPoE session.
type Status struct {
	Name    string `json:"name"`
	Iface   string `json:"iface"`
	Up      bool   `json:"up"`
	LocalIP string `json:"local_ip"`
	PeerIP  string `json:"peer_ip"`
	Uptime  string `json:"uptime"`
	RxBytes uint64 `json:"rx_bytes"`
	TxBytes uint64 `json:"tx_bytes"`
}

// ParseIfconfig parses `ip -j addr show <iface>` JSON. The PPP interface's
// local address is the scope=global inet entry; the peer address is the
// scope=link inet entry. If the interface does not exist (empty JSON array),
// Up=false is returned instead of an error.
func ParseIfconfig(jsonOut []byte, iface string) (Status, error) {
	s := Status{Iface: iface}
	var raw []struct {
		Ifname   string `json:"ifname"`
		AddrInfo []struct {
			Family string `json:"family"`
			Local  string `json:"local"`
			Scope  string `json:"scope"`
		} `json:"addr_info"`
	}
	if err := json.Unmarshal(jsonOut, &raw); err != nil {
		return s, fmt.Errorf("parse ip -j addr show: %w", err)
	}
	for _, e := range raw {
		if e.Ifname != iface {
			continue
		}
		for _, ai := range e.AddrInfo {
			if ai.Family != "inet" {
				continue
			}
			if ai.Scope == "link" {
				s.PeerIP = ai.Local
			} else if ai.Scope == "global" || ai.Scope == "" {
				if s.LocalIP == "" {
					s.LocalIP = ai.Local
				}
			}
		}
		if s.LocalIP != "" {
			s.Up = true
		}
	}
	return s, nil
}

// ParsePppdProcs parses `ps -eo pid,args` output and maps session names to
// PIDs by looking for `pppd call <name>` invocations.
func ParsePppdProcs(psOut string) map[string]int {
	res := map[string]int{}
	re := regexp.MustCompile(`(\d+)\s+.*pppd\s+.*call\s+(\S+)`)
	for _, line := range strings.Split(psOut, "\n") {
		m := re.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		pid, _ := strconv.Atoi(m[1])
		res[m[2]] = pid
	}
	return res
}
