package system

import (
	"bufio"
	"io"
	"net"
	"os"
	"strings"
)

// Interface describes a network interface with its name and byte counters.
type Interface struct {
	Name    string `json:"name"`
	IP      string `json:"ipaddr"`
	RxBytes uint64 `json:"rx_bytes"`
	TxBytes uint64 `json:"tx_bytes"`
}

// GetInterfaces reads the real list of network interfaces. It uses net.Interfaces
// for the interface list, falling back to /proc/net/dev for the counters, and
// best-effort fills a primary IPv4 address per interface.
func GetInterfaces() []Interface {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	counters := readNetDev()
	out := make([]Interface, 0, len(ifaces))
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue // skip down interfaces
		}
		// skip loopback for brevity
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		ip := firstIPv4(iface)
		c := counters[iface.Name]
		out = append(out, Interface{
			Name:    iface.Name,
			IP:      ip,
			RxBytes: c.rx,
			TxBytes: c.tx,
		})
	}
	return out
}

func firstIPv4(iface net.Interface) string {
	addrs, err := iface.Addrs()
	if err != nil {
		return ""
	}
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok {
			if v4 := ipnet.IP.To4(); v4 != nil {
				return v4.String()
			}
		}
	}
	return ""
}

type counters struct {
	rx, tx uint64
}

// readNetDev parses /proc/net/dev into a name->counters map.
func readNetDev() map[string]counters {
	m := map[string]counters{}
	f, err := openProcNetDev()
	if err != nil {
		return m
	}
	defer f.Close()
	return parseNetDev(f, m)
}

func openProcNetDev() (*os.File, error) {
	return os.Open("/proc/net/dev")
}

func parseNetDev(r io.Reader, m map[string]counters) map[string]counters {
	sc := bufio.NewScanner(r)
	n := 0
	for sc.Scan() {
		n++
		if n <= 2 { // header lines
			continue
		}
		line := sc.Text()
		// format: "  eth0: 123456 0 0 … 0 654321 … 0 0"
		i := strings.IndexByte(line, ':')
		if i < 0 {
			continue
		}
		name := strings.TrimSpace(line[:i])
		rest := strings.Fields(line[i+1:])
		if len(rest) >= 16 {
			var rx, tx uint64
			parseUint(rest[0], &rx)
			parseUint(rest[8], &tx)
			m[name] = counters{rx: rx, tx: tx}
		}
	}
	return m
}

func parseUint(s string, out *uint64) {
	var v uint64
	for _, r := range s {
		if r < '0' || r > '9' {
			return
		}
		v = v*10 + uint64(r-'0')
	}
	*out = v
}
