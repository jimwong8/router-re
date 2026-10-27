package wan

import (
	"testing"
)

type FakeHealthRunner struct {
	Output string
}

func (f *FakeHealthRunner) Run(name string, args ...string) (string, error) {
	return f.Output, nil
}

func (f *FakeHealthRunner) RunWithInput(input, name string, args ...string) (string, error) {
	return f.Output, nil
}

func TestParseTxRxAvgFullSuccess(t *testing.T) {
	out := "PING 192.168.1.1 (192.168.1.1) 56(84) bytes of data.\n64 bytes from 192.168.1.1: icmp_seq=1 ttl=64 time=0.505 ms\n64 bytes from 192.168.1.1: icmp_seq=2 ttl=64 time=0.489 ms\n64 bytes from 192.168.1.1: icmp_seq=3 ttl=64 time=0.510 ms\n--- 192.168.1.1 ping statistics ---\n3 packets transmitted, 3 received, 0% packet loss, time 2003ms\nrtt min/avg/max/mdev = 0.489/0.501/0.510/0.010 ms"
	tx, rx, avg := parseTxRxAvg(out)
	if tx != 3 {
		t.Fatalf("expected tx=3, got %d", tx)
	}
	if rx != 3 {
		t.Fatalf("expected rx=3, got %d", rx)
	}
	if avg < 0.4 || avg > 0.6 {
		t.Fatalf("expected avg ~0.501, got %f", avg)
	}
}

func TestParseFullLoss(t *testing.T) {
	out := "PING 10.0.0.99 (10.0.0.99) 56(84) bytes of data.\n--- 10.0.0.99 ping statistics ---\n3 packets transmitted, 0 received, 100% packet loss, time 2011ms"
	tx, rx, avg := parseTxRxAvg(out)
	if tx != 3 {
		t.Fatalf("expected tx=3, got %d", tx)
	}
	if rx != 0 {
		t.Fatalf("expected rx=0, got %d", rx)
	}
	_ = avg // avg is 0 when there is no rtt line; fine.
}

func TestCheckerUsesRunner(t *testing.T) {
	fr := &FakeHealthRunner{
		Output: "3 packets transmitted, 3 received, 0% packet loss, time 200ms\nrtt min/avg/max/mdev = 0.5/0.7/1.0/0.2 ms",
	}
	c := Checker{R: fr, Count: 2}
	h := c.Check([]WANLink{{Name: "w1", Interface: "w1", Gateway: "1.1.1.1"}})
	if len(h) != 1 {
		t.Fatalf("expected 1 result, got %d", len(h))
	}
	if h[0].Name != "w1" {
		t.Fatalf("expected name w1, got %s", h[0].Name)
	}
	if !h[0].Alive {
		t.Fatal("expected alive=true with 3/3 packets")
	}
}
