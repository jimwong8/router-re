package system

import (
	"strings"
	"testing"
)

func TestGetInfoHostname(t *testing.T) {
	info := GetInfo()
	if info.Hostname == "" {
		t.Fatal("hostname should be non-empty")
	}
	if info.CPUCores <= 0 {
		t.Fatalf("cpu cores should be >0, got %d", info.CPUCores)
	}
}

func TestGetInfoCPUModel(t *testing.T) {
	info := GetInfo()
	// model name may be empty on exotic /proc, but on Linux it should usually exist
	if info.CPUModel == "" {
		t.Log("cpu model empty (tolerated)")
	}
}

func TestParseNetDev(t *testing.T) {
	data := `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
  eth0: 1000     10     0    0    0     0          0         0     2000      20     0    0    0     0       0          0
    lo:  99      9     0    0    0     0          0         0       99       9     0    0    0     0       0          0
`
	m := map[string]counters{}
	m = parseNetDev(strings.NewReader(data), m)
	c, ok := m["eth0"]
	if !ok {
		t.Fatal("eth0 missing")
	}
	if c.rx != 1000 || c.tx != 2000 {
		t.Fatalf("eth0 counters wrong: rx=%d tx=%d", c.rx, c.tx)
	}
}

func TestGetInterfacesNonEmpty(t *testing.T) {
	ifaces := GetInterfaces()
	if len(ifaces) == 0 {
		t.Fatal("expected at least one interface")
	}
	for _, ifc := range ifaces {
		if ifc.Name == "" {
			t.Fatal("interface name should be non-empty")
		}
	}
}
