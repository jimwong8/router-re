package client

import (
	"strings"
	"testing"

	"github.com/jimwong/router-re/mgmtd/internal/dhcp"
)

func TestListParsesIPNeigh(t *testing.T) {
	r := &fakeExec{}
	r.addResponse("ip neigh show", `10.0.0.2 dev eth0 lladdr aa:bb:cc:dd:ee:ff REACHABLE
10.0.0.3 dev eth0  STALE`)

	clients, err := List(r)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(clients) != 1 {
		t.Fatalf("expected 1 client (only line with lladdr), got %d", len(clients))
	}
	c := clients[0]
	if c.IP != "10.0.0.2" {
		t.Errorf("IP got %q want %q", c.IP, "10.0.0.2")
	}
	if c.MAC != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("MAC got %q want %q", c.MAC, "aa:bb:cc:dd:ee:ff")
	}
	if c.State != "REACHABLE" {
		t.Errorf("State got %q want %q", c.State, "REACHABLE")
	}
}

func TestMergeFillsHostname(t *testing.T) {
	neigh := []Client{
		{IP: "192.168.1.100", MAC: "11:22:33:44:55:66", Hostname: ""},
	}
	leases := []dhcp.Lease{
		{IP: "192.168.1.100", Hostname: "laptop-01"},
	}
	merged := Merge(neigh, leases)
	if len(merged) != 1 || merged[0].Hostname != "laptop-01" {
		t.Errorf("Merge hostname failed: got %+v", merged)
	}
}

// fakeExec 是用于测试的 execx.Runner 替代，实现中记录调用并返回预设响应。
type fakeExec struct {
	responses map[string]string
}

func (f *fakeExec) addResponse(cmdLine, out string) {
	if f.responses == nil {
		f.responses = make(map[string]string)
	}
	f.responses[cmdLine] = out
}

func (f *fakeExec) Run(name string, args ...string) (string, error) {
	key := name + " " + strings.Join(args, " ")
	if out, ok := f.responses[key]; ok {
		return out, nil
	}
	return "", nil
}

func (f *fakeExec) RunWithInput(input, name string, args ...string) (string, error) {
	return f.Run(name, args...)
}
